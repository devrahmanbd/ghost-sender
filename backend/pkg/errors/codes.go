package errors

const (
	ErrInternal          = "INTERNAL_ERROR"
	ErrNotFound          = "NOT_FOUND"
	ErrAlreadyExists     = "ALREADY_EXISTS"
	ErrInvalidInput      = "INVALID_INPUT"
	ErrUnauthorized      = "UNAUTHORIZED"
	ErrForbidden         = "FORBIDDEN"
	ErrConflict          = "CONFLICT"
	ErrValidation        = "VALIDATION_ERROR"
	ErrTimeout           = "TIMEOUT"
	ErrServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrBadRequest        = "BAD_REQUEST"
	ErrMethodNotAllowed  = "METHOD_NOT_ALLOWED"
	ErrUnsupportedMediaType = "UNSUPPORTED_MEDIA_TYPE"
	ErrTooLarge          = "REQUEST_TOO_LARGE"
	ErrPreconditionFailed = "PRECONDITION_FAILED"
)

const (
	ErrDatabase           = "DATABASE_ERROR"
	ErrDatabaseConnection = "DATABASE_CONNECTION_ERROR"
	ErrDatabaseQuery      = "DATABASE_QUERY_ERROR"
	ErrDatabaseTransaction = "DATABASE_TRANSACTION_ERROR"
	ErrDatabaseConstraint = "DATABASE_CONSTRAINT_ERROR"
	ErrDatabaseDeadlock   = "DATABASE_DEADLOCK"
	ErrDatabaseTimeout    = "DATABASE_TIMEOUT"
)

const (
	ErrCache            = "CACHE_ERROR"
	ErrCacheMiss        = "CACHE_MISS"
	ErrCacheSet         = "CACHE_SET_ERROR"
	ErrCacheGet         = "CACHE_GET_ERROR"
	ErrCacheDelete      = "CACHE_DELETE_ERROR"
	ErrCacheConnection  = "CACHE_CONNECTION_ERROR"
	ErrCacheSerialization = "CACHE_SERIALIZATION_ERROR"
)

const (
	ErrCampaignNotFound      = "CAMPAIGN_NOT_FOUND"
	ErrCampaignAlreadyExists = "CAMPAIGN_ALREADY_EXISTS"
	ErrCampaignInvalidState  = "CAMPAIGN_INVALID_STATE"
	ErrCampaignOperation     = "CAMPAIGN_OPERATION_ERROR"
	ErrCampaignStart         = "CAMPAIGN_START_ERROR"
	ErrCampaignStop          = "CAMPAIGN_STOP_ERROR"
	ErrCampaignPause         = "CAMPAIGN_PAUSE_ERROR"
	ErrCampaignResume        = "CAMPAIGN_RESUME_ERROR"
	ErrCampaignCancel        = "CAMPAIGN_CANCEL_ERROR"
	ErrCampaignSchedule      = "CAMPAIGN_SCHEDULE_ERROR"
	ErrCampaignPersistence   = "CAMPAIGN_PERSISTENCE_ERROR"
	ErrCampaignCheckpoint    = "CAMPAIGN_CHECKPOINT_ERROR"
	ErrCampaignRestore       = "CAMPAIGN_RESTORE_ERROR"
	ErrCampaignCleanup       = "CAMPAIGN_CLEANUP_ERROR"
	ErrCampaignNoRecipients  = "CAMPAIGN_NO_RECIPIENTS"
	ErrCampaignNoAccounts    = "CAMPAIGN_NO_ACCOUNTS"
	ErrCampaignNoTemplates   = "CAMPAIGN_NO_TEMPLATES"
	ErrCampaignValidation    = "CAMPAIGN_VALIDATION_ERROR"
)

const (
	ErrAccountNotFound         = "ACCOUNT_NOT_FOUND"
	ErrAccountAlreadyExists    = "ACCOUNT_ALREADY_EXISTS"
	ErrAccountSuspended        = "ACCOUNT_SUSPENDED"
	ErrAccountInactive         = "ACCOUNT_INACTIVE"
	ErrAccountHealthCheck      = "ACCOUNT_HEALTH_CHECK_ERROR"
	ErrAccountRotation         = "ACCOUNT_ROTATION_ERROR"
	ErrAccountNoAvailable      = "ACCOUNT_NO_AVAILABLE"
	ErrAccountConnection       = "ACCOUNT_CONNECTION_ERROR"
	ErrAccountAuthentication   = "ACCOUNT_AUTHENTICATION_ERROR"
	ErrAccountConfiguration    = "ACCOUNT_CONFIGURATION_ERROR"
	ErrAccountValidation       = "ACCOUNT_VALIDATION_ERROR"
	ErrAccountTestConnection   = "ACCOUNT_TEST_CONNECTION_ERROR"
)

const (
	ErrDailyLimitExceeded    = "DAILY_LIMIT_EXCEEDED"
	ErrRotationLimitExceeded = "ROTATION_LIMIT_EXCEEDED"
	ErrRateLimitExceeded     = "RATE_LIMIT_EXCEEDED"
	ErrConcurrentLimitExceeded = "CONCURRENT_LIMIT_EXCEEDED"
	ErrHourlyLimitExceeded   = "HOURLY_LIMIT_EXCEEDED"
	ErrWeeklyLimitExceeded   = "WEEKLY_LIMIT_EXCEEDED"
	ErrMonthlyLimitExceeded  = "MONTHLY_LIMIT_EXCEEDED"
	ErrBandwidthLimitExceeded = "BANDWIDTH_LIMIT_EXCEEDED"
	ErrStorageLimitExceeded  = "STORAGE_LIMIT_EXCEEDED"
)

const (
	ErrTemplateNotFound      = "TEMPLATE_NOT_FOUND"
	ErrTemplateAlreadyExists = "TEMPLATE_ALREADY_EXISTS"
	ErrTemplateInvalid       = "TEMPLATE_INVALID"
	ErrTemplateOperation     = "TEMPLATE_OPERATION_ERROR"
	ErrTemplateParsing       = "TEMPLATE_PARSING_ERROR"
	ErrTemplateRendering     = "TEMPLATE_RENDERING_ERROR"
	ErrTemplateValidation    = "TEMPLATE_VALIDATION_ERROR"
	ErrTemplateVariable      = "TEMPLATE_VARIABLE_ERROR"
	ErrTemplateRotation      = "TEMPLATE_ROTATION_ERROR"
	ErrTemplateCompilation   = "TEMPLATE_COMPILATION_ERROR"
	ErrTemplateSyntax        = "TEMPLATE_SYNTAX_ERROR"
	ErrTemplateSpamScore     = "TEMPLATE_SPAM_SCORE_HIGH"
	ErrTemplateSize          = "TEMPLATE_SIZE_EXCEEDED"
	ErrTemplateHTML          = "TEMPLATE_HTML_INVALID"
)

const (
	ErrRecipientNotFound      = "RECIPIENT_NOT_FOUND"
	ErrRecipientAlreadyExists = "RECIPIENT_ALREADY_EXISTS"
	ErrRecipientInvalid       = "RECIPIENT_INVALID"
	ErrRecipientValidation    = "RECIPIENT_VALIDATION_ERROR"
	ErrRecipientImport        = "RECIPIENT_IMPORT_ERROR"
	ErrRecipientExport        = "RECIPIENT_EXPORT_ERROR"
	ErrRecipientDuplicate     = "RECIPIENT_DUPLICATE"
	ErrRecipientBulkOperation = "RECIPIENT_BULK_OPERATION_ERROR"
	ErrRecipientCSVParsing    = "RECIPIENT_CSV_PARSING_ERROR"
	ErrRecipientDNSCheck      = "RECIPIENT_DNS_CHECK_ERROR"
	ErrRecipientEmailFormat   = "RECIPIENT_EMAIL_FORMAT_ERROR"
	ErrRecipientBlacklisted   = "RECIPIENT_BLACKLISTED"
	ErrRecipientUnsubscribed  = "RECIPIENT_UNSUBSCRIBED"
)

const (
	ErrEmailSend          = "EMAIL_SEND_ERROR"
	ErrEmailDelivery      = "EMAIL_DELIVERY_ERROR"
	ErrEmailBounce        = "EMAIL_BOUNCE"
	ErrEmailRejected      = "EMAIL_REJECTED"
	ErrEmailSpamDetected  = "EMAIL_SPAM_DETECTED"
	ErrEmailBlocked       = "EMAIL_BLOCKED"
	ErrEmailTimeout       = "EMAIL_TIMEOUT"
	ErrEmailFormatting    = "EMAIL_FORMATTING_ERROR"
	ErrEmailEncoding      = "EMAIL_ENCODING_ERROR"
	ErrEmailAttachment    = "EMAIL_ATTACHMENT_ERROR"
	ErrEmailHeaders       = "EMAIL_HEADERS_ERROR"
	ErrEmailMIME          = "EMAIL_MIME_ERROR"
	ErrEmailMessageID     = "EMAIL_MESSAGE_ID_ERROR"
	ErrEmailUnsubscribe   = "EMAIL_UNSUBSCRIBE_ERROR"
)

const (
	ErrSMTP                = "SMTP_ERROR"
	ErrSMTPConnection      = "SMTP_CONNECTION_ERROR"
	ErrSMTPAuthentication  = "SMTP_AUTHENTICATION_ERROR"
	ErrSMTPTimeout         = "SMTP_TIMEOUT"
	ErrSMTPTLS             = "SMTP_TLS_ERROR"
	ErrSMTPCommand         = "SMTP_COMMAND_ERROR"
	ErrSMTPResponse        = "SMTP_RESPONSE_ERROR"
	ErrSMTPConfiguration   = "SMTP_CONFIGURATION_ERROR"
	ErrSMTPUnavailable     = "SMTP_UNAVAILABLE"
	ErrSMTPMailbox         = "SMTP_MAILBOX_ERROR"
	ErrSMTPTransactionFailed = "SMTP_TRANSACTION_FAILED"
)

const (
	ErrProxyNotFound        = "PROXY_NOT_FOUND"
	ErrProxyAlreadyExists   = "PROXY_ALREADY_EXISTS"
	ErrProxyConnection      = "PROXY_CONNECTION_ERROR"
	ErrProxyAuthentication  = "PROXY_AUTHENTICATION_ERROR"
	ErrProxyTimeout         = "PROXY_TIMEOUT"
	ErrProxyUnavailable     = "PROXY_UNAVAILABLE"
	ErrProxyConfiguration   = "PROXY_CONFIGURATION_ERROR"
	ErrProxyHealthCheck     = "PROXY_HEALTH_CHECK_ERROR"
	ErrProxyRotation        = "PROXY_ROTATION_ERROR"
	ErrProxyNoAvailable     = "PROXY_NO_AVAILABLE"
	ErrProxyValidation      = "PROXY_VALIDATION_ERROR"
	ErrProxyBlacklisted     = "PROXY_BLACKLISTED"
)

const (
	ErrOAuth2                 = "OAUTH2_ERROR"
	ErrOAuth2TokenInvalid     = "OAUTH2_TOKEN_INVALID"
	ErrOAuth2TokenExpired     = "OAUTH2_TOKEN_EXPIRED"
	ErrOAuth2TokenRefresh     = "OAUTH2_TOKEN_REFRESH_ERROR"
	ErrOAuth2Authorization    = "OAUTH2_AUTHORIZATION_ERROR"
	ErrOAuth2Exchange         = "OAUTH2_EXCHANGE_ERROR"
	ErrOAuth2Revocation       = "OAUTH2_REVOCATION_ERROR"
	ErrOAuth2Configuration    = "OAUTH2_CONFIGURATION_ERROR"
	ErrOAuth2Provider         = "OAUTH2_PROVIDER_ERROR"
	ErrOAuth2Scope            = "OAUTH2_SCOPE_ERROR"
)

const (
	ErrAttachmentNotFound      = "ATTACHMENT_NOT_FOUND"
	ErrAttachmentTooLarge      = "ATTACHMENT_TOO_LARGE"
	ErrAttachmentInvalidFormat = "ATTACHMENT_INVALID_FORMAT"
	ErrAttachmentConversion    = "ATTACHMENT_CONVERSION_ERROR"
	ErrAttachmentEncoding      = "ATTACHMENT_ENCODING_ERROR"
	ErrAttachmentCache         = "ATTACHMENT_CACHE_ERROR"
	ErrAttachmentRotation      = "ATTACHMENT_ROTATION_ERROR"
	ErrAttachmentGeneration    = "ATTACHMENT_GENERATION_ERROR"
	ErrAttachmentPDF           = "ATTACHMENT_PDF_ERROR"
	ErrAttachmentImage         = "ATTACHMENT_IMAGE_ERROR"
	ErrAttachmentValidation    = "ATTACHMENT_VALIDATION_ERROR"
)

const (
	ErrFileOperation      = "FILE_OPERATION_ERROR"
	ErrFileNotFound       = "FILE_NOT_FOUND"
	ErrFileAlreadyExists  = "FILE_ALREADY_EXISTS"
	ErrFileRead           = "FILE_READ_ERROR"
	ErrFileWrite          = "FILE_WRITE_ERROR"
	ErrFileDelete         = "FILE_DELETE_ERROR"
	ErrFilePermission     = "FILE_PERMISSION_ERROR"
	ErrFileSize           = "FILE_SIZE_ERROR"
	ErrFileFormat         = "FILE_FORMAT_ERROR"
	ErrFileCorrupted      = "FILE_CORRUPTED"
	ErrFileUpload         = "FILE_UPLOAD_ERROR"
	ErrFileDownload       = "FILE_DOWNLOAD_ERROR"
	ErrFileZIP            = "FILE_ZIP_ERROR"
	ErrFileExtraction     = "FILE_EXTRACTION_ERROR"
	ErrFileValidation     = "FILE_VALIDATION_ERROR"
	ErrFilePathTraversal  = "FILE_PATH_TRAVERSAL"
)

const (
	ErrConfiguration       = "CONFIGURATION_ERROR"
	ErrConfigurationLoad   = "CONFIGURATION_LOAD_ERROR"
	ErrConfigurationSave   = "CONFIGURATION_SAVE_ERROR"
	ErrConfigurationParse  = "CONFIGURATION_PARSE_ERROR"
	ErrConfigurationValidation = "CONFIGURATION_VALIDATION_ERROR"
	ErrConfigurationMissing = "CONFIGURATION_MISSING"
	ErrConfigurationInvalid = "CONFIGURATION_INVALID"
	ErrConfigurationBackup  = "CONFIGURATION_BACKUP_ERROR"
	ErrConfigurationRestore = "CONFIGURATION_RESTORE_ERROR"
)

const (
	ErrNotificationSend        = "NOTIFICATION_SEND_ERROR"
	ErrNotificationQueue       = "NOTIFICATION_QUEUE_ERROR"
	ErrNotificationFormat      = "NOTIFICATION_FORMAT_ERROR"
	ErrNotificationTelegram    = "NOTIFICATION_TELEGRAM_ERROR"
	ErrNotificationConfiguration = "NOTIFICATION_CONFIGURATION_ERROR"
	ErrNotificationDelivery    = "NOTIFICATION_DELIVERY_ERROR"
	ErrNotificationTemplate    = "NOTIFICATION_TEMPLATE_ERROR"
	ErrNotificationBotToken    = "NOTIFICATION_BOT_TOKEN_INVALID"
	ErrNotificationChatID      = "NOTIFICATION_CHAT_ID_INVALID"
)

const (
	ErrPersonalization       = "PERSONALIZATION_ERROR"
	ErrPersonalizationVariable = "PERSONALIZATION_VARIABLE_ERROR"
	ErrPersonalizationGeneration = "PERSONALIZATION_GENERATION_ERROR"
	ErrPersonalizationExtraction = "PERSONALIZATION_EXTRACTION_ERROR"
	ErrPersonalizationFormat   = "PERSONALIZATION_FORMAT_ERROR"
	ErrPersonalizationDateTime = "PERSONALIZATION_DATETIME_ERROR"
	ErrPersonalizationRotation = "PERSONALIZATION_ROTATION_ERROR"
)

const (
	ErrRotationStrategy   = "ROTATION_STRATEGY_ERROR"
	ErrRotationExhausted  = "ROTATION_EXHAUSTED"
	ErrRotationInvalid    = "ROTATION_INVALID"
	ErrRotationState      = "ROTATION_STATE_ERROR"
	ErrRotationStatistics = "ROTATION_STATISTICS_ERROR"
	ErrRotationWeight     = "ROTATION_WEIGHT_ERROR"
)

const (
	ErrWorkerPool         = "WORKER_POOL_ERROR"
	ErrWorkerSpawn        = "WORKER_SPAWN_ERROR"
	ErrWorkerShutdown     = "WORKER_SHUTDOWN_ERROR"
	ErrWorkerPanic        = "WORKER_PANIC"
	ErrWorkerTimeout      = "WORKER_TIMEOUT"
	ErrWorkerExhausted    = "WORKER_EXHAUSTED"
)

const (
	ErrQueue            = "QUEUE_ERROR"
	ErrQueueFull        = "QUEUE_FULL"
	ErrQueueEmpty       = "QUEUE_EMPTY"
	ErrQueueEnqueue     = "QUEUE_ENQUEUE_ERROR"
	ErrQueueDequeue     = "QUEUE_DEQUEUE_ERROR"
	ErrQueuePriority    = "QUEUE_PRIORITY_ERROR"
	ErrQueueOperation   = "QUEUE_OPERATION_ERROR"
)

const (
	ErrRetry            = "RETRY_ERROR"
	ErrRetryExhausted   = "RETRY_EXHAUSTED"
	ErrRetryBackoff     = "RETRY_BACKOFF_ERROR"
	ErrRetryMaxAttempts = "RETRY_MAX_ATTEMPTS"
)

const (
	ErrState           = "STATE_ERROR"
	ErrInvalidState    = "INVALID_STATE"
	ErrStateTransition = "STATE_TRANSITION_ERROR"
	ErrStatePersistence = "STATE_PERSISTENCE_ERROR"
	ErrStateLoad       = "STATE_LOAD_ERROR"
	ErrStateSave       = "STATE_SAVE_ERROR"
)

const (
	ErrResourceExhausted  = "RESOURCE_EXHAUSTED"
	ErrResourceNotFound   = "RESOURCE_NOT_FOUND"
	ErrResourceLocked     = "RESOURCE_LOCKED"
	ErrResourceUnavailable = "RESOURCE_UNAVAILABLE"
	ErrResourceConflict   = "RESOURCE_CONFLICT"
)

const (
	ErrScheduler        = "SCHEDULER_ERROR"
	ErrSchedulerCron    = "SCHEDULER_CRON_ERROR"
	ErrSchedulerTrigger = "SCHEDULER_TRIGGER_ERROR"
	ErrSchedulerJob     = "SCHEDULER_JOB_ERROR"
)

const (
	ErrCleanup        = "CLEANUP_ERROR"
	ErrCleanupExpired = "CLEANUP_EXPIRED_ERROR"
	ErrCleanupFailed  = "CLEANUP_FAILED_ERROR"
)

const (
	ErrStatistics       = "STATISTICS_ERROR"
	ErrStatisticsUpdate = "STATISTICS_UPDATE_ERROR"
	ErrStatisticsQuery  = "STATISTICS_QUERY_ERROR"
	ErrStatisticsAggregate = "STATISTICS_AGGREGATE_ERROR"
)

const (
	ErrLogging       = "LOGGING_ERROR"
	ErrLogWrite      = "LOG_WRITE_ERROR"
	ErrLogRotation   = "LOG_ROTATION_ERROR"
	ErrLogFormat     = "LOG_FORMAT_ERROR"
	ErrLogStream     = "LOG_STREAM_ERROR"
)

const (
	ErrSerialization   = "SERIALIZATION_ERROR"
	ErrDeserialization = "DESERIALIZATION_ERROR"
	ErrEncoding        = "ENCODING_ERROR"
	ErrDecoding        = "DECODING_ERROR"
	ErrMarshaling      = "MARSHALING_ERROR"
	ErrUnmarshaling    = "UNMARSHALING_ERROR"
)

const (
	ErrEncryption     = "ENCRYPTION_ERROR"
	ErrDecryption     = "DECRYPTION_ERROR"
	ErrHashing        = "HASHING_ERROR"
	ErrSignature      = "SIGNATURE_ERROR"
	ErrKeyGeneration  = "KEY_GENERATION_ERROR"
	ErrKeyInvalid     = "KEY_INVALID"
)

const (
	ErrWebSocket           = "WEBSOCKET_ERROR"
	ErrWebSocketConnection = "WEBSOCKET_CONNECTION_ERROR"
	ErrWebSocketSend       = "WEBSOCKET_SEND_ERROR"
	ErrWebSocketReceive    = "WEBSOCKET_RECEIVE_ERROR"
	ErrWebSocketClosed     = "WEBSOCKET_CLOSED"
	ErrWebSocketUpgrade    = "WEBSOCKET_UPGRADE_ERROR"
)

const (
	ErrAPI            = "API_ERROR"
	ErrAPIRequest     = "API_REQUEST_ERROR"
	ErrAPIResponse    = "API_RESPONSE_ERROR"
	ErrAPIValidation  = "API_VALIDATION_ERROR"
	ErrAPIRateLimit   = "API_RATE_LIMIT_ERROR"
	ErrAPITimeout     = "API_TIMEOUT"
	ErrAPIUnavailable = "API_UNAVAILABLE"
)

const (
	ErrProvider             = "PROVIDER_ERROR"
	ErrProviderNotFound     = "PROVIDER_NOT_FOUND"
	ErrProviderUnavailable  = "PROVIDER_UNAVAILABLE"
	ErrProviderConfiguration = "PROVIDER_CONFIGURATION_ERROR"
	ErrProviderAuthentication = "PROVIDER_AUTHENTICATION_ERROR"
	ErrProviderAPI          = "PROVIDER_API_ERROR"
	ErrProviderQuota        = "PROVIDER_QUOTA_EXCEEDED"
)

const (
	ErrConnectionPool       = "CONNECTION_POOL_ERROR"
	ErrConnectionPoolFull   = "CONNECTION_POOL_FULL"
	ErrConnectionPoolEmpty  = "CONNECTION_POOL_EMPTY"
	ErrConnectionAcquire    = "CONNECTION_ACQUIRE_ERROR"
	ErrConnectionRelease    = "CONNECTION_RELEASE_ERROR"
	ErrConnectionTimeout    = "CONNECTION_TIMEOUT"
	ErrConnectionClosed     = "CONNECTION_CLOSED"
)

const (
	ErrHealth          = "HEALTH_ERROR"
	ErrHealthCheck     = "HEALTH_CHECK_ERROR"
	ErrHealthScore     = "HEALTH_SCORE_ERROR"
	ErrHealthMonitor   = "HEALTH_MONITOR_ERROR"
	ErrHealthDegraded  = "HEALTH_DEGRADED"
	ErrHealthUnhealthy = "HEALTH_UNHEALTHY"
)

const (
	ErrSuspension        = "SUSPENSION_ERROR"
	ErrSuspensionActive  = "SUSPENSION_ACTIVE"
	ErrSuspensionAutomatic = "SUSPENSION_AUTOMATIC"
	ErrSuspensionManual  = "SUSPENSION_MANUAL"
)

const (
	ErrSpamDetection   = "SPAM_DETECTION_ERROR"
	ErrSpamScoreHigh   = "SPAM_SCORE_HIGH"
	ErrSpamContentDetected = "SPAM_CONTENT_DETECTED"
	ErrSpamWordDetected = "SPAM_WORD_DETECTED"
)

const (
	ErrBatch          = "BATCH_ERROR"
	ErrBatchProcessing = "BATCH_PROCESSING_ERROR"
	ErrBatchSize      = "BATCH_SIZE_ERROR"
	ErrBatchTimeout   = "BATCH_TIMEOUT"
)

const (
	ErrMigration       = "MIGRATION_ERROR"
	ErrMigrationRun    = "MIGRATION_RUN_ERROR"
	ErrMigrationRollback = "MIGRATION_ROLLBACK_ERROR"
	ErrMigrationVersion = "MIGRATION_VERSION_ERROR"
)

const (
	ErrTenant           = "TENANT_ERROR"
	ErrTenantNotFound   = "TENANT_NOT_FOUND"
	ErrTenantInvalid    = "TENANT_INVALID"
	ErrTenantValidation = "TENANT_VALIDATION_ERROR"
)

const (
	ErrSession         = "SESSION_ERROR"
	ErrSessionExpired  = "SESSION_EXPIRED"
	ErrSessionInvalid  = "SESSION_INVALID"
	ErrSessionNotFound = "SESSION_NOT_FOUND"
	ErrSessionCreate   = "SESSION_CREATE_ERROR"
	ErrSessionDestroy  = "SESSION_DESTROY_ERROR"
)

const (
	ErrToken         = "TOKEN_ERROR"
	ErrTokenInvalid  = "TOKEN_INVALID"
	ErrTokenExpired  = "TOKEN_EXPIRED"
	ErrTokenGeneration = "TOKEN_GENERATION_ERROR"
	ErrTokenValidation = "TOKEN_VALIDATION_ERROR"
	ErrTokenRevoked  = "TOKEN_REVOKED"
)

const (
	ErrDeliverability     = "DELIVERABILITY_ERROR"
	ErrReputation         = "REPUTATION_ERROR"
	ErrReputationScore    = "REPUTATION_SCORE_LOW"
	ErrReputationTracking = "REPUTATION_TRACKING_ERROR"
	ErrFeedbackLoop       = "FEEDBACK_LOOP_ERROR"
)

const (
	ErrMetrics         = "METRICS_ERROR"
	ErrMetricsCollection = "METRICS_COLLECTION_ERROR"
	ErrMetricsExport   = "METRICS_EXPORT_ERROR"
	ErrMetricsQuery    = "METRICS_QUERY_ERROR"
)

const (
	ErrShutdown         = "SHUTDOWN_ERROR"
	ErrShutdownGraceful = "SHUTDOWN_GRACEFUL_ERROR"
	ErrShutdownTimeout  = "SHUTDOWN_TIMEOUT"
	ErrShutdownForced   = "SHUTDOWN_FORCED"
)

var ErrorMessages = map[string]string{
	ErrInternal:          "An internal error occurred",
	ErrNotFound:          "Resource not found",
	ErrAlreadyExists:     "Resource already exists",
	ErrInvalidInput:      "Invalid input provided",
	ErrUnauthorized:      "Authentication required",
	ErrForbidden:         "Access denied",
	ErrValidation:        "Validation failed",
	ErrTimeout:           "Operation timed out",
	ErrDatabase:          "Database operation failed",
	ErrCache:             "Cache operation failed",
	ErrRateLimitExceeded: "Rate limit exceeded",
	ErrConfiguration:     "Configuration error",
	ErrEmailSend:         "Failed to send email",
	ErrSMTP:              "SMTP error occurred",
	ErrCampaignOperation: "Campaign operation failed",
	ErrAccountSuspended:  "Account is suspended",
	ErrTemplateOperation: "Template operation failed",
	ErrProxyConnection:   "Proxy connection failed",
	ErrOAuth2:            "OAuth2 error occurred",
	ErrFileOperation:     "File operation failed",
}

func GetErrorMessage(code string) string {
	if msg, ok := ErrorMessages[code]; ok {
		return msg
	}
	return "An error occurred"
}

func IsRetryableCode(code string) bool {
	retryableCodes := map[string]bool{
		ErrTimeout:            true,
		ErrServiceUnavailable: true,
		ErrDatabaseTimeout:    true,
		ErrDatabaseDeadlock:   true,
		ErrCacheConnection:    true,
		ErrEmailTimeout:       true,
		ErrSMTPTimeout:        true,
		ErrSMTPUnavailable:    true,
		ErrProxyTimeout:       true,
		ErrProxyUnavailable:   true,
		ErrAPITimeout:         true,
		ErrAPIUnavailable:     true,
		ErrConnectionTimeout:  true,
		ErrResourceExhausted:  true,
		ErrWorkerTimeout:      true,
		ErrQueueFull:          true,
		ErrRetryExhausted:     false,
	}
	return retryableCodes[code]
}

func GetHTTPStatusCode(code string) int {
	statusCodes := map[string]int{
		ErrNotFound:              404,
		ErrAlreadyExists:         409,
		ErrInvalidInput:          400,
		ErrUnauthorized:          401,
		ErrForbidden:             403,
		ErrConflict:              409,
		ErrValidation:            400,
		ErrTimeout:               504,
		ErrServiceUnavailable:    503,
		ErrBadRequest:            400,
		ErrMethodNotAllowed:      405,
		ErrUnsupportedMediaType:  415,
		ErrTooLarge:              413,
		ErrPreconditionFailed:    412,
		ErrRateLimitExceeded:     429,
		ErrDailyLimitExceeded:    429,
		ErrRotationLimitExceeded: 429,
		ErrAccountSuspended:      403,
		ErrEmailRejected:         400,
		ErrEmailSpamDetected:     400,
		ErrFilePathTraversal:     400,
		ErrSessionExpired:        401,
		ErrTokenExpired:          401,
		ErrTokenInvalid:          401,
	}

	if status, ok := statusCodes[code]; ok {
		return status
	}
	return 500
}

