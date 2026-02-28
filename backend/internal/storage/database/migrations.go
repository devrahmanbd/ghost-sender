package database

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Migrator struct {
	db             *PostgresDB
	migrationsPath string
	tableName      string
}

type Migration struct {
	Version   int
	Name      string
	UpSQL     string
	DownSQL   string
	AppliedAt time.Time
}

type MigrationRecord struct {
	Version   int
	Name      string
	AppliedAt time.Time
}

func NewMigrator(db *PostgresDB, migrationsPath string) (*Migrator, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	if migrationsPath == "" {
		migrationsPath = "./migrations"
	}

	m := &Migrator{
		db:             db,
		migrationsPath: migrationsPath,
		tableName:      "schema_migrations",
	}

	ctx := context.Background()
	if err := m.createMigrationsTable(ctx); err != nil {
		return nil, fmt.Errorf("failed to create migrations table: %w", err)
	}

	return m, nil
}

func (m *Migrator) createMigrationsTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version INTEGER PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`, m.tableName)

	_, err := m.db.Exec(ctx, query)
	return err
}

func (m *Migrator) Up(ctx context.Context) error {
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	appliedMap := make(map[int]bool)
	for _, version := range applied {
		appliedMap[version] = true
	}

	for _, migration := range migrations {
		if appliedMap[migration.Version] {
			continue
		}

		if err := m.applyMigration(ctx, migration, true); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

func (m *Migrator) Down(ctx context.Context) error {
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	if len(applied) == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	lastVersion := applied[len(applied)-1]

	for i := len(migrations) - 1; i >= 0; i-- {
		if migrations[i].Version == lastVersion {
			if err := m.applyMigration(ctx, migrations[i], false); err != nil {
				return fmt.Errorf("failed to rollback migration %d: %w", migrations[i].Version, err)
			}
			break
		}
	}

	return nil
}

func (m *Migrator) Step(ctx context.Context, steps int) error {
	if steps == 0 {
		return nil
	}

	if steps > 0 {
		return m.stepUp(ctx, steps)
	}

	return m.stepDown(ctx, -steps)
}

func (m *Migrator) stepUp(ctx context.Context, steps int) error {
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	appliedMap := make(map[int]bool)
	for _, version := range applied {
		appliedMap[version] = true
	}

	count := 0
	for _, migration := range migrations {
		if appliedMap[migration.Version] {
			continue
		}

		if err := m.applyMigration(ctx, migration, true); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}

		count++
		if count >= steps {
			break
		}
	}

	return nil
}

func (m *Migrator) stepDown(ctx context.Context, steps int) error {
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	if len(applied) == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	migrationsMap := make(map[int]*Migration)
	for i := range migrations {
		migrationsMap[migrations[i].Version] = &migrations[i]
	}

	count := 0
	for i := len(applied) - 1; i >= 0; i-- {
		version := applied[i]
		migration, exists := migrationsMap[version]
		if !exists {
			continue
		}

		if err := m.applyMigration(ctx, *migration, false); err != nil {
			return fmt.Errorf("failed to rollback migration %d: %w", version, err)
		}

		count++
		if count >= steps {
			break
		}
	}

	return nil
}

func (m *Migrator) Force(ctx context.Context, version int) error {
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	var migration *Migration
	for i := range migrations {
		if migrations[i].Version == version {
			migration = &migrations[i]
			break
		}
	}

	if migration == nil {
		return fmt.Errorf("migration version %d not found", version)
	}

	return m.recordMigration(ctx, migration.Version, migration.Name)
}

func (m *Migrator) Version(ctx context.Context) (int, error) {
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return 0, err
	}

	if len(applied) == 0 {
		return 0, nil
	}

	return applied[len(applied)-1], nil
}

func (m *Migrator) Status(ctx context.Context) ([]MigrationRecord, error) {
	migrations, err := m.loadMigrations()
	if err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}

	applied, err := m.getAppliedMigrationRecords(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	appliedMap := make(map[int]MigrationRecord)
	for _, record := range applied {
		appliedMap[record.Version] = record
	}

	var status []MigrationRecord
	for _, migration := range migrations {
		if record, exists := appliedMap[migration.Version]; exists {
			status = append(status, record)
		} else {
			status = append(status, MigrationRecord{
				Version: migration.Version,
				Name:    migration.Name,
			})
		}
	}

	return status, nil
}

func (m *Migrator) applyMigration(ctx context.Context, migration Migration, up bool) error {
    sql := migration.UpSQL
    if !up {
        sql = migration.DownSQL
    }

    // ✅ Fixed: Use m.db.db (lowercase field name)
    tx, err := m.db.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    if _, err := tx.ExecContext(ctx, sql); err != nil {
        return fmt.Errorf("failed to execute migration: %w", err)
    }

    if up {
        query := fmt.Sprintf("INSERT INTO %s (version, name) VALUES ($1, $2)", m.tableName)
        if _, err := tx.ExecContext(ctx, query, migration.Version, migration.Name); err != nil {
            return fmt.Errorf("failed to record migration: %w", err)
        }
    } else {
        query := fmt.Sprintf("DELETE FROM %s WHERE version = $1", m.tableName)
        if _, err := tx.ExecContext(ctx, query, migration.Version); err != nil {
            return fmt.Errorf("failed to remove migration record: %w", err)
        }
    }

    return tx.Commit()
}


func (m *Migrator) recordMigration(ctx context.Context, version int, name string) error {
	query := fmt.Sprintf("INSERT INTO %s (version, name) VALUES ($1, $2) ON CONFLICT (version) DO NOTHING", m.tableName)
	_, err := m.db.Exec(ctx, query, version, name)
	return err
}

func (m *Migrator) loadMigrations() ([]Migration, error) {
	files, err := ioutil.ReadDir(m.migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	migrationsMap := make(map[int]*Migration)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		parts := strings.Split(name, "_")
		if len(parts) < 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		isUp := strings.HasSuffix(name, ".up.sql")
		isDown := strings.HasSuffix(name, ".down.sql")

		if !isUp && !isDown {
			continue
		}

		content, err := ioutil.ReadFile(filepath.Join(m.migrationsPath, name))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", name, err)
		}

		migrationName := strings.TrimSuffix(strings.TrimSuffix(name, ".up.sql"), ".down.sql")
		migrationName = strings.Join(parts[1:], "_")
		migrationName = strings.TrimSuffix(migrationName, ".up")
		migrationName = strings.TrimSuffix(migrationName, ".down")

		if _, exists := migrationsMap[version]; !exists {
			migrationsMap[version] = &Migration{
				Version: version,
				Name:    migrationName,
			}
		}

		if isUp {
			migrationsMap[version].UpSQL = string(content)
		} else {
			migrationsMap[version].DownSQL = string(content)
		}
	}

	var migrations []Migration
	for _, migration := range migrationsMap {
		migrations = append(migrations, *migration)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func (m *Migrator) getAppliedMigrations(ctx context.Context) ([]int, error) {
	query := fmt.Sprintf("SELECT version FROM %s ORDER BY version ASC", m.tableName)

	rows, err := m.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}

	return versions, rows.Err()
}

func (m *Migrator) getAppliedMigrationRecords(ctx context.Context) ([]MigrationRecord, error) {
	query := fmt.Sprintf("SELECT version, name, applied_at FROM %s ORDER BY version ASC", m.tableName)

	rows, err := m.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []MigrationRecord
	for rows.Next() {
		var record MigrationRecord
		if err := rows.Scan(&record.Version, &record.Name, &record.AppliedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, rows.Err()
}

func (m *Migrator) Reset(ctx context.Context) error {
	for {
		version, err := m.Version(ctx)
		if err != nil {
			return err
		}

		if version == 0 {
			break
		}

		if err := m.Down(ctx); err != nil {
			return err
		}
	}

	return nil
}

