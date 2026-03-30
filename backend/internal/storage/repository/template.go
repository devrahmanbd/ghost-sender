package repository

import (
        "context"
        "database/sql"
        "encoding/json"
        "fmt"
        "strings"
        "time"
        
        "github.com/lib/pq"
)

type TemplateRepository struct {
        db *sql.DB
}

type Template struct {
        ID              string                 `json:"id" db:"id"`
        Name            string                 `json:"name" db:"name"`
        Slug            string                 `json:"slug" db:"slug"`
        Description     string                 `json:"description" db:"description"`
        Category        string                 `json:"category" db:"category"`
        Type            string                 `json:"type" db:"type"`
        Version         int                    `json:"version" db:"version"`
        IsDefault       bool                   `json:"is_default" db:"is_default"`
        IsActive        bool                   `json:"is_active" db:"is_active"`
        Language        string                 `json:"language" db:"language"`
        Subject         string                 `json:"subject" db:"subject"`
        FromName        string                 `json:"from_name" db:"from_name"`
        FromEmail       string                 `json:"from_email" db:"from_email"`
        ReplyTo         string                 `json:"reply_to" db:"reply_to"`
        HtmlContent     string                 `json:"html_content" db:"html_content"`
        TextContent     string                 `json:"text_content" db:"text_content"`
        Preheader       string                 `json:"preheader" db:"preheader"`
        Variables       []string               `json:"variables" db:"variables"`
        CustomHeaders   map[string]string      `json:"custom_headers" db:"custom_headers"`
        Attachments     []string               `json:"attachments" db:"attachments"`
        Tags            []string               `json:"tags" db:"tags"`
        Metadata        map[string]interface{} `json:"metadata" db:"metadata"`
        SpamScore       float64                `json:"spam_score" db:"spam_score"`
        SpamDetails     map[string]interface{} `json:"spam_details" db:"spam_details"`
        LastSpamCheckAt *time.Time             `json:"last_spam_check_at" db:"last_spam_check_at"`
        RenderCount     int64                  `json:"render_count" db:"render_count"`
        LastRenderedAt  *time.Time             `json:"last_rendered_at" db:"last_rendered_at"`
        LastUsedAt      *time.Time             `json:"last_used_at" db:"last_used_at"`
        UsageCount      int64                  `json:"usage_count" db:"usage_count"`
        FailureCount    int64                  `json:"failure_count" db:"failure_count"`
        LastFailureAt   *time.Time             `json:"last_failure_at" db:"last_failure_at"`
        LastFailureMsg  string                 `json:"last_failure_msg" db:"last_failure_msg"`
        Engine          string                 `json:"engine" db:"engine"`
        RenderingConfig map[string]interface{} `json:"rendering_config" db:"rendering_config"`
        RotationGroup   string                 `json:"rotation_group" db:"rotation_group"`
        RotationWeight  int                    `json:"rotation_weight" db:"rotation_weight"`
        RotationIndex   int                    `json:"rotation_index" db:"rotation_index"`
        IsArchived      bool                   `json:"is_archived" db:"is_archived"`
        ArchivedAt      *time.Time             `json:"archived_at" db:"archived_at"`
        CreatedAt       time.Time                  `json:"created_at" db:"created_at"`
        UpdatedAt       time.Time                  `json:"updated_at" db:"updated_at"`
        CreatedBy       string                     `json:"created_by" db:"created_by"`
        UpdatedBy       string                     `json:"updated_by" db:"updated_by"`
        SuccessRate     float64                    `json:"success_rate" db:"success_rate"`
        CustomVariables map[string][]string       `json:"custom_variables" db:"custom_variables"`
}

type TemplateFilter struct {
        IDs             []string
        Slugs           []string
        Categories      []string
        Tags            []string
        Types           []string
        Languages       []string
        RotationGroup   string
        IsActive        *bool
        IsDefault       *bool
        IsArchived      *bool
        MinSpamScore    *float64
        MaxSpamScore    *float64
        Search          string
        SortBy          string
        SortOrder       string
        Limit           int
        Offset          int
        CreatedAfter    *time.Time
        CreatedBefore   *time.Time
        LastUsedAfter   *time.Time
        LastUsedBefore  *time.Time
        IncludeArchived bool
}
// ============================================================================
// COUNT METHODS FOR METRICS
// ============================================================================

// Count returns the total number of templates matching the filter
func (r *TemplateRepository) Count(ctx context.Context, filter *TemplateFilter) (int, error) {
    whereClauses := []string{"1=1"}
    args := []interface{}{}
    argPos := 1

    if filter != nil {
        if len(filter.IDs) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.IDs))
            argPos++
        }

        if len(filter.Categories) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("category = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.Categories))
            argPos++
        }

        if len(filter.Types) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("type = ANY($%d)", argPos))
            args = append(args, pq.Array(filter.Types))
            argPos++
        }

        if len(filter.Tags) > 0 {
            whereClauses = append(whereClauses, fmt.Sprintf("tags && $%d", argPos))
            args = append(args, pq.Array(filter.Tags))
            argPos++
        }

        if filter.RotationGroup != "" {
            whereClauses = append(whereClauses, fmt.Sprintf("rotation_group = $%d", argPos))
            args = append(args, filter.RotationGroup)
            argPos++
        }

        if filter.IsActive != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("is_active = $%d", argPos))
            args = append(args, *filter.IsActive)
            argPos++
        }

        if filter.IsDefault != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("is_default = $%d", argPos))
            args = append(args, *filter.IsDefault)
            argPos++
        }

        if filter.IsArchived != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("is_archived = $%d", argPos))
            args = append(args, *filter.IsArchived)
            argPos++
        } else if !filter.IncludeArchived {
            whereClauses = append(whereClauses, "is_archived = false")
        }

        if filter.MinSpamScore != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("spam_score >= $%d", argPos))
            args = append(args, *filter.MinSpamScore)
            argPos++
        }

        if filter.MaxSpamScore != nil {
            whereClauses = append(whereClauses, fmt.Sprintf("spam_score <= $%d", argPos))
            args = append(args, *filter.MaxSpamScore)
            argPos++
        }
    }

    whereClause := strings.Join(whereClauses, " AND ")
    query := fmt.Sprintf("SELECT COUNT(*) FROM templates WHERE %s", whereClause)

    var count int
    err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count templates: %w", err)
    }

    return count, nil
}

// CountActive returns the count of active templates
func (r *TemplateRepository) CountActive(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM templates WHERE is_active = true AND is_archived = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count active templates: %w", err)
    }
    
    return count, nil
}

// CountArchived returns the count of archived templates
func (r *TemplateRepository) CountArchived(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM templates WHERE is_archived = true`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count archived templates: %w", err)
    }
    
    return count, nil
}

// CountDefault returns the count of default templates
func (r *TemplateRepository) CountDefault(ctx context.Context) (int, error) {
    query := `SELECT COUNT(*) FROM templates WHERE is_default = true AND is_archived = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count default templates: %w", err)
    }
    
    return count, nil
}

// CountByCategory returns the count of templates in a specific category
func (r *TemplateRepository) CountByCategory(ctx context.Context, category string) (int, error) {
    query := `SELECT COUNT(*) FROM templates WHERE category = $1 AND is_archived = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, category).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count templates by category: %w", err)
    }
    
    return count, nil
}

// CountByType returns the count of templates of a specific type
func (r *TemplateRepository) CountByType(ctx context.Context, templateType string) (int, error) {
    query := `SELECT COUNT(*) FROM templates WHERE type = $1 AND is_archived = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, templateType).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count templates by type: %w", err)
    }
    
    return count, nil
}

// CountByRotationGroup returns the count of templates in a rotation group
func (r *TemplateRepository) CountByRotationGroup(ctx context.Context, group string) (int, error) {
    query := `SELECT COUNT(*) FROM templates WHERE rotation_group = $1 AND is_active = true AND is_archived = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, group).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count templates by rotation group: %w", err)
    }
    
    return count, nil
}

// CountWithHighSpamScore returns templates with spam score above threshold
func (r *TemplateRepository) CountWithHighSpamScore(ctx context.Context, threshold float64) (int, error) {
    query := `SELECT COUNT(*) FROM templates WHERE spam_score > $1 AND is_archived = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, threshold).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count templates with high spam score: %w", err)
    }
    
    return count, nil
}

// CountWithLowSpamScore returns templates with spam score below threshold
func (r *TemplateRepository) CountWithLowSpamScore(ctx context.Context, threshold float64) (int, error) {
    query := `SELECT COUNT(*) FROM templates WHERE spam_score <= $1 AND is_archived = false`
    
    var count int
    err := r.db.QueryRowContext(ctx, query, threshold).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count templates with low spam score: %w", err)
    }
    
    return count, nil
}

// ============================================================================
// ADDITIONAL METRICS HELPERS
// ============================================================================

// GetTotalRenderCount returns the total render count across all templates
func (r *TemplateRepository) GetTotalRenderCount(ctx context.Context) (int64, error) {
    query := `SELECT COALESCE(SUM(render_count), 0) FROM templates WHERE is_archived = false`
    
    var total int64
    err := r.db.QueryRowContext(ctx, query).Scan(&total)
    if err != nil {
        return 0, fmt.Errorf("failed to get total render count: %w", err)
    }
    
    return total, nil
}

// GetTotalUsageCount returns the total usage count across all templates
func (r *TemplateRepository) GetTotalUsageCount(ctx context.Context) (int64, error) {
    query := `SELECT COALESCE(SUM(usage_count), 0) FROM templates WHERE is_archived = false`
    
    var total int64
    err := r.db.QueryRowContext(ctx, query).Scan(&total)
    if err != nil {
        return 0, fmt.Errorf("failed to get total usage count: %w", err)
    }
    
    return total, nil
}

// GetTotalFailureCount returns the total failure count across all templates
func (r *TemplateRepository) GetTotalFailureCount(ctx context.Context) (int64, error) {
    query := `SELECT COALESCE(SUM(failure_count), 0) FROM templates WHERE is_archived = false`
    
    var total int64
    err := r.db.QueryRowContext(ctx, query).Scan(&total)
    if err != nil {
        return 0, fmt.Errorf("failed to get total failure count: %w", err)
    }
    
    return total, nil
}

// GetAverageSpamScore returns the average spam score across all templates
func (r *TemplateRepository) GetAverageSpamScore(ctx context.Context) (float64, error) {
    query := `SELECT COALESCE(AVG(spam_score), 0) FROM templates WHERE is_archived = false AND spam_score > 0`
    
    var avgScore float64
    err := r.db.QueryRowContext(ctx, query).Scan(&avgScore)
    if err != nil {
        return 0, fmt.Errorf("failed to get average spam score: %w", err)
    }
    
    return avgScore, nil
}

// GetCategoryBreakdown returns count of templates grouped by category
func (r *TemplateRepository) GetCategoryBreakdown(ctx context.Context) (map[string]int, error) {
    query := `
        SELECT category, COUNT(*) as count
        FROM templates
        WHERE is_archived = false
        GROUP BY category`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get category breakdown: %w", err)
    }
    defer rows.Close()

    breakdown := make(map[string]int)
    for rows.Next() {
        var category string
        var count int
        if err := rows.Scan(&category, &count); err != nil {
            return nil, err
        }
        breakdown[category] = count
    }

    return breakdown, nil
}

// GetTypeBreakdown returns count of templates grouped by type
func (r *TemplateRepository) GetTypeBreakdown(ctx context.Context) (map[string]int, error) {
    query := `
        SELECT type, COUNT(*) as count
        FROM templates
        WHERE is_archived = false
        GROUP BY type`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get type breakdown: %w", err)
    }
    defer rows.Close()

    breakdown := make(map[string]int)
    for rows.Next() {
        var templateType string
        var count int
        if err := rows.Scan(&templateType, &count); err != nil {
            return nil, err
        }
        breakdown[templateType] = count
    }

    return breakdown, nil
}

// GetMostUsedTemplates returns templates with highest usage counts
func (r *TemplateRepository) GetMostUsedTemplates(ctx context.Context, limit int) ([]*Template, error) {
    if limit <= 0 {
        limit = 10
    }

    query := `
        SELECT id, name, slug, category, type, usage_count, render_count, 
               success_rate, last_used_at, created_at, updated_at
        FROM templates
        WHERE is_archived = false AND usage_count > 0
        ORDER BY usage_count DESC, render_count DESC
        LIMIT $1`

    rows, err := r.db.QueryContext(ctx, query, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get most used templates: %w", err)
    }
    defer rows.Close()

    templates := []*Template{}
    for rows.Next() {
        t := &Template{}
        var successRate sql.NullFloat64
        err := rows.Scan(
            &t.ID, &t.Name, &t.Slug, &t.Category, &t.Type,
            &t.UsageCount, &t.RenderCount, &successRate,
            &t.LastUsedAt, &t.CreatedAt, &t.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        if successRate.Valid {
            t.SuccessRate = successRate.Float64
        }
        templates = append(templates, t)
    }

    return templates, nil
}

// GetLeastUsedTemplates returns templates with lowest usage counts
func (r *TemplateRepository) GetLeastUsedTemplates(ctx context.Context, limit int) ([]*Template, error) {
    if limit <= 0 {
        limit = 10
    }

    query := `
        SELECT id, name, slug, category, type, usage_count, render_count, 
               last_used_at, created_at, updated_at
        FROM templates
        WHERE is_archived = false AND is_active = true
        ORDER BY usage_count ASC, created_at ASC
        LIMIT $1`

    rows, err := r.db.QueryContext(ctx, query, limit)
    if err != nil {
        return nil, fmt.Errorf("failed to get least used templates: %w", err)
    }
    defer rows.Close()

    templates := []*Template{}
    for rows.Next() {
        t := &Template{}
        err := rows.Scan(
            &t.ID, &t.Name, &t.Slug, &t.Category, &t.Type,
            &t.UsageCount, &t.RenderCount, &t.LastUsedAt,
            &t.CreatedAt, &t.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }
        templates = append(templates, t)
    }

    return templates, nil
}

// GetTemplatesNeedingSpamCheck returns templates that need spam score checking
func (r *TemplateRepository) GetTemplatesNeedingSpamCheck(ctx context.Context, olderThan time.Duration) ([]*Template, error) {
    checkTime := time.Now().Add(-olderThan)

    query := `
        SELECT id, name, slug, subject, spam_score, last_spam_check_at
        FROM templates
        WHERE is_archived = false 
          AND is_active = true
          AND (last_spam_check_at IS NULL OR last_spam_check_at < $1)
        ORDER BY last_spam_check_at ASC NULLS FIRST
        LIMIT 100`

    rows, err := r.db.QueryContext(ctx, query, checkTime)
    if err != nil {
        return nil, fmt.Errorf("failed to get templates needing spam check: %w", err)
    }
    defer rows.Close()

    templates := []*Template{}
    for rows.Next() {
        t := &Template{}
        err := rows.Scan(
            &t.ID, &t.Name, &t.Slug, &t.Subject,
            &t.SpamScore, &t.LastSpamCheckAt,
        )
        if err != nil {
            return nil, err
        }
        templates = append(templates, t)
    }

    return templates, nil
}

// GetRotationGroupSummary returns summary stats for all rotation groups
func (r *TemplateRepository) GetRotationGroupSummary(ctx context.Context) (map[string]int, error) {
    query := `
        SELECT rotation_group, COUNT(*) as count
        FROM templates
        WHERE is_archived = false 
          AND is_active = true
          AND rotation_group IS NOT NULL 
          AND rotation_group <> ''
        GROUP BY rotation_group`

    rows, err := r.db.QueryContext(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get rotation group summary: %w", err)
    }
    defer rows.Close()

    summary := make(map[string]int)
    for rows.Next() {
        var group string
        var count int
        if err := rows.Scan(&group, &count); err != nil {
            return nil, err
        }
        summary[group] = count
    }

    return summary, nil
}

type TemplateStats struct {
        TotalTemplates       int             `json:"total_templates"`
        ActiveTemplates      int             `json:"active_templates"`
        ArchivedTemplates    int             `json:"archived_templates"`
        DefaultTemplates     int             `json:"default_templates"`
        AverageSpamScore     float64         `json:"average_spam_score"`
        TotalRenderCount     int64           `json:"total_render_count"`
        TotalUsageCount      int64           `json:"total_usage_count"`
        TotalFailureCount    int64           `json:"total_failure_count"`
        CategoryBreakdown    map[string]int  `json:"category_breakdown"`
        TypeBreakdown        map[string]int  `json:"type_breakdown"`
        RotationGroupSummary map[string]int  `json:"rotation_group_summary"`
}

func NewTemplateRepository(db *sql.DB) *TemplateRepository {
        return &TemplateRepository{db: db}
}
func (r *TemplateRepository) Create(ctx context.Context, t *Template) error {
    content := t.HtmlContent
    if content == "" {
        content = t.TextContent
    }

    customHeadersJSON, _ := json.Marshal(t.CustomHeaders)
    metadataJSON, _ := json.Marshal(t.Metadata)
    spamDetailsJSON, _ := json.Marshal(t.SpamDetails)
    renderConfigJSON, _ := json.Marshal(t.RenderingConfig)
    customVariablesJSON, _ := json.Marshal(t.CustomVariables)

        query := `
                INSERT INTO templates (
                        id, name, slug, description, category, type, version, is_default, is_active,
                        language, subject, from_name, from_email, reply_to,
                        content, html_content, text_content,
                        preheader, variables, custom_headers, attachments, tags, metadata, spam_score,
                        spam_details, engine, rendering_config, rotation_group, rotation_weight,
                        rotation_index, created_by, is_archived, custom_variables, created_at, updated_at
                ) VALUES (
                        $1, $2, $3, $4, $5, $6, $7, $8, $9,
                        $10, $11, $12, $13, $14,
                        $15, $16, $17,
                        $18, $19, $20, $21, $22, $23, $24,
                        $25, $26, $27, $28, $29,
                        $30, $31, $32, $33, $34, $35
                ) RETURNING id, created_at, updated_at`

        err := r.db.QueryRowContext(
                ctx, query,
                t.ID, t.Name, t.Slug, t.Description, t.Category, t.Type, t.Version, t.IsDefault, t.IsActive,
                t.Language, t.Subject, t.FromName, t.FromEmail, t.ReplyTo,
                content, t.HtmlContent, t.TextContent,
                t.Preheader, pq.Array(t.Variables), customHeadersJSON, pq.Array(t.Attachments),
                pq.Array(t.Tags), metadataJSON, t.SpamScore, spamDetailsJSON, t.Engine, renderConfigJSON,
                t.RotationGroup, t.RotationWeight, t.RotationIndex, t.CreatedBy, t.IsArchived,
                customVariablesJSON, time.Now(), time.Now(),
        ).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
    if err != nil {
        return err
    }

    return nil
}

func (r *TemplateRepository) GetByID(ctx context.Context, id string) (*Template, error) {
        query := `
                SELECT id, name, slug, description, category, type, version, is_default, is_active,
                        language, subject, from_name, from_email, reply_to, html_content, text_content,
                        preheader, variables, custom_headers, attachments, tags, metadata, spam_score,
                        spam_details, last_spam_check_at, render_count, last_rendered_at, last_used_at,
                        usage_count, failure_count, last_failure_at, last_failure_msg, engine,
                        rendering_config, rotation_group, rotation_weight, rotation_index,
                        is_archived, archived_at, created_at, updated_at, created_by, updated_by, custom_variables
                FROM templates
                WHERE id = $1`

        t := &Template{}
        var customHeadersJSON, metadataJSON, spamDetailsJSON, renderConfigJSON, customVariablesJSON []byte

        err := r.db.QueryRowContext(ctx, query, id).Scan(
                &t.ID, &t.Name, &t.Slug, &t.Description, &t.Category, &t.Type, &t.Version,
                &t.IsDefault, &t.IsActive, &t.Language, &t.Subject, &t.FromName, &t.FromEmail,
                &t.ReplyTo, &t.HtmlContent, &t.TextContent, &t.Preheader, pq.Array(&t.Variables),
                &customHeadersJSON, pq.Array(&t.Attachments), pq.Array(&t.Tags), &metadataJSON,
                &t.SpamScore, &spamDetailsJSON, &t.LastSpamCheckAt, &t.RenderCount,
                &t.LastRenderedAt, &t.LastUsedAt, &t.UsageCount, &t.FailureCount, &t.LastFailureAt,
                &t.LastFailureMsg, &t.Engine, &renderConfigJSON, &t.RotationGroup, &t.RotationWeight,
                &t.RotationIndex, &t.IsArchived, &t.ArchivedAt, &t.CreatedAt, &t.UpdatedAt,
                &t.CreatedBy, &t.UpdatedBy, &customVariablesJSON,
        )
        if err != nil {
                if err == sql.ErrNoRows {
                        return nil, fmt.Errorf("template not found")
                }
                return nil, err
        }

        if len(customHeadersJSON) > 0 {
                json.Unmarshal(customHeadersJSON, &t.CustomHeaders)
        }
        if len(metadataJSON) > 0 {
                json.Unmarshal(metadataJSON, &t.Metadata)
        }
        if len(spamDetailsJSON) > 0 {
                json.Unmarshal(spamDetailsJSON, &t.SpamDetails)
        }
        if len(renderConfigJSON) > 0 {
                json.Unmarshal(renderConfigJSON, &t.RenderingConfig)
        }
        if len(customVariablesJSON) > 0 {
                json.Unmarshal(customVariablesJSON, &t.CustomVariables)
        }

        return t, nil
}

func (r *TemplateRepository) GetBySlug(ctx context.Context, slug string) (*Template, error) {
        query := `
                SELECT id, name, slug, description, category, type, version, is_default, is_active,
                        language, subject, from_name, from_email, reply_to, html_content, text_content,
                        preheader, variables, custom_headers, attachments, tags, metadata, spam_score,
                        spam_details, engine, rendering_config, rotation_group, rotation_weight,
                        rotation_index, is_archived, created_at, updated_at, created_by, updated_by, custom_variables
                FROM templates
                WHERE slug = $1 AND is_archived = false
                ORDER BY version DESC
                LIMIT 1`

        t := &Template{}
        var customHeadersJSON, metadataJSON, spamDetailsJSON, renderConfigJSON, customVariablesJSON []byte

        err := r.db.QueryRowContext(ctx, query, slug).Scan(
                &t.ID, &t.Name, &t.Slug, &t.Description, &t.Category, &t.Type, &t.Version,
                &t.IsDefault, &t.IsActive, &t.Language, &t.Subject, &t.FromName, &t.FromEmail,
                &t.ReplyTo, &t.HtmlContent, &t.TextContent, &t.Preheader, pq.Array(&t.Variables),
                &customHeadersJSON, pq.Array(&t.Attachments), pq.Array(&t.Tags), &metadataJSON,
                &t.SpamScore, &spamDetailsJSON, &t.Engine, &renderConfigJSON, &t.RotationGroup,
                &t.RotationWeight, &t.RotationIndex, &t.IsArchived, &t.CreatedAt, &t.UpdatedAt,
                &t.CreatedBy, &t.UpdatedBy, &customVariablesJSON,
        )

        if err != nil {
                if err == sql.ErrNoRows {
                        return nil, fmt.Errorf("template not found")
                }
                return nil, err
        }

        if len(customHeadersJSON) > 0 {
                json.Unmarshal(customHeadersJSON, &t.CustomHeaders)
        }
        if len(metadataJSON) > 0 {
                json.Unmarshal(metadataJSON, &t.Metadata)
        }
        if len(spamDetailsJSON) > 0 {
                json.Unmarshal(spamDetailsJSON, &t.SpamDetails)
        }
        if len(renderConfigJSON) > 0 {
                json.Unmarshal(renderConfigJSON, &t.RenderingConfig)
        }
        if len(customVariablesJSON) > 0 {
                json.Unmarshal(customVariablesJSON, &t.CustomVariables)
        }

        return t, nil
}

func (r *TemplateRepository) Update(ctx context.Context, t *Template) error {
        customHeadersJSON, _ := json.Marshal(t.CustomHeaders)
        metadataJSON, _ := json.Marshal(t.Metadata)
        spamDetailsJSON, _ := json.Marshal(t.SpamDetails)
        renderConfigJSON, _ := json.Marshal(t.RenderingConfig)
        customVariablesJSON, _ := json.Marshal(t.CustomVariables)

        content := t.HtmlContent
        if content == "" {
                content = t.TextContent
        }

        query := `
                UPDATE templates SET
                        name = $2, slug = $3, description = $4, category = $5, type = $6,
                        version = $7, is_default = $8, is_active = $9, language = $10,
                        subject = $11, from_name = $12, from_email = $13, reply_to = $14,
                        content = $15, html_content = $16, text_content = $17, preheader = $18,
                        variables = $19, custom_headers = $20, attachments = $21, tags = $22,
                        metadata = $23, spam_score = $24, spam_details = $25, engine = $26,
                        rendering_config = $27, rotation_group = $28, rotation_weight = $29,
                        rotation_index = $30, is_archived = $31, archived_at = $32,
                        updated_by = $33, updated_at = $34, custom_variables = $35
                WHERE id = $1`

        result, err := r.db.ExecContext(
                ctx, query,
                t.ID, t.Name, t.Slug, t.Description, t.Category, t.Type, t.Version,
                t.IsDefault, t.IsActive, t.Language, t.Subject, t.FromName, t.FromEmail,
                t.ReplyTo, content, t.HtmlContent, t.TextContent, t.Preheader,
                pq.Array(t.Variables), customHeadersJSON, pq.Array(t.Attachments), pq.Array(t.Tags),
                metadataJSON, t.SpamScore, spamDetailsJSON, t.Engine, renderConfigJSON,
                t.RotationGroup, t.RotationWeight, t.RotationIndex, t.IsArchived, t.ArchivedAt,
                t.UpdatedBy, time.Now(), customVariablesJSON,
        )
        if err != nil {
                return err
        }

        rows, err := result.RowsAffected()
        if err != nil {
                return err
        }
        if rows == 0 {
                return fmt.Errorf("template not found")
        }
        return nil
}


func (r *TemplateRepository) Delete(ctx context.Context, id string) error {
        query := `DELETE FROM templates WHERE id = $1`
        result, err := r.db.ExecContext(ctx, query, id)
        if err != nil {
                return err
        }
        rows, err := result.RowsAffected()
        if err != nil {
                return err
        }
        if rows == 0 {
                return fmt.Errorf("template not found")
        }
        return nil
}

func (r *TemplateRepository) Archive(ctx context.Context, id string) error {
        query := `
                UPDATE templates SET
                        is_archived = true,
                        archived_at = $2,
                        is_active = false,
                        updated_at = $2
                WHERE id = $1`

        _, err := r.db.ExecContext(ctx, query, id, time.Now())
        return err
}

func (r *TemplateRepository) Restore(ctx context.Context, id string) error {
        query := `
                UPDATE templates SET
                        is_archived = false,
                        archived_at = NULL,
                        is_active = true,
                        updated_at = $2
                WHERE id = $1`

        _, err := r.db.ExecContext(ctx, query, id, time.Now())
        return err
}

func (r *TemplateRepository) List(ctx context.Context, filter *TemplateFilter) ([]*Template, int, error) {
        whereClauses := []string{"1=1"}
        args := []interface{}{}
        argPos := 1

        if filter != nil {
                if len(filter.IDs) > 0 {
                        whereClauses = append(whereClauses, fmt.Sprintf("id = ANY($%d)", argPos))
                        args = append(args, pq.Array(filter.IDs))
                        argPos++
                }

                if len(filter.Slugs) > 0 {
                        whereClauses = append(whereClauses, fmt.Sprintf("slug = ANY($%d)", argPos))
                        args = append(args, pq.Array(filter.Slugs))
                        argPos++
                }

                if len(filter.Categories) > 0 {
                        whereClauses = append(whereClauses, fmt.Sprintf("category = ANY($%d)", argPos))
                        args = append(args, pq.Array(filter.Categories))
                        argPos++
                }

                if len(filter.Types) > 0 {
                        whereClauses = append(whereClauses, fmt.Sprintf("type = ANY($%d)", argPos))
                        args = append(args, pq.Array(filter.Types))
                        argPos++
                }

                if len(filter.Languages) > 0 {
                        whereClauses = append(whereClauses, fmt.Sprintf("language = ANY($%d)", argPos))
                        args = append(args, pq.Array(filter.Languages))
                        argPos++
                }

                if len(filter.Tags) > 0 {
                        whereClauses = append(whereClauses, fmt.Sprintf("tags && $%d", argPos))
                        args = append(args, pq.Array(filter.Tags))
                        argPos++
                }

                if filter.RotationGroup != "" {
                        whereClauses = append(whereClauses, fmt.Sprintf("rotation_group = $%d", argPos))
                        args = append(args, filter.RotationGroup)
                        argPos++
                }

                if filter.IsActive != nil {
                        whereClauses = append(whereClauses, fmt.Sprintf("is_active = $%d", argPos))
                        args = append(args, *filter.IsActive)
                        argPos++
                }

                if filter.IsDefault != nil {
                        whereClauses = append(whereClauses, fmt.Sprintf("is_default = $%d", argPos))
                        args = append(args, *filter.IsDefault)
                        argPos++
                }

                if filter.IsArchived != nil {
                        whereClauses = append(whereClauses, fmt.Sprintf("is_archived = $%d", argPos))
                        args = append(args, *filter.IsArchived)
                        argPos++
                } else if !filter.IncludeArchived {
                        whereClauses = append(whereClauses, "is_archived = false")
                }

                if filter.MinSpamScore != nil {
                        whereClauses = append(whereClauses, fmt.Sprintf("spam_score >= $%d", argPos))
                        args = append(args, *filter.MinSpamScore)
                        argPos++
                }

                if filter.MaxSpamScore != nil {
                        whereClauses = append(whereClauses, fmt.Sprintf("spam_score <= $%d", argPos))
                        args = append(args, *filter.MaxSpamScore)
                        argPos++
                }

                if filter.CreatedAfter != nil {
                        whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argPos))
                        args = append(args, *filter.CreatedAfter)
                        argPos++
                }

                if filter.CreatedBefore != nil {
                        whereClauses = append(whereClauses, fmt.Sprintf("created_at <= $%d", argPos))
                        args = append(args, *filter.CreatedBefore)
                        argPos++
                }

                if filter.LastUsedAfter != nil {
                        whereClauses = append(whereClauses, fmt.Sprintf("last_used_at >= $%d", argPos))
                        args = append(args, *filter.LastUsedAfter)
                        argPos++
                }

                if filter.LastUsedBefore != nil {
                        whereClauses = append(whereClauses, fmt.Sprintf("last_used_at <= $%d", argPos))
                        args = append(args, *filter.LastUsedBefore)
                        argPos++
                }

                if filter.Search != "" {
                        whereClauses = append(whereClauses, fmt.Sprintf(
                                "(name ILIKE $%d OR description ILIKE $%d OR subject ILIKE $%d)",
                                argPos, argPos, argPos))
                        args = append(args, "%"+filter.Search+"%")
                        argPos++
                }
        }

        whereClause := strings.Join(whereClauses, " AND ")

        countQuery := fmt.Sprintf("SELECT COUNT(*) FROM templates WHERE %s", whereClause)
        var total int
        if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
                return nil, 0, err
        }

        sortBy := "created_at"
        sortOrder := "DESC"
        limit := 50
        offset := 0

        allowedTemplateCols := []string{"id", "name", "slug", "category", "type", "version", "created_at", "updated_at", "usage_count", "spam_score", "render_count", "last_used_at", "is_active"}
        if filter != nil {
                sortBy = sanitizeSortColumn(filter.SortBy, "created_at", allowedTemplateCols)
                sortOrder = sanitizeSortOrder(filter.SortOrder)
                if filter.Limit > 0 {
                        limit = filter.Limit
                }
                if filter.Offset > 0 {
                        offset = filter.Offset
                }
        }

        query := fmt.Sprintf(`
                SELECT id, name, slug, description, category, type, version, is_default, is_active,
                        language, subject, from_name, from_email, reply_to, html_content, text_content,
                        preheader, variables, custom_headers, attachments, tags, metadata, spam_score,
                        spam_details, last_spam_check_at, render_count, last_rendered_at, last_used_at,
                        usage_count, failure_count, last_failure_at, last_failure_msg, engine,
                        rendering_config, rotation_group, rotation_weight, rotation_index,
                        is_archived, archived_at, created_at, updated_at, created_by, updated_by,
                        custom_variables
                FROM templates
                WHERE %s
                ORDER BY %s %s
                LIMIT $%d OFFSET $%d`,
                whereClause, sortBy, sortOrder, argPos, argPos+1)

        args = append(args, limit, offset)

        rows, err := r.db.QueryContext(ctx, query, args...)
        if err != nil {
                return nil, 0, err
        }
        defer rows.Close()

        templates := []*Template{}
        for rows.Next() {
                t := &Template{}
                var customHeadersJSON, metadataJSON, spamDetailsJSON, renderConfigJSON, customVariablesJSON []byte

                err := rows.Scan(
                        &t.ID, &t.Name, &t.Slug, &t.Description, &t.Category, &t.Type, &t.Version,
                        &t.IsDefault, &t.IsActive, &t.Language, &t.Subject, &t.FromName, &t.FromEmail,
                        &t.ReplyTo, &t.HtmlContent, &t.TextContent, &t.Preheader, pq.Array(&t.Variables),
                        &customHeadersJSON, pq.Array(&t.Attachments), pq.Array(&t.Tags), &metadataJSON,
                        &t.SpamScore, &spamDetailsJSON, &t.LastSpamCheckAt, &t.RenderCount,
                        &t.LastRenderedAt, &t.LastUsedAt, &t.UsageCount, &t.FailureCount, &t.LastFailureAt,
                        &t.LastFailureMsg, &t.Engine, &renderConfigJSON, &t.RotationGroup, &t.RotationWeight,
                        &t.RotationIndex, &t.IsArchived, &t.ArchivedAt, &t.CreatedAt, &t.UpdatedAt,
                        &t.CreatedBy, &t.UpdatedBy, &customVariablesJSON,
                )
                if err != nil {
                        return nil, 0, err
                }

                if len(customHeadersJSON) > 0 {
                        json.Unmarshal(customHeadersJSON, &t.CustomHeaders)
                }
                if len(metadataJSON) > 0 {
                        json.Unmarshal(metadataJSON, &t.Metadata)
                }
                if len(spamDetailsJSON) > 0 {
                        json.Unmarshal(spamDetailsJSON, &t.SpamDetails)
                }
                if len(renderConfigJSON) > 0 {
                        json.Unmarshal(renderConfigJSON, &t.RenderingConfig)
                }
                if len(customVariablesJSON) > 0 {
                        json.Unmarshal(customVariablesJSON, &t.CustomVariables)
                }

                templates = append(templates, t)
        }

        return templates, total, nil
}
func (r *TemplateRepository) SetDefault(ctx context.Context, id string, category string) error {
        tx, err := r.db.BeginTx(ctx, nil)
        if err != nil {
                return err
        }
        defer tx.Rollback()

        clearQuery := `
                UPDATE templates SET is_default = false, updated_at = $2
                WHERE category = $1`

        if _, err := tx.ExecContext(ctx, clearQuery, category, time.Now()); err != nil {
                return err
        }

        setQuery := `
                UPDATE templates SET is_default = true, updated_at = $2
                WHERE id = $1`

        if _, err := tx.ExecContext(ctx, setQuery, id, time.Now()); err != nil {
                return err
        }

        return tx.Commit()
}

func (r *TemplateRepository) UpdateSpamScore(ctx context.Context, id string, score float64, details map[string]interface{}) error {
        detailsJSON, _ := json.Marshal(details)

        query := `
                UPDATE templates SET
                        spam_score = $2,
                        spam_details = $3,
                        last_spam_check_at = $4,
                        updated_at = $4
                WHERE id = $1`

        _, err := r.db.ExecContext(ctx, query, id, score, detailsJSON, time.Now())
        return err
}

func (r *TemplateRepository) IncrementRenderCount(ctx context.Context, id string) error {
        query := `
                UPDATE templates SET
                        render_count = render_count + 1,
                        last_rendered_at = $2,
                        updated_at = $2
                WHERE id = $1`

        _, err := r.db.ExecContext(ctx, query, id, time.Now())
        return err
}

func (r *TemplateRepository) IncrementUsage(ctx context.Context, id string) error {
        query := `
                UPDATE templates SET
                        usage_count = usage_count + 1,
                        last_used_at = $2,
                        updated_at = $2
                WHERE id = $1`

        _, err := r.db.ExecContext(ctx, query, id, time.Now())
        return err
}

func (r *TemplateRepository) IncrementFailure(ctx context.Context, id string, msg string) error {
        query := `
                UPDATE templates SET
                        failure_count = failure_count + 1,
                        last_failure_at = $2,
                        last_failure_msg = $3,
                        updated_at = $2
                WHERE id = $1`

        _, err := r.db.ExecContext(ctx, query, id, time.Now(), msg)
        return err
}

func (r *TemplateRepository) GetRotationSet(ctx context.Context, group string, limit int) ([]*Template, error) {
        query := `
                SELECT id, name, slug, subject, html_content, text_content,
                        variables, rotation_group, rotation_weight, rotation_index,
                        is_active, is_archived, custom_variables
                FROM templates
                WHERE rotation_group = $1
                        AND is_active = true
                        AND is_archived = false
                ORDER BY rotation_index ASC, rotation_weight DESC, created_at ASC
                LIMIT $2`

        rows, err := r.db.QueryContext(ctx, query, group, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        templates := []*Template{}
        for rows.Next() {
                t := &Template{}
                var customVariablesJSON []byte
                err := rows.Scan(
                        &t.ID, &t.Name, &t.Slug, &t.Subject, &t.HtmlContent, &t.TextContent,
                        pq.Array(&t.Variables), &t.RotationGroup, &t.RotationWeight,
                        &t.RotationIndex, &t.IsActive, &t.IsArchived, &customVariablesJSON,
                )
                if err != nil {
                        return nil, err
                }
                if len(customVariablesJSON) > 0 {
                        json.Unmarshal(customVariablesJSON, &t.CustomVariables)
                }
                templates = append(templates, t)
        }

        return templates, nil
}

func (r *TemplateRepository) BumpRotationIndex(ctx context.Context, id string, index int) error {
        query := `
                UPDATE templates SET
                        rotation_index = $2,
                        updated_at = $3
                WHERE id = $1`

        _, err := r.db.ExecContext(ctx, query, id, index, time.Now())
        return err
}

func (r *TemplateRepository) GetStats(ctx context.Context) (*TemplateStats, error) {
        stats := &TemplateStats{
                CategoryBreakdown:    make(map[string]int),
                TypeBreakdown:        make(map[string]int),
                RotationGroupSummary: make(map[string]int),
        }

        query := `
                SELECT
                        COUNT(*) as total,
                        COUNT(*) FILTER (WHERE is_active = true) as active,
                        COUNT(*) FILTER (WHERE is_archived = true) as archived,
                        COUNT(*) FILTER (WHERE is_default = true) as defaults,
                        COALESCE(AVG(spam_score), 0) as avg_spam,
                        COALESCE(SUM(render_count), 0) as total_render,
                        COALESCE(SUM(usage_count), 0) as total_usage,
                        COALESCE(SUM(failure_count), 0) as total_failure
                FROM templates`

        err := r.db.QueryRowContext(ctx, query).Scan(
                &stats.TotalTemplates,
                &stats.ActiveTemplates,
                &stats.ArchivedTemplates,
                &stats.DefaultTemplates,
                &stats.AverageSpamScore,
                &stats.TotalRenderCount,
                &stats.TotalUsageCount,
                &stats.TotalFailureCount,
        )
        if err != nil {
                return nil, err
        }

        categoryQuery := `
                SELECT category, COUNT(*)
                FROM templates
                GROUP BY category`

        rows, err := r.db.QueryContext(ctx, categoryQuery)
        if err == nil {
                defer rows.Close()
                for rows.Next() {
                        var category string
                        var count int
                        if err := rows.Scan(&category, &count); err == nil {
                                stats.CategoryBreakdown[category] = count
                        }
                }
        }

        typeQuery := `
                SELECT type, COUNT(*)
                FROM templates
                GROUP BY type`

        typeRows, err := r.db.QueryContext(ctx, typeQuery)
        if err == nil {
                defer typeRows.Close()
                for typeRows.Next() {
                        var t string
                        var count int
                        if err := typeRows.Scan(&t, &count); err == nil {
                                stats.TypeBreakdown[t] = count
                        }
                }
        }

        rotationQuery := `
                SELECT rotation_group, COUNT(*)
                FROM templates
                WHERE rotation_group IS NOT NULL AND rotation_group <> ''
                GROUP BY rotation_group`

        rotationRows, err := r.db.QueryContext(ctx, rotationQuery)
        if err == nil {
                defer rotationRows.Close()
                for rotationRows.Next() {
                        var group string
                        var count int
                        if err := rotationRows.Scan(&group, &count); err == nil {
                                stats.RotationGroupSummary[group] = count
                        }
                }
        }

        return stats, nil
}

func (r *TemplateRepository) Duplicate(ctx context.Context, id string, newID string, createdBy string) (*Template, error) {
        t, err := r.GetByID(ctx, id)
        if err != nil {
                return nil, err
        }

        t.ID = newID
        t.Version++
        t.IsDefault = false
        t.IsArchived = false
        t.CreatedBy = createdBy
        t.UpdatedBy = createdBy
        t.CreatedAt = time.Time{}
        t.UpdatedAt = time.Time{}
        t.RenderCount = 0
        t.UsageCount = 0
        t.FailureCount = 0
        t.LastFailureAt = nil
        t.LastFailureMsg = ""
        t.LastRenderedAt = nil
        t.LastUsedAt = nil

        if err := r.Create(ctx, t); err != nil {
                return nil, err
        }

        return t, nil
}
