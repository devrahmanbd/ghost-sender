package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"email-campaign-system/internal/api/websocket"
	"email-campaign-system/internal/core/campaign"
	"email-campaign-system/internal/models"
	"email-campaign-system/pkg/errors"
	"email-campaign-system/pkg/logger"
	"email-campaign-system/pkg/validator"
)

type CampaignHandler struct {
	campaignManager *campaign.Manager
	wsHub           *websocket.Hub
	logger          logger.Logger
	validator       *validator.Validator
}

func NewCampaignHandler(
	campaignManager *campaign.Manager,
	wsHub *websocket.Hub,
	logger logger.Logger,
	validator *validator.Validator,
) *CampaignHandler {
	return &CampaignHandler{
		campaignManager: campaignManager,
		wsHub:           wsHub,
		logger:          logger,
		validator:       validator,
	}
}

type CreateCampaignRequest struct {
	Name              string                 `json:"name" validate:"required,min=3,max=255"`
	Description       string                 `json:"description" validate:"max=1000"`
	TemplateDir       string                 `json:"template_dir" validate:"required"`
	RecipientFile     string                 `json:"recipient_file" validate:"required"`
	SubjectLines      []string               `json:"subject_lines" validate:"required,min=1"`
	SenderNames       []string               `json:"sender_names" validate:"required,min=1"`
	WorkerCount       int                    `json:"worker_count" validate:"required,min=1,max=20"`
	RateLimit         int                    `json:"rate_limit" validate:"required,min=1,max=100"`
	DailyLimit        int                    `json:"daily_limit" validate:"min=0"`
	RotationLimit     int                    `json:"rotation_limit" validate:"min=0"`
	AccountIDs        []string                 `json:"account_ids" validate:"required,min=1"`
	TemplateIDs       []string                 `json:"template_ids" validate:"required,min=1"`
	ProxyEnabled      bool                   `json:"proxy_enabled"`
	AttachmentEnabled bool                   `json:"attachment_enabled"`
	TrackingEnabled   bool                   `json:"tracking_enabled"`
	ScheduledAt       *time.Time             `json:"scheduled_at"`
	Config            map[string]interface{} `json:"config"`
}

type UpdateCampaignRequest struct {
	Name              *string                `json:"name" validate:"omitempty,min=3,max=255"`
	Description       *string                `json:"description" validate:"omitempty,max=1000"`
	WorkerCount       *int                   `json:"worker_count" validate:"omitempty,min=1,max=20"`
	RateLimit         *int                   `json:"rate_limit" validate:"omitempty,min=1,max=100"`
	DailyLimit        *int                   `json:"daily_limit" validate:"omitempty,min=0"`
	RotationLimit     *int                   `json:"rotation_limit" validate:"omitempty,min=0"`
	ProxyEnabled      *bool                  `json:"proxy_enabled"`
	AttachmentEnabled *bool                  `json:"attachment_enabled"`
	TrackingEnabled   *bool                  `json:"tracking_enabled"`
	ScheduledAt       *time.Time             `json:"scheduled_at"`
	Config            map[string]interface{} `json:"config"`
}

type CampaignResponse struct {
	ID                uint                   `json:"id"`
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
	fmt.Printf("DEBUG req after decode: %+v\n", req) 
	if err := h.validator.Validate(req); err != nil {
		fmt.Printf("DEBUG Campaign Validation Error: %v\n", err)
		h.respondError(w, errors.ValidationFailed(err.Error()))
		return
	}
	camp := &models.Campaign{
		Name:        req.Name,
		Description: req.Description,
		Status:      models.CampaignStatusCreated,
		TemplateIDs: req.TemplateIDs,  
		AccountIDs:  req.AccountIDs,
    	TotalRecipients:   0, 
    Config: models.CampaignRuntimeConfig{
        WorkerCount:       req.WorkerCount,
        EnableProxy:       req.ProxyEnabled,
        EnableAttachments: req.AttachmentEnabled,
        EnableTracking:    req.TrackingEnabled,
        Metadata: map[string]interface{}{   // ADD THIS
            "template_dir":   req.TemplateDir,
            "recipient_file": req.RecipientFile,
            "subject_lines":  req.SubjectLines,
            "sender_names":   req.SenderNames,
        },
      },
	}

	if req.ScheduledAt != nil {
		camp.Schedule.ScheduledStartTime = req.ScheduledAt
	}

	if err := h.campaignManager.CreateCampaign(ctx, camp); err != nil {
		fmt.Printf("DEBUG CreateCampaign error: %v\n", err)  
		h.logger.Error("Failed to create campaign", logger.Field{Key: "error", Value: err})
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
	_ = r.Context()
	vars := mux.Vars(r)

	id := vars["id"]

	camp, err := h.campaignManager.GetCampaign(id)
	if err != nil {
		h.logger.Error("Failed to get campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err})
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, h.toCampaignResponse(camp))
}

func (h *CampaignHandler) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	vars := mux.Vars(r)

	id := vars["id"]

	var req UpdateCampaignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}
	fmt.Printf("DEBUG req after decode: %+v\n", req) 
	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationFailed(err.Error()))
		return
	}

	camp, err := h.campaignManager.GetCampaign(id)
	if err != nil {
		h.logger.Error("Failed to get campaign for update", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err})
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
	if req.AttachmentEnabled != nil {
		camp.Config.EnableAttachments = *req.AttachmentEnabled
	}
	if req.TrackingEnabled != nil {
		camp.Config.EnableTracking = *req.TrackingEnabled
	}

	camp.UpdatedAt = time.Now()

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
		h.logger.Error("Failed to delete campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err})
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
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]

	if err := h.campaignManager.StartCampaign(ctx, id); err != nil {
		h.logger.Error("Failed to start campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err})
		h.respondError(w, err)
		return
	}

	camp, _ := h.campaignManager.GetCampaign(id)

	h.logger.Info("Campaign started successfully", logger.Field{Key: "campaign_id", Value: id})

	campJSON, _ := json.Marshal(camp)
	h.wsHub.Broadcast(&websocket.Message{
		Type: "campaign_started",
		Data: json.RawMessage(campJSON),
	})

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Campaign started successfully",
		"campaign": h.toCampaignResponse(camp),
	})
}

func (h *CampaignHandler) PauseCampaign(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]

	if err := h.campaignManager.PauseCampaign(ctx, id); err != nil {
		h.logger.Error("Failed to pause campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err})
		h.respondError(w, err)
		return
	}

	camp, _ := h.campaignManager.GetCampaign(id)

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
		h.logger.Error("Failed to resume campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err})
		h.respondError(w, err)
		return
	}

	camp, _ := h.campaignManager.GetCampaign(id)

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
		h.logger.Error("Failed to stop campaign", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err})
		h.respondError(w, err)
		return
	}

	camp, _ := h.campaignManager.GetCampaign(id)

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
	_ = r.Context()
	vars := mux.Vars(r)

	id := vars["id"]

	camp, err := h.campaignManager.GetCampaign(id)
	if err != nil {
		h.logger.Error("Failed to get campaign status", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err})
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
	_ = r.Context()
	vars := mux.Vars(r)

	id := vars["id"]

	stats, err := h.campaignManager.GetCampaignStats(id)
	if err != nil {
		h.logger.Error("Failed to get campaign stats", logger.Field{Key: "campaign_id", Value: id}, logger.Field{Key: "error", Value: err})
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, stats)
}

func (h *CampaignHandler) GetCampaignLogs(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	vars := mux.Vars(r)

	_ = vars["id"]

	query := r.URL.Query()
	_ = query.Get("type")
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("page_size"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 100
	}

	h.respondError(w, errors.BadRequest("Campaign logs feature not implemented yet"))
}

func (h *CampaignHandler) DuplicateCampaign(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	vars := mux.Vars(r)

	_ = vars["id"]

	var req struct {
		Name string `json:"name" validate:"required,min=3,max=255"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}
	fmt.Printf("DEBUG req after decode: %+v\n", req) 
	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationFailed(err.Error()))
		return
	}

	h.respondError(w, errors.BadRequest("Campaign duplication feature not implemented yet"))
}

func (h *CampaignHandler) GetCampaignProgress(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	vars := mux.Vars(r)

	_ = vars["id"]

	h.respondError(w, errors.BadRequest("Campaign progress feature not implemented yet"))
}

func (h *CampaignHandler) GetCampaignAccounts(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	vars := mux.Vars(r)

	_ = vars["id"]

	h.respondError(w, errors.BadRequest("Campaign accounts feature not implemented yet"))
}

func (h *CampaignHandler) GetCampaignRecipients(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	vars := mux.Vars(r)

	_ = vars["id"]

	query := r.URL.Query()
	_ = query.Get("status")
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("page_size"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 100
	}

	h.respondError(w, errors.BadRequest("Campaign recipients feature not implemented yet"))
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

	var idNum uint
	fmt.Sscanf(camp.ID, "camp:%d", &idNum)

	configMap := map[string]interface{}{
		"workercount":            camp.Config.WorkerCount,
		"batchsize":              camp.Config.BatchSize,
		"enableproxy":            camp.Config.EnableProxy,
		"enableattachments":      camp.Config.EnableAttachments,
		"enabletracking":         camp.Config.EnableTracking,
		"enableaccountrotation":  camp.Config.EnableAccountRotation,
		"enabletemplaterotation": camp.Config.EnableTemplateRotation,
	}

	pendingCount := int(camp.TotalRecipients - sentCount - failedCount)

	return CampaignResponse{
		ID:                idNum,
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

func (h CampaignHandler) respondError(w http.ResponseWriter, err error) {
	var status int
	var message string
	switch e := err.(type) {
	case *errors.AppError:
		status = e.StatusCode
		message = e.Message
	default:
		status = http.StatusInternalServerError
		// Log the real error server-side; never leak internals to the client
		fmt.Printf("DEBUG respondError: internal error: %v\n", err)
		h.logger.Error("campaign handler internal error", logger.Field{Key: "error", Value: err.Error()})
		message = "Internal server error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": message, "status": status, "success": false})
}
