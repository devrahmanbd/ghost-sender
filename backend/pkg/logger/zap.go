package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type zapLogger struct {
	logger        *zap.Logger
	sugaredLogger *zap.SugaredLogger
	level         Level
	config        *Config
}

type zapFactory struct{}

func (f *zapFactory) NewLogger(name string, config *Config) (Logger, error) {
	return NewZapLogger(name, config)
}

func (f *zapFactory) NewLoggerWithOutput(name string, config *Config, output io.Writer) (Logger, error) {
	return NewZapLoggerWithOutput(name, config, output)
}

func NewZapLogger(name string, config *Config) (*zapLogger, error) {
	if config == nil {
		config = GetDefaultConfig()
	}

	_ = buildZapConfig(config)
	
	cores := make([]zapcore.Core, 0)

	for _, outputPath := range config.OutputPaths {
		writer := getWriter(outputPath, config)
		encoder := getEncoder(config)
		levelEnabler := getZapLevel(config.Level)
		
		core := zapcore.NewCore(encoder, writer, levelEnabler)
		cores = append(cores, core)
	}

	if len(config.ErrorOutputPaths) > 0 {
		for _, errorPath := range config.ErrorOutputPaths {
			writer := getWriter(errorPath, config)
			encoder := getEncoder(config)
			levelEnabler := zapcore.LevelEnabler(zapcore.ErrorLevel)
			
			core := zapcore.NewCore(encoder, writer, levelEnabler)
			cores = append(cores, core)
		}
	}

	core := zapcore.NewTee(cores...)

	if config.Sampling != nil {
		core = zapcore.NewSamplerWithOptions(
			core,
			time.Second,
			config.Sampling.Initial,
			config.Sampling.Thereafter,
		)
	}

	options := []zap.Option{
		zap.AddCaller(),
		zap.AddCallerSkip(1),
	}

	if config.EnableStacktrace {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	if config.Development {
		options = append(options, zap.Development())
	}

	if name != "" {
		options = append(options, zap.Fields(zap.String("logger", name)))
	}

	zapLog := zap.New(core, options...)

	return &zapLogger{
		logger:        zapLog,
		sugaredLogger: zapLog.Sugar(),
		level:         config.Level,
		config:        config,
	}, nil
}

func NewZapLoggerWithOutput(name string, config *Config, output io.Writer) (*zapLogger, error) {
	if config == nil {
		config = GetDefaultConfig()
	}

	encoder := getEncoder(config)
	writer := zapcore.AddSync(output)
	levelEnabler := getZapLevel(config.Level)

	core := zapcore.NewCore(encoder, writer, levelEnabler)

	options := []zap.Option{
		zap.AddCaller(),
		zap.AddCallerSkip(1),
	}

	if config.EnableStacktrace {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	if name != "" {
		options = append(options, zap.Fields(zap.String("logger", name)))
	}

	zapLog := zap.New(core, options...)

	return &zapLogger{
		logger:        zapLog,
		sugaredLogger: zapLog.Sugar(),
		level:         config.Level,
		config:        config,
	}, nil
}

func (l *zapLogger) Debug(msg string, fields ...Field) {
	l.logger.Debug(msg, convertFields(fields)...)
}

func (l *zapLogger) Info(msg string, fields ...Field) {
	l.logger.Info(msg, convertFields(fields)...)
}

func (l *zapLogger) Warn(msg string, fields ...Field) {
	l.logger.Warn(msg, convertFields(fields)...)
}

func (l *zapLogger) Error(msg string, fields ...Field) {
	l.logger.Error(msg, convertFields(fields)...)
}

func (l *zapLogger) Fatal(msg string, fields ...Field) {
	l.logger.Fatal(msg, convertFields(fields)...)
}

func (l *zapLogger) Panic(msg string, fields ...Field) {
	l.logger.Panic(msg, convertFields(fields)...)
}

func (l *zapLogger) Debugf(format string, args ...interface{}) {
	l.sugaredLogger.Debugf(format, args...)
}

func (l *zapLogger) Infof(format string, args ...interface{}) {
	l.sugaredLogger.Infof(format, args...)
}

func (l *zapLogger) Warnf(format string, args ...interface{}) {
	l.sugaredLogger.Warnf(format, args...)
}

func (l *zapLogger) Errorf(format string, args ...interface{}) {
	l.sugaredLogger.Errorf(format, args...)
}

func (l *zapLogger) Fatalf(format string, args ...interface{}) {
	l.sugaredLogger.Fatalf(format, args...)
}

func (l *zapLogger) Panicf(format string, args ...interface{}) {
	l.sugaredLogger.Panicf(format, args...)
}

func (l *zapLogger) With(fields ...Field) Logger {
	newLogger := l.logger.With(convertFields(fields)...)
	return &zapLogger{
		logger:        newLogger,
		sugaredLogger: newLogger.Sugar(),
		level:         l.level,
		config:        l.config,
	}
}

func (l *zapLogger) WithContext(ctx context.Context) Logger {
	fields := extractFieldsFromContext(ctx)
	if len(fields) == 0 {
		return l
	}
	return l.With(fields...)
}

func (l *zapLogger) WithError(err error) Logger {
	return l.With(Error(err))
}

func (l *zapLogger) SetLevel(level Level) {
	l.level = level
}

func (l *zapLogger) GetLevel() Level {
	return l.level
}

func (l *zapLogger) Sync() error {
	return l.logger.Sync()
}

func (l *zapLogger) Close() error {
	return l.logger.Sync()
}

func convertFields(fields []Field) []zap.Field {
	zapFields := make([]zap.Field, 0, len(fields))
	for _, field := range fields {
		zapFields = append(zapFields, convertField(field))
	}
	return zapFields
}

func convertField(field Field) zap.Field {
	switch field.Type {
	case StringType:
		return zap.String(field.Key, field.Value.(string))
	case IntType:
		return zap.Int(field.Key, field.Value.(int))
	case Int64Type:
		return zap.Int64(field.Key, field.Value.(int64))
	case Float64Type:
		return zap.Float64(field.Key, field.Value.(float64))
	case BoolType:
		return zap.Bool(field.Key, field.Value.(bool))
	case TimeType:
		return zap.Time(field.Key, field.Value.(time.Time))
	case DurationType:
		return zap.Duration(field.Key, field.Value.(time.Duration))
	case ErrorType:
		if err, ok := field.Value.(error); ok {
			return zap.Error(err)
		}
		return zap.String(field.Key, fmt.Sprintf("%v", field.Value))
	case AnyType:
		return zap.Any(field.Key, field.Value)
	default:
		return zap.Any(field.Key, field.Value)
	}
}

func getZapLevel(level Level) zapcore.LevelEnabler {
	switch level {
	case DebugLevel:
		return zapcore.DebugLevel
	case InfoLevel:
		return zapcore.InfoLevel
	case WarnLevel:
		return zapcore.WarnLevel
	case ErrorLevel:
		return zapcore.ErrorLevel
	case FatalLevel:
		return zapcore.FatalLevel
	case PanicLevel:
		return zapcore.PanicLevel
	default:
		return zapcore.InfoLevel
	}
}

func getEncoder(config *Config) zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    getLevelEncoder(config.LevelEncoder),
		EncodeTime:     getTimeEncoder(config.TimeEncoder),
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   getCallerEncoder(config.CallerEncoder),
	}

	if config.Format == "console" {
		return zapcore.NewConsoleEncoder(encoderConfig)
	}
	return zapcore.NewJSONEncoder(encoderConfig)
}

func getLevelEncoder(encoder string) zapcore.LevelEncoder {
	switch encoder {
	case "lowercase":
		return zapcore.LowercaseLevelEncoder
	case "capital":
		return zapcore.CapitalLevelEncoder
	case "color":
		return zapcore.CapitalColorLevelEncoder
	default:
		return zapcore.LowercaseLevelEncoder
	}
}

func getTimeEncoder(encoder string) zapcore.TimeEncoder {
	switch encoder {
	case "iso8601":
		return zapcore.ISO8601TimeEncoder
	case "epoch":
		return zapcore.EpochTimeEncoder
	case "millis":
		return zapcore.EpochMillisTimeEncoder
	case "nanos":
		return zapcore.EpochNanosTimeEncoder
	case "rfc3339":
		return zapcore.RFC3339TimeEncoder
	case "rfc3339nano":
		return zapcore.RFC3339NanoTimeEncoder
	default:
		return zapcore.ISO8601TimeEncoder
	}
}

func getCallerEncoder(encoder string) zapcore.CallerEncoder {
	switch encoder {
	case "short":
		return zapcore.ShortCallerEncoder
	case "full":
		return zapcore.FullCallerEncoder
	default:
		return zapcore.ShortCallerEncoder
	}
}

func getWriter(path string, config *Config) zapcore.WriteSyncer {
	if path == "stdout" {
		return zapcore.AddSync(os.Stdout)
	}
	if path == "stderr" {
		return zapcore.AddSync(os.Stderr)
	}

	lumberLog := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
		Compress:   config.Compress,
	}

	return zapcore.AddSync(lumberLog)
}

func buildZapConfig(config *Config) zap.Config {
	var zapConfig zap.Config

	if config.Development {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}

	zapConfig.Level = zap.NewAtomicLevelAt(convertToZapLevel(config.Level))
	zapConfig.OutputPaths = config.OutputPaths
	zapConfig.ErrorOutputPaths = config.ErrorOutputPaths
	zapConfig.DisableCaller = !config.EnableCaller
	zapConfig.DisableStacktrace = !config.EnableStacktrace

	if config.Format == "console" {
		zapConfig.Encoding = "console"
	} else {
		zapConfig.Encoding = "json"
	}

	zapConfig.EncoderConfig.TimeKey = "timestamp"
	zapConfig.EncoderConfig.LevelKey = "level"
	zapConfig.EncoderConfig.MessageKey = "message"
	zapConfig.EncoderConfig.CallerKey = "caller"
	zapConfig.EncoderConfig.StacktraceKey = "stacktrace"

	zapConfig.EncoderConfig.EncodeLevel = getLevelEncoder(config.LevelEncoder)
	zapConfig.EncoderConfig.EncodeTime = getTimeEncoder(config.TimeEncoder)
	zapConfig.EncoderConfig.EncodeCaller = getCallerEncoder(config.CallerEncoder)

	return zapConfig
}

func convertToZapLevel(level Level) zapcore.Level {
	switch level {
	case DebugLevel:
		return zapcore.DebugLevel
	case InfoLevel:
		return zapcore.InfoLevel
	case WarnLevel:
		return zapcore.WarnLevel
	case ErrorLevel:
		return zapcore.ErrorLevel
	case FatalLevel:
		return zapcore.FatalLevel
	case PanicLevel:
		return zapcore.PanicLevel
	default:
		return zapcore.InfoLevel
	}
}

func extractFieldsFromContext(ctx context.Context) []Field {
	fields := make([]Field, 0)

	if requestID := ctx.Value("request_id"); requestID != nil {
		fields = append(fields, String("request_id", fmt.Sprintf("%v", requestID)))
	}

	if userID := ctx.Value("user_id"); userID != nil {
		fields = append(fields, String("user_id", fmt.Sprintf("%v", userID)))
	}

	if sessionID := ctx.Value("session_id"); sessionID != nil {
		fields = append(fields, String("session_id", fmt.Sprintf("%v", sessionID)))
	}

	if campaignID := ctx.Value("campaign_id"); campaignID != nil {
		fields = append(fields, String("campaign_id", fmt.Sprintf("%v", campaignID)))
	}

	if accountID := ctx.Value("account_id"); accountID != nil {
		fields = append(fields, String("account_id", fmt.Sprintf("%v", accountID)))
	}

	return fields
}

func NewFileLogger(filepath string, config *Config) (*zapLogger, error) {
	if config == nil {
		config = GetDefaultConfig()
	}

	config.OutputPaths = []string{filepath}
	return NewZapLogger("", config)
}

func NewConsoleLogger(config *Config) (*zapLogger, error) {
	if config == nil {
		config = GetDefaultConfig()
	}

	config.Format = "console"
	config.OutputPaths = []string{"stdout"}
	return NewZapLogger("", config)
}

func NewJSONLogger(config *Config) (*zapLogger, error) {
	if config == nil {
		config = GetDefaultConfig()
	}

	config.Format = "json"
	return NewZapLogger("", config)
}

func NewDevelopmentLogger() (*zapLogger, error) {
	config := GetDevelopmentConfig()
	return NewZapLogger("development", config)
}

func NewProductionLogger() (*zapLogger, error) {
	config := GetProductionConfig()
	return NewZapLogger("production", config)
}

func NewCampaignFileLogger(campaignID string, filepath string) (*zapLogger, error) {
	config := &Config{
		Level:            InfoLevel,
		Format:           "json",
		OutputPaths:      []string{filepath},
		ErrorOutputPaths: []string{filepath},
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

	logger, err := NewZapLogger("campaign", config)
	if err != nil {
		return nil, err
	}

	return &zapLogger{
		logger:        logger.logger.With(zap.String("campaign_id", campaignID)),
		sugaredLogger: logger.sugaredLogger.With("campaign_id", campaignID),
		level:         logger.level,
		config:        logger.config,
	}, nil
}

func NewSessionFileLogger(sessionID string, filepath string) (*zapLogger, error) {
	config := &Config{
		Level:            DebugLevel,
		Format:           "json",
		OutputPaths:      []string{filepath},
		ErrorOutputPaths: []string{filepath},
		EnableCaller:     true,
		EnableStacktrace: true,
		TimeEncoder:      "iso8601",
		LevelEncoder:     "lowercase",
		Development:      false,
		MaxSize:          50,
		MaxBackups:       5,
		MaxAge:           7,
		Compress:         true,
	}

	logger, err := NewZapLogger("session", config)
	if err != nil {
		return nil, err
	}

	return &zapLogger{
		logger:        logger.logger.With(zap.String("session_id", sessionID)),
		sugaredLogger: logger.sugaredLogger.With("session_id", sessionID),
		level:         logger.level,
		config:        logger.config,
	}, nil
}

func NewMultiOutputLogger(name string, outputs []string, config *Config) (*zapLogger, error) {
	if config == nil {
		config = GetDefaultConfig()
	}

	config.OutputPaths = outputs
	return NewZapLogger(name, config)
}

func CloneLogger(logger *zapLogger) *zapLogger {
	return &zapLogger{
		logger:        logger.logger,
		sugaredLogger: logger.sugaredLogger,
		level:         logger.level,
		config:        logger.config,
	}
}

func GetZapLogger(logger Logger) (*zap.Logger, bool) {
	if zl, ok := logger.(*zapLogger); ok {
		return zl.logger, true
	}
	return nil, false
}

func GetSugaredLogger(logger Logger) (*zap.SugaredLogger, bool) {
	if zl, ok := logger.(*zapLogger); ok {
		return zl.sugaredLogger, true
	}
	return nil, false
}

