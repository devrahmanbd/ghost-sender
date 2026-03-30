package proxy

import (
        "context"
        "errors"
        "fmt"
        "sync"
        "time"

        "email-campaign-system/internal/models"
        "email-campaign-system/internal/storage/repository"
)

var (
        ErrProxyNotFound      = errors.New("proxy not found")
        ErrProxyDisabled      = errors.New("proxy is disabled")
        ErrNoProxiesAvailable = errors.New("no proxies available")
        ErrInvalidProxyType   = errors.New("invalid proxy type")
        ErrProxyPoolClosed    = errors.New("proxy pool closed")
)

type ProxyManager struct {
        mu           sync.RWMutex
        proxies      map[string]*ProxyEntry
        repo         repository.ProxyRepository
        validator    *ProxyValidator
        rotator      *ProxyRotator
        config       *ProxyConfig
        stats        *ProxyStats
        closed       bool
        eventChan    chan ProxyEvent
        healthTicker *time.Ticker
        ctx          context.Context
        cancel       context.CancelFunc
}

type ProxyEntry struct {
        Proxy        *repository.Proxy
        Health       *ProxyHealth
        LastUsed     time.Time
        UsageCount   int64
        FailureCount int64
        SuccessCount int64
        mu           sync.RWMutex
}

type ProxyConfig struct {
        HealthCheckInterval time.Duration
        HealthCheckTimeout  time.Duration
        MaxFailures         int
        RetryDelay          time.Duration
        EnableAutoDisable   bool
        EnableHealthCheck   bool
        PoolSize            int
}

type ProxyStats struct {
        mu               sync.RWMutex
        TotalProxies     int
        ActiveProxies    int
        DisabledProxies  int
        HealthyProxies   int
        UnhealthyProxies int
        TotalRequests    int64
        SuccessRequests  int64
        FailedRequests   int64
}

type ProxyEvent struct {
        Type      ProxyEventType
        ProxyID   string
        Timestamp time.Time
        Data      interface{}
}

type ProxyEventType string

const (
        EventProxyAdded     ProxyEventType = "proxy_added"
        EventProxyRemoved   ProxyEventType = "proxy_removed"
        EventProxyDisabled  ProxyEventType = "proxy_disabled"
        EventProxyEnabled   ProxyEventType = "proxy_enabled"
        EventProxyHealthy   ProxyEventType = "proxy_healthy"
        EventProxyUnhealthy ProxyEventType = "proxy_unhealthy"
)

func NewProxyManager(repo repository.ProxyRepository, config *ProxyConfig) (*ProxyManager, error) {
    // REMOVED: reflect nil check — repo is a value type, never nil

    if config == nil {
        config = DefaultProxyConfig()
    }

    ctx, cancel := context.WithCancel(context.Background())

    pm := &ProxyManager{
        proxies:   make(map[string]*ProxyEntry),
        repo:      repo,
        validator: NewProxyValidator(nil),
        config:    config,
        stats:     &ProxyStats{},
        eventChan: make(chan ProxyEvent, 100),
        ctx:       ctx,
        cancel:    cancel,
    }

    pm.rotator = NewProxyRotator(pm)

    if err := pm.loadProxies(); err != nil {
        cancel()
        return nil, fmt.Errorf("failed to load proxies: %w", err)
    }

    if config.EnableHealthCheck {
        pm.startHealthChecker()
    }

    return pm, nil
}


func DefaultProxyConfig() *ProxyConfig {
        return &ProxyConfig{
                HealthCheckInterval: 5 * time.Minute,
                HealthCheckTimeout:  10 * time.Second,
                MaxFailures:         3,
                RetryDelay:          30 * time.Second,
                EnableAutoDisable:   true,
                EnableHealthCheck:   true,
                PoolSize:            100,
        }
}

func (pm *ProxyManager) loadProxies() error {
        proxies, _, err := pm.repo.List(pm.ctx, &repository.ProxyFilter{})
        if err != nil {
                return err
        }

        pm.mu.Lock()
        defer pm.mu.Unlock()

        for i := range proxies {
                // Use persisted health status from the database if available,
                // otherwise default to unknown.
                healthStatus := HealthStatusUnknown
                if proxies[i].HealthStatus != "" {
                        healthStatus = HealthStatus(proxies[i].HealthStatus)
                }
                entry := &ProxyEntry{
                        Proxy:  proxies[i],
                        Health: &ProxyHealth{Status: healthStatus},
                }
                pm.proxies[proxies[i].ID] = entry
        }

        pm.updateStats()
        return nil
}

func (pm *ProxyManager) Add(proxy *repository.Proxy) error {
        pm.mu.Lock()
        defer pm.mu.Unlock()

        if pm.closed {
                return ErrProxyPoolClosed
        }

        // Convert repository.Proxy to models.Proxy for validation
        modelProxy := pm.convertToModelProxy(proxy)
        if err := pm.validator.Validate(modelProxy); err != nil {
                return fmt.Errorf("proxy validation failed: %w", err)
        }

        if err := pm.repo.Create(pm.ctx, proxy); err != nil {
                return fmt.Errorf("failed to save proxy: %w", err)
        }

        entry := &ProxyEntry{
                Proxy:  proxy,
                Health: &ProxyHealth{Status: HealthStatusUnknown},
        }

        pm.proxies[proxy.ID] = entry
        pm.updateStats()

        pm.sendEvent(ProxyEvent{
                Type:      EventProxyAdded,
                ProxyID:   proxy.ID,
                Timestamp: time.Now(),
                Data:      proxy,
        })

        return nil
}

func (pm *ProxyManager) Remove(proxyID string) error {
        pm.mu.Lock()
        defer pm.mu.Unlock()

        if pm.closed {
                return ErrProxyPoolClosed
        }

        entry, exists := pm.proxies[proxyID]
        if !exists {
                return ErrProxyNotFound
        }

        if err := pm.repo.Delete(pm.ctx, proxyID); err != nil {
                return fmt.Errorf("failed to delete proxy: %w", err)
        }

        delete(pm.proxies, proxyID)
        pm.updateStats()

        pm.sendEvent(ProxyEvent{
                Type:      EventProxyRemoved,
                ProxyID:   proxyID,
                Timestamp: time.Now(),
                Data:      entry.Proxy,
        })

        return nil
}

func (pm *ProxyManager) Get(proxyID string) (*repository.Proxy, error) {
        pm.mu.RLock()
        defer pm.mu.RUnlock()

        if pm.closed {
                return nil, ErrProxyPoolClosed
        }

        entry, exists := pm.proxies[proxyID]
        if !exists {
                return nil, ErrProxyNotFound
        }

        return entry.Proxy, nil
}

func (pm *ProxyManager) GetNext() (*repository.Proxy, error) {
        pm.mu.RLock()
        defer pm.mu.RUnlock()

        if pm.closed {
                return nil, ErrProxyPoolClosed
        }

        // Convert from models.Proxy to repository.Proxy
        modelProxy, err := pm.rotator.Next()
        if err != nil {
                return nil, err
        }

        // Find the corresponding repository proxy
        if modelProxy != nil {
                entry, exists := pm.proxies[modelProxy.ID]
                if exists {
                        return entry.Proxy, nil
                }
        }

        return nil, ErrNoProxiesAvailable
}

func (pm *ProxyManager) GetAll() []*repository.Proxy {
        pm.mu.RLock()
        defer pm.mu.RUnlock()

        proxies := make([]*repository.Proxy, 0, len(pm.proxies))
        for _, entry := range pm.proxies {
                proxies = append(proxies, entry.Proxy)
        }

        return proxies
}

func (pm *ProxyManager) GetHealthy() []*repository.Proxy {
        pm.mu.RLock()
        defer pm.mu.RUnlock()

        proxies := make([]*repository.Proxy, 0)
        for _, entry := range pm.proxies {
                entry.mu.RLock()
                if entry.Proxy.IsActive && entry.Health.Status == HealthStatusHealthy {
                        proxies = append(proxies, entry.Proxy)
                }
                entry.mu.RUnlock()
        }

        return proxies
}

// GetUnhealthy returns proxies whose live health status is unhealthy.
// It reads entry.Health.Status (updated by the health-check loop) rather
// than entry.Proxy.Status (the stale DB field) so the result is accurate.
func (pm *ProxyManager) GetUnhealthy() []*repository.Proxy {
        pm.mu.RLock()
        defer pm.mu.RUnlock()

        proxies := make([]*repository.Proxy, 0)
        for _, entry := range pm.proxies {
                entry.mu.RLock()
                if entry.Health.Status == HealthStatusUnhealthy {
                        proxies = append(proxies, entry.Proxy)
                }
                entry.mu.RUnlock()
        }

        return proxies
}

func (pm *ProxyManager) GetByType(proxyType string) []*repository.Proxy {
        pm.mu.RLock()
        defer pm.mu.RUnlock()

        proxies := make([]*repository.Proxy, 0)
        for _, entry := range pm.proxies {
                if entry.Proxy.Type == proxyType && entry.Proxy.IsActive {
                        proxies = append(proxies, entry.Proxy)
                }
        }

        return proxies
}

func (pm *ProxyManager) Update(proxy *repository.Proxy) error {
        pm.mu.Lock()
        defer pm.mu.Unlock()

        if pm.closed {
                return ErrProxyPoolClosed
        }

        entry, exists := pm.proxies[proxy.ID]
        if !exists {
                return ErrProxyNotFound
        }

        // Convert repository.Proxy to models.Proxy for validation
        modelProxy := pm.convertToModelProxy(proxy)
        if err := pm.validator.Validate(modelProxy); err != nil {
                return fmt.Errorf("proxy validation failed: %w", err)
        }

        if err := pm.repo.Update(pm.ctx, proxy); err != nil {
                return fmt.Errorf("failed to update proxy: %w", err)
        }

        entry.mu.Lock()
        entry.Proxy = proxy
        entry.mu.Unlock()

        pm.updateStats()

        return nil
}

func (pm *ProxyManager) Enable(proxyID string) error {
        pm.mu.Lock()
        defer pm.mu.Unlock()

        if pm.closed {
                return ErrProxyPoolClosed
        }

        entry, exists := pm.proxies[proxyID]
        if !exists {
                return ErrProxyNotFound
        }

        entry.mu.Lock()
        entry.Proxy.IsActive = true
        entry.mu.Unlock()

        if err := pm.repo.Update(pm.ctx, entry.Proxy); err != nil {
                return fmt.Errorf("failed to enable proxy: %w", err)
        }

        pm.updateStats()

        pm.sendEvent(ProxyEvent{
                Type:      EventProxyEnabled,
                ProxyID:   proxyID,
                Timestamp: time.Now(),
        })

        return nil
}

func (pm *ProxyManager) Disable(proxyID string) error {
        pm.mu.Lock()
        defer pm.mu.Unlock()

        if pm.closed {
                return ErrProxyPoolClosed
        }

        entry, exists := pm.proxies[proxyID]
        if !exists {
                return ErrProxyNotFound
        }

        entry.mu.Lock()
        entry.Proxy.IsActive = false
        entry.mu.Unlock()

        if err := pm.repo.Update(pm.ctx, entry.Proxy); err != nil {
                return fmt.Errorf("failed to disable proxy: %w", err)
        }

        pm.updateStats()

        pm.sendEvent(ProxyEvent{
                Type:      EventProxyDisabled,
                ProxyID:   proxyID,
                Timestamp: time.Now(),
        })

        return nil
}

func (pm *ProxyManager) Test(proxyID string) (*ProxyHealth, error) {
        pm.mu.RLock()
        entry, exists := pm.proxies[proxyID]
        pm.mu.RUnlock()

        if !exists {
                return nil, ErrProxyNotFound
        }

        ctx, cancel := context.WithTimeout(pm.ctx, pm.config.HealthCheckTimeout)
        defer cancel()

        // Convert repository.Proxy to models.Proxy for health check
        modelProxy := pm.convertToModelProxy(entry.Proxy)
        health, err := pm.validator.CheckHealth(ctx, modelProxy)
        if err != nil {
                return nil, fmt.Errorf("health check failed: %w", err)
        }

        entry.mu.Lock()
        entry.Health = health
        // Persist the health status to the database so it survives app restarts.
        // This allows the "Remove Unhealthy" button to work correctly.
        entry.Proxy.HealthStatus = string(health.Status)
        entry.mu.Unlock()

        // Save the updated health status to the database (best-effort).
        // If it fails, the in-memory status is still used; persistence is for
        // recovery after app restart.
        _ = pm.repo.Update(pm.ctx, entry.Proxy)

        return health, nil
}

func (pm *ProxyManager) RecordSuccess(proxyID string) {
        pm.mu.RLock()
        entry, exists := pm.proxies[proxyID]
        pm.mu.RUnlock()

        if !exists {
                return
        }

        entry.mu.Lock()
        entry.SuccessCount++
        entry.LastUsed = time.Now()
        entry.UsageCount++
        entry.mu.Unlock()

        pm.stats.mu.Lock()
        pm.stats.SuccessRequests++
        pm.stats.TotalRequests++
        pm.stats.mu.Unlock()
}

func (pm *ProxyManager) RecordFailure(proxyID string) {
        pm.mu.RLock()
        entry, exists := pm.proxies[proxyID]
        pm.mu.RUnlock()

        if !exists {
                return
        }

        entry.mu.Lock()
        entry.FailureCount++
        entry.LastUsed = time.Now()
        entry.UsageCount++
        entry.mu.Unlock()

        pm.stats.mu.Lock()
        pm.stats.FailedRequests++
        pm.stats.TotalRequests++
        pm.stats.mu.Unlock()

        if pm.config.EnableAutoDisable && entry.FailureCount >= int64(pm.config.MaxFailures) {
                pm.Disable(proxyID)
        }
}

func (pm *ProxyManager) GetStats() ProxyStats {
        pm.stats.mu.RLock()
        defer pm.stats.mu.RUnlock()

        return ProxyStats{
                TotalProxies:     pm.stats.TotalProxies,
                ActiveProxies:    pm.stats.ActiveProxies,
                DisabledProxies:  pm.stats.DisabledProxies,
                HealthyProxies:   pm.stats.HealthyProxies,
                UnhealthyProxies: pm.stats.UnhealthyProxies,
                TotalRequests:    pm.stats.TotalRequests,
                SuccessRequests:  pm.stats.SuccessRequests,
                FailedRequests:   pm.stats.FailedRequests,
        }
}

func (pm *ProxyManager) GetProxyStats(proxyID string) (*ProxyEntry, error) {
        pm.mu.RLock()
        defer pm.mu.RUnlock()

        entry, exists := pm.proxies[proxyID]
        if !exists {
                return nil, ErrProxyNotFound
        }

        entry.mu.RLock()
        defer entry.mu.RUnlock()

        return &ProxyEntry{
                Proxy:        entry.Proxy,
                Health:       entry.Health,
                LastUsed:     entry.LastUsed,
                UsageCount:   entry.UsageCount,
                FailureCount: entry.FailureCount,
                SuccessCount: entry.SuccessCount,
        }, nil
}

func (pm *ProxyManager) startHealthChecker() {
        pm.healthTicker = time.NewTicker(pm.config.HealthCheckInterval)

        go func() {
                for {
                        select {
                        case <-pm.healthTicker.C:
                                pm.performHealthChecks()
                        case <-pm.ctx.Done():
                                return
                        }
                }
        }()
}

func (pm *ProxyManager) performHealthChecks() {
        pm.mu.RLock()
        entries := make([]*ProxyEntry, 0, len(pm.proxies))
        for _, entry := range pm.proxies {
                if entry.Proxy.IsActive {
                        entries = append(entries, entry)
                }
        }
        pm.mu.RUnlock()

        var wg sync.WaitGroup
        semaphore := make(chan struct{}, 10)

        for _, entry := range entries {
                wg.Add(1)
                go func(e *ProxyEntry) {
                        defer wg.Done()

                        semaphore <- struct{}{}
                        defer func() { <-semaphore }()

                        ctx, cancel := context.WithTimeout(pm.ctx, pm.config.HealthCheckTimeout)
                        defer cancel()

                        // Convert repository.Proxy to models.Proxy for health check
                        modelProxy := pm.convertToModelProxy(e.Proxy)
                        health, err := pm.validator.CheckHealth(ctx, modelProxy)

                        e.mu.Lock()
                        if err != nil {
                                e.Health = &ProxyHealth{
                                        Status:    HealthStatusUnhealthy,
                                        Message:   err.Error(),
                                        CheckedAt: time.Now(),
                                }
                        } else {
                                e.Health = health
                        }
                        e.mu.Unlock()

                        if health != nil && health.Status == HealthStatusUnhealthy {
                                pm.sendEvent(ProxyEvent{
                                        Type:      EventProxyUnhealthy,
                                        ProxyID:   e.Proxy.ID,
                                        Timestamp: time.Now(),
                                        Data:      health,
                                })
                        }
                }(entry)
        }

        wg.Wait()
        pm.updateStats()
}

func (pm *ProxyManager) updateStats() {
        totalProxies := len(pm.proxies)
        activeProxies := 0
        disabledProxies := 0
        healthyProxies := 0
        unhealthyProxies := 0

        for _, entry := range pm.proxies {
                entry.mu.RLock()
                if entry.Proxy.IsActive {
                        activeProxies++
                        if entry.Health.Status == HealthStatusHealthy {
                                healthyProxies++
                        } else if entry.Health.Status == HealthStatusUnhealthy {
                                unhealthyProxies++
                        }
                } else {
                        disabledProxies++
                }
                entry.mu.RUnlock()
        }

        pm.stats.mu.Lock()
        pm.stats.TotalProxies = totalProxies
        pm.stats.ActiveProxies = activeProxies
        pm.stats.DisabledProxies = disabledProxies
        pm.stats.HealthyProxies = healthyProxies
        pm.stats.UnhealthyProxies = unhealthyProxies
        pm.stats.mu.Unlock()
}

func (pm *ProxyManager) sendEvent(event ProxyEvent) {
        select {
        case pm.eventChan <- event:
        default:
        }
}

func (pm *ProxyManager) Events() <-chan ProxyEvent {
        return pm.eventChan
}

func (pm *ProxyManager) Count() int {
        pm.mu.RLock()
        defer pm.mu.RUnlock()
        return len(pm.proxies)
}

func (pm *ProxyManager) CountHealthy() int {
        pm.mu.RLock()
        defer pm.mu.RUnlock()

        count := 0
        for _, entry := range pm.proxies {
                entry.mu.RLock()
                if entry.Proxy.IsActive && entry.Health.Status == HealthStatusHealthy {
                        count++
                }
                entry.mu.RUnlock()
        }

        return count
}

func (pm *ProxyManager) Clear() error {
        pm.mu.Lock()
        defer pm.mu.Unlock()

        if pm.closed {
                return ErrProxyPoolClosed
        }

        for proxyID := range pm.proxies {
                if err := pm.repo.Delete(pm.ctx, proxyID); err != nil {
                        return fmt.Errorf("failed to delete proxy %s: %w", proxyID, err)
                }
        }

        pm.proxies = make(map[string]*ProxyEntry)
        pm.updateStats()

        return nil
}

func (pm *ProxyManager) Refresh() error {
        pm.mu.Lock()
        defer pm.mu.Unlock()

        if pm.closed {
                return ErrProxyPoolClosed
        }

        pm.proxies = make(map[string]*ProxyEntry)
        return pm.loadProxies()
}

func (pm *ProxyManager) Close() error {
        pm.mu.Lock()
        defer pm.mu.Unlock()

        if pm.closed {
                return nil
        }

        pm.closed = true
        pm.cancel()

        if pm.healthTicker != nil {
                pm.healthTicker.Stop()
        }

        close(pm.eventChan)

        return nil
}

func (pm *ProxyManager) IsClosed() bool {
        pm.mu.RLock()
        defer pm.mu.RUnlock()
        return pm.closed
}

// Additional methods for extended functionality

func (pm *ProxyManager) List(ctx context.Context, filter *repository.ProxyFilter) ([]*repository.Proxy, int, error) {
        proxies, total, err := pm.repo.List(ctx, filter)
        if err != nil {
                return nil, 0, err
        }

        result := make([]*repository.Proxy, len(proxies))
        for i := range proxies {
                result[i] = proxies[i]
        }

        return result, total, nil
}

func (pm *ProxyManager) GetByID(ctx context.Context, id string) (*repository.Proxy, error) {
        return pm.repo.GetByID(ctx, id)
}

func (pm *ProxyManager) BulkEnable(proxyIDs []string) error {
        for _, id := range proxyIDs {
                if err := pm.Enable(id); err != nil {
                        return err
                }
        }
        return nil
}

func (pm *ProxyManager) BulkDisable(proxyIDs []string) error {
        for _, id := range proxyIDs {
                if err := pm.Disable(id); err != nil {
                        return err
                }
        }
        return nil
}

func (pm *ProxyManager) GetNextForMail() (*repository.Proxy, error) {
        return pm.GetNext()
}

func (pm *ProxyManager) GetRotator() *ProxyRotator {
        return pm.rotator
}

func (pm *ProxyManager) convertToModelProxy(repoProxy *repository.Proxy) *models.Proxy {
        return &models.Proxy{
                ID:       repoProxy.ID,
                Host:     repoProxy.Host,
                Port:     repoProxy.Port,
                Type:     models.ProxyType(repoProxy.Type),
                Username: repoProxy.Username,
                Password: repoProxy.Password,
        }
}
