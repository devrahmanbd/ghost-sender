package repository

import (
	"context"
	"database/sql"
	"time"
)

type StatsRepository struct {
	db *sql.DB
}

type GlobalStats struct {
	GeneratedAt          time.Time `json:"generated_at"`
	TotalCampaigns       int64     `json:"total_campaigns"`
	ActiveCampaigns      int64     `json:"active_campaigns"`
	PausedCampaigns      int64     `json:"paused_campaigns"`
	CompletedCampaigns   int64     `json:"completed_campaigns"`
	FailedCampaigns      int64     `json:"failed_campaigns"`
	TotalAccounts        int64     `json:"total_accounts"`
	ActiveAccounts       int64     `json:"active_accounts"`
	SuspendedAccounts    int64     `json:"suspended_accounts"`
	TotalRecipients      int64     `json:"total_recipients"`
	ValidRecipients      int64     `json:"valid_recipients"`
	InvalidRecipients    int64     `json:"invalid_recipients"`
	UnsubscribedRecipients int64   `json:"unsubscribed_recipients"`
	TotalProxies         int64     `json:"total_proxies"`
	ActiveProxies        int64     `json:"active_proxies"`
	HealthyProxies       int64     `json:"healthy_proxies"`
	TotalSentEmails      int64     `json:"total_sent_emails"`
	TotalFailedEmails    int64     `json:"total_failed_emails"`
	TotalOpens           int64     `json:"total_opens"`
	TotalClicks          int64     `json:"total_clicks"`
	AverageOpenRate      float64   `json:"average_open_rate"`
	AverageClickRate     float64   `json:"average_click_rate"`
	AverageDeliveryRate  float64   `json:"average_delivery_rate"`
	AverageBounceRate    float64   `json:"average_bounce_rate"`
}

type CampaignStats struct {
	CampaignID         string    `json:"campaign_id"`
	From               time.Time `json:"from"`
	To                 time.Time `json:"to"`
	TotalRecipients    int64     `json:"total_recipients"`
	Sent               int64     `json:"sent"`
	Delivered          int64     `json:"delivered"`
	Failed             int64     `json:"failed"`
	HardBounced        int64     `json:"hard_bounced"`
	SoftBounced        int64     `json:"soft_bounced"`
	Complaints         int64     `json:"complaints"`
	Unsubscribes       int64     `json:"unsubscribes"`
	Opens              int64     `json:"opens"`
	UniqueOpens        int64     `json:"unique_opens"`
	Clicks             int64     `json:"clicks"`
	UniqueClicks       int64     `json:"unique_clicks"`
	OpenRate           float64   `json:"open_rate"`
	ClickRate          float64   `json:"click_rate"`
	DeliveryRate       float64   `json:"delivery_rate"`
	BounceRate         float64   `json:"bounce_rate"`
	ComplaintRate      float64   `json:"complaint_rate"`
	UnsubscribeRate    float64   `json:"unsubscribe_rate"`
	AverageLatencyMs   float64   `json:"average_latency_ms"`
	P95LatencyMs       float64   `json:"p95_latency_ms"`
	P99LatencyMs       float64   `json:"p99_latency_ms"`
	ThroughputPerMin   float64   `json:"throughput_per_min"`
}

type AccountStats struct {
	AccountID          string    `json:"account_id"`
	From               time.Time `json:"from"`
	To                 time.Time `json:"to"`
	Provider           string    `json:"provider"`
	TotalSent          int64     `json:"total_sent"`
	Delivered          int64     `json:"delivered"`
	Failed             int64     `json:"failed"`
	HardBounced        int64     `json:"hard_bounced"`
	SoftBounced        int64     `json:"soft_bounced"`
	Complaints         int64     `json:"complaints"`
	Unsubscribes       int64     `json:"unsubscribes"`
	SpamScoreAvg       float64   `json:"spam_score_avg"`
	SpamScoreP95       float64   `json:"spam_score_p95"`
	SpamScoreP99       float64   `json:"spam_score_p99"`
	HealthScore        float64   `json:"health_score"`
	RotationIndex      float64   `json:"rotation_index"`
	DailyLimit         int64     `json:"daily_limit"`
	RemainingDaily     int64     `json:"remaining_daily"`
	AverageLatencyMs   float64   `json:"average_latency_ms"`
}

type TimeBucketStats struct {
	BucketTime         time.Time `json:"bucket_time"`
	Period             string    `json:"period"`
	Sent               int64     `json:"sent"`
	Delivered          int64     `json:"delivered"`
	Failed             int64     `json:"failed"`
	HardBounced        int64     `json:"hard_bounced"`
	SoftBounced        int64     `json:"soft_bounced"`
	Complaints         int64     `json:"complaints"`
	Unsubscribes       int64     `json:"unsubscribes"`
	Opens              int64     `json:"opens"`
	Clicks             int64     `json:"clicks"`
}

type StatsFilter struct {
	From       *time.Time
	To         *time.Time
	CampaignID string
	AccountID  string
}

func NewStatsRepository(db *sql.DB) *StatsRepository {
	return &StatsRepository{db: db}
}

func (r *StatsRepository) GetGlobalStats(ctx context.Context) (*GlobalStats, error) {
	stats := &GlobalStats{GeneratedAt: time.Now()}

	query := `
		SELECT
			COUNT(*) as total_campaigns,
			COUNT(*) FILTER (WHERE status = 'running') as active_campaigns,
			COUNT(*) FILTER (WHERE status = 'paused') as paused_campaigns,
			COUNT(*) FILTER (WHERE status = 'completed') as completed_campaigns,
			COUNT(*) FILTER (WHERE status = 'failed') as failed_campaigns
		FROM campaigns`
	if err := r.db.QueryRowContext(ctx, query).Scan(
		&stats.TotalCampaigns,
		&stats.ActiveCampaigns,
		&stats.PausedCampaigns,
		&stats.CompletedCampaigns,
		&stats.FailedCampaigns,
	); err != nil {
		return nil, err
	}

	accountQuery := `
		SELECT
			COUNT(*) as total_accounts,
			COUNT(*) FILTER (WHERE status = 'active') as active_accounts,
			COUNT(*) FILTER (WHERE status = 'suspended') as suspended_accounts
		FROM accounts`
	if err := r.db.QueryRowContext(ctx, accountQuery).Scan(
		&stats.TotalAccounts,
		&stats.ActiveAccounts,
		&stats.SuspendedAccounts,
	); err != nil {
		return nil, err
	}

	recipientQuery := `
		SELECT
			COUNT(*) as total_recipients,
			COUNT(*) FILTER (WHERE is_valid = true) as valid_recipients,
			COUNT(*) FILTER (WHERE is_valid = false) as invalid_recipients,
			COUNT(*) FILTER (WHERE unsubscribed = true) as unsubscribed_recipients
		FROM recipients`
	if err := r.db.QueryRowContext(ctx, recipientQuery).Scan(
		&stats.TotalRecipients,
		&stats.ValidRecipients,
		&stats.InvalidRecipients,
		&stats.UnsubscribedRecipients,
	); err != nil {
		return nil, err
	}

	proxyQuery := `
		SELECT
			COUNT(*) as total_proxies,
			COUNT(*) FILTER (WHERE is_active = true) as active_proxies,
			COUNT(*) FILTER (WHERE status = 'healthy') as healthy_proxies
		FROM proxies`
	if err := r.db.QueryRowContext(ctx, proxyQuery).Scan(
		&stats.TotalProxies,
		&stats.ActiveProxies,
		&stats.HealthyProxies,
	); err != nil {
		return nil, err
	}

	emailQuery := `
		SELECT
			COALESCE(SUM(sent_count), 0) as total_sent,
			COALESCE(SUM(failed_count), 0) as total_failed,
			COALESCE(SUM(opens), 0) as total_opens,
			COALESCE(SUM(clicks), 0) as total_clicks,
			COALESCE(SUM(hard_bounced::int), 0) as hard_bounces,
			COALESCE(SUM(soft_bounced::int), 0) as soft_bounces
		FROM recipients`
	var hardBounces, softBounces int64
	if err := r.db.QueryRowContext(ctx, emailQuery).Scan(
		&stats.TotalSentEmails,
		&stats.TotalFailedEmails,
		&stats.TotalOpens,
		&stats.TotalClicks,
		&hardBounces,
		&softBounces,
	); err != nil {
		return nil, err
	}

	totalDelivered := stats.TotalSentEmails - hardBounces - softBounces
	if stats.TotalSentEmails > 0 {
		stats.AverageDeliveryRate = float64(totalDelivered) / float64(stats.TotalSentEmails)
		stats.AverageBounceRate = float64(hardBounces+softBounces) / float64(stats.TotalSentEmails)
	}
	if totalDelivered > 0 {
		stats.AverageOpenRate = float64(stats.TotalOpens) / float64(totalDelivered)
		stats.AverageClickRate = float64(stats.TotalClicks) / float64(totalDelivered)
	}

	return stats, nil
}

func (r *StatsRepository) GetCampaignStats(ctx context.Context, campaignID string, from, to time.Time) (*CampaignStats, error) {
	stats := &CampaignStats{
		CampaignID: campaignID,
		From:       from,
		To:         to,
	}

	query := `
		SELECT
			COUNT(*) as total_recipients,
			COALESCE(SUM(sent::int), 0) as sent,
			COALESCE(SUM(delivered::int), 0) as delivered,
			COALESCE(SUM(failed::int), 0) as failed,
			COALESCE(SUM(hard_bounced::int), 0) as hard_bounced,
			COALESCE(SUM(soft_bounced::int), 0) as soft_bounced,
			COALESCE(SUM(complaint::int), 0) as complaints,
			COALESCE(SUM(unsubscribed::int), 0) as unsubscribes,
			COALESCE(SUM(opens), 0) as opens,
			COALESCE(SUM(unique_opens), 0) as unique_opens,
			COALESCE(SUM(clicks), 0) as clicks,
			COALESCE(SUM(unique_clicks), 0) as unique_clicks
		FROM campaign_recipient_stats
		WHERE campaign_id = $1
		  AND time >= $2 AND time <= $3`
	if err := r.db.QueryRowContext(ctx, query, campaignID, from, to).Scan(
		&stats.TotalRecipients,
		&stats.Sent,
		&stats.Delivered,
		&stats.Failed,
		&stats.HardBounced,
		&stats.SoftBounced,
		&stats.Complaints,
		&stats.Unsubscribes,
		&stats.Opens,
		&stats.UniqueOpens,
		&stats.Clicks,
		&stats.UniqueClicks,
	); err != nil {
		return nil, err
	}

	total := stats.Sent
	if total > 0 {
		stats.DeliveryRate = float64(stats.Delivered) / float64(total)
		stats.BounceRate = float64(stats.HardBounced+stats.SoftBounced) / float64(total)
		stats.ComplaintRate = float64(stats.Complaints) / float64(total)
		stats.UnsubscribeRate = float64(stats.Unsubscribes) / float64(total)
	}

	if stats.Delivered > 0 {
		stats.OpenRate = float64(stats.UniqueOpens) / float64(stats.Delivered)
		stats.ClickRate = float64(stats.UniqueClicks) / float64(stats.Delivered)
	}

	latencyQuery := `
		SELECT
			COALESCE(AVG(latency_ms), 0),
			COALESCE(percentile_disc(0.95) WITHIN GROUP (ORDER BY latency_ms), 0),
			COALESCE(percentile_disc(0.99) WITHIN GROUP (ORDER BY latency_ms), 0)
		FROM campaign_delivery_metrics
		WHERE campaign_id = $1
		  AND created_at >= $2 AND created_at <= $3
		  AND latency_ms > 0`
	if err := r.db.QueryRowContext(ctx, latencyQuery, campaignID, from, to).Scan(
		&stats.AverageLatencyMs,
		&stats.P95LatencyMs,
		&stats.P99LatencyMs,
	); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	throughputQuery := `
		SELECT
			COALESCE(MAX(sent_per_minute), 0)
		FROM campaign_delivery_metrics
		WHERE campaign_id = $1
		  AND created_at >= $2 AND created_at <= $3`
	if err := r.db.QueryRowContext(ctx, throughputQuery, campaignID, from, to).Scan(
		&stats.ThroughputPerMin,
	); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	return stats, nil
}

func (r *StatsRepository) GetAccountStats(ctx context.Context, accountID string, from, to time.Time) (*AccountStats, error) {
	stats := &AccountStats{
		AccountID: accountID,
		From:      from,
		To:        to,
	}

	query := `
		SELECT
			provider,
			COALESCE(SUM(sent::int), 0) as total_sent,
			COALESCE(SUM(delivered::int), 0) as delivered,
			COALESCE(SUM(failed::int), 0) as failed,
			COALESCE(SUM(hard_bounced::int), 0) as hard_bounced,
			COALESCE(SUM(soft_bounced::int), 0) as soft_bounced,
			COALESCE(SUM(complaint::int), 0) as complaints,
			COALESCE(SUM(unsubscribed::int), 0) as unsubscribes,
			COALESCE(AVG(spam_score), 0) as spam_score_avg,
			COALESCE(percentile_disc(0.95) WITHIN GROUP (ORDER BY spam_score), 0) as spam_score_p95,
			COALESCE(percentile_disc(0.99) WITHIN GROUP (ORDER BY spam_score), 0) as spam_score_p99,
			COALESCE(AVG(health_score), 0) as health_score,
			COALESCE(AVG(latency_ms), 0) as avg_latency_ms
		FROM account_delivery_stats
		WHERE account_id = $1
		  AND created_at >= $2 AND created_at <= $3
		GROUP BY provider`
	if err := r.db.QueryRowContext(ctx, query, accountID, from, to).Scan(
		&stats.Provider,
		&stats.TotalSent,
		&stats.Delivered,
		&stats.Failed,
		&stats.HardBounced,
		&stats.SoftBounced,
		&stats.Complaints,
		&stats.Unsubscribes,
		&stats.SpamScoreAvg,
		&stats.SpamScoreP95,
		&stats.SpamScoreP99,
		&stats.HealthScore,
		&stats.AverageLatencyMs,
	); err != nil {
		return nil, err
	}

	limitQuery := `
		SELECT daily_limit, remaining_daily
		FROM account_limits
		WHERE account_id = $1`
	if err := r.db.QueryRowContext(ctx, limitQuery, accountID).Scan(
		&stats.DailyLimit,
		&stats.RemainingDaily,
	); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	rotationQuery := `
		SELECT
			COALESCE(AVG(rotation_index), 0)
		FROM account_rotation_metrics
		WHERE account_id = $1
		  AND created_at >= $2 AND created_at <= $3`
	if err := r.db.QueryRowContext(ctx, rotationQuery, accountID, from, to).Scan(
		&stats.RotationIndex,
	); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	return stats, nil
}

func (r *StatsRepository) GetTimeSeriesStats(ctx context.Context, filter StatsFilter, bucket string) ([]*TimeBucketStats, error) {
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	if filter.From != nil {
		from = *filter.From
	}
	if filter.To != nil {
		to = *filter.To
	}

	trunc := "minute"
	switch bucket {
	case "hour":
		trunc = "hour"
	case "day":
		trunc = "day"
	case "week":
		trunc = "week"
	}

	query := `
		SELECT
			date_trunc($1, created_at) as bucket,
			COALESCE(SUM(sent::int), 0) as sent,
			COALESCE(SUM(delivered::int), 0) as delivered,
			COALESCE(SUM(failed::int), 0) as failed,
			COALESCE(SUM(hard_bounced::int), 0) as hard_bounced,
			COALESCE(SUM(soft_bounced::int), 0) as soft_bounced,
			COALESCE(SUM(complaint::int), 0) as complaints,
			COALESCE(SUM(unsubscribed::int), 0) as unsubscribes,
			COALESCE(SUM(opens), 0) as opens,
			COALESCE(SUM(clicks), 0) as clicks
		FROM delivery_timeseries
		WHERE created_at >= $2 AND created_at <= $3`

	args := []interface{}{trunc, from, to}
	argPos := 4

	if filter.CampaignID != "" {
		query += " AND campaign_id = $" + string('0'+argPos)
		args = append(args, filter.CampaignID)
		argPos++
	}
	if filter.AccountID != "" {
		query += " AND account_id = $" + string('0'+argPos)
		args = append(args, filter.AccountID)
		argPos++
	}

	query += `
		GROUP BY bucket
		ORDER BY bucket ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []*TimeBucketStats
	for rows.Next() {
		b := &TimeBucketStats{Period: trunc}
		if err := rows.Scan(
			&b.BucketTime,
			&b.Sent,
			&b.Delivered,
			&b.Failed,
			&b.HardBounced,
			&b.SoftBounced,
			&b.Complaints,
			&b.Unsubscribes,
			&b.Opens,
			&b.Clicks,
		); err != nil {
			return nil, err
		}
		buckets = append(buckets, b)
	}

	return buckets, nil
}
