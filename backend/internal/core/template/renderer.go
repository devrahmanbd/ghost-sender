package template

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"
	"time"

	"email-campaign-system/pkg/logger"
)

var (
	ErrRenderFailed      = errors.New("template rendering failed")
	ErrInvalidVariable   = errors.New("invalid variable format")
	ErrMissingVariable   = errors.New("required variable missing")
	ErrConditionalFailed = errors.New("conditional evaluation failed")
)

type Renderer struct {
	log                logger.Logger
	config             *RendererConfig
	stats              *RenderStats
	statsMu            sync.RWMutex
	variablePatterns   map[string]*regexp.Regexp
	conditionalPattern *regexp.Regexp
	loopPattern        *regexp.Regexp
	functionPattern    *regexp.Regexp
	mu                 sync.RWMutex
}

type RendererConfig struct {
	EnableConditionals bool
	EnableLoops        bool
	EnableFunctions    bool
	EnableHTMLEscape   bool
	EnableStrictMode   bool
	MaxRecursionDepth  int
	MaxLoopIterations  int
	VariablePrefix     string
	VariableSuffix     string
	DefaultValues      map[string]string
	RequiredVariables  []string
	AllowedFunctions   []string
	TimeFormat         string
	DateFormat         string
}

type RenderStats struct {
	TotalRenders      int64
	SuccessfulRenders int64
	FailedRenders     int64
	TotalVariables    int64
	MissingVariables  int64
	TotalConditionals int64
	TotalLoops        int64
	TotalFunctions    int64
	AverageRenderTime time.Duration
	LastRender        time.Time
}

type RenderContext struct {
	Variables      map[string]string
	RecipientEmail string
	RecipientName  string
	CampaignID     string
	TemplateID     string
	CustomFields   map[string]interface{}
	Timestamp      time.Time
	Metadata       map[string]interface{}
	RecursionDepth int
}

func NewRenderer(log logger.Logger) *Renderer {
	config := DefaultRendererConfig()

	renderer := &Renderer{
		log:              log,
		config:           config,
		variablePatterns: make(map[string]*regexp.Regexp),
		stats: &RenderStats{
			LastRender: time.Now(),
		},
	}

	renderer.initializePatterns()

	return renderer
}

func DefaultRendererConfig() *RendererConfig {
	return &RendererConfig{
		EnableConditionals: true,
		EnableLoops:        true,
		EnableFunctions:    true,
		EnableHTMLEscape:   false,
		EnableStrictMode:   false,
		MaxRecursionDepth:  5,
		MaxLoopIterations:  100,
		VariablePrefix:     "{{",
		VariableSuffix:     "}}",
		DefaultValues:      make(map[string]string),
		RequiredVariables:  []string{},
		AllowedFunctions:   []string{"upper", "lower", "title", "trim", "truncate", "default"},
		TimeFormat:         "15:04:05",
		DateFormat:         "2006-01-02",
	}
}

func (r *Renderer) initializePatterns() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.variablePatterns["double_curly"] = regexp.MustCompile(`\{\{([A-Z_][A-Z0-9_]*(?:\|[a-z]+(?:\:[^}]+)?)?)\}\}`)
	r.variablePatterns["single_curly"] = regexp.MustCompile(`\{([A-Z_][A-Z0-9_]*)\}`)
	r.variablePatterns["dollar"] = regexp.MustCompile(`\$([A-Z_][A-Z0-9_]*)\$`)
	r.variablePatterns["percent"] = regexp.MustCompile(`%([A-Z_][A-Z0-9_]*)%`)

	r.conditionalPattern = regexp.MustCompile(`\{\{if\s+([A-Z_][A-Z0-9_]*)\}\}(.*?)\{\{endif\}\}`)
	r.loopPattern = regexp.MustCompile(`\{\{foreach\s+([A-Z_][A-Z0-9_]*)\}\}(.*?)\{\{endforeach\}\}`)
	r.functionPattern = regexp.MustCompile(`([A-Z_][A-Z0-9_]*)\|([a-z]+)(?:\:([^}]+))?`)
}

func (r *Renderer) Render(template string, variables map[string]string) (string, error) {
	startTime := time.Now()

	if template == "" {
		return "", errors.New("template is empty")
	}

	if variables == nil {
		variables = make(map[string]string)
	}

	ctx := &RenderContext{
		Variables:      r.mergeWithDefaults(variables),
		Timestamp:      time.Now(),
		RecursionDepth: 0,
	}

	if r.config.EnableStrictMode {
		if err := r.validateRequiredVariables(ctx.Variables); err != nil {
			r.incrementFailedRenders()
			return "", err
		}
	}

	result := template

	if r.config.EnableConditionals {
		var err error
		result, err = r.processConditionals(result, ctx)
		if err != nil {
			r.log.Warn(
				"conditional processing failed",
				logger.String("error", err.Error()),
			)
		}
	}

	if r.config.EnableLoops {
		var err error
		result, err = r.processLoops(result, ctx)
		if err != nil {
			r.log.Warn(
				"loop processing failed",
				logger.String("error", err.Error()),
			)
		}
	}


	result = r.replaceVariables(result, ctx)

	if r.config.EnableHTMLEscape {
		result = r.escapeHTML(result)
	}

	r.updateRenderStats(time.Since(startTime), len(variables))

	return result, nil
}

func (r *Renderer) RenderWithContext(template string, ctx *RenderContext) (string, error) {
	if ctx == nil {
		return "", errors.New("render context is required")
	}

	if ctx.Variables == nil {
		ctx.Variables = make(map[string]string)
	}

	return r.Render(template, ctx.Variables)
}

func (r *Renderer) replaceVariables(template string, ctx *RenderContext) string {
	result := template

	for _, pattern := range r.variablePatterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			return r.processVariableMatch(match, pattern, ctx)
		})
	}

	return result
}

func (r *Renderer) processVariableMatch(match string, pattern *regexp.Regexp, ctx *RenderContext) string {
	matches := pattern.FindStringSubmatch(match)
	if len(matches) < 2 {
		return match
	}

	varSpec := matches[1]

	varName := varSpec
	var functions []string
	var functionArgs []string

	if r.config.EnableFunctions && strings.Contains(varSpec, "|") {
		parts := strings.Split(varSpec, "|")
		varName = parts[0]

		for _, fn := range parts[1:] {
			if strings.Contains(fn, ":") {
				fnParts := strings.SplitN(fn, ":", 2)
				functions = append(functions, fnParts[0])
				functionArgs = append(functionArgs, fnParts[1])
			} else {
				functions = append(functions, fn)
				functionArgs = append(functionArgs, "")
			}
		}
	}

	value, exists := ctx.Variables[varName]
	if !exists {
		r.incrementMissingVariables()

		if defaultVal, hasDefault := r.config.DefaultValues[varName]; hasDefault {
			value = defaultVal
		} else if r.config.EnableStrictMode {
			return match
		} else {
			return ""
		}
	}

	if r.config.EnableFunctions && len(functions) > 0 {
		for i, fn := range functions {
			arg := ""
			if i < len(functionArgs) {
				arg = functionArgs[i]
			}
			value = r.applyFunction(fn, value, arg)
		}
	}

	r.incrementTotalVariables()
	return value
}

func (r *Renderer) processConditionals(template string, ctx *RenderContext) (string, error) {
	result := template

	for {
		matches := r.conditionalPattern.FindStringSubmatch(result)
		if len(matches) == 0 {
			break
		}

		varName := matches[1]
		content := matches[2]
		fullMatch := matches[0]

		_, exists := ctx.Variables[varName]
		if exists {
			value := ctx.Variables[varName]
			if value != "" && value != "0" && strings.ToLower(value) != "false" {
				result = strings.Replace(result, fullMatch, content, 1)
			} else {
				result = strings.Replace(result, fullMatch, "", 1)
			}
		} else {
			result = strings.Replace(result, fullMatch, "", 1)
		}

		r.incrementTotalConditionals()
	}

	return result, nil
}

func (r *Renderer) processLoops(template string, ctx *RenderContext) (string, error) {
	result := template

	for {
		matches := r.loopPattern.FindStringSubmatch(result)
		if len(matches) == 0 {
			break
		}

		varName := matches[1]
		content := matches[2]
		fullMatch := matches[0]

		value, exists := ctx.Variables[varName]
		if !exists {
			result = strings.Replace(result, fullMatch, "", 1)
			continue
		}

		items := strings.Split(value, ",")
		if len(items) > r.config.MaxLoopIterations {
			items = items[:r.config.MaxLoopIterations]
		}

		var output strings.Builder
		for i, item := range items {
			itemContent := content
			itemContent = strings.ReplaceAll(itemContent, "{{ITEM}}", strings.TrimSpace(item))
			itemContent = strings.ReplaceAll(itemContent, "{{INDEX}}", fmt.Sprintf("%d", i+1))
			itemContent = strings.ReplaceAll(itemContent, "{{INDEX0}}", fmt.Sprintf("%d", i))
			output.WriteString(itemContent)
		}

		result = strings.Replace(result, fullMatch, output.String(), 1)
		r.incrementTotalLoops()
	}

	return result, nil
}

func (r *Renderer) applyFunction(function, value, arg string) string {
	if !r.isFunctionAllowed(function) {
		return value
	}

	r.incrementTotalFunctions()

	switch function {
	case "upper":
		return strings.ToUpper(value)

	case "lower":
		return strings.ToLower(value)

	case "title":
		return strings.Title(strings.ToLower(value))

	case "trim":
		return strings.TrimSpace(value)

	case "truncate":
		if arg != "" {
			var maxLen int
			fmt.Sscanf(arg, "%d", &maxLen)
			if maxLen > 0 && len(value) > maxLen {
				return value[:maxLen] + "..."
			}
		}
		return value

	case "default":
		if value == "" && arg != "" {
			return arg
		}
		return value

	case "capitalize":
		if len(value) > 0 {
			return strings.ToUpper(value[:1]) + value[1:]
		}
		return value

	case "reverse":
		runes := []rune(value)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes)

	case "repeat":
		if arg != "" {
			var count int
			fmt.Sscanf(arg, "%d", &count)
			if count > 0 && count <= 10 {
				return strings.Repeat(value, count)
			}
		}
		return value

	case "replace":
		if arg != "" {
			parts := strings.SplitN(arg, ",", 2)
			if len(parts) == 2 {
				return strings.ReplaceAll(value, parts[0], parts[1])
			}
		}
		return value

	case "substr":
		if arg != "" {
			var start, length int
			fmt.Sscanf(arg, "%d,%d", &start, &length)
			if start >= 0 && start < len(value) {
				end := start + length
				if end > len(value) {
					end = len(value)
				}
				return value[start:end]
			}
		}
		return value

	default:
		return value
	}
}

func (r *Renderer) isFunctionAllowed(function string) bool {
	for _, allowed := range r.config.AllowedFunctions {
		if function == allowed {
			return true
		}
	}
	return false
}

func (r *Renderer) escapeHTML(content string) string {
	return html.EscapeString(content)
}

func (r *Renderer) mergeWithDefaults(variables map[string]string) map[string]string {
	merged := make(map[string]string)

	for k, v := range r.config.DefaultValues {
		merged[k] = v
	}

	for k, v := range variables {
		merged[k] = v
	}

	return merged
}

func (r *Renderer) validateRequiredVariables(variables map[string]string) error {
	for _, required := range r.config.RequiredVariables {
		if _, exists := variables[required]; !exists {
			return fmt.Errorf("%w: %s", ErrMissingVariable, required)
		}
	}
	return nil
}

func (r *Renderer) SetConfig(config *RendererConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = config
	r.initializePatterns()

	r.log.Info("renderer config updated")
}

func (r *Renderer) GetConfig() *RendererConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configCopy := *r.config
	return &configCopy
}

func (r *Renderer) AddDefaultValue(key, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config.DefaultValues[key] = value
}

func (r *Renderer) RemoveDefaultValue(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.config.DefaultValues, key)
}

func (r *Renderer) AddRequiredVariable(varName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.config.RequiredVariables {
		if existing == varName {
			return
		}
	}

	r.config.RequiredVariables = append(r.config.RequiredVariables, varName)
}

func (r *Renderer) RemoveRequiredVariable(varName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := make([]string, 0)
	for _, v := range r.config.RequiredVariables {
		if v != varName {
			filtered = append(filtered, v)
		}
	}

	r.config.RequiredVariables = filtered
}

func (r *Renderer) AddAllowedFunction(function string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.config.AllowedFunctions {
		if existing == function {
			return
		}
	}

	r.config.AllowedFunctions = append(r.config.AllowedFunctions, function)
}

func (r *Renderer) GetStats() *RenderStats {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()

	stats := *r.stats
	return &stats
}

func (r *Renderer) ResetStats() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats = &RenderStats{
		LastRender: time.Now(),
	}

	r.log.Info("renderer stats reset")
}

func (r *Renderer) updateRenderStats(duration time.Duration, variableCount int) {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats.TotalRenders++
	r.stats.SuccessfulRenders++
	r.stats.LastRender = time.Now()

	if r.stats.AverageRenderTime == 0 {
		r.stats.AverageRenderTime = duration
	} else {
		r.stats.AverageRenderTime = (r.stats.AverageRenderTime + duration) / 2
	}
}

func (r *Renderer) incrementFailedRenders() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats.TotalRenders++
	r.stats.FailedRenders++
}

func (r *Renderer) incrementTotalVariables() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats.TotalVariables++
}

func (r *Renderer) incrementMissingVariables() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats.MissingVariables++
}

func (r *Renderer) incrementTotalConditionals() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats.TotalConditionals++
}

func (r *Renderer) incrementTotalLoops() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats.TotalLoops++
}

func (r *Renderer) incrementTotalFunctions() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.stats.TotalFunctions++
}

func (r *Renderer) ExtractVariables(template string) []string {
	variables := make(map[string]bool)

	for _, pattern := range r.variablePatterns {
		matches := pattern.FindAllStringSubmatch(template, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				varName := match[1]
				if strings.Contains(varName, "|") {
					varName = strings.Split(varName, "|")[0]
				}
				variables[varName] = true
			}
		}
	}

	result := make([]string, 0, len(variables))
	for varName := range variables {
		result = append(result, varName)
	}

	return result
}

func (r *Renderer) ValidateTemplate(template string) error {
	if template == "" {
		return errors.New("template is empty")
	}

	if r.config.EnableConditionals {
		openIfs := strings.Count(template, "{{if ")
		closeIfs := strings.Count(template, "{{endif}}")
		if openIfs != closeIfs {
			return errors.New("mismatched if/endif blocks")
		}
	}

	if r.config.EnableLoops {
		openLoops := strings.Count(template, "{{foreach ")
		closeLoops := strings.Count(template, "{{endforeach}}")
		if openLoops != closeLoops {
			return errors.New("mismatched foreach/endforeach blocks")
		}
	}

	return nil
}

func (r *Renderer) Preview(template string, sampleData map[string]string) (string, error) {
	if sampleData == nil {
		sampleData = r.generateSampleData()
	}

	return r.Render(template, sampleData)
}

func (r *Renderer) generateSampleData() map[string]string {
	return map[string]string{
		"RECIPIENT_NAME":  "John Doe",
		"RECIPIENT_EMAIL": "john.doe@example.com",
		"SENDER_NAME":     "Company Name",
		"SUBJECT":         "Sample Subject",
		"DATE":            time.Now().Format(r.config.DateFormat),
		"TIME":            time.Now().Format(r.config.TimeFormat),
	}
}

func (r *Renderer) RenderBatch(templates []string, variables map[string]string) ([]string, error) {
	results := make([]string, len(templates))
	var wg sync.WaitGroup
	errChan := make(chan error, len(templates))

	for i, tmpl := range templates {
		wg.Add(1)
		go func(index int, template string) {
			defer wg.Done()

			rendered, err := r.Render(template, variables)
			if err != nil {
				errChan <- err
				return
			}

			results[index] = rendered
		}(i, tmpl)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return results, <-errChan
	}

	return results, nil
}

func (r *Renderer) CloneConfig() *RendererConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config := *r.config

	config.DefaultValues = make(map[string]string)
	for k, v := range r.config.DefaultValues {
		config.DefaultValues[k] = v
	}

	config.RequiredVariables = make([]string, len(r.config.RequiredVariables))
	copy(config.RequiredVariables, r.config.RequiredVariables)

	config.AllowedFunctions = make([]string, len(r.config.AllowedFunctions))
	copy(config.AllowedFunctions, r.config.AllowedFunctions)

	return &config
}

func (r *Renderer) GetVariableCount(template string) int {
	return len(r.ExtractVariables(template))
}

func (r *Renderer) HasVariable(template, varName string) bool {
	variables := r.ExtractVariables(template)
	for _, v := range variables {
		if v == varName {
			return true
		}
	}
	return false
}

func (r *Renderer) ReplaceVariable(template, varName, value string) string {
	variables := map[string]string{
		varName: value,
	}

	rendered, _ := r.Render(template, variables)
	return rendered
}

func (r *Renderer) StripVariables(template string) string {
	result := template

	for _, pattern := range r.variablePatterns {
		result = pattern.ReplaceAllString(result, "")
	}

	return result
}
