package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type TransactionManager struct {
	db      *PostgresDB
	hooks   []TransactionHook
	options *TransactionOptions
}

type TransactionOptions struct {
	IsolationLevel sql.IsolationLevel
	ReadOnly       bool
	MaxRetries     int
	RetryDelay     time.Duration
	Timeout        time.Duration
}

type TransactionHook interface {
	BeforeBegin(ctx context.Context) error
	AfterCommit(ctx context.Context) error
	AfterRollback(ctx context.Context, err error) error
}

type TransactionFunc func(ctx context.Context, tx *sql.Tx) error

type Savepoint struct {
	name string
	tx   *sql.Tx
}

type TransactionContext struct {
	tx         *sql.Tx
	savepoints []*Savepoint
	startTime  time.Time
	ctx        context.Context
}

func NewTransactionManager(db *PostgresDB) *TransactionManager {
	return &TransactionManager{
		db:      db,
		hooks:   []TransactionHook{},
		options: DefaultTransactionOptions(),
	}
}

func DefaultTransactionOptions() *TransactionOptions {
	return &TransactionOptions{
		IsolationLevel: sql.LevelReadCommitted,
		ReadOnly:       false,
		MaxRetries:     3,
		RetryDelay:     100 * time.Millisecond,
		Timeout:        30 * time.Second,
	}
}

func (tm *TransactionManager) AddHook(hook TransactionHook) {
	tm.hooks = append(tm.hooks, hook)
}

func (tm *TransactionManager) SetOptions(options *TransactionOptions) {
	tm.options = options
}

func (tm *TransactionManager) Execute(ctx context.Context, fn TransactionFunc) error {
	return tm.ExecuteWithOptions(ctx, tm.options, fn)
}

func (tm *TransactionManager) ExecuteWithOptions(ctx context.Context, opts *TransactionOptions, fn TransactionFunc) error {
	var err error
	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(opts.RetryDelay * time.Duration(attempt)):
			}
		}

		err = tm.executeTransaction(ctx, opts, fn)
		if err == nil {
			return nil
		}

		if !tm.isRetryable(err) {
			return err
		}
	}

	return fmt.Errorf("transaction failed after %d attempts: %w", opts.MaxRetries+1, err)
}

func (tm *TransactionManager) executeTransaction(ctx context.Context, opts *TransactionOptions, fn TransactionFunc) error {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	for _, hook := range tm.hooks {
		if err := hook.BeforeBegin(ctx); err != nil {
			return fmt.Errorf("before begin hook failed: %w", err)
		}
	}

	txOpts := &sql.TxOptions{
		Isolation: opts.IsolationLevel,
		ReadOnly:  opts.ReadOnly,
	}

	tx, err := tm.db.BeginTx(ctx, txOpts)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			err = fmt.Errorf("tx error: %v, rollback error: %v", err, rbErr)
		}

		for _, hook := range tm.hooks {
			hook.AfterRollback(ctx, err)
		}

		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	for _, hook := range tm.hooks {
		if err := hook.AfterCommit(ctx); err != nil {
			return fmt.Errorf("after commit hook failed: %w", err)
		}
	}

	return nil
}

func (tm *TransactionManager) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	retryableErrors := []string{
		"deadlock detected",
		"could not serialize access",
		"serialization failure",
		"retry transaction",
	}

	for _, retryErr := range retryableErrors {
		if contains(errStr, retryErr) {
			return true
		}
	}

	return false
}

func (tm *TransactionManager) CreateSavepoint(ctx context.Context, tx *sql.Tx, name string) (*Savepoint, error) {
	query := fmt.Sprintf("SAVEPOINT %s", name)
	if _, err := tx.ExecContext(ctx, query); err != nil {
		return nil, fmt.Errorf("failed to create savepoint: %w", err)
	}

	return &Savepoint{
		name: name,
		tx:   tx,
	}, nil
}

func (tm *TransactionManager) RollbackToSavepoint(ctx context.Context, sp *Savepoint) error {
	query := fmt.Sprintf("ROLLBACK TO SAVEPOINT %s", sp.name)
	if _, err := sp.tx.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to rollback to savepoint: %w", err)
	}
	return nil
}

func (tm *TransactionManager) ReleaseSavepoint(ctx context.Context, sp *Savepoint) error {
	query := fmt.Sprintf("RELEASE SAVEPOINT %s", sp.name)
	if _, err := sp.tx.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to release savepoint: %w", err)
	}
	return nil
}

func (tm *TransactionManager) ExecuteWithSavepoint(ctx context.Context, tx *sql.Tx, name string, fn func() error) error {
	sp, err := tm.CreateSavepoint(ctx, tx, name)
	if err != nil {
		return err
	}

	if err := fn(); err != nil {
		if rbErr := tm.RollbackToSavepoint(ctx, sp); rbErr != nil {
			return fmt.Errorf("fn error: %v, rollback error: %v", err, rbErr)
		}
		return err
	}

	return tm.ReleaseSavepoint(ctx, sp)
}

func NewTransactionContext(ctx context.Context, tx *sql.Tx) *TransactionContext {
	return &TransactionContext{
		tx:         tx,
		savepoints: []*Savepoint{},
		startTime:  time.Now(),
		ctx:        ctx,
	}
}

func (tc *TransactionContext) Tx() *sql.Tx {
	return tc.tx
}

func (tc *TransactionContext) Context() context.Context {
	return tc.ctx
}

func (tc *TransactionContext) Duration() time.Duration {
	return time.Since(tc.startTime)
}

func (tc *TransactionContext) AddSavepoint(sp *Savepoint) {
	tc.savepoints = append(tc.savepoints, sp)
}

func ReadCommittedTx(ctx context.Context, db *PostgresDB, fn TransactionFunc) error {
	tm := NewTransactionManager(db)
	opts := &TransactionOptions{
		IsolationLevel: sql.LevelReadCommitted,
		ReadOnly:       false,
		MaxRetries:     3,
		RetryDelay:     100 * time.Millisecond,
		Timeout:        30 * time.Second,
	}
	return tm.ExecuteWithOptions(ctx, opts, fn)
}

func RepeatableReadTx(ctx context.Context, db *PostgresDB, fn TransactionFunc) error {
	tm := NewTransactionManager(db)
	opts := &TransactionOptions{
		IsolationLevel: sql.LevelRepeatableRead,
		ReadOnly:       false,
		MaxRetries:     5,
		RetryDelay:     200 * time.Millisecond,
		Timeout:        60 * time.Second,
	}
	return tm.ExecuteWithOptions(ctx, opts, fn)
}

func SerializableTx(ctx context.Context, db *PostgresDB, fn TransactionFunc) error {
	tm := NewTransactionManager(db)
	opts := &TransactionOptions{
		IsolationLevel: sql.LevelSerializable,
		ReadOnly:       false,
		MaxRetries:     10,
		RetryDelay:     500 * time.Millisecond,
		Timeout:        120 * time.Second,
	}
	return tm.ExecuteWithOptions(ctx, opts, fn)
}

func ReadOnlyTx(ctx context.Context, db *PostgresDB, fn TransactionFunc) error {
	tm := NewTransactionManager(db)
	opts := &TransactionOptions{
		IsolationLevel: sql.LevelReadCommitted,
		ReadOnly:       true,
		MaxRetries:     1,
		RetryDelay:     0,
		Timeout:        30 * time.Second,
	}
	return tm.ExecuteWithOptions(ctx, opts, fn)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type LoggingHook struct {
	logger interface {
		Info(msg string, fields ...interface{})
		Error(msg string, fields ...interface{})
	}
}

func (h *LoggingHook) BeforeBegin(ctx context.Context) error {
	if h.logger != nil {
		h.logger.Info("transaction starting")
	}
	return nil
}

func (h *LoggingHook) AfterCommit(ctx context.Context) error {
	if h.logger != nil {
		h.logger.Info("transaction committed")
	}
	return nil
}

func (h *LoggingHook) AfterRollback(ctx context.Context, err error) error {
	if h.logger != nil {
		h.logger.Error("transaction rolled back", "error", err)
	}
	return nil
}

type MetricsHook struct {
	onBegin    func()
	onCommit   func(duration time.Duration)
	onRollback func(duration time.Duration, err error)
	startTime  time.Time
}

func (h *MetricsHook) BeforeBegin(ctx context.Context) error {
	h.startTime = time.Now()
	if h.onBegin != nil {
		h.onBegin()
	}
	return nil
}

func (h *MetricsHook) AfterCommit(ctx context.Context) error {
	duration := time.Since(h.startTime)
	if h.onCommit != nil {
		h.onCommit(duration)
	}
	return nil
}

func (h *MetricsHook) AfterRollback(ctx context.Context, err error) error {
	duration := time.Since(h.startTime)
	if h.onRollback != nil {
		h.onRollback(duration, err)
	}
	return nil
}

