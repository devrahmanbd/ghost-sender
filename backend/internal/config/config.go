package config
import (
"fmt"
"os"
"path/filepath"
"strings"
"sync"
"time"
"gopkg.in/yaml.v3"
)
type AppConfig struct {
mu                 sync.RWMutex
App                ApplicationConfig         `yaml:"app"`
Server             ServerConfig              `yaml:"server"`
Database           DatabaseConfig            `yaml:"database"`
Cache              CacheConfig               `yaml:"cache"`
Storage            StorageConfig             `yaml:"storage"`
Email              EmailConfig               `yaml:"email"`
Account            AccountConfig             `yaml:"account"`
Campaign           CampaignConfig            `yaml:"campaign"`
Template           TemplateConfig            `yaml:"template"`
Personalization    PersonalizationConfig     `yaml:"personalization"`
Attachment         AttachmentConfig          `yaml:"attachment"`
Proxy              ProxyConfig               `yaml:"proxy"`
RateLimit          RateLimitConfig           `yaml:"rate_limit"`
Worker             WorkerConfig              `yaml:"worker"`
Notification       NotificationConfig        `yaml:"notification"`
Logging            LoggingConfig             `yaml:"logging"`
Security           SecurityConfig            `yaml:"security"`
Monitoring         MonitoringConfig          `yaml:"monitoring"`
Cleanup            CleanupConfig             `yaml:"cleanup"`
Providers          map[string]ProviderConfig `yaml:"providers"`
RotationStrategies RotationStrategyConfig    `yaml:"rotation_strategies"`
UpdatedAt          time.Time                 `yaml:"updated_at"`
configPath         string
loadedAt           time.Time
}
type ApplicationConfig struct {
Name        string `yaml:"name" env:"APP_NAME"`
Version     string `yaml:"version" env:"APP_VERSION"`
Environment string `yaml:"environment" env:"APP_ENV"`
Debug       bool   `yaml:"debug" env:"APP_DEBUG"`
TenantID    string `yaml:"tenant_id" env:"TENANT_ID"`
Timezone    string `yaml:"timezone" env:"APP_TIMEZONE"`
Locale      string `yaml:"locale" env:"APP_LOCALE"`
}
type ServerConfig struct {
Host            string        `yaml:"host" env:"SERVER_HOST"`
Port            int           `yaml:"port" env:"SERVER_PORT"`
Mode            string        `yaml:"mode" env:"SERVER_MODE"`
TLSEnabled      bool          `yaml:"tls_enabled" env:"SERVER_TLS_ENABLED"`
TLSCertFile     string        `yaml:"tls_cert_file" env:"SERVER_TLS_CERT"`
TLSKeyFile      string        `yaml:"tls_key_file" env:"SERVER_TLS_KEY"`
ReadTimeout     time.Duration `yaml:"read_timeout" env:"SERVER_READ_TIMEOUT"`
WriteTimeout    time.Duration `yaml:"write_timeout" env:"SERVER_WRITE_TIMEOUT"`
IdleTimeout     time.Duration `yaml:"idle_timeout" env:"SERVER_IDLE_TIMEOUT"`
MaxHeaderBytes  int           `yaml:"max_header_bytes" env:"SERVER_MAX_HEADER_BYTES"`
EnableCORS      bool          `yaml:"enable_cors" env:"SERVER_ENABLE_CORS"`
AllowedOrigins  []string      `yaml:"allowed_origins" env:"SERVER_ALLOWED_ORIGINS"`
EnableWebSocket bool          `yaml:"enable_websocket" env:"SERVER_ENABLE_WEBSOCKET"`
ShutdownTimeout  time.Duration `yaml:"shutdown_timeout" env:"SERVER_SHUTDOWN_TIMEOUT"`
TrustedProxies  []string      `yaml:"trusted_proxies" env:"SERVER_TRUSTED_PROXIES"`
}
type DatabaseConfig struct {
Driver          string        `yaml:"driver" env:"DB_DRIVER"`
Host            string        `yaml:"host" env:"DB_HOST"`
Port            int           `yaml:"port" env:"DB_PORT"`
Database        string        `yaml:"database" env:"DB_DATABASE"`
Username        string        `yaml:"username" env:"DB_USERNAME"`
Password        string        `yaml:"password" env:"DB_PASSWORD"`
SSLMode         string        `yaml:"ssl_mode" env:"DB_SSL_MODE"`
MaxOpenConns    int           `yaml:"max_open_conns" env:"DB_MAX_OPEN_CONNS"`
MaxIdleConns    int           `yaml:"max_idle_conns" env:"DB_MAX_IDLE_CONNS"`
ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME"`
MigrationsPath  string        `yaml:"migrations_path" env:"DB_MIGRATIONS_PATH"`
AutoMigrate     bool          `yaml:"auto_migrate" env:"DB_AUTO_MIGRATE"`
}
type CacheConfig struct {
Type              string        `yaml:"type" env:"CACHE_TYPE"`
Host              string        `yaml:"host" env:"CACHE_HOST"`
Port              int           `yaml:"port" env:"CACHE_PORT"`
Password          string        `yaml:"password" env:"CACHE_PASSWORD"`
Database          int           `yaml:"database" env:"CACHE_DATABASE"`
MaxRetries        int           `yaml:"max_retries" env:"CACHE_MAX_RETRIES"`
PoolSize          int           `yaml:"pool_size" env:"CACHE_POOL_SIZE"`
MinIdleConns      int           `yaml:"min_idle_conns" env:"CACHE_MIN_IDLE_CONNS"`
DefaultExpiration time.Duration `yaml:"default_expiration" env:"CACHE_DEFAULT_EXPIRATION"`
CleanupInterval   time.Duration `yaml:"cleanup_interval" env:"CACHE_CLEANUP_INTERVAL"`
EnableCompression bool          `yaml:"enable_compression" env:"CACHE_ENABLE_COMPRESSION"`
}
type StorageConfig struct {
Type              string   `yaml:"type" env:"STORAGE_TYPE"`
BasePath          string   `yaml:"base_path" env:"STORAGE_BASE_PATH"`
TempPath          string   `yaml:"temp_path" env:"STORAGE_TEMP_PATH"`
UploadPath        string   `yaml:"upload_path" env:"STORAGE_UPLOAD_PATH"`
TemplatePath      string   `yaml:"template_path" env:"STORAGE_TEMPLATE_PATH"`
AttachmentPath    string   `yaml:"attachment_path" env:"STORAGE_ATTACHMENT_PATH"`
LogPath           string   `yaml:"log_path" env:"STORAGE_LOG_PATH"`
BackupPath        string   `yaml:"backup_path" env:"STORAGE_BACKUP_PATH"`
MaxUploadSizeMB   int      `yaml:"max_upload_size_mb" env:"STORAGE_MAX_UPLOAD_SIZE_MB"`
AllowedExtensions []string `yaml:"allowed_extensions" env:"STORAGE_ALLOWED_EXTENSIONS"`
}
type EmailConfig struct {
FromName           string        `yaml:"from_name" env:"EMAIL_FROM_NAME"`
FromEmail          string        `yaml:"from_email" env:"EMAIL_FROM_EMAIL"`
ReplyTo            string        `yaml:"reply_to" env:"EMAIL_REPLY_TO"`
ReturnPath         string        `yaml:"return_path" env:"EMAIL_RETURN_PATH"`
DefaultSubject     string        `yaml:"default_subject" env:"EMAIL_DEFAULT_SUBJECT"`
DefaultCharset     string        `yaml:"default_charset" env:"EMAIL_DEFAULT_CHARSET"`
DefaultContentType string        `yaml:"default_content_type" env:"EMAIL_DEFAULT_CONTENT_TYPE"`
SendTimeout        time.Duration `yaml:"send_timeout" env:"EMAIL_SEND_TIMEOUT"`
ConnectionTimeout  time.Duration `yaml:"connection_timeout" env:"EMAIL_CONNECTION_TIMEOUT"`
RetryAttempts      int           `yaml:"retry_attempts" env:"EMAIL_RETRY_ATTEMPTS"`
RetryDelay         time.Duration `yaml:"retry_delay" env:"EMAIL_RETRY_DELAY"`
EnableDKIM         bool          `yaml:"enable_dkim" env:"EMAIL_ENABLE_DKIM"`
DKIMSelector       string        `yaml:"dkim_selector" env:"EMAIL_DKIM_SELECTOR"`
DKIMPrivateKeyPath string        `yaml:"dkim_private_key_path" env:"EMAIL_DKIM_PRIVATE_KEY_PATH"`
EnableFBL          bool          `yaml:"enable_fbl" env:"EMAIL_ENABLE_FBL"`
FBLIdentifier      string        `yaml:"fbl_identifier" env:"EMAIL_FBL_IDENTIFIER"`
EnableUnsubscribe  bool          `yaml:"enable_unsubscribe" env:"EMAIL_ENABLE_UNSUBSCRIBE"`
UnsubscribeURL     string        `yaml:"unsubscribe_url" env:"EMAIL_UNSUBSCRIBE_URL"`
TrackOpens         bool          `yaml:"track_opens" env:"EMAIL_TRACK_OPENS"`
TrackClicks        bool          `yaml:"track_clicks" env:"EMAIL_TRACK_CLICKS"`
}
type AccountConfig struct {
RotationStrategy    string        `yaml:"rotation_strategy" env:"ACCOUNT_ROTATION_STRATEGY"`
RotationLimit       int           `yaml:"rotation_limit" env:"ACCOUNT_ROTATION_LIMIT"`
DailyLimit          int           `yaml:"daily_limit" env:"ACCOUNT_DAILY_LIMIT"`
HourlyLimit         int           `yaml:"hourly_limit" env:"ACCOUNT_HOURLY_LIMIT"`
CooldownDuration    time.Duration `yaml:"cooldown_duration" env:"ACCOUNT_COOLDOWN_DURATION"`
HealthCheckInterval time.Duration `yaml:"health_check_interval" env:"ACCOUNT_HEALTH_CHECK_INTERVAL"`
HealthThreshold     float64       `yaml:"health_threshold" env:"ACCOUNT_HEALTH_THRESHOLD"`
AutoSuspend         bool          `yaml:"auto_suspend" env:"ACCOUNT_AUTO_SUSPEND"`
SuspendThreshold    int           `yaml:"suspend_threshold" env:"ACCOUNT_SUSPEND_THRESHOLD"`
ConsecutiveFailures int           `yaml:"consecutive_failures" env:"ACCOUNT_CONSECUTIVE_FAILURES"`
EnableWeighting     bool          `yaml:"enable_weighting" env:"ACCOUNT_ENABLE_WEIGHTING"`
TestConnection      bool          `yaml:"test_connection" env:"ACCOUNT_TEST_CONNECTION"`
}
type CampaignConfig struct {
MaxConcurrent          int           `yaml:"max_concurrent" env:"CAMPAIGN_MAX_CONCURRENT"`
DefaultPriority        string        `yaml:"default_priority" env:"CAMPAIGN_DEFAULT_PRIORITY"`
EnableScheduling       bool          `yaml:"enable_scheduling" env:"CAMPAIGN_ENABLE_SCHEDULING"`
EnableCheckpointing    bool          `yaml:"enable_checkpointing" env:"CAMPAIGN_ENABLE_CHECKPOINTING"`
CheckpointInterval     time.Duration `yaml:"checkpoint_interval" env:"CAMPAIGN_CHECKPOINT_INTERVAL"`
ProgressUpdateInterval time.Duration `yaml:"progress_update_interval" env:"CAMPAIGN_PROGRESS_UPDATE_INTERVAL"`
AutoRetry              bool          `yaml:"auto_retry" env:"CAMPAIGN_AUTO_RETRY"`
RetryFailedAfter       time.Duration `yaml:"retry_failed_after" env:"CAMPAIGN_RETRY_FAILED_AFTER"`
MaxRetryAttempts       int           `yaml:"max_retry_attempts" env:"CAMPAIGN_MAX_RETRY_ATTEMPTS"`
EnableNotifications    bool          `yaml:"enable_notifications" env:"CAMPAIGN_ENABLE_NOTIFICATIONS"`
NotifyOnStart          bool          `yaml:"notify_on_start" env:"CAMPAIGN_NOTIFY_ON_START"`
NotifyOnComplete       bool          `yaml:"notify_on_complete" env:"CAMPAIGN_NOTIFY_ON_COMPLETE"`
NotifyOnError          bool          `yaml:"notify_on_error" env:"CAMPAIGN_NOTIFY_ON_ERROR"`
NotifyOnMilestone      bool          `yaml:"notify_on_milestone" env:"CAMPAIGN_NOTIFY_ON_MILESTONE"`
MilestonePercents      []int         `yaml:"milestone_percents" env:"CAMPAIGN_MILESTONE_PERCENTS"`
}
type TemplateConfig struct {
EnableRotation     bool              `yaml:"enable_rotation" env:"TEMPLATE_ENABLE_ROTATION"`
RotationStrategy   string            `yaml:"rotation_strategy" env:"TEMPLATE_ROTATION_STRATEGY"`
EnableCaching      bool              `yaml:"enable_caching" env:"TEMPLATE_ENABLE_CACHING"`
CacheTTL           time.Duration     `yaml:"cache_ttl" env:"TEMPLATE_CACHE_TTL"`
EnableValidation   bool              `yaml:"enable_validation" env:"TEMPLATE_ENABLE_VALIDATION"`
EnableSpamCheck    bool              `yaml:"enable_spam_check" env:"TEMPLATE_ENABLE_SPAM_CHECK"`
SpamScoreThreshold float64           `yaml:"spam_score_threshold" env:"TEMPLATE_SPAM_SCORE_THRESHOLD"`
MaxTemplateSizeKB  int               `yaml:"max_template_size_kb" env:"TEMPLATE_MAX_TEMPLATE_SIZE_KB"`
AllowedTags        []string          `yaml:"allowed_tags" env:"TEMPLATE_ALLOWED_TAGS"`
DefaultVariables   map[string]string `yaml:"default_variables"`
}
type PersonalizationConfig struct {
EnableSmartExtraction  bool     `yaml:"enable_smart_extraction" env:"PERSONALIZATION_ENABLE_SMART_EXTRACTION"`
EnableRandomGeneration bool     `yaml:"enable_random_generation" env:"PERSONALIZATION_ENABLE_RANDOM_GENERATION"`
EnableDateFormatting   bool     `yaml:"enable_date_formatting" env:"PERSONALIZATION_ENABLE_DATE_FORMATTING"`
EnableTimeBasedContent bool     `yaml:"enable_time_based_content" env:"PERSONALIZATION_ENABLE_TIME_BASED_CONTENT"`
DefaultTimezone        string   `yaml:"default_timezone" env:"PERSONALIZATION_DEFAULT_TIMEZONE"`
DefaultDateFormat      string   `yaml:"default_date_format" env:"PERSONALIZATION_DEFAULT_DATE_FORMAT"`
EnableCustomFields     bool     `yaml:"enable_custom_fields" env:"PERSONALIZATION_ENABLE_CUSTOM_FIELDS"`
CustomFieldRotation    string   `yaml:"custom_field_rotation" env:"PERSONALIZATION_CUSTOM_FIELD_ROTATION"`
SenderNameRotation     string   `yaml:"sender_name_rotation" env:"PERSONALIZATION_SENDER_NAME_ROTATION"`
SubjectRotation        string   `yaml:"subject_rotation" env:"PERSONALIZATION_SUBJECT_ROTATION"`
SenderNames            []string `yaml:"sender_names" env:"PERSONALIZATION_SENDER_NAMES"`
SubjectLines           []string `yaml:"subject_lines" env:"PERSONALIZATION_SUBJECT_LINES"`
}
type AttachmentConfig struct {
Enabled               bool     `yaml:"enabled" env:"ATTACHMENT_ENABLED"`
EnableRotation        bool     `yaml:"enable_rotation" env:"ATTACHMENT_ENABLE_ROTATION"`
RotationStrategy      string   `yaml:"rotation_strategy" env:"ATTACHMENT_ROTATION_STRATEGY"`
EnableCaching         bool     `yaml:"enable_caching" env:"ATTACHMENT_ENABLE_CACHING"`
CachePath             string   `yaml:"cache_path" env:"ATTACHMENT_CACHE_PATH"`
MaxCacheSizeMB        int      `yaml:"max_cache_size_mb" env:"ATTACHMENT_MAX_CACHE_SIZE_MB"`
MaxAttachmentSizeMB   int      `yaml:"max_attachment_size_mb" env:"ATTACHMENT_MAX_ATTACHMENT_SIZE_MB"`
SupportedFormats      []string `yaml:"supported_formats" env:"ATTACHMENT_SUPPORTED_FORMATS"`
ConversionBackend     string   `yaml:"conversion_backend" env:"ATTACHMENT_CONVERSION_BACKEND"`
EnablePDFConversion   bool     `yaml:"enable_pdf_conversion" env:"ATTACHMENT_ENABLE_PDF_CONVERSION"`
EnableImageConversion bool     `yaml:"enable_image_conversion" env:"ATTACHMENT_ENABLE_IMAGE_CONVERSION"`
ImageQuality          int      `yaml:"image_quality" env:"ATTACHMENT_IMAGE_QUALITY"`
ImageFormat           string   `yaml:"image_format" env:"ATTACHMENT_IMAGE_FORMAT"`
}
type ProxyConfig struct {
Enabled             bool          `yaml:"enabled" env:"PROXY_ENABLED"`
RotationStrategy    string        `yaml:"rotation_strategy" env:"PROXY_ROTATION_STRATEGY"`
EnableHealthCheck   bool          `yaml:"enable_health_check" env:"PROXY_ENABLE_HEALTH_CHECK"`
HealthCheckInterval time.Duration `yaml:"health_check_interval" env:"PROXY_HEALTH_CHECK_INTERVAL"`
HealthCheckTimeout  time.Duration `yaml:"health_check_timeout" env:"PROXY_HEALTH_CHECK_TIMEOUT"`
MaxRetries          int           `yaml:"max_retries" env:"PROXY_MAX_RETRIES"`
RetryDelay          time.Duration `yaml:"retry_delay" env:"PROXY_RETRY_DELAY"`
ConnectionTimeout   time.Duration `yaml:"connection_timeout" env:"PROXY_CONNECTION_TIMEOUT"`
IdleConnTimeout     time.Duration `yaml:"idle_conn_timeout" env:"PROXY_IDLE_CONN_TIMEOUT"`
}
type RateLimitConfig struct {
Enabled                bool          `yaml:"enabled" env:"RATE_LIMIT_ENABLED"`
GlobalRPS              float64       `yaml:"global_rps" env:"RATE_LIMIT_GLOBAL_RPS"`
PerAccountRPS          float64       `yaml:"per_account_rps" env:"RATE_LIMIT_PER_ACCOUNT_RPS"`
BurstSize              int           `yaml:"burst_size" env:"RATE_LIMIT_BURST_SIZE"`
EnableAdaptive         bool          `yaml:"enable_adaptive" env:"RATE_LIMIT_ENABLE_ADAPTIVE"`
AdaptiveMinRPS         float64       `yaml:"adaptive_min_rps" env:"RATE_LIMIT_ADAPTIVE_MIN_RPS"`
AdaptiveMaxRPS         float64       `yaml:"adaptive_max_rps" env:"RATE_LIMIT_ADAPTIVE_MAX_RPS"`
AdaptiveAdjustInterval time.Duration `yaml:"adaptive_adjust_interval" env:"RATE_LIMIT_ADAPTIVE_ADJUST_INTERVAL"`
EnableDistributed      bool          `yaml:"enable_distributed" env:"RATE_LIMIT_ENABLE_DISTRIBUTED"`
APIRateLimit           int           `yaml:"api_rate_limit" env:"RATE_LIMIT_API_RATE_LIMIT"`
APIRateLimitWindow     time.Duration `yaml:"api_rate_limit_window" env:"RATE_LIMIT_API_RATE_LIMIT_WINDOW"`
}
type WorkerConfig struct {
MinWorkers              int           `yaml:"min_workers" env:"WORKER_MIN_WORKERS"`
MaxWorkers              int           `yaml:"max_workers" env:"WORKER_MAX_WORKERS"`
DefaultWorkers          int           `yaml:"default_workers" env:"WORKER_DEFAULT_WORKERS"`
QueueSize               int           `yaml:"queue_size" env:"WORKER_QUEUE_SIZE"`
BatchSize               int           `yaml:"batch_size" env:"WORKER_BATCH_SIZE"`
EnableAutoScaling       bool          `yaml:"enable_auto_scaling" env:"WORKER_ENABLE_AUTO_SCALING"`
ScaleUpThreshold        float64       `yaml:"scale_up_threshold" env:"WORKER_SCALE_UP_THRESHOLD"`
ScaleDownThreshold      float64       `yaml:"scale_down_threshold" env:"WORKER_SCALE_DOWN_THRESHOLD"`
IdleTimeout             time.Duration `yaml:"idle_timeout" env:"WORKER_IDLE_TIMEOUT"`
GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout" env:"WORKER_GRACEFUL_SHUTDOWN_TIMEOUT"`
}
type NotificationConfig struct {
Enabled           bool          `yaml:"enabled" env:"NOTIFICATION_ENABLED"`
Channel           string        `yaml:"channel" env:"NOTIFICATION_CHANNEL"`
TelegramBotToken  string        `yaml:"telegram_bot_token" env:"TELEGRAM_BOT_TOKEN"`
TelegramChatID    string        `yaml:"telegram_chat_id" env:"TELEGRAM_CHAT_ID"`
TelegramParseMode string        `yaml:"telegram_parse_mode" env:"TELEGRAM_PARSE_MODE"`
EmailRecipients   []string      `yaml:"email_recipients" env:"NOTIFICATION_EMAIL_RECIPIENTS"`
WebhookURL        string        `yaml:"webhook_url" env:"NOTIFICATION_WEBHOOK_URL"`
WebhookSecret     string        `yaml:"webhook_secret" env:"NOTIFICATION_WEBHOOK_SECRET"`
EnableRetry       bool          `yaml:"enable_retry" env:"NOTIFICATION_ENABLE_RETRY"`
MaxRetries        int           `yaml:"max_retries" env:"NOTIFICATION_MAX_RETRIES"`
RetryDelay        time.Duration `yaml:"retry_delay" env:"NOTIFICATION_RETRY_DELAY"`
}
type LoggingConfig struct {
Level                string   `yaml:"level" env:"LOG_LEVEL"`
Format               string   `yaml:"format" env:"LOG_FORMAT"`
OutputPaths          []string `yaml:"output_paths" env:"LOG_OUTPUT_PATHS"`
ErrorOutputPaths     []string `yaml:"error_output_paths" env:"LOG_ERROR_OUTPUT_PATHS"`
EnableCampaignLog    bool     `yaml:"enable_campaign_log" env:"LOG_ENABLE_CAMPAIGN_LOG"`
EnableDebugLog       bool     `yaml:"enable_debug_log" env:"LOG_ENABLE_DEBUG_LOG"`
EnableFailedLog      bool     `yaml:"enable_failed_log" env:"LOG_ENABLE_FAILED_LOG"`
EnableSuccessLog     bool     `yaml:"enable_success_log" env:"LOG_ENABLE_SUCCESS_LOG"`
EnableSystemLog      bool     `yaml:"enable_system_log" env:"LOG_ENABLE_SYSTEM_LOG"`
EnablePerformanceLog bool     `yaml:"enable_performance_log" env:"LOG_ENABLE_PERFORMANCE_LOG"`
EnableAccessLog      bool     `yaml:"enable_access_log" env:"LOG_ENABLE_ACCESS_LOG"`
MaxSizeMB            int      `yaml:"max_size_mb" env:"LOG_MAX_SIZE_MB"`
MaxBackups           int      `yaml:"max_backups" env:"LOG_MAX_BACKUPS"`
MaxAgeDays           int      `yaml:"max_age_days" env:"LOG_MAX_AGE_DAYS"`
Compress             bool     `yaml:"compress" env:"LOG_COMPRESS"`
}
type SecurityConfig struct {
EnableAuth          bool          `yaml:"enable_auth" env:"SECURITY_ENABLE_AUTH"`
JWTSecret           string        `yaml:"jwt_secret" env:"JWT_SECRET"`
JWTExpiration       time.Duration `yaml:"jwt_expiration" env:"JWT_EXPIRATION"`
APIKey              string        `yaml:"api_key" env:"API_KEY"`
EnableAPIKey        bool          `yaml:"enable_api_key" env:"SECURITY_ENABLE_API_KEY"`
EncryptionKey       string        `yaml:"encryption_key" env:"ENCRYPTION_KEY"`
EnableEncryption    bool          `yaml:"enable_encryption" env:"SECURITY_ENABLE_ENCRYPTION"`
EncryptionAlgorithm string        `yaml:"encryption_algorithm" env:"SECURITY_ENCRYPTION_ALGORITHM"`
HashAlgorithm       string        `yaml:"hash_algorithm" env:"SECURITY_HASH_ALGORITHM"`
EnableCSRF          bool          `yaml:"enable_csrf" env:"SECURITY_ENABLE_CSRF"`
CSRFSecret          string        `yaml:"csrf_secret" env:"CSRF_SECRET"`
SessionSecret       string        `yaml:"session_secret" env:"SESSION_SECRET"`
SessionMaxAge       time.Duration `yaml:"session_max_age" env:"SESSION_MAX_AGE"`
EnableRateLimit     bool          `yaml:"enable_rate_limit" env:"SECURITY_ENABLE_RATE_LIMIT"`
TrustedProxies      []string      `yaml:"trusted_proxies" env:"SECURITY_TRUSTED_PROXIES"`
}
type MonitoringConfig struct {
Enabled             bool          `yaml:"enabled" env:"MONITORING_ENABLED"`
EnableMetrics       bool          `yaml:"enable_metrics" env:"MONITORING_ENABLE_METRICS"`
MetricsPort         int           `yaml:"metrics_port" env:"MONITORING_METRICS_PORT"`
MetricsPath         string        `yaml:"metrics_path" env:"MONITORING_METRICS_PATH"`
EnableProfiling     bool          `yaml:"enable_profiling" env:"MONITORING_ENABLE_PROFILING"`
ProfilingPort       int           `yaml:"profiling_port" env:"MONITORING_PROFILING_PORT"`
EnableHealthCheck   bool          `yaml:"enable_health_check" env:"MONITORING_ENABLE_HEALTH_CHECK"`
HealthCheckPath     string        `yaml:"health_check_path" env:"MONITORING_HEALTH_CHECK_PATH"`
HealthCheckInterval time.Duration `yaml:"health_check_interval" env:"MONITORING_HEALTH_CHECK_INTERVAL"`
EnableTracing       bool          `yaml:"enable_tracing" env:"MONITORING_ENABLE_TRACING"`
TracingEndpoint     string        `yaml:"tracing_endpoint" env:"MONITORING_TRACING_ENDPOINT"`
}
type CleanupConfig struct {
Enabled              bool          `yaml:"enabled" env:"CLEANUP_ENABLED"`
CleanupInterval      time.Duration `yaml:"cleanup_interval" env:"CLEANUP_CLEANUP_INTERVAL"`
DeleteCompletedAfter time.Duration `yaml:"delete_completed_after" env:"CLEANUP_DELETE_COMPLETED_AFTER"`
DeleteFailedAfter    time.Duration `yaml:"delete_failed_after" env:"CLEANUP_DELETE_FAILED_AFTER"`
DeleteLogsAfter      time.Duration `yaml:"delete_logs_after" env:"CLEANUP_DELETE_LOGS_AFTER"`
DeleteTempFilesAfter time.Duration `yaml:"delete_temp_files_after" env:"CLEANUP_DELETE_TEMP_FILES_AFTER"`
DeleteCacheAfter     time.Duration `yaml:"delete_cache_after" env:"CLEANUP_DELETE_CACHE_AFTER"`
CleanupBatchSize     int           `yaml:"cleanup_batch_size" env:"CLEANUP_CLEANUP_BATCH_SIZE"`
EnableAutoBackup     bool          `yaml:"enable_auto_backup" env:"CLEANUP_ENABLE_AUTO_BACKUP"`
BackupInterval       time.Duration `yaml:"backup_interval" env:"CLEANUP_BACKUP_INTERVAL"`
}
type ProviderConfig struct {
Name                     string        `yaml:"name"`
Type                     string        `yaml:"type"`
Host                     string        `yaml:"host"`
Port                     int           `yaml:"port"`
Username                 string        `yaml:"username"`
Password                 string        `yaml:"password"`
UseSSL                   bool          `yaml:"use_ssl"`
UseTLS                   bool          `yaml:"use_tls"`
AuthMethod               string        `yaml:"auth_method"`
OAuthClientID            string        `yaml:"oauth_client_id"`
OAuthClientSecret        string        `yaml:"oauth_client_secret"`
OAuthTokenURL            string        `yaml:"oauth_token_url"`
OAuthScopes              []string      `yaml:"oauth_scopes"`
MaxConnectionsPerAccount int           `yaml:"max_connections_per_account"`
ConnectionTimeout        time.Duration `yaml:"connection_timeout"`
SendTimeout              time.Duration `yaml:"send_timeout"`
}
type RotationStrategyConfig struct {
Account     StrategySettings `yaml:"account"`
Template    StrategySettings `yaml:"template"`
SenderName  StrategySettings `yaml:"sender_name"`
Subject     StrategySettings `yaml:"subject"`
CustomField StrategySettings `yaml:"custom_field"`
Attachment  StrategySettings `yaml:"attachment"`
Proxy       StrategySettings `yaml:"proxy"`
}
type StrategySettings struct {
Strategy    string                 `yaml:"strategy"`
Weights     map[string]int         `yaml:"weights"`
TimeRanges  []TimeRangeRule        `yaml:"time_ranges"`
CustomRules map[string]interface{} `yaml:"custom_rules"`
}
type TimeRangeRule struct {
StartHour int    `yaml:"start_hour"`
EndHour   int    `yaml:"end_hour"`
Value     string `yaml:"value"`
Weight    int    `yaml:"weight"`
}
var (
globalConfig *AppConfig
configOnce   sync.Once
configMutex  sync.RWMutex
)
func New() *AppConfig {
return &AppConfig{
Providers: make(map[string]ProviderConfig),
loadedAt:  time.Now(),
}
}
func Get() *AppConfig {
configMutex.RLock()
defer configMutex.RUnlock()
return globalConfig
}
func Set(cfg *AppConfig) {
configMutex.Lock()
defer configMutex.Unlock()
globalConfig = cfg
}
func Initialize(configPath string) (*AppConfig, error) {
var initErr error
configOnce.Do(func() {
cfg := New()
cfg.configPath = configPath
if err := cfg.LoadDefaults(); err != nil {
initErr = fmt.Errorf("failed to load defaults: %w", err)
return
}
if configPath != "" {
if err := cfg.LoadFromFile(configPath); err != nil {
initErr = fmt.Errorf("failed to load config file: %w", err)
return
}
}
if err := cfg.LoadFromEnv(); err != nil {
initErr = fmt.Errorf("failed to load environment variables: %w", err)
return
}
if err := cfg.Validate(); err != nil {
initErr = fmt.Errorf("config validation failed: %w", err)
return
}
globalConfig = cfg
})
return globalConfig, initErr
}
func (c *AppConfig) LoadDefaults() error {
c.mu.Lock()
defer c.mu.Unlock()
c.App = ApplicationConfig{
Name:        "Email Campaign System",
Version:     "1.0.0",
Environment: "production",
Debug:       false,
Timezone:    "UTC",
Locale:      "en",
}
c.Server = ServerConfig{
Host:           "0.0.0.0",
Port:           8080,
Mode:           "release",
ReadTimeout:    10 * time.Second,
WriteTimeout:   10 * time.Second,
IdleTimeout:    120 * time.Second,
ShutdownTimeout: 30 * time.Second,
MaxHeaderBytes: 1048576,
EnableCORS:     true,
AllowedOrigins: []string{"*"},
}
c.Database = DatabaseConfig{
Driver:          "postgres",
Port:            5432,
SSLMode:         "disable",
MaxOpenConns:    25,
MaxIdleConns:    5,
ConnMaxLifetime: 5 * time.Minute,
MigrationsPath:  "./migrations",
AutoMigrate:     false,
}
c.Cache = CacheConfig{
Type:              "redis",
Port:              6379,
Database:          0,
MaxRetries:        3,
PoolSize:          10,
MinIdleConns:      2,
DefaultExpiration: 1 * time.Hour,
CleanupInterval:   10 * time.Minute,
}
c.Storage = StorageConfig{
Type:              "local",
BasePath:          "./storage",
TempPath:          "./storage/temp",
UploadPath:        "./storage/uploads",
TemplatePath:      "./storage/templates",
AttachmentPath:    "./storage/attachments",
LogPath:           "./storage/logs",
BackupPath:        "./storage/backups",
MaxUploadSizeMB:   100,
AllowedExtensions: []string{".zip", ".html", ".pdf", ".jpg", ".png"},
}
c.Email = EmailConfig{
DefaultCharset:     "UTF-8",
DefaultContentType: "text/html",
SendTimeout:        30 * time.Second,
ConnectionTimeout:  10 * time.Second,
RetryAttempts:      3,
RetryDelay:         5 * time.Second,
EnableFBL:          true,
EnableUnsubscribe:  true,
}
c.Account = AccountConfig{
RotationStrategy:    "round-robin",
RotationLimit:       100,
DailyLimit:          500,
HourlyLimit:         50,
CooldownDuration:    60 * time.Second,
HealthCheckInterval: 5 * time.Minute,
HealthThreshold:     50.0,
AutoSuspend:         true,
SuspendThreshold:    10,
ConsecutiveFailures: 5,
}
c.Campaign = CampaignConfig{
MaxConcurrent:          10,
DefaultPriority:        "normal",
EnableScheduling:       true,
EnableCheckpointing:    true,
CheckpointInterval:     1 * time.Minute,
ProgressUpdateInterval: 5 * time.Second,
AutoRetry:              true,
RetryFailedAfter:       1 * time.Hour,
MaxRetryAttempts:       3,
EnableNotifications:    true,
NotifyOnStart:          true,
NotifyOnComplete:       true,
NotifyOnError:          true,
MilestonePercents:      []int{25, 50, 75},
}
c.Template = TemplateConfig{
EnableRotation:     true,
RotationStrategy:   "sequential",
EnableCaching:      true,
CacheTTL:           1 * time.Hour,
EnableValidation:   true,
EnableSpamCheck:    true,
SpamScoreThreshold: 5.0,
MaxTemplateSizeKB:  500,
}
c.Personalization = PersonalizationConfig{
EnableSmartExtraction:  true,
EnableRandomGeneration: true,
EnableDateFormatting:   true,
EnableTimeBasedContent: true,
DefaultTimezone:        "UTC",
DefaultDateFormat:      "2006-01-02",
EnableCustomFields:     true,
CustomFieldRotation:    "sequential",
SenderNameRotation:     "random",
SubjectRotation:        "sequential",
}
c.Attachment = AttachmentConfig{
Enabled:               true,
EnableRotation:        true,
RotationStrategy:      "sequential",
EnableCaching:         true,
MaxCacheSizeMB:        1024,
MaxAttachmentSizeMB:   25,
SupportedFormats:      []string{"pdf", "jpg", "png", "webp"},
ConversionBackend:     "chromedp",
EnablePDFConversion:   true,
EnableImageConversion: true,
ImageQuality:          85,
ImageFormat:           "jpeg",
}
c.Proxy = ProxyConfig{
Enabled:             false,
RotationStrategy:    "round-robin",
EnableHealthCheck:   true,
HealthCheckInterval: 5 * time.Minute,
HealthCheckTimeout:  10 * time.Second,
MaxRetries:          3,
RetryDelay:          2 * time.Second,
ConnectionTimeout:   30 * time.Second,
IdleConnTimeout:     90 * time.Second,
}
c.RateLimit = RateLimitConfig{
Enabled:                true,
GlobalRPS:              10.0,
PerAccountRPS:          2.0,
BurstSize:              20,
EnableAdaptive:         false,
AdaptiveMinRPS:         1.0,
AdaptiveMaxRPS:         20.0,
AdaptiveAdjustInterval: 1 * time.Minute,
APIRateLimit:           100,
APIRateLimitWindow:     1 * time.Minute,
}
c.Worker = WorkerConfig{
MinWorkers:              1,
MaxWorkers:              10,
DefaultWorkers:          4,
QueueSize:               1000,
BatchSize:               100,
EnableAutoScaling:       true,
ScaleUpThreshold:        0.8,
ScaleDownThreshold:      0.2,
IdleTimeout:             60 * time.Second,
GracefulShutdownTimeout: 30 * time.Second,
}
c.Notification = NotificationConfig{
Enabled:           false,
Channel:           "telegram",
TelegramParseMode: "Markdown",
EnableRetry:       true,
MaxRetries:        3,
RetryDelay:        5 * time.Second,
}
c.Logging = LoggingConfig{
Level:                "info",
Format:               "json",
OutputPaths:          []string{"stdout", "./storage/logs/app.log"},
ErrorOutputPaths:     []string{"stderr", "./storage/logs/error.log"},
EnableCampaignLog:    true,
EnableDebugLog:       true,
EnableFailedLog:      true,
EnableSuccessLog:     true,
EnableSystemLog:      true,
EnablePerformanceLog: true,
EnableAccessLog:      true,
MaxSizeMB:            100,
MaxBackups:           10,
MaxAgeDays:           30,
Compress:             true,
}
c.Security = SecurityConfig{
EnableAuth:          true,
JWTExpiration:       24 * time.Hour,
EnableAPIKey:        true,
EnableEncryption:    true,
EncryptionAlgorithm: "AES-256-GCM",
HashAlgorithm:       "SHA256",
EnableCSRF:          true,
SessionMaxAge:       24 * time.Hour,
EnableRateLimit:     true,
}
c.Monitoring = MonitoringConfig{
Enabled:             true,
EnableMetrics:       true,
MetricsPort:         9090,
MetricsPath:         "/metrics",
EnableProfiling:     false,
ProfilingPort:       6060,
EnableHealthCheck:   true,
HealthCheckPath:     "/health",
HealthCheckInterval: 30 * time.Second,
}
c.Cleanup = CleanupConfig{
Enabled:              true,
CleanupInterval:      1 * time.Hour,
DeleteCompletedAfter: 7 * 24 * time.Hour,
DeleteFailedAfter:    30 * 24 * time.Hour,
DeleteLogsAfter:      90 * 24 * time.Hour,
DeleteTempFilesAfter: 24 * time.Hour,
DeleteCacheAfter:     7 * 24 * time.Hour,
CleanupBatchSize:     1000,
EnableAutoBackup:     true,
BackupInterval:       24 * time.Hour,
}
c.loadedAt = time.Now()
return nil
}
func (c *AppConfig) LoadFromFile(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	c.configPath = path
	c.loadedAt = time.Now()

	return nil
}


func (c *AppConfig) LoadFromEnv() error {
c.mu.Lock()
defer c.mu.Unlock()
if v := os.Getenv("APP_NAME"); v != "" {
c.App.Name = v
}
if v := os.Getenv("APP_ENV"); v != "" {
c.App.Environment = v
}
if v := os.Getenv("APP_DEBUG"); v == "true" {
c.App.Debug = true
}
if v := os.Getenv("TENANT_ID"); v != "" {
c.App.TenantID = v
}
if v := os.Getenv("SERVER_HOST"); v != "" {
c.Server.Host = v
}
if v := os.Getenv("SERVER_PORT"); v != "" {
fmt.Sscanf(v, "%d", &c.Server.Port)
}
if v := os.Getenv("DB_HOST"); v != "" {
c.Database.Host = v
}
if v := os.Getenv("DB_PORT"); v != "" {
fmt.Sscanf(v, "%d", &c.Database.Port)
}
if v := os.Getenv("DB_DATABASE"); v != "" {
c.Database.Database = v
}
if v := os.Getenv("DB_USERNAME"); v != "" {
c.Database.Username = v
}
if v := os.Getenv("DB_PASSWORD"); v != "" {
c.Database.Password = v
}
if v := os.Getenv("CACHE_HOST"); v != "" {
c.Cache.Host = v
}
if v := os.Getenv("CACHE_PORT"); v != "" {
fmt.Sscanf(v, "%d", &c.Cache.Port)
}
if v := os.Getenv("CACHE_PASSWORD"); v != "" {
c.Cache.Password = v
}
if v := os.Getenv("JWT_SECRET"); v != "" {
c.Security.JWTSecret = v
}
if v := os.Getenv("API_KEY"); v != "" {
c.Security.APIKey = v
}
if v := os.Getenv("ENCRYPTION_KEY"); v != "" {
c.Security.EncryptionKey = v
}
if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
c.Notification.TelegramBotToken = v
}
if v := os.Getenv("TELEGRAM_CHAT_ID"); v != "" {
c.Notification.TelegramChatID = v
}
return nil
}
func (c *AppConfig) Validate() error {
c.mu.RLock()
defer c.mu.RUnlock()
if c.Server.Port <= 0 || c.Server.Port > 65535 {
return fmt.Errorf("invalid server port: %d", c.Server.Port)
}
if c.Database.Host == "" {
return fmt.Errorf("database host is required")
}
if c.Worker.MinWorkers > c.Worker.MaxWorkers {
return fmt.Errorf("min workers (%d) cannot be greater than max workers (%d)", c.Worker.MinWorkers, c.Worker.MaxWorkers)
}
if c.Security.EnableAuth && c.Security.JWTSecret == "" {
return fmt.Errorf("JWT secret is required when auth is enabled")
}
return nil
}
func (c *AppConfig) GetConfigPath() string {
c.mu.RLock()
defer c.mu.RUnlock()
return c.configPath
}
func (c *AppConfig) GetLoadedAt() time.Time {
c.mu.RLock()
defer c.mu.RUnlock()
return c.loadedAt
}
func (c *AppConfig) Reload() error {
c.mu.Lock()
defer c.mu.Unlock()
if c.configPath != "" {
if err := c.LoadFromFile(c.configPath); err != nil {
return err
}
}
if err := c.LoadFromEnv(); err != nil {
return err
}
c.loadedAt = time.Now()
return nil
}
func (c *AppConfig) GetDatabaseDSN() string {
c.mu.RLock()
defer c.mu.RUnlock()
return fmt.Sprintf(
"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
c.Database.Host,
c.Database.Port,
c.Database.Username,
c.Database.Password,
c.Database.Database,
c.Database.SSLMode,
)
}
func (c *AppConfig) GetCacheAddress() string {
c.mu.RLock()
defer c.mu.RUnlock()
return fmt.Sprintf("%s:%d", c.Cache.Host, c.Cache.Port)
}
func (c *AppConfig) GetServerAddress() string {
c.mu.RLock()
defer c.mu.RUnlock()
return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
func (c *AppConfig) IsProduction() bool {
c.mu.RLock()
defer c.mu.RUnlock()
return c.App.Environment == "production"
}
func (c *AppConfig) IsDevelopment() bool {
c.mu.RLock()
defer c.mu.RUnlock()
return c.App.Environment == "development"
}
func (c *AppConfig) IsDebug() bool {
c.mu.RLock()
defer c.mu.RUnlock()
return c.App.Debug
}
func (c *AppConfig) GetProvider(name string) (ProviderConfig, bool) {
c.mu.RLock()
defer c.mu.RUnlock()
provider, ok := c.Providers[name]
return provider, ok
}
func (c *AppConfig) SetProvider(name string, provider ProviderConfig) {
c.mu.Lock()
defer c.mu.Unlock()
c.Providers[name] = provider
}
func (c *AppConfig) GetStoragePath(subPath string) string {
c.mu.RLock()
defer c.mu.RUnlock()
return filepath.Join(c.Storage.BasePath, subPath)
}
func (c *AppConfig) EnsureDirectories() error {
c.mu.RLock()
defer c.mu.RUnlock()
dirs := []string{
c.Storage.BasePath,
c.Storage.TempPath,
c.Storage.UploadPath,
c.Storage.TemplatePath,
c.Storage.AttachmentPath,
c.Storage.LogPath,
c.Storage.BackupPath,
}
for _, dir := range dirs {
if err := os.MkdirAll(dir, 0755); err != nil {
return fmt.Errorf("failed to create directory %s: %w", dir, err)
}
}
return nil
}
func GetEnvOrDefault(key, defaultValue string) string {
if value := os.Getenv(key); value != "" {
return value
}
return defaultValue
}
func GetEnvOrDefaultInt(key string, defaultValue int) int {
if value := os.Getenv(key); value != "" {
var result int
if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
return result
}
}
return defaultValue
}
func GetEnvOrDefaultBool(key string, defaultValue bool) bool {
if value := os.Getenv(key); value != "" {
return strings.ToLower(value) == "true" || value == "1"
}
return defaultValue
}
func GetEnvOrDefaultDuration(key string, defaultValue time.Duration) time.Duration {
if value := os.Getenv(key); value != "" {
if duration, err := time.ParseDuration(value); err == nil {
return duration
}
}
return defaultValue
}
func Load(path string) (*AppConfig, error) {
	cfg := New()
	if err := cfg.LoadDefaults(); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}
	
	if path != "" && fileExists(path) {
		if err := cfg.LoadFromFile(path); err != nil {
			return nil, fmt.Errorf("failed to load config file %s: %w", path, err)
		}
		// DEBUG: Check what was loaded
		fmt.Printf("DEBUG after LoadFromFile: Database.Database = '%s'\n", cfg.Database.Database)
		fmt.Printf("DEBUG after LoadFromFile: Database.Username = '%s'\n", cfg.Database.Username)
		fmt.Printf("DEBUG after LoadFromFile: Database.Host = '%s'\n", cfg.Database.Host)
	}
	
	if err := cfg.LoadFromEnv(); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}
	
	// DEBUG: Check after env
	fmt.Printf("DEBUG after LoadFromEnv: Database.Database = '%s'\n", cfg.Database.Database)
	
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	
	if err := cfg.EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}
	
	return cfg, nil
}

func fileExists(path string) bool {
_, err := os.Stat(path)
return err == nil
}
// Clone creates a deep copy of the configuration
func (c *AppConfig) Clone() *AppConfig {
    c.mu.RLock()
    defer c.mu.RUnlock()

    clone := &AppConfig{
        App:                c.App,
        Server:             c.Server,
        Database:           c.Database,
        Cache:              c.Cache,
        Storage:            c.Storage,
        Email:              c.Email,
        Account:            c.Account,
        Campaign:           c.Campaign,
        Template:           c.Template,
        Personalization:    c.Personalization,
        Attachment:         c.Attachment,
        Proxy:              c.Proxy,
        RateLimit:          c.RateLimit,
        Worker:             c.Worker,
        Notification:       c.Notification,
        Logging:            c.Logging,
        Security:           c.Security,
        Monitoring:         c.Monitoring,
        Cleanup:            c.Cleanup,
        RotationStrategies: c.RotationStrategies,
        configPath:         c.configPath,
        loadedAt:           c.loadedAt,
        UpdatedAt:          c.UpdatedAt,
    }

    // Deep copy the Providers map
    if c.Providers != nil {
        clone.Providers = make(map[string]ProviderConfig, len(c.Providers))
        for k, v := range c.Providers {
            clone.Providers[k] = v
        }
    }

    return clone
}
