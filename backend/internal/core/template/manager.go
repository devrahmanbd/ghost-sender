package template

import (
        "context"
        "crypto/sha256"
        "encoding/hex"
        "errors"
        "fmt"
        "io"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "sync"
        "time"
        "email-campaign-system/internal/storage/repository"
        "email-campaign-system/internal/models"
        "email-campaign-system/pkg/logger"
        "github.com/google/uuid"
)

var (
        ErrTemplateNotFound      = errors.New("template not found")
        ErrTemplateExists        = errors.New("template already exists")
        ErrNoTemplatesAvailable  = errors.New("no templates available")
        ErrInvalidTemplate       = errors.New("invalid template")
        ErrTemplateLoadFailed    = errors.New("failed to load template")
        ErrInvalidDirectory      = errors.New("invalid template directory")
        ErrRotationFailed        = errors.New("rotation failed")
)
// Types from handler (copy-paste)
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

// Manager interface for template operations
type Manager interface {
        GetNextTemplate(ctx context.Context) (*models.Template, error)
        Render(ctx context.Context, template *models.Template, data map[string]interface{}) (string, error)
}

type TemplateManager struct {
        templates       map[string]*ManagedTemplate
        templatesByName map[string]*ManagedTemplate
        templateOrder   []string
        mu              sync.RWMutex
        repository      repository.TemplateRepository
        renderer        *Renderer
        parser          *Parser
        validator       *Validator
        spamDetector    *SpamDetector
        cache           *Cache
        rotator         *Rotator
        config          *ManagerConfig
        stats           *ManagerStats
        statsMu         sync.RWMutex
        log             logger.Logger
        basePath        string
}

type ManagedTemplate struct {
        Template     *models.Template
        Content      string
        ParsedVars   []string
        SpamScore    float64
        Health       *TemplateHealth
        Usage        *TemplateUsage
        LastUsed     time.Time
        LastModified time.Time
        Hash         string
        mu           sync.RWMutex
}

type TemplateHealth struct {
        Status           HealthStatus
        Score            float64
        Issues           []string
        LastCheck        time.Time
        ValidationErrors []string
        SpamWarnings     []string
}

type TemplateUsage struct {
        TotalUsed       int64
        SuccessCount    int64
        FailureCount    int64
        LastUsed        time.Time
        AverageRender   time.Duration
        CacheHitRate    float64
}

type ManagerConfig struct {
        BasePath              string
        EnableAutoReload      bool
        EnableCaching         bool
        EnableSpamDetection   bool
        EnableValidation      bool
        ReloadInterval        time.Duration
        CacheExpiry           time.Duration
        MaxTemplateSize       int64
        AllowedExtensions     []string
        SpamScoreThreshold    float64
        MinHealthScore        float64
        RotationStrategy      string
}

type ManagerStats struct {
        TotalTemplates      int64
        ActiveTemplates     int64
        HighSpamTemplates   int64
        TotalRotations      int64
        TotalRenders        int64
        CacheHitRate        float64
        AverageSpamScore    float64
        AverageHealthScore  float64
        LastReload          time.Time
}

type TemplateFilter struct {
        MaxSpamScore   float64
        MinHealthScore float64
        Status         models.TemplateStatus
        Tags           []string
        ExcludeIDs     []string
}

type HealthStatus string

const (
        HealthStatusHealthy   HealthStatus = "healthy"
        HealthStatusWarning   HealthStatus = "warning"
        HealthStatusUnhealthy HealthStatus = "unhealthy"
)

func NewTemplateManager(
        repo repository.TemplateRepository,
        log logger.Logger,
        config *ManagerConfig,
) (*TemplateManager, error) {
        if log == nil {
                return nil, errors.New("logger is required")
        }

        if config == nil {
                config = DefaultManagerConfig()
        }

        manager := &TemplateManager{
                templates:       make(map[string]*ManagedTemplate),
                templatesByName: make(map[string]*ManagedTemplate),
                templateOrder:   make([]string, 0),
                repository:      repo,
                config:          config,
                log:             log,
                stats:           &ManagerStats{},
                basePath:        config.BasePath,
        }

        manager.log.Info("🟢 TemplateManager created", 
                logger.Int("repo", 1), 
                logger.String("path", config.BasePath))  // ADD DEBUG


        if config.EnableValidation {
                manager.validator = NewValidator(log)
        }

        if config.EnableSpamDetection {
                manager.spamDetector = NewSpamDetector(log)
        }

        manager.parser = NewParser(log)
        manager.renderer = NewRenderer(log)

        // Fixed: NewCache signature is (log, config)
        if config.EnableCaching {
                cacheConfig := &CacheConfig{
                        TTL:             config.CacheExpiry,
                        MaxEntries:      1000,
                        CleanupInterval: 5 * time.Minute,
                }
                manager.cache = NewCache(log, cacheConfig)
        }

        manager.rotator = NewRotator(manager, log, config.RotationStrategy)

        if err := manager.loadTemplatesFromRepository(); err != nil {
                return nil, fmt.Errorf("failed to load templates: %w", err)
        }

        if config.EnableAutoReload && config.BasePath != "" {
                go manager.autoReloadWorker()
        }

        return manager, nil
}

func DefaultManagerConfig() *ManagerConfig {
        return &ManagerConfig{
                EnableAutoReload:    false,
                EnableCaching:       true,
                EnableSpamDetection: true,
                EnableValidation:    true,
                ReloadInterval:      5 * time.Minute,
                CacheExpiry:         30 * time.Minute,
                MaxTemplateSize:     10 * 1024 * 1024,
                AllowedExtensions:   []string{".html", ".htm"},
                SpamScoreThreshold:  7.0,
                MinHealthScore:      50.0,
                RotationStrategy:    "sequential",
        }
}
func (m *TemplateManager) generateTemplateID(name string) string {
    return uuid.New().String()
}
// Helper functions to get/set template content
func getTemplateContent(tmpl *models.Template) string {
        if tmpl.HTMLContent != "" {
                return tmpl.HTMLContent
        }
        return tmpl.PlainTextContent
}

func setTemplateContent(tmpl *models.Template, content string) {
        if strings.Contains(content, "<html") || strings.Contains(content, "<body") {
                tmpl.HTMLContent = content
        } else {
                tmpl.PlainTextContent = content
        }
}

// Convert repository.Template to models.Template
func repoToModelTemplate(repoTmpl *repository.Template) *models.Template {
        status := models.TemplateStatusInactive
        if repoTmpl.IsActive {
                status = models.TemplateStatusActive
        }

        var subjects []string
        if repoTmpl.Subject != "" {
                subjects = []string{repoTmpl.Subject}
        }

        return &models.Template{
                ID:               repoTmpl.ID,
                Name:             repoTmpl.Name,
                Description:      repoTmpl.Description,
                HTMLContent:      repoTmpl.HtmlContent,
                PlainTextContent: repoTmpl.TextContent,
                Subjects:         subjects,
                Tags:             repoTmpl.Tags,
                Status:           status,
                SpamScore:        repoTmpl.SpamScore,
                Metadata:         repoTmpl.Metadata,
                CustomVariables:  repoTmpl.CustomVariables,
                CreatedAt:        repoTmpl.CreatedAt,
                UpdatedAt:        repoTmpl.UpdatedAt,
        }
}

func (m *TemplateManager) loadTemplatesFromRepository() error {
        ctx := context.Background()
        
        templates, _, err := m.repository.List(ctx, &repository.TemplateFilter{})
        if err != nil {
                return err
        }

        for _, repoTmpl := range templates {
                modelTmpl := repoToModelTemplate(repoTmpl)
                if err := m.addTemplateInternal(modelTmpl); err != nil {
                        m.log.Error(fmt.Sprintf("failed to add template during load: template_id=%s, name=%s, error=%v",
                                modelTmpl.ID, modelTmpl.Name, err))
                        continue
                }
        }

        m.log.Info(fmt.Sprintf("loaded templates from repository: count=%d", len(templates)))
        return nil
}

func (m *TemplateManager) LoadFromDirectory(dirPath string) error {
        if dirPath == "" {
                return ErrInvalidDirectory
        }

        if _, err := os.Stat(dirPath); os.IsNotExist(err) {
                return fmt.Errorf("%w: %s", ErrInvalidDirectory, dirPath)
        }

        var loaded int
        err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
                if err != nil {
                        return err
                }

                if info.IsDir() {
                        return nil
                }

                ext := strings.ToLower(filepath.Ext(path))
                if !m.isAllowedExtension(ext) {
                        return nil
                }

                if info.Size() > m.config.MaxTemplateSize {
                        m.log.Warn(fmt.Sprintf("template too large, skipping: path=%s, size=%d", path, info.Size()))
                        return nil
                }

                content, err := os.ReadFile(path)
                if err != nil {
                        m.log.Error(fmt.Sprintf("failed to read template file: path=%s, error=%v", path, err))
                        return nil
                }

                name := strings.TrimSuffix(filepath.Base(path), ext)
                tmpl := &models.Template{
                        Name:      name,
                        Status:    models.TemplateStatusActive,
                        CreatedAt: time.Now(),
                        UpdatedAt: time.Now(),
                }
                setTemplateContent(tmpl, string(content))

                if err := m.AddTemplate(context.Background(), tmpl); err != nil {
                        m.log.Error(fmt.Sprintf("failed to add template: name=%s, error=%v", name, err))
                        return nil
                }

                loaded++
                return nil
        })

        if err != nil {
                return fmt.Errorf("failed to walk directory: %w", err)
        }

        m.log.Info(fmt.Sprintf("loaded templates from directory: path=%s, count=%d", dirPath, loaded))

        return nil
}

func (m *TemplateManager) LoadFromZip(zipPath string) error {
        tmpDir, err := os.MkdirTemp("", "templates-*")
        if err != nil {
                return fmt.Errorf("failed to create temp directory: %w", err)
        }
        defer os.RemoveAll(tmpDir)

        if err := m.extractZip(zipPath, tmpDir); err != nil {
                return fmt.Errorf("failed to extract zip: %w", err)
        }

        return m.LoadFromDirectory(tmpDir)
}

func (m *TemplateManager) extractZip(zipPath, destPath string) error {
        // TODO: Implement ZIP extraction
        return nil
}

func (m *TemplateManager) AddTemplate(ctx context.Context, tmpl *models.Template) error {
    if tmpl == nil {
        return errors.New("template cannot be nil")
    }

    if err := m.validateTemplateData(tmpl); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    m.mu.Lock()
    if _, exists := m.templatesByName[tmpl.Name]; exists {
        m.mu.Unlock()
        return ErrTemplateExists
    }
    m.mu.Unlock()

    if tmpl.ID == "" {
        tmpl.ID = m.generateTemplateID(tmpl.Name)
    }

    if m.config.EnableSpamDetection {
        score := m.spamDetector.QuickCheck("", getTemplateContent(tmpl))
        tmpl.SpamScore = score
    }

    isActive := tmpl.Status == models.TemplateStatusActive

    subject := ""
    for _, s := range tmpl.Subjects {
        if s != "" {
            subject = s
            break
        }
    }

    repoTemplate := &repository.Template{
        ID:              tmpl.ID,
        Name:            tmpl.Name,
        Slug:            generateSlug(tmpl.Name),
        Description:     tmpl.Description,
        Subject:         subject,
        HtmlContent:     tmpl.HTMLContent,
        TextContent:     tmpl.PlainTextContent,
        Tags:            tmpl.Tags,
        IsActive:        isActive,
        SpamScore:       tmpl.SpamScore,
        Metadata:        tmpl.Metadata,
        CustomVariables: tmpl.CustomVariables,
        CreatedAt:       tmpl.CreatedAt,
        UpdatedAt:       tmpl.UpdatedAt,
    }

    if err := m.repository.Create(ctx, repoTemplate); err != nil {
        return fmt.Errorf("failed to save template: %w", err)
    }

    if err := m.addTemplateInternal(tmpl); err != nil {
        m.repository.Delete(ctx, tmpl.ID)
        return err
    }

    m.updateStats()
    return nil
}

func (m *TemplateManager) Create(ctx context.Context, req *CreateTemplateReq) (*models.Template, error) {
        subjects := req.Subjects
        if len(subjects) == 0 && req.Subject != "" {
                subjects = []string{req.Subject}
        }
        if len(subjects) == 0 {
                subjects = []string{}
        }
        tmpl := &models.Template{
                ID:              uuid.New().String(),
                Name:            req.Name,
                HTMLContent:     req.Content,
                Description:     req.Description,
                Type:            models.TemplateType(req.Type),
                Subjects:        subjects,
                Tags:            req.Tags,
                Status:          models.TemplateStatusActive,
                SpamScore:       0.2,
                Metadata:        req.Config,
                CustomVariables: req.CustomVariables,
                CreatedAt:       time.Now(),
                UpdatedAt:       time.Now(),
        }

    if err := m.AddTemplate(ctx, tmpl); err != nil {
        return nil, err
    }

    return tmpl, nil
}


// STUB List:
func (m *TemplateManager) List(ctx context.Context, opts *ListTemplateOptions) ([]*models.Template, int, error) {
    templates := m.ListTemplates(nil)  // use existing
    result := make([]*models.Template, len(templates))
    for i, t := range templates {
        result[i] = t.Template
    }
    return result, len(result), nil
}

// STUB GetByID:
func (m *TemplateManager) GetByID(ctx context.Context, id string) (*models.Template, error) {
    managed, err := m.GetTemplate(id)
    if err != nil {
        return nil, err
    }
    return managed.Template, nil
}

// STUB Update:
func (m *TemplateManager) Update(ctx context.Context, id string, req *UpdateTemplateReq) (*models.Template, error) {
    managed, err := m.GetTemplate(id)
    if err != nil {
        return nil, err
    }

    tmpl := managed.Template
    
    if req.Name != nil {
        tmpl.Name = *req.Name
    }
    if req.Description != nil {
        tmpl.Description = *req.Description
    }
    if req.Content != nil {
        tmpl.HTMLContent = *req.Content
    }
    if req.Subject != nil {
        tmpl.Subjects = []string{*req.Subject}
    } else if len(req.Subjects) > 0 {
        tmpl.Subjects = req.Subjects
    }
    if len(req.Tags) > 0 {
        tmpl.Tags = req.Tags
    }
    if req.IsActive != nil {
        if *req.IsActive {
            tmpl.Status = models.TemplateStatusActive
        } else {
            tmpl.Status = models.TemplateStatusInactive
        }
    }
    if req.Config != nil {
        tmpl.Metadata = req.Config
    }
    if len(req.CustomVariables) > 0 {
        tmpl.CustomVariables = req.CustomVariables
    }
    
    tmpl.UpdatedAt = time.Now()

    if err := m.UpdateTemplate(ctx, tmpl); err != nil {
        return nil, err
    }

    return tmpl, nil
}

// STUB Delete:
func (m *TemplateManager) Delete(ctx context.Context, id string) error {
    return m.DeleteTemplate(ctx, id)
}

func (m *TemplateManager) addTemplateInternal(tmpl *models.Template) error {
        content := getTemplateContent(tmpl)
        parsedVars := m.parser.ExtractVariables(content)
        hash := m.calculateHash(content)

        managed := &ManagedTemplate{
                Template:     tmpl,
                Content:      content,
                ParsedVars:   parsedVars,
                SpamScore:    tmpl.SpamScore,
                Hash:         hash,
                LastModified: time.Now(),
                Health: &TemplateHealth{
                        Status:    m.determineHealthStatus(tmpl.SpamScore),
                        Score:     100.0,
                        LastCheck: time.Now(),
                },
                Usage: &TemplateUsage{},
        }

        if m.config.EnableValidation {
                result, err := m.validator.Validate(content)
                if err == nil && result != nil && len(result.Errors) > 0 {
                        // Fixed: Use Code and Message fields
                        validationErrs := make([]string, len(result.Errors))
                        for i, ve := range result.Errors {
                                validationErrs[i] = fmt.Sprintf("[%s] %s", ve.Code, ve.Message)
                        }
                        managed.Health.ValidationErrors = validationErrs
                        managed.Health.Score -= float64(len(result.Errors)) * 5.0
                }
        }

        m.mu.Lock()
        m.templates[tmpl.ID] = managed
        m.templatesByName[tmpl.Name] = managed
        m.templateOrder = append(m.templateOrder, tmpl.ID)
        m.mu.Unlock()

        return nil
}

func (m *TemplateManager) GetTemplate(templateID string) (*ManagedTemplate, error) {
        m.mu.RLock()
        defer m.mu.RUnlock()

        managed, exists := m.templates[templateID]
        if !exists {
                return nil, ErrTemplateNotFound
        }

        return managed, nil
}

func (m *TemplateManager) GetTemplateByName(name string) (*ManagedTemplate, error) {
        m.mu.RLock()
        defer m.mu.RUnlock()

        managed, exists := m.templatesByName[name]
        if !exists {
                return nil, ErrTemplateNotFound
        }

        return managed, nil
}

// GetNextTemplate returns the next template using rotation strategy
func (m *TemplateManager) GetNextTemplate(ctx context.Context) (*models.Template, error) {
        managed, err := m.rotator.Next()
        if err != nil {
                return nil, err
        }
        return managed.Template, nil
}

func (m *TemplateManager) UpdateTemplate(ctx context.Context, tmpl *models.Template) error {
        if tmpl == nil {
                return errors.New("template cannot be nil")
        }

        m.mu.Lock()
        managed, exists := m.templates[tmpl.ID]
        if !exists {
                m.mu.Unlock()
                return ErrTemplateNotFound
        }
        m.mu.Unlock()

        if err := m.validateTemplateData(tmpl); err != nil {
                return fmt.Errorf("validation failed: %w", err)
        }

        // Fixed: Use CalculateScore
        if m.config.EnableSpamDetection {
                tmpl.SpamScore = m.spamDetector.QuickCheck("", getTemplateContent(tmpl))
        }

        tmpl.UpdatedAt = time.Now()

        isActive := tmpl.Status == models.TemplateStatusActive

        subject := ""
        if len(tmpl.Subjects) > 0 {
                subject = tmpl.Subjects[0]
        }

        repoTemplate := &repository.Template{
                ID:              tmpl.ID,
                Name:            tmpl.Name,
                Slug:            generateSlug(tmpl.Name),
                Description:     tmpl.Description,
                Subject:         subject,
                HtmlContent:     tmpl.HTMLContent,
                TextContent:     tmpl.PlainTextContent,
                Tags:            tmpl.Tags,
                IsActive:        isActive,
                SpamScore:       tmpl.SpamScore,
                Metadata:        tmpl.Metadata,
                CustomVariables: tmpl.CustomVariables,
                CreatedAt:       tmpl.CreatedAt,
                UpdatedAt:       tmpl.UpdatedAt,
        }

        if err := m.repository.Update(ctx, repoTemplate); err != nil {
                return fmt.Errorf("failed to update template: %w", err)
        }

        content := getTemplateContent(tmpl)
        managed.mu.Lock()
        oldName := managed.Template.Name
        managed.Template = tmpl
        managed.Content = content
        managed.ParsedVars = m.parser.ExtractVariables(content)
        managed.SpamScore = tmpl.SpamScore
        managed.Hash = m.calculateHash(content)
        managed.LastModified = time.Now()
        managed.mu.Unlock()

        if oldName != tmpl.Name {
                m.mu.Lock()
                delete(m.templatesByName, oldName)
                m.templatesByName[tmpl.Name] = managed
                m.mu.Unlock()
        }

        if m.cache != nil {
                m.cache.Delete(tmpl.ID)
        }

        m.log.Info(fmt.Sprintf("template updated: template_id=%s, name=%s", tmpl.ID, tmpl.Name))

        return nil
}

func (m *TemplateManager) DeleteTemplate(ctx context.Context, templateID string) error {
        m.mu.Lock()
        managed, exists := m.templates[templateID]
        if !exists {
                m.mu.Unlock()
                return ErrTemplateNotFound
        }

        delete(m.templates, templateID)
        delete(m.templatesByName, managed.Template.Name)

        for i, id := range m.templateOrder {
                if id == templateID {
                        m.templateOrder = append(m.templateOrder[:i], m.templateOrder[i+1:]...)
                        break
                }
        }
        m.mu.Unlock()

        if err := m.repository.Delete(ctx, templateID); err != nil {
                m.log.Error(fmt.Sprintf("failed to delete template from repository: template_id=%s, error=%v",
                        templateID, err))
        }

        if m.cache != nil {
                m.cache.Delete(templateID)
        }

        m.updateStats()

        m.log.Info(fmt.Sprintf("template deleted: template_id=%s", templateID))
        return nil
}

func (m *TemplateManager) ListTemplates(filter *TemplateFilter) []*ManagedTemplate {
        m.mu.RLock()
        defer m.mu.RUnlock()

        var result []*ManagedTemplate

        for _, managed := range m.templates {
                if filter != nil {
                        if filter.MaxSpamScore > 0 && managed.SpamScore > filter.MaxSpamScore {
                                continue
                        }

                        if filter.MinHealthScore > 0 && managed.Health.Score < filter.MinHealthScore {
                                continue
                        }

                        if filter.Status != "" && managed.Template.Status != filter.Status {
                                continue
                        }

                        if len(filter.ExcludeIDs) > 0 {
                                excluded := false
                                for _, id := range filter.ExcludeIDs {
                                        if id == managed.Template.ID {
                                                excluded = true
                                                break
                                        }
                                }
                                if excluded {
                                        continue
                                }
                        }
                }

                result = append(result, managed)
        }

        sort.Slice(result, func(i, j int) bool {
                return result[i].Template.Name < result[j].Template.Name
        })

        return result
}

func (m *TemplateManager) GetActiveTemplates() []*ManagedTemplate {
        filter := &TemplateFilter{
                Status:         models.TemplateStatusActive,
                MaxSpamScore:   m.config.SpamScoreThreshold,
                MinHealthScore: m.config.MinHealthScore,
        }

        return m.ListTemplates(filter)
}

func (m *TemplateManager) ValidateTemplate(content string) []string {
        if m.validator == nil {
                return []string{}
        }

        result, err := m.validator.Validate(content)
        if err != nil || result == nil {
                return []string{}
        }

        // Fixed: Use Code and Message
        validationErrs := make([]string, len(result.Errors))
        for i, ve := range result.Errors {
                validationErrs[i] = fmt.Sprintf("[%s] %s", ve.Code, ve.Message)
        }

        return validationErrs
}

func (m *TemplateManager) CheckSpamScore(content string) float64 {
        if m.spamDetector == nil {
                return 0.0
        }
        return m.spamDetector.QuickCheck("", content)
}

func (m *TemplateManager) RenderPreview(templateID string, variables map[string]string) (string, error) {
        managed, err := m.GetTemplate(templateID)
        if err != nil {
                return "", err
        }

        return m.renderer.Render(managed.Content, variables)
}

// Render renders a template with given data
func (m *TemplateManager) Render(ctx context.Context, template *models.Template, data map[string]interface{}) (string, error) {
        // Convert data map to string map
        variables := make(map[string]string)
        for key, value := range data {
                variables[key] = fmt.Sprintf("%v", value)
        }

        content := getTemplateContent(template)
        return m.renderer.Render(content, variables)
}

func (m *TemplateManager) RenderWithCache(templateID string, variables map[string]string) (string, error) {
        managed, err := m.GetTemplate(templateID)
        if err != nil {
                return "", err
        }

        if m.cache != nil {
                cacheKey := m.generateCacheKey(templateID, variables)
                if cached, err := m.cache.Get(cacheKey); err == nil && cached != "" {
                        m.incrementCacheHit(managed)
                        return cached, nil
                }
        }

        startTime := time.Now()
        rendered, err := m.renderer.Render(managed.Content, variables)
        renderDuration := time.Since(startTime)

        if err != nil {
                m.incrementRenderFailed(managed)
                return "", err
        }

        if m.cache != nil {
                cacheKey := m.generateCacheKey(templateID, variables)
                m.cache.Set(cacheKey, rendered, make(map[string]interface{}))
        }

        m.incrementRenderSuccess(managed, renderDuration)

        managed.mu.Lock()
        managed.LastUsed = time.Now()
        managed.mu.Unlock()

        return rendered, nil
}

func (m *TemplateManager) ParseVariables(content string) []string {
        return m.parser.ExtractVariables(content)
}

func (m *TemplateManager) GetTemplateStats(templateID string) (*TemplateUsage, error) {
        managed, err := m.GetTemplate(templateID)
        if err != nil {
                return nil, err
        }

        managed.mu.RLock()
        defer managed.mu.RUnlock()

        stats := *managed.Usage
        return &stats, nil
}

func (m *TemplateManager) GetRotationStats() map[string]interface{} {
        return m.rotator.GetStats()
}

func (m *TemplateManager) ReloadTemplates() error {
        if m.basePath == "" {
                return errors.New("base path not configured")
        }

        m.log.Info(fmt.Sprintf("reloading templates from disk: path=%s", m.basePath))

        return m.LoadFromDirectory(m.basePath)
}

func (m *TemplateManager) ClearCache() {
        if m.cache != nil {
                m.cache.Clear()
                m.log.Info("template cache cleared")
        }
}

func (m *TemplateManager) GetStats() ManagerStats {
        m.statsMu.RLock()
        defer m.statsMu.RUnlock()

        return *m.stats
}

func (m *TemplateManager) Close() error {
        m.log.Info("closing template manager")

        if m.cache != nil {
                m.cache.Clear()
        }

        m.mu.Lock()
        m.templates = make(map[string]*ManagedTemplate)
        m.templatesByName = make(map[string]*ManagedTemplate)
        m.templateOrder = make([]string, 0)
        m.mu.Unlock()

        m.log.Info("template manager closed")
        return nil
}
func (m *TemplateManager) validateTemplateData(tmpl *models.Template) error {
    m.log.Info("DEBUG validateTemplateData called",
        logger.String("name", tmpl.Name),
        logger.String("htmlcontent_len", fmt.Sprintf("%d", len(tmpl.HTMLContent))),
        logger.String("plaintextcontent_len", fmt.Sprintf("%d", len(tmpl.PlainTextContent))),
    )
    if tmpl.Name == "" {
        m.log.Info("DEBUG validateTemplateData: name is empty")
        return errors.New("template name is required")
    }
    content := getTemplateContent(tmpl)
    m.log.Info("DEBUG validateTemplateData: content", logger.String("content_len", fmt.Sprintf("%d", len(content))))
    if content == "" {
        m.log.Info("DEBUG validateTemplateData: content is empty")
        return errors.New("template content is required")
    }
    // ...
    if m.config.EnableValidation {
        result, err := m.validator.Validate(content)
        m.log.Info("DEBUG validateTemplateData: validator result",
            logger.String("err", fmt.Sprintf("%v", err)),
            logger.String("error_count", fmt.Sprintf("%d", len(result.Errors))),
        )
        if err == nil && result != nil && len(result.Errors) > 10 {
            m.log.Info("DEBUG validateTemplateData: TOO MANY VALIDATION ERRORS — returning error")
            return fmt.Errorf("template has too many validation errors: %d", len(result.Errors))
        }
    }
    return nil
}


func (m *TemplateManager) calculateHash(content string) string {
        hash := sha256.Sum256([]byte(content))
        return hex.EncodeToString(hash[:])
}


func (m *TemplateManager) generateCacheKey(templateID string, variables map[string]string) string {
        keys := make([]string, 0, len(variables))
        for k := range variables {
                keys = append(keys, k)
        }
        sort.Strings(keys)

        var builder strings.Builder
        builder.WriteString(templateID)
        for _, k := range keys {
                builder.WriteString("-")
                builder.WriteString(k)
                builder.WriteString(":")
                builder.WriteString(variables[k])
        }

        hash := sha256.Sum256([]byte(builder.String()))
        return hex.EncodeToString(hash[:])
}

func (m *TemplateManager) isAllowedExtension(ext string) bool {
        for _, allowed := range m.config.AllowedExtensions {
                if ext == allowed {
                        return true
                }
        }
        return false
}

func (m *TemplateManager) determineHealthStatus(spamScore float64) HealthStatus {
        if spamScore >= m.config.SpamScoreThreshold {
                return HealthStatusUnhealthy
        } else if spamScore >= m.config.SpamScoreThreshold*0.7 {
                return HealthStatusWarning
        }
        return HealthStatusHealthy
}

func (m *TemplateManager) incrementRenderSuccess(managed *ManagedTemplate, duration time.Duration) {
        managed.mu.Lock()
        defer managed.mu.Unlock()

        managed.Usage.TotalUsed++
        managed.Usage.SuccessCount++
        managed.Usage.LastUsed = time.Now()

        if managed.Usage.AverageRender == 0 {
                managed.Usage.AverageRender = duration
        } else {
                managed.Usage.AverageRender = (managed.Usage.AverageRender + duration) / 2
        }
}

func (m *TemplateManager) incrementRenderFailed(managed *ManagedTemplate) {
        managed.mu.Lock()
        defer managed.mu.Unlock()

        managed.Usage.FailureCount++
}

func (m *TemplateManager) incrementCacheHit(managed *ManagedTemplate) {
        managed.mu.Lock()
        defer managed.mu.Unlock()

        total := managed.Usage.TotalUsed + 1
        hits := managed.Usage.CacheHitRate*float64(managed.Usage.TotalUsed) + 1

        managed.Usage.CacheHitRate = hits / float64(total)
}

func (m *TemplateManager) updateStats() {
        m.statsMu.Lock()
        defer m.statsMu.Unlock()

        m.mu.RLock()
        defer m.mu.RUnlock()

        m.stats.TotalTemplates = int64(len(m.templates))

        var activeCount, highSpamCount int64
        var totalSpamScore, totalHealthScore float64
        var totalRenders int64

        for _, managed := range m.templates {
                if managed.Template.Status == models.TemplateStatusActive {
                        activeCount++
                }

                if managed.SpamScore >= m.config.SpamScoreThreshold {
                        highSpamCount++
                }

                totalSpamScore += managed.SpamScore
                totalHealthScore += managed.Health.Score
                totalRenders += managed.Usage.TotalUsed
        }

        m.stats.ActiveTemplates = activeCount
        m.stats.HighSpamTemplates = highSpamCount
        m.stats.TotalRenders = totalRenders

        if m.stats.TotalTemplates > 0 {
                m.stats.AverageSpamScore = totalSpamScore / float64(m.stats.TotalTemplates)
                m.stats.AverageHealthScore = totalHealthScore / float64(m.stats.TotalTemplates)
        }
}

func (m *TemplateManager) autoReloadWorker() {
        ticker := time.NewTicker(m.config.ReloadInterval)
        defer ticker.Stop()

        for range ticker.C {
                if err := m.ReloadTemplates(); err != nil {
                        m.log.Error(fmt.Sprintf("auto-reload failed: error=%v", err))
                } else {
                        m.statsMu.Lock()
                        m.stats.LastReload = time.Now()
                        m.statsMu.Unlock()
                }
        }
}

func (mt *ManagedTemplate) IsHealthy() bool {
        mt.mu.RLock()
        defer mt.mu.RUnlock()

        return mt.Health.Status == HealthStatusHealthy
}

func (mt *ManagedTemplate) GetSpamScore() float64 {
        mt.mu.RLock()
        defer mt.mu.RUnlock()

        return mt.SpamScore
}

func (mt *ManagedTemplate) GetUsageStats() TemplateUsage {
        mt.mu.RLock()
        defer mt.mu.RUnlock()

        return *mt.Usage
}

func (m *TemplateManager) ExportTemplate(templateID string, writer io.Writer) error {
        managed, err := m.GetTemplate(templateID)
        if err != nil {
                return err
        }

        _, err = writer.Write([]byte(managed.Content))
        return err
}

func (m *TemplateManager) ImportTemplate(name string, reader io.Reader) error {
        content, err := io.ReadAll(reader)
        if err != nil {
                return fmt.Errorf("failed to read template: %w", err)
        }

        tmpl := &models.Template{
                Name:      name,
                Status:    models.TemplateStatusActive,
                CreatedAt: time.Now(),
                UpdatedAt: time.Now(),
        }
        setTemplateContent(tmpl, string(content))

        return m.AddTemplate(context.Background(), tmpl)
}

func (m *TemplateManager) DuplicateTemplate(templateID, newName string) error {
        managed, err := m.GetTemplate(templateID)
        if err != nil {
                return err
        }

        newTemplate := &models.Template{
                Name:      newName,
                Status:    models.TemplateStatusActive,
                CreatedAt: time.Now(),
                UpdatedAt: time.Now(),
        }
        setTemplateContent(newTemplate, managed.Content)

        return m.AddTemplate(context.Background(), newTemplate)
}

func (m *TemplateManager) GetTemplateCount() int {
        m.mu.RLock()
        defer m.mu.RUnlock()

        return len(m.templates)
}

func (m *TemplateManager) GetActiveTemplateCount() int {
        m.mu.RLock()
        defer m.mu.RUnlock()

        count := 0
        for _, managed := range m.templates {
                if managed.Template.Status == models.TemplateStatusActive {
                        count++
                }
        }

        return count
}
func generateSlug(name string) string {
    slug := strings.ToLower(name)
    slug = strings.ReplaceAll(slug, " ", "-")
    // remove any character that isn't alphanumeric or hyphen
    var b strings.Builder
    for _, r := range slug {
        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
            b.WriteRune(r)
        }
    }
    return b.String()
}
