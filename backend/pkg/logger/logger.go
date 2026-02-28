package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Level int8

const (
	DebugLevel Level = iota - 1
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
	PanicLevel
)

func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "fatal"
	case PanicLevel:
		return "panic"
	default:
		return "unknown"
	}
}

func ParseLevel(level string) (Level, error) {
	switch level {
	case "debug":
		return DebugLevel, nil
	case "info":
		return InfoLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "error":
		return ErrorLevel, nil
	case "fatal":
		return FatalLevel, nil
	case "panic":
		return PanicLevel, nil
	default:
		return InfoLevel, fmt.Errorf("unknown level: %s", level)
	}
}

type Field struct {
	Key   string
	Value interface{}
	Type  FieldType
}

type FieldType int

const (
	StringType FieldType = iota
	IntType
	Int64Type
	Float64Type
	BoolType
	TimeType
	DurationType
	ErrorType
	AnyType
)

func String(key string, value string) Field {
	return Field{Key: key, Value: value, Type: StringType}
}

func Int(key string, value int) Field {
	return Field{Key: key, Value: value, Type: IntType}
}

func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value, Type: Int64Type}
}

func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value, Type: Float64Type}
}

func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value, Type: BoolType}
}

func Time(key string, value time.Time) Field {
	return Field{Key: key, Value: value, Type: TimeType}
}

func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value, Type: DurationType}
}

func Error(err error) Field {
	return Field{Key: "error", Value: err, Type: ErrorType}
}

func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value, Type: AnyType}
}

type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	Panic(msg string, fields ...Field)

	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Panicf(format string, args ...interface{})

	With(fields ...Field) Logger
	WithContext(ctx context.Context) Logger
	WithError(err error) Logger

	SetLevel(level Level)
	GetLevel() Level

	Sync() error
	Close() error
}

type Config struct {
	Level             Level
	Format            string
	OutputPaths       []string
	ErrorOutputPaths  []string
	EnableCaller      bool
	EnableStacktrace  bool
	TimeEncoder       string
	LevelEncoder      string
	NameEncoder       string
	CallerEncoder     string
	MaxSize           int
	MaxBackups        int
	MaxAge            int
	Compress          bool
	Development       bool
	DisableStacktrace bool
	Sampling          *SamplingConfig
}

type SamplingConfig struct {
	Initial    int
	Thereafter int
}

type LoggerFactory interface {
	NewLogger(name string, config *Config) (Logger, error)
	NewLoggerWithOutput(name string, config *Config, output io.Writer) (Logger, error)
}

var (
	defaultLogger     Logger
	defaultLoggerOnce sync.Once
	defaultLoggerMu   sync.RWMutex
)

func Default() Logger {
	defaultLoggerOnce.Do(func() {
		cfg := &Config{
			Level:            InfoLevel,
			Format:           "json",
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
			EnableCaller:     true,
			EnableStacktrace: false,
			Development:      false,
		}
		var err error
		defaultLogger, err = NewZapLogger("default", cfg)
		if err != nil {
			panic(fmt.Sprintf("failed to initialize default logger: %v", err))
		}
	})
	return defaultLogger
}

func SetDefault(logger Logger) {
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	defaultLogger = logger
}

func Debug(msg string, fields ...Field) {
	Default().Debug(msg, fields...)
}

func Info(msg string, fields ...Field) {
	Default().Info(msg, fields...)
}

func Warn(msg string, fields ...Field) {
	Default().Warn(msg, fields...)
}

func Fatal(msg string, fields ...Field) {
	Default().Fatal(msg, fields...)
}

func Panic(msg string, fields ...Field) {
	Default().Panic(msg, fields...)
}

func Debugf(format string, args ...interface{}) {
	Default().Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	Default().Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	Default().Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	Default().Errorf(format, args...)
}

func Fatalf(format string, args ...interface{}) {
	Default().Fatalf(format, args...)
}

func Panicf(format string, args ...interface{}) {
	Default().Panicf(format, args...)
}

func With(fields ...Field) Logger {
	return Default().With(fields...)
}

func WithContext(ctx context.Context) Logger {
	return Default().WithContext(ctx)
}

func WithError(err error) Logger {
	return Default().WithError(err)
}

type LogType string

const (
	CampaignLog    LogType = "campaign"
	DebugLog       LogType = "debug"
	FailedLog      LogType = "failed"
	SuccessLog     LogType = "success"
	SystemLog      LogType = "system"
	PerformanceLog LogType = "performance"
	AccountLog     LogType = "account"
	TemplateLog    LogType = "template"
	ProxyLog       LogType = "proxy"
	NotificationLog LogType = "notification"
)

type LoggerManager struct {
	mu      sync.RWMutex
	loggers map[string]Logger
	config  *Config
	factory LoggerFactory
}

func NewLoggerManager(config *Config) *LoggerManager {
	return &LoggerManager{
		loggers: make(map[string]Logger),
		config:  config,
		factory: &zapFactory{},
	}
}

func (m *LoggerManager) GetLogger(name string) (Logger, error) {
	m.mu.RLock()
	if logger, exists := m.loggers[name]; exists {
		m.mu.RUnlock()
		return logger, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if logger, exists := m.loggers[name]; exists {
		return logger, nil
	}

	logger, err := m.factory.NewLogger(name, m.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	m.loggers[name] = logger
	return logger, nil
}

func (m *LoggerManager) GetOrCreateLogger(name string, config *Config) (Logger, error) {
	m.mu.RLock()
	if logger, exists := m.loggers[name]; exists {
		m.mu.RUnlock()
		return logger, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if logger, exists := m.loggers[name]; exists {
		return logger, nil
	}

	logger, err := m.factory.NewLogger(name, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	m.loggers[name] = logger
	return logger, nil
}

func (m *LoggerManager) SetLogger(name string, logger Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loggers[name] = logger
}

func (m *LoggerManager) RemoveLogger(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if logger, exists := m.loggers[name]; exists {
		logger.Sync()
		delete(m.loggers, name)
	}
}

func (m *LoggerManager) SyncAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errs []error
	for name, logger := range m.loggers {
		if err := logger.Sync(); err != nil {
			errs = append(errs, fmt.Errorf("failed to sync logger %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors syncing loggers: %v", errs)
	}
	return nil
}

func (m *LoggerManager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for name, logger := range m.loggers {
		if err := logger.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close logger %s: %w", name, err))
		}
	}

	m.loggers = make(map[string]Logger)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing loggers: %v", errs)
	}
	return nil
}

func (m *LoggerManager) GetAllLoggers() map[string]Logger {
	m.mu.RLock()
	defer m.mu.RUnlock()

	loggers := make(map[string]Logger, len(m.loggers))
	for name, logger := range m.loggers {
		loggers[name] = logger
	}
	return loggers
}

type CampaignLogger struct {
	Logger
	campaignID   string
	campaignName string
}

func NewCampaignLogger(base Logger, campaignID, campaignName string) *CampaignLogger {
	return &CampaignLogger{
		Logger: base.With(
			String("campaign_id", campaignID),
			String("campaign_name", campaignName),
			String("log_type", string(CampaignLog)),
		),
		campaignID:   campaignID,
		campaignName: campaignName,
	}
}

func (cl *CampaignLogger) LogEmailSent(recipient string, accountEmail string) {
	cl.Info("email sent",
		String("recipient", recipient),
		String("account", accountEmail),
		Time("sent_at", time.Now()),
	)
}

func (cl *CampaignLogger) LogEmailFailed(recipient string, accountEmail string, err error) {
	cl.Error("email failed",
		String("recipient", recipient),
		String("account", accountEmail),
		Error(err),
		Time("failed_at", time.Now()),
	)
}

func (cl *CampaignLogger) LogCampaignStarted(totalRecipients int) {
	cl.Info("campaign started",
		Int("total_recipients", totalRecipients),
		Time("started_at", time.Now()),
	)
}

func (cl *CampaignLogger) LogCampaignCompleted(sent, failed int, duration time.Duration) {
	cl.Info("campaign completed",
		Int("sent", sent),
		Int("failed", failed),
		Duration("duration", duration),
		Time("completed_at", time.Now()),
	)
}

func (cl *CampaignLogger) LogCampaignPaused(reason string) {
	cl.Warn("campaign paused",
		String("reason", reason),
		Time("paused_at", time.Now()),
	)
}

func (cl *CampaignLogger) LogCampaignResumed() {
	cl.Info("campaign resumed",
		Time("resumed_at", time.Now()),
	)
}

type PerformanceLogger struct {
	Logger
	component string
}

func NewPerformanceLogger(base Logger, component string) *PerformanceLogger {
	return &PerformanceLogger{
		Logger: base.With(
			String("component", component),
			String("log_type", string(PerformanceLog)),
		),
		component: component,
	}
}

func (pl *PerformanceLogger) LogDuration(operation string, duration time.Duration, fields ...Field) {
	allFields := append([]Field{
		String("operation", operation),
		Duration("duration", duration),
		Time("timestamp", time.Now()),
	}, fields...)
	pl.Info("performance metric", allFields...)
}

func (pl *PerformanceLogger) LogThroughput(operation string, count int64, duration time.Duration) {
	rate := float64(count) / duration.Seconds()
	pl.Info("throughput metric",
		String("operation", operation),
		Int64("count", count),
		Duration("duration", duration),
		Float64("rate_per_second", rate),
		Time("timestamp", time.Now()),
	)
}

func (pl *PerformanceLogger) LogMemoryUsage(operation string, bytes int64) {
	pl.Info("memory usage",
		String("operation", operation),
		Int64("bytes", bytes),
		Float64("mb", float64(bytes)/1024/1024),
		Time("timestamp", time.Now()),
	)
}

type SessionLogger struct {
	Logger
	sessionID string
}

func NewSessionLogger(base Logger, sessionID string) *SessionLogger {
	return &SessionLogger{
		Logger: base.With(
			String("session_id", sessionID),
			Time("session_start", time.Now()),
		),
		sessionID: sessionID,
	}
}

func (sl *SessionLogger) GetSessionID() string {
	return sl.sessionID
}

func NewLogger(name string, config *Config) (Logger, error) {
	return NewZapLogger(name, config)
}

func NewLoggerWithOutput(name string, config *Config, output io.Writer) (Logger, error) {
	return NewZapLoggerWithOutput(name, config, output)
}

func GetDefaultConfig() *Config {
	return &Config{
		Level:            InfoLevel,
		Format:           "json",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EnableCaller:     true,
		EnableStacktrace: false,
		TimeEncoder:      "iso8601",
		LevelEncoder:     "lowercase",
		Development:      false,
		MaxSize:          100,
		MaxBackups:       10,
		MaxAge:           30,
		Compress:         true,
	}
}

func GetDevelopmentConfig() *Config {
	return &Config{
		Level:            DebugLevel,
		Format:           "console",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EnableCaller:     true,
		EnableStacktrace: true,
		TimeEncoder:      "iso8601",
		LevelEncoder:     "color",
		Development:      true,
		MaxSize:          50,
		MaxBackups:       3,
		MaxAge:           7,
		Compress:         false,
	}
}

func GetProductionConfig() *Config {
	return &Config{
		Level:            InfoLevel,
		Format:           "json",
		OutputPaths:      []string{"./logs/app.log"},
		ErrorOutputPaths: []string{"./logs/error.log"},
		EnableCaller:     true,
		EnableStacktrace: false,
		TimeEncoder:      "epoch",
		LevelEncoder:     "lowercase",
		Development:      false,
		MaxSize:          200,
		MaxBackups:       30,
		MaxAge:           90,
		Compress:         true,
		Sampling: &SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
	}
}

func MustNewLogger(name string, config *Config) Logger {
	logger, err := NewLogger(name, config)
	if err != nil {
		panic(fmt.Sprintf("failed to create logger: %v", err))
	}
	return logger
}

func Sync() error {
	return Default().Sync()
}

func Close() error {
	return Default().Close()
}

type MultiLogger struct {
	loggers []Logger
	mu      sync.RWMutex
}

func NewMultiLogger(loggers ...Logger) *MultiLogger {
	return &MultiLogger{
		loggers: loggers,
	}
}

func (ml *MultiLogger) Debug(msg string, fields ...Field) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	for _, logger := range ml.loggers {
		logger.Debug(msg, fields...)
	}
}

func (ml *MultiLogger) Info(msg string, fields ...Field) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	for _, logger := range ml.loggers {
		logger.Info(msg, fields...)
	}
}

func (ml *MultiLogger) Warn(msg string, fields ...Field) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	for _, logger := range ml.loggers {
		logger.Warn(msg, fields...)
	}
}

func (ml *MultiLogger) Error(msg string, fields ...Field) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	for _, logger := range ml.loggers {
		logger.Error(msg, fields...)
	}
}

func (ml *MultiLogger) Fatal(msg string, fields ...Field) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	for _, logger := range ml.loggers {
		logger.Fatal(msg, fields...)
	}
	os.Exit(1)
}

func (ml *MultiLogger) Panic(msg string, fields ...Field) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	for _, logger := range ml.loggers {
		logger.Panic(msg, fields...)
	}
	panic(msg)
}

func (ml *MultiLogger) Debugf(format string, args ...interface{}) {
	ml.Debug(fmt.Sprintf(format, args...))
}

func (ml *MultiLogger) Infof(format string, args ...interface{}) {
	ml.Info(fmt.Sprintf(format, args...))
}

func (ml *MultiLogger) Warnf(format string, args ...interface{}) {
	ml.Warn(fmt.Sprintf(format, args...))
}

func (ml *MultiLogger) Errorf(format string, args ...interface{}) {
	ml.Error(fmt.Sprintf(format, args...))
}

func (ml *MultiLogger) Fatalf(format string, args ...interface{}) {
	ml.Fatal(fmt.Sprintf(format, args...))
}

func (ml *MultiLogger) Panicf(format string, args ...interface{}) {
	ml.Panic(fmt.Sprintf(format, args...))
}

func (ml *MultiLogger) With(fields ...Field) Logger {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	newLoggers := make([]Logger, len(ml.loggers))
	for i, logger := range ml.loggers {
		newLoggers[i] = logger.With(fields...)
	}
	return &MultiLogger{loggers: newLoggers}
}

func (ml *MultiLogger) WithContext(ctx context.Context) Logger {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	newLoggers := make([]Logger, len(ml.loggers))
	for i, logger := range ml.loggers {
		newLoggers[i] = logger.WithContext(ctx)
	}
	return &MultiLogger{loggers: newLoggers}
}

func (ml *MultiLogger) WithError(err error) Logger {
	return ml.With(Error(err))
}

func (ml *MultiLogger) SetLevel(level Level) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	for _, logger := range ml.loggers {
		logger.SetLevel(level)
	}
}

func (ml *MultiLogger) GetLevel() Level {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	if len(ml.loggers) > 0 {
		return ml.loggers[0].GetLevel()
	}
	return InfoLevel
}

func (ml *MultiLogger) Sync() error {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	for _, logger := range ml.loggers {
		if err := logger.Sync(); err != nil {
			return err
		}
	}
	return nil
}

func (ml *MultiLogger) Close() error {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	for _, logger := range ml.loggers {
		if err := logger.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (ml *MultiLogger) AddLogger(logger Logger) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.loggers = append(ml.loggers, logger)
}

func (ml *MultiLogger) RemoveLogger(index int) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	if index >= 0 && index < len(ml.loggers) {
		ml.loggers = append(ml.loggers[:index], ml.loggers[index+1:]...)
	}
}

type NoOpLogger struct{}

func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

func (n *NoOpLogger) Debug(msg string, fields ...Field)                  {}
func (n *NoOpLogger) Info(msg string, fields ...Field)                   {}
func (n *NoOpLogger) Warn(msg string, fields ...Field)                   {}
func (n *NoOpLogger) Error(msg string, fields ...Field)                  {}
func (n *NoOpLogger) Fatal(msg string, fields ...Field)                  { os.Exit(1) }
func (n *NoOpLogger) Panic(msg string, fields ...Field)                  { panic(msg) }
func (n *NoOpLogger) Debugf(format string, args ...interface{})          {}
func (n *NoOpLogger) Infof(format string, args ...interface{})           {}
func (n *NoOpLogger) Warnf(format string, args ...interface{})           {}
func (n *NoOpLogger) Errorf(format string, args ...interface{})          {}
func (n *NoOpLogger) Fatalf(format string, args ...interface{})          { os.Exit(1) }
func (n *NoOpLogger) Panicf(format string, args ...interface{})          { panic(fmt.Sprintf(format, args...)) }
func (n *NoOpLogger) With(fields ...Field) Logger                        { return n }
func (n *NoOpLogger) WithContext(ctx context.Context) Logger             { return n }
func (n *NoOpLogger) WithError(err error) Logger                         { return n }
func (n *NoOpLogger) SetLevel(level Level)                               {}
func (n *NoOpLogger) GetLevel() Level                                    { return InfoLevel }
func (n *NoOpLogger) Sync() error                                        { return nil }
func (n *NoOpLogger) Close() error                                       { return nil }

