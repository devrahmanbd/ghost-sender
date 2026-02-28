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

type ConfigRepository struct {
	db *sql.DB
}

type ConfigEntry struct {
	ID           string      `json:"id" db:"id"`
	Section      string      `json:"section" db:"section"`
	Key          string      `json:"key" db:"key"`
	Value        string      `json:"value" db:"value"`
	Type         string      `json:"type" db:"type"`
	DefaultValue string      `json:"default_value" db:"default_value"`
	Description  string      `json:"description" db:"description"`
	Validation   string      `json:"validation" db:"validation"`
	IsEncrypted  bool        `json:"is_encrypted" db:"is_encrypted"`
	IsSensitive  bool        `json:"is_sensitive" db:"is_sensitive"`
	IsReadOnly   bool        `json:"is_read_only" db:"is_read_only"`
	Tags         []string    `json:"tags" db:"tags"`
	Metadata     interface{} `json:"metadata" db:"metadata"`
	Version      int         `json:"version" db:"version"`
	CreatedAt    time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at" db:"updated_at"`
	UpdatedBy    string      `json:"updated_by" db:"updated_by"`
}

type ConfigBackup struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	ConfigData  string    `json:"config_data" db:"config_data"`
	CreatedBy   string    `json:"created_by" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

type ConfigHistory struct {
	ID           string    `json:"id" db:"id"`
	ConfigID     string    `json:"config_id" db:"config_id"`
	Section      string    `json:"section" db:"section"`
	Key          string    `json:"key" db:"key"`
	OldValue     string    `json:"old_value" db:"old_value"`
	NewValue     string    `json:"new_value" db:"new_value"`
	ChangedBy    string    `json:"changed_by" db:"changed_by"`
	ChangeReason string    `json:"change_reason" db:"change_reason"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type ConfigFilter struct {
	Sections    []string
	Keys        []string
	Tags        []string
	IsSensitive *bool
	IsReadOnly  *bool
	Search      string
}

func NewConfigRepository(db *sql.DB) *ConfigRepository {
	return &ConfigRepository{db: db}
}

func (r *ConfigRepository) Get(ctx context.Context, section, key string) (*ConfigEntry, error) {
	query := `
		SELECT id, section, key, value, type, default_value, description,
			validation, is_encrypted, is_sensitive, is_read_only, tags,
			metadata, version, created_at, updated_at, updated_by
		FROM configs
		WHERE section = $1 AND key = $2`

	entry := &ConfigEntry{}
	var metaJSON []byte

	err := r.db.QueryRowContext(ctx, query, section, key).Scan(
		&entry.ID, &entry.Section, &entry.Key, &entry.Value, &entry.Type,
		&entry.DefaultValue, &entry.Description, &entry.Validation,
		&entry.IsEncrypted, &entry.IsSensitive, &entry.IsReadOnly,
		pq.Array(&entry.Tags), &metaJSON, &entry.Version,
		&entry.CreatedAt, &entry.UpdatedAt, &entry.UpdatedBy,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("config not found: %s.%s", section, key)
		}
		return nil, err
	}

	if len(metaJSON) > 0 {
		json.Unmarshal(metaJSON, &entry.Metadata)
	}

	return entry, nil
}

func (r *ConfigRepository) GetByID(ctx context.Context, id string) (*ConfigEntry, error) {
	query := `
		SELECT id, section, key, value, type, default_value, description,
			validation, is_encrypted, is_sensitive, is_read_only, tags,
			metadata, version, created_at, updated_at, updated_by
		FROM configs
		WHERE id = $1`

	entry := &ConfigEntry{}
	var metaJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&entry.ID, &entry.Section, &entry.Key, &entry.Value, &entry.Type,
		&entry.DefaultValue, &entry.Description, &entry.Validation,
		&entry.IsEncrypted, &entry.IsSensitive, &entry.IsReadOnly,
		pq.Array(&entry.Tags), &metaJSON, &entry.Version,
		&entry.CreatedAt, &entry.UpdatedAt, &entry.UpdatedBy,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("config not found")
		}
		return nil, err
	}

	if len(metaJSON) > 0 {
		json.Unmarshal(metaJSON, &entry.Metadata)
	}

	return entry, nil
}

func (r *ConfigRepository) Set(ctx context.Context, entry *ConfigEntry) error {
	existing, err := r.Get(ctx, entry.Section, entry.Key)
	
	metaJSON, _ := json.Marshal(entry.Metadata)
	now := time.Now()

	if err != nil {
		query := `
			INSERT INTO configs (
				id, section, key, value, type, default_value, description,
				validation, is_encrypted, is_sensitive, is_read_only, tags,
				metadata, version, created_at, updated_at, updated_by
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8, $9, $10, $11, $12,
				$13, $14, $15, $16, $17
			)`

		_, err := r.db.ExecContext(
			ctx, query,
			entry.ID, entry.Section, entry.Key, entry.Value, entry.Type,
			entry.DefaultValue, entry.Description, entry.Validation,
			entry.IsEncrypted, entry.IsSensitive, entry.IsReadOnly,
			pq.Array(entry.Tags), metaJSON, 1, now, now, entry.UpdatedBy,
		)
		return err
	}

	if existing.IsReadOnly {
		return fmt.Errorf("config %s.%s is read-only", entry.Section, entry.Key)
	}

	query := `
		UPDATE configs SET
			value = $3,
			type = $4,
			default_value = $5,
			description = $6,
			validation = $7,
			is_encrypted = $8,
			is_sensitive = $9,
			is_read_only = $10,
			tags = $11,
			metadata = $12,
			version = version + 1,
			updated_at = $13,
			updated_by = $14
		WHERE section = $1 AND key = $2`

	_, err = r.db.ExecContext(
		ctx, query,
		entry.Section, entry.Key, entry.Value, entry.Type,
		entry.DefaultValue, entry.Description, entry.Validation,
		entry.IsEncrypted, entry.IsSensitive, entry.IsReadOnly,
		pq.Array(entry.Tags), metaJSON, now, entry.UpdatedBy,
	)
	if err != nil {
		return err
	}

	historyQuery := `
		INSERT INTO config_history (
			id, config_id, section, key, old_value, new_value,
			changed_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err = r.db.ExecContext(
		ctx, historyQuery,
		generateID(), existing.ID, entry.Section, entry.Key,
		existing.Value, entry.Value, entry.UpdatedBy, now,
	)

	return err
}

func (r *ConfigRepository) List(ctx context.Context, filter *ConfigFilter) ([]*ConfigEntry, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if len(filter.Sections) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("section = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.Sections))
		argPos++
	}

	if len(filter.Keys) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("key = ANY($%d)", argPos))
		args = append(args, pq.Array(filter.Keys))
		argPos++
	}

	if len(filter.Tags) > 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("tags && $%d", argPos))
		args = append(args, pq.Array(filter.Tags))
		argPos++
	}

	if filter.IsSensitive != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("is_sensitive = $%d", argPos))
		args = append(args, *filter.IsSensitive)
		argPos++
	}

	if filter.IsReadOnly != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("is_read_only = $%d", argPos))
		args = append(args, *filter.IsReadOnly)
		argPos++
	}

	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(key ILIKE $%d OR description ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+filter.Search+"%")
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	query := fmt.Sprintf(`
		SELECT id, section, key, value, type, default_value, description,
			validation, is_encrypted, is_sensitive, is_read_only, tags,
			metadata, version, created_at, updated_at, updated_by
		FROM configs
		WHERE %s
		ORDER BY section, key`, whereClause)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*ConfigEntry
	for rows.Next() {
		entry := &ConfigEntry{}
		var metaJSON []byte

		err := rows.Scan(
			&entry.ID, &entry.Section, &entry.Key, &entry.Value, &entry.Type,
			&entry.DefaultValue, &entry.Description, &entry.Validation,
			&entry.IsEncrypted, &entry.IsSensitive, &entry.IsReadOnly,
			pq.Array(&entry.Tags), &metaJSON, &entry.Version,
			&entry.CreatedAt, &entry.UpdatedAt, &entry.UpdatedBy,
		)
		if err != nil {
			return nil, err
		}

		if len(metaJSON) > 0 {
			json.Unmarshal(metaJSON, &entry.Metadata)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func (r *ConfigRepository) GetSection(ctx context.Context, section string) ([]*ConfigEntry, error) {
	filter := &ConfigFilter{Sections: []string{section}}
	return r.List(ctx, filter)
}

func (r *ConfigRepository) GetAll(ctx context.Context) ([]*ConfigEntry, error) {
	return r.List(ctx, &ConfigFilter{})
}

func (r *ConfigRepository) Delete(ctx context.Context, section, key string) error {
	query := `DELETE FROM configs WHERE section = $1 AND key = $2`
	result, err := r.db.ExecContext(ctx, query, section, key)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("config not found")
	}
	return nil
}

func (r *ConfigRepository) CreateBackup(ctx context.Context, backup *ConfigBackup) error {
	configs, err := r.GetAll(ctx)
	if err != nil {
		return err
	}

	data, err := json.Marshal(configs)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO config_backups (
			id, name, description, config_data, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)`

	_, err = r.db.ExecContext(
		ctx, query,
		backup.ID, backup.Name, backup.Description, string(data),
		backup.CreatedBy, time.Now(),
	)
	return err
}

func (r *ConfigRepository) GetBackup(ctx context.Context, id string) (*ConfigBackup, error) {
	query := `
		SELECT id, name, description, config_data, created_by, created_at
		FROM config_backups
		WHERE id = $1`

	backup := &ConfigBackup{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&backup.ID, &backup.Name, &backup.Description,
		&backup.ConfigData, &backup.CreatedBy, &backup.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("backup not found")
		}
		return nil, err
	}

	return backup, nil
}

func (r *ConfigRepository) ListBackups(ctx context.Context, limit int) ([]*ConfigBackup, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, name, description, config_data, created_by, created_at
		FROM config_backups
		ORDER BY created_at DESC
		LIMIT $1`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []*ConfigBackup
	for rows.Next() {
		backup := &ConfigBackup{}
		err := rows.Scan(
			&backup.ID, &backup.Name, &backup.Description,
			&backup.ConfigData, &backup.CreatedBy, &backup.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}

	return backups, nil
}

func (r *ConfigRepository) RestoreBackup(ctx context.Context, backupID, restoredBy string) error {
	backup, err := r.GetBackup(ctx, backupID)
	if err != nil {
		return err
	}

	var configs []*ConfigEntry
	if err := json.Unmarshal([]byte(backup.ConfigData), &configs); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, config := range configs {
		config.UpdatedBy = restoredBy
		if err := r.Set(ctx, config); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *ConfigRepository) DeleteBackup(ctx context.Context, id string) error {
	query := `DELETE FROM config_backups WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("backup not found")
	}
	return nil
}

func (r *ConfigRepository) GetHistory(ctx context.Context, section, key string, limit int) ([]*ConfigHistory, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, config_id, section, key, old_value, new_value,
			changed_by, change_reason, created_at
		FROM config_history
		WHERE section = $1 AND key = $2
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := r.db.QueryContext(ctx, query, section, key, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []*ConfigHistory
	for rows.Next() {
		h := &ConfigHistory{}
		err := rows.Scan(
			&h.ID, &h.ConfigID, &h.Section, &h.Key,
			&h.OldValue, &h.NewValue, &h.ChangedBy,
			&h.ChangeReason, &h.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	return history, nil
}

func (r *ConfigRepository) BulkSet(ctx context.Context, entries []*ConfigEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, entry := range entries {
		if err := r.Set(ctx, entry); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *ConfigRepository) ResetToDefaults(ctx context.Context, section string, resetBy string) error {
	query := `
		UPDATE configs
		SET value = default_value,
			version = version + 1,
			updated_at = $2,
			updated_by = $3
		WHERE section = $1`

	_, err := r.db.ExecContext(ctx, query, section, time.Now(), resetBy)
	return err
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
