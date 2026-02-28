package personalization

import (
	"errors"
	"fmt"
	"regexp"
	"context"
	"strings"
	"sync"
	"time"

	"email-campaign-system/internal/models"
	"email-campaign-system/pkg/logger"
)

var (
	ErrInvalidVariable  = errors.New("invalid variable format")
	ErrVariableNotFound = errors.New("variable not found")
	ErrProcessingFailed = errors.New("personalization processing failed")
	ErrRecipientRequired = errors.New("recipient data required")
	ErrContextEmpty     = errors.New("personalization context is empty")
)

type Manager struct {
	log               logger.Logger
	config            *PersonalizationConfig
	variables         *VariableRegistry
	generator         *Generator
	dynamicProcessor  *DynamicProcessor
	extractor         *NameExtractor
	dateTimeFormatter *DateTimeFormatter
	cache             map[string]string
	stats             *PersonalizationStats
	mu                sync.RWMutex
}

type PersonalizationConfig struct {
	EnableCaching          bool
	CacheSize              int
	EnableSmartNameExtract bool
	EnableTimeBasedContent bool
	EnableCustomVariables  bool
	DefaultTimezone        string
	MaxVariableLength      int
	StrictMode             bool
	EnableStatistics       bool
}

type PersonalizationContext struct {
	Recipient      *models.Recipient
	Campaign       *models.Campaign
	Account        *models.Account
	CustomFields   map[string]string
	Timestamp      time.Time
	Timezone       string
	AdditionalData map[string]interface{}
}

type PersonalizationResult struct {
	Content         string
	Subject         string
	SenderName      string
	Variables       map[string]string
	ProcessedCount  int
	FailedVariables []string
	ProcessingTime  time.Duration
	FromCache       bool
}

type PersonalizationStats struct {
	TotalProcessed   int64
	TotalVariables   int64
	CacheHits        int64
	CacheMisses      int64
	FailedProcessing int64
	AverageTime      time.Duration
	mu               sync.RWMutex
}

type VariableRegistry struct {
	variables       map[string]VariableDefinition
	customVariables map[string]CustomVariableFunc
	mu              sync.RWMutex
}

type VariableDefinition struct {
	Name              string
	Description       string
	Type              string
	Example           string
	RequiresRecipient bool
	Processor         VariableProcessor
}

type VariableProcessor func(ctx *PersonalizationContext) (string, error)
type CustomVariableFunc func(ctx *PersonalizationContext) (string, error)

func NewManager(log logger.Logger, config *PersonalizationConfig) *Manager {
	if config == nil {
		config = DefaultPersonalizationConfig()
	}

	manager := &Manager{
		log:       log,
		config:    config,
		variables: NewVariableRegistry(),
		cache:     make(map[string]string),
		stats:     &PersonalizationStats{},
	}

	manager.generator = NewGenerator(log)
	manager.dynamicProcessor = NewDynamicProcessor(log)
	manager.extractor = NewNameExtractor(log)
	manager.dateTimeFormatter = NewDateTimeFormatter(log, config.DefaultTimezone)

	manager.registerBuiltInVariables()

	return manager
}

func DefaultPersonalizationConfig() *PersonalizationConfig {
	return &PersonalizationConfig{
		EnableCaching:          true,
		CacheSize:              10000,
		EnableSmartNameExtract: true,
		EnableTimeBasedContent: true,
		EnableCustomVariables:  true,
		DefaultTimezone:        "UTC",
		MaxVariableLength:      100,
		StrictMode:             false,
		EnableStatistics:       true,
	}
}

func (m *Manager) Personalize(content, subject, senderName string, ctx *PersonalizationContext) (*PersonalizationResult, error) {
	if ctx == nil {
		return nil, ErrContextEmpty
	}

	startTime := time.Now()

	result := &PersonalizationResult{
		Variables:       make(map[string]string),
		FailedVariables: make([]string, 0),
	}

	if ctx.Timestamp.IsZero() {
		ctx.Timestamp = time.Now()
	}

	if ctx.Timezone == "" {
		ctx.Timezone = m.config.DefaultTimezone
	}

	personalizedContent, contentVars, contentFailed := m.processContent(content, ctx)
	result.Content = personalizedContent

	personalizedSubject, subjectVars, subjectFailed := m.processContent(subject, ctx)
	result.Subject = personalizedSubject

	personalizedSenderName, senderVars, senderFailed := m.processContent(senderName, ctx)
	result.SenderName = personalizedSenderName

	for k, v := range contentVars {
		result.Variables[k] = v
	}
	for k, v := range subjectVars {
		result.Variables[k] = v
	}
	for k, v := range senderVars {
		result.Variables[k] = v
	}

	result.FailedVariables = append(result.FailedVariables, contentFailed...)
	result.FailedVariables = append(result.FailedVariables, subjectFailed...)
	result.FailedVariables = append(result.FailedVariables, senderFailed...)

	result.ProcessedCount = len(result.Variables)
	result.ProcessingTime = time.Since(startTime)

	m.recordProcessing(result)

	return result, nil
}

func (m *Manager) PersonalizeForRecipient(content string, recipient *models.Recipient) (string, error) {
	if recipient == nil {
		return "", ErrRecipientRequired
	}

	ctx := &PersonalizationContext{
		Recipient:    recipient,
		Timestamp:    time.Now(),
		Timezone:     m.config.DefaultTimezone,
		CustomFields: make(map[string]string),
	}

	result, err := m.Personalize(content, "", "", ctx)
	if err != nil {
		return "", err
	}

	return result.Content, nil
}

func (m *Manager) Generate(ctx context.Context, recipient *models.Recipient, template *models.Template) (map[string]interface{}, error) {
    if recipient == nil {
        return nil, ErrRecipientRequired
    }

    if template == nil {
        return nil, errors.New("template cannot be nil")
    }

    personalizationCtx := &PersonalizationContext{
        Recipient:      recipient,
        Timestamp:      time.Now(),
        Timezone:       m.config.DefaultTimezone,
        CustomFields:   make(map[string]string),
        AdditionalData: make(map[string]interface{}),
    }

    // Variables is a SLICE of TemplateVariable structs
    if template.Variables != nil {
        for _, variable := range template.Variables {
            // Add variable name and default value to CustomFields
            if variable.Name != "" {
                value := variable.DefaultValue
                if value == "" {
                    value = "" // Use empty string if no default
                }
                personalizationCtx.CustomFields[variable.Name] = value
            }
        }
    }

    // Use correct template field names from models.Template
    templateContent := template.HTMLContent
    if templateContent == "" {
        templateContent = template.PlainTextContent
    }

    templateSubject := ""
    if len(template.Subjects) > 0 {
        templateSubject = template.Subjects[0]
    }

    templateSenderName := ""
    if len(template.SenderNames) > 0 {
        templateSenderName = template.SenderNames[0]
    }

    result, err := m.Personalize(
        templateContent,
        templateSubject,
        templateSenderName,
        personalizationCtx,
    )
    if err != nil {
        if m.log != nil {
            m.log.Error(fmt.Sprintf("personalization failed for recipient %s: %v", recipient.Email, err))
        }
        return nil, fmt.Errorf("failed to generate personalized content: %w", err)
    }

    response := map[string]interface{}{
        "subject":          result.Subject,
        "body":             result.Content,
        "sender_name":      result.SenderName,
        "recipient_email":  recipient.Email,
        "variables":        result.Variables,
        "processed_count":  result.ProcessedCount,
        "failed_variables": result.FailedVariables,
        "processing_time":  result.ProcessingTime.String(),
        "from_cache":       result.FromCache,
        "template_id":      template.ID,
        "template_name":    template.Name,
    }

    if m.log != nil {
        m.log.Debug(fmt.Sprintf("generated personalized content for %s: processed=%d, failed=%d",
            recipient.Email, result.ProcessedCount, len(result.FailedVariables)))
    }

    return response, nil
}


func (m *Manager) processContent(content string, ctx *PersonalizationContext) (string, map[string]string, []string) {
	variables := make(map[string]string)
	failed := make([]string, 0)

	variablePattern := regexp.MustCompile(`\{\{([A-Z_][A-Z0-9_]*(?:[+-]\d+)?)\}\}`)
	matches := variablePattern.FindAllStringSubmatch(content, -1)

	processedContent := content

	for _, match := range matches {
		fullMatch := match[0]
		variableName := match[1]

		value, err := m.processVariable(variableName, ctx)
		if err != nil {
			failed = append(failed, variableName)
			if m.log != nil {
				m.log.Warn(fmt.Sprintf("failed to process variable %s: %v", variableName, err))
			}
			continue
		}

		variables[variableName] = value
		processedContent = strings.ReplaceAll(processedContent, fullMatch, value)
	}

	return processedContent, variables, failed
}

func (m *Manager) processVariable(variableName string, ctx *PersonalizationContext) (string, error) {
	variableName = strings.TrimSpace(variableName)

	if strings.Contains(variableName, "_DATE_") && (strings.Contains(variableName, "+") || strings.Contains(variableName, "-")) {
		return m.dynamicProcessor.ProcessDateWithOffset(variableName, ctx)
	}

	if strings.HasPrefix(variableName, "RANDOM_") {
		return m.dynamicProcessor.ProcessRandomVariable(variableName, ctx)
	}

	if strings.HasPrefix(variableName, "CUSTOM_") {
		return m.processCustomVariable(variableName, ctx)
	}

	switch variableName {
	case "RECIPIENT_EMAIL":
		if ctx.Recipient == nil {
			return "", ErrRecipientRequired
		}
		return ctx.Recipient.Email, nil

	case "RECIPIENT_NAME":
		if ctx.Recipient == nil {
			return "", ErrRecipientRequired
		}
		fullName := m.extractor.buildFullName(ctx.Recipient.FirstName, ctx.Recipient.LastName)
		if fullName != "" {
			return fullName, nil
		}
		if m.config.EnableSmartNameExtract {
			return m.extractor.ExtractNameFromEmail(ctx.Recipient.Email), nil
		}
		return ctx.Recipient.Email, nil

	case "FIRST_NAME":
		if ctx.Recipient == nil {
			return "", ErrRecipientRequired
		}
		return m.extractor.ExtractFirstName(ctx.Recipient), nil

	case "LAST_NAME":
		if ctx.Recipient == nil {
			return "", ErrRecipientRequired
		}
		return m.extractor.ExtractLastName(ctx.Recipient), nil

	case "EMAIL_USERNAME":
		if ctx.Recipient == nil {
			return "", ErrRecipientRequired
		}
		parts := strings.Split(ctx.Recipient.Email, "@")
		if len(parts) > 0 {
			return parts[0], nil
		}
		return "", ErrProcessingFailed

	case "EMAIL_DOMAIN":
		if ctx.Recipient == nil {
			return "", ErrRecipientRequired
		}
		parts := strings.Split(ctx.Recipient.Email, "@")
		if len(parts) > 1 {
			return parts[1], nil
		}
		return "", ErrProcessingFailed

	case "TODAY_DATE":
		return m.dateTimeFormatter.FormatDate(ctx.Timestamp, "2006-01-02"), nil

	case "TODAY_DATE_LONG":
		return m.dateTimeFormatter.FormatDate(ctx.Timestamp, "January 2, 2006"), nil

	case "TODAY_DATE_SHORT":
		return m.dateTimeFormatter.FormatDate(ctx.Timestamp, "01/02/2006"), nil

	case "CURRENT_TIME":
		return m.dateTimeFormatter.FormatDate(ctx.Timestamp, "15:04:05"), nil

	case "CURRENT_YEAR":
		return m.dateTimeFormatter.FormatDate(ctx.Timestamp, "2006"), nil

	case "CURRENT_MONTH":
		return m.dateTimeFormatter.FormatDate(ctx.Timestamp, "January"), nil

	case "CURRENT_DAY":
		return m.dateTimeFormatter.FormatDate(ctx.Timestamp, "Monday"), nil

	case "TIME_OF_DAY":
		return m.getTimeOfDay(ctx.Timestamp), nil

	case "GREETING":
		return m.getGreeting(ctx.Timestamp), nil

	case "INVOICE_NUMBER":
		return m.generator.GenerateInvoiceNumber(), nil

	case "ORDER_NUMBER":
		return m.generator.GenerateOrderNumber(), nil

	case "TRACKING_NUMBER":
		return m.generator.GenerateTrackingNumber(), nil

	case "REFERENCE_NUMBER":
		return m.generator.GenerateReferenceNumber(), nil

	case "PHONE_NUMBER":
		return m.generator.GeneratePhoneNumber(), nil

	case "RANDOM_NUMBER":
		return m.generator.GenerateRandomNumber(6), nil

	case "RANDOM_ALPHA":
		return m.generator.GenerateRandomAlpha(8), nil

	case "RANDOM_ALPHANUMERIC":
		return m.generator.GenerateRandomAlphanumeric(10), nil

	case "UUID":
		return m.generator.GenerateUUID(), nil

	case "TIMESTAMP":
		return fmt.Sprintf("%d", ctx.Timestamp.Unix()), nil

	case "UNSUBSCRIBE_LINK":
		return m.generateUnsubscribeLink(ctx), nil

	default:
		if ctx.CustomFields != nil {
			if value, exists := ctx.CustomFields[variableName]; exists {
				return value, nil
			}
		}

		varDef, err := m.variables.Get(variableName)
		if err != nil {
			return "", ErrVariableNotFound
		}

		if varDef.Processor != nil {
			return varDef.Processor(ctx)
		}

		return "", ErrVariableNotFound
	}
}

func (m *Manager) processCustomVariable(variableName string, ctx *PersonalizationContext) (string, error) {
	m.mu.RLock()
	customFunc, exists := m.variables.customVariables[variableName]
	m.mu.RUnlock()

	if !exists {
		if ctx.CustomFields != nil {
			if value, ok := ctx.CustomFields[variableName]; ok {
				return value, nil
			}
		}
		return "", ErrVariableNotFound
	}

	return customFunc(ctx)
}

func (m *Manager) getTimeOfDay(t time.Time) string {
	hour := t.Hour()

	if hour >= 5 && hour < 12 {
		return "morning"
	} else if hour >= 12 && hour < 17 {
		return "afternoon"
	} else if hour >= 17 && hour < 21 {
		return "evening"
	}
	return "night"
}

func (m *Manager) getGreeting(t time.Time) string {
	hour := t.Hour()

	if hour >= 5 && hour < 12 {
		return "Good morning"
	} else if hour >= 12 && hour < 17 {
		return "Good afternoon"
	} else if hour >= 17 && hour < 21 {
		return "Good evening"
	}
	return "Hello"
}

func (m *Manager) generateUnsubscribeLink(ctx *PersonalizationContext) string {
	if ctx.Recipient == nil {
		return ""
	}
	return fmt.Sprintf("https://example.com/unsubscribe?email=%s", ctx.Recipient.Email)
}

func (m *Manager) ExtractVariables(content string) []string {
	variablePattern := regexp.MustCompile(`\{\{([A-Z_][A-Z0-9_]*(?:[+-]\d+)?)\}\}`)
	matches := variablePattern.FindAllStringSubmatch(content, -1)

	variables := make([]string, 0)
	seen := make(map[string]bool)

	for _, match := range matches {
		varName := match[1]
		if !seen[varName] {
			variables = append(variables, varName)
			seen[varName] = true
		}
	}

	return variables
}

func (m *Manager) ValidateVariables(content string) (bool, []string) {
	variables := m.ExtractVariables(content)
	invalid := make([]string, 0)

	for _, varName := range variables {
		if !m.isValidVariable(varName) {
			invalid = append(invalid, varName)
		}
	}

	return len(invalid) == 0, invalid
}

func (m *Manager) isValidVariable(varName string) bool {
	if strings.HasPrefix(varName, "RANDOM_") || strings.HasPrefix(varName, "CUSTOM_") {
		return true
	}

	if strings.Contains(varName, "_DATE_") {
		return true
	}

	_, err := m.variables.Get(varName)
	return err == nil
}

func (m *Manager) RegisterCustomVariable(name string, processor CustomVariableFunc) error {
	if name == "" {
		return errors.New("variable name cannot be empty")
	}

	if processor == nil {
		return errors.New("processor function cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.variables.customVariables[name] = processor

	if m.log != nil {
		m.log.Info(fmt.Sprintf("registered custom variable: %s", name))
	}

	return nil
}

func (m *Manager) GetAvailableVariables() []VariableDefinition {
	return m.variables.GetAll()
}

func (m *Manager) GetStats() *PersonalizationStats {
	m.stats.mu.RLock()
	defer m.stats.mu.RUnlock()

	return &PersonalizationStats{
		TotalProcessed:   m.stats.TotalProcessed,
		TotalVariables:   m.stats.TotalVariables,
		CacheHits:        m.stats.CacheHits,
		CacheMisses:      m.stats.CacheMisses,
		FailedProcessing: m.stats.FailedProcessing,
		AverageTime:      m.stats.AverageTime,
	}
}

func (m *Manager) ResetStats() {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.TotalProcessed = 0
	m.stats.TotalVariables = 0
	m.stats.CacheHits = 0
	m.stats.CacheMisses = 0
	m.stats.FailedProcessing = 0
	m.stats.AverageTime = 0
}

func (m *Manager) recordProcessing(result *PersonalizationResult) {
	if !m.config.EnableStatistics {
		return
	}

	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.TotalProcessed++
	m.stats.TotalVariables += int64(result.ProcessedCount)

	if len(result.FailedVariables) > 0 {
		m.stats.FailedProcessing++
	}

	if result.FromCache {
		m.stats.CacheHits++
	} else {
		m.stats.CacheMisses++
	}

	totalTime := int64(m.stats.AverageTime) * (m.stats.TotalProcessed - 1)
	totalTime += int64(result.ProcessingTime)
	m.stats.AverageTime = time.Duration(totalTime / m.stats.TotalProcessed)
}

func (m *Manager) registerBuiltInVariables() {
	builtInVars := []VariableDefinition{
		{
			Name:              "RECIPIENT_EMAIL",
			Description:       "Recipient's email address",
			Type:              "string",
			Example:           "user@example.com",
			RequiresRecipient: true,
		},
		{
			Name:              "RECIPIENT_NAME",
			Description:       "Recipient's full name",
			Type:              "string",
			Example:           "John Doe",
			RequiresRecipient: true,
		},
		{
			Name:              "FIRST_NAME",
			Description:       "Recipient's first name",
			Type:              "string",
			Example:           "John",
			RequiresRecipient: true,
		},
		{
			Name:              "LAST_NAME",
			Description:       "Recipient's last name",
			Type:              "string",
			Example:           "Doe",
			RequiresRecipient: true,
		},
		{
			Name:        "TODAY_DATE",
			Description: "Current date (YYYY-MM-DD)",
			Type:        "date",
			Example:     "2026-02-07",
		},
		{
			Name:        "INVOICE_NUMBER",
			Description: "Random invoice number",
			Type:        "string",
			Example:     "INV-123456",
		},
		{
			Name:        "GREETING",
			Description: "Time-based greeting",
			Type:        "string",
			Example:     "Good morning",
		},
	}

	for _, varDef := range builtInVars {
		m.variables.Register(varDef)
	}
}

func NewVariableRegistry() *VariableRegistry {
	return &VariableRegistry{
		variables:       make(map[string]VariableDefinition),
		customVariables: make(map[string]CustomVariableFunc),
	}
}

func (vr *VariableRegistry) Register(varDef VariableDefinition) {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	vr.variables[varDef.Name] = varDef
}

func (vr *VariableRegistry) Get(name string) (VariableDefinition, error) {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	varDef, exists := vr.variables[name]
	if !exists {
		return VariableDefinition{}, ErrVariableNotFound
	}

	return varDef, nil
}

func (vr *VariableRegistry) GetAll() []VariableDefinition {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	vars := make([]VariableDefinition, 0, len(vr.variables))
	for _, varDef := range vr.variables {
		vars = append(vars, varDef)
	}

	return vars
}

func (m *Manager) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache = make(map[string]string)
	if m.log != nil {
		m.log.Info("personalization cache cleared")
	}
}

func (m *Manager) SetConfig(config *PersonalizationConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = config
}

func (m *Manager) GetConfig() *PersonalizationConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	configCopy := *m.config
	return &configCopy
}
