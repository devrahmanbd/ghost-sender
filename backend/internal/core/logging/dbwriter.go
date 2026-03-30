package logging

import (
        "context"
        "crypto/rand"
        "fmt"
        "os"
        "strings"
        "sync"
        "time"

        "email-campaign-system/internal/storage/repository"
        "email-campaign-system/pkg/logger"
)

func generateUUID() string {
        b := make([]byte, 16)
        _, _ = rand.Read(b)
        b[6] = (b[6] & 0x0f) | 0x40
        b[8] = (b[8] & 0x3f) | 0x80
        return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
                b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type DBLogger struct {
        repo      *repository.LogRepository
        inner     logger.Logger
        hostname  string
        version   string
        env       string
        mu        sync.Mutex
        buffer    []*repository.LogEntry
        flushSize int
        flushTick *time.Ticker
        stopChan  chan struct{}
        stopped   bool
}

func NewDBLogger(repo *repository.LogRepository, inner logger.Logger, version, env string) *DBLogger {
        hostname, _ := os.Hostname()
        dl := &DBLogger{
                repo:      repo,
                inner:     inner,
                hostname:  hostname,
                version:   version,
                env:       env,
                buffer:    make([]*repository.LogEntry, 0, 64),
                flushSize: 20,
                flushTick: time.NewTicker(5 * time.Second),
                stopChan:  make(chan struct{}),
        }
        go dl.flushLoop()
        return dl
}

func (d *DBLogger) flushLoop() {
        for {
                select {
                case <-d.flushTick.C:
                        d.flush()
                case <-d.stopChan:
                        d.flush()
                        return
                }
        }
}

func (d *DBLogger) flush() {
        d.mu.Lock()
        if len(d.buffer) == 0 {
                d.mu.Unlock()
                return
        }
        batch := d.buffer
        d.buffer = make([]*repository.LogEntry, 0, 64)
        d.mu.Unlock()

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        if err := d.repo.InsertBatch(ctx, batch); err != nil {
                d.inner.Error("failed to flush log entries to DB", logger.Error(err))
        }
}

func (d *DBLogger) enqueue(entry *repository.LogEntry) {
        d.mu.Lock()
        defer d.mu.Unlock()
        if d.stopped {
                return
        }
        d.buffer = append(d.buffer, entry)
        if len(d.buffer) >= d.flushSize {
                go d.flush()
        }
}

func levelToDBLevel(lvl string) repository.LogLevel {
        switch strings.ToUpper(lvl) {
        case "DEBUG":
                return repository.LogLevelDebug
        case "INFO":
                return repository.LogLevelInfo
        case "WARN", "WARNING":
                return repository.LogLevelWarn
        case "ERROR":
                return repository.LogLevelError
        case "FATAL":
                return repository.LogLevelFatal
        default:
                return repository.LogLevelInfo
        }
}

func (d *DBLogger) buildEntry(level string, msg string, fields ...logger.Field) *repository.LogEntry {
        now := time.Now()
        entry := &repository.LogEntry{
                ID:          generateUUID(),
                Time:        now,
                Level:       levelToDBLevel(level),
                Category:    repository.LogCategorySystem,
                Message:     msg,
                Hostname:    d.hostname,
                Version:     d.version,
                Environment: d.env,
                Source:      "system",
                Component:   "server",
                CreatedAt:   now,
        }

        details := make(map[string]interface{})
        for _, f := range fields {
                switch f.Key {
                case "campaign_id":
                        if v, ok := f.Value.(string); ok {
                                entry.CampaignID = v
                                entry.Category = repository.LogCategoryCampaign
                        }
                case "account_id":
                        if v, ok := f.Value.(string); ok {
                                entry.AccountID = v
                                entry.Category = repository.LogCategoryAccount
                        }
                case "template_id":
                        if v, ok := f.Value.(string); ok {
                                entry.TemplateID = v
                                entry.Category = repository.LogCategoryTemplate
                        }
                case "recipient_id":
                        if v, ok := f.Value.(string); ok {
                                entry.RecipientID = v
                        }
                case "proxy_id":
                        if v, ok := f.Value.(string); ok {
                                entry.ProxyID = v
                                entry.Category = repository.LogCategoryProxy
                        }
                case "category":
                        if v, ok := f.Value.(string); ok {
                                entry.Category = repository.LogCategory(v)
                        }
                case "component":
                        if v, ok := f.Value.(string); ok {
                                entry.Component = v
                        }
                case "error":
                        if v, ok := f.Value.(error); ok && v != nil {
                                entry.ErrorClass = v.Error()
                        }
                case "error_code":
                        if v, ok := f.Value.(string); ok {
                                entry.ErrorCode = v
                        }
                case "request_id":
                        if v, ok := f.Value.(string); ok {
                                entry.RequestID = v
                        }
                case "http_method":
                        if v, ok := f.Value.(string); ok {
                                entry.HTTPMethod = v
                        }
                case "http_path", "path":
                        if v, ok := f.Value.(string); ok {
                                entry.HTTPPath = v
                        }
                case "http_status", "status":
                        if v, ok := f.Value.(int); ok {
                                entry.HTTPStatus = v
                        }
                case "duration_ms":
                        if v, ok := f.Value.(int64); ok {
                                entry.DurationMs = v
                        }
                case "client_ip", "remote_ip":
                        if v, ok := f.Value.(string); ok {
                                entry.ClientIP = v
                        }
                case "user_agent":
                        if v, ok := f.Value.(string); ok {
                                entry.UserAgent = v
                        }
                default:
                        details[f.Key] = f.Value
                }
        }
        if len(details) > 0 {
                entry.Details = details
        }

        return entry
}

func (d *DBLogger) Debug(msg string, fields ...logger.Field) {
        d.inner.Debug(msg, fields...)
}

func (d *DBLogger) Info(msg string, fields ...logger.Field) {
        d.inner.Info(msg, fields...)
        d.enqueue(d.buildEntry("INFO", msg, fields...))
}

func (d *DBLogger) Warn(msg string, fields ...logger.Field) {
        d.inner.Warn(msg, fields...)
        d.enqueue(d.buildEntry("WARN", msg, fields...))
}

func (d *DBLogger) Error(msg string, fields ...logger.Field) {
        d.inner.Error(msg, fields...)
        d.enqueue(d.buildEntry("ERROR", msg, fields...))
}

func (d *DBLogger) Fatal(msg string, fields ...logger.Field) {
        d.enqueue(d.buildEntry("FATAL", msg, fields...))
        d.flush()
        d.inner.Fatal(msg, fields...)
}

func (d *DBLogger) Panic(msg string, fields ...logger.Field) {
        d.enqueue(d.buildEntry("FATAL", msg, fields...))
        d.flush()
        d.inner.Panic(msg, fields...)
}

func (d *DBLogger) Debugf(format string, args ...interface{}) {
        d.inner.Debugf(format, args...)
}

func (d *DBLogger) Infof(format string, args ...interface{}) {
        msg := fmt.Sprintf(format, args...)
        d.inner.Info(msg)
        d.enqueue(d.buildEntry("INFO", msg))
}

func (d *DBLogger) Warnf(format string, args ...interface{}) {
        msg := fmt.Sprintf(format, args...)
        d.inner.Warn(msg)
        d.enqueue(d.buildEntry("WARN", msg))
}

func (d *DBLogger) Errorf(format string, args ...interface{}) {
        msg := fmt.Sprintf(format, args...)
        d.inner.Error(msg)
        d.enqueue(d.buildEntry("ERROR", msg))
}

func (d *DBLogger) Fatalf(format string, args ...interface{}) {
        msg := fmt.Sprintf(format, args...)
        d.enqueue(d.buildEntry("FATAL", msg))
        d.flush()
        d.inner.Fatal(msg)
}

func (d *DBLogger) Panicf(format string, args ...interface{}) {
        msg := fmt.Sprintf(format, args...)
        d.enqueue(d.buildEntry("FATAL", msg))
        d.flush()
        d.inner.Panic(msg)
}

func (d *DBLogger) With(fields ...logger.Field) logger.Logger {
        return &DBLogger{
                repo:      d.repo,
                inner:     d.inner.With(fields...),
                hostname:  d.hostname,
                version:   d.version,
                env:       d.env,
                buffer:    d.buffer,
                flushSize: d.flushSize,
                flushTick: d.flushTick,
                stopChan:  d.stopChan,
        }
}

func (d *DBLogger) WithContext(ctx context.Context) logger.Logger {
        return &DBLogger{
                repo:      d.repo,
                inner:     d.inner.WithContext(ctx),
                hostname:  d.hostname,
                version:   d.version,
                env:       d.env,
                buffer:    d.buffer,
                flushSize: d.flushSize,
                flushTick: d.flushTick,
                stopChan:  d.stopChan,
        }
}

func (d *DBLogger) WithError(err error) logger.Logger {
        return d.With(logger.Error(err))
}

func (d *DBLogger) SetLevel(level logger.Level) {
        d.inner.SetLevel(level)
}

func (d *DBLogger) GetLevel() logger.Level {
        return d.inner.GetLevel()
}

func (d *DBLogger) Sync() error {
        d.flush()
        return d.inner.Sync()
}

func (d *DBLogger) Close() error {
        d.mu.Lock()
        d.stopped = true
        d.mu.Unlock()
        close(d.stopChan)
        d.flushTick.Stop()
        d.flush()
        return d.inner.Close()
}

func (d *DBLogger) InsertDirect(entry *repository.LogEntry) {
        if entry.ID == "" {
                entry.ID = generateUUID()
        }
        if entry.Time.IsZero() {
                entry.Time = time.Now()
        }
        if entry.CreatedAt.IsZero() {
                entry.CreatedAt = time.Now()
        }
        if entry.Hostname == "" {
                entry.Hostname = d.hostname
        }
        if entry.Version == "" {
                entry.Version = d.version
        }
        if entry.Environment == "" {
                entry.Environment = d.env
        }
        d.enqueue(entry)
}
