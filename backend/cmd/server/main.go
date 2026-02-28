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
	"email-campaign-system/internal/core/proxy"
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

	app, err := initializeApp(db, cfg, appLogger)
	if err != nil {
		return fmt.Errorf("application initialization failed: %w", err)
	}

	return startServer(app, cfg, appLogger)
}

func initLogger(cfg *config.AppConfig) (logger.Logger, error) {
	lvl, err := logger.ParseLevel(cfg.Logging.Level)
	if err != nil {
		lvl = logger.InfoLevel
	}

	return logger.NewZapLogger(cfg.Logging.Level, &logger.Config{
		Level:  lvl,
		Format: cfg.Logging.Format,
	})
}

func initDatabase(cfg *config.AppConfig, log logger.Logger) (*sql.DB, error) {
	var dsn string
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

	h := initHandlers(repos, wsHub, fileStorage, services, cfg, log, v, encryptor)
	log.Info("  ✓ Handlers initialized")

	middlewares := initMiddlewares(cfg, log)
	log.Info("  ✓ Middlewares initialized")

	router := api.NewRouter(
		log,
		h.campaign,
		h.account,
		h.template,
		h.recipient,
		h.proxy,
		h.metrics,
		h.config,
		h.notification,
		h.file,
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
	log.Info("  ✓ Attachment manager initialized")

	// ── Sender Engine (real SMTP delivery) ────────────────────────────
	senderEngine := sender.NewEngine(
		accountMgr,
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
			WorkerCount:         4,
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

	// ── Campaign Executor ──────────────────────────────────────────────
	campaignExecutor := campaign.NewExecutor(
		senderEngine,
		accountMgr,
		attachmentMgr,
		personalizationMgr,
		*repos.recipient,
		*repos.template, // ← the templateRepo fix
		*repos.log,
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

	// ── Campaign Manager ───────────────────────────────────────────────
	campaignManager := campaign.NewManager(
		*repos.campaign,
		*campaignExecutor, // ← was nil — this is the key fix
		nil,               // Persistence (file-based, skip for now)
		nil,               // Scheduler (add when you need scheduled sends)
		nil,               // Cleanup (add when you need auto-purge)
		wsHub,
		log,
		campaign.ManagerConfig{
			MaxConcurrentCampaigns: 5,
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
	campaign     *handlers.CampaignHandler
	account      *handlers.AccountHandler
	template     *handlers.TemplateHandler
	recipient    *handlers.RecipientHandler
	proxy        *handlers.ProxyHandler
	metrics      *handlers.MetricsHandler
	config       *handlers.ConfigHandler
	notification *handlers.NotificationHandler
	file         *handlers.FileHandler
	websocket    *websocket.Handler
}

func initHandlers(
	repos *repositories,
	wsHub *websocket.Hub,
	fileStorage files.Storage,
	services *CoreServices,
	cfg *config.AppConfig,
	log logger.Logger,
	v *validator.Validator,
	encryptor *crypto.AES,
) *appHandlers {
	return &appHandlers{
		account:      handlers.NewAccountHandler(services.AccountMgr, wsHub, log, v, encryptor),
		campaign:     handlers.NewCampaignHandler(services.CampaignManager, wsHub, log, v),
		template:     handlers.NewTemplateHandler(handlers.NewTemplateManagerAdapter(services.TemplateMgr), wsHub, log, v),
		recipient:    handlers.NewRecipientHandler(handlers.NewRecipientManagerAdapter(services.RecipientMgr), wsHub, log, v), 
		proxy:        handlers.NewProxyHandler(services.ProxyMgr, wsHub, log, v),
		metrics:      handlers.NewMetricsHandler(repos.campaign, repos.account, repos.template, repos.recipient, repos.proxy, repos.stats, log),
		config:       handlers.NewConfigHandler(cfg, *repos.config, v, wsHub, log),
		notification: handlers.NewNotificationHandler(repos.config, v, wsHub, log, nil),
		file: handlers.NewFileHandler(
			fileStorage, wsHub, log, v,
			int64(cfg.Storage.MaxUploadSizeMB*1024*1024),
			cfg.Storage.AllowedExtensions,
			cfg.Storage.BasePath,
		),
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
			Enabled: cfg.Security.EnableAuth,
			Token:   cfg.Security.APIKey,
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
