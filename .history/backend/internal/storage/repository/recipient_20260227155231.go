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

// Only the columns that actually exist in the recipients table
type Recipient struct {
	ID        string                 `json:"id"`
	ListID    string                 `json:"list_id"`
	Email     string                 `json:"email"`
	Name      string                 `json:"name"`
	FirstName string                 `json:"first_name"`
	LastName  string                 `json:"last_name"`
	Status    string                 `json:"status"`
	Metadata  map[string]interface{} `json:"metadata"`
	SentAt    *time.Time             `json:"sent_at"`
	FailedAt  *time.Time             `json:"failed_at"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type RecipientFilter struct {
	IDs       []string
	ListIDs   []string
	Emails    []string
	Status    []string
	Search    string
	SortBy    string
	SortOrder string
	Limit     int
	Offset    int
}

// Kept for backward compat — manager uses this type
type RecipientStats struct {
	TotalRecipients int            `json:"total_recipients"`
	StatusBreakdown map[string]int `json:"status_breakdown"`
}

func NewRecipientRepository(db *sql.DB) *RecipientRepository {
	return &RecipientRepository{db: db}
}

func (r *RecipientRepository) Create(ctx context.Context, rec *Recipient) error {
	fmt.Printf("🟢 DEBUG Repo.Create: list_id=%s email=%s\n", rec.ListID, rec.Email)

	metaJSON, _ := json.Marshal(rec.Metadata)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}

	now := time.Now()
	query := `
		INSERT INTO recipients
			(list_id, email, name, first_name, last_name, status, metadata, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, query,
		rec.ListID,    // $1
		rec.Email,     // $2
		rec.Name,      // $3
		rec.FirstName, // $4
		rec.LastName,  // $5
		rec.Status,    // $6
		metaJSON,      // $7
		now,           // $8
		now,           // $9
	).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		fmt.Printf("🔴 DEBUG Repo.Create ERROR: %v\n", err)
		return err
	}

	fmt.Printf("🟢 DEBUG Repo.Create SUCCESS id=%s\n", rec.ID)
	return nil
}

func (r *RecipientRepository) GetByID(ctx context.Context, id string) (*Recipient, error) {
	query := `
		SELECT id, list_id, email, COALESCE(name,''), COALESCE(first_name,''), COALESCE(last_name,''),
		       status, COALESCE(metadata,'{}'), sent_at, failed_at, created_at, updated_at
		FROM recipients
		WHERE id = $1`

	rec := &Recipient{}
	var metaJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&rec.ID, &rec.ListID, &rec.Email, &rec.Name, &rec.FirstName, &rec.LastName,
		&rec.Status, &metaJSON, &rec.SentAt, &rec.FailedAt, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("recipient not found")
		}
		return nil, err
	}

	if len(metaJSON) > 0 {
		json.Unmarshal(metaJSON, &rec.Metadata)
	}

	return rec, nil
}

func (r *RecipientRepository) GetByEmail(ctx context.Context, listID, email string) (*Recipient, error) {
	query := `
		SELECT id, list_id, email, COALESCE(name,''), COALESCE(first_name,''), COALESCE(last_name,''),
		       status, COALESCE(metadata,'{}'), sent_at, failed_at, created_at, updated_at
		FROM recipients
		WHERE list_id = $1 AND email = $2
		LIMIT 1`

	rec := &Recipient{}
	var metaJSON []byte

	err := r.db.QueryRowContext(ctx, query, listID, email).Scan(
		&rec.ID, &rec.ListID, &rec.Email, &rec.Name, &rec.FirstName, &rec.LastName,
		&rec.Status, &metaJSON, &rec.SentAt, &rec.FailedAt, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("recipient not found")
		}
		return nil, err
	}

	if len(metaJSON) > 0 {
		json.Unmarshal(metaJSON, &rec.Metadata)
	}

	return rec, nil
}

func (r *RecipientRepository) Update(ctx context.Context, rec *Recipient) error {
	metaJSON, _ := json.Marshal(rec.Metadata)

	query := `
		UPDATE recipients SET
			list_id    = $2,
			email      = $3,
			name       = $4,
			first_name = $5,
			last_name  = $6,
			status     = $7,
			metadata   = $8,
			sent_at    = $9,
			failed_at  = $10,
			updated_at = $11
		WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query,
		rec.ID, rec.ListID, rec.Email, rec.Name, rec.FirstName, rec.LastName,
		rec.Status, metaJSON, rec.SentAt, rec.FailedAt, time.Now(),
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
	result, err := r.db.ExecContext(ctx, `DELETE FROM recipients WHERE id = $1`, id)
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
		if len(filter.Emails) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("email = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.Emails))
			argPos++
		}
		if len(filter.Status) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("status = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.Status))
			argPos++
		}
		if filter.Search != "" {
			whereClauses = append(whereClauses, fmt.Sprintf(
				"(email ILIKE $%d OR first_name ILIKE $%d OR last_name ILIKE $%d)",
				argPos, argPos, argPos))
			args = append(args, "%"+filter.Search+"%")
			argPos++
		}
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// Count
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM recipients WHERE %s", whereSQL)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		fmt.Printf("🔴 DEBUG Repo.List COUNT ERROR: %v\n", err)
		return nil, 0, err
	}

	// Sort
	sortBy := "created_at"
	sortOrder := "DESC"
	if filter != nil {
		if filter.SortBy != "" {
			sortBy = filter.SortBy
		}
		if filter.SortOrder != "" {
			sortOrder = strings.ToUpper(filter.SortOrder)
		}
	}

	limit := 100
	offset := 0
	if filter != nil {
		if filter.Limit > 0 {
			limit = filter.Limit
		}
		if filter.Offset > 0 {
			offset = filter.Offset
		}
	}

	query := fmt.Sprintf(`
		SELECT id, list_id, email,
		       COALESCE(name,''), COALESCE(first_name,''), COALESCE(last_name,''),
		       status, COALESCE(metadata,'{}'), sent_at, failed_at, created_at, updated_at
		FROM recipients
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d`,
		whereSQL, sortBy, sortOrder, argPos, argPos+1)

	args = append(args, limit, offset)

	fmt.Printf("🟢 DEBUG Repo.List: whereSQL=%s args=%v\n", whereSQL, args)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		fmt.Printf("🔴 DEBUG Repo.List QUERY ERROR: %v\n", err)
		return nil, 0, err
	}
	defer rows.Close()

	var recipients []*Recipient
	for rows.Next() {
		rec := &Recipient{}
		var metaJSON []byte

		err := rows.Scan(
			&rec.ID, &rec.ListID, &rec.Email, &rec.Name, &rec.FirstName, &rec.LastName,
			&rec.Status, &metaJSON, &rec.SentAt, &rec.FailedAt, &rec.CreatedAt, &rec.UpdatedAt,
		)
		if err != nil {
			fmt.Printf("🔴 DEBUG Repo.List SCAN ERROR: %v\n", err)
			return nil, 0, err
		}

		if len(metaJSON) > 0 {
			json.Unmarshal(metaJSON, &rec.Metadata)
		}

		recipients = append(recipients, rec)
	}

	fmt.Printf("🟢 DEBUG Repo.List SUCCESS: found=%d total=%d\n", len(recipients), total)
	return recipients, total, nil
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
		if len(filter.Emails) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("email = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.Emails))
			argPos++
		}
		if len(filter.Status) > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("status = ANY($%d)", argPos))
			args = append(args, pq.Array(filter.Status))
			argPos++
		}
	}

	whereSQL := strings.Join(whereClauses, " AND ")
	query := fmt.Sprintf("SELECT COUNT(*) FROM recipients WHERE %s", whereSQL)

	var count int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count recipients: %w", err)
	}
	return count, nil
}

func (r *RecipientRepository) BulkDeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM recipients WHERE id = ANY($1)`, pq.Array(ids))
	return err
}

func (r *RecipientRepository) BulkDeleteByList(ctx context.Context, listID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM recipients WHERE list_id = $1`, listID)
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

func (r *RecipientRepository) GetStats(ctx context.Context, listID string) (*RecipientStats, error) {
	stats := &RecipientStats{
		StatusBreakdown: make(map[string]int),
	}

	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recipients WHERE list_id = $1`, listID,
	).Scan(&stats.TotalRecipients)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM recipients WHERE list_id = $1 GROUP BY status`, listID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			if rows.Scan(&status, &count) == nil {
				stats.StatusBreakdown[status] = count
			}
		}
	}

	return stats, nil
}
