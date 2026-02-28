package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	
	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "; ")
}

func (e *ValidationErrors) Add(field, message string) {
	*e = append(*e, ValidationError{Field: field, Message: message})
}

func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

type Validator struct {
	errors ValidationErrors
}

func NewValidator() *Validator {
	return &Validator{
		errors: ValidationErrors{},
	}
}

func (v *Validator) Validate(cfg *AppConfig) error {
	v.validateApp(&cfg.App)
	v.validateServer(&cfg.Server)
	v.validateDatabase(&cfg.Database)
	v.validateCache(&cfg.Cache)
	v.validateStorage(&cfg.Storage)
	v.validateEmail(&cfg.Email)
	v.validateAccount(&cfg.Account)
	v.validateCampaign(&cfg.Campaign)
	v.validateTemplate(&cfg.Template)
	v.validatePersonalization(&cfg.Personalization)
	v.validateAttachment(&cfg.Attachment)
	v.validateProxy(&cfg.Proxy)
	v.validateRateLimit(&cfg.RateLimit)
	v.validateWorker(&cfg.Worker)
	v.validateNotification(&cfg.Notification)
	v.validateLogging(&cfg.Logging)
	v.validateSecurity(&cfg.Security)
	v.validateMonitoring(&cfg.Monitoring)
	v.validateCleanup(&cfg.Cleanup)
	v.validateProviders(cfg.Providers)
	
	if v.errors.HasErrors() {
		return v.errors
	}
	
	return nil
}

func (v *Validator) validateApp(cfg *ApplicationConfig) {
	if cfg.Name == "" {
		v.errors.Add("app.name", "application name is required")
	}
	
	if cfg.Version == "" {
		v.errors.Add("app.version", "application version is required")
	}
	
	validEnvs := []string{"development", "staging", "production", "test"}
	if !v.isInSlice(cfg.Environment, validEnvs) {
		v.errors.Add("app.environment", fmt.Sprintf("environment must be one of: %s", strings.Join(validEnvs, ", ")))
	}
	
	if cfg.Timezone != "" {
		if _, err := time.LoadLocation(cfg.Timezone); err != nil {
			v.errors.Add("app.timezone", "invalid timezone")
		}
	}
}

func (v *Validator) validateServer(cfg *ServerConfig) {
	if cfg.Port < 1 || cfg.Port > 65535 {
		v.errors.Add("server.port", "port must be between 1 and 65535")
	}
	
	if cfg.Host == "" {
		v.errors.Add("server.host", "host is required")
	}
	
	validModes := []string{"debug", "release", "test"}
	if !v.isInSlice(cfg.Mode, validModes) {
		v.errors.Add("server.mode", fmt.Sprintf("mode must be one of: %s", strings.Join(validModes, ", ")))
	}
	
	if cfg.TLSEnabled {
		if cfg.TLSCertFile == "" {
			v.errors.Add("server.tls_cert_file", "TLS certificate file is required when TLS is enabled")
		} else if !v.fileExists(cfg.TLSCertFile) {
			v.errors.Add("server.tls_cert_file", "TLS certificate file does not exist")
		}
		
		if cfg.TLSKeyFile == "" {
			v.errors.Add("server.tls_key_file", "TLS key file is required when TLS is enabled")
		} else if !v.fileExists(cfg.TLSKeyFile) {
			v.errors.Add("server.tls_key_file", "TLS key file does not exist")
		}
	}
	
	if cfg.ReadTimeout < 0 {
		v.errors.Add("server.read_timeout", "read timeout cannot be negative")
	}
	
	if cfg.WriteTimeout < 0 {
		v.errors.Add("server.write_timeout", "write timeout cannot be negative")
	}
	
	if cfg.IdleTimeout < 0 {
		v.errors.Add("server.idle_timeout", "idle timeout cannot be negative")
	}
	
	if cfg.MaxHeaderBytes < 0 {
		v.errors.Add("server.max_header_bytes", "max header bytes cannot be negative")
	}
}

func (v *Validator) validateDatabase(cfg *DatabaseConfig) {
	if cfg.Host == "" {
		v.errors.Add("database.host", "database host is required")
	}
	
	if cfg.Port < 1 || cfg.Port > 65535 {
		v.errors.Add("database.port", "database port must be between 1 and 65535")
	}
	
	if cfg.Database == "" {
		v.errors.Add("database.database", "database name is required")
	}
	
	if cfg.Username == "" {
		v.errors.Add("database.username", "database username is required")
	}
	
	validSSLModes := []string{"disable", "require", "verify-ca", "verify-full"}
	if !v.isInSlice(cfg.SSLMode, validSSLModes) {
		v.errors.Add("database.ssl_mode", fmt.Sprintf("SSL mode must be one of: %s", strings.Join(validSSLModes, ", ")))
	}
	
	if cfg.MaxOpenConns < 1 {
		v.errors.Add("database.max_open_conns", "max open connections must be at least 1")
	}
	
	if cfg.MaxIdleConns < 0 {
		v.errors.Add("database.max_idle_conns", "max idle connections cannot be negative")
	}
	
	if cfg.MaxIdleConns > cfg.MaxOpenConns {
		v.errors.Add("database.max_idle_conns", "max idle connections cannot exceed max open connections")
	}
	
	if cfg.ConnMaxLifetime < 0 {
		v.errors.Add("database.conn_max_lifetime", "connection max lifetime cannot be negative")
	}
}

func (v *Validator) validateCache(cfg *CacheConfig) {
	validTypes := []string{"redis", "memory", "none"}
	if !v.isInSlice(cfg.Type, validTypes) {
		v.errors.Add("cache.type", fmt.Sprintf("cache type must be one of: %s", strings.Join(validTypes, ", ")))
	}
	
	if cfg.Type == "redis" {
		if cfg.Host == "" {
			v.errors.Add("cache.host", "cache host is required for Redis")
		}
		
		if cfg.Port < 1 || cfg.Port > 65535 {
			v.errors.Add("cache.port", "cache port must be between 1 and 65535")
		}
		
		if cfg.Database < 0 || cfg.Database > 15 {
			v.errors.Add("cache.database", "Redis database must be between 0 and 15")
		}
	}
	
	if cfg.MaxRetries < 0 {
		v.errors.Add("cache.max_retries", "max retries cannot be negative")
	}
	
	if cfg.PoolSize < 1 {
		v.errors.Add("cache.pool_size", "pool size must be at least 1")
	}
	
	if cfg.MinIdleConns < 0 {
		v.errors.Add("cache.min_idle_conns", "min idle connections cannot be negative")
	}
	
	if cfg.DefaultExpiration < 0 {
		v.errors.Add("cache.default_expiration", "default expiration cannot be negative")
	}
}

func (v *Validator) validateStorage(cfg *StorageConfig) {
	validTypes := []string{"local", "s3", "gcs"}
	if !v.isInSlice(cfg.Type, validTypes) {
		v.errors.Add("storage.type", fmt.Sprintf("storage type must be one of: %s", strings.Join(validTypes, ", ")))
	}
	
	if cfg.BasePath == "" {
		v.errors.Add("storage.base_path", "base path is required")
	}
	
	if cfg.MaxUploadSizeMB < 1 {
		v.errors.Add("storage.max_upload_size_mb", "max upload size must be at least 1 MB")
	}
	
	if cfg.MaxUploadSizeMB > 1024 {
		v.errors.Add("storage.max_upload_size_mb", "max upload size cannot exceed 1024 MB")
	}
}

func (v *Validator) validateEmail(cfg *EmailConfig) {
	if cfg.FromEmail != "" && !v.isValidEmail(cfg.FromEmail) {
		v.errors.Add("email.from_email", "invalid from email address")
	}
	
	if cfg.ReplyTo != "" && !v.isValidEmail(cfg.ReplyTo) {
		v.errors.Add("email.reply_to", "invalid reply-to email address")
	}
	
	if cfg.ReturnPath != "" && !v.isValidEmail(cfg.ReturnPath) {
		v.errors.Add("email.return_path", "invalid return path email address")
	}
	
	if cfg.SendTimeout < 0 {
		v.errors.Add("email.send_timeout", "send timeout cannot be negative")
	}
	
	if cfg.ConnectionTimeout < 0 {
		v.errors.Add("email.connection_timeout", "connection timeout cannot be negative")
	}
	
	if cfg.RetryAttempts < 0 {
		v.errors.Add("email.retry_attempts", "retry attempts cannot be negative")
	}
	
	if cfg.RetryAttempts > 10 {
		v.errors.Add("email.retry_attempts", "retry attempts cannot exceed 10")
	}
	
	if cfg.RetryDelay < 0 {
		v.errors.Add("email.retry_delay", "retry delay cannot be negative")
	}
	
	if cfg.EnableDKIM && cfg.DKIMPrivateKeyPath != "" && !v.fileExists(cfg.DKIMPrivateKeyPath) {
		v.errors.Add("email.dkim_private_key_path", "DKIM private key file does not exist")
	}
	
	if cfg.EnableUnsubscribe && cfg.UnsubscribeURL != "" && !v.isValidURL(cfg.UnsubscribeURL) {
		v.errors.Add("email.unsubscribe_url", "invalid unsubscribe URL")
	}
}

func (v *Validator) validateAccount(cfg *AccountConfig) {
	validStrategies := []string{"round-robin", "weighted", "health-based", "random", "least-used"}
	if !v.isInSlice(cfg.RotationStrategy, validStrategies) {
		v.errors.Add("account.rotation_strategy", fmt.Sprintf("rotation strategy must be one of: %s", strings.Join(validStrategies, ", ")))
	}
	
	if cfg.RotationLimit < 1 {
		v.errors.Add("account.rotation_limit", "rotation limit must be at least 1")
	}
	
	if cfg.DailyLimit < 1 {
		v.errors.Add("account.daily_limit", "daily limit must be at least 1")
	}
	
	if cfg.HourlyLimit < 0 {
		v.errors.Add("account.hourly_limit", "hourly limit cannot be negative")
	}
	
	if cfg.HourlyLimit > cfg.DailyLimit {
		v.errors.Add("account.hourly_limit", "hourly limit cannot exceed daily limit")
	}
	
	if cfg.CooldownDuration < 0 {
		v.errors.Add("account.cooldown_duration", "cooldown duration cannot be negative")
	}
	
	if cfg.HealthCheckInterval < 0 {
		v.errors.Add("account.health_check_interval", "health check interval cannot be negative")
	}
	
	if cfg.HealthThreshold < 0 || cfg.HealthThreshold > 100 {
		v.errors.Add("account.health_threshold", "health threshold must be between 0 and 100")
	}
	
	if cfg.SuspendThreshold < 1 {
		v.errors.Add("account.suspend_threshold", "suspend threshold must be at least 1")
	}
	
	if cfg.ConsecutiveFailures < 1 {
		v.errors.Add("account.consecutive_failures", "consecutive failures must be at least 1")
	}
}

func (v *Validator) validateCampaign(cfg *CampaignConfig) {
	if cfg.MaxConcurrent < 1 {
		v.errors.Add("campaign.max_concurrent", "max concurrent campaigns must be at least 1")
	}
	
	if cfg.MaxConcurrent > 100 {
		v.errors.Add("campaign.max_concurrent", "max concurrent campaigns cannot exceed 100")
	}
	
	validPriorities := []string{"low", "normal", "high", "critical"}
	if !v.isInSlice(cfg.DefaultPriority, validPriorities) {
		v.errors.Add("campaign.default_priority", fmt.Sprintf("priority must be one of: %s", strings.Join(validPriorities, ", ")))
	}
	
	if cfg.CheckpointInterval < 0 {
		v.errors.Add("campaign.checkpoint_interval", "checkpoint interval cannot be negative")
	}
	
	if cfg.ProgressUpdateInterval < 0 {
		v.errors.Add("campaign.progress_update_interval", "progress update interval cannot be negative")
	}
	
	if cfg.RetryFailedAfter < 0 {
		v.errors.Add("campaign.retry_failed_after", "retry failed after cannot be negative")
	}
	
	if cfg.MaxRetryAttempts < 0 {
		v.errors.Add("campaign.max_retry_attempts", "max retry attempts cannot be negative")
	}
	
	if cfg.MaxRetryAttempts > 10 {
		v.errors.Add("campaign.max_retry_attempts", "max retry attempts cannot exceed 10")
	}
}

func (v *Validator) validateTemplate(cfg *TemplateConfig) {
	validStrategies := []string{"sequential", "random", "weighted", "round-robin"}
	if cfg.EnableRotation && !v.isInSlice(cfg.RotationStrategy, validStrategies) {
		v.errors.Add("template.rotation_strategy", fmt.Sprintf("rotation strategy must be one of: %s", strings.Join(validStrategies, ", ")))
	}
	
	if cfg.CacheTTL < 0 {
		v.errors.Add("template.cache_ttl", "cache TTL cannot be negative")
	}
	
	if cfg.SpamScoreThreshold < 0 {
		v.errors.Add("template.spam_score_threshold", "spam score threshold cannot be negative")
	}
	
	if cfg.MaxTemplateSizeKB < 1 {
		v.errors.Add("template.max_template_size_kb", "max template size must be at least 1 KB")
	}
	
	if cfg.MaxTemplateSizeKB > 10240 {
		v.errors.Add("template.max_template_size_kb", "max template size cannot exceed 10 MB")
	}
}

func (v *Validator) validatePersonalization(cfg *PersonalizationConfig) {
	if cfg.DefaultTimezone != "" {
		if _, err := time.LoadLocation(cfg.DefaultTimezone); err != nil {
			v.errors.Add("personalization.default_timezone", "invalid timezone")
		}
	}
	
	validStrategies := []string{"sequential", "random", "weighted", "time-based"}
	
	if cfg.CustomFieldRotation != "" && !v.isInSlice(cfg.CustomFieldRotation, validStrategies) {
		v.errors.Add("personalization.custom_field_rotation", fmt.Sprintf("rotation strategy must be one of: %s", strings.Join(validStrategies, ", ")))
	}
	
	if cfg.SenderNameRotation != "" && !v.isInSlice(cfg.SenderNameRotation, validStrategies) {
		v.errors.Add("personalization.sender_name_rotation", fmt.Sprintf("rotation strategy must be one of: %s", strings.Join(validStrategies, ", ")))
	}
	
	if cfg.SubjectRotation != "" && !v.isInSlice(cfg.SubjectRotation, validStrategies) {
		v.errors.Add("personalization.subject_rotation", fmt.Sprintf("rotation strategy must be one of: %s", strings.Join(validStrategies, ", ")))
	}
}

func (v *Validator) validateAttachment(cfg *AttachmentConfig) {
	validStrategies := []string{"sequential", "random", "round-robin"}
	if cfg.EnableRotation && !v.isInSlice(cfg.RotationStrategy, validStrategies) {
		v.errors.Add("attachment.rotation_strategy", fmt.Sprintf("rotation strategy must be one of: %s", strings.Join(validStrategies, ", ")))
	}
	
	if cfg.MaxCacheSizeMB < 1 {
		v.errors.Add("attachment.max_cache_size_mb", "max cache size must be at least 1 MB")
	}
	
	if cfg.MaxAttachmentSizeMB < 1 {
		v.errors.Add("attachment.max_attachment_size_mb", "max attachment size must be at least 1 MB")
	}
	
	if cfg.MaxAttachmentSizeMB > 100 {
		v.errors.Add("attachment.max_attachment_size_mb", "max attachment size cannot exceed 100 MB")
	}
	
	validBackends := []string{"chromedp", "wkhtmltopdf", "weasyprint"}
	if cfg.ConversionBackend != "" && !v.isInSlice(cfg.ConversionBackend, validBackends) {
		v.errors.Add("attachment.conversion_backend", fmt.Sprintf("conversion backend must be one of: %s", strings.Join(validBackends, ", ")))
	}
	
	if cfg.ImageQuality < 1 || cfg.ImageQuality > 100 {
		v.errors.Add("attachment.image_quality", "image quality must be between 1 and 100")
	}
	
	validFormats := []string{"jpeg", "jpg", "png", "webp"}
	if cfg.ImageFormat != "" && !v.isInSlice(cfg.ImageFormat, validFormats) {
		v.errors.Add("attachment.image_format", fmt.Sprintf("image format must be one of: %s", strings.Join(validFormats, ", ")))
	}
}

func (v *Validator) validateProxy(cfg *ProxyConfig) {
	if !cfg.Enabled {
		return
	}
	
	validStrategies := []string{"round-robin", "random", "least-used", "weighted"}
	if !v.isInSlice(cfg.RotationStrategy, validStrategies) {
		v.errors.Add("proxy.rotation_strategy", fmt.Sprintf("rotation strategy must be one of: %s", strings.Join(validStrategies, ", ")))
	}
	
	if cfg.HealthCheckInterval < 0 {
		v.errors.Add("proxy.health_check_interval", "health check interval cannot be negative")
	}
	
	if cfg.HealthCheckTimeout < 0 {
		v.errors.Add("proxy.health_check_timeout", "health check timeout cannot be negative")
	}
	
	if cfg.MaxRetries < 0 {
		v.errors.Add("proxy.max_retries", "max retries cannot be negative")
	}
	
	if cfg.RetryDelay < 0 {
		v.errors.Add("proxy.retry_delay", "retry delay cannot be negative")
	}
	
	if cfg.ConnectionTimeout < 0 {
		v.errors.Add("proxy.connection_timeout", "connection timeout cannot be negative")
	}
	
	if cfg.IdleConnTimeout < 0 {
		v.errors.Add("proxy.idle_conn_timeout", "idle connection timeout cannot be negative")
	}
}

func (v *Validator) validateRateLimit(cfg *RateLimitConfig) {
	if !cfg.Enabled {
		return
	}
	
	if cfg.GlobalRPS < 0 {
		v.errors.Add("rate_limit.global_rps", "global RPS cannot be negative")
	}
	
	if cfg.PerAccountRPS < 0 {
		v.errors.Add("rate_limit.per_account_rps", "per-account RPS cannot be negative")
	}
	
	if cfg.BurstSize < 1 {
		v.errors.Add("rate_limit.burst_size", "burst size must be at least 1")
	}
	
	if cfg.EnableAdaptive {
		if cfg.AdaptiveMinRPS < 0 {
			v.errors.Add("rate_limit.adaptive_min_rps", "adaptive min RPS cannot be negative")
		}
		
		if cfg.AdaptiveMaxRPS < cfg.AdaptiveMinRPS {
			v.errors.Add("rate_limit.adaptive_max_rps", "adaptive max RPS must be greater than min RPS")
		}
		
		if cfg.AdaptiveAdjustInterval < 0 {
			v.errors.Add("rate_limit.adaptive_adjust_interval", "adaptive adjust interval cannot be negative")
		}
	}
	
	if cfg.APIRateLimit < 1 {
		v.errors.Add("rate_limit.api_rate_limit", "API rate limit must be at least 1")
	}
	
	if cfg.APIRateLimitWindow < 0 {
		v.errors.Add("rate_limit.api_rate_limit_window", "API rate limit window cannot be negative")
	}
}

func (v *Validator) validateWorker(cfg *WorkerConfig) {
	if cfg.MinWorkers < 1 {
		v.errors.Add("worker.min_workers", "min workers must be at least 1")
	}
	
	if cfg.MaxWorkers < cfg.MinWorkers {
		v.errors.Add("worker.max_workers", "max workers must be greater than or equal to min workers")
	}
	
	if cfg.DefaultWorkers < cfg.MinWorkers || cfg.DefaultWorkers > cfg.MaxWorkers {
		v.errors.Add("worker.default_workers", "default workers must be between min and max workers")
	}
	
	if cfg.QueueSize < 1 {
		v.errors.Add("worker.queue_size", "queue size must be at least 1")
	}
	
	if cfg.BatchSize < 1 {
		v.errors.Add("worker.batch_size", "batch size must be at least 1")
	}
	
	if cfg.ScaleUpThreshold < 0 || cfg.ScaleUpThreshold > 1 {
		v.errors.Add("worker.scale_up_threshold", "scale up threshold must be between 0 and 1")
	}
	
	if cfg.ScaleDownThreshold < 0 || cfg.ScaleDownThreshold > 1 {
		v.errors.Add("worker.scale_down_threshold", "scale down threshold must be between 0 and 1")
	}
	
	if cfg.ScaleDownThreshold >= cfg.ScaleUpThreshold {
		v.errors.Add("worker.scale_down_threshold", "scale down threshold must be less than scale up threshold")
	}
	
	if cfg.IdleTimeout < 0 {
		v.errors.Add("worker.idle_timeout", "idle timeout cannot be negative")
	}
	
	if cfg.GracefulShutdownTimeout < 0 {
		v.errors.Add("worker.graceful_shutdown_timeout", "graceful shutdown timeout cannot be negative")
	}
}

func (v *Validator) validateNotification(cfg *NotificationConfig) {
	if !cfg.Enabled {
		return
	}
	
	validChannels := []string{"telegram", "email", "webhook", "slack"}
	if !v.isInSlice(cfg.Channel, validChannels) {
		v.errors.Add("notification.channel", fmt.Sprintf("channel must be one of: %s", strings.Join(validChannels, ", ")))
	}
	
	if cfg.Channel == "telegram" {
		if cfg.TelegramBotToken == "" {
			v.errors.Add("notification.telegram_bot_token", "Telegram bot token is required")
		}
		
		if cfg.TelegramChatID == "" {
			v.errors.Add("notification.telegram_chat_id", "Telegram chat ID is required")
		}
		
		validParseModes := []string{"Markdown", "MarkdownV2", "HTML"}
		if !v.isInSlice(cfg.TelegramParseMode, validParseModes) {
			v.errors.Add("notification.telegram_parse_mode", fmt.Sprintf("parse mode must be one of: %s", strings.Join(validParseModes, ", ")))
		}
	}
	
	if cfg.Channel == "webhook" {
		if cfg.WebhookURL == "" {
			v.errors.Add("notification.webhook_url", "webhook URL is required")
		} else if !v.isValidURL(cfg.WebhookURL) {
			v.errors.Add("notification.webhook_url", "invalid webhook URL")
		}
	}
	
	if cfg.MaxRetries < 0 {
		v.errors.Add("notification.max_retries", "max retries cannot be negative")
	}
	
	if cfg.RetryDelay < 0 {
		v.errors.Add("notification.retry_delay", "retry delay cannot be negative")
	}
}

func (v *Validator) validateLogging(cfg *LoggingConfig) {
	validLevels := []string{"debug", "info", "warn", "error", "fatal", "panic"}
	if !v.isInSlice(cfg.Level, validLevels) {
		v.errors.Add("logging.level", fmt.Sprintf("log level must be one of: %s", strings.Join(validLevels, ", ")))
	}
	
	validFormats := []string{"json", "text", "console"}
	if !v.isInSlice(cfg.Format, validFormats) {
		v.errors.Add("logging.format", fmt.Sprintf("log format must be one of: %s", strings.Join(validFormats, ", ")))
	}
	
	if cfg.MaxSizeMB < 1 {
		v.errors.Add("logging.max_size_mb", "max size must be at least 1 MB")
	}
	
	if cfg.MaxBackups < 0 {
		v.errors.Add("logging.max_backups", "max backups cannot be negative")
	}
	
	if cfg.MaxAgeDays < 0 {
		v.errors.Add("logging.max_age_days", "max age cannot be negative")
	}
}

func (v *Validator) validateSecurity(cfg *SecurityConfig) {
	if cfg.EnableAuth && cfg.JWTSecret == "" {
		v.errors.Add("security.jwt_secret", "JWT secret is required when authentication is enabled")
	}
	
	if len(cfg.JWTSecret) > 0 && len(cfg.JWTSecret) < 32 {
		v.errors.Add("security.jwt_secret", "JWT secret must be at least 32 characters")
	}
	
	if cfg.JWTExpiration < 0 {
		v.errors.Add("security.jwt_expiration", "JWT expiration cannot be negative")
	}
	
	if cfg.EnableAPIKey && cfg.APIKey == "" {
		v.errors.Add("security.api_key", "API key is required when API key authentication is enabled")
	}
	
	if cfg.EnableEncryption && cfg.EncryptionKey == "" {
		v.errors.Add("security.encryption_key", "encryption key is required when encryption is enabled")
	}
	
	if len(cfg.EncryptionKey) > 0 && len(cfg.EncryptionKey) != 32 {
		v.errors.Add("security.encryption_key", "encryption key must be exactly 32 characters for AES-256")
	}
	
	validAlgorithms := []string{"AES-256-GCM", "AES-256-CBC"}
	if cfg.EncryptionAlgorithm != "" && !v.isInSlice(cfg.EncryptionAlgorithm, validAlgorithms) {
		v.errors.Add("security.encryption_algorithm", fmt.Sprintf("encryption algorithm must be one of: %s", strings.Join(validAlgorithms, ", ")))
	}
	
	validHashAlgorithms := []string{"SHA256", "SHA512", "bcrypt", "argon2"}
	if cfg.HashAlgorithm != "" && !v.isInSlice(cfg.HashAlgorithm, validHashAlgorithms) {
		v.errors.Add("security.hash_algorithm", fmt.Sprintf("hash algorithm must be one of: %s", strings.Join(validHashAlgorithms, ", ")))
	}
	
	if cfg.SessionMaxAge < 0 {
		v.errors.Add("security.session_max_age", "session max age cannot be negative")
	}
}

func (v *Validator) validateMonitoring(cfg *MonitoringConfig) {
	if !cfg.Enabled {
		return
	}
	
	if cfg.EnableMetrics {
		if cfg.MetricsPort < 1 || cfg.MetricsPort > 65535 {
			v.errors.Add("monitoring.metrics_port", "metrics port must be between 1 and 65535")
		}
		
		if cfg.MetricsPath == "" {
			v.errors.Add("monitoring.metrics_path", "metrics path is required")
		}
	}
	
	if cfg.EnableProfiling {
		if cfg.ProfilingPort < 1 || cfg.ProfilingPort > 65535 {
			v.errors.Add("monitoring.profiling_port", "profiling port must be between 1 and 65535")
		}
	}
	
	if cfg.EnableHealthCheck {
		if cfg.HealthCheckPath == "" {
			v.errors.Add("monitoring.health_check_path", "health check path is required")
		}
		
		if cfg.HealthCheckInterval < 0 {
			v.errors.Add("monitoring.health_check_interval", "health check interval cannot be negative")
		}
	}
	
	if cfg.EnableTracing && cfg.TracingEndpoint != "" && !v.isValidURL(cfg.TracingEndpoint) {
		v.errors.Add("monitoring.tracing_endpoint", "invalid tracing endpoint URL")
	}
}

func (v *Validator) validateCleanup(cfg *CleanupConfig) {
	if !cfg.Enabled {
		return
	}
	
	if cfg.CleanupInterval < 0 {
		v.errors.Add("cleanup.cleanup_interval", "cleanup interval cannot be negative")
	}
	
	if cfg.DeleteCompletedAfter < 0 {
		v.errors.Add("cleanup.delete_completed_after", "delete completed after cannot be negative")
	}
	
	if cfg.DeleteFailedAfter < 0 {
		v.errors.Add("cleanup.delete_failed_after", "delete failed after cannot be negative")
	}
	
	if cfg.DeleteLogsAfter < 0 {
		v.errors.Add("cleanup.delete_logs_after", "delete logs after cannot be negative")
	}
	
	if cfg.DeleteTempFilesAfter < 0 {
		v.errors.Add("cleanup.delete_temp_files_after", "delete temp files after cannot be negative")
	}
	
	if cfg.DeleteCacheAfter < 0 {
		v.errors.Add("cleanup.delete_cache_after", "delete cache after cannot be negative")
	}
	
	if cfg.CleanupBatchSize < 1 {
		v.errors.Add("cleanup.cleanup_batch_size", "cleanup batch size must be at least 1")
	}
	
	if cfg.BackupInterval < 0 {
		v.errors.Add("cleanup.backup_interval", "backup interval cannot be negative")
	}
}

func (v *Validator) validateProviders(providers map[string]ProviderConfig) {
	for name, provider := range providers {
		v.validateProvider(name, &provider)
	}
}

func (v *Validator) validateProvider(name string, cfg *ProviderConfig) {
	prefix := fmt.Sprintf("providers.%s", name)
	
	validTypes := []string{"gmail", "smtp", "yahoo", "outlook", "icloud", "workspace"}
	if !v.isInSlice(cfg.Type, validTypes) {
		v.errors.Add(prefix+".type", fmt.Sprintf("provider type must be one of: %s", strings.Join(validTypes, ", ")))
	}
	
	if cfg.Type == "smtp" || cfg.Type == "yahoo" || cfg.Type == "outlook" || cfg.Type == "icloud" {
		if cfg.Host == "" {
			v.errors.Add(prefix+".host", "host is required for SMTP provider")
		}
		
		if cfg.Port < 1 || cfg.Port > 65535 {
			v.errors.Add(prefix+".port", "port must be between 1 and 65535")
		}
		
		if cfg.Username == "" {
			v.errors.Add(prefix+".username", "username is required for SMTP provider")
		}
		
		if cfg.Password == "" && cfg.AuthMethod != "oauth2" {
			v.errors.Add(prefix+".password", "password is required for SMTP provider")
		}
	}
	
	if cfg.AuthMethod == "oauth2" {
		if cfg.OAuthClientID == "" {
			v.errors.Add(prefix+".oauth_client_id", "OAuth client ID is required for OAuth2 authentication")
		}
		
		if cfg.OAuthClientSecret == "" {
			v.errors.Add(prefix+".oauth_client_secret", "OAuth client secret is required for OAuth2 authentication")
		}
		
		if cfg.OAuthTokenURL == "" {
			v.errors.Add(prefix+".oauth_token_url", "OAuth token URL is required for OAuth2 authentication")
		}
	}
	
	if cfg.MaxConnectionsPerAccount < 1 {
		v.errors.Add(prefix+".max_connections_per_account", "max connections per account must be at least 1")
	}
	
	if cfg.ConnectionTimeout < 0 {
		v.errors.Add(prefix+".connection_timeout", "connection timeout cannot be negative")
	}
	
	if cfg.SendTimeout < 0 {
		v.errors.Add(prefix+".send_timeout", "send timeout cannot be negative")
	}
}

func (v *Validator) isInSlice(value string, slice []string) bool {
	for _, item := range slice {
		if strings.EqualFold(value, item) {
			return true
		}
	}
	return false
}

func (v *Validator) isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

func (v *Validator) isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

func (v *Validator) fileExists(path string) bool {
	expandedPath := os.ExpandEnv(path)
	_, err := os.Stat(expandedPath)
	return err == nil
}

func (v *Validator) isValidPort(port int) bool {
	return port >= 1 && port <= 65535
}

func (v *Validator) isValidHost(host string) bool {
	if host == "" {
		return false
	}
	
	if host == "localhost" || host == "0.0.0.0" {
		return true
	}
	
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	
	return true
}

func (v *Validator) isValidPath(path string) bool {
	if path == "" {
		return false
	}
	
	expandedPath := os.ExpandEnv(path)
	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return false
	}
	
	if strings.Contains(absPath, "..") {
		return false
	}
	
	return true
}

func ValidateConfig(cfg *AppConfig) error {
	validator := NewValidator()
	return validator.Validate(cfg)
}

func ValidateSection(cfg *AppConfig, section string) error {
	validator := NewValidator()
	
	switch strings.ToLower(section) {
	case "app", "application":
		validator.validateApp(&cfg.App)
	case "server":
		validator.validateServer(&cfg.Server)
	case "database", "db":
		validator.validateDatabase(&cfg.Database)
	case "cache":
		validator.validateCache(&cfg.Cache)
	case "storage":
		validator.validateStorage(&cfg.Storage)
	case "email":
		validator.validateEmail(&cfg.Email)
	case "account":
		validator.validateAccount(&cfg.Account)
	case "campaign":
		validator.validateCampaign(&cfg.Campaign)
	case "template":
		validator.validateTemplate(&cfg.Template)
	case "worker":
		validator.validateWorker(&cfg.Worker)
	case "security":
		validator.validateSecurity(&cfg.Security)
	default:
		return fmt.Errorf("unknown section: %s", section)
	}
	
	if validator.errors.HasErrors() {
		return validator.errors
	}
	
	return nil
}
