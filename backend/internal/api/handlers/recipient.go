package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"fmt"
	"github.com/gorilla/mux"

	"email-campaign-system/internal/api/websocket"
	"email-campaign-system/internal/models"
	"email-campaign-system/pkg/errors"
	"email-campaign-system/pkg/logger"
	"email-campaign-system/pkg/validator"
)

// RecipientManager interface - implement this in your recipient package
type RecipientManager interface {
	Create(ctx context.Context, req *CreateRecipientReq) (*models.Recipient, error)
	List(ctx context.Context, opts *ListRecipientOptions) ([]*models.Recipient, int, error)
	GetByID(ctx context.Context, id string) (*models.Recipient, error)
	Update(ctx context.Context, id string, req *UpdateRecipientReq) (*models.Recipient, error)
	Delete(ctx context.Context, id string) error
	ImportFromFile(ctx context.Context, req *ImportRecipientRequest) (*ImportRecipientResult, error)
	ValidateEmail(ctx context.Context, email string) (*EmailValidationResult, error)
	BulkDelete(ctx context.Context, ids []string) (int, error)
	DeleteFirst(ctx context.Context, campaignID string, count int) (int, error)
	DeleteLast(ctx context.Context, campaignID string, count int) (int, error)
	DeleteBefore(ctx context.Context, campaignID string, email string) (int, error)
	DeleteAfter(ctx context.Context, campaignID string, email string) (int, error)
	RemoveDuplicates(ctx context.Context, campaignID string) (int, error)
	GetStats(ctx context.Context, campaignID *string) (interface{}, error)
	ExportToFile(ctx context.Context, campaignID *string, format string, filename string) error
}

type RecipientHandler struct {
	recipientManager RecipientManager
	wsHub            *websocket.Hub
	logger           logger.Logger
	validator        *validator.Validator
}

func NewRecipientHandler(
	recipientManager RecipientManager,
	wsHub *websocket.Hub,
	log logger.Logger,
	validator *validator.Validator,
) *RecipientHandler {
	return &RecipientHandler{
		recipientManager: recipientManager,
		wsHub:            wsHub,
		logger:           log,
		validator:        validator,
	}
}

// Request/Response types
type CreateRecipientReq struct {
	Email      string                 `json:"email"`
	FirstName  string                 `json:"first_name"`
	LastName   string                 `json:"last_name"`
	CustomData map[string]interface{} `json:"custom_data"`
	Tags       []string               `json:"tags"`
	CampaignID *string                `json:"campaign_id"`
}

type UpdateRecipientReq struct {
	Email      *string                `json:"email"`
	FirstName  *string                `json:"first_name"`
	LastName   *string                `json:"last_name"`
	CustomData map[string]interface{} `json:"custom_data"`
	Tags       []string               `json:"tags"`
}

type ListRecipientOptions struct {
	CampaignID *string
	Tag        string
	Search     string
	Status     string
	IsValid    *bool
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
}

type ImportRecipientRequest struct {
	Content     []byte
	FileType    string
	CampaignID  *string
	Deduplicate bool
	Validate    bool
}

type ImportRecipientResult struct {
	Total      int
	Successful int
	Failed     int
	Duplicates int
	Invalid    int
	Errors     []string
}

type EmailValidationResult struct {
	IsValid    bool
	Reason     string
	DNSValid   bool
	HasMX      bool
	Disposable bool
}

type CreateRecipientRequest struct {
	Email      string                 `json:"email" validate:"required,email"`
	FirstName  string                 `json:"first_name"`
	LastName   string                 `json:"last_name"`
	CustomData map[string]interface{} `json:"custom_data"`
	Tags       []string               `json:"tags"`
	CampaignID *string                `json:"campaign_id"`
}

type UpdateRecipientRequest struct {
	Email      *string                `json:"email" validate:"omitempty,email"`
	FirstName  *string                `json:"first_name"`
	LastName   *string                `json:"last_name"`
	CustomData map[string]interface{} `json:"custom_data"`
	Tags       []string               `json:"tags"`
}

type RecipientResponse struct {
	ID         string                 `json:"id"`
	Email      string                 `json:"email"`
	FirstName  string                 `json:"first_name"`
	LastName   string                 `json:"last_name"`
	FullName   string                 `json:"full_name"`
	CustomData map[string]interface{} `json:"custom_data"`
	Tags       []string               `json:"tags"`
	IsValid    bool                   `json:"is_valid"`
	IsSent     bool                   `json:"is_sent"`
	SentAt     *time.Time             `json:"sent_at"`
	Status     string                 `json:"status"`
	CampaignID string                 `json:"campaign_id"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

type ImportResponse struct {
	Total      int      `json:"total"`
	Successful int      `json:"successful"`
	Failed     int      `json:"failed"`
	Duplicates int      `json:"duplicates"`
	Invalid    int      `json:"invalid"`
	Errors     []string `json:"errors"`
}

type ValidationResponse struct {
	Email      string `json:"email"`
	IsValid    bool   `json:"is_valid"`
	Reason     string `json:"reason"`
	DNSValid   bool   `json:"dns_valid"`
	HasMX      bool   `json:"has_mx"`
	Disposable bool   `json:"disposable"`
}

type BulkDeleteRequest struct {
	IDs []string `json:"ids" validate:"required,min=1"`
}

func (h *RecipientHandler) CreateRecipient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateRecipientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}
	fmt.Printf("🟢 DEBUG CreateRecipient: email=%s campaign_id=%v\n", req.Email, req.CampaignID)

	createReq := &CreateRecipientReq{
		Email:      req.Email,
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		CustomData: req.CustomData,
		Tags:       req.Tags,
		CampaignID: req.CampaignID,
	}

	rec, err := h.recipientManager.Create(ctx, createReq)
	if err != nil {
		h.logger.Error("Failed to create recipient", logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Recipient created successfully", logger.String("recipient_id", rec.ID), logger.String("email", rec.Email))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "recipient_created",
		Data: h.marshalJSON(rec),
	})

	h.respondJSON(w, http.StatusCreated, h.toRecipientResponse(rec))
}

func (h *RecipientHandler) ListRecipients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	campaignID := query.Get("campaign_id")
	tag := query.Get("tag")
	search := query.Get("search")
	status := query.Get("status")
	valid := query.Get("valid")
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("page_size"))
	sortBy := query.Get("sort_by")
	sortOrder := query.Get("sort_order")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 1000 {
		pageSize = 100
	}
	if sortBy == "" {
		sortBy = "created_at"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}

	var campID *string
	if campaignID != "" {
		campID = &campaignID
	}

	var isValid *bool
	if valid != "" {
		val := valid == "true"
		isValid = &val
	}

	recipients, total, err := h.recipientManager.List(ctx, &ListRecipientOptions{
		CampaignID: campID,
		Tag:        tag,
		Search:     search,
		Status:     status,
		IsValid:    isValid,
		Page:       page,
		PageSize:   pageSize,
		SortBy:     sortBy,
		SortOrder:  sortOrder,
	})

	if err != nil {
		h.logger.Error("Failed to list recipients", logger.Error(err))
		h.respondError(w, err)
		return
	}

	response := make([]RecipientResponse, len(recipients))
	for i, rec := range recipients {
		response[i] = h.toRecipientResponse(rec)
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"recipients":  response,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + pageSize - 1) / pageSize,
	})
}

func (h *RecipientHandler) GetRecipient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid recipient ID"))
		return
	}

	rec, err := h.recipientManager.GetByID(ctx, id)
	if err != nil {
		h.logger.Error("Failed to get recipient", logger.String("recipient_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, h.toRecipientResponse(rec))
}

func (h *RecipientHandler) UpdateRecipient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid recipient ID"))
		return
	}

	var req UpdateRecipientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	updateReq := &UpdateRecipientReq{
		Email:      req.Email,
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		CustomData: req.CustomData,
		Tags:       req.Tags,
	}

	rec, err := h.recipientManager.Update(ctx, id, updateReq)
	if err != nil {
		h.logger.Error("Failed to update recipient", logger.String("recipient_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Recipient updated successfully", logger.String("recipient_id", rec.ID))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "recipient_updated",
		Data: h.marshalJSON(rec),
	})

	h.respondJSON(w, http.StatusOK, h.toRecipientResponse(rec))
}

func (h *RecipientHandler) DeleteRecipient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid recipient ID"))
		return
	}

	if err := h.recipientManager.Delete(ctx, id); err != nil {
		h.logger.Error("Failed to delete recipient", logger.String("recipient_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Recipient deleted successfully", logger.String("recipient_id", id))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "recipient_deleted",
		Data: h.marshalJSON(map[string]interface{}{"id": id}),
	})

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Recipient deleted successfully",
	})
}

func (h *RecipientHandler) ImportRecipients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseMultipartForm(50 << 20); err != nil {
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
	if ext != ".csv" && ext != ".txt" {
		h.respondError(w, errors.BadRequest("Only CSV and TXT files are supported"))
		return
	}

	campaignIDStr := r.FormValue("campaign_id")
	var campaignID *string
	if campaignIDStr != "" {
		campaignID = &campaignIDStr
	}

	deduplicate := r.FormValue("deduplicate") == "true"
	validateEmails := r.FormValue("validate") == "true"

	content, err := io.ReadAll(file)
	if err != nil {
		h.respondError(w, errors.BadRequest("Failed to read file"))
		return
	}

	result, err := h.recipientManager.ImportFromFile(ctx, &ImportRecipientRequest{
		Content:     content,
		FileType:    ext,
		CampaignID:  campaignID,
		Deduplicate: deduplicate,
		Validate:    validateEmails,
	})

	if err != nil {
		h.logger.Error("Failed to import recipients", logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Recipients imported successfully",
		logger.String("filename", header.Filename),
		logger.Int("total", result.Total),
		logger.Int("successful", result.Successful),
		logger.Int("failed", result.Failed),
		logger.Int("duplicates", result.Duplicates))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "recipients_imported",
		Data: h.marshalJSON(result),
	})

	response := ImportResponse{
		Total:      result.Total,
		Successful: result.Successful,
		Failed:     result.Failed,
		Duplicates: result.Duplicates,
		Invalid:    result.Invalid,
		Errors:     result.Errors,
	}

	h.respondJSON(w, http.StatusOK, response)
}

func (h *RecipientHandler) ValidateRecipient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Email string `json:"email" validate:"required,email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	result, err := h.recipientManager.ValidateEmail(ctx, req.Email)
	if err != nil {
		h.logger.Error("Failed to validate email", logger.String("email", req.Email), logger.Error(err))
		h.respondError(w, err)
		return
	}

	response := ValidationResponse{
		Email:      req.Email,
		IsValid:    result.IsValid,
		Reason:     result.Reason,
		DNSValid:   result.DNSValid,
		HasMX:      result.HasMX,
		Disposable: result.Disposable,
	}

	h.respondJSON(w, http.StatusOK, response)
}

func (h *RecipientHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req BulkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	deleted, err := h.recipientManager.BulkDelete(ctx, req.IDs)
	if err != nil {
		h.logger.Error("Failed to bulk delete recipients", logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Recipients bulk deleted", logger.Int("count", deleted))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "recipients_bulk_deleted",
		Data: h.marshalJSON(map[string]interface{}{"count": deleted}),
	})

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Recipients deleted successfully",
		"count":   deleted,
	})
}

// Continue with other methods...
func (h *RecipientHandler) DeleteFirst(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Count      int    `json:"count" validate:"required,min=1"`
		CampaignID string `json:"campaign_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}
	deleted, err := h.recipientManager.DeleteFirst(ctx, req.CampaignID, req.Count)
	if err != nil {
		h.logger.Error("Failed to delete first recipients", logger.Error(err))
		h.respondError(w, err)
		return
	}
	h.logger.Info("First recipients deleted", logger.Int("count", deleted))
	h.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "First recipients deleted successfully", "count": deleted})
}

func (h *RecipientHandler) DeleteLast(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Count      int    `json:"count" validate:"required,min=1"`
		CampaignID string `json:"campaign_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}
	deleted, err := h.recipientManager.DeleteLast(ctx, req.CampaignID, req.Count)
	if err != nil {
		h.logger.Error("Failed to delete last recipients", logger.Error(err))
		h.respondError(w, err)
		return
	}
	h.logger.Info("Last recipients deleted", logger.Int("count", deleted))
	h.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Last recipients deleted successfully", "count": deleted})
}

func (h *RecipientHandler) DeleteBefore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Email      string `json:"email" validate:"required,email"`
		CampaignID string `json:"campaign_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}
	deleted, err := h.recipientManager.DeleteBefore(ctx, req.CampaignID, req.Email)
	if err != nil {
		h.logger.Error("Failed to delete recipients before email", logger.Error(err))
		h.respondError(w, err)
		return
	}
	h.logger.Info("Recipients before email deleted", logger.Int("count", deleted))
	h.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Recipients deleted successfully", "count": deleted})
}

func (h *RecipientHandler) DeleteAfter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Email      string `json:"email" validate:"required,email"`
		CampaignID string `json:"campaign_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}
	deleted, err := h.recipientManager.DeleteAfter(ctx, req.CampaignID, req.Email)
	if err != nil {
		h.logger.Error("Failed to delete recipients after email", logger.Error(err))
		h.respondError(w, err)
		return
	}
	h.logger.Info("Recipients after email deleted", logger.Int("count", deleted))
	h.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Recipients deleted successfully", "count": deleted})
}

func (h *RecipientHandler) RemoveDuplicates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		CampaignID string `json:"campaign_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}
	removed, err := h.recipientManager.RemoveDuplicates(ctx, req.CampaignID)
	if err != nil {
		h.logger.Error("Failed to remove duplicates", logger.Error(err))
		h.respondError(w, err)
		return
	}
	h.logger.Info("Duplicates removed", logger.Int("count", removed))
	h.respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Duplicates removed successfully", "count": removed})
}

func (h *RecipientHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()
	campaignIDStr := query.Get("campaign_id")
	var campaignID *string
	if campaignIDStr != "" {
		campaignID = &campaignIDStr
	}
	stats, err := h.recipientManager.GetStats(ctx, campaignID)
	if err != nil {
		h.logger.Error("Failed to get recipient stats", logger.Error(err))
		h.respondError(w, err)
		return
	}
	h.respondJSON(w, http.StatusOK, stats)
}

func (h *RecipientHandler) ExportRecipients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()
	campaignIDStr := query.Get("campaign_id")
	format := query.Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "txt" && format != "json" {
		h.respondError(w, errors.BadRequest("Invalid export format"))
		return
	}
	var campaignID *string
	if campaignIDStr != "" {
		campaignID = &campaignIDStr
	}
	tempFile, err := os.CreateTemp("", "recipients-*."+format)
	if err != nil {
		h.respondError(w, errors.Internal("Failed to create export file"))
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	if err := h.recipientManager.ExportToFile(ctx, campaignID, format, tempFile.Name()); err != nil {
		h.logger.Error("Failed to export recipients", logger.Error(err))
		h.respondError(w, err)
		return
	}
	content, err := os.ReadFile(tempFile.Name())
	if err != nil {
		h.respondError(w, errors.Internal("Failed to read export file"))
		return
	}
	filename := "recipients." + format
	if campaignID != nil {
		filename = "campaign_" + *campaignID + "_recipients." + format
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func (h *RecipientHandler) toRecipientResponse(rec *models.Recipient) RecipientResponse {
	fullName := strings.TrimSpace(rec.FirstName + " " + rec.LastName)
	customData := rec.GetPersonalizationData()
	return RecipientResponse{
		ID:         rec.ID,
		Email:      rec.Email,
		FirstName:  rec.FirstName,
		LastName:   rec.LastName,
		FullName:   fullName,
		CustomData: customData,
		Tags:       rec.Tags,
		IsValid:    rec.IsValid,
		IsSent:     rec.IsSent(),
		SentAt:     rec.SentAt,
		Status:     string(rec.Status),
		CampaignID: rec.CampaignID,
		CreatedAt:  rec.CreatedAt,
		UpdatedAt:  rec.UpdatedAt,
	}
}

func (h *RecipientHandler) marshalJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}

func (h *RecipientHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *RecipientHandler) respondError(w http.ResponseWriter, err error) {
	var status int
	var message string
	switch e := err.(type) {
	case *errors.Error:
		status = e.StatusCode
		message = e.Message
	default:
		status = http.StatusInternalServerError
		message = "Internal server error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   message,
		"status":  status,
		"success": false,
	})
}
// DeduplicateRecipients removes duplicate recipients (alias for RemoveDuplicates)
func (h *RecipientHandler) DeduplicateRecipients(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    var req struct {
        CampaignID string `json:"campaign_id" validate:"omitempty"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, errors.BadRequest("Invalid request body"))
        return
    }

    if err := h.validator.Validate(req); err != nil {
        h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
        return
    }

    // Use existing RemoveDuplicates method
    removed, err := h.recipientManager.RemoveDuplicates(ctx, req.CampaignID)
    if err != nil {
        h.logger.Error("Failed to remove duplicates", logger.Error(err))
        h.respondError(w, err)
        return
    }

    h.logger.Info("Recipients deduplicated",
        logger.String("campaign_id", req.CampaignID),
        logger.Int("removed", removed),
    )

    h.wsHub.Broadcast(&websocket.Message{
        Type: "recipients_deduplicated",
        Data: h.marshalJSON(map[string]interface{}{
            "campaign_id": req.CampaignID,
            "removed":     removed,
        }),
    })

    h.respondJSON(w, http.StatusOK, map[string]interface{}{
        "message": "Duplicates removed successfully",
        "removed": removed,
    })
}
