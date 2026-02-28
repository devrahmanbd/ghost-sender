package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type LogRepository struct {
	db *sql.DB
}

type LogLevel string

const (
	LogLevelDebug   LogLevel = "DEBUG"
	LogLevelInfo    LogLevel = "INFO"
	LogLevelWarn    LogLevel = "WARN"
	LogLevelError   LogLevel = "ERROR"
	LogLevelFatal   LogLevel = "FATAL"
	LogLevelMetric  LogLevel = "METRIC"
	LogLevelAudit   LogLevel = "AUDIT"
	LogLevelSystem  LogLevel = "SYSTEM"
	LogLevelDeliver LogLevel = "DELIVERABILITY"
)

type LogCategory string

const (
	LogCategoryCampaign   LogCategory = "campaign"
	LogCategoryAccount    LogCategory = "account"
	LogCategoryTemplate   LogCategory = "template"
	LogCategoryRecipient  LogCategory = "recipient"
	LogCategoryProxy      LogCategory = "proxy"
	LogCategorySender     LogCategory = "sender"
	LogCategorySystem     LogCategory = "system"
	LogCategoryWebhook    LogCategory = "webhook"
	LogCategoryNotification LogCategory = "notification"
	LogCategorySecurity   LogCategory = "security"
	LogCategoryPerformance LogCategory = "performance"
	LogCategoryMetrics    LogCategory = "metrics"
	LogCategoryAudit      LogCategory = "audit"
)

type LogEntry struct {
	ID            string                 `json:"id" db:"id"`
	Time          time.Time              `json:"time" db:"time"`
	Level         LogLevel               `json:"level" db:"level"`
	Category      LogCategory            `json:"category" db:"category"`
	SessionID     string                 `json:"session_id" db:"session_id"`
	CampaignID    string                 `json:"campaign_id" db:"campaign_id"`
	AccountID     string                 `json:"account_id" db:"account_id"`
	RecipientID   string                 `json:"recipient_id" db:"recipient_id"`
	ProxyID       string                 `json:"proxy_id" db:"proxy_id"`
	TemplateID    string                 `json:"template_id" db:"template_id"`
	Message       string                 `json:"message" db:"message"`
	Details       map[string]interface{} `json:"details" db:"details"`
	ErrorCode     string                 `json:"error_code" db:"error_code"`
	ErrorClass    string                 `json:"error_class" db:"error_class"`
	StackTrace    string                 `json:"stack_trace" db:"stack_trace"`
	RequestID     string                 `json:"request_id" db:"request_id"`
	TraceID       string                 `json:"trace_id" db:"trace_id"`
	SpanID        string                 `json:"span_id" db:"span_id"`
	HTTPMethod    string                 `json:"http_method" db:"http_method"`
	HTTPPath      string                 `json:"http_path" db:"http_path"`
	HTTPStatus    int                    `json:"http_status" db:"http_status"`
	DurationMs    int64                  `json:"duration_ms" db:"duration_ms"`
	ClientIP      string                 `json:"client_ip" db:"client_ip"`
	UserAgent     string                 `json:"user_agent" db:"user_agent"`
	NodeID        string                 `json:"node_id" db:"node_id"`
	Hostname      string                 `json:"hostname" db:"hostname"`
	Environment   string                 `json:"environment" db:"environment"`
	Shard         string                 `json:"shard" db:"shard"`
	MetricName    string                 `json:"metric_name" db:"metric_name"`
	MetricValue   float64                `json:"metric_value" db:"metric_value"`
	MetricUnit    string                 `json:"metric_unit" db:"metric_unit"`
	MetricLabels  map[string]string      `json:"metric_labels" db:"metric_labels"`
	UserID        string                 `json:"user_id" db:"user_id"`
	Username      string                 `json:"username" db:"username"`
	TenantID      string                 `json:"tenant_id" db:"tenant_id"`
	Source        string                 `json:"source" db:"source"`
	Subsystem     string                 `json:"subsystem" db:"subsystem"`
	Component     string                 `json:"component" db:"component"`
	Version       string                 `json:"version" db:"version"`
	CorrelationID string                 `json:"correlation_id" db:"correlation_id"`
	Archived      bool                   `json:"archived" db:"archived"`
	CreatedAt     time.Time              `json:"created_at" db:"created_at"`
}

type LogFilter struct {
	Levels        []LogLevel
	Categories    []LogCategory
	SessionIDs    []string
	CampaignIDs   []string
	AccountIDs    []string
	RecipientIDs  []string
	ProxyIDs      []string
	TemplateIDs   []string
	UserIDs       []string
	TenantIDs     []string
	ErrorCodes    []string
	MinTime       *time.Time
	MaxTime       *time.Time
	Search        string
	Source        string
	Subsystem     string
	Component     string
	MinDurationMs *int64
	MaxDurationMs *int64
	HTTPMethods   []string
	HTTPStatuses  []int
	IncludeArchived bool
	SortBy        string
	SortOrder     string
	Limit         int
	Offset        int
}

type LogStats struct {
	From                     time.Time          `json:"from"`
	To                       time.Time          `json:"to"`
	TotalLogs                int64              `json:"total_logs"`
	DebugCount               int64              `json:"debug_count"`
	InfoCount                int64              `json:"info_count"`
	WarnCount                int64              `json:"warn_count"`
	ErrorCount               int64              `json:"error_count"`
	FatalCount               int64              `json:"fatal_count"`
	SystemCount              int64              `json:"system_count"`
	DeliverabilityCount      int64              `json:"deliverability_count"`
	CampaignLogCount         int64              `json:"campaign_log_count"`
	AccountLogCount          int64              `json:"account_log_count"`
	RecipientLogCount        int64              `json:"recipient_log_count"`
	ProxyLogCount            int64              `json:"proxy_log_count"`
	TemplateLogCount         int64              `json:"template_log_count"`
	MaxErrorBurstCount       int64              `json:"max_error_burst_count"`
	AverageRequestDurationMs float64            `json:"average_request_duration_ms"`
	P95RequestDurationMs     float64            `json:"p95_request_duration_ms"`
	P99RequestDurationMs     float64            `json:"p99_request_duration_ms"`
	TopErrorCodes            map[string]int64   `json:"top_error_codes"`
	TopHTTPStatus            map[int]int64      `json:"top_http_status"`
	TopCampaignsByErrors     map[string]int64   `json:"top_campaigns_by_errors"`
	TopAccountsByErrors      map[string]int64   `json:"top_accounts_by_errors"`
	TopRecipientsByFailures  map[string]int64   `json:"top_recipients_by_failures"`
	TopProxiesByErrors       map[string]int64   `json:"top_proxies_by_errors"`
}

func NewLogRepository(db *sql.DB) *LogRepository {
	return &LogRepository{db: db}
}

func (r *LogRepository) Insert(ctx context.Context, entry *LogEntry) error {
	query := `
		INSERT INTO logs (
			id, time, level, category, session_id, campaign_id, account_id,
			recipient_id, proxy_id, template_id, message, details,
			error_code, error_class, stack_trace, request_id, trace_id,
			span_id, http_method, http_path, http_status, duration_ms,
			client_ip, user_agent, node_id, hostname, environment,
			shard, metric_name, metric_value, metric_unit, metric_labels,
			user_id, username, tenant_id, source, subsystem, component,
			version, correlation_id, archived, created_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,
			$8,$9,$10,$11,$12,
			$13,$14,$15,$16,$17,
			$18,$19,$20,$21,$22,
			$23,$24,$25,$26,$27,
			$28,$29,$30,$31,$32,
			$33,$34,$35,$36,$37,$38,
			$39,$40,$41,$42
		)`

	detailsJSON, _ := json.Marshal(entry.Details)
	labelsJSON, _ := json.Marshal(entry.MetricLabels)

	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	_, err := r.db.ExecContext(
		ctx, query,
		entry.ID, entry.Time, entry.Level, entry.Category, entry.SessionID, entry.CampaignID, entry.AccountID,
		entry.RecipientID, entry.ProxyID, entry.TemplateID, entry.Message, detailsJSON,
		entry.ErrorCode, entry.ErrorClass, entry.StackTrace, entry.RequestID, entry.TraceID,
		entry.SpanID, entry.HTTPMethod, entry.HTTPPath, entry.HTTPStatus, entry.DurationMs,
		entry.ClientIP, entry.UserAgent, entry.NodeID, entry.Hostname, entry.Environment,
		entry.Shard, entry.MetricName, entry.MetricValue, entry.MetricUnit, labelsJSON,
		entry.UserID, entry.Username, entry.TenantID, entry.Source, entry.Subsystem, entry.Component,
		entry.Version, entry.CorrelationID, entry.Archived, entry.CreatedAt,
	)

	return err
}

func (r *LogRepository) InsertBatch(ctx context.Context, entries []*LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	baseQuery := `
		INSERT INTO logs (
			id, time, level, category, session_id, campaign_id, account_id,
			recipient_id, proxy_id, template_id, message, details,
			error_code, error_class, stack_trace, request_id, trace_id,
			span_id, http_method, http_path, http_status, duration_ms,
			client_ip, user_agent, node_id, hostname, environment,
			shard, metric_name, metric_value, metric_unit, metric_labels,
			user_id, username, tenant_id, source, subsystem, component,
			version, correlation_id, archived, created_at
		) VALUES `

	args := []interface{}{}
	valueStrings := []string{}
	argPos := 1

	for _, entry := range entries {
		if entry.Time.IsZero() {
			entry.Time = time.Now()
		}
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = time.Now()
		}

		detailsJSON, _ := json.Marshal(entry.Details)
		labelsJSON, _ := json.Marshal(entry.MetricLabels)

		valueStrings = append(valueStrings, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			argPos, argPos+1, argPos+2, argPos+3, argPos+4, argPos+5, argPos+6,
			argPos+7, argPos+8, argPos+9, argPos+10, argPos+11, argPos+12,
			argPos+13, argPos+14, argPos+15, argPos+16, argPos+17, argPos+18,
			argPos+19, argPos+20, argPos+21, argPos+22, argPos+23, argPos+24,
			argPos+25, argPos+26, argPos+27, argPos+28, argPos+29, argPos+30,
			argPos+31, argPos+32, argPos+33, argPos+34, argPos+35, argPos+36,
			argPos+37, argPos+38, argPos+39, argPos+40, argPos+41, argPos+42,
		))

		args = append(args,
			entry.ID, entry.Time, entry.Level, entry.Category, entry.SessionID, entry.CampaignID, entry.AccountID,
			entry.RecipientID, entry.ProxyID, entry.TemplateID, entry.Message, detailsJSON,
			entry.ErrorCode, entry.ErrorClass, entry.StackTrace, entry.RequestID, entry.TraceID,
			entry.SpanID, entry.HTTPMethod, entry.HTTPPath, entry.HTTPStatus, entry.DurationMs,
			entry.ClientIP, entry.UserAgent, entry.NodeID, entry.Hostname, entry.Environment,
			entry.Shard, entry.MetricName, entry.MetricValue, entry.MetricUnit, labelsJSON,
			entry.UserID, entry.Username, entry.TenantID, entry.Source, entry.Subsystem, entry.Component,
			entry.Version, entry.CorrelationID, entry.Archived, entry.CreatedAt,
		)

		argPos += 42
	}

	query := baseQuery + strings.Join(valueStrings, ",")

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *LogRepository) GetByID(ctx context.Context, id string) (*LogEntry, error) {
	query := `
		SELECT
			id, time, level, category, session_id, campaign_id, account_id,
			recipient_id, proxy_id, template_id, message, details,
			error_code, error_class, stack_trace, request_id, trace_id,
			span_id, http_method, http_path, http_status, duration_ms,
			client_ip, user_agent, node_id, hostname, environment,
			shard, metric_name, metric_value, metric_unit, metric_labels,
			user_id, username, tenant_id, source, subsystem, component,
			version, correlation_id, archived, created_at
		FROM logs
		WHERE id = $1`

	entry := &LogEntry{}
	var detailsJSON []byte
	var labelsJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&entry.ID, &entry.Time, &entry.Level, &entry.Category, &entry.SessionID, &entry.CampaignID, &entry.AccountID,
		&entry.RecipientID, &entry.ProxyID, &entry.TemplateID, &entry.Message, &detailsJSON,
		&entry.ErrorCode, &entry.ErrorClass, &entry.StackTrace, &entry.RequestID, &entry.TraceID,
		&entry.SpanID, &entry.HTTPMethod, &entry.HTTPPath, &entry.HTTPStatus, &entry.DurationMs,
		&entry.ClientIP, &entry.UserAgent, &entry.NodeID, &entry.Hostname, &entry.Environment,
		&entry.Shard, &entry.MetricName, &entry.MetricValue, &entry.MetricUnit, &labelsJSON,
		&entry.UserID, &entry.Username, &entry.TenantID, &entry.Source, &entry.Subsystem, &entry.Component,
		&entry.Version, &entry.CorrelationID, &entry.Archived, &entry.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("log entry not found")
		}
		return nil, err
	}

	if len(detailsJSON) > 0 {
		json.Unmarshal(detailsJSON, &entry.Details)
	}
	if len(labelsJSON) > 0 {
		json.Unmarshal(labelsJSON, &entry.MetricLabels)
	}

	return entry, nil
}

func (r *LogRepository) List(ctx context.Context, filter *LogFilter) ([]*LogEntry, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if len(filter.Levels) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("level = ANY($%d)", argPos))
		levels := make([]string, len(filter.Levels))
		for i, l := range filter.Levels {
			levels[i] = string(l)
		}
		args = append(args, pq.Array(levels))
		argPos++
	}

	if len(filter.Categories) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("category = ANY($%d)", argPos))
		cats := make([]string, len(filter.Categories))
		for i, c := range filter.Categories {
			cats[i] = string(c)
		}
		args = append(args, pq.Array(cats))
		argPos++
	}

	if len(filter.SessionIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("session_id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.SessionIDs))
		argPos++
	}

	if len(filter.CampaignIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("campaign_id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.CampaignIDs))
		argPos++
	}

	if len(filter.AccountIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("account_id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.AccountIDs))
		argPos++
	}

	if len(filter.RecipientIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("recipient_id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.RecipientIDs))
		argPos++
	}

	if len(filter.ProxyIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("proxy_id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.ProxyIDs))
		argPos++
	}

	if len(filter.TemplateIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("template_id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.TemplateIDs))
		argPos++
	}

	if len(filter.UserIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("user_id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.UserIDs))
		argPos++
	}

	if len(filter.TenantIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("tenant_id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.TenantIDs))
		argPos++
	}

	if len(filter.ErrorCodes) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("error_code = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.ErrorCodes))
		argPos++
	}

	if filter.MinTime != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("time >= $%d", argPos))
		args = append(args, *filter.MinTime)
		argPos++
	}

	if filter.MaxTime != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("time <= $%d", argPos))
		args = append(args, *filter.MaxTime)
		argPos++
	}

	if filter.Source != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("source = $%d", argPos))
		args = append(args, filter.Source)
		argPos++
	}

	if filter.Subsystem != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("subsystem = $%d", argPos))
		args = append(args, filter.Subsystem)
		argPos++
	}

	if filter.Component != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("component = $%d", argPos))
		args = append(args, filter.Component)
		argPos++
	}

	if filter.MinDurationMs != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("duration_ms >= $%d", argPos))
		args = append(args, *filter.MinDurationMs)
		argPos++
	}

	if filter.MaxDurationMs != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("duration_ms <= $%d", argPos))
		args = append(args, *filter.MaxDurationMs)
		argPos++
	}

	if len(filter.HTTPMethods) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("http_method = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.HTTPMethods))
		argPos++
	}

	if len(filter.HTTPStatuses) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("http_status = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.HTTPStatuses))
		argPos++
	}

	if !filter.IncludeArchived {
		whereClauses = append(whereClauses, "archived = false")
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(message ILIKE $%d OR error_code ILIKE $%d OR error_class ILIKE $%d)", argPos, argPos, argPos))
		args = append(args, "%"+filter.Search+"%")
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM logs WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortBy := "time"
	if filter.SortBy != "" {
		sortBy = filter.SortBy
	}
	sortOrder := "DESC"
	if filter.SortOrder != "" {
		sortOrder = strings.ToUpper(filter.SortOrder)
	}

	limit := 100
	if filter.Limit > 0 {
		limit = filter.Limit
	}
	offset := 0
	if filter.Offset > 0 {
		offset = filter.Offset
	}

	query := fmt.Sprintf(`
		SELECT
			id, time, level, category, session_id, campaign_id, account_id,
			recipient_id, proxy_id, template_id, message, details,
			error_code, error_class, stack_trace, request_id, trace_id,
			span_id, http_method, http_path, http_status, duration_ms,
			client_ip, user_agent, node_id, hostname, environment,
			shard, metric_name, metric_value, metric_unit, metric_labels,
			user_id, username, tenant_id, source, subsystem, component,
			version, correlation_id, archived, created_at
		FROM logs
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

	var entries []*LogEntry
	for rows.Next() {
		entry := &LogEntry{}
		var detailsJSON []byte
		var labelsJSON []byte

		err := rows.Scan(
			&entry.ID, &entry.Time, &entry.Level, &entry.Category, &entry.SessionID, &entry.CampaignID, &entry.AccountID,
			&entry.RecipientID, &entry.ProxyID, &entry.TemplateID, &entry.Message, &detailsJSON,
			&entry.ErrorCode, &entry.ErrorClass, &entry.StackTrace, &entry.RequestID, &entry.TraceID,
			&entry.SpanID, &entry.HTTPMethod, &entry.HTTPPath, &entry.HTTPStatus, &entry.DurationMs,
			&entry.ClientIP, &entry.UserAgent, &entry.NodeID, &entry.Hostname, &entry.Environment,
			&entry.Shard, &entry.MetricName, &entry.MetricValue, &entry.MetricUnit, &labelsJSON,
			&entry.UserID, &entry.Username, &entry.TenantID, &entry.Source, &entry.Subsystem, &entry.Component,
			&entry.Version, &entry.CorrelationID, &entry.Archived, &entry.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}

		if len(detailsJSON) > 0 {
			json.Unmarshal(detailsJSON, &entry.Details)
		}
		if len(labelsJSON) > 0 {
			json.Unmarshal(labelsJSON, &entry.MetricLabels)
		}

		entries = append(entries, entry)
	}

	return entries, total, nil
}

func (r *LogRepository) ArchiveOlderThan(ctx context.Context, t time.Time) (int64, error) {
	query := `
		UPDATE logs
		SET archived = true
		WHERE time < $1
			AND archived = false`

	result, err := r.db.ExecContext(ctx, query, t)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (r *LogRepository) DeleteArchivedOlderThan(ctx context.Context, t time.Time, batchSize int) (int64, error) {
	query := `
		DELETE FROM logs
		WHERE id IN (
			SELECT id
			FROM logs
			WHERE archived = true
				AND time < $1
			ORDER BY time
			LIMIT $2
		)`

	result, err := r.db.ExecContext(ctx, query, t, batchSize)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (r *LogRepository) GetStats(ctx context.Context, from, to time.Time) (*LogStats, error) {
	stats := &LogStats{
		From:                 from,
		To:                   to,
		TopErrorCodes:        make(map[string]int64),
		TopHTTPStatus:        make(map[int]int64),
		TopCampaignsByErrors: make(map[string]int64),
		TopAccountsByErrors:  make(map[string]int64),
		TopRecipientsByFailures: make(map[string]int64),
		TopProxiesByErrors:   make(map[string]int64),
	}

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE level = 'DEBUG') as debug_count,
			COUNT(*) FILTER (WHERE level = 'INFO') as info_count,
			COUNT(*) FILTER (WHERE level = 'WARN') as warn_count,
			COUNT(*) FILTER (WHERE level = 'ERROR') as error_count,
			COUNT(*) FILTER (WHERE level = 'FATAL') as fatal_count,
			COUNT(*) FILTER (WHERE category = 'system') as system_count,
			COUNT(*) FILTER (WHERE category = 'deliverability') as deliverability_count,
			COUNT(*) FILTER (WHERE category = 'campaign') as campaign_log_count,
			COUNT(*) FILTER (WHERE category = 'account') as account_log_count,
			COUNT(*) FILTER (WHERE category = 'recipient') as recipient_log_count,
			COUNT(*) FILTER (WHERE category = 'proxy') as proxy_log_count,
			COUNT(*) FILTER (WHERE category = 'template') as template_log_count,
			COALESCE(AVG(duration_ms) FILTER (WHERE duration_ms > 0), 0) as avg_duration_ms
		FROM logs
		WHERE time >= $1 AND time <= $2`

	err := r.db.QueryRowContext(ctx, query, from, to).Scan(
		&stats.TotalLogs,
		&stats.DebugCount,
		&stats.InfoCount,
		&stats.WarnCount,
		&stats.ErrorCount,
		&stats.FatalCount,
		&stats.SystemCount,
		&stats.DeliverabilityCount,
		&stats.CampaignLogCount,
		&stats.AccountLogCount,
		&stats.RecipientLogCount,
		&stats.ProxyLogCount,
		&stats.TemplateLogCount,
		&stats.AverageRequestDurationMs,
	)
	if err != nil {
		return nil, err
	}

	pctQuery := `
		SELECT
			percentile_disc(0.95) WITHIN GROUP (ORDER BY duration_ms) as p95,
			percentile_disc(0.99) WITHIN GROUP (ORDER BY duration_ms) as p99
		FROM logs
		WHERE time >= $1 AND time <= $2 AND duration_ms > 0`

	var p95, p99 sql.NullFloat64
	if err := r.db.QueryRowContext(ctx, pctQuery, from, to).Scan(&p95, &p99); err == nil {
		if p95.Valid {
			stats.P95RequestDurationMs = p95.Float64
		}
		if p99.Valid {
			stats.P99RequestDurationMs = p99.Float64
		}
	}

	errorCodeQuery := `
		SELECT error_code, COUNT(*)
		FROM logs
		WHERE time >= $1 AND time <= $2 AND error_code <> ''
		GROUP BY error_code
		ORDER BY COUNT(*) DESC
		LIMIT 50`

	rows, err := r.db.QueryContext(ctx, errorCodeQuery, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var code string
			var count int64
			if err := rows.Scan(&code, &count); err == nil {
				stats.TopErrorCodes[code] = count
			}
		}
	}

	statusQuery := `
		SELECT http_status, COUNT(*)
		FROM logs
		WHERE time >= $1 AND time <= $2 AND http_status > 0
		GROUP BY http_status
		ORDER BY COUNT(*) DESC
		LIMIT 50`

	statusRows, err := r.db.QueryContext(ctx, statusQuery, from, to)
	if err == nil {
		defer statusRows.Close()
		for statusRows.Next() {
			var status int
			var count int64
			if err := statusRows.Scan(&status, &count); err == nil {
				stats.TopHTTPStatus[status] = count
			}
		}
	}

	campaignErrorQuery := `
		SELECT campaign_id, COUNT(*)
		FROM logs
		WHERE time >= $1 AND time <= $2
			AND level IN ('ERROR','FATAL')
			AND campaign_id <> ''
		GROUP BY campaign_id
		ORDER BY COUNT(*) DESC
		LIMIT 50`

	cRows, err := r.db.QueryContext(ctx, campaignErrorQuery, from, to)
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var id string
			var count int64
			if err := cRows.Scan(&id, &count); err == nil {
				stats.TopCampaignsByErrors[id] = count
			}
		}
	}

	accountErrorQuery := `
		SELECT account_id, COUNT(*)
		FROM logs
		WHERE time >= $1 AND time <= $2
			AND level IN ('ERROR','FATAL')
			AND account_id <> ''
		GROUP BY account_id
		ORDER BY COUNT(*) DESC
		LIMIT 50`

	aRows, err := r.db.QueryContext(ctx, accountErrorQuery, from, to)
	if err == nil {
		defer aRows.Close()
		for aRows.Next() {
			var id string
			var count int64
			if err := aRows.Scan(&id, &count); err == nil {
				stats.TopAccountsByErrors[id] = count
			}
		}
	}

	recipientFailureQuery := `
		SELECT recipient_id, COUNT(*)
		FROM logs
		WHERE time >= $1 AND time <= $2
			AND category = 'recipient'
			AND level IN ('ERROR','WARN')
			AND recipient_id <> ''
		GROUP BY recipient_id
		ORDER BY COUNT(*) DESC
		LIMIT 50`

	rRows, err := r.db.QueryContext(ctx, recipientFailureQuery, from, to)
	if err == nil {
		defer rRows.Close()
		for rRows.Next() {
			var id string
			var count int64
			if err := rRows.Scan(&id, &count); err == nil {
				stats.TopRecipientsByFailures[id] = count
			}
		}
	}

	proxyErrorQuery := `
		SELECT proxy_id, COUNT(*)
		FROM logs
		WHERE time >= $1 AND time <= $2
			AND category = 'proxy'
			AND level IN ('ERROR','WARN')
			AND proxy_id <> ''
		GROUP BY proxy_id
		ORDER BY COUNT(*) DESC
		LIMIT 50`

	pRows, err := r.db.QueryContext(ctx, proxyErrorQuery, from, to)
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var id string
			var count int64
			if err := pRows.Scan(&id, &count); err == nil {
				stats.TopProxiesByErrors[id] = count
			}
		}
	}

	burstQuery := `
		SELECT COALESCE(MAX(cnt), 0)
		FROM (
			SELECT date_trunc('minute', time) AS bucket, COUNT(*) AS cnt
			FROM logs
			WHERE time >= $1 AND time <= $2 AND level IN ('ERROR','FATAL')
			GROUP BY bucket
		) s`

	if err := r.db.QueryRowContext(ctx, burstQuery, from, to).Scan(&stats.MaxErrorBurstCount); err != nil {
		return nil, err
	}

	return stats, nil
}

func (r *LogRepository) DeleteBySession(ctx context.Context, sessionID string) (int64, error) {
	query := `
		DELETE FROM logs
		WHERE session_id = $1`

	result, err := r.db.ExecContext(ctx, query, sessionID)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (r *LogRepository) DeleteByCampaign(ctx context.Context, campaignID string) (int64, error) {
	query := `
		DELETE FROM logs
		WHERE campaign_id = $1`

	result, err := r.db.ExecContext(ctx, query, campaignID)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (r *LogRepository) DeleteByTenant(ctx context.Context, tenantID string, olderThan *time.Time) (int64, error) {
	where := []string{"tenant_id = $1"}
	args := []interface{}{tenantID}
	argPos := 2

	if olderThan != nil {
		where = append(where, fmt.Sprintf("time < $%d", argPos))
		args = append(args, *olderThan)
		argPos++
	}

	query := fmt.Sprintf(`
		DELETE FROM logs
		WHERE %s`, strings.Join(where, " AND "))

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}
