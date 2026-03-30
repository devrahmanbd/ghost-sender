package handlers

import (
        "context"
        "encoding/json"
        "io"
        "net/http"
        "path/filepath"
        "strconv"
        "strings"
        "time"
        "github.com/gorilla/mux"
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

type CreateTemplateReq struct {
        Name            string
        Description     string
        Type            string
        Content         string
        Subject         string
        Subjects        []string
        Tags            []string
        IsActive        bool
        Config          map[string]interface{}
        CustomVariables map[string][]string
}

type UpdateTemplateReq struct {
        Name            *string
        Description     *string
        Content         *string
        Subject         *string
        Subjects        []string
        Tags            []string
        IsActive        *bool
        Config          map[string]interface{}
        CustomVariables map[string][]string
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
        Name              string                 `json:"name" validate:"required,min=3,max=255"`
        Description       string                 `json:"description" validate:"max=1000"`
        Type              string                 `json:"type" validate:"required,oneof=html plain text"`
        Content           string                 `json:"content" validate:"required"`
        Subject           string                 `json:"subject"`
        Subjects          []string               `json:"subjects"`
        Tags              []string               `json:"tags"`
        IsActive          bool                   `json:"is_active"`
        Config            map[string]interface{} `json:"config"`
        CustomVariables   map[string][]string    `json:"custom_variables"`
}

type UpdateTemplateRequest struct {
        Name            *string                `json:"name" validate:"omitempty,min=3,max=255"`
        Description     *string                `json:"description" validate:"omitempty,max=1000"`
        Content         *string                `json:"content"`
        Subject         *string                `json:"subject"`
        Subjects        []string               `json:"subjects"`
        Tags            []string               `json:"tags"`
        IsActive        *bool                  `json:"is_active"`
        Config          map[string]interface{} `json:"config"`
        CustomVariables map[string][]string    `json:"custom_variables"`
}

type TemplateResponse struct {
        ID              string                 `json:"id"`
        Name            string                 `json:"name"`
        Description     string                 `json:"description"`
        Type            string                 `json:"type"`
        Content         string                 `json:"content"`
        Subject         string                 `json:"subject"`
        Subjects        []string               `json:"subjects"`
        SenderNames     []string               `json:"sender_names"`
        Tags            []string               `json:"tags"`
        IsActive        bool                   `json:"is_active"`
        SpamScore       float64                `json:"spam_score"`
        Variables       []string               `json:"variables"`
        Size            int64                  `json:"size_bytes"`
        UsageCount      int                    `json:"usage_count"`
        CreatedAt       time.Time              `json:"created_at"`
        UpdatedAt       time.Time              `json:"updated_at"`
        Config          map[string]interface{} `json:"config"`
        CustomVariables map[string][]string    `json:"custom_variables"`
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

    if err := h.validator.Validate(req); err != nil {
        h.respondError(w, errors.ValidationError("validation",  []string{err.Error()}))
        return
    }

    createReq := CreateTemplateReq{
        Name:            req.Name,
        Description:     req.Description,
        Type:            req.Type,
        Content:         req.Content,
        Subject:         req.Subject,
        Subjects:        req.Subjects,
        Tags:            req.Tags,
        IsActive:        req.IsActive,
        Config:          req.Config,
        CustomVariables: req.CustomVariables,
    }

        tpl, err := h.templateManager.Create(ctx, &createReq)
    if err != nil {
        h.logger.Error("Failed to create template", logger.Error(err))
        h.respondError(w, err)
        return
    }

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
                Name:            req.Name,
                Description:     req.Description,
                Content:         req.Content,
                Subject:         req.Subject,
                Subjects:        req.Subjects,
                Tags:            req.Tags,
                IsActive:        req.IsActive,
                Config:          req.Config,
                CustomVariables: req.CustomVariables,
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
                ID:              tpl.ID,
                Name:            tpl.Name,
                Description:     tpl.Description,
                Type:            string(tpl.Type),
                Content:         content,
                Subject:         subject,
                Subjects:        tpl.Subjects,
                SenderNames:     tpl.SenderNames,
                Tags:            tpl.Tags,
                IsActive:        tpl.IsActive(),
                SpamScore:       tpl.SpamScore,
                Variables:       variableNames,
                Size:            int64(len(content)),
                UsageCount:      int(tpl.Stats.TimesUsed),
                CreatedAt:       tpl.CreatedAt,
                UpdatedAt:       tpl.UpdatedAt,
                Config:          tpl.Metadata,
                CustomVariables: tpl.CustomVariables,
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
