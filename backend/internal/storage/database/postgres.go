package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

type PostgresDB struct {
	db     *sql.DB
	config *PostgresConfig
	mu     sync.RWMutex
	stats  *DBStats
}

type PostgresConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	ConnTimeout     time.Duration
	QueryTimeout    time.Duration
	EnableStats     bool
}

type DBStats struct {
	TotalQueries      int64
	FailedQueries     int64
	TotalTransactions int64
	FailedTransactions int64
	LastQueryTime     time.Time
	mu                sync.Mutex
}

type QueryResult struct {
	RowsAffected int64
	LastInsertID int64
	Error        error
}

func NewPostgresDB(config *PostgresConfig) (*PostgresDB, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	dsn := buildDSN(config)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	pgDB := &PostgresDB{
		db:     db,
		config: config,
		stats:  &DBStats{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.ConnTimeout)
	defer cancel()

	if err := pgDB.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pgDB, nil
}

func DefaultPostgresConfig() *PostgresConfig {
	return &PostgresConfig{
		Host:            "localhost",
		Port:            5432,
		User:            "postgres",
		Password:        "",
		Database:        "emailcampaign",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
		ConnTimeout:     10 * time.Second,
		QueryTimeout:    30 * time.Second,
		EnableStats:     true,
	}
}

func (c *PostgresConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.User == "" {
		return fmt.Errorf("user is required")
	}
	if c.Database == "" {
		return fmt.Errorf("database is required")
	}
	if c.MaxOpenConns <= 0 {
		return fmt.Errorf("max open connections must be positive")
	}
	if c.MaxIdleConns < 0 {
		return fmt.Errorf("max idle connections cannot be negative")
	}
	if c.MaxIdleConns > c.MaxOpenConns {
		return fmt.Errorf("max idle connections cannot exceed max open connections")
	}
	return nil
}

func buildDSN(config *PostgresConfig) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host,
		config.Port,
		config.User,
		config.Password,
		config.Database,
		config.SSLMode,
	)
}

func (pg *PostgresDB) Ping(ctx context.Context) error {
	return pg.db.PingContext(ctx)
}

func (pg *PostgresDB) Close() error {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	if pg.db != nil {
		return pg.db.Close()
	}
	return nil
}

func (pg *PostgresDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if pg.config.EnableStats {
		pg.incrementQueryStats()
	}

	queryCtx, cancel := context.WithTimeout(ctx, pg.config.QueryTimeout)
	defer cancel()

	rows, err := pg.db.QueryContext(queryCtx, query, args...)
	if err != nil {
		if pg.config.EnableStats {
			pg.incrementFailedQueryStats()
		}
		return nil, fmt.Errorf("query failed: %w", err)
	}

	return rows, nil
}

func (pg *PostgresDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if pg.config.EnableStats {
		pg.incrementQueryStats()
	}

	queryCtx, cancel := context.WithTimeout(ctx, pg.config.QueryTimeout)
	defer cancel()

	return pg.db.QueryRowContext(queryCtx, query, args...)
}

func (pg *PostgresDB) Exec(ctx context.Context, query string, args ...interface{}) (*QueryResult, error) {
	if pg.config.EnableStats {
		pg.incrementQueryStats()
	}

	queryCtx, cancel := context.WithTimeout(ctx, pg.config.QueryTimeout)
	defer cancel()

	result, err := pg.db.ExecContext(queryCtx, query, args...)
	if err != nil {
		if pg.config.EnableStats {
			pg.incrementFailedQueryStats()
		}
		return nil, fmt.Errorf("exec failed: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	lastInsertID, _ := result.LastInsertId()

	return &QueryResult{
		RowsAffected: rowsAffected,
		LastInsertID: lastInsertID,
		Error:        nil,
	}, nil
}

func (pg *PostgresDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if pg.config.EnableStats {
		pg.incrementTransactionStats()
	}

	tx, err := pg.db.BeginTx(ctx, opts)
	if err != nil {
		if pg.config.EnableStats {
			pg.incrementFailedTransactionStats()
		}
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	return tx, nil
}

func (pg *PostgresDB) GetDB() *sql.DB {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	return pg.db
}

func (pg *PostgresDB) GetStats() sql.DBStats {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	return pg.db.Stats()
}

func (pg *PostgresDB) GetQueryStats() *DBStats {
	pg.stats.mu.Lock()
	defer pg.stats.mu.Unlock()

	return &DBStats{
		TotalQueries:       pg.stats.TotalQueries,
		FailedQueries:      pg.stats.FailedQueries,
		TotalTransactions:  pg.stats.TotalTransactions,
		FailedTransactions: pg.stats.FailedTransactions,
		LastQueryTime:      pg.stats.LastQueryTime,
	}
}

func (pg *PostgresDB) ResetStats() {
	pg.stats.mu.Lock()
	defer pg.stats.mu.Unlock()

	pg.stats.TotalQueries = 0
	pg.stats.FailedQueries = 0
	pg.stats.TotalTransactions = 0
	pg.stats.FailedTransactions = 0
}

func (pg *PostgresDB) IsHealthy(ctx context.Context) bool {
	if err := pg.Ping(ctx); err != nil {
		return false
	}

	stats := pg.GetStats()
	if stats.OpenConnections >= pg.config.MaxOpenConns {
		return false
	}

	return true
}

func (pg *PostgresDB) incrementQueryStats() {
	pg.stats.mu.Lock()
	defer pg.stats.mu.Unlock()
	pg.stats.TotalQueries++
	pg.stats.LastQueryTime = time.Now()
}

func (pg *PostgresDB) incrementFailedQueryStats() {
	pg.stats.mu.Lock()
	defer pg.stats.mu.Unlock()
	pg.stats.FailedQueries++
}

func (pg *PostgresDB) incrementTransactionStats() {
	pg.stats.mu.Lock()
	defer pg.stats.mu.Unlock()
	pg.stats.TotalTransactions++
}

func (pg *PostgresDB) incrementFailedTransactionStats() {
	pg.stats.mu.Lock()
	defer pg.stats.mu.Unlock()
	pg.stats.FailedTransactions++
}

func (pg *PostgresDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return pg.db.PrepareContext(ctx, query)
}

func (pg *PostgresDB) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := pg.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx err: %v, rb err: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (pg *PostgresDB) BulkInsert(ctx context.Context, table string, columns []string, values [][]interface{}) error {
	if len(values) == 0 {
		return nil
	}

	return pg.WithTransaction(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, buildBulkInsertQuery(table, columns, len(values[0])))
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, row := range values {
			if _, err := stmt.ExecContext(ctx, row...); err != nil {
				return fmt.Errorf("failed to insert row: %w", err)
			}
		}

		return nil
	})
}

func buildBulkInsertQuery(table string, columns []string, valueCount int) string {
	query := fmt.Sprintf("INSERT INTO %s (", table)

	for i, col := range columns {
		if i > 0 {
			query += ", "
		}
		query += col
	}

	query += ") VALUES ("

	for i := 0; i < valueCount; i++ {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("$%d", i+1)
	}

	query += ")"

	return query
}

func (pg *PostgresDB) TableExists(ctx context.Context, tableName string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		)
	`

	var exists bool
	err := pg.QueryRow(ctx, query, tableName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}

	return exists, nil
}

func (pg *PostgresDB) GetTableRowCount(ctx context.Context, tableName string) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)

	var count int64
	err := pg.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get row count: %w", err)
	}

	return count, nil
}

func (pg *PostgresDB) TruncateTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", tableName)

	_, err := pg.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to truncate table: %w", err)
	}

	return nil
}

func (pg *PostgresDB) GetConfig() *PostgresConfig {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	return pg.config
}

func (pg *PostgresDB) SetMaxOpenConns(n int) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.config.MaxOpenConns = n
	pg.db.SetMaxOpenConns(n)
}

func (pg *PostgresDB) SetMaxIdleConns(n int) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.config.MaxIdleConns = n
	pg.db.SetMaxIdleConns(n)
}

func (pg *PostgresDB) SetConnMaxLifetime(d time.Duration) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.config.ConnMaxLifetime = d
	pg.db.SetConnMaxLifetime(d)
}

func (pg *PostgresDB) SetConnMaxIdleTime(d time.Duration) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.config.ConnMaxIdleTime = d
	pg.db.SetConnMaxIdleTime(d)
}

