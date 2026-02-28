package logger

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const (
	loggerKey      contextKey = "logger"
	requestIDKey   contextKey = "request_id"
	sessionIDKey   contextKey = "session_id"
	campaignIDKey  contextKey = "campaign_id"
	accountIDKey   contextKey = "account_id"
	userIDKey      contextKey = "user_id"
	traceIDKey     contextKey = "trace_id"
	spanIDKey      contextKey = "span_id"
	tenantIDKey    contextKey = "tenant_id"
	operationKey   contextKey = "operation"
	componentKey   contextKey = "component"
	workerIDKey    contextKey = "worker_id"
	recipientKey   contextKey = "recipient"
	templateIDKey  contextKey = "template_id"
	providerKey    contextKey = "provider"
	proxyIDKey     contextKey = "proxy_id"
	ipAddressKey   contextKey = "ip_address"
	userAgentKey   contextKey = "user_agent"
	methodKey      contextKey = "method"
	pathKey        contextKey = "path"
	statusCodeKey  contextKey = "status_code"
	durationKey    contextKey = "duration"
	errorKey       contextKey = "error"
	startTimeKey   contextKey = "start_time"
)

func FromContext(ctx context.Context) Logger {
	if logger, ok := ctx.Value(loggerKey).(Logger); ok {
		return logger
	}
	return Default()
}

func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("request_id", requestID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, requestIDKey, requestID)
}

func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("session_id", sessionID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

func GetSessionID(ctx context.Context) string {
	if id, ok := ctx.Value(sessionIDKey).(string); ok {
		return id
	}
	return ""
}

func WithCampaignID(ctx context.Context, campaignID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("campaign_id", campaignID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, campaignIDKey, campaignID)
}

func GetCampaignID(ctx context.Context) string {
	if id, ok := ctx.Value(campaignIDKey).(string); ok {
		return id
	}
	return ""
}

func WithAccountID(ctx context.Context, accountID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("account_id", accountID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, accountIDKey, accountID)
}

func GetAccountID(ctx context.Context) string {
	if id, ok := ctx.Value(accountIDKey).(string); ok {
		return id
	}
	return ""
}

func WithUserID(ctx context.Context, userID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("user_id", userID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, userIDKey, userID)
}

func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("trace_id", traceID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, traceIDKey, traceID)
}

func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey).(string); ok {
		return id
	}
	return ""
}

func WithSpanID(ctx context.Context, spanID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("span_id", spanID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, spanIDKey, spanID)
}

func GetSpanID(ctx context.Context) string {
	if id, ok := ctx.Value(spanIDKey).(string); ok {
		return id
	}
	return ""
}

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("tenant_id", tenantID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

func GetTenantID(ctx context.Context) string {
	if id, ok := ctx.Value(tenantIDKey).(string); ok {
		return id
	}
	return ""
}

func WithOperation(ctx context.Context, operation string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("operation", operation))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, operationKey, operation)
}

func GetOperation(ctx context.Context) string {
	if op, ok := ctx.Value(operationKey).(string); ok {
		return op
	}
	return ""
}

func WithComponent(ctx context.Context, component string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("component", component))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, componentKey, component)
}

func GetComponent(ctx context.Context) string {
	if comp, ok := ctx.Value(componentKey).(string); ok {
		return comp
	}
	return ""
}

func WithWorkerID(ctx context.Context, workerID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("worker_id", workerID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, workerIDKey, workerID)
}

func GetWorkerID(ctx context.Context) string {
	if id, ok := ctx.Value(workerIDKey).(string); ok {
		return id
	}
	return ""
}

func WithRecipient(ctx context.Context, recipient string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("recipient", recipient))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, recipientKey, recipient)
}

func GetRecipient(ctx context.Context) string {
	if r, ok := ctx.Value(recipientKey).(string); ok {
		return r
	}
	return ""
}

func WithTemplateID(ctx context.Context, templateID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("template_id", templateID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, templateIDKey, templateID)
}

func GetTemplateID(ctx context.Context) string {
	if id, ok := ctx.Value(templateIDKey).(string); ok {
		return id
	}
	return ""
}

func WithProvider(ctx context.Context, provider string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("provider", provider))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, providerKey, provider)
}

func GetProvider(ctx context.Context) string {
	if p, ok := ctx.Value(providerKey).(string); ok {
		return p
	}
	return ""
}

func WithProxyID(ctx context.Context, proxyID string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("proxy_id", proxyID))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, proxyIDKey, proxyID)
}

func GetProxyID(ctx context.Context) string {
	if id, ok := ctx.Value(proxyIDKey).(string); ok {
		return id
	}
	return ""
}

func WithIPAddress(ctx context.Context, ipAddress string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("ip_address", ipAddress))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, ipAddressKey, ipAddress)
}

func GetIPAddress(ctx context.Context) string {
	if ip, ok := ctx.Value(ipAddressKey).(string); ok {
		return ip
	}
	return ""
}

func WithUserAgent(ctx context.Context, userAgent string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("user_agent", userAgent))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, userAgentKey, userAgent)
}

func GetUserAgent(ctx context.Context) string {
	if ua, ok := ctx.Value(userAgentKey).(string); ok {
		return ua
	}
	return ""
}

func WithMethod(ctx context.Context, method string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("method", method))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, methodKey, method)
}

func GetMethod(ctx context.Context) string {
	if m, ok := ctx.Value(methodKey).(string); ok {
		return m
	}
	return ""
}

func WithPath(ctx context.Context, path string) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(String("path", path))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, pathKey, path)
}

func GetPath(ctx context.Context) string {
	if p, ok := ctx.Value(pathKey).(string); ok {
		return p
	}
	return ""
}

func WithStatusCode(ctx context.Context, statusCode int) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(Int("status_code", statusCode))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, statusCodeKey, statusCode)
}

func GetStatusCode(ctx context.Context) int {
	if code, ok := ctx.Value(statusCodeKey).(int); ok {
		return code
	}
	return 0
}

func WithDuration(ctx context.Context, duration time.Duration) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(Duration("duration", duration))
	ctx = WithLogger(ctx, newLogger)
	return context.WithValue(ctx, durationKey, duration)
}

func GetDuration(ctx context.Context) time.Duration {
	if d, ok := ctx.Value(durationKey).(time.Duration); ok {
		return d
	}
	return 0
}


func GetError(ctx context.Context) error {
	if err, ok := ctx.Value(errorKey).(error); ok {
		return err
	}
	return nil
}

func WithStartTime(ctx context.Context, startTime time.Time) context.Context {
	return context.WithValue(ctx, startTimeKey, startTime)
}

func GetStartTime(ctx context.Context) time.Time {
	if t, ok := ctx.Value(startTimeKey).(time.Time); ok {
		return t
	}
	return time.Time{}
}

func WithFields(ctx context.Context, fields ...Field) context.Context {
	logger := FromContext(ctx)
	newLogger := logger.With(fields...)
	return WithLogger(ctx, newLogger)
}

func NewRequestContext(ctx context.Context) context.Context {
	requestID := uuid.New().String()
	return WithRequestID(ctx, requestID)
}

func NewSessionContext(ctx context.Context) context.Context {
	sessionID := uuid.New().String()
	return WithSessionID(ctx, sessionID)
}

func NewCampaignContext(ctx context.Context, campaignID string) context.Context {
	ctx = WithCampaignID(ctx, campaignID)
	ctx = WithStartTime(ctx, time.Now())
	return ctx
}

func NewWorkerContext(ctx context.Context, workerID string) context.Context {
	return WithWorkerID(ctx, workerID)
}

func ExtractAllFields(ctx context.Context) []Field {
	fields := make([]Field, 0)

	if requestID := GetRequestID(ctx); requestID != "" {
		fields = append(fields, String("request_id", requestID))
	}

	if sessionID := GetSessionID(ctx); sessionID != "" {
		fields = append(fields, String("session_id", sessionID))
	}

	if campaignID := GetCampaignID(ctx); campaignID != "" {
		fields = append(fields, String("campaign_id", campaignID))
	}

	if accountID := GetAccountID(ctx); accountID != "" {
		fields = append(fields, String("account_id", accountID))
	}

	if userID := GetUserID(ctx); userID != "" {
		fields = append(fields, String("user_id", userID))
	}

	if traceID := GetTraceID(ctx); traceID != "" {
		fields = append(fields, String("trace_id", traceID))
	}

	if spanID := GetSpanID(ctx); spanID != "" {
		fields = append(fields, String("span_id", spanID))
	}

	if tenantID := GetTenantID(ctx); tenantID != "" {
		fields = append(fields, String("tenant_id", tenantID))
	}

	if operation := GetOperation(ctx); operation != "" {
		fields = append(fields, String("operation", operation))
	}

	if component := GetComponent(ctx); component != "" {
		fields = append(fields, String("component", component))
	}

	if workerID := GetWorkerID(ctx); workerID != "" {
		fields = append(fields, String("worker_id", workerID))
	}

	if recipient := GetRecipient(ctx); recipient != "" {
		fields = append(fields, String("recipient", recipient))
	}

	if templateID := GetTemplateID(ctx); templateID != "" {
		fields = append(fields, String("template_id", templateID))
	}

	if provider := GetProvider(ctx); provider != "" {
		fields = append(fields, String("provider", provider))
	}

	if proxyID := GetProxyID(ctx); proxyID != "" {
		fields = append(fields, String("proxy_id", proxyID))
	}

	if ipAddress := GetIPAddress(ctx); ipAddress != "" {
		fields = append(fields, String("ip_address", ipAddress))
	}

	if userAgent := GetUserAgent(ctx); userAgent != "" {
		fields = append(fields, String("user_agent", userAgent))
	}

	if method := GetMethod(ctx); method != "" {
		fields = append(fields, String("method", method))
	}

	if path := GetPath(ctx); path != "" {
		fields = append(fields, String("path", path))
	}

	if statusCode := GetStatusCode(ctx); statusCode != 0 {
		fields = append(fields, Int("status_code", statusCode))
	}

	if duration := GetDuration(ctx); duration != 0 {
		fields = append(fields, Duration("duration", duration))
	}

	if err := GetError(ctx); err != nil {
		fields = append(fields, Error(err))
	}

	if startTime := GetStartTime(ctx); !startTime.IsZero() {
		fields = append(fields, Time("start_time", startTime))
	}

	return fields
}

func LogDebug(ctx context.Context, msg string, fields ...Field) {
	FromContext(ctx).Debug(msg, fields...)
}

func LogInfo(ctx context.Context, msg string, fields ...Field) {
	FromContext(ctx).Info(msg, fields...)
}

func LogWarn(ctx context.Context, msg string, fields ...Field) {
	FromContext(ctx).Warn(msg, fields...)
}

func LogError(ctx context.Context, msg string, fields ...Field) {
	FromContext(ctx).Error(msg, fields...)
}

func LogFatal(ctx context.Context, msg string, fields ...Field) {
	FromContext(ctx).Fatal(msg, fields...)
}

func LogPanic(ctx context.Context, msg string, fields ...Field) {
	FromContext(ctx).Panic(msg, fields...)
}

func LogDebugf(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Debugf(format, args...)
}

func LogInfof(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Infof(format, args...)
}

func LogWarnf(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Warnf(format, args...)
}

func LogErrorf(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Errorf(format, args...)
}

func LogFatalf(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Fatalf(format, args...)
}

func LogPanicf(ctx context.Context, format string, args ...interface{}) {
	FromContext(ctx).Panicf(format, args...)
}

func StartOperation(ctx context.Context, operation string) context.Context {
	ctx = WithOperation(ctx, operation)
	ctx = WithStartTime(ctx, time.Now())
	LogInfo(ctx, fmt.Sprintf("starting operation: %s", operation))
	return ctx
}

func EndOperation(ctx context.Context, err error) {
	startTime := GetStartTime(ctx)
	if !startTime.IsZero() {
		duration := time.Since(startTime)
		ctx = WithDuration(ctx, duration)
	}

	operation := GetOperation(ctx)
	if err != nil {
		LogError(ctx, fmt.Sprintf("operation failed: %s", operation), Error(err))
	} else {
		LogInfo(ctx, fmt.Sprintf("operation completed: %s", operation))
	}
}

func MeasureOperation(ctx context.Context, operation string, fn func() error) error {
	ctx = StartOperation(ctx, operation)
	err := fn()
	EndOperation(ctx, err)
	return err
}

func PropagateContext(parent context.Context) context.Context {
	ctx := context.Background()
	
	logger := FromContext(parent)
	ctx = WithLogger(ctx, logger)

	if requestID := GetRequestID(parent); requestID != "" {
		ctx = context.WithValue(ctx, requestIDKey, requestID)
	}

	if sessionID := GetSessionID(parent); sessionID != "" {
		ctx = context.WithValue(ctx, sessionIDKey, sessionID)
	}

	if campaignID := GetCampaignID(parent); campaignID != "" {
		ctx = context.WithValue(ctx, campaignIDKey, campaignID)
	}

	if traceID := GetTraceID(parent); traceID != "" {
		ctx = context.WithValue(ctx, traceIDKey, traceID)
	}

	return ctx
}

func CloneContext(ctx context.Context) context.Context {
	return PropagateContext(ctx)
}

