package models

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type ProxyType string

const (
	ProxyTypeHTTP   ProxyType = "http"
	ProxyTypeHTTPS  ProxyType = "https"
	ProxyTypeSOCKS5 ProxyType = "socks5"
	ProxyTypeSOCKS4 ProxyType = "socks4"
)

type ProxyStatus string

const (
	ProxyStatusActive    ProxyStatus = "active"
	ProxyStatusInactive  ProxyStatus = "inactive"
	ProxyStatusTesting   ProxyStatus = "testing"
	ProxyStatusFailed    ProxyStatus = "failed"
	ProxyStatusSuspended ProxyStatus = "suspended"
	ProxyStatusWarning   ProxyStatus = "warning"
)

type Proxy struct {
	ID                  string            `json:"id" db:"id"`
	TenantID            string            `json:"tenant_id" db:"tenant_id"`
	Name                string            `json:"name" db:"name"`
	Type                ProxyType         `json:"type" db:"type"`
	Host                string            `json:"host" db:"host"`
	Port                int               `json:"port" db:"port"`
	Username            string            `json:"username" db:"username"`
	Password            string            `json:"password,omitempty" db:"password"`
	Status              ProxyStatus       `json:"status" db:"status"`
	IsDefault           bool              `json:"is_default" db:"is_default"`
	IsAnonymous         bool              `json:"is_anonymous" db:"is_anonymous"`
	Priority            int               `json:"priority" db:"priority"`
	Weight              float64           `json:"weight" db:"weight"`
	Tags                []string          `json:"tags" db:"tags"`
	HealthMetrics       ProxyHealth       `json:"health_metrics"`
	Stats               ProxyStats        `json:"stats"`
	RotationInfo        ProxyRotation     `json:"rotation_info"`
	Limits              ProxyLimits       `json:"limits"`
	GeoLocation         *GeoLocation      `json:"geo_location,omitempty"`
	SpeedTest           *SpeedTestResult  `json:"speed_test,omitempty"`
	LastTestedAt        *time.Time        `json:"last_tested_at" db:"last_tested_at"`
	LastUsedAt          *time.Time        `json:"last_used_at" db:"last_used_at"`
	LastHealthCheckAt   *time.Time        `json:"last_health_check_at" db:"last_health_check_at"`
	LastErrorAt         *time.Time        `json:"last_error_at" db:"last_error_at"`
	LastError           string            `json:"last_error" db:"last_error"`
	CreatedAt           time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at" db:"updated_at"`
	CreatedBy           string            `json:"created_by" db:"created_by"`
	UpdatedBy           string            `json:"updated_by" db:"updated_by"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

type ProxyHealth struct {
	HealthScore          float64   `json:"health_score"`
	IsHealthy            bool      `json:"is_healthy"`
	ConsecutiveSuccesses int       `json:"consecutive_successes"`
	ConsecutiveFailures  int       `json:"consecutive_failures"`
	TotalSuccesses       int64     `json:"total_successes"`
	TotalFailures        int64     `json:"total_failures"`
	SuccessRate          float64   `json:"success_rate"`
	FailureRate          float64   `json:"failure_rate"`
	AverageResponseTimeMs float64  `json:"average_response_time_ms"`
	LastResponseTimeMs    float64  `json:"last_response_time_ms"`
	TimeoutCount         int64     `json:"timeout_count"`
	ConnectionRefused    int64     `json:"connection_refused"`
	LastCalculatedAt     time.Time `json:"last_calculated_at"`
}

type ProxyStats struct {
	TotalRequests       int64     `json:"total_requests"`
	TotalSuccesses      int64     `json:"total_successes"`
	TotalFailures       int64     `json:"total_failures"`
	TotalTimeouts       int64     `json:"total_timeouts"`
	TotalBytesTransferred int64   `json:"total_bytes_transferred"`
	AverageLatencyMs    float64   `json:"average_latency_ms"`
	MinLatencyMs        float64   `json:"min_latency_ms"`
	MaxLatencyMs        float64   `json:"max_latency_ms"`
	Uptime              float64   `json:"uptime"`
	LastUpdatedAt       time.Time `json:"last_updated_at"`
}

type ProxyRotation struct {
	TimesUsed        int64      `json:"times_used"`
	TimesRotated     int64      `json:"times_rotated"`
	LastRotatedAt    *time.Time `json:"last_rotated_at,omitempty"`
	RotationWeight   float64    `json:"rotation_weight"`
	UsesSinceRotation int64     `json:"uses_since_rotation"`
}

type ProxyLimits struct {
	MaxConnections      int   `json:"max_connections"`
	MaxRequestsPerHour  int   `json:"max_requests_per_hour"`
	MaxRequestsPerDay   int   `json:"max_requests_per_day"`
	RequestsThisHour    int64 `json:"requests_this_hour"`
	RequestsToday       int64 `json:"requests_today"`
	CurrentConnections  int   `json:"current_connections"`
	BandwidthLimitMB    int   `json:"bandwidth_limit_mb"`
	BandwidthUsedMB     int64 `json:"bandwidth_used_mb"`
}

type GeoLocation struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`
	Organization string `json:"organization"`
}

type SpeedTestResult struct {
	DownloadSpeedMbps float64   `json:"download_speed_mbps"`
	UploadSpeedMbps   float64   `json:"upload_speed_mbps"`
	LatencyMs         float64   `json:"latency_ms"`
	JitterMs          float64   `json:"jitter_ms"`
	PacketLoss        float64   `json:"packet_loss"`
	TestedAt          time.Time `json:"tested_at"`
}

func NewProxy(host string, port int, proxyType ProxyType, tenantID, createdBy string) (*Proxy, error) {
	if host == "" {
		return nil, errors.New("proxy host is required")
	}
	if port <= 0 || port > 65535 {
		return nil, errors.New("proxy port must be between 1 and 65535")
	}

	now := time.Now()
	return &Proxy{
		ID:        generateProxyID(),
		TenantID:  tenantID,
		Name:      fmt.Sprintf("%s:%d", host, port),
		Type:      proxyType,
		Host:      host,
		Port:      port,
		Status:    ProxyStatusInactive,
		Priority:  100,
		Weight:    1.0,
		Tags:      []string{},
		HealthMetrics: ProxyHealth{
			HealthScore:      100.0,
			IsHealthy:        true,
			LastCalculatedAt: now,
		},
		Stats: ProxyStats{
			LastUpdatedAt: now,
		},
		RotationInfo: ProxyRotation{
			RotationWeight: 1.0,
		},
		Limits: ProxyLimits{
			MaxConnections:     100,
			MaxRequestsPerHour: 1000,
			MaxRequestsPerDay:  10000,
			BandwidthLimitMB:   10240,
		},
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: createdBy,
		UpdatedBy: createdBy,
		Metadata:  make(map[string]interface{}),
	}, nil
}

func (p *Proxy) Validate() error {
	if p.Host == "" {
		return errors.New("proxy host is required")
	}
	if p.Port <= 0 || p.Port > 65535 {
		return errors.New("proxy port must be between 1 and 65535")
	}
	if p.TenantID == "" {
		return errors.New("tenant ID is required")
	}
	if !p.Type.IsValid() {
		return fmt.Errorf("invalid proxy type: %s", p.Type)
	}
	if !p.Status.IsValid() {
		return fmt.Errorf("invalid proxy status: %s", p.Status)
	}
	if p.Priority < 0 || p.Priority > 1000 {
		return errors.New("priority must be between 0 and 1000")
	}
	if p.Weight < 0 || p.Weight > 100 {
		return errors.New("weight must be between 0 and 100")
	}
	return nil
}

func (p *Proxy) IsActive() bool {
	return p.Status == ProxyStatusActive
}

func (p *Proxy) IsHealthy() bool {
	return p.HealthMetrics.IsHealthy && p.HealthMetrics.HealthScore >= 70.0
}

func (p *Proxy) CanUse() bool {
	if !p.IsActive() {
		return false
	}
	if !p.IsHealthy() {
		return false
	}
	if p.Limits.MaxConnections > 0 && p.Limits.CurrentConnections >= p.Limits.MaxConnections {
		return false
	}
	if p.Limits.MaxRequestsPerHour > 0 && p.Limits.RequestsThisHour >= int64(p.Limits.MaxRequestsPerHour) {
		return false
	}
	if p.Limits.MaxRequestsPerDay > 0 && p.Limits.RequestsToday >= int64(p.Limits.MaxRequestsPerDay) {
		return false
	}
	return true
}

func (p *Proxy) Activate() error {
	if p.Status == ProxyStatusActive {
		return errors.New("proxy is already active")
	}
	p.Status = ProxyStatusActive
	p.UpdatedAt = time.Now()
	return nil
}

func (p *Proxy) Deactivate() error {
	if p.Status == ProxyStatusInactive {
		return errors.New("proxy is already inactive")
	}
	p.Status = ProxyStatusInactive
	p.UpdatedAt = time.Now()
	return nil
}

func (p *Proxy) MarkAsTesting() {
	p.Status = ProxyStatusTesting
	p.UpdatedAt = time.Now()
}

func (p *Proxy) MarkAsFailed(reason string) {
	p.Status = ProxyStatusFailed
	p.LastError = reason
	now := time.Now()
	p.LastErrorAt = &now
	p.UpdatedAt = now
}

func (p *Proxy) MarkAsSuspended(reason string) {
	p.Status = ProxyStatusSuspended
	p.LastError = reason
	p.UpdatedAt = time.Now()
}

func (p *Proxy) RecordSuccess(responseTimeMs float64) {
	p.HealthMetrics.ConsecutiveSuccesses++
	p.HealthMetrics.ConsecutiveFailures = 0
	p.HealthMetrics.TotalSuccesses++
	p.HealthMetrics.LastResponseTimeMs = responseTimeMs
	
	p.Stats.TotalRequests++
	p.Stats.TotalSuccesses++
	
	if p.Stats.MinLatencyMs == 0 || responseTimeMs < p.Stats.MinLatencyMs {
		p.Stats.MinLatencyMs = responseTimeMs
	}
	if responseTimeMs > p.Stats.MaxLatencyMs {
		p.Stats.MaxLatencyMs = responseTimeMs
	}
	
	p.Limits.RequestsThisHour++
	p.Limits.RequestsToday++
	
	p.calculateHealthScore()
	p.calculateAverageResponseTime()
	p.calculateAverageLatency()
	
	now := time.Now()
	p.LastUsedAt = &now
	p.Stats.LastUpdatedAt = now
	p.UpdatedAt = now
}

func (p *Proxy) RecordFailure(errorMsg string, isTimeout bool) {
	p.HealthMetrics.ConsecutiveFailures++
	p.HealthMetrics.ConsecutiveSuccesses = 0
	p.HealthMetrics.TotalFailures++
	
	p.Stats.TotalRequests++
	p.Stats.TotalFailures++
	
	if isTimeout {
		p.HealthMetrics.TimeoutCount++
		p.Stats.TotalTimeouts++
	}
	
	if strings.Contains(strings.ToLower(errorMsg), "connection refused") {
		p.HealthMetrics.ConnectionRefused++
	}
	
	now := time.Now()
	p.LastError = errorMsg
	p.LastErrorAt = &now
	
	p.calculateHealthScore()
	
	p.Stats.LastUpdatedAt = now
	p.UpdatedAt = now
}

func (p *Proxy) calculateHealthScore() {
	total := p.HealthMetrics.TotalSuccesses + p.HealthMetrics.TotalFailures
	if total == 0 {
		p.HealthMetrics.HealthScore = 100.0
		p.HealthMetrics.IsHealthy = true
		p.HealthMetrics.SuccessRate = 0
		p.HealthMetrics.FailureRate = 0
		p.HealthMetrics.LastCalculatedAt = time.Now()
		return
	}
	
	successRate := float64(p.HealthMetrics.TotalSuccesses) / float64(total)
	p.HealthMetrics.SuccessRate = successRate
	p.HealthMetrics.FailureRate = 1.0 - successRate
	
	baseScore := successRate * 100.0
	
	consecutiveFailurePenalty := float64(p.HealthMetrics.ConsecutiveFailures) * 10.0
	timeoutPenalty := float64(p.HealthMetrics.TimeoutCount) * 2.0
	connectionRefusedPenalty := float64(p.HealthMetrics.ConnectionRefused) * 5.0
	
	score := baseScore - consecutiveFailurePenalty - timeoutPenalty - connectionRefusedPenalty
	
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	
	p.HealthMetrics.HealthScore = score
	p.HealthMetrics.IsHealthy = score >= 70.0
	p.HealthMetrics.LastCalculatedAt = time.Now()
}

func (p *Proxy) calculateAverageResponseTime() {
	total := p.HealthMetrics.TotalSuccesses + p.HealthMetrics.TotalFailures
	if total == 0 {
		return
	}
	
	if p.HealthMetrics.AverageResponseTimeMs == 0 {
		p.HealthMetrics.AverageResponseTimeMs = p.HealthMetrics.LastResponseTimeMs
	} else {
		p.HealthMetrics.AverageResponseTimeMs = 
			(p.HealthMetrics.AverageResponseTimeMs*float64(total-1) + p.HealthMetrics.LastResponseTimeMs) / float64(total)
	}
}

func (p *Proxy) calculateAverageLatency() {
	if p.Stats.TotalSuccesses == 0 {
		return
	}
	
	totalLatency := p.Stats.MinLatencyMs + p.Stats.MaxLatencyMs
	p.Stats.AverageLatencyMs = totalLatency / 2.0
}

func (p *Proxy) IncrementUsage() {
	p.RotationInfo.TimesUsed++
	p.RotationInfo.UsesSinceRotation++
	now := time.Now()
	p.LastUsedAt = &now
	p.UpdatedAt = now
}

func (p *Proxy) ResetRotationCounter() {
	p.RotationInfo.UsesSinceRotation = 0
	now := time.Now()
	p.RotationInfo.LastRotatedAt = &now
	p.RotationInfo.TimesRotated++
	p.UpdatedAt = now
}

func (p *Proxy) GetProxyURL() string {
	if p.Username != "" && p.Password != "" {
		return fmt.Sprintf("%s://%s:%s@%s:%d", 
			p.Type, 
			url.QueryEscape(p.Username), 
			url.QueryEscape(p.Password), 
			p.Host, 
			p.Port)
	}
	return fmt.Sprintf("%s://%s:%d", p.Type, p.Host, p.Port)
}

func (p *Proxy) GetAddress() string {
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}

func (p *Proxy) HasAuthentication() bool {
	return p.Username != "" && p.Password != ""
}

func (p *Proxy) TestConnection() error {
	p.MarkAsTesting()
	now := time.Now()
	p.LastTestedAt = &now
	p.UpdatedAt = now
	return nil
}

func (p *Proxy) NeedsHealthCheck() bool {
	if p.LastHealthCheckAt == nil {
		return true
	}
	return time.Since(*p.LastHealthCheckAt) > 5*time.Minute
}

func (p *Proxy) UpdateHealthCheck() {
	now := time.Now()
	p.LastHealthCheckAt = &now
	p.UpdatedAt = now
}

func (p *Proxy) SetGeoLocation(geo *GeoLocation) {
	p.GeoLocation = geo
	p.UpdatedAt = time.Now()
}

func (p *Proxy) SetSpeedTest(speedTest *SpeedTestResult) {
	p.SpeedTest = speedTest
	p.UpdatedAt = time.Now()
}

func (p *Proxy) ResetLimits() {
	p.Limits.RequestsThisHour = 0
	p.Limits.RequestsToday = 0
	p.Limits.BandwidthUsedMB = 0
	p.Limits.CurrentConnections = 0
	p.UpdatedAt = time.Now()
}

func (p *Proxy) AddTag(tag string) {
	if p.Tags == nil {
		p.Tags = []string{}
	}
	for _, t := range p.Tags {
		if t == tag {
			return
		}
	}
	p.Tags = append(p.Tags, tag)
	p.UpdatedAt = time.Now()
}

func (p *Proxy) RemoveTag(tag string) {
	if p.Tags == nil {
		return
	}
	for i, t := range p.Tags {
		if t == tag {
			p.Tags = append(p.Tags[:i], p.Tags[i+1:]...)
			p.UpdatedAt = time.Now()
			break
		}
	}
}

func (p *Proxy) HasTag(tag string) bool {
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func (p *Proxy) Clone() *Proxy {
	clone := *p
	clone.ID = generateProxyID()
	now := time.Now()
	clone.CreatedAt = now
	clone.UpdatedAt = now
	clone.Stats = ProxyStats{LastUpdatedAt: now}
	clone.HealthMetrics.LastCalculatedAt = now
	clone.LastTestedAt = nil
	clone.LastUsedAt = nil
	clone.LastHealthCheckAt = nil
	clone.LastErrorAt = nil
	return &clone
}

func (p *Proxy) GetLatency() float64 {
	return p.HealthMetrics.AverageResponseTimeMs
}

func (p *Proxy) GetSuccessRate() float64 {
	return p.HealthMetrics.SuccessRate
}

func (p *Proxy) IsOverloaded() bool {
	if p.Limits.MaxConnections > 0 && p.Limits.CurrentConnections >= p.Limits.MaxConnections {
		return true
	}
	if p.Limits.MaxRequestsPerHour > 0 && p.Limits.RequestsThisHour >= int64(p.Limits.MaxRequestsPerHour) {
		return true
	}
	return false
}

func (p *Proxy) CalculateUptime() float64 {
	total := p.HealthMetrics.TotalSuccesses + p.HealthMetrics.TotalFailures
	if total == 0 {
		return 100.0
	}
	uptime := (float64(p.HealthMetrics.TotalSuccesses) / float64(total)) * 100.0
	p.Stats.Uptime = uptime
	return uptime
}

func (p *Proxy) GetQualityScore() float64 {
	healthWeight := 0.4
	latencyWeight := 0.3
	uptimeWeight := 0.3
	
	healthScore := p.HealthMetrics.HealthScore
	
	latencyScore := 100.0
	if p.HealthMetrics.AverageResponseTimeMs > 0 {
		latencyScore = 100.0 - (p.HealthMetrics.AverageResponseTimeMs / 10.0)
		if latencyScore < 0 {
			latencyScore = 0
		}
	}
	
	uptimeScore := p.CalculateUptime()
	
	qualityScore := (healthScore * healthWeight) + 
		(latencyScore * latencyWeight) + 
		(uptimeScore * uptimeWeight)
	
	return qualityScore
}

func (pt ProxyType) IsValid() bool {
	switch pt {
	case ProxyTypeHTTP, ProxyTypeHTTPS, ProxyTypeSOCKS5, ProxyTypeSOCKS4:
		return true
	}
	return false
}

func (ps ProxyStatus) IsValid() bool {
	switch ps {
	case ProxyStatusActive, ProxyStatusInactive, ProxyStatusTesting, 
		ProxyStatusFailed, ProxyStatusSuspended, ProxyStatusWarning:
		return true
	}
	return false
}

func generateProxyID() string {
	return fmt.Sprintf("prx_%d", time.Now().UnixNano())
}

func (p *Proxy) GetConnectionString() string {
	return p.GetProxyURL()
}

func (p *Proxy) ParseFromURL(proxyURL string) error {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}
	
	p.Type = ProxyType(u.Scheme)
	p.Host = u.Hostname()
	
	if u.Port() != "" {
		fmt.Sscanf(u.Port(), "%d", &p.Port)
	}
	
	if u.User != nil {
		p.Username = u.User.Username()
		password, _ := u.User.Password()
		p.Password = password
	}
	
	p.UpdatedAt = time.Now()
	return nil
}

func (p *Proxy) ShouldAutoSuspend() bool {
	if p.HealthMetrics.ConsecutiveFailures >= 10 {
		return true
	}
	if p.HealthMetrics.HealthScore < 30.0 {
		return true
	}
	if p.HealthMetrics.FailureRate > 0.7 {
		return true
	}
	return false
}

func (p *Proxy) IncrementConnection() {
	p.Limits.CurrentConnections++
	p.UpdatedAt = time.Now()
}

func (p *Proxy) DecrementConnection() {
	if p.Limits.CurrentConnections > 0 {
		p.Limits.CurrentConnections--
	}
	p.UpdatedAt = time.Now()
}

func (p *Proxy) RecordBandwidth(bytesTransferred int64) {
	p.Stats.TotalBytesTransferred += bytesTransferred
	p.Limits.BandwidthUsedMB += bytesTransferred / (1024 * 1024)
	p.UpdatedAt = time.Now()
}
