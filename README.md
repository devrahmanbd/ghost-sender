

email-campaign-system/
├── backend/                                    # Golang Backend (Go 1.21+)
│   │
│   ├── cmd/                                    # Application Entry Points
│   │   ├── server/                             # Main API + Worker Server
│   │   │   └── main.go                         # [210 lines] Server initialization, graceful shutdown
│   │   └── cli/                                # CLI Tool
│   │       └── main.go                         # [350 lines] CLI interface, commands
│   │
│   ├── internal/                               # Private Application Code
│   │   │
│   │   ├── api/                                # HTTP API Layer
│   │   │   ├── handlers/                       # HTTP Request Handlers
│   │   │   │   ├── campaign.go                 # [480 lines] Campaign CRUD, start/stop/pause
│   │   │   │   ├── account.go                  # [420 lines] Account management, test connection
│   │   │   │   ├── template.go                 # [380 lines] Template CRUD, preview, spam check
│   │   │   │   ├── recipient.go                # [320 lines] Import, validate, bulk ops
│   │   │   │   ├── proxy.go                    # [280 lines] Proxy CRUD, test, health check
│   │   │   │   ├── metrics.go                  # [200 lines] System metrics, campaign stats
│   │   │   │   ├── config.go                   # [240 lines] Config get/update, validation
│   │   │   │   ├── notification.go             # [180 lines] Telegram config, test notification
│   │   │   │   └── file.go                     # [220 lines] File upload, download, ZIP extract
│   │   │   │
│   │   │   ├── middleware/                     # HTTP Middleware
│   │   │   │   ├── tenant.go                   # [120 lines] Single-tenant validation
│   │   │   │   ├── ratelimit.go                # [150 lines] API rate limiting
│   │   │   │   ├── cors.go                     # [80 lines] CORS headers
│   │   │   │   ├── logging.go                  # [100 lines] Request/response logging
│   │   │   │   ├── recovery.go                 # [90 lines] Panic recovery
│   │   │   │   └── auth.go                     # [140 lines] Simple token auth
│   │   │   │
│   │   │   ├── websocket/                      # WebSocket Support
│   │   │   │   ├── hub.go                      # [280 lines] WS connection hub, broadcasting
│   │   │   │   ├── handlers.go                 # [220 lines] WS endpoint handlers
│   │   │   │   └── client.go                   # [180 lines] WS client connection
│   │   │   │
│   │   │   ├── routes.go                       # [320 lines] All route definitions
│   │   │   └── server.go                       # [240 lines] HTTP server setup, middleware chain
│   │   │
│   │   ├── core/                               # Core Business Logic
│   │   │   │
│   │   │   ├── campaign/                       # Campaign Management
│   │   │   │   ├── manager.go                  # [520 lines] Campaign lifecycle, orchestration
│   │   │   │   ├── state.go                    # [280 lines] State machine (created→running→completed)
│   │   │   │   ├── executor.go                 # [640 lines] Campaign execution engine
│   │   │   │   ├── persistence.go              # [310 lines] State save/load, checkpointing
│   │   │   │   ├── scheduler.go                # [240 lines] Scheduled campaigns
│   │   │   │   └── cleanup.go                  # [180 lines] Automatic cleanup policies
│   │   │   │
│   │   │   ├── sender/                         # Email Sending Engine
│   │   │   │   ├── engine.go                   # [580 lines] Main sending orchestration
│   │   │   │   ├── worker_pool.go              # [420 lines] Goroutine worker pool management
│   │   │   │   ├── queue.go                    # [340 lines] Priority email queue
│   │   │   │   ├── batch.go                    # [280 lines] Batch processing logic
│   │   │   │   └── retry.go                    # [220 lines] Retry with exponential backoff
│   │   │   │
│   │   │   ├── provider/                       # Email Provider Implementations
│   │   │   │   ├── interface.go                # [120 lines] Provider interface definition
│   │   │   │   ├── factory.go                  # [180 lines] Provider factory pattern
│   │   │   │   ├── gmail.go                    # [640 lines] Gmail OAuth2, API integration
│   │   │   │   ├── smtp.go                     # [480 lines] Generic SMTP with TLS/SSL
│   │   │   │   ├── yahoo.go                    # [320 lines] Yahoo SMTP (587)
│   │   │   │   ├── outlook.go                  # [320 lines] Outlook/Hotmail SMTP (587)
│   │   │   │   ├── icloud.go                   # [320 lines] iCloud SMTP (587)
│   │   │   │   ├── workspace.go                # [380 lines] Google Workspace app password
│   │   │   │   └── connection_pool.go          # [280 lines] Provider connection pooling
│   │   │   │
│   │   │   ├── account/                        # Account Management System
│   │   │   │   ├── manager.go                  # [520 lines] Account pool management
│   │   │   │   ├── rotator.go                  # [480 lines] Smart rotation (round-robin, weighted, health-based)
│   │   │   │   ├── health.go                   # [380 lines] Health monitoring, scoring
│   │   │   │   ├── suspension.go               # [420 lines] Auto-suspension on errors/spam
│   │   │   │   ├── limiter.go                  # [280 lines] Daily/rotation limits per account
│   │   │   │
│   │   │   ├── template/                       # Template System (Like gsend.py)
│   │   │   │   ├── manager.go                  # [580 lines] Template pool, loading from dir
│   │   │   │   ├── rotator.go                  # [420 lines] Unlimited rotation (1.html→∞.html)
│   │   │   │   ├── renderer.go                 # [680 lines] Variable replacement, rendering
│   │   │   │   ├── parser.go                   # [380 lines] HTML parsing, variable extraction
│   │   │   │   ├── validator.go                # [280 lines] HTML validation, sanitization
│   │   │   │   ├── spam_detector.go            # [720 lines] ⭐ Spam content analysis & scoring
│   │   │   │   └── cache.go                    # [220 lines] Rendered template caching
│   │   │   │
│   │   │   ├── personalization/                # ⭐ Personalization Engine (50+ Variables)
│   │   │   │   ├── manager.go                  # [420 lines] Personalization orchestration
│   │   │   │   ├── variables.go                # [880 lines] All 50+ variable definitions
│   │   │   │   ├── generator.go                # [520 lines] Static generators (date, invoice, phone)
│   │   │   │   ├── dynamic.go                  # [640 lines] Dynamic processors (RANDOM_NUM_X, CUSTOM_DATE)
│   │   │   │   ├── extractor.go                # [280 lines] Smart name extraction from email
│   │   │   │   ├── datetime.go                 # [340 lines] Date formatting, offset support
│   │   │   │   │
│   │   │   │   └── rotation/                   # ⭐⭐ ROTATION SYSTEM (NEW!)
│   │   │   │       ├── interface.go            # [180 lines] Rotator interface, strategies
│   │   │   │       ├── manager.go              # [420 lines] Master rotation manager
│   │   │   │       ├── sender_name.go          # [680 lines] Sender name rotation (4 strategies)
│   │   │   │       ├── subject.go              # [580 lines] Subject line rotation
│   │   │   │       ├── custom_field.go         # [480 lines] Custom field rotation
│   │   │   │       ├── strategies.go           # [320 lines] Sequential, random, weighted, time-based
│   │   │   │       └── stats.go                # [180 lines] Rotation statistics tracking
│   │   │   │
│   │   │   ├── attachment/                     # Attachment Processing
│   │   │   │   ├── manager.go                  # [420 lines] Attachment orchestration
│   │   │   │   ├── converter.go                # [580 lines] HTML→PDF/Image conversion
│   │   │   │   ├── cache.go                    # [380 lines] Attachment caching (hash-based)
│   │   │   │   ├── rotator.go                  # [280 lines] Format rotation (PDF→JPG→PNG→WebP)
│   │   │   │   └── formats/                    # Format-specific handlers
│   │   │   │       ├── pdf.go                  # [420 lines] PDF generation (chromedp)
│   │   │   │       ├── image.go                # [380 lines] JPG/PNG generation
│   │   │   │       └── webp.go                 # [320 lines] WebP generation
│   │   │   │
│   │   │   ├── proxy/                          # ⭐ Proxy System
│   │   │   │   ├── manager.go                  # [520 lines] Proxy pool management
│   │   │   │   ├── rotator.go                  # [420 lines] Proxy rotation strategies
│   │   │   │   ├── validator.go                # [380 lines] Proxy health checking
│   │   │   │   ├── types.go                    # [220 lines] HTTP/HTTPS/SOCKS5 support
│   │   │   │   ├── dialer.go                   # [480 lines] Custom net.Dialer with proxy
│   │   │   │   └── authenticator.go            # [180 lines] Proxy authentication
│   │   │   │
│   │   │   ├── notification/                   # ⭐ Notification System
│   │   │   │   ├── manager.go                  # [320 lines] Notification orchestration
│   │   │   │   ├── telegram.go                 # [620 lines] Telegram bot integration
│   │   │   │   ├── dispatcher.go               # [380 lines] Event → notification dispatcher
│   │   │   │   ├── templates.go                # [420 lines] Rich notification templates
│   │   │   │   ├── queue.go                    # [240 lines] Notification queue
│   │   │   │   └── formatter.go                # [180 lines] Message formatting (Markdown/HTML)
│   │   │   │
│   │   │   ├── recipient/                      # Recipient Management
│   │   │   │   ├── manager.go                  # [420 lines] Recipient pool management
│   │   │   │   ├── importer.go                 # [480 lines] CSV/TXT import, parsing
│   │   │   │   ├── validator.go                # [380 lines] Email validation, DNS check
│   │   │   │   ├── deduplicator.go             # [240 lines] Duplicate detection/removal
│   │   │   │   └── bulk_ops.go                 # [320 lines] Bulk delete operations
│   │   │   │
│   │   │   ├── deliverability/                 # Email Deliverability
│   │   │   │   ├── headers.go                  # [480 lines] FBL, List-Unsubscribe, Message-ID
│   │   │   │   ├── mime.go                     # [420 lines] Multipart MIME formatting
│   │   │   │   ├── reputation.go               # [380 lines] Spam score tracking per account
│   │   │   │   ├── unsubscribe.go              # [280 lines] Unsubscribe link generation
│   │   │   │
│   │   │   └── ratelimiter/                    # Rate Limiting System
│   │   │       ├── limiter.go                  # [320 lines] Rate limiter interface
│   │   │       ├── token_bucket.go             # [420 lines] Token bucket algorithm
│   │   │       ├── adaptive.go                 # [380 lines] Adaptive rate adjustment
│   │   │       ├── distributed.go              # [480 lines] Redis-backed distributed limiter
│   │   │       └── per_account.go              # [240 lines] Per-account rate limiting
│   │   │
│   │   ├── storage/                            # Data Storage Layer
│   │   │   │
│   │   │   ├── database/                       # PostgreSQL Integration
│   │   │   │   ├── postgres.go                 # [420 lines] DB connection, pooling
│   │   │   │   ├── migrations.go               # [280 lines] Migration runner
│   │   │   │   ├── queries.go                  # [640 lines] SQL query builders
│   │   │   │   └── transaction.go              # [180 lines] Transaction management
│   │   │   │
│   │   │   ├── cache/                          # Caching Layer
│   │   │   │   ├── interface.go                # [120 lines] Cache interface
│   │   │   │   ├── redis.go                    # [420 lines] Redis implementation
│   │   │   │   ├── memory.go                   # [280 lines] In-memory fallback
│   │   │   │   └── serializer.go               # [180 lines] Data serialization
│   │   │   │
│   │   │   ├── files/                          # File Storage
│   │   │   │   ├── interface.go                # [100 lines] Storage interface
│   │   │   │   ├── local.go                    # [380 lines] Local filesystem
│   │   │   │   └── zip.go                      # [320 lines] ZIP archive handling
│   │   │   │
│   │   │   └── repository/                     # Data Access Objects (DAO)
│   │   │       ├── campaign.go                 # [520 lines] Campaign CRUD, queries
│   │   │       ├── account.go                  # [480 lines] Account CRUD, filtering
│   │   │       ├── template.go                 # [420 lines] Template CRUD, versioning
│   │   │       ├── recipient.go                # [380 lines] Recipient CRUD, bulk ops
│   │   │       ├── proxy.go                    # [320 lines] Proxy CRUD, health tracking
│   │   │       ├── log.go                      # [420 lines] Log storage, querying
│   │   │       ├── stats.go                    # [280 lines] Statistics aggregation
│   │   │       └── config.go                   # [240 lines] Config persistence
│   │   │
│   │   ├── models/                             # Data Models (Domain Objects)
│   │   │   ├── campaign.go                     # [380 lines] Campaign model, validation
│   │   │   ├── account.go                      # [420 lines] Account model, provider enum
│   │   │   ├── template.go                     # [320 lines] Template model, spam score
│   │   │   ├── recipient.go                    # [240 lines] Recipient model
│   │   │   ├── proxy.go                        # [280 lines] Proxy model, types
│   │   │   ├── email.go                        # [380 lines] Email message model
│   │   │   ├── attachment.go                   # [220 lines] Attachment model
│   │   │   ├── log.go                          # [280 lines] Log entry model
│   │   │   ├── stats.go                        # [240 lines] Statistics model
│   │   │   ├── notification.go                 # [180 lines] Notification model
│   │   │   └── config.go                       # [520 lines] Configuration model
│   │   │
│   │   │
│   │   └── config/                             # Configuration Management
│   │       ├── config.go                       # [680 lines] Config struct, all sections
│   │       ├── loader.go                       # [420 lines] YAML/ENV loading
│   │       ├── validator.go                    # [380 lines] Config validation rules
│   │       ├── defaults.go                     # [520 lines] Default configuration values
│   │       └── watcher.go                      # [240 lines] Hot reload (future)
│   │
│   ├── pkg/                                    # Public Reusable Packages
│   │   │
│   │   ├── logger/                             # Structured Logging
│   │   │   ├── logger.go                       # [420 lines] Logger interface
│   │   │   ├── zap.go                          # [320 lines] Zap implementation
│   │   │   └── context.go                      # [180 lines] Context-aware logging
│   │   │
│   │   ├── validator/                          # Input Validation
│   │   │   ├── validator.go                    # [380 lines] Validation rules
│   │   │   └── custom.go                       # [220 lines] Custom validators
│   │   │
│   │   ├── errors/                             # Error Handling
│   │   │   ├── errors.go                       # [320 lines] Custom error types
│   │   │   ├── codes.go                        # [180 lines] Error codes
│   │   │   └── handler.go                      # [240 lines] Error response handler
│   │   │
│   │   ├── crypto/                             # Cryptography Utilities
│   │   │   ├── aes.go                          # [280 lines] AES-256 encryption
│   │   │   ├── hash.go                         # [180 lines] Hashing (SHA256, bcrypt)
│   │   │   ├── jwt.go                          # [240 lines] JWT token management
│   │   │   └── hmac.go                         # [160 lines] HMAC signature
│   │   │
│   │   ├── utils/                              # General Utilities
│   │   │   ├── strings.go                      # [220 lines] String helpers
│   │   │   ├── time.go                         # [180 lines] Time utilities
│   │   │   ├── file.go                         # [280 lines] File operations
│   │   │   ├── random.go                       # [240 lines] Random generators
│   │   │   └── email.go                        # [320 lines] Email parsing/validation
│   │   │
│   │   └── proxypool/                          # Proxy Pool Utilities
│   │       ├── pool.go                         # [420 lines] Generic proxy pool
│   │       ├── checker.go                      # [320 lines] Proxy health checker
│   │       └── balancer.go                     # [280 lines] Load balancing
│   │
│   ├── migrations/                             # Database Migrations (SQL)
│   │   ├── 000001_init_schema.up.sql           # [280 lines] Initial tables
│   │   ├── 000001_init_schema.down.sql         # [80 lines] Rollback
│   │   ├── 000002_add_proxies.up.sql           # [120 lines] Proxy tables
│   │   ├── 000002_add_proxies.down.sql         # [40 lines] Rollback
│   │   ├── 000003_add_telegram.up.sql          # [80 lines] Telegram config
│   │   ├── 000003_add_telegram.down.sql        # [30 lines] Rollback
│   │   ├── 000004_add_rotation.up.sql          # [160 lines] Rotation tracking tables
│   │   ├── 000004_add_rotation.down.sql        # [50 lines] Rollback
│   │   ├── 000005_add_indexes.up.sql           # [120 lines] Performance indexes
│   │   └── 000005_add_indexes.down.sql         # [40 lines] Rollback
│   │
│   ├── configs/                                # Configuration Files
│   │   ├── config.yaml                         # [420 lines] Production config
│   │   ├── config.example.yaml                 # [450 lines] Example with comments
│   │   ├── config.dev.yaml                     # [380 lines] Development config
│   │   ├── providers.yaml                      # [280 lines] SMTP provider configs
│   │   └── rotation.yaml                       # [180 lines] Rotation strategies config
│   ├── tests/                                  # Test Files
│   │   ├── unit/                               # Unit tests
│   │   │   ├── rotation_test.go                # [480 lines] Rotation tests
│   │   │   ├── template_test.go                # [420 lines] Template tests
│   │   │   ├── spam_detector_test.go           # [380 lines] Spam detector tests
│   │   │   └── ...
│   │   ├── integration/                        # Integration tests
│   │   │   ├── campaign_test.go                # [520 lines] Campaign flow tests
│   │   │   ├── provider_test.go                # [480 lines] Provider tests
│   │   │   └── ...
│   │   └── e2e/                                # End-to-end tests
│   │       └── full_campaign_test.go           # [680 lines] Complete flow
│   │
│   ├── .env.example                            # [80 lines] Environment variables template
│   ├── .gitignore                              # [60 lines] Git ignore rules
│   ├── go.mod                                  # [40 lines] Go dependencies
│   ├── go.sum                                  # [Auto-generated] Dependency checksums
│   ├── Makefile                                # [280 lines] Build automation
│   ├── Dockerfile                              # [80 lines] Docker image
│   ├── docker-compose.yml                      # [180 lines] Full stack (Go + PG + Redis + PHP)
│   ├── .dockerignore                           # [30 lines] Docker ignore
│   └── README.md                               # [400 lines] Project overview
│
└── frontend/                                   # PHP Frontend (PHP 8.2+)
    │
    ├── public/                                 # Web-accessible files
    │   ├── index.php                           # [120 lines] Application entry point
    │   ├── .htaccess                           # [60 lines] Apache rewrite rules
    │   │
    │   ├── assets/                             # Static assets
    │   │   ├── css/
    │   │   │   ├── app.css                     # [2400 lines] Main stylesheet
    │   │   │   ├── dashboard.css               # [1200 lines] Dashboard styles
    │   │   │   ├── editor.css                  # [800 lines] Code editor styles
    │   │   │   └── vendor/                     # Third-party CSS
    │   │   │       ├── tailwind.min.css
    │   │   │       ├── codemirror.css
    │   │   │       └── chart.css
    │   │   │
    │   │   ├── js/
    │   │   │   ├── app.js                      # [1800 lines] Main JavaScript
    │   │   │   ├── websocket.js                # [620 lines] WebSocket client
    │   │   │   ├── campaign.js                 # [880 lines] Campaign UI logic
    │   │   │   ├── template-editor.js          # [720 lines] Template editor
    │   │   │   ├── spam-checker.js             # [420 lines] Spam check UI
    │   │   │   ├── proxy-manager.js            # [520 lines] Proxy management UI
    │   │   │   ├── charts.js                   # [480 lines] Chart rendering
    │   │   │   └── vendor/                     # Third-party JS
    │   │   │       ├── alpine.min.js
    │   │   │       ├── chart.min.js
    │   │   │       ├── codemirror.min.js
    │   │   │       └── socket.io.min.js
    │   │   │
    │   │   └── images/                         # Images
    │   │       ├── logo.svg
    │   │       └── icons/
    │   │
    │   └── uploads/                            # Temporary file uploads
    │       └── .gitkeep
    │
    ├── src/                                    # PHP source code
    │   │
    │   ├── Controllers/                        # MVC Controllers
    │   │   ├── BaseController.php              # [180 lines] Base controller
    │   │   ├── DashboardController.php         # [420 lines] Dashboard, metrics
    │   │   ├── CampaignController.php          # [720 lines] Campaign CRUD, monitoring
    │   │   ├── AccountController.php           # [580 lines] Account management
    │   │   ├── TemplateController.php          # [680 lines] Template CRUD, editor, spam check
    │   │   ├── RecipientController.php         # [520 lines] Recipient import, management
    │   │   ├── ProxyController.php             # [480 lines] Proxy CRUD, testing
    │   │   ├── ConfigController.php            # [420 lines] Configuration UI
    │   │   ├── NotificationController.php      # [320 lines] Telegram setup
    │   │   ├── LogController.php               # [280 lines] Log viewing
    │   │   └── FileController.php              # [380 lines] File upload/download
    │   │
    │   ├── Services/                           # Business Logic Services
    │   │   ├── ApiClient.php                   # [880 lines] Complete backend API wrapper
    │   │   ├── WebSocketClient.php             # [420 lines] WS connection manager
    │   │   ├── TenantService.php               # [220 lines] Single tenant management
    │   │   ├── CacheService.php                # [280 lines] Frontend caching
    │   │   └── ValidationService.php           # [320 lines] Form validation
    │   │
    │   ├── Models/                             # Data Models (PHP representations)
    │   │   ├── Campaign.php                    # [240 lines] Campaign model
    │   │   ├── Account.php                     # [220 lines] Account model
    │   │   ├── Template.php                    # [200 lines] Template model
    │   │   ├── Recipient.php                   # [160 lines] Recipient model
    │   │   └── Proxy.php                       # [180 lines] Proxy model
    │   │
    │   ├── Views/                              # Templates (Twig or plain PHP)
    │   │   │
    │   │   ├── layouts/                        # Layout templates
    │   │   │   ├── app.php                     # [280 lines] Main layout
    │   │   │   ├── header.php                  # [140 lines] Header component
    │   │   │   ├── sidebar.php                 # [320 lines] Sidebar navigation
    │   │   │   └── footer.php                  # [80 lines] Footer component
    │   │   │
    │   │   ├── dashboard/                      # Dashboard views
    │   │   │   └── index.php                   # [520 lines] Main dashboard
    │   │   │
    │   │   ├── campaign/                       # Campaign views
    │   │   │   ├── list.php                    # [420 lines] Campaign list with filters
    │   │   │   ├── create.php                  # [680 lines] Create campaign wizard
    │   │   │   ├── edit.php                    # [620 lines] Edit campaign
    │   │   │   ├── monitor.php                 # [780 lines] Real-time monitoring
    │   │   │   └── logs.php                    # [380 lines] Campaign logs
    │   │   │
    │   │   ├── account/                        # Account views
    │   │   │   ├── list.php                    # [480 lines] Account list
    │   │   │   ├── add.php                     # [580 lines] Add account form
    │   │   │   ├── edit.php                    # [520 lines] Edit account
    │   │   │   └── suspended.php               # [320 lines] Suspended accounts
    │   │   │
    │   │   ├── template/                       # Template views
    │   │   │   ├── list.php                    # [420 lines] Template list
    │   │   │   ├── editor.php                  # [880 lines] HTML editor with CodeMirror
    │   │   │   ├── spam-check.php              # [520 lines] Spam detector UI
    │   │   │   ├── preview.php                 # [380 lines] Template preview
    │   │   │   └── rotation.php                # [420 lines] Rotation settings
    │   │   │
    │   │   ├── recipient/                      # Recipient views
    │   │   │   ├── list.php                    # [380 lines] Recipient list
    │   │   │   ├── import.php                  # [620 lines] Import wizard
    │   │   │   └── manage.php                  # [480 lines] Bulk operations
    │   │   │
    │   │   ├── proxy/                          # Proxy views
    │   │   │   ├── list.php                    # [420 lines] Proxy list
    │   │   │   ├── add.php                     # [480 lines] Add proxy form
    │   │   │   ├── edit.php                    # [420 lines] Edit proxy
    │   │   │   └── test.php                    # [320 lines] Proxy testing UI
    │   │   │
    │   │   ├── notification/                   # Notification views
    │   │   │   └── telegram.php                # [580 lines] Telegram bot setup
    │   │   │
    │   │   ├── config/                         # Configuration views
    │   │   │   ├── general.php                 # [520 lines] General settings
    │   │   │   ├── limits.php                  # [420 lines] Limit settings
    │   │   │   ├── rotation.php                # [680 lines] Rotation configuration
    │   │   │   └── advanced.php                # [480 lines] Advanced settings
    │   │   │
    │   │   └── components/                     # Reusable components
    │   │       ├── alert.php                   # [80 lines] Alert component
    │   │       ├── modal.php                   # [120 lines] Modal component
    │   │       ├── table.php                   # [180 lines] Data table
    │   │       └── chart.php                   # [140 lines] Chart component
    │   │
    │   ├── Middleware/                         # HTTP Middleware
    │   │   ├── TenantMiddleware.php            # [180 lines] Tenant validation
    │   │   ├── CsrfMiddleware.php              # [140 lines] CSRF protection
    │   │   └── SessionMiddleware.php           # [120 lines] Session handling
    │   │
    │   └── Config/                             # Configuration
    │       ├── app.php                         # [220 lines] App config
    │       ├── routes.php                      # [380 lines] Route definitions
    │       └── database.php                    # [80 lines] DB config (if needed)
    │
    ├── storage/                                # Storage directories
    │   ├── logs/                               # Application logs
    │   │   └── .gitkeep
    │   ├── cache/                              # Cache files
    │   │   └── .gitkeep
    │   └── sessions/                           # Session files
    │       └── .gitkeep
    │
    ├── vendor/                                 # Composer dependencies (auto-generated)
    │
    ├── composer.json                           # [80 lines] PHP dependencies
    ├── composer.lock                           # [Auto-generated] Dependency lock
    ├── .env.example                            # [40 lines] Environment variables
    └── README.md                               # [200 lines] Frontend docs



Now plan a roadmap to develop into golang as backend system and api to create a front end with php.
Feature List

1. Core Email Sending Features
- Bulk Gmail email sending – Send mass emails through multiple Gmail accounts using Google API authentication
- Multi-account management – Load and manage multiple Gmail accounts from file with automatic rotation
- OAuth2 authentication – Secure Google OAuth2 authentication for each Gmail account with token management
- Automatic account rotation – Smart rotation between Gmail accounts based on limits and health status
- Automatic account suspension – Detect and suspend accounts showing spam signs or errors
- Rate limiting & throttling – Configurable requests per second, retry delays, and exponential backoff
- Thread-safe SSL connections – Secure multi-threaded sending with SSL connection pooling
- Parallel worker processing – Multi-threaded email sending (1–4 workers) with queue management

2. Template & Personalization Features
- HTML email template rotation – Rotate through multiple HTML email templates
- Dynamic variable replacement – 50+ placeholders (name, date, invoice numbers, etc.)
- Sender name rotation – Sequential, random, weighted, or time-based strategies
- Subject line rotation – Multiple subject templates with rotation strategies
- Custom field rotation – User-defined custom fields with rotating config values
- Smart name extraction – Extract recipient names from email addresses
- Date formatting – Multiple formats with offset support (e.g., CUSTOM_DATE_+7)
- Dynamic random generators – Random numbers, alphas, alphanumerics (variable length)
- Time-of-day based content – Adjust content for morning/afternoon/evening/night

3. Attachment Features
- HTML to PDF conversion – Convert HTML templates to PDF attachments
- Multi-format attachment support – PDF, JPG, PNG, WebP, HEIC, HEIF
- Attachment template rotation – Rotate multiple attachment templates
- Format rotation – Rotate formats (PDF → JPG → PNG → WebP)
- Attachment caching – Avoid regenerating identical attachments
- Dynamic attachment personalization – Recipient-specific variables
- Conversion backend flexibility – Support WeasyPrint, imgkit, pdfkit

4. Web Dashboard Features
- Flask web interface – Web-based dashboard for campaign management
- User authentication – Login system with session management
- Session management – Isolated campaign sessions
- ZIP file upload – Upload campaign packages (templates + config)
- Real-time monitoring – Live campaign progress tracking
- Template file editor – Built-in HTML/config editor
- File management – Download, edit, manage campaign files
- Campaign persistence – Save and restore campaign state
- Log viewer – View debug, failed, and system logs
- System metrics dashboard – Monitor CPU, memory, disk, network usage

5. Campaign Management Features
- Production campaign manager – Enterprise-grade lifecycle management
- Campaign state tracking – Created / Running / Paused / Completed / Failed
- Progress monitoring – Real-time percentage and ETA calculation
- Automatic cleanup policies – Remove expired sessions and failed campaigns
- Campaign retry logic – Exponential backoff on failures
- Recipient list management – Load and manage recipient lists
- Bulk recipient operations – Delete first/last N, delete before/after specific email
- Statistics tracking – Sent, failed, success rate, throughput

6. Email Deliverability Features
- Feedback-ID header – Gmail FBL identifier for complaint tracking
- List-Unsubscribe headers – RFC-compliant one-click unsubscribe
- MIME message formatting – Proper multipart HTML/text structure
- Message-ID generation – Unique IDs with proper domain formatting
- Sender reputation management – Spam score tracking + auto rotation
- Daily limit enforcement – Per-account daily limits
- Rotation limit enforcement – Emails per account before rotation

7. Configuration & Setup Features
- Comprehensive configuration system – INI-style config (10+ sections, 100+ parameters)
- Default config generation – Auto-create default config file
- Config validation – Validate and merge missing defaults
- Config backup – Auto-backup before saving changes
- Command-line arguments – Override config via CLI
- Multiple interface modes – CLI, TUI, Web Dashboard

8. Logging & Debugging Features
- Multi-level logging – Campaign, debug, failed, success, system, performance logs
- Per-session logging – Isolated logs per campaign session
- Real-time log streaming – Live log view in dashboard
- Debug mode – Verbose logging with detailed traces
- Failed email tracking – Timestamp, reason, account info
- Success tracking – Log successful sends

9. Memory & Performance Features
- Memory-optimized recipient loading – Efficient large list handling
- Garbage collection – Automatic cleanup during long campaigns
- Resource monitoring – CPU, RAM, disk tracking
- Connection pooling – Reuse Gmail API connections
- Batch processing – Configurable chunk processing
- Cache management – Clear/manage attachment cache

10. Error Handling & Recovery Features
- Automatic error recovery – Retry with exponential backoff
- Account health monitoring – Suspend after consecutive failures
- Graceful shutdown – Clean resource cleanup on interruption
- Exception handling – Comprehensive try/catch coverage
- Account cooldown – Cooling period after rotation

11. Security Features
- Secure credential storage – Separate OAuth tokens per account
- Session security – Flask secret key + session management
- Path traversal protection – Prevent directory traversal attacks
- File upload validation – ZIP structure validation
- HMAC signature validation – Secure unsubscribe link generationcl





## TODO
1. The Message-ID generation falls back to using the local machine name, which is an immediate red flag for modern spam filters (Proof: if domain == "" { domain = "localhost" } in headers.go).

The system explicitly flags itself as mass marketing by defaulting to promotional headers, which guarantees routing to Spam or Promotions tabs in Gmail and Outlook (Proof: PrecedenceValue: "bulk" and SetBulkPrecedence() in headers.go).

The default mailer signature broadcasts that the email was sent by a script rather than a human, heavily penalizing your sender reputation (Proof: MailerName: "Email Campaign System v1.0" in headers.go).

The MIME builder encodes HTML and Text bodies using base64 by default instead of quoted-printable, which needlessly bloats the email size by 33% and triggers spam heuristics (Proof: PreferredEncoding: EncodingBase64 in mime.go).

Inline images are incorrectly packaged inside a multipart/mixed container instead of the required multipart/related standard, causing strict email clients to misinterpret the email structure and flag it as suspicious (Proof: mb.inlineFiles are appended under Content-Type: multipart/mixed in mime.go).

Missing Header / MIME components:

The Return-Path uses the primary sender email instead of a dedicated Variable Envelope Return Path (VERP) subdomain, meaning bounce-backs will directly damage your primary domain's reputation (Proof: headers.ReturnPath = fmt.Sprintf("<%s>", hb.accountEmail) in headers.go).

Why attachments can't convert properly:

The backend engine is forcibly applying its own default margins instead of respecting your CSS because 0.4-inch margins are hardcoded into the default configurations for both PDF engines (Proof: MarginTop: 0.4 and --margin-top fmt.Sprintf("%.1fmm", opts.MarginTop*25.4) in both pdf.go and native_converter.go).

The document is not rendering edge-to-edge because the headless Chrome engine is explicitly configured to ignore your CSS @page dimension rules (Proof: PreferCSSPageSize: false in DefaultPDFConfig() in pdf.go).

The entire HTML layout is being automatically scaled down because the native converter explicitly injects a command-line flag to shrink your content to fit inside the forced boundaries (Proof: args = append(args, "--enable-smart-shrinking") in native_converter.go).

Possible Fix: use have johnfercher/maroto render the PDF directly to a bytes.Buffer instead of writing it to the disk, Take that byte buffer and attach it directly to the email payload
1. Nil panic:
```
0x1400059cca0?)\n\t/opt/homebrew/Cellar/go/1.25.0/libexec/src/net/http/server.go:2322 +0x38\nemail-campaign-system/internal/api/middleware.(*LoggingMiddleware).Handler-fm.(*LoggingMiddleware).Handler.func1({0x105243cb0, 0x140003c2870}, 0x140005e6640)\n\t/Users/rahman/Downloads/Ghost-Senderzip-38/backend/internal/api/middleware/logging.go:55 +0x128\nnet/http.HandlerFunc.ServeHTTP(0x14000047948?, {0x105243cb0?, 0x140003c2870?}, 0x1051fee80?)\n\t/opt/homebrew/Cellar/go/1.25.0/libexec/src/net/http/server.go:2322 +0x38\nemail-campaign-system/internal/api/middleware.(*RecoveryMiddleware).Handler-fm.(*RecoveryMiddleware).Handler.func1({0x105243cb0, 0x140003c2870}, 0x140005e6640)\n\t/Users/rahman/Downloads/Ghost-Senderzip-38/backend/internal/api/middleware/recovery.go:54 +0xa8\nnet/http.HandlerFunc.ServeHTTP(0x140005e6500?, {0x105243cb0?, 0x140003c2870?}, 0x14000148b40?)\n\t/opt/homebrew/Cellar/go/1.25.0/libexec/src/net/http/server.go:2322 +0x38\ngithub.com/gorilla/mux.(*Router).ServeHTTP(0x1400048e0c0, {0x105243cb0, 0x140003c2870}, 0x140005e63c0)\n\t/Users/rahman/go/pkg/mod/github.com/gorilla/mux@v1.8.1/mux.go:212 +0x18c\nemail-campaign-system/internal/api.(*Router).ServeHTTP(0x10?, {0x105243cb0?, 0x140003c2870?}, 0x140003c2870?)\n\t/Users/rahman/Downloads/Ghost-Senderzip-38/backend/internal/api/routes.go:254 +0x28\nnet/http.serverHandler.ServeHTTP({0x1052424d8?}, {0x105243cb0?, 0x140003c2870?}, 0x6?)\n\t/opt/homebrew/Cellar/go/1.25.0/libexec/src/net/http/server.go:3340 +0xb0\nnet/http.(*conn).serve(0x14000148b40, {0x105244738, 0x14000258270})\n\t/opt/homebrew/Cellar/go/1.25.0/libexec/src/net/http/server.go:2109 +0x528\ncreated by net/http.(*Server).Serve in goroutine 63\n\t/opt/homebrew/Cellar/go/1.25.0/libexec/src/net/http/server.go:3493 +0x384\n"}
```
2. Campaign Stop if all SMTP Account stops working
3. Telegram Update
4. SMTP Checker