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

type CampaignRepository struct {
	db *sql.DB
}

type Campaign struct {
	ID              string                 `json:"id" db:"id"`
	Name            string                 `json:"name" db:"name"`
	Description     string                 `json:"description" db:"description"`
	Status          string                 `json:"status" db:"status"`
	State           string                 `json:"state" db:"state"`
	Priority        int                    `json:"priority" db:"priority"`
	ScheduledAt     *time.Time             `json:"scheduled_at" db:"scheduled_at"`
	StartedAt       *time.Time             `json:"started_at" db:"started_at"`
	CompletedAt     *time.Time             `json:"completed_at" db:"completed_at"`
	PausedAt        *time.Time             `json:"paused_at" db:"paused_at"`
	FailedAt        *time.Time             `json:"failed_at" db:"failed_at"`
	TotalRecipients int                    `json:"total_recipients" db:"total_recipients"`
	SentCount       int                    `json:"sent_count" db:"sent_count"`
	FailedCount     int                    `json:"failed_count" db:"failed_count"`
	PendingCount    int                    `json:"pending_count" db:"pending_count"`
	SuccessRate     float64                `json:"success_rate" db:"success_rate"`
	Progress        float64                `json:"progress" db:"progress"`
	Throughput      float64                `json:"throughput" db:"throughput"`
	EstimatedETA    *time.Time             `json:"estimated_eta" db:"estimated_eta"`
	Config          map[string]interface{} `json:"config" db:"config"`
	TemplateIDs     []string               `json:"template_ids" db:"template_ids"`
	AccountIDs      []string               `json:"account_ids" db:"account_ids"`
	RecipientListID *string                 `json:"recipient_list_id" db:"recipient_list_id"`
	ProxyIDs        []string               `json:"proxy_ids" db:"proxy_ids"`
	Tags            []string               `json:"tags" db:"tags"`
	Metadata        map[string]interface{} `json:"metadata" db:"metadata"`
	ErrorMessage    string                 `json:"error_message" db:"error_message"`
	LastCheckpoint  *time.Time             `json:"last_checkpoint" db:"last_checkpoint"`
	CheckpointData  map[string]interface{} `json:"checkpoint_data" db:"checkpoint_data"`
	RetryCount      int                    `json:"retry_count" db:"retry_count"`
	MaxRetries      int                    `json:"max_retries" db:"max_retries"`
	CreatedAt       time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at" db:"updated_at"`
	CreatedBy       string                 `json:"created_by" db:"created_by"`
	UpdatedBy       string                 `json:"updated_by" db:"updated_by"`
	IsArchived      bool                   `json:"is_archived" db:"is_archived"`
	ArchivedAt      *time.Time             `json:"archived_at" db:"archived_at"`
}

type CampaignFilter struct {
	IDs              []string
	Status           []string
	State            []string
	Tags             []string
	CreatedAfter     *time.Time
	CreatedBefore    *time.Time
	ScheduledAfter   *time.Time
	ScheduledBefore  *time.Time
	MinSuccessRate   *float64
	MaxSuccessRate   *float64
	IsArchived       *bool
	Search           string
	SortBy           string
	SortOrder        string
	Limit            int
	Offset           int
	IncludeArchived  bool
	RecipientListID  string
	CreatedBy        string
}

type CampaignStatsResult struct {
	TotalCampaigns     int                    `json:"total_campaigns"`
	ActiveCampaigns    int                    `json:"active_campaigns"`
	CompletedCampaigns int                    `json:"completed_campaigns"`
	FailedCampaigns    int                    `json:"failed_campaigns"`
	PausedCampaigns    int                    `json:"paused_campaigns"`
	ScheduledCampaigns int                    `json:"scheduled_campaigns"`
	TotalSent          int64                  `json:"total_sent"`
	TotalFailed        int64                  `json:"total_failed"`
	AverageSuccessRate float64                `json:"average_success_rate"`
	AverageThroughput  float64                `json:"average_throughput"`
	StatusBreakdown    map[string]int         `json:"status_breakdown"`
	StateBreakdown     map[string]int         `json:"state_breakdown"`
	RecentActivity     []*CampaignActivity    `json:"recent_activity"`
}

type CampaignActivity struct {
	CampaignID   string                 `json:"campaign_id"`
	CampaignName string                 `json:"campaign_name"`
	ActivityType string                 `json:"activity_type"`
	Details      map[string]interface{} `json:"details"`
	Timestamp    time.Time              `json:"timestamp"`
}

// ============================================================================
// COUNT METHODS FOR METRICS
// ============================================================================

// Count returns the total number of campaigns matching the filter
func (r *CampaignRepository) Count(ctx context.Context, filter *CampaignFilter) (int, error) {
    whereClauses := []string{}
    args := []interface{}{}
    argPos := 1

    // Default: exclude archived
    if filter == nil || !filter.IncludeArchived {
        whereClauses = append(whereClauses, "is_archived = false")
    }

    if filter != nil {
        if len(filter.IDs) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.IDs))
            argPos++
        }

        if len(filter.Status) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("status = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.Status))
            argPos++
        }

        if len(filter.State) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("state = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.State))
            argPos++
        }

        if len(filter.Tags) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("tags && $%d", argPos))
            args = append(args, pq.Array(filter.Tags))
            argPos++
        }

        if filter.CreatedAfter != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argPos))
            args = append(args, filter.CreatedAfter)
            argPos++
        }

        if filter.CreatedBefore != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("created_at <= $%d", argPos))
            args = append(args, filter.CreatedBefore)
            argPos++
        }

        if filter.CreatedBy != "" {
            whereClauses = append(whereClauses, fmt.Sprintf("created_by = $%d", argPos))
            args = append(args, filter.CreatedBy)
            argPos++
        }

        if filter.RecipientListID != "" {
            whereClauses = append(whereClauses, fmt.Sprintf("recipient_list_id = $%d", argPos))
            args = append(args, filter.RecipientListID)
            argPos++
        }
    }

    whereClause := ""
    if len(whereClauses) > 0 {
        whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
    }

    query := fmt.Sprintf("SELECT COUNT(*) FROM campaigns %s", whereClause)

    var count int
    err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count campaigns: %w", err)
    }

    return count, nil
}

// CountByStatus returns the count of campaigns with a specific status
func (r *CampaignRepository) CountByStatus(ctx context.Context, status string) (int, error) {
    query := `SELECT COUNT(*) FROM campaigns WHERE status = $1 AND is_archived = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, status).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count campaigns by status: %w", err)
    }
    
    return count, nil
}

// CountByState returns the count of campaigns with a specific state
func (r *CampaignRepository) CountByState(ctx context.Context, state string) (int, error) {
    query := `SELECT COUNT(*) FROM campaigns WHERE state = $1 AND is_archived = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, state).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count campaigns by state: %w", err)
    }
    
    return count, nil
}

// CountActive returns the count of running campaigns
func (r *CampaignRepository) CountActive(ctx context.Context) (int, error) {
    return r.CountByStatus(ctx, "running")
}

// CountCompleted returns the count of completed campaigns
func (r *CampaignRepository) CountCompleted(ctx context.Context) (int, error) {
    return r.CountByStatus(ctx, "completed")
}

// CountFailed returns the count of failed campaigns
func (r *CampaignRepository) CountFailed(ctx context.Context) (int, error) {
    return r.CountByStatus(ctx, "failed")
}

// CountScheduled returns the count of scheduled campaigns
func (r *CampaignRepository) CountScheduled(ctx context.Context) (int, error) {
    return r.CountByStatus(ctx, "scheduled")
}

// CountPaused returns the count of paused campaigns
func (r *CampaignRepository) CountPaused(ctx context.Context) (int, error) {
    return r.CountByStatus(ctx, "paused")
}

// ============================================================================
// ADDITIONAL METRICS HELPERS
// ============================================================================

// GetTotalEmailsSent returns the total number of emails sent across all campaigns
func (r *CampaignRepository) GetTotalEmailsSent(ctx context.Context) (int64, error) {
    query := `SELECT COALESCE(SUM(sent_count), 0) FROM campaigns WHERE is_archived = false`
    
    var total int64
    err := r.db.QueryRowContext(ctx, query).Scan(&total)
    if err != nil {
        return 0, fmt.Errorf("failed to get total emails sent: %w", err)
    }
    
    return total, nil
}

// GetTotalEmailsFailed returns the total number of failed emails across all campaigns
func (r *CampaignRepository) GetTotalEmailsFailed(ctx context.Context) (int64, error) {
    query := `SELECT COALESCE(SUM(failed_count), 0) FROM campaigns WHERE is_archived = false`
    
    var total int64
    err := r.db.QueryRowContext(ctx, query).Scan(&total)
    if err != nil {
        return 0, fmt.Errorf("failed to get total emails failed: %w", err)
    }
    
    return total, nil
}

// GetAverageSuccessRate returns the average success rate across all campaigns
func (r *CampaignRepository) GetAverageSuccessRate(ctx context.Context) (float64, error) {
    query := `SELECT COALESCE(AVG(success_rate), 0) FROM campaigns WHERE is_archived = false AND status IN ('running', 'completed')`
    
    var avgRate float64
    err := r.db.QueryRowContext(ctx, query).Scan(&avgRate)
    if err != nil {
        return 0, fmt.Errorf("failed to get average success rate: %w", err)
    }
    
    return avgRate, nil
}

// GetRecentActivity returns recent campaign activities
func (r *CampaignRepository) GetRecentActivity(ctx context.Context, limit int) ([]*CampaignActivity, error) {
    if limit <= 0 {
        limit = 10
    }

    query := `
        SELECT id, name, status, state, updated_at
        FROM campaigns
        WHERE is_archived = false
        ORDER BY updated_at DESC
        LIMIT $1`

    rows, err := r.db.QueryContext(ctx, query, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get recent activity: %w", err)
    }
    defer rows.Close()

    activities := []*CampaignActivity{}
    for rows.Next() {
        var id, name, status, state string
        var timestamp time.Time

        if err := rows.Scan(&id, &name, &status, &state, &timestamp); err != nil {
            return nil, err
        }

        activity := &CampaignActivity{
            CampaignID:   id,
            CampaignName: name,
            ActivityType: determineActivityType(status, state),
            Details: map[string]interface{}{
                "status": status,
                "state":  state,
            },
            Timestamp: timestamp,
        }

        activities = append(activities, activity)
    }

    return activities, nil
}

// GetStatusBreakdown returns count of campaigns by status
func (r *CampaignRepository) GetStatusBreakdown(ctx context.Context) (map[string]int, error) {
    query := `
        SELECT status, COUNT(*) as count
        FROM campaigns
        WHERE is_archived = false
        GROUP BY status`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get status breakdown: %w", err)
    }
    defer rows.Close()

    breakdown := make(map[string]int)
    for rows.Next() {
        var status string
        var count int
        if err := rows.Scan(&status, &count); err != nil {
            return nil, err
        }
        breakdown[status] = count
    }

    return breakdown, nil
}

// GetStateBreakdown returns count of campaigns by state
func (r *CampaignRepository) GetStateBreakdown(ctx context.Context) (map[string]int, error) {
    query := `
        SELECT state, COUNT(*) as count
        FROM campaigns
        WHERE is_archived = false
        GROUP BY state`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get state breakdown: %w", err)
    }
    defer rows.Close()

    breakdown := make(map[string]int)
    for rows.Next() {
        var state string
        var count int
        if err := rows.Scan(&state, &count); err != nil {
            return nil, err
        }
        breakdown[state] = count
    }

    return breakdown, nil
}

// GetCampaignsByDateRange returns campaigns within a date range
func (r *CampaignRepository) GetCampaignsByDateRange(ctx context.Context, startDate, endDate time.Time) ([]*Campaign, error) {
    filter := &CampaignFilter{
        CreatedAfter:  &startDate,
        CreatedBefore: &endDate,
        Limit:         1000,
    }
    campaigns, _, err := r.List(ctx, filter)
    return campaigns, err
}

// GetTopPerformingCampaigns returns campaigns with highest success rates
func (r *CampaignRepository) GetTopPerformingCampaigns(ctx context.Context, limit int) ([]*Campaign, error) {
    if limit <= 0 {
        limit = 10
    }

    query := `
        SELECT id, name, description, status, state, success_rate, sent_count, 
               failed_count, total_recipients, created_at, updated_at
        FROM campaigns
        WHERE is_archived = false 
          AND status IN ('completed', 'running')
          AND total_recipients > 0
        ORDER BY success_rate DESC, sent_count DESC
        LIMIT $1`

    rows, err := r.db.QueryContext(ctx, query, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get top performing campaigns: %w", err)
    }
    defer rows.Close()

    campaigns := []*Campaign{}
    for rows.Next() {
        campaign := &Campaign{}
        err := rows.Scan(
            &campaign.ID, &campaign.Name, &campaign.Description, &campaign.Status,
            &campaign.State, &campaign.SuccessRate, &campaign.SentCount,
            &campaign.FailedCount, &campaign.TotalRecipients,
            &campaign.CreatedAt, &campaign.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        campaigns = append(campaigns, campaign)
    }

    return campaigns, nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// determineActivityType determines the activity type based on status and state
func determineActivityType(status, state string) string {
    switch status {
    case "running":
        return "campaign_started"
    case "completed":
        return "campaign_completed"
    case "failed":
        return "campaign_failed"
    case "paused":
        return "campaign_paused"
    case "scheduled":
        return "campaign_scheduled"
    default:
        return "campaign_updated"
    }
}
func NewCampaignRepository(db *sql.DB) *CampaignRepository {
	return &CampaignRepository{db: db}
}

func (r *CampaignRepository) Create(ctx context.Context, campaign *Campaign) error {
	query := `
		INSERT INTO campaigns (
			id, name, description, status, state, priority, scheduled_at,
			total_recipients, config, template_ids, account_ids, recipient_list_id,
			proxy_ids, tags, metadata, max_retries, created_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
		) RETURNING id, created_at, updated_at`

	configJSON, err := json.Marshal(campaign.Config)
	if err != nil {
		return fmt.Errorf("campaign create: failed to marshal config: %w", err)
	}
	metadataJSON, err := json.Marshal(campaign.Metadata)
	if err != nil {
		return fmt.Errorf("campaign create: failed to marshal metadata: %w", err)
	}

	// recipientListID must be passed as a typed nil so lib/pq correctly
	// sends NULL to the uuid column. A (*string)(nil) does NOT work.
	var recipientListID interface{}
	if campaign.RecipientListID != nil {
		recipientListID = *campaign.RecipientListID
	}

	now := time.Now()

	fmt.Printf("DEBUG repo.Create: id=%s status=%s state=%s priority=%d templateIDs=%v accountIDs=%v recipientListID=%v templateCount=%d accountCount=%d\n",
		campaign.ID, campaign.Status, campaign.State, campaign.Priority,
		campaign.TemplateIDs, campaign.AccountIDs, recipientListID,
		len(campaign.TemplateIDs), len(campaign.AccountIDs),
	)

	row := r.db.QueryRowContext(
		ctx, query,
		campaign.ID, campaign.Name, campaign.Description, campaign.Status, campaign.State,
		campaign.Priority, campaign.ScheduledAt, campaign.TotalRecipients,
		configJSON, pq.Array(campaign.TemplateIDs), pq.Array(campaign.AccountIDs),
		recipientListID, pq.Array(campaign.ProxyIDs), pq.Array(campaign.Tags),
		metadataJSON, campaign.MaxRetries, campaign.CreatedBy, now, now,
	)

	if err := row.Scan(&campaign.ID, &campaign.CreatedAt, &campaign.UpdatedAt); err != nil {
		return fmt.Errorf("campaign create: insert scan failed (check uuid format of template_ids/account_ids/recipient_list_id): %w", err)
	}

	fmt.Printf("DEBUG repo.Create: SUCCESS id=%s createdAt=%s\n", campaign.ID, campaign.CreatedAt.Format(time.RFC3339))
	return nil
}

func (r *CampaignRepository) GetByID(ctx context.Context, id string) (*Campaign, error) {
	query := `
		SELECT id, name, description, status, state, priority, scheduled_at, started_at,
			completed_at, paused_at, failed_at, total_recipients, sent_count, failed_count,
			pending_count, success_rate, progress, throughput, estimated_eta, config,
			template_ids, account_ids, recipient_list_id, proxy_ids, tags, metadata,
			error_message, last_checkpoint, checkpoint_data, retry_count, max_retries,
			created_at, updated_at, created_by, updated_by, is_archived, archived_at
		FROM campaigns
		WHERE id = $1 AND (is_archived = false OR is_archived IS NULL)`

	campaign := &Campaign{}
	var configJSON, metadataJSON, checkpointJSON []byte
	var errorMessage, createdBy, updatedBy, description sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&campaign.ID, &campaign.Name, &description, &campaign.Status, &campaign.State,
		&campaign.Priority, &campaign.ScheduledAt, &campaign.StartedAt, &campaign.CompletedAt,
		&campaign.PausedAt, &campaign.FailedAt, &campaign.TotalRecipients, &campaign.SentCount,
		&campaign.FailedCount, &campaign.PendingCount, &campaign.SuccessRate, &campaign.Progress,
		&campaign.Throughput, &campaign.EstimatedETA, &configJSON, pq.Array(&campaign.TemplateIDs),
		pq.Array(&campaign.AccountIDs), &campaign.RecipientListID, pq.Array(&campaign.ProxyIDs),
		pq.Array(&campaign.Tags), &metadataJSON, &errorMessage, &campaign.LastCheckpoint,
		&checkpointJSON, &campaign.RetryCount, &campaign.MaxRetries, &campaign.CreatedAt,
		&campaign.UpdatedAt, &campaign.CreatedBy, &campaign.UpdatedBy,  &createdBy, &updatedBy, 
		&campaign.IsArchived, &campaign.ArchivedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("campaign not found")
		}
		return nil, err
	}
	campaign.Description  = description.String
	campaign.ErrorMessage = errorMessage.String
	campaign.CreatedBy    = createdBy.String
	campaign.UpdatedBy    = updatedBy.String

	if len(configJSON) > 0 {
		json.Unmarshal(configJSON, &campaign.Config)
	}
	if len(metadataJSON) > 0 {
		json.Unmarshal(metadataJSON, &campaign.Metadata)
	}
	if len(checkpointJSON) > 0 {
		json.Unmarshal(checkpointJSON, &campaign.CheckpointData)
	}

	return campaign, nil
}

func (r *CampaignRepository) Update(ctx context.Context, campaign *Campaign) error {
	query := `
		UPDATE campaigns SET
			name = $2, description = $3, status = $4, state = $5, priority = $6,
			scheduled_at = $7, started_at = $8, completed_at = $9, paused_at = $10,
			failed_at = $11, total_recipients = $12, sent_count = $13, failed_count = $14,
			pending_count = $15, success_rate = $16, progress = $17, throughput = $18,
			estimated_eta = $19, config = $20, template_ids = $21, account_ids = $22,
			recipient_list_id = $23, proxy_ids = $24, tags = $25, metadata = $26,
			error_message = $27, last_checkpoint = $28, checkpoint_data = $29,
			retry_count = $30, max_retries = $31, updated_at = $32, updated_by = $33
		WHERE id = $1 AND (is_archived = false OR is_archived IS NULL)`

	configJSON, _ := json.Marshal(campaign.Config)
	metadataJSON, _ := json.Marshal(campaign.Metadata)
	checkpointJSON, _ := json.Marshal(campaign.CheckpointData)

	result, err := r.db.ExecContext(
		ctx, query,
		campaign.ID, campaign.Name, campaign.Description, campaign.Status, campaign.State,
		campaign.Priority, campaign.ScheduledAt, campaign.StartedAt, campaign.CompletedAt,
		campaign.PausedAt, campaign.FailedAt, campaign.TotalRecipients, campaign.SentCount,
		campaign.FailedCount, campaign.PendingCount, campaign.SuccessRate, campaign.Progress,
		campaign.Throughput, campaign.EstimatedETA, configJSON, pq.Array(campaign.TemplateIDs),
		pq.Array(campaign.AccountIDs), nullableUUID(campaign.RecipientListID), pq.Array(campaign.ProxyIDs),
		pq.Array(campaign.Tags), metadataJSON, campaign.ErrorMessage, campaign.LastCheckpoint,
		checkpointJSON, campaign.RetryCount, campaign.MaxRetries, time.Now(), campaign.UpdatedBy,
	)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("campaign not found or archived")
	}

	return nil
}

func (r *CampaignRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM campaigns WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return fmt.Errorf("campaign not found")
	}

	return nil
}

func (r *CampaignRepository) List(ctx context.Context, filter *CampaignFilter) ([]*Campaign, int, error) {
	whereClauses := []string{}
	args := []interface{}{}
	argPos := 1

	if !filter.IncludeArchived {
		whereClauses = append(whereClauses, "(is_archived IS NULL OR is_archived = false)")
	}

	if len(filter.IDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.IDs))
		argPos++
	}

	if len(filter.Status) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("status = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.Status))
		argPos++
	}

	if len(filter.State) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("state = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.State))
		argPos++
	}

	if len(filter.Tags) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("tags && $%d", argPos))
		args = append(args, pq.Array(filter.Tags))
		argPos++
	}

	if filter.CreatedAfter != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argPos))
		args = append(args, filter.CreatedAfter)
		argPos++
	}

	if filter.CreatedBefore != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at <= $%d", argPos))
		args = append(args, filter.CreatedBefore)
		argPos++
	}

	if filter.ScheduledAfter != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("scheduled_at >= $%d", argPos))
		args = append(args, filter.ScheduledAfter)
		argPos++
	}

	if filter.ScheduledBefore != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("scheduled_at <= $%d", argPos))
		args = append(args, filter.ScheduledBefore)
		argPos++
	}

	if filter.MinSuccessRate != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("success_rate >= $%d", argPos))
		args = append(args, filter.MinSuccessRate)
		argPos++
	}

	if filter.MaxSuccessRate != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("success_rate <= $%d", argPos))
		args = append(args, filter.MaxSuccessRate)
		argPos++
	}

	if filter.RecipientListID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("recipient_list_id = $%d", argPos))
		args = append(args, filter.RecipientListID)
		argPos++
	}

	if filter.CreatedBy != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("created_by = $%d", argPos))
		args = append(args, filter.CreatedBy)
		argPos++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR description ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+filter.Search+"%")
		argPos++
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM campaigns %s", whereClause)
	var total int
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	sortBy := "created_at"
	if filter.SortBy != "" {
		sortBy = filter.SortBy
	}

	sortOrder := "DESC"
	if filter.SortOrder != "" {
		sortOrder = strings.ToUpper(filter.SortOrder)
	}

	limit := 50
	if filter.Limit > 0 {
		limit = filter.Limit
	}

	offset := 0
	if filter.Offset > 0 {
		offset = filter.Offset
	}

	query := fmt.Sprintf(`
		SELECT id, name, description, status, state, priority, scheduled_at, started_at,
			completed_at, paused_at, failed_at, total_recipients, sent_count, failed_count,
			pending_count, success_rate, progress, throughput, estimated_eta, config,
			template_ids, account_ids, recipient_list_id, proxy_ids, tags, metadata,
			error_message, last_checkpoint, checkpoint_data, retry_count, max_retries,
			created_at, updated_at, created_by, updated_by, is_archived, archived_at
		FROM campaigns
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d`,
		whereClause, sortBy, sortOrder, argPos, argPos+1)

	args = append(args, limit, offset)
    rows, err := r.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()

    campaigns := []*Campaign{}
    for rows.Next() {
        campaign := &Campaign{}
        var configJSON, metadataJSON, checkpointJSON []byte
        var errorMessage, createdBy, updatedBy, description sql.NullString

        err := rows.Scan(
            &campaign.ID, &campaign.Name, &description, &campaign.Status, &campaign.State,
            &campaign.Priority, &campaign.ScheduledAt, &campaign.StartedAt, &campaign.CompletedAt,
            &campaign.PausedAt, &campaign.FailedAt, &campaign.TotalRecipients, &campaign.SentCount,
            &campaign.FailedCount, &campaign.PendingCount, &campaign.SuccessRate, &campaign.Progress,
            &campaign.Throughput, &campaign.EstimatedETA, &configJSON, pq.Array(&campaign.TemplateIDs),
            pq.Array(&campaign.AccountIDs), &campaign.RecipientListID, pq.Array(&campaign.ProxyIDs),
            pq.Array(&campaign.Tags), &metadataJSON, &errorMessage, &campaign.LastCheckpoint,
            &checkpointJSON, &campaign.RetryCount, &campaign.MaxRetries, &campaign.CreatedAt,
            &campaign.UpdatedAt, &createdBy, &updatedBy, &campaign.IsArchived,
            &campaign.ArchivedAt,
        )
        if err != nil {
            return nil, 0, err
        }

        campaign.ErrorMessage = errorMessage.String
        campaign.CreatedBy = createdBy.String
        campaign.UpdatedBy = updatedBy.String
		campaign.Description = description.String

        if len(configJSON) > 0 {
            json.Unmarshal(configJSON, &campaign.Config)
        }
        if len(metadataJSON) > 0 {
            json.Unmarshal(metadataJSON, &campaign.Metadata)
        }
        if len(checkpointJSON) > 0 {
            json.Unmarshal(checkpointJSON, &campaign.CheckpointData)
        }

        campaigns = append(campaigns, campaign)
    }

    if err := rows.Err(); err != nil {
        return nil, 0, err
    }

    return campaigns, total, nil
}

func (r *CampaignRepository) UpdateStatus(ctx context.Context, id, status string, updatedBy string) error {
	query := `
		UPDATE campaigns SET status = $2, updated_at = $3, updated_by = $4
		WHERE id = $1 AND (is_archived = false OR is_archived IS NULL)`

	result, err := r.db.ExecContext(ctx, query, id, status, time.Now(), updatedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("campaign not found")
	}
	return nil
}


func (r *CampaignRepository) UpdateState(ctx context.Context, id, state string) error {
	query := `
		UPDATE campaigns SET state = $2, updated_at = $3
		WHERE id = $1 AND (is_archived = false OR is_archived IS NULL)`

	result, err := r.db.ExecContext(ctx, query, id, state, time.Now())
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("campaign not found")
	}
	return nil
}


func (r *CampaignRepository) UpdateProgress(ctx context.Context, id string, sent, failed, pending int) error {
	query := `
		UPDATE campaigns SET
			sent_count = $2,
			failed_count = $3,
			pending_count = $4,
			progress = CASE WHEN total_recipients > 0 THEN (($2 + $3) * 100.0 / total_recipients) ELSE 0 END,
			success_rate = CASE WHEN ($2 + $3) > 0 THEN ($2 * 100.0 / ($2 + $3)) ELSE 0 END,
			updated_at = $5
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id, sent, failed, pending, time.Now())
	return err
}

func (r *CampaignRepository) Archive(ctx context.Context, id string) error {
	query := `
		UPDATE campaigns SET is_archived = true, archived_at = $2, updated_at = $3
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id, time.Now(), time.Now())
	return err
}

func (r *CampaignRepository) Restore(ctx context.Context, id string) error {
	query := `
		UPDATE campaigns SET is_archived = false, archived_at = NULL, updated_at = $2
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *CampaignRepository) GetScheduled(ctx context.Context, before time.Time) ([]*Campaign, error) {
	query := `
		SELECT id, name, description, status, state, priority, scheduled_at, config,
			template_ids, account_ids, recipient_list_id, proxy_ids, tags, metadata,
			total_recipients, created_at, updated_at
		FROM campaigns
		WHERE scheduled_at <= $1 AND status = 'scheduled' AND is_archived = false
		ORDER BY scheduled_at ASC`

	rows, err := r.db.QueryContext(ctx, query, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	campaigns := []*Campaign{}
	for rows.Next() {
		campaign := &Campaign{}
		var configJSON, metadataJSON []byte

		err := rows.Scan(
			&campaign.ID, &campaign.Name, &campaign.Description, &campaign.Status, &campaign.State,
			&campaign.Priority, &campaign.ScheduledAt, &configJSON, pq.Array(&campaign.TemplateIDs),
			pq.Array(&campaign.AccountIDs), &campaign.RecipientListID, pq.Array(&campaign.ProxyIDs),
			pq.Array(&campaign.Tags), &metadataJSON, &campaign.TotalRecipients,
			&campaign.CreatedAt, &campaign.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if len(configJSON) > 0 {
			json.Unmarshal(configJSON, &campaign.Config)
		}
		if len(metadataJSON) > 0 {
			json.Unmarshal(metadataJSON, &campaign.Metadata)
		}

		campaigns = append(campaigns, campaign)
	}

	return campaigns, nil
}

func (r *CampaignRepository) GetStats(ctx context.Context) (*CampaignStatsResult, error) {
	stats := &CampaignStatsResult{
		StatusBreakdown: make(map[string]int),
		StateBreakdown:  make(map[string]int),
	}

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'running') as active,
			COUNT(*) FILTER (WHERE status = 'completed') as completed,
			COUNT(*) FILTER (WHERE status = 'failed') as failed,
			COUNT(*) FILTER (WHERE status = 'paused') as paused,
			COUNT(*) FILTER (WHERE status = 'scheduled') as scheduled,
			COALESCE(SUM(sent_count), 0) as total_sent,
			COALESCE(SUM(failed_count), 0) as total_failed,
			COALESCE(AVG(success_rate), 0) as avg_success_rate,
			COALESCE(AVG(throughput), 0) as avg_throughput
		FROM campaigns
		WHERE is_archived = false`

	err := r.db.QueryRowContext(ctx, query).Scan(
		&stats.TotalCampaigns,
		&stats.ActiveCampaigns,
		&stats.CompletedCampaigns,
		&stats.FailedCampaigns,
		&stats.PausedCampaigns,
		&stats.ScheduledCampaigns,
		&stats.TotalSent,
		&stats.TotalFailed,
		&stats.AverageSuccessRate,
		&stats.AverageThroughput,
	)

	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (r *CampaignRepository) SaveCheckpoint(ctx context.Context, id string, data map[string]interface{}) error {
	checkpointJSON, _ := json.Marshal(data)

	query := `
		UPDATE campaigns SET
			checkpoint_data = $2,
			last_checkpoint = $3,
			updated_at = $4
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id, checkpointJSON, time.Now(), time.Now())
	return err
}

func (r *CampaignRepository) IncrementRetry(ctx context.Context, id string) error {
	query := `
		UPDATE campaigns SET retry_count = retry_count + 1, updated_at = $2
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *CampaignRepository) GetByStatus(ctx context.Context, status string) ([]*Campaign, error) {
	filter := &CampaignFilter{
		Status: []string{status},
		Limit:  1000,
	}
	campaigns, _, err := r.List(ctx, filter)
	return campaigns, err
}

func (r *CampaignRepository) BulkUpdateStatus(ctx context.Context, ids []string, status string) error {
	query := `
		UPDATE campaigns SET status = $2, updated_at = $3
		WHERE id = ANY($1) AND is_archived = false`

	_, err := r.db.ExecContext(ctx, query, pq.Array(ids), status, time.Now())
	return err
}

func (r *CampaignRepository) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	query := `
		DELETE FROM campaigns
		WHERE created_at < $1 AND status IN ('completed', 'failed') AND is_archived = true`

	result, err := r.db.ExecContext(ctx, query, before)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
func nullableUUID(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}
