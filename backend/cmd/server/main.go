package main

import (
        "context"
        "database/sql"
        "fmt"
        "log"
        "os"
        "os/signal"
        "syscall"
        "time"

        "github.com/joho/godotenv"
        _ "github.com/lib/pq"
        "runtime"

        "email-campaign-system/internal/core/logging"
        "email-campaign-system/internal/core/proxy"
        "email-campaign-system/internal/storage/cache"
        "email-campaign-system/internal/api"
        "email-campaign-system/internal/api/handlers"
        "email-campaign-system/internal/api/middleware"
        "email-campaign-system/internal/api/websocket"
        "email-campaign-system/internal/config"
        "email-campaign-system/internal/core/account"
        "email-campaign-system/internal/core/attachment"
        "email-campaign-system/internal/core/campaign"
        "email-campaign-system/internal/core/personalization"
        "email-campaign-system/internal/core/provider"
        "email-campaign-system/internal/core/recipient"
        "email-campaign-system/internal/core/template"
        "email-campaign-system/internal/storage/files"
        "email-campaign-system/internal/storage/repository"
        "email-campaign-system/pkg/crypto"
        "email-campaign-system/pkg/logger"
        "email-campaign-system/pkg/validator"
        "email-campaign-system/internal/core/sender"
)

const (
        appVersion = "1.0.0"
        appBuild   = "2026.02"
)

func main() {
        if err := run(); err != nil {
                log.Fatal(err)
        }
}

func run() error {
        _ = godotenv.Load()

        cfg, err := config.Load("./configs/config.yaml")
        if err != nil {
                return fmt.Errorf("failed to load config: %w", err)
        }

        if err := cfg.Validate(); err != nil {
                return fmt.Errorf("config validation failed: %w", err)
        }

        appLogger, err := initLogger(cfg)
        if err != nil {
                return fmt.Errorf("failed to initialize logger: %w", err)
        }

        appLogger.Info(fmt.Sprintf("🚀 Starting Email Campaign System v%s (Build %s)", appVersion, appBuild))
        appLogger.Info(fmt.Sprintf("📊 Environment: %s", cfg.App.Environment))

        db, err := initDatabase(cfg, appLogger)
        if err != nil {
                return fmt.Errorf("database initialization failed: %w", err)
        }
        defer db.Close()

        logRepo := repository.NewLogRepository(db)
        dbLogger := logging.NewDBLogger(logRepo, appLogger, appVersion, cfg.App.Environment)
        appLogger = dbLogger

        logStartupEvents(dbLogger, cfg)

        app, err := initializeApp(db, cfg, appLogger)
        if err != nil {
                return fmt.Errorf("application initialization failed: %w", err)
        }

        if dl, ok := appLogger.(*logging.DBLogger); ok {
                go runPeriodicHealthLog(dl)
        }

        return startServer(app, cfg, appLogger)
}

func initLogger(cfg *config.AppConfig) (logger.Logger, error) {
        lvl, err := logger.ParseLevel(cfg.Logging.Level)
        if err != nil {
                lvl = logger.InfoLevel
        }

        outputPaths := cfg.Logging.OutputPaths
        if len(outputPaths) == 0 {
                outputPaths = []string{"stdout"}
        }
        errorOutputPaths := cfg.Logging.ErrorOutputPaths
        if len(errorOutputPaths) == 0 {
                errorOutputPaths = []string{"stderr"}
        }

        return logger.NewZapLogger(cfg.Logging.Level, &logger.Config{
                Level:            lvl,
                Format:           cfg.Logging.Format,
                OutputPaths:      outputPaths,
                ErrorOutputPaths: errorOutputPaths,
        })
}

func initDatabase(cfg *config.AppConfig, log logger.Logger) (*sql.DB, error) {
        dsn := os.Getenv("DATABASE_URL")
        if dsn == "" {
                if cfg.Database.Password != "" {
                        dsn = fmt.Sprintf(
                                "host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
                                cfg.Database.Host,
                                cfg.Database.Port,
                                cfg.Database.Username,
                                cfg.Database.Password,
                                cfg.Database.Database,
                                cfg.Database.SSLMode,
                        )
                } else {
                        dsn = fmt.Sprintf(
                                "host=%s port=%d user=%s dbname=%s sslmode=%s",
                                cfg.Database.Host,
                                cfg.Database.Port,
                                cfg.Database.Username,
                                cfg.Database.Database,
                                cfg.Database.SSLMode,
                        )
                }
        }

        db, err := sql.Open("postgres", dsn)
        if err != nil {
                return nil, fmt.Errorf("failed to open database: %w", err)
        }

        db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
        db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
        db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := db.PingContext(ctx); err != nil {
                db.Close()
                return nil, fmt.Errorf("failed to ping database: %w", err)
        }

        log.Info("✅ Database connected successfully")
        return db, nil
}

type CoreServices struct {
        AccountMgr         *account.AccountManager
        RecipientMgr       *recipient.RecipientManager
        TemplateMgr        *template.TemplateManager
        PersonalizationMgr *personalization.Manager
        AttachmentMgr      *attachment.Manager
        ProviderFactory    *provider.ProviderFactory
        CampaignManager    *campaign.Manager
        ProxyMgr           *proxy.ProxyManager
        Cache              *cache.TieredCache
}

type Application struct {
        DB          *sql.DB
        Config      *config.AppConfig
        Logger      logger.Logger
        Router      *api.Router
        Server      *api.Server
        Hub         *websocket.Hub
        FileStorage files.Storage
        Services    *CoreServices
}

func initializeApp(db *sql.DB, cfg *config.AppConfig, log logger.Logger) (*Application, error) {
        log.Info("🔧 Initializing application...")

        // FIX: Create a root context for the full initialization lifetime so
        // NewAccountManager (and any other startup I/O) can be cancelled on error.
        ctx := context.Background()

        v := validator.New()
        encryptor, err := crypto.NewAES(cfg.Security.EncryptionKey)
        if err != nil {
                return nil, fmt.Errorf("failed to initialize encryptor: %w", err)
        }

        repos := initRepositories(db)
        log.Info("  ✓ Repositories initialized")

        wsHub := websocket.NewHub(log)
        log.Info("  ✓ WebSocket hub created")

        fileStorage, err := initFileStorage(cfg)
        if err != nil {
                return nil, fmt.Errorf("failed to initialize file storage: %w", err)
        }
        log.Info("  ✓ File storage initialized")

        // FIX: Pass ctx down so NewAccountManager receives it.
        services, err := initCoreServices(ctx, repos, cfg, log, wsHub)
        if err != nil {
                return nil, fmt.Errorf("failed to initialize core services: %w", err)
        }
        log.Info("  ✓ Core services initialized")

        h := initHandlers(db, repos, wsHub, fileStorage, services, cfg, log, v, encryptor)
        log.Info("  ✓ Handlers initialized")

        middlewares := initMiddlewares(cfg, log)
        log.Info("  ✓ Middlewares initialized")

        router := api.NewRouter(
                log,
                h.campaign,
                h.account,
                h.template,
                h.recipient,
                h.recipientList,
                h.proxy,
                h.metrics,
                h.config,
                h.notification,
                h.file,
                h.auth,
                h.websocket,
                middlewares.auth,
                middlewares.rateLimit,
                middlewares.cors,
                middlewares.logging,
                middlewares.recovery,
                middlewares.tenant,
        )
        log.Info("  ✓ Router configured")

        server := api.NewServer(router, wsHub, cfg, log)
        log.Info("  ✓ HTTP server created")

        log.Info("✅ Application initialization complete!")

        return &Application{
                DB:          db,
                Config:      cfg,
                Logger:      log,
                Router:      router,
                Server:      server,
                Hub:         wsHub,
                FileStorage: fileStorage,
                Services:    services,
        }, nil
}
func initCoreServices(ctx context.Context, repos *repositories, cfg *config.AppConfig, log logger.Logger, wsHub *websocket.Hub) (*CoreServices, error) {
        log.Info("🔧 Initializing core services...")

        providerFactory := provider.NewProviderFactory(log)
        log.Info("  ✓ Provider factory created")

        accountMgr, err := account.NewAccountManager(
                ctx,
                *repos.account,
                log,
                &account.ManagerConfig{
                        EnableAutoRotation:    true,
                        EnableHealthCheck:     true,
                        EnableAutoSuspension:  true,
                        HealthCheckInterval:   5 * time.Minute,
                        SuspensionThreshold:   5,
                        MaxConcurrentUse:      10,
                        AccountCooldown:       30 * time.Second,
                        ProviderRetryAttempts: 3,
                },
        )
        if err != nil {
                return nil, fmt.Errorf("failed to create account manager: %w", err)
        }
        log.Info("  ✓ Account manager initialized")

        // FIX 1: repos.account is already *repository.AccountRepository — do NOT dereference
        simpleAcctMgr := account.NewManager(repos.account)

        recipientMgr := recipient.NewRecipientManager(
                *repos.recipient,
                recipient.NewValidator(),
                log,
        )
        log.Info("  ✓ Recipient manager initialized")

        templateMgr, err := template.NewTemplateManager(
                *repos.template,
                log,
                &template.ManagerConfig{
                        BasePath:            cfg.Storage.TemplatePath,
                        EnableAutoReload:    false,
                        EnableCaching:       true,
                        EnableSpamDetection: true,
                        EnableValidation:    true,
                        ReloadInterval:      5 * time.Minute,
                        CacheExpiry:         30 * time.Minute,
                        SpamScoreThreshold:  7.0,
                        MinHealthScore:      50.0,
                },
        )
        if err != nil {
                return nil, fmt.Errorf("failed to create template manager: %w", err)
        }
        log.Info("  ✓ Template manager initialized")

        personalizationMgr := personalization.NewManager(log, &personalization.PersonalizationConfig{})
        log.Info("  ✓ Personalization manager initialized")

        attachmentMgr, err := attachment.NewManager(&attachment.Config{
                TemplateDir:    cfg.Storage.AttachmentPath,
                DefaultFormat:  attachment.FormatPDF,
                EnableCache:    true,
                EnableRotation: true,
        })
        if err != nil {
                return nil, fmt.Errorf("failed to create attachment manager: %w", err)
        }
        
        // Load attachment templates from directory
        if err := attachmentMgr.LoadTemplates(); err != nil {
                log.Warn(fmt.Sprintf("failed to load attachment templates: %v (continuing)", err))
        }
        
        os.MkdirAll("./temp", 0755)
        converterCfg := &attachment.ConverterConfig{
                Backend: attachment.BackendNative,
                TempDir: "./temp",
        }
        converter, err := attachment.NewMultiConverter(converterCfg)
        if err != nil {
                log.Warn(fmt.Sprintf("failed to initialize converter: %v (attachments will not work)", err))
        } else {
                attachmentMgr.SetConverter(converter)
                log.Info("  ✓ Converter initialized for attachments (wkhtmltopdf)")
        }
        
        log.Info("  ✓ Attachment manager initialized")

        // FIX 2: simpleAcctMgr is already *account.Manager — do NOT take its address
        senderEngine := sender.NewEngine(
                simpleAcctMgr,
                newSenderTemplateAdapter(*repos.template, log),
                attachmentMgr,
                personalizationMgr,
                &simpleDeliverabilityManager{},
                newProviderFactoryAdapter(providerFactory, log),
                &noopRateLimiter{},
                *repos.log,
                *repos.stats,
                log,
                sender.EngineConfig{
                        WorkerCount:         0,
                        QueueSize:           1000,
                        BatchSize:           100,
                        MaxRetries:          3,
                        RetryDelay:          5 * time.Second,
                        SendTimeout:         30 * time.Second,
                        EnableRateLimiting:  false,
                        StatsUpdateInterval: 5 * time.Second,
                        ProgressInterval:    1 * time.Second,
                },
        )
        log.Info("  ✓ Sender engine created")

        // FIX 3: same — simpleAcctMgr is already *account.Manager
        campaignExecutor := campaign.NewExecutor(
                senderEngine,
                simpleAcctMgr,
                attachmentMgr,
                personalizationMgr,
                *repos.recipient,
                *repos.template,
                repos.log,
                *repos.stats,
                log,
                campaign.ExecutorConfig{
                        BatchSize:           100,
                        WorkerCount:         4,
                        MaxRetries:          3,
                        RetryDelay:          5 * time.Second,
                        StatsUpdateInterval: 5 * time.Second,
                        CheckpointInterval:  30 * time.Second,
                },
        )
        log.Info("  ✓ Campaign executor created")

        // Use security.max_concurrent_campaigns as the single source of truth.
        // -1 = all campaigns blocked (none may start)
        //  0 = unlimited (no restriction)
        // >0 = at most N campaigns may run concurrently
        maxConcurrentCampaigns := cfg.Security.MaxConcurrentCampaigns
        // Write back so the live config pointer reflects the resolved value.
        cfg.Security.MaxConcurrentCampaigns = maxConcurrentCampaigns

        campaignManager := campaign.NewManager(
                *repos.campaign,
                campaignExecutor,
                nil,
                nil,
                nil,
                wsHub,
                log,
                campaign.ManagerConfig{
                        MaxConcurrentCampaigns: maxConcurrentCampaigns,
                        CleanupInterval:        1 * time.Hour,
                        StatsUpdateInterval:    5 * time.Second,
                        CheckpointInterval:     30 * time.Second,
                },
        )
        log.Info("  ✓ Campaign manager initialized")

        proxyMgr, err := proxy.NewProxyManager(*repos.proxy, proxy.DefaultProxyConfig())
        if err != nil {
                return nil, fmt.Errorf("failed to create proxy manager: %w", err)
        }
        log.Info("  ✓ Proxy manager initialized")

        redisHost := ""
        if cfg.Cache.Host != "" {
                redisHost = cfg.Cache.Host
        }
        tieredCache := cache.NewTieredCache(&cache.CacheConfig{
                Host:     redisHost,
                Port:     cfg.Cache.Port,
                Password: cfg.Cache.Password,
                Database: cfg.Cache.Database,
        })
        if tieredCache.IsRedisAvailable() {
                log.Info("  ✓ Cache initialized (Redis primary + memory fallback)")
        } else {
                log.Info("  ✓ Cache initialized (memory-only, Redis unavailable)")
        }

        log.Info("✅ All core services ready")

        return &CoreServices{
                AccountMgr:         accountMgr,
                RecipientMgr:       recipientMgr,
                TemplateMgr:        templateMgr,
                PersonalizationMgr: personalizationMgr,
                AttachmentMgr:      attachmentMgr,
                ProviderFactory:    providerFactory,
                CampaignManager:    campaignManager,
                ProxyMgr:           proxyMgr,
                Cache:              tieredCache,
        }, nil
}


type repositories struct {
        account   *repository.AccountRepository
        campaign  *repository.CampaignRepository
        template  *repository.TemplateRepository
        recipient *repository.RecipientRepository
        proxy     *repository.ProxyRepository
        stats     *repository.StatsRepository
        log       *repository.LogRepository
        config    *repository.ConfigRepository
}


func initRepositories(db *sql.DB) *repositories {
        return &repositories{
                account:   repository.NewAccountRepository(db),
                campaign:  repository.NewCampaignRepository(db),
                template:  repository.NewTemplateRepository(db),
                recipient: repository.NewRecipientRepository(db),
                proxy:     repository.NewProxyRepository(db),
                stats:     repository.NewStatsRepository(db),
                log:       repository.NewLogRepository(db),
                config:    repository.NewConfigRepository(db),
        }
}

type appHandlers struct {
        campaign      *handlers.CampaignHandler
        account       *handlers.AccountHandler
        template      *handlers.TemplateHandler
        recipient     *handlers.RecipientHandler
        recipientList *handlers.RecipientListHandler
        proxy         *handlers.ProxyHandler
        metrics       *handlers.MetricsHandler
        config        *handlers.ConfigHandler
        notification  *handlers.NotificationHandler
        file          *handlers.FileHandler
        auth          *handlers.AuthHandler
        websocket     *websocket.Handler
}

func initHandlers(
        db *sql.DB,
        repos *repositories,
        wsHub *websocket.Hub,
        fileStorage files.Storage,
        services *CoreServices,
        cfg *config.AppConfig,
        log logger.Logger,
        v *validator.Validator,
        encryptor *crypto.AES,
) *appHandlers {
        authH := handlers.NewAuthHandler(cfg, log)
        // Wire the campaign manager into the auth handler so UpdateLicense
        // propagates the new concurrent limit to the manager in real time.
        authH.WithCampaignManagerLimiter(services.CampaignManager)

        return &appHandlers{
                account:      handlers.NewAccountHandler(services.AccountMgr, wsHub, log, v, encryptor),
                campaign:     handlers.NewCampaignHandler(services.CampaignManager, wsHub, log, v,
                        handlers.WithLogRepo(repos.log),
                        handlers.WithAccountRepo(repos.account),
                        handlers.WithRecipientRepo(repos.recipient),
                        handlers.WithConfig(cfg),
                ),
                template:     handlers.NewTemplateHandler(handlers.NewTemplateManagerAdapter(services.TemplateMgr), wsHub, log, v),
                recipient:    handlers.NewRecipientHandler(handlers.NewRecipientManagerAdapter(services.RecipientMgr), wsHub, log, v),
                recipientList: handlers.NewRecipientListHandler(db, log),
                proxy:        handlers.NewProxyHandler(services.ProxyMgr, wsHub, log, v),
                metrics:      handlers.NewMetricsHandler(repos.campaign, repos.account, repos.template, repos.recipient, repos.proxy, repos.stats, repos.log, log),
                config:       handlers.NewConfigHandler(cfg, *repos.config, v, wsHub, log),
                notification: handlers.NewNotificationHandler(repos.config, v, wsHub, log, nil),
                file: handlers.NewFileHandler(
                        fileStorage, wsHub, log, v,
                        int64(cfg.Storage.MaxUploadSizeMB*1024*1024),
                        cfg.Storage.AllowedExtensions,
                        cfg.Storage.BasePath,
                        handlers.WithAttachmentManager(services.AttachmentMgr),
                ),
                auth:      authH,
                websocket: websocket.NewHandler(wsHub, log),
        }
}

type appMiddlewares struct {
        auth      *middleware.AuthMiddleware
        rateLimit *middleware.RateLimitMiddleware
        cors      *middleware.CORSMiddleware
        logging   *middleware.LoggingMiddleware
        recovery  *middleware.RecoveryMiddleware
        tenant    *middleware.TenantMiddleware
}

func initMiddlewares(cfg *config.AppConfig, log logger.Logger) *appMiddlewares {
        return &appMiddlewares{
                auth: middleware.NewAuthMiddleware(log, middleware.AuthConfig{
                        Enabled:   cfg.Security.EnableAuth,
                        Token:     cfg.Security.APIKey,
                        JWTSecret: cfg.Security.JWTSecret,
                        BypassPaths: []string{
                                "/api/v1/auth/login",
                                "/health",
                        },
                }),
                rateLimit: middleware.NewRateLimitMiddleware(log, middleware.RateLimitConfig{
                        Enabled: cfg.RateLimit.Enabled,
                        RPS:     cfg.RateLimit.GlobalRPS,
                        Burst:   cfg.RateLimit.BurstSize,
                        PerIP:   true,
                }),
                cors: middleware.NewCORSMiddleware(middleware.CORSConfig{
                        AllowedOrigins:   cfg.Server.AllowedOrigins,
                        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
                        AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Requested-With"},
                        AllowCredentials: true,
                        MaxAgeSeconds:    600,
                }),
                logging: middleware.NewLoggingMiddleware(log, middleware.LoggingConfig{
                        Enabled:            true,
                        LogRequestHeaders:  cfg.Logging.Level == "debug",
                        LogResponseHeaders: cfg.Logging.Level == "debug",
                        RequestIDHeader:    "X-Request-ID",
                }),
                recovery: middleware.NewRecoveryMiddleware(log, middleware.RecoveryConfig{
                        Enabled: true,
                }),
                tenant: middleware.NewTenantMiddleware(log, middleware.TenantConfig{
                        Enabled:       false,
                        HeaderName:    "X-Tenant-ID",
                        DefaultTenant: "default",
                }),
        }
}

func initFileStorage(cfg *config.AppConfig) (files.Storage, error) {
        storageConfig := &files.StorageConfig{
                BasePath:      cfg.Storage.BasePath,
                MaxFileSize:   int64(cfg.Storage.MaxUploadSizeMB * 1024 * 1024),
                TempDir:       cfg.Storage.TempPath,
                AutoClean:     true,
                CleanInterval: 1 * time.Hour,
                CleanAge:      7 * 24 * time.Hour,
                Permissions:   0755,
        }
        return files.NewLocalStorage(storageConfig)
}

func startServer(app *Application, cfg *config.AppConfig, log logger.Logger) error {
        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()

        errChan := make(chan error, 1)

        go func() {
                log.Info(fmt.Sprintf("🌐 Server starting on %s:%d", cfg.Server.Host, cfg.Server.Port))
                log.Info(fmt.Sprintf("📊 Health: http://%s:%d/health", cfg.Server.Host, cfg.Server.Port))
                log.Info(fmt.Sprintf("🔌 API: http://%s:%d/api/v1", cfg.Server.Host, cfg.Server.Port))

                if err := app.Server.Start(ctx); err != nil {
                        errChan <- fmt.Errorf("server failed to start: %w", err)
                }
        }()

        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

        log.Info("✅ Server ready! Press Ctrl+C to shutdown...")

        select {
        case err := <-errChan:
                return err
        case sig := <-sigChan:
                log.Info(fmt.Sprintf("🛑 Shutdown signal received: %v", sig))
        }

        log.Info("⏳ Shutting down gracefully...")

        shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer shutdownCancel()

        if err := app.Server.Shutdown(shutdownCtx); err != nil {
                return fmt.Errorf("server shutdown failed: %w", err)
        }

        log.Info("✅ Server stopped gracefully")
        return nil
}

func runPeriodicHealthLog(dbLogger *logging.DBLogger) {
        ticker := time.NewTicker(10 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
                var m runtime.MemStats
                runtime.ReadMemStats(&m)
                dbLogger.InsertDirect(&repository.LogEntry{
                        Level:     repository.LogLevelInfo,
                        Category:  repository.LogCategorySystem,
                        Message:   fmt.Sprintf("Periodic health check: %d goroutines, %.1f MB allocated", runtime.NumGoroutine(), float64(m.Alloc)/1024/1024),
                        Source:    "health_check",
                        Component: "system",
                        Details: map[string]interface{}{
                                "goroutines":      runtime.NumGoroutine(),
                                "memory_alloc_mb": float64(m.Alloc) / 1024 / 1024,
                                "memory_sys_mb":   float64(m.Sys) / 1024 / 1024,
                                "heap_alloc_mb":   float64(m.HeapAlloc) / 1024 / 1024,
                                "num_gc":          m.NumGC,
                        },
                })
        }
}

func logStartupEvents(dbLogger *logging.DBLogger, cfg *config.AppConfig) {
        var m runtime.MemStats
        runtime.ReadMemStats(&m)

        dbLogger.InsertDirect(&repository.LogEntry{
                Level:    repository.LogLevelSystem,
                Category: repository.LogCategorySystem,
                Message:  fmt.Sprintf("Email Campaign System v%s (Build %s) starting", appVersion, appBuild),
                Source:   "startup",
                Details: map[string]interface{}{
                        "version":     appVersion,
                        "build":       appBuild,
                        "environment": cfg.App.Environment,
                        "go_version":  runtime.Version(),
                        "num_cpu":     runtime.NumCPU(),
                },
        })

        dbLogger.InsertDirect(&repository.LogEntry{
                Level:    repository.LogLevelInfo,
                Category: repository.LogCategorySystem,
                Message:  "Database connected successfully",
                Source:   "startup",
                Component: "database",
                Details: map[string]interface{}{
                        "max_open_conns": cfg.Database.MaxOpenConns,
                        "max_idle_conns": cfg.Database.MaxIdleConns,
                },
        })

        dbLogger.InsertDirect(&repository.LogEntry{
                Level:    repository.LogLevelInfo,
                Category: repository.LogCategorySystem,
                Message:  fmt.Sprintf("System health check: %d goroutines, %.1f MB memory allocated", runtime.NumGoroutine(), float64(m.Alloc)/1024/1024),
                Source:   "health_check",
                Component: "system",
                Details: map[string]interface{}{
                        "goroutines":  runtime.NumGoroutine(),
                        "memory_alloc_mb": float64(m.Alloc) / 1024 / 1024,
                        "memory_sys_mb":   float64(m.Sys) / 1024 / 1024,
                        "heap_alloc_mb":   float64(m.HeapAlloc) / 1024 / 1024,
                        "num_gc":          m.NumGC,
                },
        })

        dbLogger.InsertDirect(&repository.LogEntry{
                Level:    repository.LogLevelInfo,
                Category: repository.LogCategorySystem,
                Message:  fmt.Sprintf("Server configured on %s:%d", cfg.Server.Host, cfg.Server.Port),
                Source:   "startup",
                Component: "server",
                Details: map[string]interface{}{
                        "host":          cfg.Server.Host,
                        "port":          cfg.Server.Port,
                        "auth_enabled":  cfg.Security.EnableAuth,
                        "rate_limiting": cfg.RateLimit.Enabled,
                },
        })
}
