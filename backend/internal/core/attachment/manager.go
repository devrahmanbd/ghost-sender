package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	ID          string
	Name        string
	Path        string
	HTMLContent string
	Size        int64
	ModTime     time.Time
	Variables   []string
}

type GenerateRequest struct {
	TemplateID string
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
		cfg.SupportedFormats = []Format{FormatPDF, FormatJPG, FormatPNG, FormatWebP}
	}

	m := &Manager{
		config:    cfg,
		templates: make(map[string]*Template),
		closeCh:   make(chan struct{}),
		workerCh:  make(chan *generateRequest, cfg.WorkerQueueSize),
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
		atomic.AddUint64(&m.stats.failedGenerations, 1)
		return nil, fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}

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

	keys := make([]string, 0, len(req.PersonData))
	for k := range req.PersonData {
		keys = append(keys, k)
	}

	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(req.PersonData[k]))
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

func personalizeHTML(html string, data map[string]string) string {
	result := html
	for key, value := range data {
		placeholder := fmt.Sprintf("{{%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
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
// Prepare generates attachments for a recipient using available templates and personalization data
func (m *Manager) Prepare(ctx context.Context, recipient interface{}, data map[string]interface{}) ([]interface{}, error) {
    if recipient == nil {
        return nil, errors.New("attachment: recipient required")
    }

    m.mu.RLock()
    closed := m.closed
    templates := make(map[string]*Template)
    for k, v := range m.templates {
        templates[k] = v
    }
    m.mu.RUnlock()

    if closed {
        return nil, ErrManagerClosed
    }

    if len(templates) == 0 {
        return nil, ErrNoTemplates
    }

    // Convert data to map[string]string for personalization
    personData := make(map[string]string)
    for key, value := range data {
        personData[key] = fmt.Sprintf("%v", value)
    }

    var results []interface{}
    var mu sync.Mutex
    var wg sync.WaitGroup
    errChan := make(chan error, len(templates))

    // Generate attachments from all templates
    for _, tmpl := range templates {
        wg.Add(1)
        go func(template *Template) {
            defer wg.Done()

            // Determine format
            format := m.config.DefaultFormat
            if format == "" {
                format = FormatPDF
            }

            // Create generation request
            req := &GenerateRequest{
                TemplateID: template.ID,
                Format:     format,
                PersonData: personData,
                Context:    ctx,
            }

            // Generate attachment
            result, err := m.Generate(req)
            if err != nil {
                errChan <- fmt.Errorf("template %s: %w", template.ID, err)
                return
            }

            mu.Lock()
            results = append(results, result)
            mu.Unlock()
        }(tmpl)
    }

    wg.Wait()
    close(errChan)

    // Collect errors
    var errs []string
    for err := range errChan {
        errs = append(errs, err.Error())
    }

    if len(errs) > 0 && len(results) == 0 {
        return nil, fmt.Errorf("all generations failed: %s", strings.Join(errs, "; "))
    }

    return results, nil
}
