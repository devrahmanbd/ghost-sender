package handlers

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
	"strconv"
	"fmt"
	"github.com/gorilla/mux"

	"email-campaign-system/internal/storage/repository"
	"email-campaign-system/pkg/errors"
	"email-campaign-system/pkg/logger"
)

// ============================================================================
// HANDLER STRUCT
// ============================================================================

type MetricsHandler struct {
	campaignRepo  *repository.CampaignRepository
	accountRepo   *repository.AccountRepository
	templateRepo  *repository.TemplateRepository
	recipientRepo *repository.RecipientRepository
	proxyRepo     *repository.ProxyRepository
	statsRepo     *repository.StatsRepository
	logger        logger.Logger // Interface, not pointer
	startTime     time.Time
}

func NewMetricsHandler(
	campaignRepo *repository.CampaignRepository,
	accountRepo *repository.AccountRepository,
	templateRepo *repository.TemplateRepository,
	recipientRepo *repository.RecipientRepository,
	proxyRepo *repository.ProxyRepository,
	statsRepo *repository.StatsRepository,
	log logger.Logger, // Interface, not pointer
) *MetricsHandler {
	return &MetricsHandler{
		campaignRepo:  campaignRepo,
		accountRepo:   accountRepo,
		templateRepo:  templateRepo,
		recipientRepo: recipientRepo,
		proxyRepo:     proxyRepo,
		statsRepo:     statsRepo,
		logger:        log,
		startTime:     time.Now(),
	}
}

// ============================================================================
// RESPONSE TYPES
// ============================================================================

type SystemMetrics struct {
	CPUUsage      float64 `json:"cpu_usage_percent"`
	MemoryUsed    uint64  `json:"memory_used_bytes"`
	MemoryTotal   uint64  `json:"memory_total_bytes"`
	MemoryPercent float64 `json:"memory_percent"`
	Goroutines    int     `json:"goroutines"`
	HeapAlloc     uint64  `json:"heap_alloc_bytes"`
	HeapSys       uint64  `json:"heap_sys_bytes"`
	NumGC         uint32  `json:"num_gc"`
	Uptime        int64   `json:"uptime_seconds"`
	Timestamp     int64   `json:"timestamp"`
}

type DashboardMetrics struct {
	TotalCampaigns    int     `json:"total_campaigns"`
	ActiveCampaigns   int     `json:"active_campaigns"`
	CompletedCampaigns int    `json:"completed_campaigns"`
	FailedCampaigns   int     `json:"failed_campaigns"`
	TotalAccounts     int     `json:"total_accounts"`
	ActiveAccounts    int     `json:"active_accounts"`
	SuspendedAccounts int     `json:"suspended_accounts"`
	TotalTemplates    int     `json:"total_templates"`
	TotalRecipients   int     `json:"total_recipients"`
	TotalProxies      int     `json:"total_proxies"`
	HealthyProxies    int     `json:"healthy_proxies"`
	EmailsSentToday   int64   `json:"emails_sent_today"`
	EmailsSentTotal   int64   `json:"emails_sent_total"`
	EmailsFailed      int64   `json:"emails_failed"`
	SuccessRate       float64 `json:"success_rate"`
	SystemUptime      int64   `json:"system_uptime_seconds"`
}

type CampaignMetrics struct {
	CampaignID      string  `json:"campaign_id"`
	CampaignName    string  `json:"campaign_name"`
	Status          string  `json:"status"`
	TotalRecipients int     `json:"total_recipients"`
	Sent            int     `json:"sent"`
	Failed          int     `json:"failed"`
	Pending         int     `json:"pending"`
	SuccessRate     float64 `json:"success_rate"`
	Progress        float64 `json:"progress_percent"`
	Throughput      float64 `json:"throughput_per_minute"`
	AvgResponseTime int     `json:"avg_response_time_ms"`
	EstimatedTime   int     `json:"estimated_time_seconds"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

type SendingMetrics struct {
	TotalSent       int64   `json:"total_sent"`
	TotalFailed     int64   `json:"total_failed"`
	TotalPending    int64   `json:"total_pending"`
	SuccessRate     float64 `json:"success_rate"`
	ThroughputHour  float64 `json:"throughput_per_hour"`
	ThroughputDay   float64 `json:"throughput_per_day"`
	AvgResponseTime int     `json:"avg_response_time_ms"`
	ActiveCampaigns int     `json:"active_campaigns"`
	QueueSize       int     `json:"queue_size"`
	LastSentTime    int64   `json:"last_sent_time"`
}

type TimeSeriesDataPoint struct {
	Timestamp int64       `json:"timestamp"`
	Value     interface{} `json:"value"`
}

type TimeSeriesResponse struct {
	Metric   string                `json:"metric"`
	Interval string                `json:"interval"`
	Duration string                `json:"duration"`
	Data     []TimeSeriesDataPoint `json:"data"`
}

// ============================================================================
// HANDLERS
// ============================================================================

// GetSystemMetrics returns system-level metrics (CPU, memory, goroutines, etc.)
func (h *MetricsHandler) GetSystemMetrics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// CPU usage would require tracking over time
	cpuUsage := 0.0

	metrics := SystemMetrics{
		CPUUsage:      cpuUsage,
		MemoryUsed:    m.Alloc,
		MemoryTotal:   m.Sys,
		MemoryPercent: float64(m.Alloc) / float64(m.Sys) * 100,
		Goroutines:    runtime.NumGoroutine(),
		HeapAlloc:     m.HeapAlloc,
		HeapSys:       m.HeapSys,
		NumGC:         m.NumGC,
		Uptime:        int64(time.Since(h.startTime).Seconds()),
		Timestamp:     time.Now().Unix(),
	}

	h.respondJSON(w, http.StatusOK, metrics)
}

// GetDashboardMetrics returns aggregated metrics for the dashboard
func (h *MetricsHandler) GetDashboardMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get campaign stats
	campaignStats, err := h.campaignRepo.GetStats(ctx)
	if err != nil {
		h.logger.Error("Failed to get campaign stats", logger.Error(err))
		// Continue with zero values
	}

	// Get account stats
	accountStats, err := h.accountRepo.GetStats(ctx)
	if err != nil {
		h.logger.Error("Failed to get account stats", logger.Error(err))
		// Continue with zero values
	}

	// Get recipient stats
	recipientStats, err := h.recipientRepo.GetStats(ctx, "")
	if err != nil {
		h.logger.Error("Failed to get recipient stats", logger.Error(err))
		// Continue with zero values
	}

	// Get counts from repositories using the new Count methods
	totalTemplates, _ := h.templateRepo.Count(ctx, &repository.TemplateFilter{})
	totalProxies, _ := h.proxyRepo.Count(ctx, &repository.ProxyFilter{})
	healthyProxies, _ := h.proxyRepo.CountHealthy(ctx)

	// Extract values with nil checks
	totalCampaigns := 0
	activeCampaigns := 0
	completedCampaigns := 0
	failedCampaigns := 0
	emailsSentTotal := int64(0)
	emailsFailed := int64(0)

	if campaignStats != nil {
		totalCampaigns = campaignStats.TotalCampaigns
		activeCampaigns = campaignStats.ActiveCampaigns
		completedCampaigns = campaignStats.CompletedCampaigns
		failedCampaigns = campaignStats.FailedCampaigns
		emailsSentTotal = campaignStats.TotalSent
		emailsFailed = campaignStats.TotalFailed
	}

	totalAccounts := 0
	activeAccounts := 0
	suspendedAccounts := 0

	if accountStats != nil {
		totalAccounts = accountStats.TotalAccounts
		activeAccounts = accountStats.ActiveAccounts
		suspendedAccounts = accountStats.SuspendedAccounts
	}

	totalRecipients := 0
	if recipientStats != nil {
		totalRecipients = recipientStats.TotalRecipients
	}

	// Calculate success rate
	successRate := calculateSuccessRate(emailsSentTotal, emailsFailed)

	metrics := DashboardMetrics{
		TotalCampaigns:     totalCampaigns,
		ActiveCampaigns:    activeCampaigns,
		CompletedCampaigns: completedCampaigns,
		FailedCampaigns:    failedCampaigns,
		TotalAccounts:      totalAccounts,
		ActiveAccounts:     activeAccounts,
		SuspendedAccounts:  suspendedAccounts,
		TotalTemplates:     totalTemplates,
		TotalRecipients:    totalRecipients,
		TotalProxies:       totalProxies,
		HealthyProxies:     healthyProxies,
		EmailsSentToday:    emailsSentTotal, // Simplified for now
		EmailsSentTotal:    emailsSentTotal,
		EmailsFailed:       emailsFailed,
		SuccessRate:        successRate,
		SystemUptime:       int64(time.Since(h.startTime).Seconds()),
	}

	h.respondJSON(w, http.StatusOK, metrics)
}

// GetCampaignMetrics returns detailed metrics for a specific campaign
func (h *MetricsHandler) GetCampaignMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	campaignID := vars["id"]
	if campaignID == "" {
		h.respondError(w, errors.BadRequest("Invalid campaign ID"))
		return
	}

	// Get campaign details
	campaign, err := h.campaignRepo.GetByID(ctx, campaignID)
	if err != nil {
		h.logger.Error("Failed to get campaign",
			logger.String("campaign_id", campaignID),
			logger.Error(err))
		h.respondError(w, err)
		return
	}

	// Calculate metrics from campaign data
	totalRecipients := campaign.TotalRecipients
	sent := campaign.SentCount
	failed := campaign.FailedCount
	pending := campaign.PendingCount

	progress := campaign.Progress
	successRate := campaign.SuccessRate
	throughput := campaign.Throughput

	// Estimate time remaining
	estimatedTime := 0
	if throughput > 0 && pending > 0 {
		estimatedTime = int(float64(pending) / throughput * 60) // Convert to seconds
	}

	metrics := CampaignMetrics{
		CampaignID:      campaignID,
		CampaignName:    campaign.Name,
		Status:          campaign.Status,
		TotalRecipients: totalRecipients,
		Sent:            sent,
		Failed:          failed,
		Pending:         pending,
		SuccessRate:     successRate,
		Progress:        progress,
		Throughput:      throughput,
		AvgResponseTime: 0, 
		EstimatedTime:   estimatedTime,
		StartedAt:       campaign.StartedAt,
		CompletedAt:     campaign.CompletedAt,
	}

	h.respondJSON(w, http.StatusOK, metrics)
}

// GetSendingMetrics returns overall sending statistics
func (h *MetricsHandler) GetSendingMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get overall campaign stats
	stats, err := h.campaignRepo.GetStats(ctx)
	if err != nil {
		h.logger.Error("Failed to get sending metrics", logger.Error(err))
		h.respondError(w, err)
		return
	}

	successRate := calculateSuccessRate(stats.TotalSent, stats.TotalFailed)

	// Calculate throughput
	throughputHour := stats.AverageThroughput * 60  // per minute to per hour
	throughputDay := throughputHour * 24            // per hour to per day

	// Calculate total pending from active campaigns
	totalPending := int64(0)
	activeCampaigns, err := h.campaignRepo.GetByStatus(ctx, "running")
	if err == nil {
		for _, campaign := range activeCampaigns {
			totalPending += int64(campaign.PendingCount)
		}
	}

	metrics := SendingMetrics{
		TotalSent:       stats.TotalSent,
		TotalFailed:     stats.TotalFailed,
		TotalPending:    totalPending,
		SuccessRate:     successRate,
		ThroughputHour:  throughputHour,
		ThroughputDay:   throughputDay,
		AvgResponseTime: 0, // Would need separate tracking
		ActiveCampaigns: stats.ActiveCampaigns,
		QueueSize:       int(totalPending),
		LastSentTime:    time.Now().Unix(),
	}

	h.respondJSON(w, http.StatusOK, metrics)
}

// GetTimeSeries returns time-series data for a specific metric
func (h *MetricsHandler) GetTimeSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	metric := query.Get("metric")
	interval := query.Get("interval")
	duration := query.Get("duration")

	// Validate required parameters
	if metric == "" {
		h.respondError(w, errors.BadRequest("Metric type is required"))
		return
	}

	// Set defaults
	if interval == "" {
		interval = "1h"
	}

	if duration == "" {
		duration = "24h"
	}

	// Parse duration
	parsedDuration, err := time.ParseDuration(duration)
	if err != nil {
		h.respondError(w, errors.BadRequest("Invalid duration format"))
		return
	}

	// Get current stats
	stats, err := h.campaignRepo.GetStats(ctx)
	if err != nil {
		h.logger.Error("Failed to get time series data", logger.Error(err))
		h.respondError(w, err)
		return
	}

	// Generate time series data points
	now := time.Now()
	startTime := now.Add(-parsedDuration)

	// For now, return simplified data
	// In production, you'd query historical data from a time-series DB
	dataPoints := []TimeSeriesDataPoint{
		{
			Timestamp: startTime.Unix(),
			Value:     0,
		},
		{
			Timestamp: now.Unix(),
			Value:     stats.TotalSent,
		},
	}

	response := TimeSeriesResponse{
		Metric:   metric,
		Interval: interval,
		Duration: duration,
		Data:     dataPoints,
	}

	h.respondJSON(w, http.StatusOK, response)
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// calculateSuccessRate calculates the success rate as a percentage
func calculateSuccessRate(sent, failed int64) float64 {
	total := sent + failed
	if total == 0 {
		return 0.0
	}
	return (float64(sent) / float64(total)) * 100
}

// respondJSON sends a JSON response
func (h *MetricsHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", logger.Error(err))
	}
}

// respondError sends an error response
func (h *MetricsHandler) respondError(w http.ResponseWriter, err error) {
	var status int
	var message string

	// Check if it's our custom error type
	if appErr, ok := err.(*errors.Error); ok {
		status = appErr.StatusCode
		message = appErr.Message
	} else {
		status = http.StatusInternalServerError
		message = "Internal server error"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	
	response := map[string]interface{}{
		"error":   message,
		"status":  status,
		"success": false,
	}
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode error response", logger.Error(err))
	}
}
// Add this helper method to your MetricsHandler
func (h *MetricsHandler) getIntQuery(r *http.Request, key string, defaultValue int) int {
    if val := r.URL.Query().Get(key); val != "" {
        if parsed, err := strconv.Atoi(val); err == nil {
            return parsed
        }
    }
    return defaultValue
}
func (h *MetricsHandler) GetAccountMetrics(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    accountStats, err := h.accountRepo.GetStats(ctx)
    if err != nil {
        h.logger.Error("Failed to get account metrics", logger.Error(err))
        h.respondError(w, err)
        return
    }
    metrics := map[string]interface{}{
        "total_accounts":     accountStats.TotalAccounts,
        "active_accounts":    accountStats.ActiveAccounts,
        "suspended_accounts": accountStats.SuspendedAccounts,
        "total_sent":         accountStats.TotalSent,
        "total_failed":       accountStats.TotalFailed,
    }

    // Add health metrics if available
    // If your AccountStatsResult has these fields, uncomment:
    // "healthy_accounts":   accountStats.HealthyAccounts,
    // "degraded_accounts":  accountStats.DegradedAccounts,
    // "unhealthy_accounts": accountStats.UnhealthyAccounts,
    // "average_health":     accountStats.AverageHealth,

    h.respondJSON(w, http.StatusOK, metrics)
}


// GetLogs returns application logs (simplified version)
func (h *MetricsHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
    level := r.URL.Query().Get("level")
    limit := h.getIntQuery(r, "limit", 100)
    offset := h.getIntQuery(r, "offset", 0)

    // In production, you would query from a log storage system
    // For now, return mock data
    logs := []map[string]interface{}{
        {
            "timestamp":   time.Now().Add(-1 * time.Hour).Unix(),
            "level":       "info",
            "message":     "Campaign started",
            "campaign_id": "camp_123",
        },
        {
            "timestamp":  time.Now().Add(-30 * time.Minute).Unix(),
            "level":      "warn",
            "message":    "Rate limit approaching",
            "account_id": "acc_456",
        },
        {
            "timestamp": time.Now().Add(-15 * time.Minute).Unix(),
            "level":     "error",
            "message":   "Failed to send email",
            "error":     "SMTP timeout",
        },
    }

    // Filter by level if provided
    if level != "" {
        filtered := make([]map[string]interface{}, 0)
        for _, log := range logs {
            if log["level"] == level {
                filtered = append(filtered, log)
            }
        }
        logs = filtered
    }

    // Apply pagination
    start := offset
    end := offset + limit
    if start > len(logs) {
        start = len(logs)
    }
    if end > len(logs) {
        end = len(logs)
    }

    h.respondJSON(w, http.StatusOK, map[string]interface{}{
        "logs":   logs[start:end],
        "total":  len(logs),
        "limit":  limit,
        "offset": offset,
    })
}

// GetCampaignLogs returns logs for a specific campaign
func (h *MetricsHandler) GetCampaignLogs(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    campaignID := vars["id"]
    
    if campaignID == "" {
        h.respondError(w, errors.BadRequest("Invalid campaign ID"))
        return
    }

    limit := h.getIntQuery(r, "limit", 100)
    offset := h.getIntQuery(r, "offset", 0)

    // In production, query from log storage filtered by campaign_id
    logs := []map[string]interface{}{
        {
            "timestamp":   time.Now().Add(-1 * time.Hour).Unix(),
            "level":       "info",
            "message":     "Campaign started",
            "campaign_id": campaignID,
        },
        {
            "timestamp":   time.Now().Add(-30 * time.Minute).Unix(),
            "level":       "info",
            "message":     "Email sent successfully",
            "campaign_id": campaignID,
            "recipient":   "user@example.com",
        },
        {
            "timestamp":   time.Now().Add(-10 * time.Minute).Unix(),
            "level":       "error",
            "message":     "Email delivery failed",
            "campaign_id": campaignID,
            "recipient":   "invalid@example.com",
            "error":       "Invalid recipient",
        },
    }

    // Apply pagination
    start := offset
    end := offset + limit
    if start > len(logs) {
        start = len(logs)
    }
    if end > len(logs) {
        end = len(logs)
    }

    h.respondJSON(w, http.StatusOK, map[string]interface{}{
        "logs":        logs[start:end],
        "total":       len(logs),
        "campaign_id": campaignID,
        "limit":       limit,
        "offset":      offset,
    })
}

// GetFailedLogs returns failed operation logs
func (h *MetricsHandler) GetFailedLogs(w http.ResponseWriter, r *http.Request) {
    limit := h.getIntQuery(r, "limit", 100)
    offset := h.getIntQuery(r, "offset", 0)

    // In production, query logs with level="error" or status="failed"
    logs := []map[string]interface{}{
        {
            "timestamp":   time.Now().Add(-1 * time.Hour).Unix(),
            "level":       "error",
            "message":     "Failed to send email",
            "error":       "SMTP connection timeout",
            "campaign_id": "camp_123",
            "account_id":  "acc_456",
        },
        {
            "timestamp":  time.Now().Add(-45 * time.Minute).Unix(),
            "level":      "error",
            "message":    "Authentication failed",
            "error":      "Invalid credentials",
            "account_id": "acc_789",
        },
        {
            "timestamp":   time.Now().Add(-20 * time.Minute).Unix(),
            "level":       "error",
            "message":     "Campaign execution failed",
            "error":       "No active accounts available",
            "campaign_id": "camp_456",
        },
    }

    // Apply pagination
    start := offset
    end := offset + limit
    if start > len(logs) {
        start = len(logs)
    }
    if end > len(logs) {
        end = len(logs)
    }

    h.respondJSON(w, http.StatusOK, map[string]interface{}{
        "logs":   logs[start:end],
        "total":  len(logs),
        "limit":  limit,
        "offset": offset,
    })
}

// GetSystemLogs returns system-level logs
func (h *MetricsHandler) GetSystemLogs(w http.ResponseWriter, r *http.Request) {
    limit := h.getIntQuery(r, "limit", 100)
    offset := h.getIntQuery(r, "offset", 0)

    // In production, query system-level logs
    logs := []map[string]interface{}{
        {
            "timestamp": time.Now().Add(-2 * time.Hour).Unix(),
            "level":     "info",
            "message":   "Application started",
            "component": "server",
            "version":   "1.0.0",
        },
        {
            "timestamp": time.Now().Add(-90 * time.Minute).Unix(),
            "level":     "info",
            "message":   "Database connection established",
            "component": "database",
        },
        {
            "timestamp": time.Now().Add(-1 * time.Hour).Unix(),
            "level":     "info",
            "message":   "Worker pool initialized",
            "component": "worker",
            "workers":   10,
        },
        {
            "timestamp": time.Now().Add(-30 * time.Minute).Unix(),
            "level":     "warn",
            "message":   "High memory usage detected",
            "component": "monitor",
            "memory_mb": 1024,
        },
    }

    // Apply pagination
    start := offset
    end := offset + limit
    if start > len(logs) {
        start = len(logs)
    }
    if end > len(logs) {
        end = len(logs)
    }

    h.respondJSON(w, http.StatusOK, map[string]interface{}{
        "logs":   logs[start:end],
        "total":  len(logs),
        "limit":  limit,
        "offset": offset,
    })
}

// StreamLogs streams logs in real-time using Server-Sent Events (SSE)
func (h *MetricsHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

    flusher, ok := w.(http.Flusher)
    if !ok {
        h.logger.Error("Streaming unsupported by response writer")
        http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
        return
    }

    ctx := r.Context()
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    h.logger.Info("Client connected to log stream")

    // Send initial connection message
    fmt.Fprintf(w, "data: %s\n\n", `{"message":"Connected to log stream"}`)
    flusher.Flush()

    for {
        select {
        case <-ctx.Done():
            h.logger.Info("Client disconnected from log stream")
            return
        case <-ticker.C:
            // In production, you would get real logs from a channel/queue
            log := map[string]interface{}{
                "timestamp": time.Now().Unix(),
                "level":     "info",
                "message":   "System running normally",
                "component": "monitor",
            }
            
            data, err := json.Marshal(log)
            if err != nil {
                h.logger.Error("Failed to marshal log", logger.Error(err))
                continue
            }
            
            // SSE format: "data: <json>\n\n"
            fmt.Fprintf(w, "data: %s\n\n", data)
            flusher.Flush()
        }
    }
}
