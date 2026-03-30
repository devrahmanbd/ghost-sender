package attachment

import (
        "context"
        "crypto/sha256"
        "encoding/hex"
        "encoding/json"
        "errors"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "sync"
        "sync/atomic"
        "time"

        "email-campaign-system/internal/models"
)

var (
        ErrManagerClosed       = errors.New("attachment: manager closed")
        ErrTemplateNotFound    = errors.New("attachment: template not found")
        ErrInvalidFormat       = errors.New("attachment: invalid format")
        ErrGenerationFailed    = errors.New("attachment: generation failed")
        ErrNoTemplates         = errors.New("attachment: no templates available")
        ErrConverterNotSet     = errors.New("attachment: converter not set")
        ErrCacheNotSet         = errors.New("attachment: cache not set")
)

type Format string

const (
        FormatPDF  Format = "pdf"
        FormatJPG  Format = "jpg"
        FormatPNG  Format = "png"
        FormatWebP Format = "webp"
        FormatHEIC Format = "heic"
        FormatHEIF Format = "heif"
        FormatHTML Format = "html"
)

type Manager struct {
        mu sync.RWMutex

        config    *Config
        templates map[string]*Template
        converter Converter
        cache     Cache
        rotator   *FormatRotator

        closed   bool
        closeCh  chan struct{}
        wg       sync.WaitGroup
        workerCh chan *generateRequest

        stats *Stats

        // rowCounters tracks a per-template row index that advances once per
        // Prepare() call for that template.  Every custom variable in the
        // template reads values[rowIdx % len(values)] so they all rotate in
        // lockstep instead of each picking independently via rand.
        rowCounters   map[string]*uint64
        rowCountersMu sync.Mutex
}

type Config struct {
        TemplateDir      string
        CacheDir         string
        DefaultFormat    Format
        EnableRotation   bool
        EnableCache      bool
        MaxWorkers       int
        WorkerQueueSize  int
        GenerationTimeout time.Duration
        MaxFileSize      int64
        SupportedFormats []Format
}

type Template struct {
        ID              string
        Name            string
        Path            string
        HTMLContent     string
        Size            int64
        ModTime         time.Time
        Variables       []string
        Formats         []string
        Filename        string
        CustomVariables map[string][]string
}

type templateMeta struct {
        Formats         []string                       `json:"formats"`
        CustomVariables map[string][]string            `json:"custom_variables"`
        Filename        string                         `json:"filename"`
}

type GenerateRequest struct {
        TemplateID string
        CampaignID string
        Format     Format
        PersonData map[string]string
        Context    context.Context
}

type GenerateResult struct {
        TemplateID   string
        Format       Format
        Data         []byte
        Size         int64
        Hash         string
        Cached       bool
        GeneratedAt  time.Time
        Duration     time.Duration
}

type generateRequest struct {
        req    *GenerateRequest
        result chan *GenerateResult
        err    chan error
}

type Stats struct {
        mu                 sync.RWMutex
        totalGenerated     uint64
        cacheHits          uint64
        cacheMisses        uint64
        totalSize          uint64
        totalDuration      time.Duration
        failedGenerations  uint64
        formatCounts       map[Format]uint64
        templateCounts     map[string]uint64
}

func NewManager(cfg *Config) (*Manager, error) {
        if cfg == nil {
                return nil, errors.New("attachment: config required")
        }

        if cfg.TemplateDir == "" {
                return nil, errors.New("attachment: template directory required")
        }

        if cfg.MaxWorkers <= 0 {
                cfg.MaxWorkers = 4
        }
        if cfg.WorkerQueueSize <= 0 {
                cfg.WorkerQueueSize = 100
        }
        if cfg.GenerationTimeout == 0 {
                cfg.GenerationTimeout = 30 * time.Second
        }
        if cfg.MaxFileSize == 0 {
                cfg.MaxFileSize = 10 * 1024 * 1024
        }

        if len(cfg.SupportedFormats) == 0 {
                cfg.SupportedFormats = []Format{FormatPDF, FormatJPG, FormatPNG, FormatWebP, FormatHEIC, FormatHEIF}
        }

        m := &Manager{
                config:      cfg,
                templates:   make(map[string]*Template),
                closeCh:     make(chan struct{}),
                workerCh:    make(chan *generateRequest, cfg.WorkerQueueSize),
                rowCounters: make(map[string]*uint64),
                stats: &Stats{
                        formatCounts:   make(map[Format]uint64),
                        templateCounts: make(map[string]uint64),
                },
        }

        if cfg.EnableRotation {
                m.rotator = NewFormatRotator(cfg.SupportedFormats)
        }

        for i := 0; i < cfg.MaxWorkers; i++ {
                m.wg.Add(1)
                go m.worker()
        }

        return m, nil
}

func (m *Manager) SetConverter(conv Converter) {
        m.mu.Lock()
        defer m.mu.Unlock()
        m.converter = conv
}

func (m *Manager) SetCache(cache Cache) {
        m.mu.Lock()
        defer m.mu.Unlock()
        m.cache = cache
}

func (m *Manager) LoadTemplates() error {
        m.mu.Lock()
        defer m.mu.Unlock()

        if m.closed {
                return ErrManagerClosed
        }

        templates := make(map[string]*Template)

        err := filepath.Walk(m.config.TemplateDir, func(path string, info os.FileInfo, err error) error {
                if err != nil {
                        return err
                }

                if info.IsDir() {
                        return nil
                }

                ext := strings.ToLower(filepath.Ext(path))
                if ext != ".html" && ext != ".htm" {
                        return nil
                }

                content, err := os.ReadFile(path)
                if err != nil {
                        return fmt.Errorf("read template %s: %w", path, err)
                }

                relPath, _ := filepath.Rel(m.config.TemplateDir, path)
                id := strings.TrimSuffix(relPath, ext)
                id = strings.ReplaceAll(id, string(os.PathSeparator), "/")

                tmpl := &Template{
                        ID:          id,
                        Name:        filepath.Base(path),
                        Path:        path,
                        HTMLContent: string(content),
                        Size:        info.Size(),
                        ModTime:     info.ModTime(),
                        Variables:   extractVariables(string(content)),
                        Formats:     []string{"pdf"},
                }

                metaPath := strings.TrimSuffix(path, ext) + ".meta.json"
                if metaData, err := os.ReadFile(metaPath); err == nil {
                        var meta templateMeta
                        if err := json.Unmarshal(metaData, &meta); err == nil {
                                if len(meta.Formats) > 0 {
                                        tmpl.Formats = meta.Formats
                                }
                                tmpl.Filename = meta.Filename
                                if len(meta.CustomVariables) > 0 {
                                        tmpl.CustomVariables = meta.CustomVariables
                                }
                                fmt.Printf("[ATTACH-LOAD] template=%s formats=%v filename=%q custom_vars=%v\n", id, tmpl.Formats, tmpl.Filename, func() []string {
                                        keys := make([]string, 0, len(meta.CustomVariables))
                                        for k := range meta.CustomVariables {
                                                keys = append(keys, k)
                                        }
                                        return keys
                                }())
                        }
                }

                templates[id] = tmpl
                return nil
        })

        if err != nil {
                return fmt.Errorf("load templates: %w", err)
        }

        if len(templates) == 0 {
                return ErrNoTemplates
        }

        m.templates = templates
        return nil
}

func (m *Manager) Generate(req *GenerateRequest) (*GenerateResult, error) {
        if req == nil {
                return nil, errors.New("attachment: request required")
        }

        m.mu.RLock()
        closed := m.closed
        m.mu.RUnlock()

        if closed {
                return nil, ErrManagerClosed
        }

        if req.Format == "" {
                req.Format = m.config.DefaultFormat
                if req.Format == "" {
                        req.Format = FormatPDF
                }
        }

        if !m.isFormatSupported(req.Format) {
                return nil, ErrInvalidFormat
        }

        if req.Context == nil {
                ctx, cancel := context.WithTimeout(context.Background(), m.config.GenerationTimeout)
                defer cancel()
                req.Context = ctx
        }

        if m.config.EnableCache && m.cache != nil {
                cacheKey := m.buildCacheKey(req)
                if cached, err := m.cache.Get(req.Context, cacheKey); err == nil && cached != nil {
                        atomic.AddUint64(&m.stats.cacheHits, 1)
                        return &GenerateResult{
                                TemplateID:  req.TemplateID,
                                Format:      req.Format,
                                Data:        cached.Data,
                                Size:        cached.Size,
                                Hash:        cached.Hash,
                                Cached:      true,
                                GeneratedAt: cached.GeneratedAt,
                        }, nil
                }
                atomic.AddUint64(&m.stats.cacheMisses, 1)
        }

        genReq := &generateRequest{
                req:    req,
                result: make(chan *GenerateResult, 1),
                err:    make(chan error, 1),
        }

        select {
        case m.workerCh <- genReq:
        case <-req.Context.Done():
                return nil, req.Context.Err()
        case <-m.closeCh:
                return nil, ErrManagerClosed
        }

        select {
        case result := <-genReq.result:
                return result, nil
        case err := <-genReq.err:
                return nil, err
        case <-req.Context.Done():
                return nil, req.Context.Err()
        case <-m.closeCh:
                return nil, ErrManagerClosed
        }
}

func (m *Manager) GenerateWithRotation(req *GenerateRequest) (*GenerateResult, error) {
        if !m.config.EnableRotation || m.rotator == nil {
                return m.Generate(req)
        }

        format := m.rotator.Next()
        req.Format = format
        return m.Generate(req)
}

func (m *Manager) worker() {
        defer m.wg.Done()

        for {
                select {
                case req := <-m.workerCh:
                        result, err := m.doGenerate(req.req)
                        if err != nil {
                                select {
                                case req.err <- err:
                                default:
                                }
                        } else {
                                select {
                                case req.result <- result:
                                default:
                                }
                        }

                case <-m.closeCh:
                        return
                }
        }
}

func (m *Manager) doGenerate(req *GenerateRequest) (*GenerateResult, error) {
        startTime := time.Now()

        m.mu.RLock()
        tmpl, exists := m.templates[req.TemplateID]
        conv := m.converter
        m.mu.RUnlock()

        if !exists {
                atomic.AddUint64(&m.stats.failedGenerations, 1)
                return nil, ErrTemplateNotFound
        }

        if conv == nil {
                atomic.AddUint64(&m.stats.failedGenerations, 1)
                return nil, ErrConverterNotSet
        }

        html := personalizeHTML(tmpl.HTMLContent, req.PersonData)

        convReq := &ConversionRequest{
                HTML:    html,
                Format:  req.Format,
                Context: req.Context,
        }

        data, err := conv.Convert(convReq)
        if err != nil {
                fmt.Printf("[ATTACH-CONVERT] ERROR converting template=%s format=%s: %v\n", req.TemplateID, req.Format, err)
                atomic.AddUint64(&m.stats.failedGenerations, 1)
                return nil, fmt.Errorf("conversion failed for %s (format=%s): %w", req.TemplateID, req.Format, err)
        }
        fmt.Printf("[ATTACH-CONVERT] SUCCESS template=%s format=%s size=%d bytes\n", req.TemplateID, req.Format, len(data))

        if m.config.MaxFileSize > 0 && int64(len(data)) > m.config.MaxFileSize {
                atomic.AddUint64(&m.stats.failedGenerations, 1)
                return nil, fmt.Errorf("attachment: exceeds max size %d", m.config.MaxFileSize)
        }

        hash := computeHash(data)
        duration := time.Since(startTime)

        result := &GenerateResult{
                TemplateID:  req.TemplateID,
                Format:      req.Format,
                Data:        data,
                Size:        int64(len(data)),
                Hash:        hash,
                Cached:      false,
                GeneratedAt: time.Now(),
                Duration:    duration,
        }

        if m.config.EnableCache && m.cache != nil {
                cacheKey := m.buildCacheKey(req)
                cacheEntry := &CacheEntry{
                        Key:         cacheKey,
                        Data:        data,
                        Size:        result.Size,
                        Hash:        hash,
                        TemplateID:  req.TemplateID,
                        Format:      req.Format,
                        GeneratedAt: result.GeneratedAt,
                }
                _ = m.cache.Set(req.Context, cacheKey, cacheEntry)
        }

        m.updateStats(result)
        return result, nil
}

func (m *Manager) buildCacheKey(req *GenerateRequest) string {
        h := sha256.New()
        h.Write([]byte(req.TemplateID))
        h.Write([]byte(string(req.Format)))
        h.Write([]byte(req.CampaignID))

        keys := make([]string, 0, len(req.PersonData))
        for k := range req.PersonData {
                keys = append(keys, k)
        }
        sort.Strings(keys)

        for _, k := range keys {
                h.Write([]byte(k))
                h.Write([]byte("="))
                h.Write([]byte(req.PersonData[k]))
                h.Write([]byte("\x00"))
        }

        return hex.EncodeToString(h.Sum(nil))
}

func (m *Manager) updateStats(result *GenerateResult) {
        atomic.AddUint64(&m.stats.totalGenerated, 1)
        atomic.AddUint64(&m.stats.totalSize, uint64(result.Size))

        m.stats.mu.Lock()
        m.stats.totalDuration += result.Duration
        m.stats.formatCounts[result.Format]++
        m.stats.templateCounts[result.TemplateID]++
        m.stats.mu.Unlock()
}

func (m *Manager) GetTemplate(id string) (*Template, error) {
        m.mu.RLock()
        defer m.mu.RUnlock()

        tmpl, exists := m.templates[id]
        if !exists {
                return nil, ErrTemplateNotFound
        }

        return tmpl, nil
}

func (m *Manager) GetTemplates() []*Template {
        m.mu.RLock()
        defer m.mu.RUnlock()

        templates := make([]*Template, 0, len(m.templates))
        for _, tmpl := range m.templates {
                templates = append(templates, tmpl)
        }
        return templates
}

func (m *Manager) InvalidateCache(templateID string) error {
        if !m.config.EnableCache || m.cache == nil {
                return nil
        }

        return m.cache.Invalidate(context.Background(), templateID)
}

func (m *Manager) ClearCache() error {
        if !m.config.EnableCache || m.cache == nil {
                return nil
        }

        return m.cache.Clear(context.Background())
}

func (m *Manager) GetStats() *StatsSnapshot {
        m.stats.mu.RLock()
        defer m.stats.mu.RUnlock()

        snapshot := &StatsSnapshot{
                TotalGenerated:    atomic.LoadUint64(&m.stats.totalGenerated),
                CacheHits:         atomic.LoadUint64(&m.stats.cacheHits),
                CacheMisses:       atomic.LoadUint64(&m.stats.cacheMisses),
                TotalSize:         atomic.LoadUint64(&m.stats.totalSize),
                TotalDuration:     m.stats.totalDuration,
                FailedGenerations: atomic.LoadUint64(&m.stats.failedGenerations),
                FormatCounts:      make(map[Format]uint64),
                TemplateCounts:    make(map[string]uint64),
        }

        for format, count := range m.stats.formatCounts {
                snapshot.FormatCounts[format] = count
        }

        for tmpl, count := range m.stats.templateCounts {
                snapshot.TemplateCounts[tmpl] = count
        }

        total := snapshot.TotalGenerated
        if total > 0 {
                snapshot.CacheHitRate = float64(snapshot.CacheHits) / float64(total) * 100
                snapshot.AvgDuration = m.stats.totalDuration / time.Duration(total)
                snapshot.AvgSize = snapshot.TotalSize / total
        }

        return snapshot
}

func (m *Manager) Close() error {
        m.mu.Lock()
        if m.closed {
                m.mu.Unlock()
                return nil
        }
        m.closed = true
        close(m.closeCh)
        m.mu.Unlock()

        m.wg.Wait()

        if m.cache != nil {
                _ = m.cache.Close()
        }

        if m.converter != nil {
                _ = m.converter.Close()
        }

        return nil
}

func (m *Manager) isFormatSupported(format Format) bool {
        for _, f := range m.config.SupportedFormats {
                if f == format {
                        return true
                }
        }
        return false
}

func extractVariables(html string) []string {
        vars := make(map[string]bool)
        content := html

        for {
                start := strings.Index(content, "{{")
                if start == -1 {
                        break
                }
                end := strings.Index(content[start:], "}}")
                if end == -1 {
                        break
                }
                end += start

                varName := strings.TrimSpace(content[start+2 : end])
                if varName != "" {
                        vars[varName] = true
                }

                content = content[end+2:]
        }

        result := make([]string, 0, len(vars))
        for v := range vars {
                result = append(result, v)
        }
        return result
}

func personalizeHTML(htmlContent string, data map[string]string) string {
        result := htmlContent
        for key, value := range data {
                if strings.HasPrefix(key, "_") {
                        continue
                }
                result = strings.ReplaceAll(result, fmt.Sprintf("{%s}", key), value)
                result = strings.ReplaceAll(result, fmt.Sprintf("{{%s}}", key), value)
        }
        return result
}

func computeHash(data []byte) string {
        h := sha256.Sum256(data)
        return hex.EncodeToString(h[:])
}

type StatsSnapshot struct {
        TotalGenerated    uint64
        CacheHits         uint64
        CacheMisses       uint64
        CacheHitRate      float64
        TotalSize         uint64
        AvgSize           uint64
        TotalDuration     time.Duration
        AvgDuration       time.Duration
        FailedGenerations uint64
        FormatCounts      map[Format]uint64
        TemplateCounts    map[string]uint64
}

type Converter interface {
        Convert(req *ConversionRequest) ([]byte, error)
        Close() error
}

type ConversionRequest struct {
        HTML    string
        Format  Format
        Context context.Context
}

type Cache interface {
        Get(ctx context.Context, key string) (*CacheEntry, error)
        Set(ctx context.Context, key string, entry *CacheEntry) error
        Invalidate(ctx context.Context, templateID string) error
        Clear(ctx context.Context) error
        Close() error
}

type CacheEntry struct {
        Key         string
        Data        []byte
        Size        int64
        Hash        string
        TemplateID  string
        Format      Format
        GeneratedAt time.Time
        AccessCount uint64
        LastAccess  time.Time
}
func (m *Manager) RefreshTemplates() error {
        return m.LoadTemplates()
}

// getNextTemplateRow returns the next row index for the given attachment
// template and atomically increments the counter.  Calling this once before
// spawning the goroutine ensures all custom variables in that template read
// the same row and rotate in lockstep.
func (m *Manager) getNextTemplateRow(templateID string) int {
        m.rowCountersMu.Lock()
        defer m.rowCountersMu.Unlock()
        counter, exists := m.rowCounters[templateID]
        if !exists {
                var initial uint64
                m.rowCounters[templateID] = &initial
                counter = &initial
        }
        row := int(*counter)
        *counter++
        return row
}

func (m *Manager) Prepare(ctx context.Context, recipient interface{}, data map[string]interface{}, templateIDs []string, campaignID string) ([]interface{}, error) {
        if recipient == nil {
                return nil, errors.New("attachment: recipient required")
        }

        m.mu.RLock()
        closed := m.closed
        allTemplates := m.templates
        m.mu.RUnlock()

        if closed {
                return nil, ErrManagerClosed
        }

        if len(allTemplates) == 0 {
                return nil, ErrNoTemplates
        }

        selected := make(map[string]*Template)
        if len(templateIDs) > 0 {
                for _, id := range templateIDs {
                        if tmpl, ok := allTemplates[id]; ok {
                                selected[id] = tmpl
                        }
                }
        } else {
                for k, v := range allTemplates {
                        selected[k] = v
                }
        }

        if len(selected) == 0 {
                return nil, ErrNoTemplates
        }

        // emailOnlyKeys holds keys whose values come from the email-template
        // personalization pass and must not leak into attachment rendering.
        // "variables" contains the email template's already-resolved custom
        // variable values; including them would let Template1's choices bleed
        // into attachment templates that may define the same variable names.
        emailOnlyKeys := map[string]bool{
                "variables":      true,
                "body":           true,
                "subject":        true,
                "sender_name":    true,
                "from_cache":     true,
                "processing_time": true,
                "processed_count": true,
                "failed_variables": true,
                "template_id":    true,
                "template_name":  true,
        }

        personData := make(map[string]string)
        for key, value := range data {
                if strings.HasPrefix(key, "_") {
                        continue
                }
                if emailOnlyKeys[key] {
                        continue
                }
                switch v := value.(type) {
                case string:
                        personData[key] = v
                case map[string]string:
                        for k, val := range v {
                                personData[k] = val
                        }
                case map[string]interface{}:
                        for k, val := range v {
                                if s, ok := val.(string); ok {
                                        personData[k] = s
                                } else {
                                        personData[k] = fmt.Sprintf("%v", val)
                                }
                        }
                default:
                        personData[key] = fmt.Sprintf("%v", value)
                }
        }
        if r, ok := recipient.(*models.Recipient); ok && r != nil {
                if r.Email != "" {
                        personData["Recipient_Email"] = r.Email
                        personData["Email"] = r.Email
                }
                if r.FirstName != "" {
                        personData["First_Name"] = r.FirstName
                }
                if r.LastName != "" {
                        personData["Last_Name"] = r.LastName
                }
                if r.FullName != "" {
                        personData["Full_Name"] = r.FullName
                        personData["Recipient_Name"] = r.FullName
                }
                for k, v := range r.CustomFields {
                        if v != "" {
                                personData[k] = v
                        }
                }
        }
        fmt.Printf("[ATTACH-PERSON] personData keys=%v\n", func() []string {
                keys := make([]string, 0, len(personData))
                for k := range personData {
                        keys = append(keys, k)
                }
                sort.Strings(keys)
                return keys
        }())

        // Extract the format override from the raw data map before launching
        // goroutines. The key starts with "_" so it was intentionally skipped
        // from personData; read it directly here.
        overrideFormatStr := ""
        if v, ok := data["_attachment_format"]; ok {
                if s, ok := v.(string); ok {
                        overrideFormatStr = s
                }
        }

        // Pre-compute one row index per template (sequentially, before any
        // goroutine runs) so the counter advances deterministically and every
        // custom variable in a given template reads values[rowIdx % len].
        type tmplWork struct {
                template *Template
                rowIdx   int
        }
        works := make([]tmplWork, 0, len(selected))
        for _, tmpl := range selected {
                works = append(works, tmplWork{
                        template: tmpl,
                        rowIdx:   m.getNextTemplateRow(tmpl.ID),
                })
        }

        var results []interface{}
        var mu sync.Mutex
        var wg sync.WaitGroup
        errChan := make(chan error, len(works))

        for _, w := range works {
                wg.Add(1)
                go func(tw tmplWork) {
                        defer wg.Done()
                        template := tw.template
                        rowIdx := tw.rowIdx

                        format := m.config.DefaultFormat
                        if format == "" {
                                format = FormatPDF
                        }
                        // Priority 1: explicit format override supplied by the engine (slot rotation)
                        if overrideStr := overrideFormatStr; overrideStr != "" {
                                f := Format(strings.ToLower(overrideStr))
                                if f == "jpeg" {
                                        f = FormatJPG
                                }
                                if m.isFormatSupported(f) {
                                        format = f
                                        fmt.Printf("[ATTACH-FORMAT] template=%s using override format=%q\n", template.ID, format)
                                }
                        } else if len(template.Formats) > 0 {
                                // Priority 2: first format in template metadata
                                requested := Format(strings.ToLower(template.Formats[0]))
                                if requested == "jpeg" {
                                        requested = FormatJPG
                                }
                                if m.isFormatSupported(requested) {
                                        format = requested
                                }
                                fmt.Printf("[ATTACH-FORMAT] template=%s using format=%q from template meta (available=%v)\n", template.ID, format, template.Formats)
                        } else {
                                fmt.Printf("[ATTACH-FORMAT] template=%s no formats in meta, using default=%q\n", template.ID, format)
                        }

                        // Make a per-template copy of personData so custom variables
                        // from one template don't bleed into another template's render.
                        tmplPersonData := make(map[string]string, len(personData))
                        for k, v := range personData {
                                tmplPersonData[k] = v
                        }
                        // Inject this template's own custom variables using the
                        // pre-computed row index so all variables rotate in lockstep.
                        if len(template.CustomVariables) > 0 {
                                for varName, values := range template.CustomVariables {
                                        if len(values) > 0 {
                                                tmplPersonData[varName] = values[rowIdx%len(values)]
                                        }
                                }
                                fmt.Printf("[ATTACH-CUSTOMVAR] template=%s injected vars=%v\n", template.ID, func() []string {
                                        keys := make([]string, 0, len(template.CustomVariables))
                                        for k := range template.CustomVariables {
                                                keys = append(keys, k)
                                        }
                                        return keys
                                }())
                        }

                        req := &GenerateRequest{
                                TemplateID: template.ID,
                                CampaignID: campaignID,
                                Format:     format,
                                PersonData: tmplPersonData,
                                Context:    ctx,
                        }

                        result, err := m.Generate(req)
                        if err != nil {
                                errChan <- fmt.Errorf("template %s: %w", template.ID, err)
                                return
                        }

                        ext := GetFileExtension(result.Format)
                        mimeType := GetMIMEType(result.Format)
                        filename := template.ID + ext
                        if template.Filename != "" {
                                filename = template.Filename + ext
                        }

                        att := &models.Attachment{
                                ID:          result.Hash,
                                TemplateID:  result.TemplateID,
                                Name:        template.ID,
                                Filename:    filename,
                                ContentType: mimeType,
                                Data:        result.Data,
                                Size:        result.Size,
                                Hash:        result.Hash,
                        }

                        mu.Lock()
                        results = append(results, att)
                        mu.Unlock()
                }(w)
        }

        wg.Wait()
        close(errChan)

        var errs []string
        for err := range errChan {
                errs = append(errs, err.Error())
        }

        if len(errs) > 0 && len(results) == 0 {
                return nil, fmt.Errorf("all generations failed: %s", strings.Join(errs, "; "))
        }

        return results, nil
}

// AttachmentSlot represents a single (templateID, format) pair used for per-recipient rotation.
type AttachmentSlot struct {
        TemplateID string
        Format     Format
}

// GetAttachmentSlots builds the flattened rotation list of (templateID, format) pairs
// for the given template IDs. Recipients are assigned exactly one slot in round-robin order,
// ensuring each gets one attachment file.
//
// Ordering: grouped by format (all templates sharing a format appear consecutively), so
// the variety is spread across recipients. Example:
//
//   attachment1=[pdf,webp], attachment2=[pdf,jpeg]
//   → [(a1,pdf),(a2,pdf),(a1,webp),(a2,jpeg)]
func (m *Manager) GetAttachmentSlots(templateIDs []string) []AttachmentSlot {
        m.mu.RLock()
        defer m.mu.RUnlock()

        // Resolve templates in the order they were listed
        templates := make([]*Template, 0, len(templateIDs))
        for _, id := range templateIDs {
                if tmpl, ok := m.templates[id]; ok {
                        templates = append(templates, tmpl)
                }
        }
        if len(templates) == 0 {
                return nil
        }

        defaultFmt := m.config.DefaultFormat
        if defaultFmt == "" {
                defaultFmt = FormatPDF
        }

        // Collect unique formats across all templates, preserving first-seen order.
        seenFmt := make(map[Format]bool)
        orderedFmts := make([]Format, 0)
        for _, tmpl := range templates {
                if len(tmpl.Formats) == 0 {
                        if !seenFmt[defaultFmt] {
                                seenFmt[defaultFmt] = true
                                orderedFmts = append(orderedFmts, defaultFmt)
                        }
                        continue
                }
                for _, f := range tmpl.Formats {
                        norm := Format(strings.ToLower(f))
                        if norm == "jpeg" {
                                norm = FormatJPG
                        }
                        if !seenFmt[norm] {
                                seenFmt[norm] = true
                                orderedFmts = append(orderedFmts, norm)
                        }
                }
        }

        // Build slots: for each format emit (template, format) for every template
        // that declares that format (or the default if no formats are listed).
        slots := make([]AttachmentSlot, 0)
        for _, fmt := range orderedFmts {
                for _, tmpl := range templates {
                        has := false
                        if len(tmpl.Formats) == 0 {
                                has = (fmt == defaultFmt)
                        } else {
                                for _, f := range tmpl.Formats {
                                        norm := Format(strings.ToLower(f))
                                        if norm == "jpeg" {
                                                norm = FormatJPG
                                        }
                                        if norm == fmt {
                                                has = true
                                                break
                                        }
                                }
                        }
                        if has {
                                slots = append(slots, AttachmentSlot{TemplateID: tmpl.ID, Format: fmt})
                        }
                }
        }

        return slots
}
