package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	coretemplate "email-campaign-system/internal/core/template"
	"email-campaign-system/internal/models"
)

type templateManagerAdapter struct {
	core *coretemplate.TemplateManager
}

func NewTemplateManagerAdapter(mgr *coretemplate.TemplateManager) TemplateManager {
	return &templateManagerAdapter{core: mgr}
}

func (a *templateManagerAdapter) Create(ctx context.Context, req *CreateTemplateReq) (*models.Template, error) {
    fmt.Printf("DEBUG templateManagerAdapter.Create: name=%s content_len=%d\n", req.Name, len(req.Content))

    result, err := a.core.Create(ctx, &coretemplate.CreateTemplateReq{
        Name:        req.Name,
        Description: req.Description,
        Type:        req.Type,
        Content:     req.Content,
        Subject:     req.Subject,
        Tags:        req.Tags,
        IsActive:    req.IsActive,
        Config:      req.Config,
    })

    fmt.Printf("DEBUG templateManagerAdapter.Create: result=%v err=%v\n", result, err)
    return result, err
}


// Interface: List(ctx, *ListTemplateOptions) ([]*models.Template, int, error)
// Core:      List(ctx, *ListTemplateOptions) ([]*models.Template, int, error) — direct pass-through
func (a *templateManagerAdapter) List(ctx context.Context, opts *ListTemplateOptions) ([]*models.Template, int, error) {
	coreOpts := &coretemplate.ListTemplateOptions{}
	if opts != nil {
		coreOpts = &coretemplate.ListTemplateOptions{
			Type:      opts.Type,
			Tag:       opts.Tag,
			Search:    opts.Search,
			IsActive:  opts.IsActive,
			Page:      opts.Page,
			PageSize:  opts.PageSize,
			SortBy:    opts.SortBy,
			SortOrder: opts.SortOrder,
		}
	}
	return a.core.List(ctx, coreOpts)
}

// Interface: GetByID(ctx, id) (*models.Template, error)
// Core:      GetByID(ctx, id) (*models.Template, error) — direct pass-through
func (a *templateManagerAdapter) GetByID(ctx context.Context, id string) (*models.Template, error) {
	return a.core.GetByID(ctx, id)
}

// Interface: Update(ctx, id, *UpdateTemplateReq) (*models.Template, error)
// Core has no Update(id, req) — fetch → patch → UpdateTemplate(*models.Template) → re-fetch.
// UpdateTemplateReq fields are *string / *bool — nil means "leave unchanged".
func (a *templateManagerAdapter) Update(ctx context.Context, id string, req *UpdateTemplateReq) (*models.Template, error) {
	tpl, err := a.core.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		tpl.Name = *req.Name
	}
	if req.Description != nil {
		tpl.Description = *req.Description
	}
	if req.Content != nil {
		if tpl.Type == models.TemplateTypePlainText {
			tpl.PlainTextContent = *req.Content
		} else {
			tpl.HTMLContent = *req.Content
		}
	}
	if req.Subject != nil {
		if len(tpl.Subjects) > 0 {
			tpl.Subjects[0] = *req.Subject
		} else {
			tpl.Subjects = []string{*req.Subject}
		}
	}
	if req.Tags != nil {
		tpl.Tags = req.Tags
	}
	if req.Config != nil {
		tpl.Metadata = req.Config
	}
	if req.IsActive != nil {
		if *req.IsActive {
			tpl.Status = models.TemplateStatusActive
		} else {
			tpl.Status = models.TemplateStatusInactive
		}
	}
	if err := a.core.UpdateTemplate(ctx, tpl); err != nil {
		return nil, err
	}
	return a.core.GetByID(ctx, id)
}

func (a *templateManagerAdapter) Delete(ctx context.Context, id string) error {
	return a.core.DeleteTemplate(ctx, id)
}

// Interface: Validate(ctx, id) (*TemplateValidationResult, error)
// Core: ValidateTemplate(content string) []string — no ctx, no error return
func (a *templateManagerAdapter) Validate(ctx context.Context, id string) (*TemplateValidationResult, error) {
	tpl, err := a.core.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	content := tpl.HTMLContent
	if content == "" {
		content = tpl.PlainTextContent
	}
	errs := a.core.ValidateTemplate(content)
	return &TemplateValidationResult{Valid: len(errs) == 0, Errors: errs}, nil
}

// Interface: CheckSpam(ctx, id) (*SpamCheckResult, error)
// Core: CheckSpamScore(content string) float64 — no ctx, no error return
func (a *templateManagerAdapter) CheckSpam(ctx context.Context, id string) (*SpamCheckResult, error) {
	tpl, err := a.core.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	content := tpl.HTMLContent
	if content == "" {
		content = tpl.PlainTextContent
	}
	score := a.core.CheckSpamScore(content)
	return &SpamCheckResult{Score: score}, nil
}

// Interface: Preview(ctx, *TemplatePreviewRequest) (*TemplatePreviewResult, error)
// Core: RenderPreview(templateID, vars map[string]string) (string, error) — no ctx
func (a *templateManagerAdapter) Preview(ctx context.Context, req *TemplatePreviewRequest) (*TemplatePreviewResult, error) {
	tpl, err := a.core.GetByID(ctx, req.TemplateID)
	if err != nil {
		return nil, err
	}
	subject := ""
	if len(tpl.Subjects) > 0 {
		subject = tpl.Subjects[0]
	}
	// Convert map[string]interface{} → map[string]string for RenderPreview.
	vars := make(map[string]string, len(req.SampleData))
	for k, v := range req.SampleData {
		vars[k] = fmt.Sprintf("%v", v)
	}
	rendered, renderErr := a.core.RenderPreview(req.TemplateID, vars)
	if renderErr != nil {
		// Fall back to raw content — preview still returns something usable.
		rendered = tpl.HTMLContent
		if rendered == "" {
			rendered = tpl.PlainTextContent
		}
	}
	return &TemplatePreviewResult{
		Subject:     subject,
		Content:     rendered,
		PlainText:   tpl.PlainTextContent,
		PreviewHTML: rendered,
	}, nil
}

// Interface: ExtractVariables(ctx, id) ([]string, error)
// Core: GetTemplate(id) (*ManagedTemplate, error) — no ctx
func (a *templateManagerAdapter) ExtractVariables(_ context.Context, id string) ([]string, error) {
	managed, err := a.core.GetTemplate(id)
	if err != nil {
		return nil, err
	}
	return managed.ParsedVars, nil
}

// Interface: ImportFromDirectory(ctx, dirPath) (*ImportResult, error)
// Core: LoadFromDirectory(dirPath string) error — no ctx, no count returned
func (a *templateManagerAdapter) ImportFromDirectory(_ context.Context, dirPath string) (*ImportResult, error) {
	if err := a.core.LoadFromDirectory(dirPath); err != nil {
		return &ImportResult{Failed: 1, Errors: []string{err.Error()}}, err
	}
	n := countTemplateFiles(dirPath)
	return &ImportResult{Total: n, Successful: n}, nil
}

// Interface: ImportFromZip(ctx, zipPath) (*ImportResult, error)
// Core: LoadFromZip(zipPath string) error — no ctx
func (a *templateManagerAdapter) ImportFromZip(_ context.Context, zipPath string) (*ImportResult, error) {
	if err := a.core.LoadFromZip(zipPath); err != nil {
		return &ImportResult{Failed: 1, Errors: []string{err.Error()}}, err
	}
	return &ImportResult{Total: 1, Successful: 1}, nil
}

// Interface: Duplicate(ctx, id, newName) (*models.Template, error)
// Core Create returns *models.Template — direct return after creation.
func (a *templateManagerAdapter) Duplicate(ctx context.Context, id, newName string) (*models.Template, error) {
	original, err := a.core.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	content := original.HTMLContent
	if content == "" {
		content = original.PlainTextContent
	}
	subject := ""
	if len(original.Subjects) > 0 {
		subject = original.Subjects[0]
	}
	return a.core.Create(ctx, &coretemplate.CreateTemplateReq{
		Name:        newName,
		Description: original.Description,
		Type:        string(original.Type),
		Content:     content,
		Subject:     subject,
		Tags:        original.Tags,
		IsActive:    original.IsActive(),
		Config:      original.Metadata,
	})
}

func (a *templateManagerAdapter) GetCampaigns(_ context.Context, _ string) (interface{}, error) {
	return []interface{}{}, nil
}

func (a *templateManagerAdapter) GetUsageStats(_ context.Context, id string) (interface{}, error) {
	managed, err := a.core.GetTemplate(id) // no ctx
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"total_used":     managed.Usage.TotalUsed,
		"success_count":  managed.Usage.SuccessCount,
		"failure_count":  managed.Usage.FailureCount,
		"last_used":      managed.Usage.LastUsed,
		"cache_hit_rate": managed.Usage.CacheHitRate,
	}, nil
}

func countTemplateFiles(dirPath string) int {
	count := 0
	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".html" || ext == ".htm" || ext == ".txt" {
			count++
		}
		return nil
	})
	return count
}
