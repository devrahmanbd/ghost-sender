package handlers

import (
        "encoding/json"
        "fmt"
        "net/http"
        "strconv"
        "strings"
        "time"
        stderrors "errors"
        "github.com/gorilla/mux"

        "email-campaign-system/internal/api/websocket"
        "email-campaign-system/internal/config"
        "email-campaign-system/internal/core/campaign"
        "email-campaign-system/internal/models"
        "email-campaign-system/internal/storage/repository"
        "email-campaign-system/pkg/errors"
        "email-campaign-system/pkg/logger"
        "email-campaign-system/pkg/validator"
)

type CampaignHandler struct {
        campaignManager *campaign.Manager
        wsHub           *websocket.Hub
        logger          logger.Logger
        validator       *validator.Validator
        logRepo         *repository.LogRepository
        accountRepo     *repository.AccountRepository
        recipientRepo   *repository.RecipientRepository
        // cfg is the live application config; maxConcurrent is read from
        // cfg.Security.MaxConcurrentCampaigns on every StartCampaign call so
        // that changes made via the UpdateLicense API take effect immediately.
        cfg           *config.AppConfig
        maxConcurrent int // fallback when cfg is nil
}

func NewCampaignHandler(
        campaignManager *campaign.Manager,
        wsHub *websocket.Hub,
        logger logger.Logger,
        validator *validator.Validator,
        opts ...CampaignHandlerOption,
) *CampaignHandler {
        h := &CampaignHandler{
                campaignManager: campaignManager,
                wsHub:           wsHub,
                logger:          logger,
                validator:       validator,
        }
        for _, opt := range opts {
                opt(h)
        }
        return h
}

type CampaignHandlerOption func(*CampaignHandler)

func WithLogRepo(repo *repository.LogRepository) CampaignHandlerOption {
        return func(h *CampaignHandler) { h.logRepo = repo }
}

func WithAccountRepo(repo *repository.AccountRepository) CampaignHandlerOption {
        return func(h *CampaignHandler) { h.accountRepo = repo }
}

func WithRecipientRepo(repo *repository.RecipientRepository) CampaignHandlerOption {
        return func(h *CampaignHandler) { h.recipientRepo = repo }
}

func WithMaxConcurrentCampaigns(max int) CampaignHandlerOption {
        return func(h *CampaignHandler) { h.maxConcurrent = max }
}

// WithConfig wires the live application config into the handler.  When set,
// StartCampaign always reads the current Security.MaxConcurrentCampaigns
// value so changes via UpdateLicense take effect immediately.
func WithConfig(cfg *config.AppConfig) CampaignHandlerOption {
        return func(h *CampaignHandler) { h.cfg = cfg }
}

type CreateCampaignRequest struct {
        Name              string                 `json:"name" validate:"required,min=3,max=255"`
        Description       string                 `json:"description" validate:"max=1000"`
        TemplateDir       string                 `json:"template_dir"`
        RecipientFile     string                 `json:"recipient_file"`
        RecipientListID   string                 `json:"recipient_list_id"`
        SubjectLines      []string               `json:"subject_lines"`
        SenderNames       []string               `json:"sender_names"`
        CustomFields      map[string][]string    `json:"custom_fields"`
        WorkerCount       int                    `json:"worker_count" validate:"required,min=1,max=20"`
        RateLimit         int                    `json:"rate_limit" validate:"required,min=1,max=100"`
        DailyLimit        int                    `json:"daily_limit" validate:"min=0"`
        RotationLimit     int                    `json:"rotation_limit" validate:"min=0"`
        AccountIDs        []string                 `json:"account_ids" validate:"required,min=1"`
        TemplateIDs       []string                 `json:"template_ids"`
        ProxyEnabled      bool                   `json:"proxy_enabled"`
        SmartSending      bool                   `json:"smart_sending"`
        AttachmentEnabled     bool                   `json:"attachment_enabled"`
        AttachmentTemplateIDs []string               `json:"attachment_template_ids"`
        AttachmentFormat      string                 `json:"attachment_format"`
        TrackingEnabled       bool                   `json:"tracking_enabled"`
        ScheduledAt           *time.Time             `json:"scheduled_at"`
        Config                map[string]interface{} `json:"config"`
}

type UpdateCampaignRequest struct {
        Name              *string                `json:"name" validate:"omitempty,min=3,max=255"`
        Description       *string                `json:"description" validate:"omitempty,max=1000"`
        WorkerCount       *int                   `json:"worker_count" validate:"omitempty,min=1,max=20"`
        RateLimit         *int                   `json:"rate_limit" validate:"omitempty,min=1,max=100"`
        DailyLimit        *int                   `json:"daily_limit" validate:"omitempty,min=0"`
        RotationLimit     *int                   `json:"rotation_limit" validate:"omitempty,min=0"`
        ProxyEnabled      *bool                  `json:"proxy_enabled"`
        SmartSending      *bool                  `json:"smart_sending"`
        AttachmentEnabled *bool                  `json:"attachment_enabled"`
        TrackingEnabled   *bool                  `json:"tracking_enabled"`
        ScheduledAt       *time.Time             `json:"scheduled_at"`
        Config            map[string]interface{} `json:"config"`
}

type CampaignResponse struct {
        ID                string                   `json:"id"`
        Name              string                 `json:"name"`
        Description       string                 `json:"description"`
        Status            string                 `json:"status"`
        TotalRecipients   int                    `json:"total_recipients"`
        SentCount         int                    `json:"sent_count"`
        FailedCount       int                    `json:"failed_count"`
        PendingCount      int                    `json:"pending_count"`
        Progress          float64                `json:"progress"`
        SuccessRate       float64                `json:"success_rate"`
        WorkerCount       int                    `json:"worker_count"`
        RateLimit         int                    `json:"rate_limit"`
        DailyLimit        int                    `json:"daily_limit"`
        RotationLimit     int                    `json:"rotation_limit"`
        ProxyEnabled      bool                   `json:"proxy_enabled"`
        AttachmentEnabled bool                   `json:"attachment_enabled"`
        TrackingEnabled   bool                   `json:"tracking_enabled"`
        ScheduledAt       *time.Time             `json:"scheduled_at"`
        CreatedAt         time.Time              `json:"created_at"`
        UpdatedAt         time.Time              `json:"updated_at"`
        StartedAt         *time.Time             `json:"started_at"`
        CompletedAt       *time.Time             `json:"completed_at"`
        EstimatedDuration *int64                 `json:"estimated_duration_seconds"`
        Config            map[string]interface{} `json:"config"`
}

type CampaignStatsResponse struct {
        ID              uint       `json:"id"`
        Name            string     `json:"name"`
        Status          string     `json:"status"`
        TotalRecipients int        `json:"total_recipients"`
        SentCount       int        `json:"sent_count"`
        FailedCount     int        `json:"failed_count"`
        PendingCount    int        `json:"pending_count"`
        Progress        float64    `json:"progress"`
        SuccessRate     float64    `json:"success_rate"`
        AvgSpeed        int        `json:"avg_speed_per_minute"`
        CurrentSpeed    int        `json:"current_speed_per_minute"`
        EstimatedETA    *int64     `json:"estimated_eta_seconds"`
        Throughput      float64    `json:"throughput"`
        ErrorRate       float64    `json:"error_rate"`
        AccountsUsed    int        `json:"accounts_used"`
        ProxiesUsed     int        `json:"proxies_used"`
        Duration        *int64     `json:"duration_seconds"`
        StartedAt       *time.Time `json:"started_at"`
        CompletedAt     *time.Time `json:"completed_at"`
}

func (h *CampaignHandler) CreateCampaign(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        var req CreateCampaignRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                h.respondError(w, errors.BadRequest("Invalid request body"))
                return
        }
        if err := h.validator.Validate(req); err != nil {
                h.respondError(w, errors.ValidationFailed(err.Error()))
                return
        }
        metadata := make(map[string]interface{})
        if req.TemplateDir != "" {
                metadata["template_dir"] = req.TemplateDir
        }
        if req.RecipientFile != "" {
                metadata["recipient_file"] = req.RecipientFile
        }
        if len(req.SubjectLines) > 0 {
                metadata["subject_lines"] = req.SubjectLines
        }
        if len(req.SenderNames) > 0 {
                metadata["sender_names"] = req.SenderNames
        }
        if len(req.CustomFields) > 0 {
                metadata["custom_fields"] = req.CustomFields
        }
        if len(req.AttachmentTemplateIDs) > 0 {
                metadata["attachment_template_ids"] = req.AttachmentTemplateIDs
                if req.AttachmentFormat != "" {
                        metadata["attachment_format"] = req.AttachmentFormat
                }
        }

        camp := &models.Campaign{
                Name:             req.Name,
                Description:      req.Description,
                Status:           models.CampaignStatusCreated,
                TemplateIDs:      req.TemplateIDs,
                AccountIDs:       req.AccountIDs,
                RecipientGroupID: req.RecipientListID,
                TotalRecipients:  0,
                Config: models.CampaignRuntimeConfig{
                        WorkerCount:       req.WorkerCount,
                        EnableProxy:       req.ProxyEnabled,
                        SmartSending:      req.SmartSending,
                        EnableAttachments: req.AttachmentEnabled || len(req.AttachmentTemplateIDs) > 0,
                        EnableTracking:    req.TrackingEnabled,
                        Metadata:          metadata,
                },
        }
        if req.ScheduledAt != nil {
                camp.Schedule = &models.CampaignSchedule{
                        ScheduledStartTime: req.ScheduledAt,
                }
        }


        if err := h.campaignManager.CreateCampaign(ctx, camp); err != nil {
                h.logger.Error("Failed to create campaign", logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        h.logger.Info("Campaign created successfully", logger.Field{Key: "campaign_id", Value: camp.ID}, logger.Field{Key: "name", Value: camp.Name})

        campJSON, _ := json.Marshal(camp)
        h.wsHub.Broadcast(&websocket.Message{
                Type: "campaign_created",
                Data: json.RawMessage(campJSON),
        })

        h.respondJSON(w, http.StatusCreated, h.toCampaignResponse(camp))
}

func (h *CampaignHandler) ListCampaigns(w http.ResponseWriter, r *http.Request) {
        _ = r.Context()

        query := r.URL.Query()
        _ = query.Get("status")
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

        campaigns := h.campaignManager.ListCampaigns()
        total := len(campaigns)

        response := make([]CampaignResponse, len(campaigns))
        for i, camp := range campaigns {
                response[i] = h.toCampaignResponse(camp)
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "campaigns":   response,
                "total":       total,
                "page":        page,
                "page_size":   pageSize,
                "total_pages": (total + pageSize - 1) / pageSize,
        })
}
func (h *CampaignHandler) GetCampaign(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        camp, err := h.campaignManager.GetCampaign(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        h.respondJSON(w, http.StatusOK, h.toCampaignResponse(camp))
}


func (h *CampaignHandler) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        var req UpdateCampaignRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                h.respondError(w, errors.BadRequest("Invalid request body"))
                return
        }
        if err := h.validator.Validate(req); err != nil {
                h.respondError(w, errors.ValidationFailed(err.Error()))
                return
        }

        camp, err := h.campaignManager.GetCampaign(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get campaign for update", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        if req.Name != nil {
                camp.Name = *req.Name
        }
        if req.Description != nil {
                camp.Description = *req.Description
        }
        if req.WorkerCount != nil {
                camp.Config.WorkerCount = *req.WorkerCount
        }
        if req.ProxyEnabled != nil {
                camp.Config.EnableProxy = *req.ProxyEnabled
        }
        if req.SmartSending != nil {
                camp.Config.SmartSending = *req.SmartSending
        }
        if req.AttachmentEnabled != nil {
                camp.Config.EnableAttachments = *req.AttachmentEnabled
        }
        if req.TrackingEnabled != nil {
                camp.Config.EnableTracking = *req.TrackingEnabled
        }

        camp.UpdatedAt = time.Now()
        if err := h.campaignManager.UpdateCampaign(ctx, camp); err != nil {
                h.logger.Error("Failed to update campaign",
                        logger.Field{Key: "campaign_id", Value: id},
                        logger.Field{Key: "error", Value: err.Error()},
                )
                h.respondError(w, err)
                return
        }
        h.logger.Info("Campaign updated successfully", logger.Field{Key: "campaign_id", Value: camp.ID})

        campJSON, _ := json.Marshal(camp)
        h.wsHub.Broadcast(&websocket.Message{
                Type: "campaign_updated",
                Data: json.RawMessage(campJSON),
        })

        h.respondJSON(w, http.StatusOK, h.toCampaignResponse(camp))
}


func (h *CampaignHandler) DeleteCampaign(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)

        id := vars["id"]

        if err := h.campaignManager.DeleteCampaign(ctx, id); err != nil {
                h.logger.Error("Failed to delete campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        h.logger.Info("Campaign deleted successfully", logger.Field{Key: "campaign_id", Value: id})

        dataJSON, _ := json.Marshal(map[string]interface{}{"id": id})
        h.wsHub.Broadcast(&websocket.Message{
                Type: "campaign_deleted",
                Data: json.RawMessage(dataJSON),
        })

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "message": "Campaign deleted successfully",
        })
}

func (h *CampaignHandler) StartCampaign(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id := vars["id"]

    // Resolve the effective concurrent limit.  The live config pointer takes
    // priority so any update via the UpdateLicense API is reflected instantly.
    // Fall back to the static maxConcurrent field if cfg is not wired.
    // -1 = all blocked, 0 = unlimited, >0 = cap at N.
    effectiveLimit := h.maxConcurrent
    if h.cfg != nil {
        effectiveLimit = h.cfg.Security.MaxConcurrentCampaigns
    }

    if effectiveLimit == -1 {
        h.respondError(w, errors.BadRequest("Campaign starting is disabled. Set max_concurrent_campaigns to 0 (unlimited) or a positive number to enable."))
        return
    }
    if effectiveLimit > 0 {
        running := 0
        for _, c := range h.campaignManager.ListCampaigns() {
            if c.Status == models.CampaignStatusRunning {
                running++
            }
        }
        if running >= effectiveLimit {
            h.respondError(w, errors.BadRequest(fmt.Sprintf(
                "Concurrent campaign limit reached (%d/%d active). Stop a running campaign before starting a new one.",
                running, effectiveLimit,
            )))
            return
        }
    }

    if err := h.campaignManager.StartCampaign(r.Context(), id); err != nil {
        h.logger.Error("Failed to start campaign",
            logger.Field{Key: "campaign_id", Value: id},
            logger.Field{Key: "error", Value: err.Error()},
        )
        h.respondError(w, err)
        return
    }

    h.respondJSON(w, http.StatusOK, map[string]interface{}{
        "message":     "campaign started",
        "campaign_id": id,
    })
}


func (h *CampaignHandler) PauseCampaign(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        if err := h.campaignManager.PauseCampaign(ctx, id); err != nil {
                h.logger.Error("Failed to pause campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        camp, _ := h.campaignManager.GetCampaign(ctx, id)

        h.logger.Info("Campaign paused successfully", logger.Field{Key: "campaign_id", Value: id})

        campJSON, _ := json.Marshal(camp)
        h.wsHub.Broadcast(&websocket.Message{
                Type: "campaign_paused",
                Data: json.RawMessage(campJSON),
        })

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "message":  "Campaign paused successfully",
                "campaign": h.toCampaignResponse(camp),
        })
}
func (h *CampaignHandler) ResumeCampaign(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        if err := h.campaignManager.ResumeCampaign(ctx, id); err != nil {
                h.logger.Error("Failed to resume campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        camp, _ := h.campaignManager.GetCampaign(ctx, id)

        h.logger.Info("Campaign resumed successfully", logger.Field{Key: "campaign_id", Value: id})

        campJSON, _ := json.Marshal(camp)
        h.wsHub.Broadcast(&websocket.Message{
                Type: "campaign_resumed",
                Data: json.RawMessage(campJSON),
        })

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "message":  "Campaign resumed successfully",
                "campaign": h.toCampaignResponse(camp),
        })
}

func (h *CampaignHandler) StopCampaign(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        if err := h.campaignManager.StopCampaign(ctx, id); err != nil {
                h.logger.Error("Failed to stop campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        camp, _ := h.campaignManager.GetCampaign(ctx, id)

        h.logger.Info("Campaign stopped successfully", logger.Field{Key: "campaign_id", Value: id})

        campJSON, _ := json.Marshal(camp)
        h.wsHub.Broadcast(&websocket.Message{
                Type: "campaign_stopped",
                Data: json.RawMessage(campJSON),
        })

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "message":  "Campaign stopped successfully",
                "campaign": h.toCampaignResponse(camp),
        })
}


func (h *CampaignHandler) GetCampaignStatus(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        camp, err := h.campaignManager.GetCampaign(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get campaign status", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "id":     camp.ID,
                "name":   camp.Name,
                "status": string(camp.Status),
        })
}


func (h *CampaignHandler) GetCampaignStats(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        stats, err := h.campaignManager.GetCampaignStats(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get campaign stats", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        h.respondJSON(w, http.StatusOK, stats)
}

func (h *CampaignHandler) GetCampaignLogs(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

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

        if h.logRepo == nil {
                h.respondJSON(w, http.StatusOK, map[string]interface{}{
                        "logs":        []interface{}{},
                        "total":       0,
                        "page":        page,
                        "page_size":   pageSize,
                        "total_pages": 0,
                        "campaign_id": id,
                })
                return
        }

        filter := &repository.LogFilter{
                CampaignIDs: []string{id},
                Limit:       pageSize,
                Offset:      (page - 1) * pageSize,
                SortBy:      "time",
                SortOrder:   "desc",
        }
        if logType != "" {
                filter.Categories = []repository.LogCategory{repository.LogCategory(logType)}
        }

        entries, total, err := h.logRepo.List(ctx, filter)
        if err != nil {
                h.logger.Error("Failed to get campaign logs",
                        logger.Field{Key: "campaign_id", Value: id},
                        logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        totalPages := int(total+int64(pageSize)-1) / pageSize

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "logs":        entries,
                "total":       total,
                "page":        page,
                "page_size":   pageSize,
                "total_pages": totalPages,
                "campaign_id": id,
        })
}

func (h *CampaignHandler) DuplicateCampaign(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        var req struct {
                Name string `json:"name" validate:"required,min=3,max=255"`
        }

        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                h.respondError(w, errors.BadRequest("Invalid request body"))
                return
        }
        if err := h.validator.Validate(req); err != nil {
                h.respondError(w, errors.ValidationFailed(err.Error()))
                return
        }

        original, err := h.campaignManager.GetCampaign(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get campaign for duplication",
                        logger.Field{Key: "campaign_id", Value: id},
                        logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        newCamp := &models.Campaign{
                Name:              req.Name,
                Description:       original.Description,
                Type:              original.Type,
                Status:            models.CampaignStatusCreated,
                Priority:          original.Priority,
                Tags:              original.Tags,
                AccountIDs:        original.AccountIDs,
                TemplateIDs:       original.TemplateIDs,
                ProxyIDs:          original.ProxyIDs,
                TotalRecipients:   0,
                Config:            original.Config,
                RotationConfig:    original.RotationConfig,
                CampaignRateLimitSettings:    original.CampaignRateLimitSettings,
                CampaignRetrySettings:        original.CampaignRetrySettings,
                CampaignNotificationSettings: original.CampaignNotificationSettings,
                Schedule:          original.Schedule,
        }

        if err := h.campaignManager.CreateCampaign(ctx, newCamp); err != nil {
                h.logger.Error("Failed to duplicate campaign",
                        logger.Field{Key: "original_id", Value: id},
                        logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        h.logger.Info("Campaign duplicated successfully",
                logger.Field{Key: "original_id", Value: id},
                logger.Field{Key: "new_id", Value: newCamp.ID})

        h.respondJSON(w, http.StatusCreated, h.toCampaignResponse(newCamp))
}

func (h *CampaignHandler) GetCampaignProgress(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        camp, err := h.campaignManager.GetCampaign(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get campaign progress",
                        logger.Field{Key: "campaign_id", Value: id},
                        logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        stats, _ := h.campaignManager.GetCampaignStats(ctx, id)

        sentCount := camp.Stats.TotalSent
        failedCount := camp.Stats.TotalFailed
        totalRecipients := camp.TotalRecipients

        if stats != nil {
                sentCount = stats.Sent
                failedCount = stats.Failed
                totalRecipients = stats.TotalRecipients
        }

        processed := sentCount + failedCount
        remaining := totalRecipients - processed
        if remaining < 0 {
                remaining = 0
        }

        progressPct := float64(0)
        if totalRecipients > 0 {
                progressPct = float64(processed) / float64(totalRecipients) * 100.0
        }

        var estimatedTimeRemaining int64
        if camp.StartedAt != nil && processed > 0 && camp.Status == models.CampaignStatusRunning {
                elapsed := time.Since(*camp.StartedAt).Seconds()
                rate := float64(processed) / elapsed
                if rate > 0 {
                        estimatedTimeRemaining = int64(float64(remaining) / rate)
                }
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "campaign_id":                  id,
                "status":                       string(camp.Status),
                "total_recipients":             totalRecipients,
                "processed_recipients":         processed,
                "remaining_recipients":         remaining,
                "sent_count":                   sentCount,
                "failed_count":                 failedCount,
                "progress_percentage":          progressPct,
                "estimated_time_remaining_sec": estimatedTimeRemaining,
                "started_at":                   camp.StartedAt,
                "updated_at":                   camp.UpdatedAt,
        })
}

func (h *CampaignHandler) GetCampaignAccounts(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        camp, err := h.campaignManager.GetCampaign(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get campaign accounts",
                        logger.Field{Key: "campaign_id", Value: id},
                        logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        if h.accountRepo == nil || len(camp.AccountIDs) == 0 {
                h.respondJSON(w, http.StatusOK, map[string]interface{}{
                        "accounts":    []interface{}{},
                        "total":       len(camp.AccountIDs),
                        "campaign_id": id,
                })
                return
        }

        accounts, _, err := h.accountRepo.List(ctx, &repository.AccountFilter{
                IDs: camp.AccountIDs,
        })
        if err != nil {
                h.logger.Error("Failed to list campaign accounts",
                        logger.Field{Key: "campaign_id", Value: id},
                        logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        type accountSummary struct {
                ID          string  `json:"id"`
                Name        string  `json:"name"`
                Email       string  `json:"email"`
                Provider    string  `json:"provider"`
                Status      string  `json:"status"`
                HealthScore float64 `json:"health_score"`
                SentToday   int     `json:"sent_today"`
                DailyLimit  int     `json:"daily_limit"`
                IsActive    bool    `json:"is_active"`
                IsSuspended bool    `json:"is_suspended"`
        }

        summaries := make([]accountSummary, 0, len(accounts))
        for _, a := range accounts {
                summaries = append(summaries, accountSummary{
                        ID:          a.ID,
                        Name:        a.Name,
                        Email:       a.Email,
                        Provider:    a.Provider,
                        Status:      a.Status,
                        HealthScore: a.HealthScore,
                        SentToday:   a.SentToday,
                        DailyLimit:  a.DailyLimit,
                        IsActive:    a.IsActive,
                        IsSuspended: a.IsSuspended,
                })
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "accounts":    summaries,
                "total":       len(summaries),
                "campaign_id": id,
        })
}

func (h *CampaignHandler) GetCampaignRecipients(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        id := vars["id"]

        query := r.URL.Query()
        status := query.Get("status")
        page, _ := strconv.Atoi(query.Get("page"))
        pageSize, _ := strconv.Atoi(query.Get("page_size"))

        if page < 1 {
                page = 1
        }
        if pageSize < 1 || pageSize > 500 {
                pageSize = 100
        }

        camp, err := h.campaignManager.GetCampaign(ctx, id)
        if err != nil {
                h.logger.Error("Failed to get campaign for recipients",
                        logger.Field{Key: "campaign_id", Value: id},
                        logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        if h.recipientRepo == nil || camp.RecipientGroupID == "" {
                h.respondJSON(w, http.StatusOK, map[string]interface{}{
                        "recipients":  []interface{}{},
                        "total":       0,
                        "page":        page,
                        "page_size":   pageSize,
                        "total_pages": 0,
                        "campaign_id": id,
                })
                return
        }

        filter := &repository.RecipientFilter{
                ListIDs:   []string{camp.RecipientGroupID},
                Limit:     pageSize,
                Offset:    (page - 1) * pageSize,
                SortBy:    "created_at",
                SortOrder: "desc",
        }
        if status != "" {
                filter.Status = []string{status}
        }

        recipients, total, err := h.recipientRepo.List(ctx, filter)
        if err != nil {
                h.logger.Error("Failed to list campaign recipients",
                        logger.Field{Key: "campaign_id", Value: id},
                        logger.Field{Key: "error", Value: err.Error()})
                h.respondError(w, err)
                return
        }

        totalPages := (total + pageSize - 1) / pageSize

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "recipients":  recipients,
                "total":       total,
                "page":        page,
                "page_size":   pageSize,
                "total_pages": totalPages,
                "campaign_id": id,
        })
}

func (h *CampaignHandler) toCampaignResponse(camp *models.Campaign) CampaignResponse {
        progress := float64(0)
        sentCount := camp.Stats.TotalSent
        failedCount := camp.Stats.TotalFailed

        if camp.TotalRecipients > 0 {
                progress = (float64(sentCount) / float64(camp.TotalRecipients)) * 100
        }

        successRate := float64(0)
        if sentCount > 0 {
                successRate = (float64(sentCount-failedCount) / float64(sentCount)) * 100
        }

        var estimatedDuration int64
        if camp.StartedAt != nil && camp.CompletedAt == nil && camp.Status == models.CampaignStatusRunning {
                elapsed := time.Since(*camp.StartedAt).Seconds()
                if sentCount > 0 {
                        totalEstimated := (elapsed / float64(sentCount)) * float64(camp.TotalRecipients)
                        duration := int64(totalEstimated)
                        estimatedDuration = duration
                }
        }

        configMap := map[string]interface{}{
                "workercount":            camp.Config.WorkerCount,
                "batchsize":              camp.Config.BatchSize,
                "enableproxy":            camp.Config.EnableProxy,
                "smartsending":           camp.Config.SmartSending,
                "enableattachments":      camp.Config.EnableAttachments,
                "enabletracking":         camp.Config.EnableTracking,
                "enableaccountrotation":  camp.Config.EnableAccountRotation,
                "enabletemplaterotation": camp.Config.EnableTemplateRotation,
        }

        pendingCount := int(camp.TotalRecipients - sentCount - failedCount)

        return CampaignResponse{
                ID:                camp.ID,
                Name:              camp.Name,
                Description:       camp.Description,
                Status:            string(camp.Status),
                TotalRecipients:   int(camp.TotalRecipients),
                SentCount:         int(sentCount),
                FailedCount:       int(failedCount),
                PendingCount:      pendingCount,
                Progress:          progress,
                SuccessRate:       successRate,
                WorkerCount:       camp.Config.WorkerCount,
                RateLimit:         0,
                DailyLimit:        0,
                RotationLimit:     0,
                ProxyEnabled:      camp.Config.EnableProxy,
                AttachmentEnabled: camp.Config.EnableAttachments,
                TrackingEnabled:   camp.Config.EnableTracking,
                ScheduledAt: func() *time.Time {
                        if camp.Schedule != nil {
                                return camp.Schedule.ScheduledStartTime
                        }
                        return nil
                }(),

                CreatedAt:         camp.CreatedAt,
                UpdatedAt:         camp.UpdatedAt,
                StartedAt:         camp.StartedAt,
                CompletedAt:       camp.CompletedAt,
                EstimatedDuration: &estimatedDuration,
                Config:            configMap,
        }
}

func (h *CampaignHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(status)
        json.NewEncoder(w).Encode(data)
}

func (h *CampaignHandler) respondError(w http.ResponseWriter, err error) {
        var status int
        var message string

        switch e := err.(type) {
        case *errors.Error:
                status = e.StatusCode
                message = e.Message
        default:
                switch {
                case stderrors.Is(err, campaign.ErrCampaignNotFound):
                        status = http.StatusNotFound
                        message = err.Error()
                case stderrors.Is(err, campaign.ErrCampaignAlreadyExists),
                        stderrors.Is(err, campaign.ErrCampaignAlreadyRunning):
                        status = http.StatusConflict
                        message = err.Error()
                case stderrors.Is(err, campaign.ErrCampaignNotRunning),
                        stderrors.Is(err, campaign.ErrInvalidCampaignState),
                        stderrors.Is(err, campaign.ErrCampaignCompleted):
                        status = http.StatusUnprocessableEntity
                        message = err.Error()
                default:
                        errMsg := err.Error()
                        if strings.Contains(errMsg, "no recipient list assigned") ||
                                strings.Contains(errMsg, "no accounts") ||
                                strings.Contains(errMsg, "no templates") ||
                                strings.Contains(errMsg, "maximum concurrent") {
                                status = http.StatusBadRequest
                                message = errMsg
                        } else {
                                h.logger.Error("campaign handler internal error", logger.Field{Key: "error", Value: err.Error()})
                                status = http.StatusInternalServerError
                                message = "Internal server error"
                        }
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

