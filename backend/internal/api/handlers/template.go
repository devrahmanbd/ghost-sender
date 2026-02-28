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
	"github.com/gorilla/mux"
	"fmt"
	"email-campaign-system/internal/api/websocket"
	"email-campaign-system/internal/models"
	"email-campaign-system/pkg/errors"
	"email-campaign-system/pkg/logger"
	"email-campaign-system/pkg/validator"
)

// TemplateManager interface - implement this in your template package
type TemplateManager interface {
	Create(ctx context.Context, req *CreateTemplateReq) (*models.Template, error)
	List(ctx context.Context, opts *ListTemplateOptions) ([]*models.Template, int, error)
	GetByID(ctx context.Context, id string) (*models.Template, error)
	Update(ctx context.Context, id string, req *UpdateTemplateReq) (*models.Template, error)
	Delete(ctx context.Context, id string) error
	Validate(ctx context.Context, id string) (*TemplateValidationResult, error)
	CheckSpam(ctx context.Context, id string) (*SpamCheckResult, error)
	Preview(ctx context.Context, req *TemplatePreviewRequest) (*TemplatePreviewResult, error)
	ExtractVariables(ctx context.Context, id string) ([]string, error)
	ImportFromDirectory(ctx context.Context, dirPath string) (*ImportResult, error)
	ImportFromZip(ctx context.Context, zipPath string) (*ImportResult, error)
	Duplicate(ctx context.Context, id string, newName string) (*models.Template, error)
	GetCampaigns(ctx context.Context, id string) (interface{}, error)
	GetUsageStats(ctx context.Context, id string) (interface{}, error)
}

type TemplateHandler struct {
	templateManager TemplateManager
	wsHub           *websocket.Hub
	logger          logger.Logger
	validator       *validator.Validator
}

func NewTemplateHandler(
	templateManager TemplateManager,
	wsHub *websocket.Hub,
	log logger.Logger,
	validator *validator.Validator,
) *TemplateHandler {
	return &TemplateHandler{
		templateManager: templateManager,
		wsHub:           wsHub,
		logger:          log,
		validator:       validator,
	}
}

// Request/Response types
type CreateTemplateReq struct {
	Name        string
	Description string
	Type        string
	Content     string
	Subject     string
	Tags        []string
	IsActive    bool
	Config      map[string]interface{}
}

type UpdateTemplateReq struct {
	Name        *string
	Description *string
	Content     *string
	Subject     *string
	Tags        []string
	IsActive    *bool
	Config      map[string]interface{}
}

type ListTemplateOptions struct {
	Type      string
	Tag       string
	Search    string
	IsActive  *bool
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

type TemplateValidationResult struct {
	Valid    bool
	Errors   []string
	Warnings []string
}

type SpamCheckResult struct {
	Score    float64
	Warnings []string
	Issues   []SpamIssueDetail
}

type SpamIssueDetail struct {
	Category    string
	Severity    string
	Description string
	Impact      float64
}

type TemplatePreviewRequest struct {
	TemplateID     string
	RecipientEmail string
	SampleData     map[string]interface{}
}

type TemplatePreviewResult struct {
	Subject     string
	Content     string
	PlainText   string
	Variables   map[string]string
	PreviewHTML string
}

type ImportResult struct {
	Total      int
	Successful int
	Failed     int
	Errors     []string
}

type CreateTemplateRequest struct {
	Name        string                 `json:"name" validate:"required,min=3,max=255"`
	Description string                 `json:"description" validate:"max=1000"`
	Type        string                 `json:"type" validate:"required,oneof=html plain text"`
	Content     string                 `json:"content" validate:"required"`
	Subject     string                 `json:"subject"`
	Tags        []string               `json:"tags"`
	IsActive    bool                   `json:"is_active"`
	Config      map[string]interface{} `json:"config"`
}

type UpdateTemplateRequest struct {
	Name        *string                `json:"name" validate:"omitempty,min=3,max=255"`
	Description *string                `json:"description" validate:"omitempty,max=1000"`
	Content     *string                `json:"content"`
	Subject     *string                `json:"subject"`
	Tags        []string               `json:"tags"`
	IsActive    *bool                  `json:"is_active"`
	Config      map[string]interface{} `json:"config"`
}

type TemplateResponse struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Content     string                 `json:"content"`
	Subject     string                 `json:"subject"`
	Tags        []string               `json:"tags"`
	IsActive    bool                   `json:"is_active"`
	SpamScore   float64                `json:"spam_score"`
	Variables   []string               `json:"variables"`
	Size        int64                  `json:"size_bytes"`
	UsageCount  int                    `json:"usage_count"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Config      map[string]interface{} `json:"config"`
}

type SpamCheckResponse struct {
	Score    float64     `json:"score"`
	MaxScore float64     `json:"max_score"`
	Grade    string      `json:"grade"`
	Passed   bool        `json:"passed"`
	Warnings []string    `json:"warnings"`
	Issues   []SpamIssue `json:"issues"`
}

type SpamIssue struct {
	Category    string  `json:"category"`
	Severity    string  `json:"severity"`
	Description string  `json:"description"`
	Impact      float64 `json:"impact"`
}

type PreviewRequest struct {
	RecipientEmail string                 `json:"recipient_email" validate:"required,email"`
	SampleData     map[string]interface{} `json:"sample_data"`
}

type PreviewResponse struct {
	Subject     string            `json:"subject"`
	Content     string            `json:"content"`
	PlainText   string            `json:"plain_text"`
	Variables   map[string]string `json:"variables"`
	PreviewHTML string            `json:"preview_html"`
}

type UploadTemplateRequest struct {
	Name        string   `json:"name" validate:"required"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type BulkImportResponse struct {
	Total      int                `json:"total"`
	Successful int                `json:"successful"`
	Failed     int                `json:"failed"`
	Errors     []string           `json:"errors"`
	Templates  []TemplateResponse `json:"templates"`
}

func (h *TemplateHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    var req CreateTemplateRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, errors.BadRequest("Invalid request body"))
        return
    }

    // DEBUG
    fmt.Printf("DEBUG CreateTemplate: decoded req: name=%s type=%s content_len=%d is_active=%v\n",
        req.Name, req.Type, len(req.Content), req.IsActive)

    if err := h.validator.Validate(req); err != nil {
        fmt.Printf("DEBUG CreateTemplate: validation failed: %v\n", err)
        h.respondError(w, errors.ValidationError("validation",  []string{err.Error()}))
        return
    }

    createReq := CreateTemplateReq{
        Name:        req.Name,
        Description: req.Description,
        Type:        req.Type,
        Content:     req.Content,
        Subject:     req.Subject,
        Tags:        req.Tags,
        IsActive:    req.IsActive,
        Config:      req.Config,
    }

    fmt.Printf("DEBUG CreateTemplate: calling templateManager.Create\n")

	tpl, err := h.templateManager.Create(ctx, &createReq)
    if err != nil {
        fmt.Printf("DEBUG CreateTemplate: templateManager.Create error: %v\n", err)
        h.logger.Error("Failed to create template", logger.Error(err))
        h.respondError(w, err)
        return
    }

    fmt.Printf("DEBUG CreateTemplate: success, id=%s\n", tpl.ID)

	h.logger.Info("Template created successfully", logger.String("template_id", tpl.ID), logger.String("name", tpl.Name))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "template_created",
		Data: h.marshalJSON(tpl),
	})

	h.respondJSON(w, http.StatusCreated, h.toTemplateResponse(tpl))
}

func (h *TemplateHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	templateType := query.Get("type")
	tag := query.Get("tag")
	search := query.Get("search")
	active := query.Get("active")
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

	var isActive *bool
	if active != "" {
		val := active == "true"
		isActive = &val
	}

	templates, total, err := h.templateManager.List(ctx, &ListTemplateOptions{
		Type:      templateType,
		Tag:       tag,
		Search:    search,
		IsActive:  isActive,
		Page:      page,
		PageSize:  pageSize,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	})

	if err != nil {
		h.logger.Error("Failed to list templates", logger.Error(err))
		h.respondError(w, err)
		return
	}

	response := make([]TemplateResponse, len(templates))
	for i, tpl := range templates {
		response[i] = h.toTemplateResponse(tpl)
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"templates":   response,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + pageSize - 1) / pageSize,
	})
}

func (h *TemplateHandler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	tpl, err := h.templateManager.GetByID(ctx, id)
	if err != nil {
		h.logger.Error("Failed to get template", logger.String("template_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, h.toTemplateResponse(tpl))
}

func (h *TemplateHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	var req UpdateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	updateReq := &UpdateTemplateReq{
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
		Subject:     req.Subject,
		Tags:        req.Tags,
		IsActive:    req.IsActive,
		Config:      req.Config,
	}

	tpl, err := h.templateManager.Update(ctx, id, updateReq)
	if err != nil {
		h.logger.Error("Failed to update template", logger.String("template_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Template updated successfully", logger.String("template_id", tpl.ID))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "template_updated",
		Data: h.marshalJSON(tpl),
	})

	h.respondJSON(w, http.StatusOK, h.toTemplateResponse(tpl))
}

func (h *TemplateHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	if err := h.templateManager.Delete(ctx, id); err != nil {
		h.logger.Error("Failed to delete template", logger.String("template_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Template deleted successfully", logger.String("template_id", id))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "template_deleted",
		Data: h.marshalJSON(map[string]interface{}{"id": id}),
	})

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Template deleted successfully",
	})
}

func (h *TemplateHandler) ValidateTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	h.logger.Info("Validating template", logger.String("template_id", id))

	result, err := h.templateManager.Validate(ctx, id)
	if err != nil {
		h.logger.Error("Template validation failed", logger.String("template_id", id), logger.Error(err))
		h.respondJSON(w, http.StatusOK, map[string]interface{}{
			"valid":  false,
			"errors": []string{err.Error()},
		})
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"valid":    result.Valid,
		"errors":   result.Errors,
		"warnings": result.Warnings,
	})
}

func (h *TemplateHandler) CheckSpam(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	h.logger.Info("Checking template for spam", logger.String("template_id", id))

	result, err := h.templateManager.CheckSpam(ctx, id)
	if err != nil {
		h.logger.Error("Spam check failed", logger.String("template_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	grade := "A"
	if result.Score > 7.0 {
		grade = "F"
	} else if result.Score > 5.0 {
		grade = "D"
	} else if result.Score > 3.0 {
		grade = "C"
	} else if result.Score > 1.0 {
		grade = "B"
	}

	issues := make([]SpamIssue, len(result.Issues))
	for i, issue := range result.Issues {
		issues[i] = SpamIssue{
			Category:    issue.Category,
			Severity:    issue.Severity,
			Description: issue.Description,
			Impact:      issue.Impact,
		}
	}

	response := SpamCheckResponse{
		Score:    result.Score,
		MaxScore: 10.0,
		Grade:    grade,
		Passed:   result.Score < 7.0,
		Warnings: result.Warnings,
		Issues:   issues,
	}

	h.respondJSON(w, http.StatusOK, response)
}

func (h *TemplateHandler) PreviewTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	var req PreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	previewReq := &TemplatePreviewRequest{
		TemplateID:     id,
		RecipientEmail: req.RecipientEmail,
		SampleData:     req.SampleData,
	}

	preview, err := h.templateManager.Preview(ctx, previewReq)
	if err != nil {
		h.logger.Error("Failed to preview template", logger.String("template_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	response := PreviewResponse{
		Subject:     preview.Subject,
		Content:     preview.Content,
		PlainText:   preview.PlainText,
		Variables:   preview.Variables,
		PreviewHTML: preview.PreviewHTML,
	}

	h.respondJSON(w, http.StatusOK, response)
}

func (h *TemplateHandler) GetTemplateVariables(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	variables, err := h.templateManager.ExtractVariables(ctx, id)
	if err != nil {
		h.logger.Error("Failed to extract variables", logger.String("template_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"variables": variables,
		"count":     len(variables),
	})
}

func (h *TemplateHandler) UploadTemplate(w http.ResponseWriter, r *http.Request) {
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

	ext := filepath.Ext(header.Filename)
	if ext != ".html" && ext != ".htm" && ext != ".txt" {
		h.respondError(w, errors.BadRequest("Only HTML and TXT files are supported"))
		return
	}

	content, err := io.ReadAll(file)
	if err != nil {
		h.respondError(w, errors.BadRequest("Failed to read file"))
		return
	}

	name := r.FormValue("name")
	if name == "" {
		name = strings.TrimSuffix(header.Filename, ext)
	}

	description := r.FormValue("description")
	subject := r.FormValue("subject")

	templateType := "html"
	if ext == ".txt" {
		templateType = "text"
	}

	createReq := &CreateTemplateReq{
		Name:        name,
		Description: description,
		Type:        templateType,
		Content:     string(content),
		Subject:     subject,
		IsActive:    true,
	}

	tpl, err := h.templateManager.Create(ctx, createReq)
	if err != nil {
		h.logger.Error("Failed to create template from upload", logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Template uploaded successfully", logger.String("template_id", tpl.ID), logger.String("filename", header.Filename))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "template_created",
		Data: h.marshalJSON(tpl),
	})

	h.respondJSON(w, http.StatusCreated, h.toTemplateResponse(tpl))
}

func (h *TemplateHandler) ImportFromDirectory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		DirectoryPath string `json:"directory_path" validate:"required"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	if !filepath.IsAbs(req.DirectoryPath) {
		h.respondError(w, errors.BadRequest("Directory path must be absolute"))
		return
	}

	if _, err := os.Stat(req.DirectoryPath); os.IsNotExist(err) {
		h.respondError(w, errors.BadRequest("Directory does not exist"))
		return
	}

	result, err := h.templateManager.ImportFromDirectory(ctx, req.DirectoryPath)
	if err != nil {
		h.logger.Error("Failed to import templates", logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Templates imported from directory",
		logger.String("directory", req.DirectoryPath),
		logger.Int("total", result.Total),
		logger.Int("successful", result.Successful),
		logger.Int("failed", result.Failed))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "templates_imported",
		Data: h.marshalJSON(result),
	})

	h.respondJSON(w, http.StatusOK, result)
}

func (h *TemplateHandler) ImportFromZip(w http.ResponseWriter, r *http.Request) {
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

	if filepath.Ext(header.Filename) != ".zip" {
		h.respondError(w, errors.BadRequest("Only ZIP files are supported"))
		return
	}

	tempFile, err := os.CreateTemp("", "templates-*.zip")
	if err != nil {
		h.respondError(w, errors.Internal("Failed to create temporary file"))
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, file); err != nil {
		h.respondError(w, errors.Internal("Failed to save uploaded file"))
		return
	}

	result, err := h.templateManager.ImportFromZip(ctx, tempFile.Name())
	if err != nil {
		h.logger.Error("Failed to import templates from ZIP", logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Templates imported from ZIP",
		logger.String("filename", header.Filename),
		logger.Int("total", result.Total),
		logger.Int("successful", result.Successful),
		logger.Int("failed", result.Failed))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "templates_imported",
		Data: h.marshalJSON(result),
	})

	h.respondJSON(w, http.StatusOK, result)
}

func (h *TemplateHandler) DuplicateTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	var req struct {
		Name string `json:"name" validate:"required,min=3,max=255"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	newTpl, err := h.templateManager.Duplicate(ctx, id, req.Name)
	if err != nil {
		h.logger.Error("Failed to duplicate template", logger.String("template_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Template duplicated successfully", logger.String("original_id", id), logger.String("new_id", newTpl.ID))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "template_created",
		Data: h.marshalJSON(newTpl),
	})

	h.respondJSON(w, http.StatusCreated, h.toTemplateResponse(newTpl))
}

func (h *TemplateHandler) GetTemplateCampaigns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	campaigns, err := h.templateManager.GetCampaigns(ctx, id)
	if err != nil {
		h.logger.Error("Failed to get template campaigns", logger.String("template_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, campaigns)
}

func (h *TemplateHandler) GetTemplateUsageStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid template ID"))
		return
	}

	stats, err := h.templateManager.GetUsageStats(ctx, id)
	if err != nil {
		h.logger.Error("Failed to get template usage stats", logger.String("template_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, stats)
}

func (h *TemplateHandler) toTemplateResponse(tpl *models.Template) TemplateResponse {
	// Get content from template fields directly
	content := tpl.HTMLContent
	if content == "" {
		content = tpl.PlainTextContent
	}
	
	// Get subject - use first subject or empty string
	subject := ""
	if len(tpl.Subjects) > 0 {
		subject = tpl.Subjects[0]
	}
	
	// Convert Variables ([]TemplateVariable) to []string
	variableNames := make([]string, len(tpl.Variables))
	for i, v := range tpl.Variables {
		variableNames[i] = v.Name
	}
	
	return TemplateResponse{
		ID:          tpl.ID,                  // Already string
		Name:        tpl.Name,
		Description: tpl.Description,
		Type:        string(tpl.Type),        // Convert TemplateType to string
		Content:     content,
		Subject:     subject,
		Tags:        tpl.Tags,
		IsActive:    tpl.IsActive(),          // Call the function
		SpamScore:   tpl.SpamScore,
		Variables:   variableNames,            // Converted to []string
		Size:        int64(len(content)),
		UsageCount:  int(tpl.Stats.TimesUsed), // Get from Stats
		CreatedAt:   tpl.CreatedAt,
		UpdatedAt:   tpl.UpdatedAt,
		Config:      tpl.Metadata,             // Use Metadata as Config
	}
}



func (h *TemplateHandler) marshalJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}

func (h *TemplateHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *TemplateHandler) respondError(w http.ResponseWriter, err error) {
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
