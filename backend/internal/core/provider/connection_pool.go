package provider

import (
	"container/list"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/smtp"
	"sync"
	"time"

	"email-campaign-system/pkg/logger"
)

var (
	ErrPoolClosed      = errors.New("connection pool is closed")
	ErrPoolExhausted   = errors.New("connection pool exhausted")
	ErrConnectionDead  = errors.New("connection is dead")
	ErrAcquireTimeout  = errors.New("acquire connection timeout")
	ErrInvalidPoolSize = errors.New("invalid pool size configuration")
)

type PoolConfig struct {
	MinConnections    int
	MaxConnections    int
	MaxIdleTime       time.Duration
	MaxLifetime       time.Duration
	AcquireTimeout    time.Duration
	HealthCheckPeriod time.Duration
	CleanupPeriod     time.Duration
	WaitQueueTimeout  time.Duration
	ConnectionRetries int
	RetryDelay        time.Duration
}

type PooledConnection struct {
	conn       *smtp.Client
	pool       *SMTPConnectionPool
	createdAt  time.Time
	lastUsedAt time.Time
	usageCount int64
	isHealthy  bool
	mu         sync.Mutex
}

type SMTPConnectionPool struct {
	config      *PoolConfig
	smtpConfig  *ProviderConfig
	log         logger.Logger
	connections *list.List
	mu          sync.Mutex
	closed      bool
	activeCount int
	totalCount  int
	stats       *PoolStats
	statsMu     sync.RWMutex
	waitQueue   chan chan *PooledConnection
	stopCleanup chan struct{}
	stopHealth  chan struct{}
	wg          sync.WaitGroup
}

type PoolStats struct {
	TotalAcquired     int64
	TotalReleased     int64
	TotalCreated      int64
	TotalClosed       int64
	TotalFailedHealth int64
	CurrentActive     int64
	CurrentIdle       int64
	WaitQueueLength   int64
	AverageWaitTime   time.Duration
	totalWaitTime     time.Duration
	waitCount         int64
}

func NewConnectionPool(smtpConfig *ProviderConfig, poolConfig *PoolConfig, log logger.Logger) (*SMTPConnectionPool, error) {
	if poolConfig == nil {
		poolConfig = DefaultPoolConfig()
	}

	if err := validatePoolConfig(poolConfig); err != nil {
		return nil, err
	}

	pool := &SMTPConnectionPool{
		config:      poolConfig,
		smtpConfig:  smtpConfig,
		log:         log,
		connections: list.New(),
		stats:       &PoolStats{},
		waitQueue:   make(chan chan *PooledConnection, poolConfig.MaxConnections),
		stopCleanup: make(chan struct{}),
		stopHealth:  make(chan struct{}),
	}

	if err := pool.initializeMinConnections(); err != nil {
		return nil, fmt.Errorf("failed to initialize minimum connections: %w", err)
	}

	pool.startBackgroundTasks()

	return pool, nil
}

func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MinConnections:    2,
		MaxConnections:    10,
		MaxIdleTime:       5 * time.Minute,
		MaxLifetime:       30 * time.Minute,
		AcquireTimeout:    10 * time.Second,
		HealthCheckPeriod: 30 * time.Second,
		CleanupPeriod:     1 * time.Minute,
		WaitQueueTimeout:  30 * time.Second,
		ConnectionRetries: 3,
		RetryDelay:        1 * time.Second,
	}
}

func validatePoolConfig(config *PoolConfig) error {
	if config.MinConnections < 0 {
		return fmt.Errorf("%w: min connections cannot be negative", ErrInvalidPoolSize)
	}

	if config.MaxConnections < 1 {
		return fmt.Errorf("%w: max connections must be at least 1", ErrInvalidPoolSize)
	}

	if config.MinConnections > config.MaxConnections {
		return fmt.Errorf("%w: min connections cannot exceed max connections", ErrInvalidPoolSize)
	}

	return nil
}

func (p *SMTPConnectionPool) initializeMinConnections() error {
	for i := 0; i < p.config.MinConnections; i++ {
		conn, err := p.createConnection()
		if err != nil {
			if p.log != nil {
				(p.log).Warn(fmt.Sprintf("failed to create minimum connection at index %d: %v", i, err))
			}
			continue
		}

		p.connections.PushBack(conn)
		p.totalCount++
	}

	return nil
}

func (p *SMTPConnectionPool) createConnection() (*PooledConnection, error) {
	var conn *smtp.Client
	var err error

	for attempt := 0; attempt < p.config.ConnectionRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(p.config.RetryDelay)
		}

		conn, err = p.dialSMTP()
		if err == nil {
			break
		}

		if p.log != nil {
			(p.log).Warn(fmt.Sprintf("failed to create connection on attempt %d: %v", attempt+1, err))
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create connection after %d attempts: %w", p.config.ConnectionRetries, err)
	}

	pooledConn := &PooledConnection{
		conn:       conn,
		pool:       p,
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
		isHealthy:  true,
	}

	p.statsMu.Lock()
	p.stats.TotalCreated++
	p.statsMu.Unlock()

	return pooledConn, nil
}

func (p *SMTPConnectionPool) dialSMTP() (*smtp.Client, error) {
	addr := fmt.Sprintf("%s:%d", p.smtpConfig.Host, p.smtpConfig.Port)

	conn, err := smtp.Dial(addr)
	if err != nil {
		return nil, err
	}

	if p.smtpConfig.TLSConfig != nil && p.smtpConfig.TLSConfig.Enabled {
		tlsConfig := &tls.Config{
			ServerName:         p.smtpConfig.Host,
			InsecureSkipVerify: p.smtpConfig.TLSConfig.InsecureSkipVerify,
		}
		if err := conn.StartTLS(tlsConfig); err != nil {
			conn.Close()
			return nil, err
		}
	}

	auth := smtp.PlainAuth("", p.smtpConfig.Username, p.smtpConfig.Password, p.smtpConfig.Host)
	if err := conn.Auth(auth); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func (p *SMTPConnectionPool) Acquire(ctx context.Context) (*PooledConnection, error) {
	startTime := time.Now()

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}

	if p.connections.Len() > 0 {
		elem := p.connections.Front()
		conn := elem.Value.(*PooledConnection)
		p.connections.Remove(elem)
		p.activeCount++
		p.mu.Unlock()

		conn.mu.Lock()
		conn.lastUsedAt = time.Now()
		conn.usageCount++
		conn.mu.Unlock()

		p.recordAcquire(time.Since(startTime))

		if !p.isConnectionHealthy(conn) {
			if p.log != nil {
				(p.log).Warn("acquired unhealthy connection, creating new one")
			}
			p.closeConnection(conn)
			return p.Acquire(ctx)
		}

		return conn, nil
	}

	if p.totalCount < p.config.MaxConnections {
		p.totalCount++
		p.activeCount++
		p.mu.Unlock()

		conn, err := p.createConnection()
		if err != nil {
			p.mu.Lock()
			p.totalCount--
			p.activeCount--
			p.mu.Unlock()
			return nil, err
		}

		p.recordAcquire(time.Since(startTime))
		return conn, nil
	}

	p.mu.Unlock()

	return p.waitForConnection(ctx, startTime)
}

func (p *SMTPConnectionPool) waitForConnection(ctx context.Context, startTime time.Time) (*PooledConnection, error) {
	waitChan := make(chan *PooledConnection, 1)

	select {
	case p.waitQueue <- waitChan:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(p.config.WaitQueueTimeout):
		return nil, ErrPoolExhausted
	}

	p.statsMu.Lock()
	p.stats.WaitQueueLength++
	p.statsMu.Unlock()

	defer func() {
		p.statsMu.Lock()
		p.stats.WaitQueueLength--
		p.statsMu.Unlock()
	}()

	timeoutCtx, cancel := context.WithTimeout(ctx, p.config.AcquireTimeout)
	defer cancel()

	select {
	case conn := <-waitChan:
		if conn == nil {
			return nil, ErrPoolClosed
		}
		p.recordAcquire(time.Since(startTime))
		return conn, nil
	case <-timeoutCtx.Done():
		return nil, ErrAcquireTimeout
	}
}

func (p *SMTPConnectionPool) Release(conn *PooledConnection) error {
	if conn == nil {
		return errors.New("cannot release nil connection")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		p.closeConnection(conn)
		return ErrPoolClosed
	}

	p.activeCount--

	conn.mu.Lock()
	conn.lastUsedAt = time.Now()
	conn.mu.Unlock()

	if !p.isConnectionHealthy(conn) {
		p.totalCount--
		p.closeConnection(conn)
		return nil
	}

	select {
	case waitChan := <-p.waitQueue:
		waitChan <- conn
		p.activeCount++
	default:
		p.connections.PushBack(conn)
	}

	p.recordRelease()

	return nil
}

func (p *SMTPConnectionPool) isConnectionHealthy(conn *PooledConnection) bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if !conn.isHealthy {
		return false
	}

	age := time.Since(conn.createdAt)
	if age > p.config.MaxLifetime {
		return false
	}

	idleTime := time.Since(conn.lastUsedAt)
	if idleTime > p.config.MaxIdleTime {
		return false
	}

	if err := conn.conn.Noop(); err != nil {
		conn.isHealthy = false
		return false
	}

	return true
}

func (p *SMTPConnectionPool) closeConnection(conn *PooledConnection) {
	if conn == nil || conn.conn == nil {
		return
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	conn.conn.Quit()
	conn.conn.Close()
	conn.isHealthy = false

	p.statsMu.Lock()
	p.stats.TotalClosed++
	p.statsMu.Unlock()
}

func (p *SMTPConnectionPool) startBackgroundTasks() {
	p.wg.Add(2)

	go p.cleanupLoop()
	go p.healthCheckLoop()
}

func (p *SMTPConnectionPool) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.CleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanup()
		case <-p.stopCleanup:
			return
		}
	}
}

func (p *SMTPConnectionPool) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	var toRemove []*list.Element

	for elem := p.connections.Front(); elem != nil; elem = elem.Next() {
		conn := elem.Value.(*PooledConnection)

		if !p.isConnectionHealthy(conn) {
			toRemove = append(toRemove, elem)
		}
	}

	for _, elem := range toRemove {
		conn := elem.Value.(*PooledConnection)
		p.connections.Remove(elem)
		p.totalCount--
		p.closeConnection(conn)
	}

	if p.totalCount < p.config.MinConnections {
		needed := p.config.MinConnections - p.totalCount
		for i := 0; i < needed; i++ {
			conn, err := p.createConnection()
			if err != nil {
				if p.log != nil {
					(p.log).Warn(fmt.Sprintf("failed to create connection during cleanup: %v", err))
				}
				continue
			}
			p.connections.PushBack(conn)
			p.totalCount++
		}
	}
}

func (p *SMTPConnectionPool) healthCheckLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.HealthCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performHealthCheck()
		case <-p.stopHealth:
			return
		}
	}
}

func (p *SMTPConnectionPool) performHealthCheck() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for elem := p.connections.Front(); elem != nil; elem = elem.Next() {
		conn := elem.Value.(*PooledConnection)

		if !p.isConnectionHealthy(conn) {
			p.statsMu.Lock()
			p.stats.TotalFailedHealth++
			p.statsMu.Unlock()
		}
	}
}

func (p *SMTPConnectionPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	close(p.stopCleanup)
	close(p.stopHealth)

	close(p.waitQueue)
	for waitChan := range p.waitQueue {
		waitChan <- nil
		close(waitChan)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for elem := p.connections.Front(); elem != nil; elem = elem.Next() {
		conn := elem.Value.(*PooledConnection)
		p.closeConnection(conn)
	}
	p.connections.Init()
	p.totalCount = 0

	p.wg.Wait()

	return nil
}

func (p *SMTPConnectionPool) GetStats() PoolStats {
	p.statsMu.RLock()
	defer p.statsMu.RUnlock()

	p.mu.Lock()
	stats := *p.stats
	stats.CurrentActive = int64(p.activeCount)
	stats.CurrentIdle = int64(p.connections.Len())
	p.mu.Unlock()

	return stats
}

func (p *SMTPConnectionPool) recordAcquire(waitTime time.Duration) {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()

	p.stats.TotalAcquired++
	p.stats.totalWaitTime += waitTime
	p.stats.waitCount++

	if p.stats.waitCount > 0 {
		p.stats.AverageWaitTime = p.stats.totalWaitTime / time.Duration(p.stats.waitCount)
	}
}

func (p *SMTPConnectionPool) recordRelease() {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()

	p.stats.TotalReleased++
}

func (pc *PooledConnection) GetSMTPClient() *smtp.Client {
	return pc.conn
}

func (pc *PooledConnection) GetUsageCount() int64 {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.usageCount
}

func (pc *PooledConnection) GetAge() time.Duration {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return time.Since(pc.createdAt)
}

func (pc *PooledConnection) GetIdleTime() time.Duration {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return time.Since(pc.lastUsedAt)
}
