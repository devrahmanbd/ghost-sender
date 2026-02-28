package config

import (
    "time"
)

func GetDefaultConfig() *AppConfig {
    cfg := New()
    cfg.LoadDefaults()
    return cfg
}

func GetProductionDefaults() *AppConfig {
    cfg := GetDefaultConfig()
    cfg.App.Environment = "production"
    cfg.App.Debug = false
    cfg.Server.Mode = "release"
    cfg.Logging.Level = "info"
    cfg.Monitoring.Enabled = true
    cfg.Security.EnableAuth = true
    cfg.Security.EnableEncryption = true
    return cfg
}

func GetDevelopmentDefaults() *AppConfig {
    cfg := GetDefaultConfig()
    cfg.App.Environment = "development"
    cfg.App.Debug = true
    cfg.Server.Mode = "debug"
    cfg.Logging.Level = "debug"
    cfg.Logging.EnableDebugLog = true
    cfg.Monitoring.Enabled = false
    cfg.Security.EnableAuth = false
    return cfg
}

func GetStagingDefaults() *AppConfig {
    cfg := GetDefaultConfig()
    cfg.App.Environment = "staging"
    cfg.App.Debug = false
    cfg.Server.Mode = "release"
    cfg.Logging.Level = "debug"
    cfg.Monitoring.Enabled = true
    cfg.Security.EnableAuth = true
    return cfg
}

func GetTestDefaults() *AppConfig {
    cfg := GetDefaultConfig()
    cfg.App.Environment = "test"
    cfg.App.Debug = true
    cfg.Server.Mode = "test"
    cfg.Server.Port = 8081
    cfg.Database.Database = "email_campaign_test"
    cfg.Logging.Level = "error"
    cfg.Monitoring.Enabled = false
    return cfg
}

func GetDefaultProviders() map[string]ProviderConfig {
    return map[string]ProviderConfig{
        "gmail": {
            Name:                     "Gmail",
            Type:                     "gmail",
            Host:                     "smtp.gmail.com",
            Port:                     587,
            UseSSL:                   false,
            UseTLS:                   true,
            AuthMethod:               "oauth2",
            OAuthTokenURL:            "https://oauth2.googleapis.com/token",
            OAuthScopes:              []string{"https://www.googleapis.com/auth/gmail.send"},
            MaxConnectionsPerAccount: 5,
            ConnectionTimeout:        30 * time.Second,
            SendTimeout:              60 * time.Second,
        },
        "workspace": {
            Name:                     "Google Workspace",
            Type:                     "workspace",
            Host:                     "smtp.gmail.com",
            Port:                     587,
            UseSSL:                   false,
            UseTLS:                   true,
            AuthMethod:               "password",
            MaxConnectionsPerAccount: 5,
            ConnectionTimeout:        30 * time.Second,
            SendTimeout:              60 * time.Second,
        },
        "yahoo": {
            Name:                     "Yahoo Mail",
            Type:                     "yahoo",
            Host:                     "smtp.mail.yahoo.com",
            Port:                     587,
            UseSSL:                   false,
            UseTLS:                   true,
            AuthMethod:               "password",
            MaxConnectionsPerAccount: 3,
            ConnectionTimeout:        30 * time.Second,
            SendTimeout:              60 * time.Second,
        },
        "outlook": {
            Name:                     "Outlook",
            Type:                     "outlook",
            Host:                     "smtp-mail.outlook.com",
            Port:                     587,
            UseSSL:                   false,
            UseTLS:                   true,
            AuthMethod:               "password",
            MaxConnectionsPerAccount: 4,
            ConnectionTimeout:        30 * time.Second,
            SendTimeout:              60 * time.Second,
        },
        "hotmail": {
            Name:                     "Hotmail",
            Type:                     "outlook",
            Host:                     "smtp-mail.outlook.com",
            Port:                     587,
            UseSSL:                   false,
            UseTLS:                   true,
            AuthMethod:               "password",
            MaxConnectionsPerAccount: 4,
            ConnectionTimeout:        30 * time.Second,
            SendTimeout:              60 * time.Second,
        },
        "icloud": {
            Name:                     "iCloud",
            Type:                     "icloud",
            Host:                     "smtp.mail.me.com",
            Port:                     587,
            UseSSL:                   false,
            UseTLS:                   true,
            AuthMethod:               "password",
            MaxConnectionsPerAccount: 3,
            ConnectionTimeout:        30 * time.Second,
            SendTimeout:              60 * time.Second,
        },
        "smtp": {
            Name:                     "Generic SMTP",
            Type:                     "smtp",
            Host:                     "",
            Port:                     587,
            UseSSL:                   false,
            UseTLS:                   true,
            AuthMethod:               "password",
            MaxConnectionsPerAccount: 5,
            ConnectionTimeout:        30 * time.Second,
            SendTimeout:              60 * time.Second,
        },
    }
}

func GetDefaultRotationStrategies() RotationStrategyConfig {
    return RotationStrategyConfig{
        Account: StrategySettings{
            Strategy: "round-robin",
            Weights:  map[string]int{},
            TimeRanges: []TimeRangeRule{},
            CustomRules: map[string]interface{}{
                "prefer_healthy": true,
                "avoid_suspended": true,
            },
        },
        Template: StrategySettings{
            Strategy: "sequential",
            Weights:  map[string]int{},
            TimeRanges: []TimeRangeRule{},
            CustomRules: map[string]interface{}{
                "loop": true,
            },
        },
        SenderName: StrategySettings{
            Strategy: "random",
            Weights:  map[string]int{},
            TimeRanges: []TimeRangeRule{},
            CustomRules: map[string]interface{}{
                "avoid_repeat": true,
            },
        },
        Subject: StrategySettings{
            Strategy: "sequential",
            Weights:  map[string]int{},
            TimeRanges: []TimeRangeRule{},
            CustomRules: map[string]interface{}{
                "loop": true,
            },
        },
        CustomField: StrategySettings{
            Strategy: "sequential",
            Weights:  map[string]int{},
            TimeRanges: []TimeRangeRule{},
            CustomRules: map[string]interface{}{
                "loop": true,
            },
        },
        Attachment: StrategySettings{
            Strategy: "sequential",
            Weights:  map[string]int{},
            TimeRanges: []TimeRangeRule{},
            CustomRules: map[string]interface{}{
                "loop": true,
            },
        },
        Proxy: StrategySettings{
            Strategy: "round-robin",
            Weights:  map[string]int{},
            TimeRanges: []TimeRangeRule{},
            CustomRules: map[string]interface{}{
                "prefer_healthy": true,
            },
        },
    }
}

func ApplyDefaultsToApp(cfg *ApplicationConfig) {
    if cfg.Name == "" {
        cfg.Name = "Email Campaign System"
    }
    if cfg.Version == "" {
        cfg.Version = "1.0.0"
    }
    if cfg.Environment == "" {
        cfg.Environment = "production"
    }
    if cfg.Timezone == "" {
        cfg.Timezone = "UTC"
    }
    if cfg.Locale == "" {
        cfg.Locale = "en"
    }
}

func ApplyDefaultsToServer(cfg *ServerConfig) {
    if cfg.Host == "" {
        cfg.Host = "0.0.0.0"
    }
    if cfg.Port == 0 {
        cfg.Port = 8080
    }
    if cfg.Mode == "" {
        cfg.Mode = "release"
    }
    if cfg.ReadTimeout == 0 {
        cfg.ReadTimeout = 10 * time.Second
    }
    if cfg.WriteTimeout == 0 {
        cfg.WriteTimeout = 10 * time.Second
    }
    if cfg.IdleTimeout == 0 {
        cfg.IdleTimeout = 120 * time.Second
    }
    if cfg.MaxHeaderBytes == 0 {
        cfg.MaxHeaderBytes = 1048576
    }
    if len(cfg.AllowedOrigins) == 0 {
        cfg.AllowedOrigins = []string{"*"}
    }
}

func ApplyDefaultsToDatabase(cfg *DatabaseConfig) {
    if cfg.Driver == "" {
        cfg.Driver = "postgres"
    }
    if cfg.Host == "" {
        cfg.Host = "localhost"
    }
    if cfg.Port == 0 {
        cfg.Port = 5432
    }
    if cfg.Database == "" {
        cfg.Database = "email_campaign"
    }
    if cfg.Username == "" {
        cfg.Username = "postgres"
    }
    if cfg.SSLMode == "" {
        cfg.SSLMode = "disable"
    }
    if cfg.MaxOpenConns == 0 {
        cfg.MaxOpenConns = 25
    }
    if cfg.MaxIdleConns == 0 {
        cfg.MaxIdleConns = 5
    }
    if cfg.ConnMaxLifetime == 0 {
        cfg.ConnMaxLifetime = 5 * time.Minute
    }
    if cfg.MigrationsPath == "" {
        cfg.MigrationsPath = "./migrations"
    }
}

func ApplyDefaultsToCache(cfg *CacheConfig) {
    if cfg.Type == "" {
        cfg.Type = "redis"
    }
    if cfg.Host == "" {
        cfg.Host = "localhost"
    }
    if cfg.Port == 0 {
        cfg.Port = 6379
    }
    if cfg.MaxRetries == 0 {
        cfg.MaxRetries = 3
    }
    if cfg.PoolSize == 0 {
        cfg.PoolSize = 10
    }
    if cfg.MinIdleConns == 0 {
        cfg.MinIdleConns = 2
    }
    if cfg.DefaultExpiration == 0 {
        cfg.DefaultExpiration = 1 * time.Hour
    }
    if cfg.CleanupInterval == 0 {
        cfg.CleanupInterval = 10 * time.Minute
    }
}

func ApplyDefaultsToStorage(cfg *StorageConfig) {
    if cfg.Type == "" {
        cfg.Type = "local"
    }
    if cfg.BasePath == "" {
        cfg.BasePath = "./storage"
    }
    if cfg.TempPath == "" {
        cfg.TempPath = "./storage/temp"
    }
    if cfg.UploadPath == "" {
        cfg.UploadPath = "./storage/uploads"
    }
    if cfg.TemplatePath == "" {
        cfg.TemplatePath = "./storage/templates"
    }
    if cfg.AttachmentPath == "" {
        cfg.AttachmentPath = "./storage/attachments"
    }
    if cfg.LogPath == "" {
        cfg.LogPath = "./storage/logs"
    }
    if cfg.BackupPath == "" {
        cfg.BackupPath = "./storage/backups"
    }
    if cfg.MaxUploadSizeMB == 0 {
        cfg.MaxUploadSizeMB = 100
    }
    if len(cfg.AllowedExtensions) == 0 {
        cfg.AllowedExtensions = []string{".zip", ".html", ".pdf", ".jpg", ".png", ".csv", ".txt"}
    }
}

func ApplyDefaultsToEmail(cfg *EmailConfig) {
    if cfg.DefaultCharset == "" {
        cfg.DefaultCharset = "UTF-8"
    }
    if cfg.DefaultContentType == "" {
        cfg.DefaultContentType = "text/html"
    }
    if cfg.SendTimeout == 0 {
        cfg.SendTimeout = 30 * time.Second
    }
    if cfg.ConnectionTimeout == 0 {
        cfg.ConnectionTimeout = 10 * time.Second
    }
    if cfg.RetryAttempts == 0 {
        cfg.RetryAttempts = 3
    }
    if cfg.RetryDelay == 0 {
        cfg.RetryDelay = 5 * time.Second
    }
}

func ApplyDefaultsToAccount(cfg *AccountConfig) {
    if cfg.RotationStrategy == "" {
        cfg.RotationStrategy = "round-robin"
    }
    if cfg.RotationLimit == 0 {
        cfg.RotationLimit = 100
    }
    if cfg.DailyLimit == 0 {
        cfg.DailyLimit = 500
    }
    if cfg.HourlyLimit == 0 {
        cfg.HourlyLimit = 50
    }
    if cfg.CooldownDuration == 0 {
        cfg.CooldownDuration = 60 * time.Second
    }
    if cfg.HealthCheckInterval == 0 {
        cfg.HealthCheckInterval = 5 * time.Minute
    }
    if cfg.HealthThreshold == 0 {
        cfg.HealthThreshold = 50.0
    }
    if cfg.SuspendThreshold == 0 {
        cfg.SuspendThreshold = 10
    }
    if cfg.ConsecutiveFailures == 0 {
        cfg.ConsecutiveFailures = 5
    }
}

func ApplyDefaultsToCampaign(cfg *CampaignConfig) {
    if cfg.MaxConcurrent == 0 {
        cfg.MaxConcurrent = 10
    }
    if cfg.DefaultPriority == "" {
        cfg.DefaultPriority = "normal"
    }
    if cfg.CheckpointInterval == 0 {
        cfg.CheckpointInterval = 1 * time.Minute
    }
    if cfg.ProgressUpdateInterval == 0 {
        cfg.ProgressUpdateInterval = 5 * time.Second
    }
    if cfg.RetryFailedAfter == 0 {
        cfg.RetryFailedAfter = 1 * time.Hour
    }
    if cfg.MaxRetryAttempts == 0 {
        cfg.MaxRetryAttempts = 3
    }
    if len(cfg.MilestonePercents) == 0 {
        cfg.MilestonePercents = []int{25, 50, 75}
    }
}

func ApplyDefaultsToTemplate(cfg *TemplateConfig) {
    if cfg.RotationStrategy == "" {
        cfg.RotationStrategy = "sequential"
    }
    if cfg.CacheTTL == 0 {
        cfg.CacheTTL = 1 * time.Hour
    }
    if cfg.SpamScoreThreshold == 0 {
        cfg.SpamScoreThreshold = 5.0
    }
    if cfg.MaxTemplateSizeKB == 0 {
        cfg.MaxTemplateSizeKB = 500
    }
    if len(cfg.AllowedTags) == 0 {
        cfg.AllowedTags = []string{
            "a", "abbr", "b", "blockquote", "br", "caption", "cite", "code",
            "col", "colgroup", "dd", "div", "dl", "dt", "em", "h1", "h2", "h3",
            "h4", "h5", "h6", "i", "img", "li", "ol", "p", "pre", "q", "small",
            "span", "strong", "sub", "sup", "table", "tbody", "td", "tfoot",
            "th", "thead", "tr", "u", "ul",
        }
    }
    if cfg.DefaultVariables == nil {
        cfg.DefaultVariables = map[string]string{
            "company_name": "Company Name",
            "support_email": "support@example.com",
            "website": "https://example.com",
        }
    }
}

func ApplyDefaultsToPersonalization(cfg *PersonalizationConfig) {
    if cfg.DefaultTimezone == "" {
        cfg.DefaultTimezone = "UTC"
    }
    if cfg.DefaultDateFormat == "" {
        cfg.DefaultDateFormat = "2006-01-02"
    }
    if cfg.CustomFieldRotation == "" {
        cfg.CustomFieldRotation = "sequential"
    }
    if cfg.SenderNameRotation == "" {
        cfg.SenderNameRotation = "random"
    }
    if cfg.SubjectRotation == "" {
        cfg.SubjectRotation = "sequential"
    }
    if len(cfg.SenderNames) == 0 {
        cfg.SenderNames = []string{"Support Team", "Customer Service", "Info"}
    }
    if len(cfg.SubjectLines) == 0 {
        cfg.SubjectLines = []string{"Important Update", "New Information", "Message for You"}
    }
}

func ApplyDefaultsToAttachment(cfg *AttachmentConfig) {
    if cfg.RotationStrategy == "" {
        cfg.RotationStrategy = "sequential"
    }
    if cfg.CachePath == "" {
        cfg.CachePath = "./storage/attachments/cache"
    }
    if cfg.MaxCacheSizeMB == 0 {
        cfg.MaxCacheSizeMB = 1024
    }
    if cfg.MaxAttachmentSizeMB == 0 {
        cfg.MaxAttachmentSizeMB = 25
    }
    if len(cfg.SupportedFormats) == 0 {
        cfg.SupportedFormats = []string{"pdf", "jpg", "jpeg", "png", "webp", "heic", "heif"}
    }
    if cfg.ConversionBackend == "" {
        cfg.ConversionBackend = "chromedp"
    }
    if cfg.ImageQuality == 0 {
        cfg.ImageQuality = 85
    }
    if cfg.ImageFormat == "" {
        cfg.ImageFormat = "jpeg"
    }
}

func ApplyDefaultsToProxy(cfg *ProxyConfig) {
    if cfg.RotationStrategy == "" {
        cfg.RotationStrategy = "round-robin"
    }
    if cfg.HealthCheckInterval == 0 {
        cfg.HealthCheckInterval = 5 * time.Minute
    }
    if cfg.HealthCheckTimeout == 0 {
        cfg.HealthCheckTimeout = 10 * time.Second
    }
    if cfg.MaxRetries == 0 {
        cfg.MaxRetries = 3
    }
    if cfg.RetryDelay == 0 {
        cfg.RetryDelay = 2 * time.Second
    }
    if cfg.ConnectionTimeout == 0 {
        cfg.ConnectionTimeout = 30 * time.Second
    }
    if cfg.IdleConnTimeout == 0 {
        cfg.IdleConnTimeout = 90 * time.Second
    }
}

func ApplyDefaultsToRateLimit(cfg *RateLimitConfig) {
    if cfg.GlobalRPS == 0 {
        cfg.GlobalRPS = 10.0
    }
    if cfg.PerAccountRPS == 0 {
        cfg.PerAccountRPS = 2.0
    }
    if cfg.BurstSize == 0 {
        cfg.BurstSize = 20
    }
    if cfg.AdaptiveMinRPS == 0 {
        cfg.AdaptiveMinRPS = 1.0
    }
    if cfg.AdaptiveMaxRPS == 0 {
        cfg.AdaptiveMaxRPS = 20.0
    }
    if cfg.AdaptiveAdjustInterval == 0 {
        cfg.AdaptiveAdjustInterval = 1 * time.Minute
    }
    if cfg.APIRateLimit == 0 {
        cfg.APIRateLimit = 100
    }
    if cfg.APIRateLimitWindow == 0 {
        cfg.APIRateLimitWindow = 1 * time.Minute
    }
}

func ApplyDefaultsToWorker(cfg *WorkerConfig) {
    if cfg.MinWorkers == 0 {
        cfg.MinWorkers = 1
    }
    if cfg.MaxWorkers == 0 {
        cfg.MaxWorkers = 10
    }
    if cfg.DefaultWorkers == 0 {
        cfg.DefaultWorkers = 4
    }
    if cfg.QueueSize == 0 {
        cfg.QueueSize = 1000
    }
    if cfg.BatchSize == 0 {
        cfg.BatchSize = 100
    }
    if cfg.ScaleUpThreshold == 0 {
        cfg.ScaleUpThreshold = 0.8
    }
    if cfg.ScaleDownThreshold == 0 {
        cfg.ScaleDownThreshold = 0.2
    }
    if cfg.IdleTimeout == 0 {
        cfg.IdleTimeout = 60 * time.Second
    }
    if cfg.GracefulShutdownTimeout == 0 {
        cfg.GracefulShutdownTimeout = 30 * time.Second
    }
}

func ApplyDefaultsToNotification(cfg *NotificationConfig) {
    if cfg.Channel == "" {
        cfg.Channel = "telegram"
    }
    if cfg.TelegramParseMode == "" {
        cfg.TelegramParseMode = "Markdown"
    }
    if cfg.MaxRetries == 0 {
        cfg.MaxRetries = 3
    }
    if cfg.RetryDelay == 0 {
        cfg.RetryDelay = 5 * time.Second
    }
}

func ApplyDefaultsToLogging(cfg *LoggingConfig) {
    if cfg.Level == "" {
        cfg.Level = "info"
    }
    if cfg.Format == "" {
        cfg.Format = "json"
    }
    if len(cfg.OutputPaths) == 0 {
        cfg.OutputPaths = []string{"stdout", "./storage/logs/app.log"}
    }
    if len(cfg.ErrorOutputPaths) == 0 {
        cfg.ErrorOutputPaths = []string{"stderr", "./storage/logs/error.log"}
    }
    if cfg.MaxSizeMB == 0 {
        cfg.MaxSizeMB = 100
    }
    if cfg.MaxBackups == 0 {
        cfg.MaxBackups = 10
    }
    if cfg.MaxAgeDays == 0 {
        cfg.MaxAgeDays = 30
    }
}

func ApplyDefaultsToSecurity(cfg *SecurityConfig) {
    if cfg.JWTExpiration == 0 {
        cfg.JWTExpiration = 24 * time.Hour
    }
    if cfg.EncryptionAlgorithm == "" {
        cfg.EncryptionAlgorithm = "AES-256-GCM"
    }
    if cfg.HashAlgorithm == "" {
        cfg.HashAlgorithm = "SHA256"
    }
    if cfg.SessionMaxAge == 0 {
        cfg.SessionMaxAge = 24 * time.Hour
    }
}

func ApplyDefaultsToMonitoring(cfg *MonitoringConfig) {
    if cfg.MetricsPort == 0 {
        cfg.MetricsPort = 9090
    }
    if cfg.MetricsPath == "" {
        cfg.MetricsPath = "/metrics"
    }
    if cfg.ProfilingPort == 0 {
        cfg.ProfilingPort = 6060
    }
    if cfg.HealthCheckPath == "" {
        cfg.HealthCheckPath = "/health"
    }
    if cfg.HealthCheckInterval == 0 {
        cfg.HealthCheckInterval = 30 * time.Second
    }
}

func ApplyDefaultsToCleanup(cfg *CleanupConfig) {
    if cfg.CleanupInterval == 0 {
        cfg.CleanupInterval = 1 * time.Hour
    }
    if cfg.DeleteCompletedAfter == 0 {
        cfg.DeleteCompletedAfter = 7 * 24 * time.Hour
    }
    if cfg.DeleteFailedAfter == 0 {
        cfg.DeleteFailedAfter = 30 * 24 * time.Hour
    }
    if cfg.DeleteLogsAfter == 0 {
        cfg.DeleteLogsAfter = 90 * 24 * time.Hour
    }
    if cfg.DeleteTempFilesAfter == 0 {
        cfg.DeleteTempFilesAfter = 24 * time.Hour
    }
    if cfg.DeleteCacheAfter == 0 {
        cfg.DeleteCacheAfter = 7 * 24 * time.Hour
    }
    if cfg.CleanupBatchSize == 0 {
        cfg.CleanupBatchSize = 1000
    }
    if cfg.BackupInterval == 0 {
        cfg.BackupInterval = 24 * time.Hour
    }
}

func GetMinimalConfig() *AppConfig {
    return &AppConfig{
        App: ApplicationConfig{
            Name:        "Email Campaign System",
            Version:     "1.0.0",
            Environment: "production",
        },
        Server: ServerConfig{
            Host: "0.0.0.0",
            Port: 8080,
        },
        Database: DatabaseConfig{
            Host:     "localhost",
            Port:     5432,
            Database: "email_campaign",
            Username: "postgres",
        },
        Worker: WorkerConfig{
            MinWorkers:     1,
            MaxWorkers:     4,
            DefaultWorkers: 2,
        },
    }
}

func GetHighPerformanceDefaults() *AppConfig {
    cfg := GetDefaultConfig()
    
    cfg.Server.ReadTimeout = 30 * time.Second
    cfg.Server.WriteTimeout = 30 * time.Second
    cfg.Server.IdleTimeout = 180 * time.Second
    
    cfg.Database.MaxOpenConns = 100
    cfg.Database.MaxIdleConns = 25
    cfg.Database.ConnMaxLifetime = 10 * time.Minute
    
    cfg.Cache.PoolSize = 50
    cfg.Cache.MinIdleConns = 10
    
    cfg.Worker.MaxWorkers = 20
    cfg.Worker.DefaultWorkers = 10
    cfg.Worker.QueueSize = 5000
    cfg.Worker.BatchSize = 500
    cfg.Worker.EnableAutoScaling = true
    
    cfg.RateLimit.GlobalRPS = 50.0
    cfg.RateLimit.PerAccountRPS = 5.0
    cfg.RateLimit.BurstSize = 100
    
    cfg.Campaign.MaxConcurrent = 50
    
    return cfg
}

func GetLowResourceDefaults() *AppConfig {
    cfg := GetDefaultConfig()
    
    cfg.Database.MaxOpenConns = 10
    cfg.Database.MaxIdleConns = 2
    
    cfg.Cache.PoolSize = 5
    cfg.Cache.MinIdleConns = 1
    
    cfg.Worker.MinWorkers = 1
    cfg.Worker.MaxWorkers = 2
    cfg.Worker.DefaultWorkers = 1
    cfg.Worker.QueueSize = 100
    cfg.Worker.BatchSize = 50
    cfg.Worker.EnableAutoScaling = false
    
    cfg.RateLimit.GlobalRPS = 5.0
    cfg.RateLimit.PerAccountRPS = 1.0
    cfg.RateLimit.BurstSize = 10
    
    cfg.Campaign.MaxConcurrent = 2
    
    cfg.Template.EnableCaching = false
    cfg.Attachment.EnableCaching = false
    
    return cfg
}

func GetSecureDefaults() *AppConfig {
    cfg := GetDefaultConfig()
    
    cfg.Server.TLSEnabled = true
    cfg.Server.EnableCORS = false
    
    cfg.Security.EnableAuth = true
    cfg.Security.EnableAPIKey = true
    cfg.Security.EnableEncryption = true
    cfg.Security.EnableCSRF = true
    cfg.Security.EnableRateLimit = true
    
    cfg.Logging.Level = "warn"
    cfg.Logging.EnableDebugLog = false
    
    cfg.Monitoring.Enabled = true
    cfg.Monitoring.EnableMetrics = true
    cfg.Monitoring.EnableHealthCheck = true
    
    return cfg
}

func GetBulkEmailDefaults() *AppConfig {
    cfg := GetDefaultConfig()
    
    cfg.Account.DailyLimit = 2000
    cfg.Account.HourlyLimit = 200
    cfg.Account.RotationLimit = 500
    
    cfg.Email.RetryAttempts = 5
    cfg.Email.EnableFBL = true
    cfg.Email.EnableUnsubscribe = true
    
    cfg.Template.EnableRotation = true
    cfg.Template.EnableSpamCheck = true
    
    cfg.Personalization.EnableSmartExtraction = true
    cfg.Personalization.EnableRandomGeneration = true
    cfg.Personalization.EnableTimeBasedContent = true
    
    cfg.RateLimit.Enabled = true
    cfg.RateLimit.EnableAdaptive = true
    
    return cfg
}

func GetDevelopmentSafeDefaults() *AppConfig {
    cfg := GetDevelopmentDefaults()
    
    cfg.Account.DailyLimit = 10
    cfg.Account.HourlyLimit = 5
    cfg.Account.RotationLimit = 5
    
    cfg.Campaign.MaxConcurrent = 1
    
    cfg.Worker.MaxWorkers = 2
    cfg.Worker.DefaultWorkers = 1
    
    cfg.RateLimit.GlobalRPS = 1.0
    cfg.RateLimit.PerAccountRPS = 0.5
    
    cfg.Email.RetryAttempts = 1
    
    return cfg
}

func ApplyEnvironmentDefaults(cfg *AppConfig, environment string) {
    switch environment {
    case "production":
        prodCfg := GetProductionDefaults()
        cfg.App = prodCfg.App
        cfg.Logging.Level = prodCfg.Logging.Level
        cfg.Server.Mode = prodCfg.Server.Mode
        cfg.Security = prodCfg.Security
        
    case "development":
        devCfg := GetDevelopmentDefaults()
        cfg.App = devCfg.App
        cfg.Logging.Level = devCfg.Logging.Level
        cfg.Server.Mode = devCfg.Server.Mode
        
    case "staging":
        stagingCfg := GetStagingDefaults()
        cfg.App = stagingCfg.App
        cfg.Logging.Level = stagingCfg.Logging.Level
        cfg.Server.Mode = stagingCfg.Server.Mode
        
    case "test":
        testCfg := GetTestDefaults()
        cfg.App = testCfg.App
        cfg.Server = testCfg.Server
        cfg.Database = testCfg.Database
        cfg.Logging.Level = testCfg.Logging.Level
    }
}

func GetDefaultPersonalizationVariables() map[string]string {
    return map[string]string{
        "RECIPIENT_EMAIL":     "{{.RecipientEmail}}",
        "RECIPIENT_NAME":      "{{.RecipientName}}",
        "FIRST_NAME":          "{{.FirstName}}",
        "LAST_NAME":           "{{.LastName}}",
        "TODAY_DATE":          "{{.TodayDate}}",
        "CURRENT_TIME":        "{{.CurrentTime}}",
        "CURRENT_YEAR":        "{{.CurrentYear}}",
        "COMPANY_NAME":        "{{.CompanyName}}",
        "SENDER_NAME":         "{{.SenderName}}",
        "SENDER_EMAIL":        "{{.SenderEmail}}",
        "UNSUBSCRIBE_LINK":    "{{.UnsubscribeLink}}",
        "TRACKING_PIXEL":      "{{.TrackingPixel}}",
    }
}

func GetDefaultTimeRanges() []TimeRangeRule {
    return []TimeRangeRule{
        {StartHour: 6, EndHour: 12, Value: "morning", Weight: 1},
        {StartHour: 12, EndHour: 17, Value: "afternoon", Weight: 1},
        {StartHour: 17, EndHour: 21, Value: "evening", Weight: 1},
        {StartHour: 21, EndHour: 6, Value: "night", Weight: 1},
    }
}
