package repository

import (
        "context"
        "database/sql"
        "encoding/json"
        "fmt"
        "net/url"
        "strings"
        "time"

        "github.com/lib/pq"
)

type ProxyRepository struct {
        db *sql.DB
}
// ============================================================================
// COUNT METHODS FOR METRICS
// ============================================================================

// Count returns the total number of proxies matching the filter
func (r *ProxyRepository) Count(ctx context.Context, filter *ProxyFilter) (int, error) {
    whereClauses := []string{"1=1"}
    args := []interface{}{}
    argPos := 1

    if filter != nil {
        if len(filter.IDs) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.IDs))
            argPos++
        }

        if len(filter.Types) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("type = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.Types))
            argPos++
        }

        if len(filter.Regions) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("region = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.Regions))
            argPos++
        }

        if len(filter.Providers) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("provider = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.Providers))
            argPos++
        }

        if len(filter.Status) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("status = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.Status))
            argPos++
        }

        if len(filter.Tags) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("tags && $%d", argPos))
            args = append(args, pq.Array(filter.Tags))
            argPos++
        }

        if filter.RotationGroup != "" {
            whereClauses = append(whereClauses, fmt.Sprintf("rotation_group = $%d", argPos))
            args = append(args, filter.RotationGroup)
            argPos++
        }

        if filter.IsActive != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("is_active = $%d", argPos))
            args = append(args, *filter.IsActive)
            argPos++
        }

        if filter.MinHealthScore != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("health_score >= $%d", argPos))
            args = append(args, *filter.MinHealthScore)
            argPos++
        }

        if filter.MaxHealthScore != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("health_score <= $%d", argPos))
            args = append(args, *filter.MaxHealthScore)
            argPos++
        }

        if filter.MaxLatencyMs != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("latency_ms <= $%d", argPos))
            args = append(args, *filter.MaxLatencyMs)
            argPos++
        }

        if filter.AvailableOnly {
            whereClauses = append(whereClauses, "is_active = true")
            whereClauses = append(whereClauses, "status = 'healthy'")
            whereClauses = append(whereClauses, "(current_conns < max_connections OR max_connections = 0)")
            whereClauses = append(whereClauses, "(bandwidth_limit_mb = 0 OR bandwidth_mb < bandwidth_limit_mb)")
        }

        if filter.Search != "" {
            whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR host ILIKE $%d OR provider ILIKE $%d)", argPos, argPos, argPos))
            args = append(args, "%"+filter.Search+"%")
            argPos++
        }
    }

    whereClause := strings.Join(whereClauses, " AND ")
    query := fmt.Sprintf("SELECT COUNT(*) FROM proxies WHERE %s", whereClause)

    var count int
    err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count proxies: %w", err)
    }

    return count, nil
}

// CountActive returns the count of active proxies
func (r *ProxyRepository) CountActive(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE is_active = true`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count active proxies: %w", err)
    }
    
    return count, nil
}

// CountHealthy returns the count of healthy proxies
func (r *ProxyRepository) CountHealthy(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE status = 'healthy' AND is_active = true`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count healthy proxies: %w", err)
    }
    
    return count, nil
}

// CountUnhealthy returns the count of unhealthy proxies
func (r *ProxyRepository) CountUnhealthy(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE status != 'healthy' OR is_active = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count unhealthy proxies: %w", err)
    }
    
    return count, nil
}

// CountInUse returns the count of proxies currently in use
func (r *ProxyRepository) CountInUse(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE in_use = true`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count proxies in use: %w", err)
    }
    
    return count, nil
}

// CountOverloaded returns the count of overloaded proxies (at connection or bandwidth limit)
func (r *ProxyRepository) CountOverloaded(ctx context.Context) (int, error) {
    query := `
        SELECT COUNT(*) FROM proxies 
        WHERE (current_conns >= max_connections AND max_connections > 0)
           OR (bandwidth_limit_mb > 0 AND bandwidth_mb >= bandwidth_limit_mb)`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count overloaded proxies: %w", err)
    }
    
    return count, nil
}

// CountAvailable returns the count of proxies available for use
func (r *ProxyRepository) CountAvailable(ctx context.Context) (int, error) {
    query := `
        SELECT COUNT(*) FROM proxies 
        WHERE is_active = true 
          AND status = 'healthy' 
          AND (current_conns < max_connections OR max_connections = 0)
          AND (bandwidth_limit_mb = 0 OR bandwidth_mb < bandwidth_limit_mb)`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count available proxies: %w", err)
    }
    
    return count, nil
}

// CountByStatus returns the count of proxies with a specific status
func (r *ProxyRepository) CountByStatus(ctx context.Context, status string) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE status = $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, status).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count proxies by status: %w", err)
    }
    
    return count, nil
}

// CountByType returns the count of proxies of a specific type
func (r *ProxyRepository) CountByType(ctx context.Context, proxyType string) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE type = $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, proxyType).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count proxies by type: %w", err)
    }
    
    return count, nil
}

// CountByRegion returns the count of proxies in a specific region
func (r *ProxyRepository) CountByRegion(ctx context.Context, region string) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE region = $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, region).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count proxies by region: %w", err)
    }
    
    return count, nil
}

// CountByProvider returns the count of proxies from a specific provider
func (r *ProxyRepository) CountByProvider(ctx context.Context, provider string) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE provider = $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, provider).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count proxies by provider: %w", err)
    }
    
    return count, nil
}

// CountByRotationGroup returns the count of proxies in a rotation group
func (r *ProxyRepository) CountByRotationGroup(ctx context.Context, group string) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE rotation_group = $1 AND is_active = true`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, group).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count proxies by rotation group: %w", err)
    }
    
    return count, nil
}

// CountWithHighLatency returns proxies with latency above threshold
func (r *ProxyRepository) CountWithHighLatency(ctx context.Context, thresholdMs int) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE latency_ms > $1 AND is_active = true`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, thresholdMs).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count proxies with high latency: %w", err)
    }
    
    return count, nil
}

// CountWithConsecutiveFails returns proxies with consecutive failures above threshold
func (r *ProxyRepository) CountWithConsecutiveFails(ctx context.Context, threshold int) (int, error) {
    query := `SELECT COUNT(*) FROM proxies WHERE consecutive_fails >= $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, threshold).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count proxies with consecutive fails: %w", err)
    }
    
    return count, nil
}

// ============================================================================
// ADDITIONAL METRICS HELPERS
// ============================================================================

// GetAverageHealthScore returns the average health score across all active proxies
func (r *ProxyRepository) GetAverageHealthScore(ctx context.Context) (float64, error) {
    query := `SELECT COALESCE(AVG(health_score), 0) FROM proxies WHERE is_active = true`
    
    var avgScore float64
    err := r.db.QueryRowContext(ctx, query).Scan(&avgScore)
    if err != nil {
        return 0, fmt.Errorf("failed to get average health score: %w", err)
    }
    
    return avgScore, nil
}

// GetAverageLatency returns the average latency across all healthy proxies
func (r *ProxyRepository) GetAverageLatency(ctx context.Context) (float64, error) {
    query := `SELECT COALESCE(AVG(latency_ms), 0) FROM proxies WHERE status = 'healthy' AND is_active = true`
    
    var avgLatency float64
    err := r.db.QueryRowContext(ctx, query).Scan(&avgLatency)
    if err != nil {
        return 0, fmt.Errorf("failed to get average latency: %w", err)
    }
    
    return avgLatency, nil
}

// GetTotalBandwidth returns the total bandwidth used across all proxies
func (r *ProxyRepository) GetTotalBandwidth(ctx context.Context) (float64, error) {
    query := `SELECT COALESCE(SUM(bandwidth_mb), 0) FROM proxies`
    
    var totalBandwidth float64
    err := r.db.QueryRowContext(ctx, query).Scan(&totalBandwidth)
    if err != nil {
        return 0, fmt.Errorf("failed to get total bandwidth: %w", err)
    }
    
    return totalBandwidth, nil
}

// GetTotalConnections returns the total number of current connections
func (r *ProxyRepository) GetTotalConnections(ctx context.Context) (int64, error) {
    query := `SELECT COALESCE(SUM(current_conns), 0) FROM proxies WHERE is_active = true`
    
    var totalConns int64
    err := r.db.QueryRowContext(ctx, query).Scan(&totalConns)
    if err != nil {
        return 0, fmt.Errorf("failed to get total connections: %w", err)
    }
    
    return totalConns, nil
}

// GetTotalAssignedAccounts returns the total number of accounts assigned to proxies
func (r *ProxyRepository) GetTotalAssignedAccounts(ctx context.Context) (int64, error) {
    query := `SELECT COALESCE(SUM(assigned_accounts), 0) FROM proxies WHERE is_active = true`
    
    var totalAccounts int64
    err := r.db.QueryRowContext(ctx, query).Scan(&totalAccounts)
    if err != nil {
        return 0, fmt.Errorf("failed to get total assigned accounts: %w", err)
    }
    
    return totalAccounts, nil
}

// GetTypeBreakdown returns count of proxies grouped by type
func (r *ProxyRepository) GetTypeBreakdown(ctx context.Context) (map[string]int, error) {
    query := `
        SELECT type, COUNT(*) as count
        FROM proxies
        WHERE is_active = true
        GROUP BY type`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get type breakdown: %w", err)
    }
    defer rows.Close()

    breakdown := make(map[string]int)
    for rows.Next() {
        var proxyType string
        var count int
        if err := rows.Scan(&proxyType, &count); err != nil {
            return nil, err
        }
        breakdown[proxyType] = count
    }

    return breakdown, nil
}

// GetRegionBreakdown returns count of proxies grouped by region
func (r *ProxyRepository) GetRegionBreakdown(ctx context.Context) (map[string]int, error) {
    query := `
        SELECT region, COUNT(*) as count
        FROM proxies
        WHERE is_active = true
        GROUP BY region`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get region breakdown: %w", err)
    }
    defer rows.Close()

    breakdown := make(map[string]int)
    for rows.Next() {
        var region string
        var count int
        if err := rows.Scan(&region, &count); err != nil {
            return nil, err
        }
        breakdown[region] = count
    }

    return breakdown, nil
}

// GetProviderBreakdown returns count of proxies grouped by provider
func (r *ProxyRepository) GetProviderBreakdown(ctx context.Context) (map[string]int, error) {
    query := `
        SELECT provider, COUNT(*) as count
        FROM proxies
        WHERE is_active = true
        GROUP BY provider`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get provider breakdown: %w", err)
    }
    defer rows.Close()

    breakdown := make(map[string]int)
    for rows.Next() {
        var provider string
        var count int
        if err := rows.Scan(&provider, &count); err != nil {
            return nil, err
        }
        breakdown[provider] = count
    }

    return breakdown, nil
}

// GetTopPerformingProxies returns proxies with best health scores and lowest latency
func (r *ProxyRepository) GetTopPerformingProxies(ctx context.Context, limit int) ([]*Proxy, error) {
    if limit <= 0 {
        limit = 10
    }

    query := `
        SELECT id, name, host, port, type, region, provider, status, health_score, 
               latency_ms, success_count, failure_count, consecutive_fails, 
               in_use, assigned_accounts, created_at, updated_at
        FROM proxies
        WHERE is_active = true AND status = 'healthy'
        ORDER BY health_score DESC, latency_ms ASC, success_count DESC
        LIMIT $1`

    rows, err := r.db.QueryContext(ctx, query, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get top performing proxies: %w", err)
    }
    defer rows.Close()

    proxies := []*Proxy{}
    for rows.Next() {
        p := &Proxy{}
        err := rows.Scan(
            &p.ID, &p.Name, &p.Host, &p.Port, &p.Type, &p.Region, &p.Provider,
            &p.Status, &p.HealthScore, &p.LatencyMs, &p.SuccessCount,
            &p.FailureCount, &p.ConsecutiveFails, &p.InUse, &p.AssignedAccounts,
            &p.CreatedAt, &p.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        proxies = append(proxies, p)
    }

    return proxies, nil
}

// GetWorstPerformingProxies returns proxies with worst health scores or high latency
func (r *ProxyRepository) GetWorstPerformingProxies(ctx context.Context, limit int) ([]*Proxy, error) {
    if limit <= 0 {
        limit = 10
    }

    query := `
        SELECT id, name, host, port, type, region, provider, status, health_score, 
               latency_ms, success_count, failure_count, consecutive_fails, 
               last_error, last_error_at, created_at, updated_at
        FROM proxies
        WHERE is_active = true
        ORDER BY health_score ASC, consecutive_fails DESC, latency_ms DESC
        LIMIT $1`

    rows, err := r.db.QueryContext(ctx, query, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get worst performing proxies: %w", err)
    }
    defer rows.Close()

    proxies := []*Proxy{}
    for rows.Next() {
        p := &Proxy{}
        err := rows.Scan(
            &p.ID, &p.Name, &p.Host, &p.Port, &p.Type, &p.Region, &p.Provider,
            &p.Status, &p.HealthScore, &p.LatencyMs, &p.SuccessCount,
            &p.FailureCount, &p.ConsecutiveFails, &p.LastError, &p.LastErrorAt,
            &p.CreatedAt, &p.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        proxies = append(proxies, p)
    }

    return proxies, nil
}

// GetProxiesNeedingHealthCheck returns proxies that haven't been checked recently
func (r *ProxyRepository) GetProxiesNeedingHealthCheck(ctx context.Context, olderThan time.Duration, limit int) ([]*Proxy, error) {
    if limit <= 0 {
        limit = 100
    }
    
    checkTime := time.Now().Add(-olderThan)

    query := `
        SELECT id, name, host, port, type, status, last_checked_at, 
               consecutive_fails, health_score, created_at, updated_at
        FROM proxies
        WHERE is_active = true 
          AND (last_checked_at IS NULL OR last_checked_at < $1)
        ORDER BY last_checked_at ASC NULLS FIRST, consecutive_fails DESC
        LIMIT $2`

    rows, err := r.db.QueryContext(ctx, query, checkTime, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get proxies needing health check: %w", err)
    }
    defer rows.Close()

    proxies := []*Proxy{}
    for rows.Next() {
        p := &Proxy{}
        err := rows.Scan(
            &p.ID, &p.Name, &p.Host, &p.Port, &p.Type, &p.Status,
            &p.LastCheckedAt, &p.ConsecutiveFails, &p.HealthScore,
            &p.CreatedAt, &p.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        proxies = append(proxies, p)
    }

    return proxies, nil
}

// GetProxiesAtCapacity returns proxies at or near capacity limits
func (r *ProxyRepository) GetProxiesAtCapacity(ctx context.Context, thresholdPercent float64) ([]*Proxy, error) {
    query := `
        SELECT id, name, host, port, type, region, status, 
               current_conns, max_connections, assigned_accounts, max_accounts,
               bandwidth_mb, bandwidth_limit_mb, health_score, created_at, updated_at
        FROM proxies
        WHERE is_active = true
          AND (
              (max_connections > 0 AND current_conns::float / max_connections >= $1 / 100.0)
              OR (max_accounts > 0 AND assigned_accounts::float / max_accounts >= $1 / 100.0)
              OR (bandwidth_limit_mb > 0 AND bandwidth_mb / bandwidth_limit_mb >= $1 / 100.0)
          )
        ORDER BY 
            GREATEST(
                CASE WHEN max_connections > 0 THEN current_conns::float / max_connections ELSE 0 END,
                CASE WHEN max_accounts > 0 THEN assigned_accounts::float / max_accounts ELSE 0 END,
                CASE WHEN bandwidth_limit_mb > 0 THEN bandwidth_mb / bandwidth_limit_mb ELSE 0 END
            ) DESC`

    rows, err := r.db.QueryContext(ctx, query, thresholdPercent)
    if err != nil {
        return nil, fmt.Errorf("failed to get proxies at capacity: %w", err)
    }
    defer rows.Close()

    proxies := []*Proxy{}
    for rows.Next() {
        p := &Proxy{}
        err := rows.Scan(
            &p.ID, &p.Name, &p.Host, &p.Port, &p.Type, &p.Region, &p.Status,
            &p.CurrentConns, &p.MaxConnections, &p.AssignedAccounts, &p.MaxAccounts,
            &p.BandwidthMB, &p.BandwidthLimitMB, &p.HealthScore,
            &p.CreatedAt, &p.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        proxies = append(proxies, p)
    }

    return proxies, nil
}

// GetRotationGroupSummary returns summary stats for all rotation groups
func (r *ProxyRepository) GetRotationGroupSummary(ctx context.Context) (map[string]int, error) {
    query := `
        SELECT rotation_group, COUNT(*) as count
        FROM proxies
        WHERE is_active = true 
          AND status = 'healthy'
          AND rotation_group IS NOT NULL 
          AND rotation_group <> ''
        GROUP BY rotation_group`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get rotation group summary: %w", err)
    }
    defer rows.Close()

    summary := make(map[string]int)
    for rows.Next() {
        var group string
        var count int
        if err := rows.Scan(&group, &count); err != nil {
            return nil, err
        }
        summary[group] = count
    }

    return summary, nil
}

type Proxy struct {
        ID               string                 `json:"id" db:"id"`
        Name             string                 `json:"name" db:"name"`
        Host             string                 `json:"host" db:"host"`
        Port             int                    `json:"port" db:"port"`
        Username         string                 `json:"username" db:"username"`
        Password         string                 `json:"password" db:"password"`
        Type             string                 `json:"type" db:"type"`
        Region           string                 `json:"region" db:"region"`
        Provider         string                 `json:"provider" db:"provider"`
        Status           string                 `json:"status" db:"status"`
        // HealthStatus tracks the current health state (healthy/unhealthy/degraded/unknown)
        // persisted so it survives across app restarts.
        HealthStatus     string                 `json:"health_status" db:"health_status"`
        IsActive         bool                   `json:"is_active" db:"is_active"`
        Tags             []string               `json:"tags" db:"tags"`
        Metadata         map[string]interface{} `json:"metadata" db:"metadata"`
        LastCheckedAt    *time.Time             `json:"last_checked_at" db:"last_checked_at"`
        LastHealthyAt    *time.Time             `json:"last_healthy_at" db:"last_healthy_at"`
        LastErrorAt      *time.Time             `json:"last_error_at" db:"last_error_at"`
        LastError        string                 `json:"last_error" db:"last_error"`
        LatencyMs        int                    `json:"latency_ms" db:"latency_ms"`
        SuccessCount     int64                  `json:"success_count" db:"success_count"`
        FailureCount     int64                  `json:"failure_count" db:"failure_count"`
        ConsecutiveFails int                    `json:"consecutive_fails" db:"consecutive_fails"`
        MaxConsecutive   int                    `json:"max_consecutive" db:"max_consecutive"`
        HealthScore      float64                `json:"health_score" db:"health_score"`
        RotationWeight   int                    `json:"rotation_weight" db:"rotation_weight"`
        RotationGroup    string                 `json:"rotation_group" db:"rotation_group"`
        InUse            bool                   `json:"in_use" db:"in_use"`
        AssignedAccounts int                    `json:"assigned_accounts" db:"assigned_accounts"`
        MaxAccounts      int                    `json:"max_accounts" db:"max_accounts"`
        MaxConnections   int                    `json:"max_connections" db:"max_connections"`
        CurrentConns     int                    `json:"current_conns" db:"current_conns"`
        BandwidthMB      float64                `json:"bandwidth_mb" db:"bandwidth_mb"`
        BandwidthLimitMB float64                `json:"bandwidth_limit_mb" db:"bandwidth_limit_mb"`
        ResetAt          *time.Time             `json:"reset_at" db:"reset_at"`
        CreatedAt        time.Time              `json:"created_at" db:"created_at"`
        UpdatedAt        time.Time              `json:"updated_at" db:"updated_at"`
        CreatedBy        string                 `json:"created_by" db:"created_by"`
        UpdatedBy        string                 `json:"updated_by" db:"updated_by"`
}

type ProxyFilter struct {
        IDs             []string
        Types           []string
        Regions         []string
        Providers       []string
        Status          []string
        Tags            []string
        RotationGroup   string
        IsActive        *bool
        MinHealthScore  *float64
        MaxHealthScore  *float64
        MaxLatencyMs    *int
        AvailableOnly   bool
        Search          string
        SortBy          string
        SortOrder       string
        Limit           int
        Offset          int
}

type ProxyStats struct {
        TotalProxies       int             `json:"total_proxies"`
        ActiveProxies      int             `json:"active_proxies"`
        HealthyProxies     int             `json:"healthy_proxies"`
        UnhealthyProxies   int             `json:"unhealthy_proxies"`
        InUseProxies       int             `json:"in_use_proxies"`
        OverloadedProxies  int             `json:"overloaded_proxies"`
        AverageHealthScore float64         `json:"average_health_score"`
        AverageLatencyMs   float64         `json:"average_latency_ms"`
        TotalBandwidthMB   float64         `json:"total_bandwidth_mb"`
        TypeBreakdown      map[string]int  `json:"type_breakdown"`
        RegionBreakdown    map[string]int  `json:"region_breakdown"`
        ProviderBreakdown  map[string]int  `json:"provider_breakdown"`
}

func NewProxyRepository(db *sql.DB) *ProxyRepository {
        return &ProxyRepository{db: db}
}

func buildProxyURL(p *Proxy) string {
        scheme := p.Type
        if scheme == "" {
                scheme = "http"
        }
        if p.Username != "" && p.Password != "" {
                return fmt.Sprintf("%s://%s:%s@%s:%d",
                        scheme,
                        url.QueryEscape(p.Username),
                        url.QueryEscape(p.Password),
                        p.Host,
                        p.Port,
                )
        }
        return fmt.Sprintf("%s://%s:%d", scheme, p.Host, p.Port)
}

func (r *ProxyRepository) Create(ctx context.Context, p *Proxy) error {
        query := `
                INSERT INTO proxies (
                        id, name, host, port, username, password, type, region, provider,
                        status, is_active, tags, metadata, latency_ms, success_count,
                        failure_count, consecutive_fails, max_consecutive, health_score,
                        rotation_weight, rotation_group, in_use, assigned_accounts,
                        max_accounts, max_connections, current_conns, bandwidth_mb,
                        bandwidth_limit_mb, reset_at, created_by, created_at, updated_at,
                        proxy_url
                ) VALUES (
                        $1,$2,$3,$4,$5,$6,$7,$8,$9,
                        $10,$11,$12,$13,$14,$15,
                        $16,$17,$18,$19,
                        $20,$21,$22,$23,
                        $24,$25,$26,$27,
                        $28,$29,$30,$31,$32,
                        $33
                ) RETURNING id, created_at, updated_at`

        metaJSON, err := json.Marshal(p.Metadata)
        if err != nil {
                return fmt.Errorf("failed to marshal proxy metadata: %w", err)
        }

        proxyURL := buildProxyURL(p)

        err = r.db.QueryRowContext(
                ctx, query,
                p.ID, p.Name, p.Host, p.Port, p.Username, p.Password, p.Type, p.Region, p.Provider,
                p.Status, p.IsActive, pq.Array(p.Tags), metaJSON, p.LatencyMs, p.SuccessCount,
                p.FailureCount, p.ConsecutiveFails, p.MaxConsecutive, p.HealthScore,
                p.RotationWeight, p.RotationGroup, p.InUse, p.AssignedAccounts,
                p.MaxAccounts, p.MaxConnections, p.CurrentConns, p.BandwidthMB,
                p.BandwidthLimitMB, p.ResetAt, p.CreatedBy, time.Now(), time.Now(),
                proxyURL,
        ).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

        return err
}

func (r *ProxyRepository) GetByID(ctx context.Context, id string) (*Proxy, error) {
        query := `
                SELECT id, COALESCE(name,''), host, port, COALESCE(username,''), COALESCE(password,''),
                        type, COALESCE(region,''), COALESCE(provider,''),
                        status, is_active, tags, metadata, last_checked_at, last_healthy_at,
                        last_error_at, COALESCE(last_error,''),
                        CAST(COALESCE(latency_ms,0) AS INTEGER), success_count, failure_count,
                        consecutive_fails, max_consecutive,
                        CAST(COALESCE(health_score,0) AS DOUBLE PRECISION), rotation_weight,
                        COALESCE(rotation_group,''), in_use, assigned_accounts, max_accounts, max_connections,
                        current_conns, bandwidth_mb, bandwidth_limit_mb, reset_at,
                        created_at, updated_at, COALESCE(created_by,''), COALESCE(updated_by,'')
                FROM proxies
                WHERE id = $1`

        p := &Proxy{}
        var metaJSON []byte

        err := r.db.QueryRowContext(ctx, query, id).Scan(
                &p.ID, &p.Name, &p.Host, &p.Port, &p.Username, &p.Password, &p.Type, &p.Region,
                &p.Provider, &p.Status, &p.IsActive, pq.Array(&p.Tags), &metaJSON,
                &p.LastCheckedAt, &p.LastHealthyAt, &p.LastErrorAt, &p.LastError,
                &p.LatencyMs, &p.SuccessCount, &p.FailureCount, &p.ConsecutiveFails,
                &p.MaxConsecutive, &p.HealthScore, &p.RotationWeight, &p.RotationGroup,
                &p.InUse, &p.AssignedAccounts, &p.MaxAccounts, &p.MaxConnections,
                &p.CurrentConns, &p.BandwidthMB, &p.BandwidthLimitMB, &p.ResetAt,
                &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
        )
        if err != nil {
                if err == sql.ErrNoRows {
                        return nil, fmt.Errorf("proxy not found")
                }
                return nil, err
        }

        if len(metaJSON) > 0 {
                json.Unmarshal(metaJSON, &p.Metadata)
        }

        return p, nil
}

func (r *ProxyRepository) Update(ctx context.Context, p *Proxy) error {
        query := `
                UPDATE proxies SET
                        name = $2, host = $3, port = $4, username = $5, password = $6,
                        type = $7, region = $8, provider = $9, status = $10, is_active = $11,
                        tags = $12, metadata = $13, last_checked_at = $14, last_healthy_at = $15,
                        last_error_at = $16, last_error = $17, latency_ms = $18,
                        success_count = $19, failure_count = $20, consecutive_fails = $21,
                        max_consecutive = $22, health_score = $23, rotation_weight = $24,
                        rotation_group = $25, in_use = $26, assigned_accounts = $27,
                        max_accounts = $28, max_connections = $29, current_conns = $30,
                        bandwidth_mb = $31, bandwidth_limit_mb = $32, reset_at = $33,
                        updated_by = $34, updated_at = $35
                WHERE id = $1`

        metaJSON, err := json.Marshal(p.Metadata)
        if err != nil {
                return fmt.Errorf("failed to marshal proxy metadata: %w", err)
        }

        result, err := r.db.ExecContext(
                ctx, query,
                p.ID, p.Name, p.Host, p.Port, p.Username, p.Password, p.Type, p.Region, p.Provider,
                p.Status, p.IsActive, pq.Array(p.Tags), metaJSON, p.LastCheckedAt, p.LastHealthyAt,
                p.LastErrorAt, p.LastError, p.LatencyMs, p.SuccessCount, p.FailureCount,
                p.ConsecutiveFails, p.MaxConsecutive, p.HealthScore, p.RotationWeight,
                p.RotationGroup, p.InUse, p.AssignedAccounts, p.MaxAccounts, p.MaxConnections,
                p.CurrentConns, p.BandwidthMB, p.BandwidthLimitMB, p.ResetAt, p.UpdatedBy, time.Now(),
        )
        if err != nil {
                return err
        }
        rows, err := result.RowsAffected()
        if err != nil {
                return err
        }
        if rows == 0 {
                return fmt.Errorf("proxy not found")
        }
        return nil
}

func (r *ProxyRepository) Delete(ctx context.Context, id string) error {
        query := `DELETE FROM proxies WHERE id = $1`
        result, err := r.db.ExecContext(ctx, query, id)
        if err != nil {
                return err
        }
        rows, err := result.RowsAffected()
        if err != nil {
                return err
        }
        if rows == 0 {
                return fmt.Errorf("proxy not found")
        }
        return nil
}

func (r *ProxyRepository) List(ctx context.Context, filter *ProxyFilter) ([]*Proxy, int, error) {
        whereClauses := []string{"1=1"}
        args := []interface{}{}
        argPos := 1

        if len(filter.IDs) > 0 {
                whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
                args = append(args, pq.Array(filter.IDs))
                argPos++
        }

        if len(filter.Types) > 0 {
                whereClauses = append(whereClauses, fmt.Sprintf("type = ANY($%d)", argPos))
                args = append(args, pq.Array(filter.Types))
                argPos++
        }

        if len(filter.Regions) > 0 {
                whereClauses = append(whereClauses, fmt.Sprintf("region = ANY($%d)", argPos))
                args = append(args, pq.Array(filter.Regions))
                argPos++
        }

        if len(filter.Providers) > 0 {
                whereClauses = append(whereClauses, fmt.Sprintf("provider = ANY($%d)", argPos))
                args = append(args, pq.Array(filter.Providers))
                argPos++
        }

        if len(filter.Status) > 0 {
                whereClauses = append(whereClauses, fmt.Sprintf("status = ANY($%d)", argPos))
                args = append(args, pq.Array(filter.Status))
                argPos++
        }

        if len(filter.Tags) > 0 {
                whereClauses = append(whereClauses, fmt.Sprintf("tags && $%d", argPos))
                args = append(args, pq.Array(filter.Tags))
                argPos++
        }

        if filter.RotationGroup != "" {
                whereClauses = append(whereClauses, fmt.Sprintf("rotation_group = $%d", argPos))
                args = append(args, filter.RotationGroup)
                argPos++
        }

        if filter.IsActive != nil {
                whereClauses = append(whereClauses, fmt.Sprintf("is_active = $%d", argPos))
                args = append(args, *filter.IsActive)
                argPos++
        }

        if filter.MinHealthScore != nil {
                whereClauses = append(whereClauses, fmt.Sprintf("health_score >= $%d", argPos))
                args = append(args, *filter.MinHealthScore)
                argPos++
        }

        if filter.MaxHealthScore != nil {
                whereClauses = append(whereClauses, fmt.Sprintf("health_score <= $%d", argPos))
                args = append(args, *filter.MaxHealthScore)
                argPos++
        }

        if filter.MaxLatencyMs != nil {
                whereClauses = append(whereClauses, fmt.Sprintf("latency_ms <= $%d", argPos))
                args = append(args, *filter.MaxLatencyMs)
                argPos++
        }

        if filter.AvailableOnly {
                whereClauses = append(whereClauses, "is_active = true")
                whereClauses = append(whereClauses, "status = 'healthy'")
                whereClauses = append(whereClauses, "(current_conns < max_connections OR max_connections = 0)")
                whereClauses = append(whereClauses, "(bandwidth_limit_mb = 0 OR bandwidth_mb < bandwidth_limit_mb)")
        }

        if filter.Search != "" {
                whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR host ILIKE $%d OR provider ILIKE $%d)", argPos, argPos, argPos))
                args = append(args, "%"+filter.Search+"%")
                argPos++
        }

        whereClause := strings.Join(whereClauses, " AND ")

        countQuery := fmt.Sprintf("SELECT COUNT(*) FROM proxies WHERE %s", whereClause)
        var total int
        if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
                return nil, 0, err
        }

        allowedProxyCols := []string{"id", "name", "host", "port", "type", "region", "provider", "status", "health_score", "latency_ms", "success_count", "failure_count", "created_at", "updated_at", "last_checked_at", "is_active"}
        sortBy := sanitizeSortColumn(filter.SortBy, "created_at", allowedProxyCols)
        sortOrder := sanitizeSortOrder(filter.SortOrder)

        limit := 50
        if filter.Limit > 0 {
                limit = filter.Limit
        }
        offset := 0
        if filter.Offset > 0 {
                offset = filter.Offset
        }

        query := fmt.Sprintf(`
                SELECT id, COALESCE(name,''), host, port, COALESCE(username,''), COALESCE(password,''),
                        type, COALESCE(region,''), COALESCE(provider,''),
                        status, is_active, tags, metadata, last_checked_at, last_healthy_at,
                        last_error_at, COALESCE(last_error,''),
                        CAST(COALESCE(latency_ms,0) AS INTEGER), success_count, failure_count,
                        consecutive_fails, max_consecutive,
                        CAST(COALESCE(health_score,0) AS DOUBLE PRECISION), rotation_weight,
                        COALESCE(rotation_group,''), in_use, assigned_accounts, max_accounts, max_connections,
                        current_conns, bandwidth_mb, bandwidth_limit_mb, reset_at,
                        created_at, updated_at, COALESCE(created_by,''), COALESCE(updated_by,'')
                FROM proxies
                WHERE %s
                ORDER BY %s %s
                LIMIT $%d OFFSET $%d`,
                whereClause, sortBy, sortOrder, argPos, argPos+1)

        args = append(args, limit, offset)

        rows, err := r.db.QueryContext(ctx, query, args...)
        if err != nil {
                return nil, 0, err
        }
        defer rows.Close()

        proxies := []*Proxy{}
        for rows.Next() {
                p := &Proxy{}
                var metaJSON []byte

                err := rows.Scan(
                        &p.ID, &p.Name, &p.Host, &p.Port, &p.Username, &p.Password, &p.Type, &p.Region,
                        &p.Provider, &p.Status, &p.IsActive, pq.Array(&p.Tags), &metaJSON,
                        &p.LastCheckedAt, &p.LastHealthyAt, &p.LastErrorAt, &p.LastError,
                        &p.LatencyMs, &p.SuccessCount, &p.FailureCount, &p.ConsecutiveFails,
                        &p.MaxConsecutive, &p.HealthScore, &p.RotationWeight, &p.RotationGroup,
                        &p.InUse, &p.AssignedAccounts, &p.MaxAccounts, &p.MaxConnections,
                        &p.CurrentConns, &p.BandwidthMB, &p.BandwidthLimitMB, &p.ResetAt,
                        &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
                )
                if err != nil {
                        return nil, 0, err
                }

                if len(metaJSON) > 0 {
                        json.Unmarshal(metaJSON, &p.Metadata)
                }

                proxies = append(proxies, p)
        }

        return proxies, total, nil
}

func (r *ProxyRepository) UpdateHealth(ctx context.Context, id string, healthy bool, latencyMs int, healthScore float64, errMsg string) error {
        now := time.Now()
        query := `
                UPDATE proxies SET
                        last_checked_at = $2,
                        last_healthy_at = CASE WHEN $3 THEN $2 ELSE last_healthy_at END,
                        last_error_at = CASE WHEN $3 THEN last_error_at ELSE $2 END,
                        last_error = CASE WHEN $3 THEN '' ELSE $5 END,
                        latency_ms = $4,
                        health_score = $6,
                        status = CASE WHEN $3 THEN 'healthy' ELSE 'unhealthy' END,
                        success_count = success_count + CASE WHEN $3 THEN 1 ELSE 0 END,
                        failure_count = failure_count + CASE WHEN $3 THEN 0 ELSE 1 END,
                        consecutive_fails = CASE WHEN $3 THEN 0 ELSE consecutive_fails + 1 END,
                        updated_at = $2
                WHERE id = $1`

        _, err := r.db.ExecContext(ctx, query, id, now, healthy, latencyMs, errMsg, healthScore)
        return err
}

func (r *ProxyRepository) IncrementConnections(ctx context.Context, id string) error {
        query := `
                UPDATE proxies SET
                        current_conns = current_conns + 1,
                        in_use = true,
                        updated_at = $2
                WHERE id = $1`
        _, err := r.db.ExecContext(ctx, query, id, time.Now())
        return err
}

func (r *ProxyRepository) DecrementConnections(ctx context.Context, id string) error {
        query := `
                UPDATE proxies SET
                        current_conns = GREATEST(0, current_conns - 1),
                        in_use = GREATEST(0, current_conns - 1) > 0,
                        updated_at = $2
                WHERE id = $1`
        _, err := r.db.ExecContext(ctx, query, id, time.Now())
        return err
}

func (r *ProxyRepository) IncrementBandwidth(ctx context.Context, id string, mb float64) error {
        query := `
                UPDATE proxies SET
                        bandwidth_mb = bandwidth_mb + $2,
                        updated_at = $3
                WHERE id = $1`
        _, err := r.db.ExecContext(ctx, query, id, mb, time.Now())
        return err
}

func (r *ProxyRepository) ResetBandwidthAndConns(ctx context.Context) error {
        query := `
                UPDATE proxies SET
                        bandwidth_mb = 0,
                        current_conns = 0,
                        in_use = false,
                        reset_at = $1,
                        updated_at = $1
                WHERE reset_at IS NULL OR reset_at < CURRENT_DATE`
        _, err := r.db.ExecContext(ctx, query, time.Now())
        return err
}

func (r *ProxyRepository) AssignAccount(ctx context.Context, id string) error {
        query := `
                UPDATE proxies SET
                        assigned_accounts = assigned_accounts + 1,
                        updated_at = $2
                WHERE id = $1
                        AND (max_accounts = 0 OR assigned_accounts < max_accounts)`
        result, err := r.db.ExecContext(ctx, query, id, time.Now())
        if err != nil {
                return err
        }
        rows, err := result.RowsAffected()
        if err != nil {
                return err
        }
        if rows == 0 {
                return fmt.Errorf("proxy not available for assignment")
        }
        return nil
}

func (r *ProxyRepository) ReleaseAccount(ctx context.Context, id string) error {
        query := `
                UPDATE proxies SET
                        assigned_accounts = GREATEST(0, assigned_accounts - 1),
                        updated_at = $2
                WHERE id = $1`
        _, err := r.db.ExecContext(ctx, query, id, time.Now())
        return err
}

func (r *ProxyRepository) GetAvailableForRotation(ctx context.Context, group string, limit int) ([]*Proxy, error) {
        query := `
                SELECT id, name, host, port, username, password, type, region, provider,
                        status, is_active, tags, metadata, latency_ms, health_score,
                        rotation_weight, rotation_group, in_use, assigned_accounts,
                        max_accounts, max_connections, current_conns, bandwidth_mb,
                        bandwidth_limit_mb, created_at, updated_at
                FROM proxies
                WHERE is_active = true
                        AND status = 'healthy'
                        AND (rotation_group = $1 OR $1 = '')
                        AND (current_conns < max_connections OR max_connections = 0)
                        AND (bandwidth_limit_mb = 0 OR bandwidth_mb < bandwidth_limit_mb)
                ORDER BY rotation_weight DESC, health_score DESC, latency_ms ASC, created_at ASC
                LIMIT $2`

        rows, err := r.db.QueryContext(ctx, query, group, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        proxies := []*Proxy{}
        for rows.Next() {
                p := &Proxy{}
                var metaJSON []byte

                err := rows.Scan(
                        &p.ID, &p.Name, &p.Host, &p.Port, &p.Username, &p.Password, &p.Type, &p.Region,
                        &p.Provider, &p.Status, &p.IsActive, pq.Array(&p.Tags), &metaJSON,
                        &p.LatencyMs, &p.HealthScore, &p.RotationWeight, &p.RotationGroup,
                        &p.InUse, &p.AssignedAccounts, &p.MaxAccounts, &p.MaxConnections,
                        &p.CurrentConns, &p.BandwidthMB, &p.BandwidthLimitMB, &p.CreatedAt, &p.UpdatedAt,
                )
                if err != nil {
                        return nil, err
                }
                if len(metaJSON) > 0 {
                        json.Unmarshal(metaJSON, &p.Metadata)
                }
                proxies = append(proxies, p)
        }

        return proxies, nil
}

func (r *ProxyRepository) BulkDeactivateUnhealthy(ctx context.Context, maxFails int) error {
        query := `
                UPDATE proxies SET
                        is_active = false,
                        status = 'disabled',
                        updated_at = $2
                WHERE consecutive_fails >= $1
                        AND is_active = true`
        _, err := r.db.ExecContext(ctx, query, maxFails, time.Now())
        return err
}

func (r *ProxyRepository) GetStats(ctx context.Context) (*ProxyStats, error) {
        stats := &ProxyStats{
                TypeBreakdown:     make(map[string]int),
                RegionBreakdown:   make(map[string]int),
                ProviderBreakdown: make(map[string]int),
        }

        query := `
                SELECT
                        COUNT(*) as total,
                        COUNT(*) FILTER (WHERE is_active = true) as active,
                        COUNT(*) FILTER (WHERE status = 'healthy') as healthy,
                        COUNT(*) FILTER (WHERE status != 'healthy') as unhealthy,
                        COUNT(*) FILTER (WHERE in_use = true) as in_use,
                        COUNT(*) FILTER (
                                WHERE (current_conns >= max_connections AND max_connections > 0)
                                   OR (bandwidth_limit_mb > 0 AND bandwidth_mb >= bandwidth_limit_mb)
                        ) as overloaded,
                        COALESCE(AVG(health_score), 0) as avg_health,
                        COALESCE(AVG(latency_ms), 0) as avg_latency,
                        COALESCE(SUM(bandwidth_mb), 0) as total_bandwidth
                FROM proxies`

        err := r.db.QueryRowContext(ctx, query).Scan(
                &stats.TotalProxies,
                &stats.ActiveProxies,
                &stats.HealthyProxies,
                &stats.UnhealthyProxies,
                &stats.InUseProxies,
                &stats.OverloadedProxies,
                &stats.AverageHealthScore,
                &stats.AverageLatencyMs,
                &stats.TotalBandwidthMB,
        )
        if err != nil {
                return nil, err
        }

        typeQuery := `
                SELECT type, COUNT(*)
                FROM proxies
                GROUP BY type`
        typeRows, err := r.db.QueryContext(ctx, typeQuery)
        if err == nil {
                defer typeRows.Close()
                for typeRows.Next() {
                        var t string
                        var count int
                        if err := typeRows.Scan(&t, &count); err == nil {
                                stats.TypeBreakdown[t] = count
                        }
                }
        }

        regionQuery := `
                SELECT region, COUNT(*)
                FROM proxies
                GROUP BY region`
        regionRows, err := r.db.QueryContext(ctx, regionQuery)
        if err == nil {
                defer regionRows.Close()
                for regionRows.Next() {
                        var region string
                        var count int
                        if err := regionRows.Scan(&region, &count); err == nil {
                                stats.RegionBreakdown[region] = count
                        }
                }
        }

        providerQuery := `
                SELECT provider, COUNT(*)
                FROM proxies
                GROUP BY provider`
        providerRows, err := r.db.QueryContext(ctx, providerQuery)
        if err == nil {
                defer providerRows.Close()
                for providerRows.Next() {
                        var provider string
                        var count int
                        if err := providerRows.Scan(&provider, &count); err == nil {
                                stats.ProviderBreakdown[provider] = count
                        }
                }
        }

        return stats, nil
}
