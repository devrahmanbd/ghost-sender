package api

import (
        "net/http"
        "os"
        "path/filepath"
        "strings"

        "github.com/gorilla/mux"

        "email-campaign-system/internal/api/handlers"
        "email-campaign-system/internal/api/middleware"
        "email-campaign-system/internal/api/websocket"
        "email-campaign-system/pkg/logger"
)

type Router struct {
        router *mux.Router
        log    logger.Logger
}

func NewRouter(
        log logger.Logger,
        campaignHandler *handlers.CampaignHandler,
        accountHandler *handlers.AccountHandler,
        templateHandler *handlers.TemplateHandler,
        recipientHandler *handlers.RecipientHandler,
        recipientListHandler *handlers.RecipientListHandler,
        proxyHandler *handlers.ProxyHandler,
        metricsHandler *handlers.MetricsHandler,
        configHandler *handlers.ConfigHandler,
        notificationHandler *handlers.NotificationHandler,
        fileHandler *handlers.FileHandler,
        authHandler *handlers.AuthHandler,
        wsHandler *websocket.Handler,
        authMiddleware *middleware.AuthMiddleware,
        rateLimitMiddleware *middleware.RateLimitMiddleware,
        corsMiddleware *middleware.CORSMiddleware,
        loggingMiddleware *middleware.LoggingMiddleware,
        recoveryMiddleware *middleware.RecoveryMiddleware,
        tenantMiddleware *middleware.TenantMiddleware,
) *Router {
        r := mux.NewRouter()

        r.Use(recoveryMiddleware.Handler)
        r.Use(loggingMiddleware.Handler)
        r.Use(corsMiddleware.Handler)

        r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{"status":"healthy"}`))
        }).Methods(http.MethodGet)

        auth := r.PathPrefix("/api/v1/auth").Subrouter()
        auth.HandleFunc("/login", authHandler.Login).Methods(http.MethodPost, http.MethodOptions)

        api := r.PathPrefix("/api/v1").Subrouter()
        api.Use(authMiddleware.Handler)
        api.Use(tenantMiddleware.Handler)
        api.Use(rateLimitMiddleware.Handler)

        api.HandleFunc("/auth/profile", authHandler.GetProfile).Methods(http.MethodGet)
        api.HandleFunc("/auth/license", authHandler.UpdateLicense).Methods(http.MethodPut)

        campaigns := api.PathPrefix("/campaigns").Subrouter()
        campaigns.StrictSlash(true)
        campaigns.HandleFunc("", campaignHandler.ListCampaigns).Methods(http.MethodGet)
        campaigns.HandleFunc("", campaignHandler.CreateCampaign).Methods(http.MethodPost)
        campaigns.HandleFunc("/{id}", campaignHandler.GetCampaign).Methods(http.MethodGet)
        campaigns.HandleFunc("/{id}", campaignHandler.UpdateCampaign).Methods(http.MethodPut)
        campaigns.HandleFunc("/{id}", campaignHandler.DeleteCampaign).Methods(http.MethodDelete)
        campaigns.HandleFunc("/{id}/start", campaignHandler.StartCampaign).Methods(http.MethodPost)
        campaigns.HandleFunc("/{id}/pause", campaignHandler.PauseCampaign).Methods(http.MethodPost)
        campaigns.HandleFunc("/{id}/resume", campaignHandler.ResumeCampaign).Methods(http.MethodPost)
        campaigns.HandleFunc("/{id}/stop", campaignHandler.StopCampaign).Methods(http.MethodPost)
        campaigns.HandleFunc("/{id}/status", campaignHandler.GetCampaignStatus).Methods(http.MethodGet)
        campaigns.HandleFunc("/{id}/stats", campaignHandler.GetCampaignStats).Methods(http.MethodGet)
        campaigns.HandleFunc("/{id}/logs", campaignHandler.GetCampaignLogs).Methods(http.MethodGet)
        campaigns.HandleFunc("/{id}/progress", campaignHandler.GetCampaignProgress).Methods(http.MethodGet)
        campaigns.HandleFunc("/{id}/duplicate", campaignHandler.DuplicateCampaign).Methods(http.MethodPost)
        campaigns.HandleFunc("/{id}/accounts", campaignHandler.GetCampaignAccounts).Methods(http.MethodGet)
        campaigns.HandleFunc("/{id}/recipients", campaignHandler.GetCampaignRecipients).Methods(http.MethodGet)

        // ========== ACCOUNTS ==========
        accounts := api.PathPrefix("/accounts").Subrouter()
        accounts.HandleFunc("", accountHandler.ListAccounts).Methods(http.MethodGet)
        accounts.HandleFunc("", accountHandler.CreateAccount).Methods(http.MethodPost)
        accounts.HandleFunc("/{id}", accountHandler.GetAccount).Methods(http.MethodGet)
        accounts.HandleFunc("/{id}", accountHandler.UpdateAccount).Methods(http.MethodPut)
        accounts.HandleFunc("/{id}", accountHandler.DeleteAccount).Methods(http.MethodDelete)
        accounts.HandleFunc("/{id}/test", accountHandler.TestAccount).Methods(http.MethodPost)
        accounts.HandleFunc("/{id}/health", accountHandler.GetAccountHealth).Methods(http.MethodGet)
        accounts.HandleFunc("/{id}/suspend", accountHandler.SuspendAccount).Methods(http.MethodPost)
        accounts.HandleFunc("/{id}/activate", accountHandler.ActivateAccount).Methods(http.MethodPost)
        accounts.HandleFunc("/{id}/refresh", accountHandler.RefreshOAuth).Methods(http.MethodPost)
        accounts.HandleFunc("/bulk/import", accountHandler.BulkImport).Methods(http.MethodPost)
        accounts.HandleFunc("/bulk/import-file", accountHandler.ImportFromFile).Methods(http.MethodPost)
        accounts.HandleFunc("/suspended", accountHandler.ListSuspendedAccounts).Methods(http.MethodGet)

        // ========== TEMPLATES ==========
        templates := api.PathPrefix("/templates").Subrouter()
        templates.HandleFunc("", templateHandler.ListTemplates).Methods(http.MethodGet)
        templates.HandleFunc("", templateHandler.CreateTemplate).Methods(http.MethodPost)
        templates.HandleFunc("/{id}", templateHandler.GetTemplate).Methods(http.MethodGet)
        templates.HandleFunc("/{id}", templateHandler.UpdateTemplate).Methods(http.MethodPut)
        templates.HandleFunc("/{id}", templateHandler.DeleteTemplate).Methods(http.MethodDelete)
        templates.HandleFunc("/upload", templateHandler.UploadTemplate).Methods(http.MethodPost)

        // ========== RECIPIENT LISTS ==========
        recipientLists := api.PathPrefix("/recipient-lists").Subrouter()
        recipientLists.HandleFunc("", recipientListHandler.ListRecipientLists).Methods(http.MethodGet)
        recipientLists.HandleFunc("", recipientListHandler.CreateRecipientList).Methods(http.MethodPost)
        recipientLists.HandleFunc("/{id}", recipientListHandler.GetRecipientList).Methods(http.MethodGet)
        recipientLists.HandleFunc("/{id}", recipientListHandler.DeleteRecipientList).Methods(http.MethodDelete)
        recipientLists.HandleFunc("/{id}/recipients", recipientListHandler.GetListRecipients).Methods(http.MethodGet)
        recipientLists.HandleFunc("/{id}/recipients", recipientListHandler.AddRecipientToList).Methods(http.MethodPost)
        recipientLists.HandleFunc("/{id}/import", recipientListHandler.ImportToList).Methods(http.MethodPost)

        // ========== RECIPIENTS ==========
        recipients := api.PathPrefix("/recipients").Subrouter()
        recipients.HandleFunc("", recipientHandler.ListRecipients).Methods(http.MethodGet)
        recipients.HandleFunc("", recipientHandler.CreateRecipient).Methods(http.MethodPost)
        recipients.HandleFunc("/{id}", recipientHandler.GetRecipient).Methods(http.MethodGet)
        recipients.HandleFunc("/{id}", recipientHandler.UpdateRecipient).Methods(http.MethodPut)
        recipients.HandleFunc("/{id}", recipientHandler.DeleteRecipient).Methods(http.MethodDelete)
        recipients.HandleFunc("/import", recipientHandler.ImportRecipients).Methods(http.MethodPost)
        recipients.HandleFunc("/validate", recipientHandler.ValidateRecipient).Methods(http.MethodPost)
        recipients.HandleFunc("/deduplicate", recipientHandler.DeduplicateRecipients).Methods(http.MethodPost)
        recipients.HandleFunc("/bulk/delete", recipientHandler.BulkDelete).Methods(http.MethodPost)
        recipients.HandleFunc("/bulk/delete-first", recipientHandler.DeleteFirst).Methods(http.MethodPost)
        recipients.HandleFunc("/bulk/delete-last", recipientHandler.DeleteLast).Methods(http.MethodPost)
        recipients.HandleFunc("/bulk/delete-before", recipientHandler.DeleteBefore).Methods(http.MethodPost)
        recipients.HandleFunc("/bulk/delete-after", recipientHandler.DeleteAfter).Methods(http.MethodPost)
        recipients.HandleFunc("/export", recipientHandler.ExportRecipients).Methods(http.MethodGet)

        // ========== PROXIES ==========
        proxies := api.PathPrefix("/proxies").Subrouter()
        proxies.HandleFunc("", proxyHandler.ListProxies).Methods(http.MethodGet)
        proxies.HandleFunc("", proxyHandler.CreateProxy).Methods(http.MethodPost)
        proxies.HandleFunc("/{id}", proxyHandler.GetProxy).Methods(http.MethodGet)
        proxies.HandleFunc("/{id}", proxyHandler.UpdateProxy).Methods(http.MethodPut)
        proxies.HandleFunc("/{id}", proxyHandler.DeleteProxy).Methods(http.MethodDelete)
        proxies.HandleFunc("/{id}/test", proxyHandler.TestProxy).Methods(http.MethodPost)
        proxies.HandleFunc("/{id}/health", proxyHandler.GetProxyHealth).Methods(http.MethodGet)
        proxies.HandleFunc("/import", proxyHandler.ImportProxies).Methods(http.MethodPost)
        proxies.HandleFunc("/import-text", proxyHandler.ImportProxiesFromText).Methods(http.MethodPost)
        proxies.HandleFunc("/bulk/test", proxyHandler.BulkTest).Methods(http.MethodPost)
        proxies.HandleFunc("/bulk/delete-unhealthy", proxyHandler.BulkDeleteUnhealthy).Methods(http.MethodPost)
        proxies.HandleFunc("/health-check", proxyHandler.CheckHealth).Methods(http.MethodPost)

        // ========== METRICS ==========
        metricsAPI := api.PathPrefix("/metrics").Subrouter()
        metricsAPI.HandleFunc("/system", metricsHandler.GetSystemMetrics).Methods(http.MethodGet)
        metricsAPI.HandleFunc("/dashboard", metricsHandler.GetDashboardMetrics).Methods(http.MethodGet)
        metricsAPI.HandleFunc("/campaigns/{id}", metricsHandler.GetCampaignMetrics).Methods(http.MethodGet)
        metricsAPI.HandleFunc("/accounts", metricsHandler.GetAccountMetrics).Methods(http.MethodGet)
        metricsAPI.HandleFunc("/sending", metricsHandler.GetSendingMetrics).Methods(http.MethodGet)
        metricsAPI.HandleFunc("/timeseries", metricsHandler.GetTimeSeries).Methods(http.MethodGet)

        // ========== CONFIG ==========
        config := api.PathPrefix("/config").Subrouter()
        config.HandleFunc("", configHandler.GetConfig).Methods(http.MethodGet)
        config.HandleFunc("", configHandler.UpdateConfig).Methods(http.MethodPut)
        config.HandleFunc("/validate", configHandler.ValidateConfig).Methods(http.MethodPost)
        config.HandleFunc("/reset", configHandler.ResetToDefaults).Methods(http.MethodPost)
        config.HandleFunc("/backup", configHandler.BackupConfig).Methods(http.MethodPost)
        config.HandleFunc("/restore", configHandler.RestoreConfig).Methods(http.MethodPost)
        config.HandleFunc("/sections/{section}", configHandler.GetConfigSection).Methods(http.MethodGet)
        config.HandleFunc("/sections/{section}", configHandler.UpdateConfigSection).Methods(http.MethodPut)

        // ========== NOTIFICATIONS ==========
        notifications := api.PathPrefix("/notifications").Subrouter()
        notifications.HandleFunc("", notificationHandler.GetConfig).Methods(http.MethodGet)
        notifications.HandleFunc("/telegram", notificationHandler.UpdateTelegramConfig).Methods(http.MethodPut)
        notifications.HandleFunc("/test", notificationHandler.TestNotification).Methods(http.MethodPost)
        notifications.HandleFunc("/telegram/bot-info", notificationHandler.GetBotInfo).Methods(http.MethodGet)
        notifications.HandleFunc("/enable", notificationHandler.EnableNotifications).Methods(http.MethodPost)
        notifications.HandleFunc("/disable", notificationHandler.DisableNotifications).Methods(http.MethodPost)
        notifications.HandleFunc("/preferences", notificationHandler.UpdatePreferences).Methods(http.MethodPut)
        notifications.HandleFunc("/history", notificationHandler.GetHistory).Methods(http.MethodGet)

        // ========== FILES ==========
        files := api.PathPrefix("/files").Subrouter()
        files.HandleFunc("/upload", fileHandler.UploadFile).Methods(http.MethodPost)
        files.HandleFunc("/upload/zip", fileHandler.UploadZIP).Methods(http.MethodPost)
        files.HandleFunc("/download/{category}/{filename}", fileHandler.DownloadFile).Methods(http.MethodGet)
        files.HandleFunc("/list/{category}", fileHandler.ListFiles).Methods(http.MethodGet)
        files.HandleFunc("/delete/{category}/{filename}", fileHandler.DeleteFile).Methods(http.MethodDelete)
        files.HandleFunc("/zip/extract", fileHandler.ExtractZip).Methods(http.MethodPost)
        files.HandleFunc("/zip/create", fileHandler.CreateZip).Methods(http.MethodPost)

        // ========== LOGS ==========
        logs := api.PathPrefix("/logs").Subrouter()
        logs.HandleFunc("", metricsHandler.GetLogs).Methods(http.MethodGet)
        logs.HandleFunc("/campaign/{id}", metricsHandler.GetCampaignLogs).Methods(http.MethodGet)
        logs.HandleFunc("/failed", metricsHandler.GetFailedLogs).Methods(http.MethodGet)
        logs.HandleFunc("/system", metricsHandler.GetSystemLogs).Methods(http.MethodGet)
        logs.HandleFunc("/stream", metricsHandler.StreamLogs).Methods(http.MethodGet)

        // ========== WEBSOCKET ==========
        ws := r.PathPrefix("/ws").Subrouter()
        ws.Use(authMiddleware.Handler)
        ws.HandleFunc("/connect", wsHandler.HandleWebSocket)
        ws.HandleFunc("/stats", wsHandler.HandleStats).Methods(http.MethodGet)

        // ========== STATIC FRONTEND ==========
        frontendDir := filepath.Join("..", "frontend", "public")
        if envDir := os.Getenv("FRONTEND_DIR"); envDir != "" {
                frontendDir = envDir
        }
        spa := &spaHandler{staticDir: frontendDir, indexPath: "index.html"}
        r.PathPrefix("/").Handler(spa)

        return &Router{
                router: r,
                log:    log,
        }
}

type spaHandler struct {
        staticDir string
        indexPath string
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
        path := r.URL.Path
        if path == "/" {
                path = h.indexPath
        } else {
                path = strings.TrimPrefix(path, "/")
        }

        absPath := filepath.Join(h.staticDir, filepath.Clean(path))

        if !strings.HasPrefix(absPath, filepath.Clean(h.staticDir)) {
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
        }

        _, err := os.Stat(absPath)
        if os.IsNotExist(err) {
                http.ServeFile(w, r, filepath.Join(h.staticDir, h.indexPath))
                return
        } else if err != nil {
                http.Error(w, "Internal Server Error", http.StatusInternalServerError)
                return
        }

        http.ServeFile(w, r, absPath)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
        r.router.ServeHTTP(w, req)
}

func (r *Router) GetRouter() *mux.Router {
        return r.router
}
