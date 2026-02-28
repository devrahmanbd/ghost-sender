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

type AccountRepository struct {
	db *sql.DB
}

type Account struct {
	ID                  string                 `json:"id" db:"id"`
	Name                string                 `json:"name" db:"name"`
	Email               string                 `json:"email" db:"email"`
	Provider            string                 `json:"provider" db:"provider"`
	Status              string                 `json:"status" db:"status"`
	HealthScore         float64                `json:"health_score" db:"health_score"`
	IsSuspended         bool                   `json:"is_suspended" db:"is_suspended"`
	SuspendedAt         *time.Time             `json:"suspended_at" db:"suspended_at"`
	SuspensionReason    string                 `json:"suspension_reason" db:"suspension_reason"`
	AutoSuspend         bool                   `json:"auto_suspend" db:"-"`
	Credentials         map[string]interface{} `json:"credentials" db:"-"`
	EncryptedPassword   []byte                 `json:"encrypted_password" db:"encrypted_password"`
	OAuthToken          string                 `json:"oauth_token" db:"oauth_token"`
	OAuthRefreshToken   string                 `json:"oauth_refresh_token" db:"oauth_refresh_token"`
	OAuthExpiry         *time.Time             `json:"oauth_expiry" db:"oauth_expiry"`
	SMTPHost            string                 `json:"smtp_host" db:"smtp_host"`
	SMTPPort            int                    `json:"smtp_port" db:"smtp_port"`
	SMTPUsername        string                 `json:"smtp_username" db:"smtp_username"`
	SMTPUseTLS          bool                   `json:"smtp_use_tls" db:"smtp_use_tls"`
	SMTPUseSSL          bool                   `json:"smtp_use_ssl" db:"smtp_use_ssl"`
	DailyLimit          int                    `json:"daily_limit" db:"daily_limit"`
	RotationLimit       int                    `json:"rotation_limit" db:"rotation_limit"`
	SentToday           int                    `json:"sent_today" db:"sent_today"`
	SentThisRotation    int                    `json:"rotation_sent" db:"rotation_sent"`
	TotalSent           int64                  `json:"total_sent" db:"total_sent"`
	TotalFailed         int64                  `json:"total_failed" db:"total_failed"`
	SuccessRate         float64                `json:"success_rate" db:"success_rate"`
	LastUsedAt          *time.Time             `json:"last_used_at" db:"last_used_at"`
	LastResetAt         *time.Time             `json:"last_reset_at" db:"last_reset"`
	LastHealthCheckAt   *time.Time             `json:"last_health_check_at" db:"last_health_check"`
	ConsecutiveFailures int                    `json:"consecutive_failures" db:"consecutive_failures"`
	MaxConsecutiveFails int                    `json:"max_consecutive_fails" db:"-"`
	CooldownUntil       *time.Time             `json:"cooldown_until" db:"cooldown_until"`
	CooldownDuration    int                    `json:"cooldown_duration" db:"-"`
	Weight              int                    `json:"weight" db:"weight"`
	Priority            int                    `json:"priority" db:"priority"`
	Tags                []string               `json:"tags" db:"-"`
	Config              map[string]interface{} `json:"config" db:"settings"`
	Metadata            map[string]interface{} `json:"metadata" db:"metadata"`
	ProxyID             string                 `json:"proxy_id" db:"proxy_id"`
	UseProxy            bool                   `json:"use_proxy" db:"use_proxy"`
	Notes               string                 `json:"notes" db:"-"`
	CreatedAt           time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at" db:"updated_at"`
	CreatedBy           string                 `json:"created_by" db:"-"`
	UpdatedBy           string                 `json:"updated_by" db:"-"`
	IsActive            bool                   `json:"is_active" db:"is_active"`
	LastErrorMessage    string                 `json:"last_error_message" db:"-"`
	LastErrorAt         *time.Time             `json:"last_error_at" db:"last_failure_at"`
}

type AccountFilter struct {
	IDs             []string
	Provider        []string
	Status          []string
	Tags            []string
	IsSuspended     *bool
	IsActive        *bool
	MinHealthScore  *float64
	MaxHealthScore  *float64
	UseProxy        *bool
	Search          string
	SortBy          string
	SortOrder       string
	Limit           int
	Offset          int
	AvailableOnly   bool
	UnderDailyLimit bool
	NotInCooldown   bool
}


type AccountStatsResult struct {
	TotalAccounts      int            `json:"total_accounts"`
	ActiveAccounts     int            `json:"active_accounts"`
	SuspendedAccounts  int            `json:"suspended_accounts"`
	HealthyAccounts    int            `json:"healthy_accounts"`
	UnhealthyAccounts  int            `json:"unhealthy_accounts"`
	ProviderBreakdown  map[string]int `json:"provider_breakdown"`
	AverageHealthScore float64        `json:"average_health_score"`
	AverageSuccessRate float64        `json:"average_success_rate"`
	TotalSent          int64          `json:"total_sent"`
	TotalFailed        int64          `json:"total_failed"`
	AccountsAtLimit    int            `json:"accounts_at_limit"`
	AccountsInCooldown int            `json:"accounts_in_cooldown"`
}

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) Create(ctx context.Context, account *Account) error {
    configJSON, err := json.Marshal(account.Config)
    if err != nil {
        return fmt.Errorf("failed to marshal config: %w", err)
    }
    metadataJSON, err := json.Marshal(account.Metadata)
    if err != nil {
        return fmt.Errorf("failed to marshal metadata: %w", err)
    }

    now := time.Now()
    if account.CreatedAt.IsZero() {
        account.CreatedAt = now
    }
    if account.UpdatedAt.IsZero() {
        account.UpdatedAt = now
    }

    query := `
        INSERT INTO accounts (
            name, email, provider, status, health_score,
            encrypted_password, oauth_token, oauth_refresh_token, oauth_expiry,
            smtp_host, smtp_port, smtp_username, smtp_use_tls, smtp_use_ssl,
            daily_limit, rotation_limit,
            weight, priority,
            settings, metadata,
            proxy_id, use_proxy, is_active,
            created_at, updated_at
        ) VALUES (
            $1,  $2,  $3,  $4,  $5,
            $6,  $7,  $8,  $9,
            $10, $11, $12, $13, $14,
            $15, $16,
            $17, $18,
            $19, $20,
            $21, $22, $23,
            $24, $25
        )
        RETURNING id, created_at, updated_at`
// NEW — "return" → "row :="
    row := r.db.QueryRowContext(
        ctx, query,
        account.Name,                  // $1
        account.Email,                 // $2
        account.Provider,              // $3
        account.Status,                // $4
        account.HealthScore,           // $5
        []byte(account.EncryptedPassword),     // $6
        account.OAuthToken,            // $7
        account.OAuthRefreshToken,     // $8
        account.OAuthExpiry,           // $9
        account.SMTPHost,              // $10
        account.SMTPPort,              // $11
        account.SMTPUsername,          // $12
        account.SMTPUseTLS,            // $13
        account.SMTPUseSSL,            // $14
        account.DailyLimit,            // $15
        account.RotationLimit,         // $16
        account.Weight,                // $17
        account.Priority,              // $18
        configJSON,                    // $19
        metadataJSON,                  // $20
        func() interface{} {
    if account.ProxyID == "" {
        return nil
    }
    return account.ProxyID
	}(),
        account.UseProxy,              // $22
        account.IsActive,              // $23
        account.CreatedAt,             // $24
        account.UpdatedAt,             // $25
    )
    err = row.Scan(&account.ID, &account.CreatedAt, &account.UpdatedAt)
    if err != nil {
        fmt.Printf("🔴 DB INSERT ERROR: %v\n", err)
    }
    return err
}

func (r *AccountRepository) GetByID(ctx context.Context, id string) (*Account, error) {
	query := `
		SELECT
			id, name, email, provider, status, health_score,
			is_suspended, suspended_at, suspension_reason,
			encrypted_password, oauth_token, oauth_refresh_token, oauth_expiry,
			smtp_host, smtp_port, smtp_username, smtp_use_tls, smtp_use_ssl,
			daily_limit, rotation_limit, sent_today, rotation_sent,
			total_sent, total_failed, success_rate,
			last_used_at, last_reset, last_health_check,
			consecutive_failures,
			cooldown_until,
			weight, priority, settings, metadata,
			proxy_id, use_proxy,
			created_at, updated_at,
			is_active, last_failure_at
		FROM accounts
		WHERE id = $1 AND deleted_at IS NULL`

	account := &Account{}
	var configJSON, metadataJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&account.ID, &account.Name, &account.Email, &account.Provider,
		&account.Status, &account.HealthScore,
		&account.IsSuspended, &account.SuspendedAt, &account.SuspensionReason,
		&account.EncryptedPassword, &account.OAuthToken, &account.OAuthRefreshToken, &account.OAuthExpiry,
		&account.SMTPHost, &account.SMTPPort, &account.SMTPUsername, &account.SMTPUseTLS, &account.SMTPUseSSL,
		&account.DailyLimit, &account.RotationLimit, &account.SentToday, &account.SentThisRotation,
		&account.TotalSent, &account.TotalFailed, &account.SuccessRate,
		&account.LastUsedAt, &account.LastResetAt, &account.LastHealthCheckAt,
		&account.ConsecutiveFailures,
		&account.CooldownUntil,
		&account.Weight, &account.Priority, &configJSON, &metadataJSON,
		&account.ProxyID, &account.UseProxy,
		&account.CreatedAt, &account.UpdatedAt,
		&account.IsActive, &account.LastErrorAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("account not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &account.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &account.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return account, nil
}


func (r *AccountRepository) Update(ctx context.Context, account *Account) error {
	query := `
		UPDATE accounts SET
			name             = $2,
			email            = $3,
			provider         = $4,
			status           = $5,
			health_score     = $6,
			encrypted_password = $7,
			oauth_token      = $8,
			smtp_host        = $9,
			smtp_port        = $10,
			smtp_use_tls     = $11,
			smtp_use_ssl     = $12,
			daily_limit      = $13,
			rotation_limit   = $14,
			total_sent       = $15,
			total_failed     = $16,
			updated_at       = $17
		WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query,
		account.ID, account.Name, account.Email, account.Provider,
		account.Status, account.HealthScore,
		account.EncryptedPassword, account.OAuthToken,
		account.SMTPHost, account.SMTPPort, account.SMTPUseTLS, account.SMTPUseSSL,
		account.DailyLimit, account.RotationLimit,
		account.TotalSent, account.TotalFailed,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("account not found")
	}
	return nil
}

func (r *AccountRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("account not found")
	}
	return nil
}

// List returns accounts matching the filter with a total count.
// FIX: argPos is incremented after LIMIT/OFFSET args to stay correct if clauses are added later.
func (r *AccountRepository) List(ctx context.Context, filter *AccountFilter) ([]*Account, int, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter != nil {
		if len(filter.IDs) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.IDs))
			argPos++
		}
		if len(filter.Provider) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("provider = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.Provider))
			argPos++
		}
		if len(filter.Status) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("status = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.Status))
			argPos++
		}
		if filter.IsSuspended != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("is_suspended = $%d", argPos))
			args = append(args, *filter.IsSuspended)
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
		if filter.AvailableOnly {
			whereClauses = append(whereClauses, "is_active = true AND is_suspended = false")
		}
		if filter.UnderDailyLimit {
			whereClauses = append(whereClauses, "sent_today < daily_limit")
		}
		if filter.NotInCooldown {
			whereClauses = append(whereClauses, "(cooldown_until IS NULL OR cooldown_until < NOW())")
		}
		if filter.Search != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR email ILIKE $%d)", argPos, argPos))
			args = append(args, "%"+filter.Search+"%")
			argPos++
		}
	}

	whereClause := strings.Join(whereClauses, " AND ")

	// Count query — uses the same args slice (no LIMIT/OFFSET yet)
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM accounts WHERE %s", whereClause)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count accounts: %w", err)
	}

	// Determine sort
	sortBy := "created_at"
	sortOrder := "DESC"
	if filter != nil && filter.SortBy != "" {
		sortBy = filter.SortBy
	}
	if filter != nil && strings.ToUpper(filter.SortOrder) == "ASC" {
		sortOrder = "ASC"
	}

	// Build data query
	query := fmt.Sprintf(`
		SELECT
			id, name, email, provider, status, health_score,
			is_suspended, suspended_at, suspension_reason,
			encrypted_password, oauth_token,
			smtp_host, smtp_port, smtp_use_tls, smtp_use_ssl,
			daily_limit, rotation_limit, sent_today,
			total_sent, total_failed, success_rate,
			last_used_at, is_active, created_at, updated_at
		FROM accounts
		WHERE %s
		ORDER BY %s %s`,
		whereClause, sortBy, sortOrder)

	if filter != nil && filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argPos, argPos+1)
		args = append(args, filter.Limit, filter.Offset)
		argPos += 2 // FIX: increment so any future appended clauses use correct positions
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*Account
	for rows.Next() {
		account := &Account{}
		if err := rows.Scan(
			&account.ID, &account.Name, &account.Email, &account.Provider,
			&account.Status, &account.HealthScore,
			&account.IsSuspended, &account.SuspendedAt, &account.SuspensionReason,
			&account.EncryptedPassword, &account.OAuthToken,
			&account.SMTPHost, &account.SMTPPort, &account.SMTPUseTLS, &account.SMTPUseSSL,
			&account.DailyLimit, &account.RotationLimit, &account.SentToday,
			&account.TotalSent, &account.TotalFailed, &account.SuccessRate,
			&account.LastUsedAt, &account.IsActive, &account.CreatedAt, &account.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan account row: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("row iteration error: %w", err)
	}

	return accounts, total, nil
}

// ============================================================================
// COUNT METHODS
// ============================================================================

func (r *AccountRepository) Count(ctx context.Context, filter *AccountFilter) (int, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter != nil {
		if len(filter.IDs) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.IDs))
			argPos++
		}
		if len(filter.Provider) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("provider = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.Provider))
			argPos++
		}
		if len(filter.Status) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("status = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.Status))
			argPos++
		}
		if filter.IsSuspended != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("is_suspended = $%d", argPos))
			args = append(args, *filter.IsSuspended)
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
	}

	whereClause := strings.Join(whereClauses, " AND ")
	var count int
	err := r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM accounts WHERE %s", whereClause),
		args...,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count accounts: %w", err)
	}
	return count, nil
}

func (r *AccountRepository) CountActive(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE is_active = true AND is_suspended = false`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active accounts: %w", err)
	}
	return count, nil
}

func (r *AccountRepository) CountSuspended(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE is_suspended = true`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count suspended accounts: %w", err)
	}
	return count, nil
}

func (r *AccountRepository) CountHealthy(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE health_score >= 70 AND is_active = true`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count healthy accounts: %w", err)
	}
	return count, nil
}

func (r *AccountRepository) CountUnhealthy(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE health_score < 70 AND is_active = true`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count unhealthy accounts: %w", err)
	}
	return count, nil
}

func (r *AccountRepository) CountByProvider(ctx context.Context, provider string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE provider = $1 AND is_active = true`, provider,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count accounts by provider: %w", err)
	}
	return count, nil
}

func (r *AccountRepository) CountAtLimit(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE sent_today >= daily_limit AND is_active = true`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count accounts at limit: %w", err)
	}
	return count, nil
}

func (r *AccountRepository) CountInCooldown(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE cooldown_until > NOW() AND is_active = true`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count accounts in cooldown: %w", err)
	}
	return count, nil
}

func (r *AccountRepository) CountAvailable(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM accounts
		WHERE is_active = true
		  AND is_suspended = false
		  AND sent_today < daily_limit
		  AND (cooldown_until IS NULL OR cooldown_until < NOW())`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count available accounts: %w", err)
	}
	return count, nil
}

// ============================================================================
// AGGREGATE / METRICS HELPERS
// ============================================================================

func (r *AccountRepository) GetTotalEmailsSent(ctx context.Context) (int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(total_sent), 0) FROM accounts WHERE is_active = true`,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total emails sent: %w", err)
	}
	return total, nil
}

func (r *AccountRepository) GetTotalEmailsFailed(ctx context.Context) (int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(total_failed), 0) FROM accounts WHERE is_active = true`,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total emails failed: %w", err)
	}
	return total, nil
}

func (r *AccountRepository) GetAverageHealthScore(ctx context.Context) (float64, error) {
	var avg float64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(AVG(health_score), 0) FROM accounts WHERE is_active = true`,
	).Scan(&avg)
	if err != nil {
		return 0, fmt.Errorf("failed to get average health score: %w", err)
	}
	return avg, nil
}

func (r *AccountRepository) GetAverageSuccessRate(ctx context.Context) (float64, error) {
	var avg float64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(AVG(success_rate), 0) FROM accounts WHERE is_active = true AND total_sent > 0`,
	).Scan(&avg)
	if err != nil {
		return 0, fmt.Errorf("failed to get average success rate: %w", err)
	}
	return avg, nil
}

func (r *AccountRepository) GetProviderBreakdown(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT provider, COUNT(*) AS count
		FROM accounts
		WHERE is_active = true
		GROUP BY provider`)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider breakdown: %w", err)
	}
	defer rows.Close()

	breakdown := make(map[string]int)
	for rows.Next() {
		var provider string
		var count int
		if err := rows.Scan(&provider, &count); err != nil {
			return nil, fmt.Errorf("failed to scan provider row: %w", err)
		}
		breakdown[provider] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("provider breakdown row error: %w", err)
	}
	return breakdown, nil
}

// GetStats returns aggregated account statistics.
// FIX: GetProviderBreakdown error is no longer silently discarded.
func (r *AccountRepository) GetStats(ctx context.Context) (*AccountStatsResult, error) {
	stats := &AccountStatsResult{
		ProviderBreakdown: make(map[string]int),
	}

	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*)                                          AS total,
			COUNT(*) FILTER (WHERE is_active = true)         AS active,
			COUNT(*) FILTER (WHERE is_suspended = true)      AS suspended,
			COUNT(*) FILTER (WHERE health_score >= 70)       AS healthy,
			COUNT(*) FILTER (WHERE health_score < 70)        AS unhealthy,
			COUNT(*) FILTER (WHERE sent_today >= daily_limit) AS at_limit,
			COUNT(*) FILTER (WHERE cooldown_until > NOW())   AS in_cooldown,
			COALESCE(AVG(health_score), 0)                   AS avg_health,
			COALESCE(AVG(success_rate), 0)                   AS avg_success,
			COALESCE(SUM(total_sent), 0)                     AS total_sent,
			COALESCE(SUM(total_failed), 0)                   AS total_failed
		FROM accounts`,
	).Scan(
		&stats.TotalAccounts,
		&stats.ActiveAccounts,
		&stats.SuspendedAccounts,
		&stats.HealthyAccounts,
		&stats.UnhealthyAccounts,
		&stats.AccountsAtLimit,
		&stats.AccountsInCooldown,
		&stats.AverageHealthScore,
		&stats.AverageSuccessRate,
		&stats.TotalSent,
		&stats.TotalFailed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get account stats: %w", err)
	}

	// FIX: propagate error from GetProviderBreakdown instead of discarding it
	breakdown, err := r.GetProviderBreakdown(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider breakdown for stats: %w", err)
	}
	stats.ProviderBreakdown = breakdown

	return stats, nil
}

func (r *AccountRepository) GetTopPerformingAccounts(ctx context.Context, limit int) ([]*Account, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, email, provider, status, health_score, success_rate,
		       total_sent, total_failed, last_used_at, created_at, updated_at
		FROM accounts
		WHERE is_active = true AND total_sent > 0
		ORDER BY success_rate DESC, total_sent DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top performing accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*Account
	for rows.Next() {
		a := &Account{}
		if err := rows.Scan(
			&a.ID, &a.Name, &a.Email, &a.Provider,
			&a.Status, &a.HealthScore, &a.SuccessRate,
			&a.TotalSent, &a.TotalFailed, &a.LastUsedAt,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan account row: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}
	return accounts, nil
}

func (r *AccountRepository) GetLowHealthAccounts(ctx context.Context, threshold float64, limit int) ([]*Account, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
 	SELECT id, name, email, provider, status, health_score,
        consecutive_failures, last_failure_at
    FROM accounts WHERE is_active = true AND health_score < $1
    ORDER BY health_score ASC, consecutive_failures DESC LIMIT $2`, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get low health accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*Account
	for rows.Next() {
		a := &Account{}
		if err := rows.Scan(
			&a.ID, &a.Name, &a.Email, &a.Provider,
			&a.Status, &a.HealthScore, &a.ConsecutiveFailures,
			&a.LastErrorAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan account row: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}
	return accounts, nil
}

func (r *AccountRepository) GetAccountsNeedingReset(ctx context.Context) ([]*Account, error) {
	rows, err := r.db.QueryContext(ctx, `
  SELECT id, name, email, sent_today, last_reset
    FROM accounts WHERE is_active = true
    AND (last_reset IS NULL OR last_reset < CURRENT_DATE)
    AND sent_today > 0`)
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts needing reset: %w", err)
	}
	defer rows.Close()

	var accounts []*Account
	for rows.Next() {
		a := &Account{}
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.SentToday, &a.LastResetAt); err != nil {
			return nil, fmt.Errorf("failed to scan account row: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}
	return accounts, nil
}

func (r *AccountRepository) GetAccountUsageStats(ctx context.Context) (map[string]interface{}, error) {
	var totalAccounts int
	var totalSentToday, totalCapacity int64
	var avgUtilization float64

	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*)                                                              AS total_accounts,
			COALESCE(SUM(sent_today), 0)                                         AS total_sent_today,
			COALESCE(SUM(daily_limit), 0)                                        AS total_daily_capacity,
			COALESCE(ROUND(AVG(sent_today::numeric / NULLIF(daily_limit,0)*100),2), 0) AS avg_utilization
		FROM accounts
		WHERE is_active = true`,
	).Scan(&totalAccounts, &totalSentToday, &totalCapacity, &avgUtilization)
	if err != nil {
		return nil, fmt.Errorf("failed to get account usage stats: %w", err)
	}

	return map[string]interface{}{
		"total_accounts":       totalAccounts,
		"total_sent_today":     totalSentToday,
		"total_daily_capacity": totalCapacity,
		"avg_utilization_pct":  avgUtilization,
		"remaining_capacity":   totalCapacity - totalSentToday,
	}, nil
}
