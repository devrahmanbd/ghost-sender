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

type RecipientRepository struct {
	db *sql.DB
}

type Recipient struct {
	ID              string                 `json:"id" db:"id"`
	ListID          string                 `json:"list_id" db:"list_id"`
	Email           string                 `json:"email" db:"email"`
	Name            string                 `json:"name" db:"name"`
	FirstName       string                 `json:"first_name" db:"first_name"`
	LastName        string                 `json:"last_name" db:"last_name"`
	Domain          string                 `json:"domain" db:"domain"`
	Status          string                 `json:"status" db:"status"`
	Source          string                 `json:"source" db:"source"`
	Tags            []string               `json:"tags" db:"tags"`
	Attributes      map[string]interface{} `json:"attributes" db:"attributes"`
	Metadata        map[string]interface{} `json:"metadata" db:"metadata"`
	IsValid         bool                   `json:"is_valid" db:"is_valid"`
	ValidationError string                 `json:"validation_error" db:"validation_error"`
	ValidationAt    *time.Time             `json:"validation_at" db:"validation_at"`
	ValidMX         bool                   `json:"valid_mx" db:"valid_mx"`
	HasDNS          bool                   `json:"has_dns" db:"has_dns"`
	BounceCount     int                    `json:"bounce_count" db:"bounce_count"`
	ComplaintCount  int                    `json:"complaint_count" db:"complaint_count"`
	LastBounceAt    *time.Time             `json:"last_bounce_at" db:"last_bounce_at"`
	LastComplaintAt *time.Time             `json:"last_complaint_at" db:"last_complaint_at"`
	Unsubscribed    bool                   `json:"unsubscribed" db:"unsubscribed"`
	UnsubscribedAt  *time.Time             `json:"unsubscribed_at" db:"unsubscribed_at"`
	UnsubscribeReason string               `json:"unsubscribe_reason" db:"unsubscribe_reason"`
	UnsubscribeToken  string               `json:"unsubscribe_token" db:"unsubscribe_token"`
	HardBounced     bool                   `json:"hard_bounced" db:"hard_bounced"`
	SoftBounced     bool                   `json:"soft_bounced" db:"soft_bounced"`
	EngagementScore float64                `json:"engagement_score" db:"engagement_score"`
	Opens           int                    `json:"opens" db:"opens"`
	Clicks          int                    `json:"clicks" db:"clicks"`
	LastOpenAt      *time.Time             `json:"last_open_at" db:"last_open_at"`
	LastClickAt     *time.Time             `json:"last_click_at" db:"last_click_at"`
	LastSentAt      *time.Time             `json:"last_sent_at" db:"last_sent_at"`
	SentCount       int64                  `json:"sent_count" db:"sent_count"`
	FailedCount     int64                  `json:"failed_count" db:"failed_count"`
	CreatedAt       time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at" db:"updated_at"`
	CreatedBy       string                 `json:"created_by" db:"created_by"`
	UpdatedBy       string                 `json:"updated_by" db:"updated_by"`
}

func (r *RecipientRepository) Count(ctx context.Context, filter *RecipientFilter) (int, error) {
    whereClauses := []string{"1=1"}
    args := []interface{}{}
    argPos := 1

    if filter != nil {
        if len(filter.IDs) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.IDs))
            argPos++
        }

        if len(filter.ListIDs) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("list_id = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.ListIDs))
            argPos++
        }

        if len(filter.Domains) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("domain = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.Domains))
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

        if filter.SubscribedOnly {
            whereClauses = append(whereClauses, "unsubscribed = false")
        }

        if filter.UnsubscribedOnly {
            whereClauses = append(whereClauses, "unsubscribed = true")
        }

        if filter.ValidOnly {
            whereClauses = append(whereClauses, "is_valid = true")
        }

        if filter.InvalidOnly {
            whereClauses = append(whereClauses, "is_valid = false")
        }

        if filter.HasBounces {
            whereClauses = append(whereClauses, "(bounce_count > 0 OR hard_bounced = true OR soft_bounced = true)")
        }

        if filter.HasComplaints {
            whereClauses = append(whereClauses, "complaint_count > 0")
        }

        if filter.MinEngagement != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("engagement_score >= $%d", argPos))
            args = append(args, *filter.MinEngagement)
            argPos++
        }

        if filter.MaxEngagement != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("engagement_score <= $%d", argPos))
            args = append(args, *filter.MaxEngagement)
            argPos++
        }
    }

    whereClause := strings.Join(whereClauses, " AND ")
    query := fmt.Sprintf("SELECT COUNT(*) FROM recipients WHERE %s", whereClause)

    var count int
    err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count recipients: %w", err)
    }

    return count, nil
}

// CountValid returns the count of valid recipients
func (r *RecipientRepository) CountValid(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE is_valid = true`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count valid recipients: %w", err)
    }
    
    return count, nil
}

// CountInvalid returns the count of invalid recipients
func (r *RecipientRepository) CountInvalid(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE is_valid = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count invalid recipients: %w", err)
    }
    
    return count, nil
}

// CountSubscribed returns the count of subscribed recipients
func (r *RecipientRepository) CountSubscribed(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE unsubscribed = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count subscribed recipients: %w", err)
    }
    
    return count, nil
}

// CountUnsubscribed returns the count of unsubscribed recipients
func (r *RecipientRepository) CountUnsubscribed(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE unsubscribed = true`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count unsubscribed recipients: %w", err)
    }
    
    return count, nil
}

// CountHardBounced returns the count of hard bounced recipients
func (r *RecipientRepository) CountHardBounced(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE hard_bounced = true`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count hard bounced recipients: %w", err)
    }
    
    return count, nil
}

// CountSoftBounced returns the count of soft bounced recipients
func (r *RecipientRepository) CountSoftBounced(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE soft_bounced = true AND hard_bounced = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count soft bounced recipients: %w", err)
    }
    
    return count, nil
}

// CountWithComplaints returns the count of recipients with complaints
func (r *RecipientRepository) CountWithComplaints(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE complaint_count > 0`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count recipients with complaints: %w", err)
    }
    
    return count, nil
}

// CountByStatus returns the count of recipients with a specific status
func (r *RecipientRepository) CountByStatus(ctx context.Context, status string) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE status = $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, status).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count recipients by status: %w", err)
    }
    
    return count, nil
}

// CountByDomain returns the count of recipients from a specific domain
func (r *RecipientRepository) CountByDomain(ctx context.Context, domain string) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE domain = $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, domain).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count recipients by domain: %w", err)
    }
    
    return count, nil
}

// CountByList returns the count of recipients in a specific list
func (r *RecipientRepository) CountByList(ctx context.Context, listID string) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE list_id = $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, listID).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count recipients by list: %w", err)
    }
    
    return count, nil
}

// CountByListAndStatus returns the count of recipients in a list with specific status
func (r *RecipientRepository) CountByListAndStatus(ctx context.Context, listID, status string) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE list_id = $1 AND status = $2`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, listID, status).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count recipients by list and status: %w", err)
    }
    
    return count, nil
}

// CountHighEngagement returns recipients with engagement score above threshold
func (r *RecipientRepository) CountHighEngagement(ctx context.Context, threshold float64) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE engagement_score >= $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, threshold).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count high engagement recipients: %w", err)
    }
    
    return count, nil
}

// CountLowEngagement returns recipients with engagement score below threshold
func (r *RecipientRepository) CountLowEngagement(ctx context.Context, threshold float64) (int, error) {
    query := `SELECT COUNT(*) FROM recipients WHERE engagement_score < $1`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, threshold).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count low engagement recipients: %w", err)
    }
    
    return count, nil
}

func (r *RecipientRepository) GetTotalEmailsSent(ctx context.Context) (int64, error) {
    query := `SELECT COALESCE(SUM(sent_count), 0) FROM recipients`
    
    var total int64
    err := r.db.QueryRowContext(ctx, query).Scan(&total)
    if err != nil {
        return 0, fmt.Errorf("failed to get total emails sent: %w", err)
    }
    
    return total, nil
}

// GetTotalEmailsFailed returns the total number of failed emails across all recipients
func (r *RecipientRepository) GetTotalEmailsFailed(ctx context.Context) (int64, error) {
    query := `SELECT COALESCE(SUM(failed_count), 0) FROM recipients`
    
    var total int64
    err := r.db.QueryRowContext(ctx, query).Scan(&total)
    if err != nil {
        return 0, fmt.Errorf("failed to get total emails failed: %w", err)
    }
    
    return total, nil
}

// GetAverageEngagement returns the average engagement score across all recipients
func (r *RecipientRepository) GetAverageEngagement(ctx context.Context) (float64, error) {
    query := `SELECT COALESCE(AVG(engagement_score), 0) FROM recipients WHERE sent_count > 0`
    
    var avgScore float64
    err := r.db.QueryRowContext(ctx, query).Scan(&avgScore)
    if err != nil {
        return 0, fmt.Errorf("failed to get average engagement: %w", err)
    }
    
    return avgScore, nil
}

// GetDomainBreakdown returns count of recipients grouped by domain
func (r *RecipientRepository) GetDomainBreakdown(ctx context.Context, listID string) (map[string]int, error) {
    var query string
    var args []interface{}
    
    if listID != "" {
        query = `
            SELECT domain, COUNT(*) as count
            FROM recipients
            WHERE list_id = $1
            GROUP BY domain
            ORDER BY count DESC
            LIMIT 100`
        args = append(args, listID)
    } else {
        query = `
            SELECT domain, COUNT(*) as count
            FROM recipients
            GROUP BY domain
            ORDER BY count DESC
            LIMIT 100`
    }

    rows, err := r.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("failed to get domain breakdown: %w", err)
    }
    defer rows.Close()

    breakdown := make(map[string]int)
    for rows.Next() {
        var domain string
        var count int
        if err := rows.Scan(&domain, &count); err != nil {
            return nil, err
        }
        breakdown[domain] = count
    }

    return breakdown, nil
}

// GetStatusBreakdown returns count of recipients grouped by status
func (r *RecipientRepository) GetStatusBreakdown(ctx context.Context, listID string) (map[string]int, error) {
    var query string
    var args []interface{}
    
    if listID != "" {
        query = `
            SELECT status, COUNT(*) as count
            FROM recipients
            WHERE list_id = $1
            GROUP BY status`
        args = append(args, listID)
    } else {
        query = `
            SELECT status, COUNT(*) as count
            FROM recipients
            GROUP BY status`
    }

    rows, err := r.db.QueryContext(ctx, query, args...)
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

// GetMostEngaged returns recipients with highest engagement scores
func (r *RecipientRepository) GetMostEngaged(ctx context.Context, listID string, limit int) ([]*Recipient, error) {
    if limit <= 0 {
        limit = 10
    }

    var query string
    var args []interface{}
    
    if listID != "" {
        query = `
            SELECT id, list_id, email, name, engagement_score, opens, clicks, 
                   sent_count, last_open_at, last_click_at, created_at, updated_at
            FROM recipients
            WHERE list_id = $1 AND sent_count > 0
            ORDER BY engagement_score DESC, opens DESC, clicks DESC
            LIMIT $2`
        args = append(args, listID, limit)
    } else {
        query = `
            SELECT id, list_id, email, name, engagement_score, opens, clicks, 
                   sent_count, last_open_at, last_click_at, created_at, updated_at
            FROM recipients
            WHERE sent_count > 0
            ORDER BY engagement_score DESC, opens DESC, clicks DESC
            LIMIT $1`
        args = append(args, limit)
    }

    rows, err := r.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("failed to get most engaged recipients: %w", err)
    }
    defer rows.Close()

    recipients := []*Recipient{}
    for rows.Next() {
        rec := &Recipient{}
        err := rows.Scan(
            &rec.ID, &rec.ListID, &rec.Email, &rec.Name,
            &rec.EngagementScore, &rec.Opens, &rec.Clicks,
            &rec.SentCount, &rec.LastOpenAt, &rec.LastClickAt,
            &rec.CreatedAt, &rec.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        recipients = append(recipients, rec)
    }

    return recipients, nil
}

// GetLeastEngaged returns recipients with lowest engagement scores
func (r *RecipientRepository) GetLeastEngaged(ctx context.Context, listID string, limit int) ([]*Recipient, error) {
    if limit <= 0 {
        limit = 10
    }

    var query string
    var args []interface{}
    
    if listID != "" {
        query = `
            SELECT id, list_id, email, name, engagement_score, opens, clicks, 
                   sent_count, last_sent_at, created_at, updated_at
            FROM recipients
            WHERE list_id = $1 AND sent_count > 0 AND unsubscribed = false
            ORDER BY engagement_score ASC, last_sent_at ASC
            LIMIT $2`
        args = append(args, listID, limit)
    } else {
        query = `
            SELECT id, list_id, email, name, engagement_score, opens, clicks, 
                   sent_count, last_sent_at, created_at, updated_at
            FROM recipients
            WHERE sent_count > 0 AND unsubscribed = false
            ORDER BY engagement_score ASC, last_sent_at ASC
            LIMIT $1`
        args = append(args, limit)
    }

    rows, err := r.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("failed to get least engaged recipients: %w", err)
    }
    defer rows.Close()

    recipients := []*Recipient{}
    for rows.Next() {
        rec := &Recipient{}
        err := rows.Scan(
            &rec.ID, &rec.ListID, &rec.Email, &rec.Name,
            &rec.EngagementScore, &rec.Opens, &rec.Clicks,
            &rec.SentCount, &rec.LastSentAt,
            &rec.CreatedAt, &rec.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        recipients = append(recipients, rec)
    }

    return recipients, nil
}

// GetRecentlyAdded returns recently added recipients
func (r *RecipientRepository) GetRecentlyAdded(ctx context.Context, listID string, limit int) ([]*Recipient, error) {
    if limit <= 0 {
        limit = 100
    }

    var query string
    var args []interface{}
    
    if listID != "" {
        query = `
            SELECT id, list_id, email, name, status, is_valid, created_at, updated_at
            FROM recipients
            WHERE list_id = $1
            ORDER BY created_at DESC
            LIMIT $2`
        args = append(args, listID, limit)
    } else {
        query = `
            SELECT id, list_id, email, name, status, is_valid, created_at, updated_at
            FROM recipients
            ORDER BY created_at DESC
            LIMIT $1`
        args = append(args, limit)
    }

    rows, err := r.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("failed to get recently added recipients: %w", err)
    }
    defer rows.Close()

    recipients := []*Recipient{}
    for rows.Next() {
        rec := &Recipient{}
        err := rows.Scan(
            &rec.ID, &rec.ListID, &rec.Email, &rec.Name,
            &rec.Status, &rec.IsValid,
            &rec.CreatedAt, &rec.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        recipients = append(recipients, rec)
    }

    return recipients, nil
}

// GetInactiveRecipients returns recipients who haven't engaged in specified duration
func (r *RecipientRepository) GetInactiveRecipients(ctx context.Context, listID string, inactiveDays int, limit int) ([]*Recipient, error) {
    if limit <= 0 {
        limit = 100
    }
    
    inactiveDate := time.Now().AddDate(0, 0, -inactiveDays)

    var query string
    var args []interface{}
    
    if listID != "" {
        query = `
            SELECT id, list_id, email, name, engagement_score, last_open_at, 
                   last_click_at, last_sent_at, sent_count, created_at, updated_at
            FROM recipients
            WHERE list_id = $1 
              AND sent_count > 0 
              AND unsubscribed = false
              AND (last_open_at IS NULL OR last_open_at < $2)
              AND (last_click_at IS NULL OR last_click_at < $2)
            ORDER BY last_sent_at DESC
            LIMIT $3`
        args = append(args, listID, inactiveDate, limit)
    } else {
        query = `
            SELECT id, list_id, email, name, engagement_score, last_open_at, 
                   last_click_at, last_sent_at, sent_count, created_at, updated_at
            FROM recipients
            WHERE sent_count > 0 
              AND unsubscribed = false
              AND (last_open_at IS NULL OR last_open_at < $1)
              AND (last_click_at IS NULL OR last_click_at < $1)
            ORDER BY last_sent_at DESC
            LIMIT $2`
        args = append(args, inactiveDate, limit)
    }

    rows, err := r.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("failed to get inactive recipients: %w", err)
    }
    defer rows.Close()

    recipients := []*Recipient{}
    for rows.Next() {
        rec := &Recipient{}
        err := rows.Scan(
            &rec.ID, &rec.ListID, &rec.Email, &rec.Name,
            &rec.EngagementScore, &rec.LastOpenAt, &rec.LastClickAt,
            &rec.LastSentAt, &rec.SentCount,
            &rec.CreatedAt, &rec.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        recipients = append(recipients, rec)
    }

    return recipients, nil
}

// GetTopDomains returns the most common domains
func (r *RecipientRepository) GetTopDomains(ctx context.Context, limit int) ([]struct {
    Domain string
    Count  int
}, error) {
    if limit <= 0 {
        limit = 10
    }

    query := `
        SELECT domain, COUNT(*) as count
        FROM recipients
        WHERE domain IS NOT NULL AND domain <> ''
        GROUP BY domain
        ORDER BY count DESC
        LIMIT $1`

    rows, err := r.db.QueryContext(ctx, query, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get top domains: %w", err)
    }
    defer rows.Close()

    type domainCount struct {
        Domain string
        Count  int
    }

    results := []struct {
        Domain string
        Count  int
    }{}

    for rows.Next() {
        var dc domainCount
        if err := rows.Scan(&dc.Domain, &dc.Count); err != nil {
            return nil, err
        }
        results = append(results, struct {
            Domain string
            Count  int
        }{Domain: dc.Domain, Count: dc.Count})
    }

    return results, nil
}

type RecipientFilter struct {
	IDs              []string
	ListIDs          []string
	Emails           []string
	Domains          []string
	Tags             []string
	Status           []string
	SubscribedOnly   bool
	UnsubscribedOnly bool
	ValidOnly        bool
	InvalidOnly      bool
	HasBounces       bool
	HasComplaints    bool
	Search           string
	MinEngagement    *float64
	MaxEngagement    *float64
	CreatedAfter     *time.Time
	CreatedBefore    *time.Time
	UpdatedAfter     *time.Time
	UpdatedBefore    *time.Time
	SortBy           string
	SortOrder        string
	Limit            int
	Offset           int
}

type RecipientStats struct {
	TotalRecipients    int             `json:"total_recipients"`
	ValidRecipients    int             `json:"valid_recipients"`
	InvalidRecipients  int             `json:"invalid_recipients"`
	Subscribed         int             `json:"subscribed"`
	Unsubscribed       int             `json:"unsubscribed"`
	HardBounced        int             `json:"hard_bounced"`
	SoftBounced        int             `json:"soft_bounced"`
	WithComplaints     int             `json:"with_complaints"`
	TotalSent          int64           `json:"total_sent"`
	TotalFailed        int64           `json:"total_failed"`
	AverageEngagement  float64         `json:"average_engagement"`
	DomainBreakdown    map[string]int  `json:"domain_breakdown"`
	StatusBreakdown    map[string]int  `json:"status_breakdown"`
	TagTop             map[string]int  `json:"tag_top"`
}

func NewRecipientRepository(db *sql.DB) *RecipientRepository {
	return &RecipientRepository{db: db}
}

// backend/internal/storage/repository/recipient.go
// Replace only the Create method:

func (r *RecipientRepository) Create(ctx context.Context, rec *Recipient) error {
	fmt.Printf("🟢 DEBUG Repo.Create: list_id=%s email=%s\n", rec.ListID, rec.Email)

	customFieldsJSON, _ := json.Marshal(rec.Attributes)
	if customFieldsJSON == nil {
		customFieldsJSON = []byte("{}")
	}
	metaJSON, _ := json.Marshal(rec.Metadata)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}

	now := time.Now()
	query := `
		INSERT INTO recipients
			(list_id, email, name, first_name, last_name, status, custom_fields, metadata, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, query,
		rec.ListID,       // $1
		rec.Email,        // $2
		rec.Name,         // $3
		rec.FirstName,    // $4
		rec.LastName,     // $5
		rec.Status,       // $6
		customFieldsJSON, // $7
		metaJSON,         // $8
		now,              // $9
		now,              // $10
	).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		fmt.Printf("🔴 DEBUG DB Scan ERROR: %v\n", err)
		return err
	}

	fmt.Printf("🟢 DEBUG Repo.Create SUCCESS id=%s\n", rec.ID)
	return nil
}

func (r *RecipientRepository) GetByID(ctx context.Context, id string) (*Recipient, error) {
	query := `
		SELECT id, list_id, email, name, first_name, last_name, domain, status, source,
			tags, attributes, metadata, is_valid, validation_error, validation_at,
			valid_mx, has_dns, bounce_count, complaint_count, last_bounce_at,
			last_complaint_at, unsubscribed, unsubscribed_at, unsubscribe_reason,
			unsubscribe_token, hard_bounced, soft_bounced, engagement_score,
			opens, clicks, last_open_at, last_click_at, last_sent_at,
			sent_count, failed_count, created_at, updated_at, created_by, updated_by
		FROM recipients
		WHERE id = $1`

	rec := &Recipient{}
	var attrsJSON, metaJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&rec.ID, &rec.ListID, &rec.Email, &rec.Name, &rec.FirstName, &rec.LastName,
		&rec.Domain, &rec.Status, &rec.Source, pq.Array(&rec.Tags), &attrsJSON,
		&metaJSON, &rec.IsValid, &rec.ValidationError, &rec.ValidationAt,
		&rec.ValidMX, &rec.HasDNS, &rec.BounceCount, &rec.ComplaintCount,
		&rec.LastBounceAt, &rec.LastComplaintAt, &rec.Unsubscribed, &rec.UnsubscribedAt,
		&rec.UnsubscribeReason, &rec.UnsubscribeToken, &rec.HardBounced, &rec.SoftBounced,
		&rec.EngagementScore, &rec.Opens, &rec.Clicks, &rec.LastOpenAt, &rec.LastClickAt,
		&rec.LastSentAt, &rec.SentCount, &rec.FailedCount, &rec.CreatedAt, &rec.UpdatedAt,
		&rec.CreatedBy, &rec.UpdatedBy,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("recipient not found")
		}
		return nil, err
	}

	if len(attrsJSON) > 0 {
		json.Unmarshal(attrsJSON, &rec.Attributes)
	}
	if len(metaJSON) > 0 {
		json.Unmarshal(metaJSON, &rec.Metadata)
	}

	return rec, nil
}

func (r *RecipientRepository) GetByEmail(ctx context.Context, listID, email string) (*Recipient, error) {
	query := `
		SELECT id, list_id, email, name, first_name, last_name, domain, status, source,
			tags, attributes, metadata, unsubscribed, hard_bounced, soft_bounced,
			engagement_score, created_at, updated_at
		FROM recipients
		WHERE list_id = $1 AND email = $2
		LIMIT 1`

	rec := &Recipient{}
	var attrsJSON, metaJSON []byte

	err := r.db.QueryRowContext(ctx, query, listID, email).Scan(
		&rec.ID, &rec.ListID, &rec.Email, &rec.Name, &rec.FirstName, &rec.LastName,
		&rec.Domain, &rec.Status, &rec.Source, pq.Array(&rec.Tags), &attrsJSON,
		&metaJSON, &rec.Unsubscribed, &rec.HardBounced, &rec.SoftBounced,
		&rec.EngagementScore, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("recipient not found")
		}
		return nil, err
	}

	if len(attrsJSON) > 0 {
		json.Unmarshal(attrsJSON, &rec.Attributes)
	}
	if len(metaJSON) > 0 {
		json.Unmarshal(metaJSON, &rec.Metadata)
	}

	return rec, nil
}

func (r *RecipientRepository) Update(ctx context.Context, rec *Recipient) error {
	query := `
		UPDATE recipients SET
			list_id = $2, email = $3, name = $4, first_name = $5, last_name = $6,
			domain = $7, status = $8, source = $9, tags = $10, attributes = $11,
			metadata = $12, is_valid = $13, validation_error = $14, validation_at = $15,
			valid_mx = $16, has_dns = $17, bounce_count = $18, complaint_count = $19,
			last_bounce_at = $20, last_complaint_at = $21, unsubscribed = $22,
			unsubscribed_at = $23, unsubscribe_reason = $24, unsubscribe_token = $25,
			hard_bounced = $26, soft_bounced = $27, engagement_score = $28,
			opens = $29, clicks = $30, last_open_at = $31, last_click_at = $32,
			last_sent_at = $33, sent_count = $34, failed_count = $35,
			updated_by = $36, updated_at = $37
		WHERE id = $1`

	attrsJSON, _ := json.Marshal(rec.Attributes)
	metaJSON, _ := json.Marshal(rec.Metadata)

	result, err := r.db.ExecContext(
		ctx, query,
		rec.ID, rec.ListID, rec.Email, rec.Name, rec.FirstName, rec.LastName,
		rec.Domain, rec.Status, rec.Source, pq.Array(rec.Tags), attrsJSON, metaJSON,
		rec.IsValid, rec.ValidationError, rec.ValidationAt, rec.ValidMX, rec.HasDNS,
		rec.BounceCount, rec.ComplaintCount, rec.LastBounceAt, rec.LastComplaintAt,
		rec.Unsubscribed, rec.UnsubscribedAt, rec.UnsubscribeReason, rec.UnsubscribeToken,
		rec.HardBounced, rec.SoftBounced, rec.EngagementScore, rec.Opens, rec.Clicks,
		rec.LastOpenAt, rec.LastClickAt, rec.LastSentAt, rec.SentCount, rec.FailedCount,
		rec.UpdatedBy, time.Now(),
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("recipient not found")
	}
	return nil
}

func (r *RecipientRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM recipients WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("recipient not found")
	}
	return nil
}

func (r *RecipientRepository) List(ctx context.Context, filter *RecipientFilter) ([]*Recipient, int, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if len(filter.IDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.IDs))
		argPos++
	}

	if len(filter.ListIDs) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("list_id = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.ListIDs))
		argPos++
	}

	if len(filter.Emails) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("email = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.Emails))
		argPos++
	}

	if len(filter.Domains) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("domain = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.Domains))
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

	if filter.SubscribedOnly {
		whereClauses = append(whereClauses, "unsubscribed = false")
	}

	if filter.UnsubscribedOnly {
		whereClauses = append(whereClauses, "unsubscribed = true")
	}

	if filter.ValidOnly {
		whereClauses = append(whereClauses, "is_valid = true")
	}

	if filter.InvalidOnly {
		whereClauses = append(whereClauses, "is_valid = false")
	}

	if filter.HasBounces {
		whereClauses = append(whereClauses, "bounce_count > 0 OR hard_bounced = true OR soft_bounced = true")
	}

	if filter.HasComplaints {
		whereClauses = append(whereClauses, "complaint_count > 0")
	}

	if filter.MinEngagement != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("engagement_score >= $%d", argPos))
		args = append(args, *filter.MinEngagement)
		argPos++
	}

	if filter.MaxEngagement != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("engagement_score <= $%d", argPos))
		args = append(args, *filter.MaxEngagement)
		argPos++
	}

	if filter.CreatedAfter != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argPos))
		args = append(args, *filter.CreatedAfter)
		argPos++
	}

	if filter.CreatedBefore != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at <= $%d", argPos))
		args = append(args, *filter.CreatedBefore)
		argPos++
	}

	if filter.UpdatedAfter != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("updated_at >= $%d", argPos))
		args = append(args, *filter.UpdatedAfter)
		argPos++
	}

	if filter.UpdatedBefore != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("updated_at <= $%d", argPos))
		args = append(args, *filter.UpdatedBefore)
		argPos++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(email ILIKE $%d OR name ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+filter.Search+"%")
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM recipients WHERE %s", whereClause)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
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

	limit := 100
	if filter.Limit > 0 {
		limit = filter.Limit
	}
	offset := 0
	if filter.Offset > 0 {
		offset = filter.Offset
	}

	query := fmt.Sprintf(`
		SELECT id, list_id, email, name, first_name, last_name, domain, status, source,
			tags, attributes, metadata, is_valid, validation_error, validation_at,
			valid_mx, has_dns, bounce_count, complaint_count, last_bounce_at,
			last_complaint_at, unsubscribed, unsubscribed_at, unsubscribe_reason,
			unsubscribe_token, hard_bounced, soft_bounced, engagement_score,
			opens, clicks, last_open_at, last_click_at, last_sent_at,
			sent_count, failed_count, created_at, updated_at, created_by, updated_by
		FROM recipients
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

	recipients := []*Recipient{}
	for rows.Next() {
		rec := &Recipient{}
		var attrsJSON, metaJSON []byte

		err := rows.Scan(
			&rec.ID, &rec.ListID, &rec.Email, &rec.Name, &rec.FirstName, &rec.LastName,
			&rec.Domain, &rec.Status, &rec.Source, pq.Array(&rec.Tags), &attrsJSON,
			&metaJSON, &rec.IsValid, &rec.ValidationError, &rec.ValidationAt,
			&rec.ValidMX, &rec.HasDNS, &rec.BounceCount, &rec.ComplaintCount,
			&rec.LastBounceAt, &rec.LastComplaintAt, &rec.Unsubscribed, &rec.UnsubscribedAt,
			&rec.UnsubscribeReason, &rec.UnsubscribeToken, &rec.HardBounced, &rec.SoftBounced,
			&rec.EngagementScore, &rec.Opens, &rec.Clicks, &rec.LastOpenAt, &rec.LastClickAt,
			&rec.LastSentAt, &rec.SentCount, &rec.FailedCount, &rec.CreatedAt, &rec.UpdatedAt,
			&rec.CreatedBy, &rec.UpdatedBy,
		)
		if err != nil {
			return nil, 0, err
		}

		if len(attrsJSON) > 0 {
			json.Unmarshal(attrsJSON, &rec.Attributes)
		}
		if len(metaJSON) > 0 {
			json.Unmarshal(metaJSON, &rec.Metadata)
		}

		recipients = append(recipients, rec)
	}

	return recipients, total, nil
}

func (r *RecipientRepository) BulkInsert(ctx context.Context, recs []*Recipient) error {
	if len(recs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	baseQuery := `
		INSERT INTO recipients (
			id, list_id, email, name, first_name, last_name, domain, status, source,
			tags, attributes, metadata, is_valid, validation_error, validation_at,
			valid_mx, has_dns, bounce_count, complaint_count, unsubscribed,
			unsubscribed_at, unsubscribe_reason, unsubscribe_token,
			hard_bounced, soft_bounced, engagement_score,
			created_by, created_at, updated_at
		) VALUES `

	args := []interface{}{}
	valueStrings := []string{}
	argPos := 1
	now := time.Now()

	for _, rec := range recs {
		attrsJSON, _ := json.Marshal(rec.Attributes)
		metaJSON, _ := json.Marshal(rec.Metadata)

		valueStrings = append(valueStrings, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			argPos, argPos+1, argPos+2, argPos+3, argPos+4, argPos+5, argPos+6, argPos+7, argPos+8,
			argPos+9, argPos+10, argPos+11, argPos+12, argPos+13, argPos+14, argPos+15, argPos+16,
			argPos+17, argPos+18, argPos+19, argPos+20, argPos+21, argPos+22, argPos+23, argPos+24,
			argPos+25, argPos+26, argPos+27, argPos+28,
		))

		args = append(args,
			rec.ID, rec.ListID, rec.Email, rec.Name, rec.FirstName, rec.LastName,
			rec.Domain, rec.Status, rec.Source, pq.Array(rec.Tags), attrsJSON, metaJSON,
			rec.IsValid, rec.ValidationError, rec.ValidationAt, rec.ValidMX, rec.HasDNS,
			rec.BounceCount, rec.ComplaintCount, rec.Unsubscribed, rec.UnsubscribedAt,
			rec.UnsubscribeReason, rec.UnsubscribeToken, rec.HardBounced, rec.SoftBounced,
			rec.EngagementScore, rec.CreatedBy, now, now,
		)
		argPos += 29
	}

	query := baseQuery + strings.Join(valueStrings, ",")

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *RecipientRepository) BulkDeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	query := `DELETE FROM recipients WHERE id = ANY($1)`
	_, err := r.db.ExecContext(ctx, query, pq.Array(ids))
	return err
}

func (r *RecipientRepository) BulkDeleteByList(ctx context.Context, listID string) error {
	query := `DELETE FROM recipients WHERE list_id = $1`
	_, err := r.db.ExecContext(ctx, query, listID)
	return err
}

func (r *RecipientRepository) DeleteFirstN(ctx context.Context, listID string, n int) error {
	query := `
		DELETE FROM recipients
		WHERE id IN (
			SELECT id FROM recipients
			WHERE list_id = $1
			ORDER BY created_at ASC
			LIMIT $2
		)`
	_, err := r.db.ExecContext(ctx, query, listID, n)
	return err
}

func (r *RecipientRepository) DeleteLastN(ctx context.Context, listID string, n int) error {
	query := `
		DELETE FROM recipients
		WHERE id IN (
			SELECT id FROM recipients
			WHERE list_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		)`
	_, err := r.db.ExecContext(ctx, query, listID, n)
	return err
}

func (r *RecipientRepository) DeleteBeforeEmail(ctx context.Context, listID, email string) error {
	query := `
		DELETE FROM recipients
		WHERE list_id = $1
			AND created_at < (
				SELECT created_at FROM recipients
				WHERE list_id = $1 AND email = $2
				LIMIT 1
			)`
	_, err := r.db.ExecContext(ctx, query, listID, email)
	return err
}

func (r *RecipientRepository) DeleteAfterEmail(ctx context.Context, listID, email string) error {
	query := `
		DELETE FROM recipients
		WHERE list_id = $1
			AND created_at > (
				SELECT created_at FROM recipients
				WHERE list_id = $1 AND email = $2
				LIMIT 1
			)`
	_, err := r.db.ExecContext(ctx, query, listID, email)
	return err
}

func (r *RecipientRepository) MarkUnsubscribed(ctx context.Context, listID, email, reason string) error {
	query := `
		UPDATE recipients SET
			unsubscribed = true,
			unsubscribed_at = $4,
			unsubscribe_reason = $3,
			status = 'unsubscribed',
			updated_at = $4
		WHERE list_id = $1 AND email = $2`
	_, err := r.db.ExecContext(ctx, query, listID, email, reason, time.Now())
	return err
}

func (r *RecipientRepository) MarkSubscribed(ctx context.Context, listID, email string) error {
	query := `
		UPDATE recipients SET
			unsubscribed = false,
			unsubscribed_at = NULL,
			unsubscribe_reason = '',
			status = 'active',
			updated_at = $3
		WHERE list_id = $1 AND email = $2`
	_, err := r.db.ExecContext(ctx, query, listID, email, time.Now())
	return err
}

func (r *RecipientRepository) IncrementSent(ctx context.Context, id string) error {
	query := `
		UPDATE recipients SET
			sent_count = sent_count + 1,
			last_sent_at = $2,
			updated_at = $2
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *RecipientRepository) IncrementFailed(ctx context.Context, id string) error {
	query := `
		UPDATE recipients SET
			failed_count = failed_count + 1,
			updated_at = $2
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *RecipientRepository) RegisterOpen(ctx context.Context, id string) error {
	query := `
		UPDATE recipients SET
			opens = opens + 1,
			last_open_at = $2,
			engagement_score = LEAST(100, engagement_score + 1),
			updated_at = $2
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *RecipientRepository) RegisterClick(ctx context.Context, id string) error {
	query := `
		UPDATE recipients SET
			clicks = clicks + 1,
			last_click_at = $2,
			engagement_score = LEAST(100, engagement_score + 2),
			updated_at = $2
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *RecipientRepository) RegisterBounce(ctx context.Context, id string, hard bool) error {
	query := `
		UPDATE recipients SET
			bounce_count = bounce_count + 1,
			hard_bounced = hard_bounced OR $2,
			soft_bounced = soft_bounced OR NOT $2,
			last_bounce_at = $3,
			status = CASE WHEN $2 THEN 'bounced' ELSE status END,
			updated_at = $3
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, hard, time.Now())
	return err
}

func (r *RecipientRepository) RegisterComplaint(ctx context.Context, id string) error {
	query := `
		UPDATE recipients SET
			complaint_count = complaint_count + 1,
			last_complaint_at = $2,
			status = 'complained',
			unsubscribed = true,
			unsubscribed_at = $2,
			updated_at = $2
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *RecipientRepository) GetStats(ctx context.Context, listID string) (*RecipientStats, error) {
	stats := &RecipientStats{
		DomainBreakdown: make(map[string]int),
		StatusBreakdown: make(map[string]int),
		TagTop:          make(map[string]int),
	}

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE is_valid = true) as valid,
			COUNT(*) FILTER (WHERE is_valid = false) as invalid,
			COUNT(*) FILTER (WHERE unsubscribed = false) as subscribed,
			COUNT(*) FILTER (WHERE unsubscribed = true) as unsubscribed,
			COUNT(*) FILTER (WHERE hard_bounced = true) as hard_bounced,
			COUNT(*) FILTER (WHERE soft_bounced = true) as soft_bounced,
			COUNT(*) FILTER (WHERE complaint_count > 0) as with_complaints,
			COALESCE(SUM(sent_count), 0) as total_sent,
			COALESCE(SUM(failed_count), 0) as total_failed,
			COALESCE(AVG(engagement_score), 0) as avg_engagement
		FROM recipients
		WHERE list_id = $1`

	err := r.db.QueryRowContext(ctx, query, listID).Scan(
		&stats.TotalRecipients,
		&stats.ValidRecipients,
		&stats.InvalidRecipients,
		&stats.Subscribed,
		&stats.Unsubscribed,
		&stats.HardBounced,
		&stats.SoftBounced,
		&stats.WithComplaints,
		&stats.TotalSent,
		&stats.TotalFailed,
		&stats.AverageEngagement,
	)
	if err != nil {
		return nil, err
	}

	domainQuery := `
		SELECT domain, COUNT(*)
		FROM recipients
		WHERE list_id = $1
		GROUP BY domain`

	domainRows, err := r.db.QueryContext(ctx, domainQuery, listID)
	if err == nil {
		defer domainRows.Close()
		for domainRows.Next() {
			var domain string
			var count int
			if err := domainRows.Scan(&domain, &count); err == nil {
				stats.DomainBreakdown[domain] = count
			}
		}
	}

	statusQuery := `
		SELECT status, COUNT(*)
		FROM recipients
		WHERE list_id = $1
		GROUP BY status`

	statusRows, err := r.db.QueryContext(ctx, statusQuery, listID)
	if err == nil {
		defer statusRows.Close()
		for statusRows.Next() {
			var status string
			var count int
			if err := statusRows.Scan(&status, &count); err == nil {
				stats.StatusBreakdown[status] = count
			}
		}
	}

	tagQuery := `
		SELECT unnest(tags) as tag, COUNT(*)
		FROM recipients
		WHERE list_id = $1
		GROUP BY tag
		ORDER BY COUNT(*) DESC
		LIMIT 100`

	tagRows, err := r.db.QueryContext(ctx, tagQuery, listID)
	if err == nil {
		defer tagRows.Close()
		for tagRows.Next() {
			var tag string
			var count int
			if err := tagRows.Scan(&tag, &count); err == nil {
				stats.TagTop[tag] = count
			}
		}
	}

	return stats, nil
}
