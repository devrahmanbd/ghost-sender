package handlers

import ( 
        "bytes"
        "encoding/csv"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "os"
        "path/filepath"
        "strconv"
        "strings"
        "time"
        "encoding/base64"
        "github.com/gorilla/mux"

        "email-campaign-system/internal/api/websocket"
        "email-campaign-system/internal/core/account"
        "email-campaign-system/internal/models"
        "email-campaign-system/pkg/crypto"
        "email-campaign-system/pkg/errors"
        "email-campaign-system/pkg/logger"
        "email-campaign-system/pkg/validator"
)

func lf(key string, value interface{}) logger.Field {
        return logger.Field{Key: key, Value: value}
}

type AccountHandler struct {
        accountManager *account.AccountManager
        wsHub          *websocket.Hub
        logger         logger.Logger
        validator      *validator.Validator
        encryptor      *crypto.AES
}

func NewAccountHandler(
        accountManager *account.AccountManager,
        wsHub *websocket.Hub,
        log logger.Logger,
        validator *validator.Validator,
        encryptor *crypto.AES,
) *AccountHandler {
        return &AccountHandler{
                accountManager: accountManager,
                wsHub:          wsHub,
                logger:         log,
                validator:      validator,
                encryptor:      encryptor,
        }
}

type CreateAccountRequest struct {
        Email          string                 `json:"email" validate:"required,email"`
        Provider       string                 `json:"provider" validate:"required,oneof=gmail office365 yahoo outlook hotmail icloud workspace smtp custom"`
        Password       string                 `json:"password" validate:"required_if=Provider smtp"`
        AppPassword    string                 `json:"app_password"`
        OAuthToken     string                 `json:"oauth_token"`
        OAuthTokenFile string                 `json:"oauth_token_file"`
        SenderName     string                 `json:"sender_name"`
        SenderNames    []string               `json:"sender_names"`
        DailyLimit     int                    `json:"daily_limit" validate:"min=0"`
        RotationLimit  int                    `json:"rotation_limit" validate:"min=0"`
        ProxyID        *string                `json:"proxy_id"`
        SMTPHost       string                 `json:"smtp_host"`
        SMTPPort       int                    `json:"smtp_port" validate:"min=0,max=65535"`
        UseSSL         bool                   `json:"use_ssl"`
        UseTLS         bool                   `json:"use_tls"`
        Config         map[string]interface{} `json:"config"`
}

// handler/account.go
type UpdateAccountRequest struct {
    Email         *string                `json:"email"          validate:"omitempty,email"`
    Password      *string                `json:"password"`
    AppPassword   *string                `json:"app_password"`
    SenderName    *string                `json:"sender_name"`
    DailyLimit    *int                   `json:"daily_limit"    validate:"omitempty,min=0"`
    RotationLimit *int                   `json:"rotation_limit" validate:"omitempty,min=0"`
    ProxyID       *string                `json:"proxy_id"`
    SMTPHost      *string                `json:"smtp_host"`
    SMTPPort      *int                   `json:"smtp_port"      validate:"omitempty,min=0,max=65535"`
    UseSSL        *bool                  `json:"use_ssl"`
    UseTLS        *bool                  `json:"use_tls"`
    Config        map[string]interface{} `json:"config"`
}


type AccountResponse struct {
        ID                  string                 `json:"id"`
        Email               string                 `json:"email"`
        Provider            string                 `json:"provider"`
        SenderName          string                 `json:"sender_name"`
        SenderNames         []string               `json:"sender_names"`
        Status              string                 `json:"status"`
        HealthScore         float64                `json:"health_score"`
        IsSuspended         bool                   `json:"is_suspended"`
        SuspensionReason    string                 `json:"suspension_reason"`
        DailyLimit          int                    `json:"daily_limit"`
        RotationLimit       int                    `json:"rotation_limit"`
        SentToday           int64                  `json:"sent_today"`
        SentTotal           int64                  `json:"sent_total"`
        FailedCount         int64                  `json:"failed_count"`
        SuccessCount        int64                  `json:"success_count"`
        ConsecutiveFailures int                    `json:"consecutive_failures"`
        LastUsedAt          *time.Time             `json:"last_used_at"`
        LastErrorAt         *time.Time             `json:"last_error_at"`
        LastError           string                 `json:"last_error"`
        ProxyID             *string                `json:"proxy_id"`
        SMTPHost            string                 `json:"smtp_host"`
        SMTPPort            int                    `json:"smtp_port"`
        UseSSL              bool                   `json:"use_ssl"`
        UseTLS              bool                   `json:"use_tls"`
        HasOAuth            bool                   `json:"has_oauth"`
        OAuthExpiry         *time.Time             `json:"oauth_expiry"`
        CreatedAt           time.Time              `json:"created_at"`
        UpdatedAt           time.Time              `json:"updated_at"`
        Config              map[string]interface{} `json:"config"`
}

type AccountStatsResponse struct {
        ID                  string     `json:"id"`
        Email               string     `json:"email"`
        TotalSent           int        `json:"total_sent"`
        TotalFailed         int        `json:"total_failed"`
        SuccessRate         float64    `json:"success_rate"`
        AvgResponseTime     int        `json:"avg_response_time_ms"`
        SentToday           int        `json:"sent_today"`
        SentThisWeek        int        `json:"sent_this_week"`
        SentThisMonth       int        `json:"sent_this_month"`
        HealthScore         float64    `json:"health_score"`
        ConsecutiveFailures int        `json:"consecutive_failures"`
        LastUsedAt          *time.Time `json:"last_used_at"`
        DailyLimitRemaining int        `json:"daily_limit_remaining"`
        RotationRemaining   int        `json:"rotation_remaining"`
}

type BulkImportRequest struct {
        Accounts []CreateAccountRequest `json:"accounts" validate:"required,min=1"`
}

type AccountBulkImportResponse struct {
        Total      int               `json:"total"`
        Successful int               `json:"successful"`
        Failed     int               `json:"failed"`
        Errors     []string          `json:"errors"`
        Accounts   []AccountResponse `json:"accounts"`
}

func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        var req CreateAccountRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                h.respondError(w, errors.BadRequest("Invalid request body"))
                return
        }

        if req.Provider == "hotmail" || req.Provider == "live" || req.Provider == "msn" {
                req.Provider = "outlook"
        }

        if req.SenderName == "" && len(req.SenderNames) > 0 {
                req.SenderName = req.SenderNames[0]
        }
        if req.SenderName == "" {
                req.SenderName = req.Email
        }

        if err := h.validator.Validate(req); err != nil {
                h.respondError(w, errors.ValidationFailed(err.Error()))
                return
        }

        encryptedPassword := req.Password

        oauthToken, err := h.readOAuthToken(req.OAuthTokenFile, req.OAuthToken)
        if err != nil {
                h.respondError(w, errors.BadRequest("Failed to read OAuth token file"))
                return
        }

        if req.Config == nil {
                req.Config = make(map[string]interface{})
        }
        if len(req.SenderNames) > 0 {
                req.Config["sender_names"] = req.SenderNames
        }

        createReq := account.CreateRequest{
                Email:         req.Email,
                Provider:      req.Provider,
                Password:      encryptedPassword,
                AppPassword:   req.AppPassword,
                OAuthToken:    oauthToken,
                SenderName:    req.SenderName,
                SenderNames:   req.SenderNames,
                DailyLimit:    req.DailyLimit,
                RotationLimit: req.RotationLimit,
                ProxyID:       req.ProxyID,
                SMTPHost:      req.SMTPHost,
                SMTPPort:      req.SMTPPort,
                UseSSL:        req.UseSSL,
                UseTLS:        req.UseTLS,
                Config:        req.Config,
        }

        acc, err := h.accountManager.Create(ctx, createReq)
        if err != nil {
                h.logger.Error("Failed to create account", lf("email", req.Email), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.logger.Info("Account created successfully", lf("account_id", acc.ID), lf("email", acc.Email))
    h.broadcastAccountEvent("account_created", acc)
    h.respondJSON(w, http.StatusCreated, h.toAccountResponse(acc))
}

func (h *AccountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        opts := h.parseListOptions(r)

        accounts, total, err := h.accountManager.List(ctx, opts)
        if err != nil {
                h.logger.Error("Failed to list accounts", lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        response := make([]AccountResponse, len(accounts))
        for i, acc := range accounts {
                response[i] = h.toAccountResponse(*acc)
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "accounts":    response,
                "total":       total,
                "page":        opts.Page,
                "page_size":   opts.PageSize,
                "total_pages": (total + opts.PageSize - 1) / opts.PageSize,
        })
}

func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        acc, err := h.accountManager.GetByID(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get account", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.respondJSON(w, http.StatusOK, h.toAccountResponse(acc))
}

func (h *AccountHandler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    id, err := h.parseAccountID(r)
    if err != nil {
        h.respondError(w, err)
        return
    }

    var req UpdateAccountRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, errors.BadRequest("Invalid request body"))
        return
    }
    if err := h.validator.Validate(req); err != nil {
        h.respondError(w, errors.ValidationFailed(err.Error()))
        return
    }

    // Encrypt password only when it was actually provided
    if req.Password != nil && *req.Password != "" {
        encrypted, err := h.encryptPassword(*req.Password)
        if err != nil {
            h.logger.Error("Failed to encrypt password", lf("error", err.Error()))
            h.respondError(w, errors.Internal("Failed to secure credentials"))
            return
        }
        req.Password = &encrypted // replace plain with encrypted in-place
    }

    // account.UpdateRequest now accepts *string/*int/*bool — direct pass-through
    updateReq := account.UpdateRequest{
        Email:         req.Email,
        Password:      req.Password,
        AppPassword:   req.AppPassword,
        SenderName:    req.SenderName,
        DailyLimit:    req.DailyLimit,
        RotationLimit: req.RotationLimit,
        ProxyID:       req.ProxyID,
        SMTPHost:      req.SMTPHost,
        SMTPPort:      req.SMTPPort,
        UseSSL:        req.UseSSL,
        UseTLS:        req.UseTLS,
        Config:        req.Config,
    }

    acc, err := h.accountManager.Update(ctx, id, updateReq)
    if err != nil {
        h.logger.Error("Failed to update account", lf("account_id", id), lf("error", err.Error()))
        h.respondError(w, err)
        return
    }

    h.logger.Info("Account updated successfully", lf("account_id", acc.ID))
    h.broadcastAccountEvent("account:updated", acc)
    h.respondJSON(w, http.StatusOK, h.toAccountResponse(acc))
}


func (h *AccountHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        if err := h.accountManager.Delete(ctx, id); err != nil {
                h.logger.Error("Failed to delete account", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.logger.Info("Account deleted successfully", lf("account_id", id))
        h.broadcastAccountEvent("account_deleted", map[string]interface{}{"id": id})
        h.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Account deleted successfully"})
}

func (h *AccountHandler) TestAccount(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        h.logger.Info("Testing account connection", lf("account_id", id))

        result, err := h.accountManager.TestConnection(ctx, id)
        if err != nil {
                h.logger.Error("Account connection test failed", lf("account_id", id), lf("error", err.Error()))
                h.respondJSON(w, http.StatusOK, map[string]interface{}{
                        "success":       false,
                        "error":         err.Error(),
                        "response_time": 0,
                })
                return
        }

        h.logger.Info("Account connection test successful", lf("account_id", id), lf("response_time", result.ResponseTime))
        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "success":       true,
                "message":       "Connection successful",
                "response_time": result.ResponseTime,
                "provider":      result.Provider,
                "server_info":   result.ServerInfo,
        })
}

func (h *AccountHandler) SuspendAccount(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        var req struct {
                Reason string `json:"reason" validate:"required"`
        }

        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                h.respondError(w, errors.BadRequest("Invalid request body"))
                return
        }

        if err := h.validator.Validate(req); err != nil {
                h.respondError(w, errors.ValidationFailed(err.Error()))
                return
        }

        acc, err := h.accountManager.Suspend(ctx, id, req.Reason)
        if err != nil {
                h.logger.Error("Failed to suspend account", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.logger.Info("Account suspended", lf("account_id", id), lf("reason", req.Reason))
        h.broadcastAccountEvent("account_suspended", acc)
        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "message": "Account suspended successfully",
                "account": h.toAccountResponse(acc),
        })
}

func (h *AccountHandler) ActivateAccount(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        acc, err := h.accountManager.Activate(ctx, id)
        if err != nil {
                h.logger.Error("Failed to activate account", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.logger.Info("Account activated", lf("account_id", id))
        h.broadcastAccountEvent("account_activated", acc)
        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "message": "Account activated successfully",
                "account": h.toAccountResponse(acc),
        })
}

func (h *AccountHandler) GetAccountStats(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        stats, err := h.accountManager.GetStats(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get account stats", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        response := AccountStatsResponse{
                ID:                  stats.ID,
                Email:               stats.Email,
                TotalSent:           stats.TotalSent,
                TotalFailed:         stats.TotalFailed,
                SuccessRate:         stats.SuccessRate,
                AvgResponseTime:     stats.AvgResponseTime,
                SentToday:           stats.SentToday,
                SentThisWeek:        stats.SentThisWeek,
                SentThisMonth:       stats.SentThisMonth,
                HealthScore:         stats.HealthScore,
                ConsecutiveFailures: stats.ConsecutiveFailures,
                LastUsedAt:          stats.LastUsedAt,
                DailyLimitRemaining: stats.DailyLimitRemaining,
                RotationRemaining:   stats.RotationRemaining,
        }

        h.respondJSON(w, http.StatusOK, response)
}

func (h *AccountHandler) GetAccountHealth(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        health, err := h.accountManager.GetHealth(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get account health", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.respondJSON(w, http.StatusOK, health)
}

func (h *AccountHandler) RefreshOAuth(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        h.logger.Info("Refreshing OAuth token", lf("account_id", id))

        _, err = h.accountManager.RefreshOAuth(ctx, id)
        if err != nil {
                h.logger.Error("Failed to refresh OAuth token", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.logger.Info("OAuth token refreshed", lf("account_id", id))
        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "message": "OAuth token refreshed successfully",
        })
}

func (h *AccountHandler) BulkImport(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        var req BulkImportRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                h.respondError(w, errors.BadRequest("Invalid request body"))
                return
        }

        if err := h.validator.Validate(req); err != nil {
                h.respondError(w, errors.ValidationFailed(err.Error()))
                return
        }

        response := AccountBulkImportResponse{
                Total:    len(req.Accounts),
                Accounts: make([]AccountResponse, 0),
                Errors:   make([]string, 0),
        }

        for i, accReq := range req.Accounts {
                encryptedBytes, err := h.encryptor.Encrypt([]byte(accReq.Password))  
                if err != nil {
                        h.logger.Error("Failed to encrypt password", lf("error", err.Error()))
                        h.respondError(w, errors.Internal("Failed to secure credentials"))
                        return
                }
                encryptedPassword := base64.StdEncoding.EncodeToString(encryptedBytes)

                if err != nil {
                        response.Failed++
                        response.Errors = append(response.Errors, fmt.Sprintf("%d: Failed to encrypt password", i+1))
                        continue
                }

                createReq := account.CreateRequest{
                        Email:         accReq.Email,
                        Provider:      accReq.Provider,
                        Password:      encryptedPassword,
                        AppPassword:   accReq.AppPassword,
                        OAuthToken:    accReq.OAuthToken,
                        SenderName:    accReq.SenderName,
                        SenderNames:   accReq.SenderNames,
                        DailyLimit:    accReq.DailyLimit,
                        RotationLimit: accReq.RotationLimit,
                        ProxyID:       accReq.ProxyID,
                        SMTPHost:      accReq.SMTPHost,
                        SMTPPort:      accReq.SMTPPort,
                        UseSSL:        accReq.UseSSL,
                        UseTLS:        accReq.UseTLS,
                        Config:        accReq.Config,
                }

                acc, err := h.accountManager.Create(ctx, createReq)
                if err != nil {
                        response.Failed++
                        response.Errors = append(response.Errors, accReq.Email+": "+err.Error())
                        continue
                }

                response.Successful++
                response.Accounts = append(response.Accounts, h.toAccountResponse(acc))
        }

        h.logger.Info("Bulk import completed",
                lf("total", response.Total),
                lf("successful", response.Successful),
                lf("failed", response.Failed))

        h.broadcastAccountEvent("accounts_imported", response)
        h.respondJSON(w, http.StatusOK, response)
}

func (h *AccountHandler) ImportFromFile(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        if err := r.ParseMultipartForm(10 << 20); err != nil {
                h.respondError(w, errors.BadRequest("Failed to parse multipart form"))
                return
        }

        file, header, err := r.FormFile("file")
        if err != nil {
                h.respondError(w, errors.BadRequest("No file uploaded"))
                return
        }
        defer file.Close()

        ext := strings.ToLower(filepath.Ext(header.Filename))
        if ext != ".json" && ext != ".csv" {
                h.respondError(w, errors.BadRequest("Only JSON and CSV files are supported"))
                return
        }

        data, err := io.ReadAll(file)
        if err != nil {
                h.respondError(w, errors.BadRequest("Failed to read file"))
                return
        }

        result, err := h.accountManager.ImportFromFile(ctx, data, ext)
        if err != nil {
                h.logger.Error("Failed to import accounts from file", lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.logger.Info("Accounts imported from file",
                lf("total", result.Total),
                lf("successful", result.Successful),
                lf("failed", result.Failed))

        h.broadcastAccountEvent("accounts_imported", result)
        h.respondJSON(w, http.StatusOK, result)
}

func (h *AccountHandler) GetAccountLogs(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        query := r.URL.Query()
        logType := query.Get("type")
        page, _ := strconv.Atoi(query.Get("page"))
        pageSize, _ := strconv.Atoi(query.Get("page_size"))

        if page < 1 {
                page = 1
        }
        if pageSize < 1 || pageSize > 500 {
                pageSize = 100
        }

        logs, total, err := h.accountManager.GetLogs(ctx, id, account.LogOptions{
                Type:     logType,
                Page:     page,
                PageSize: pageSize,
        })
        if err != nil {
                h.logger.Error("Failed to get account logs", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "logs":        logs,
                "total":       total,
                "page":        page,
                "page_size":   pageSize,
                "total_pages": (total + pageSize - 1) / pageSize,
        })
}

func (h *AccountHandler) ResetLimits(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        if err := h.accountManager.ResetLimits(ctx, id); err != nil {
                h.logger.Error("Failed to reset account limits", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.logger.Info("Account limits reset", lf("account_id", id))
        h.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Account limits reset successfully"})
}

func (h *AccountHandler) GetAccountCampaigns(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        id, err := h.parseAccountID(r)
        if err != nil {
                h.respondError(w, err)
                return
        }

        campaigns, err := h.accountManager.GetCampaigns(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get account campaigns", lf("account_id", id), lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        h.respondJSON(w, http.StatusOK, campaigns)
}

func (h *AccountHandler) ListSuspendedAccounts(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        opts := account.ListOptions{
                Status:    string(models.AccountStatusSuspended),
                Page:      1,
                PageSize:  100,
                SortBy:    "updated_at",
                SortOrder: "desc",
        }

        query := r.URL.Query()
        if page := query.Get("page"); page != "" {
                if p, err := strconv.Atoi(page); err == nil && p > 0 {
                        opts.Page = p
                }
        }
        if pageSize := query.Get("page_size"); pageSize != "" {
                if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 && ps <= 100 {
                        opts.PageSize = ps
                }
        }

        accounts, total, err := h.accountManager.List(ctx, &opts)
        if err != nil {
                h.logger.Error("Failed to list suspended accounts", lf("error", err.Error()))
                h.respondError(w, err)
                return
        }

        response := make([]AccountResponse, len(accounts))
        for i, acc := range accounts {
                response[i] = h.toAccountResponse(*acc)
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "accounts":    response,
                "total":       total,
                "page":        opts.Page,
                "page_size":   opts.PageSize,
                "total_pages": (total + opts.PageSize - 1) / opts.PageSize,
        })
}

func (h *AccountHandler) parseAccountID(r *http.Request) (string, error) {
        vars := mux.Vars(r)
        id := vars["id"]
        if id == "" {
                return "", errors.BadRequest("Invalid account ID")
        }
        return id, nil
}

func (h *AccountHandler) parseListOptions(r *http.Request) *account.ListOptions {
        query := r.URL.Query()
        page, _ := strconv.Atoi(query.Get("page"))
        pageSize, _ := strconv.Atoi(query.Get("page_size"))
        sortBy := query.Get("sort_by")
        sortOrder := query.Get("sort_order")

        if page < 1 {
                page = 1
        }
        if pageSize < 1 || pageSize > 100 {
                pageSize = 20
        }
        if sortBy == "" {
                sortBy = "created_at"
        }
        if sortOrder == "" {
                sortOrder = "desc"
        }

        var isSuspended *bool
        if suspended := query.Get("suspended"); suspended != "" {
                val := suspended == "true"
                isSuspended = &val
        }

        return &account.ListOptions{
                Provider:    query.Get("provider"),
                Status:      query.Get("status"),
                IsSuspended: isSuspended,
                Page:        page,
                PageSize:    pageSize,
                SortBy:      sortBy,
                SortOrder:   sortOrder,
        }
}

func (h *AccountHandler) encryptPassword(password string) (string, error) {
    if password == "" {
        return "", nil
    }
    if h.encryptor == nil {
        // encryption disabled or not configured — store plaintext
        h.logger.Warn("encryptor not configured, storing password unencrypted")
        return password, nil
    }
    encrypted, err := h.encryptor.Encrypt([]byte(password))
    if err != nil {
        return "", err
    }
    return string(encrypted), nil
}


func (h *AccountHandler) readOAuthToken(tokenFile, token string) (string, error) {
        if tokenFile != "" {
                cleaned := filepath.Clean(tokenFile)
                if filepath.IsAbs(cleaned) || strings.Contains(cleaned, "..") {
                        return "", fmt.Errorf("invalid token file path")
                }
                tokenData, err := os.ReadFile(cleaned)
                if err != nil {
                        return "", err
                }
                return string(tokenData), nil
        }
        return token, nil
}

func (h *AccountHandler) broadcastAccountEvent(eventType string, data interface{}) {
        jsonData, _ := json.Marshal(data)
        h.wsHub.Broadcast(&websocket.Message{
                Type: websocket.MessageType(eventType),
                Data: jsonData,
        })
}

// handler/account.go — change function signature
func (h *AccountHandler) toAccountResponse(acc models.Account) AccountResponse {
    // body unchanged — acc.Credentials works fine on value
    hasOAuth := acc.Credentials != nil && acc.Credentials.AccessToken != ""
    
    var suspensionReason string
    if acc.SuspensionInfo != nil {
        suspensionReason = acc.SuspensionInfo.SuspensionReason
    }
    
    var oauthExpiry *time.Time
    if acc.Credentials != nil {
        oauthExpiry = acc.Credentials.TokenExpiry
    }
    
    var smtpHost string
    var smtpPort int
    var useSSL, useTLS bool
    if acc.SMTPConfig != nil {
        smtpHost = acc.SMTPConfig.Host
        smtpPort = acc.SMTPConfig.Port
        useSSL = acc.SMTPConfig.UseSSL
        useTLS = acc.SMTPConfig.UseTLS
    }

    var senderNames []string
    if acc.Metadata != nil {
        if sn, ok := acc.Metadata["sender_names"]; ok {
            if snArr, ok := sn.([]interface{}); ok {
                for _, s := range snArr {
                    if str, ok := s.(string); ok {
                        senderNames = append(senderNames, str)
                    }
                }
            }
            if snArr, ok := sn.([]string); ok {
                senderNames = snArr
            }
        }
    }
    if senderNames == nil {
        senderNames = []string{}
    }

    return AccountResponse{
        ID:                  acc.ID,
        Email:               acc.Email,
        Provider:            string(acc.Provider),
        SenderName:          acc.Name,
        SenderNames:         senderNames,
        Status:              string(acc.Status),
        HealthScore:         acc.HealthMetrics.HealthScore,
        IsSuspended:         acc.Status == models.AccountStatusSuspended,
        SuspensionReason:    suspensionReason,
        DailyLimit:          acc.Limits.DailyLimit,
        RotationLimit:       acc.Limits.RotationLimit,
        SentToday:           acc.Limits.SentToday,
        SentTotal:           acc.Stats.TotalSent,
        FailedCount:         acc.Stats.TotalFailed,
        SuccessCount:        acc.Stats.TotalDelivered,
        ConsecutiveFailures: acc.HealthMetrics.ConsecutiveFailures,
        LastUsedAt:          acc.LastUsedAt,
        LastErrorAt:         acc.LastErrorAt,
        LastError:           acc.LastError,
        ProxyID:             nil,
        SMTPHost:            smtpHost,
        SMTPPort:            smtpPort,
        UseSSL:              useSSL,
        UseTLS:              useTLS,
        HasOAuth:            hasOAuth,
        OAuthExpiry:         oauthExpiry,
        CreatedAt:           acc.CreatedAt,
        UpdatedAt:           acc.UpdatedAt,
        Config:              acc.Metadata,
    }
}


func (h *AccountHandler) parseAccountsCSV(content []byte) ([]CreateAccountRequest, error) {
        reader := csv.NewReader(bytes.NewReader(content))
        records, err := reader.ReadAll()
        if err != nil {
                return nil, err
        }

        if len(records) < 2 {
                return nil, fmt.Errorf("CSV must have header and at least one row")
        }

        accounts := make([]CreateAccountRequest, 0, len(records)-1)
        for i, record := range records[1:] {
                if len(record) < 6 {
                        return nil, fmt.Errorf("row %d: insufficient columns (need at least 6)", i+2)
                }

                port, err := strconv.Atoi(strings.TrimSpace(record[5]))
                if err != nil {
                        port = 587
                }

                accounts = append(accounts, CreateAccountRequest{
                        Email:         strings.TrimSpace(record[0]),
                        Password:      strings.TrimSpace(record[1]),
                        Provider:      strings.TrimSpace(record[2]),
                        SenderName:    strings.TrimSpace(record[3]),
                        SMTPHost:      strings.TrimSpace(record[4]),
                        SMTPPort:      port,
                        UseTLS:        true,
                        DailyLimit:    500,
                        RotationLimit: 100,
                })
        }

        return accounts, nil
}
func (h *AccountHandler) respondError(w http.ResponseWriter, err error) {
    // FromStdError: returns *Error as-is if already one,
    // otherwise wraps as Internal (500)
    appErr := errors.FromStdError(err)

    status := appErr.StatusCode
    message := appErr.Message
    if appErr.Details != "" {
        message = appErr.Details
    }

    // Plain errors from AccountManager (fmt.Errorf) arrive as 500 Internal.
    // Reclassify them if they're actually validation/input errors.
    if status == http.StatusInternalServerError {
        rawMsg := err.Error()
        if isValidationError(rawMsg) {
            status = http.StatusBadRequest
            message = rawMsg
        } else {
            // Don't leak internal details to client
            h.logger.Error("internal error", lf("error", rawMsg))
            message = "Internal server error"
        }
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "error":   message,
        "status":  status,
        "success": false,
    })
}

func isValidationError(msg string) bool {
    lower := strings.ToLower(msg)
    for _, kw := range []string{
        "required", "invalid", "validation", "failed",
        "password", "email", "smtp", "credentials",
    } {
        if strings.Contains(lower, kw) {
            return true
        }
    }
    return false
}
func (h *AccountHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if err := json.NewEncoder(w).Encode(data); err != nil {
        h.logger.Error("failed to encode response", lf("error", err.Error()))
    }
}

