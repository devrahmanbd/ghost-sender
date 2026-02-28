package validator

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

type CampaignValidator struct {
	minRecipients int
	maxRecipients int
	minWorkers    int
	maxWorkers    int
}

func NewCampaignValidator() *CampaignValidator {
	return &CampaignValidator{
		minRecipients: 1,
		maxRecipients: 1000000,
		minWorkers:    1,
		maxWorkers:    32,
	}
}

func (cv *CampaignValidator) ValidateCampaignName(name string) error {
	if err := ValidateRequired(name); err != nil {
		return err
	}

	if err := ValidateMinLength(name, 3); err != nil {
		return err
	}

	if err := ValidateMaxLength(name, 100); err != nil {
		return err
	}

	nameRegex := regexp.MustCompile(`^[a-zA-Z0-9\s\-_]+$`)
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("campaign name can only contain letters, numbers, spaces, hyphens, and underscores")
	}

	return nil
}

func (cv *CampaignValidator) ValidateRecipientCount(count int) error {
	if count < cv.minRecipients {
		return fmt.Errorf("minimum %d recipient required", cv.minRecipients)
	}

	if count > cv.maxRecipients {
		return fmt.Errorf("maximum %d recipients allowed", cv.maxRecipients)
	}

	return nil
}

func (cv *CampaignValidator) ValidateWorkerCount(count int) error {
	if count < cv.minWorkers {
		return fmt.Errorf("minimum %d worker required", cv.minWorkers)
	}

	if count > cv.maxWorkers {
		return fmt.Errorf("maximum %d workers allowed", cv.maxWorkers)
	}

	return nil
}

func (cv *CampaignValidator) ValidateScheduledTime(scheduledTime time.Time) error {
	if scheduledTime.Before(time.Now()) {
		return fmt.Errorf("scheduled time cannot be in the past")
	}

	maxFuture := time.Now().Add(365 * 24 * time.Hour)
	if scheduledTime.After(maxFuture) {
		return fmt.Errorf("scheduled time cannot be more than 1 year in the future")
	}

	return nil
}

type TemplateValidator struct {
	maxSizeBytes      int64
	allowedExtensions []string
}

func NewTemplateValidator() *TemplateValidator {
	return &TemplateValidator{
		maxSizeBytes:      10 * 1024 * 1024,
		allowedExtensions: []string{"html", "htm"},
	}
}

func (tv *TemplateValidator) ValidateHTMLTemplate(content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("template content cannot be empty")
	}

	if len(content) > int(tv.maxSizeBytes) {
		return fmt.Errorf("template size exceeds maximum allowed size")
	}

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("invalid HTML: %w", err)
	}

	if doc == nil {
		return fmt.Errorf("failed to parse HTML template")
	}

	return nil
}

func (tv *TemplateValidator) ValidateTemplateVariables(content string) error {
	variableRegex := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	matches := variableRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		variable := strings.TrimSpace(match[1])
		if variable == "" {
			return fmt.Errorf("empty variable placeholder found")
		}

		if !isValidVariableName(variable) {
			return fmt.Errorf("invalid variable name: %s", variable)
		}
	}

	return nil
}

func (tv *TemplateValidator) ValidateSpamScore(score float64) error {
	if score < 0 || score > 100 {
		return fmt.Errorf("spam score must be between 0 and 100")
	}

	return nil
}

func (tv *TemplateValidator) ValidateTemplateSize(sizeBytes int64) error {
	if sizeBytes <= 0 {
		return fmt.Errorf("template size must be greater than 0")
	}

	if sizeBytes > tv.maxSizeBytes {
		return fmt.Errorf("template size %d bytes exceeds maximum %d bytes", sizeBytes, tv.maxSizeBytes)
	}

	return nil
}

type RecipientValidator struct {
	verifyDNS   bool
	verifyMX    bool
	checkSyntax bool
}

func NewRecipientValidator(verifyDNS, verifyMX bool) *RecipientValidator {
	return &RecipientValidator{
		verifyDNS:   verifyDNS,
		verifyMX:    verifyMX,
		checkSyntax: true,
	}
}

func (rv *RecipientValidator) ValidateRecipientEmail(email string) error {
	email = strings.TrimSpace(email)

	if email == "" {
		return fmt.Errorf("email address cannot be empty")
	}

	if rv.checkSyntax {
		addr, err := mail.ParseAddress(email)
		if err != nil {
			return fmt.Errorf("invalid email format: %w", err)
		}
		email = addr.Address
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid email format")
	}

	localPart := parts[0]
	domain := parts[1]

	if err := validateLocalPart(localPart); err != nil {
		return err
	}

	if err := validateDomainPart(domain); err != nil {
		return err
	}

	if rv.verifyDNS {
		if _, err := net.LookupHost(domain); err != nil {
			return fmt.Errorf("domain DNS lookup failed: %w", err)
		}
	}

	if rv.verifyMX {
		mxRecords, err := net.LookupMX(domain)
		if err != nil || len(mxRecords) == 0 {
			return fmt.Errorf("domain has no valid MX records")
		}
	}

	return nil
}

func (rv *RecipientValidator) ValidateRecipientList(emails []string) ([]string, []error) {
	validEmails := make([]string, 0)
	errors := make([]error, 0)

	for _, email := range emails {
		if err := rv.ValidateRecipientEmail(email); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", email, err))
		} else {
			validEmails = append(validEmails, email)
		}
	}

	return validEmails, errors
}

func (rv *RecipientValidator) ValidateCSVFormat(content string) error {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return fmt.Errorf("CSV file is empty")
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !emailRegex.MatchString(line) {
			return fmt.Errorf("invalid email format at line %d: %s", i+1, line)
		}
	}

	return nil
}

type AccountValidator struct {
	testConnection bool
}

func NewAccountValidator(testConnection bool) *AccountValidator {
	return &AccountValidator{
		testConnection: testConnection,
	}
}

func (av *AccountValidator) ValidateEmailAccount(email, password, provider string) error {
	if err := ValidateEmail(email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}

	if err := ValidateRequired(password); err != nil {
		return fmt.Errorf("password is required")
	}

	if err := ValidateMinLength(password, 6); err != nil {
		return err
	}

	validProviders := []string{"gmail", "yahoo", "outlook", "hotmail", "icloud", "workspace", "smtp"}
	if err := ValidateIn(provider, validProviders); err != nil {
		return err
	}

	return nil
}

func (av *AccountValidator) ValidateSMTPConfig(host string, port int, username, password string, useTLS bool) error {
	if err := ValidateHost(host); err != nil {
		return err
	}

	if err := ValidatePort(port); err != nil {
		return err
	}

	if err := ValidateRequired(username); err != nil {
		return fmt.Errorf("username is required")
	}

	if err := ValidateRequired(password); err != nil {
		return fmt.Errorf("password is required")
	}

	if av.testConnection {
		return av.testSMTPConnection(host, port, username, password, useTLS)
	}

	return nil
}

func (av *AccountValidator) testSMTPConnection(host string, port int, username, password string, useTLS bool) error {
	address := fmt.Sprintf("%s:%d", host, port)

	client, err := smtp.Dial(address)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer client.Close()

	if useTLS {
		if err := client.StartTLS(nil); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	auth := smtp.PlainAuth("", username, password, host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}

func (av *AccountValidator) ValidateOAuth2Token(token string) error {
	if err := ValidateRequired(token); err != nil {
		return fmt.Errorf("OAuth2 token is required")
	}

	tokenParts := strings.Split(token, ".")
	if len(tokenParts) != 3 {
		return fmt.Errorf("invalid OAuth2 token format")
	}

	for _, part := range tokenParts {
		if _, err := base64.RawURLEncoding.DecodeString(part); err != nil {
			return fmt.Errorf("invalid OAuth2 token encoding: %w", err)
		}
	}

	return nil
}

func (av *AccountValidator) ValidateDailyLimit(limit int) error {
	if limit < 1 {
		return fmt.Errorf("daily limit must be at least 1")
	}

	if limit > 10000 {
		return fmt.Errorf("daily limit cannot exceed 10000")
	}

	return nil
}

func (av *AccountValidator) ValidateRotationLimit(limit int) error {
	if limit < 1 {
		return fmt.Errorf("rotation limit must be at least 1")
	}

	if limit > 1000 {
		return fmt.Errorf("rotation limit cannot exceed 1000")
	}

	return nil
}

type ProxyValidator struct{}

func NewProxyValidator() *ProxyValidator {
	return &ProxyValidator{}
}

func (pv *ProxyValidator) ValidateProxyConfig(proxyType, host string, port int, username, password string) error {
	validTypes := []string{"http", "https", "socks5"}
	if err := ValidateIn(proxyType, validTypes); err != nil {
		return err
	}

	if err := ValidateHost(host); err != nil {
		return err
	}

	if err := ValidatePort(port); err != nil {
		return err
	}

	return nil
}

func (pv *ProxyValidator) ValidateProxyURL(proxyURL string) error {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}

	validSchemes := []string{"http", "https", "socks5"}
	if err := ValidateIn(u.Scheme, validSchemes); err != nil {
		return fmt.Errorf("invalid proxy scheme: %w", err)
	}

	if u.Host == "" {
		return fmt.Errorf("proxy host is required")
	}

	return nil
}

func (pv *ProxyValidator) TestProxyConnection(proxyURL string, timeout time.Duration) error {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}

	conn, err := net.DialTimeout("tcp", u.Host, timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to proxy: %w", err)
	}
	defer conn.Close()

	return nil
}

type AttachmentValidator struct {
	maxSizeBytes      int64
	allowedFormats    []string
	allowedMimeTypes  []string
}

func NewAttachmentValidator() *AttachmentValidator {
	return &AttachmentValidator{
		maxSizeBytes: 25 * 1024 * 1024,
		allowedFormats: []string{"pdf", "jpg", "jpeg", "png", "webp", "heic", "heif"},
		allowedMimeTypes: []string{
			"application/pdf",
			"image/jpeg",
			"image/png",
			"image/webp",
			"image/heic",
			"image/heif",
		},
	}
}

func (av *AttachmentValidator) ValidateAttachmentFormat(format string) error {
	format = strings.ToLower(strings.TrimPrefix(format, "."))
	return ValidateIn(format, av.allowedFormats)
}

func (av *AttachmentValidator) ValidateAttachmentSize(sizeBytes int64) error {
	if sizeBytes <= 0 {
		return fmt.Errorf("attachment size must be greater than 0")
	}

	if sizeBytes > av.maxSizeBytes {
		return fmt.Errorf("attachment size %d bytes exceeds maximum %d bytes", sizeBytes, av.maxSizeBytes)
	}

	return nil
}

func (av *AttachmentValidator) ValidateMimeType(mimeType string) error {
	return ValidateIn(mimeType, av.allowedMimeTypes)
}

type RateLimitValidator struct{}

func NewRateLimitValidator() *RateLimitValidator {
	return &RateLimitValidator{}
}

func (rlv *RateLimitValidator) ValidateRateLimit(rate int, period time.Duration) error {
	if rate < 1 {
		return fmt.Errorf("rate must be at least 1")
	}

	if rate > 10000 {
		return fmt.Errorf("rate cannot exceed 10000")
	}

	if period < time.Second {
		return fmt.Errorf("period must be at least 1 second")
	}

	if period > 24*time.Hour {
		return fmt.Errorf("period cannot exceed 24 hours")
	}

	return nil
}

func (rlv *RateLimitValidator) ValidateRequestsPerSecond(rps float64) error {
	if rps <= 0 {
		return fmt.Errorf("requests per second must be greater than 0")
	}

	if rps > 100 {
		return fmt.Errorf("requests per second cannot exceed 100")
	}

	return nil
}

func (rlv *RateLimitValidator) ValidateRetryDelay(delay time.Duration) error {
	if delay < 0 {
		return fmt.Errorf("retry delay cannot be negative")
	}

	if delay > 1*time.Hour {
		return fmt.Errorf("retry delay cannot exceed 1 hour")
	}

	return nil
}

type NotificationValidator struct{}

func NewNotificationValidator() *NotificationValidator {
	return &NotificationValidator{}
}

func (nv *NotificationValidator) ValidateTelegramBotToken(token string) error {
	if err := ValidateRequired(token); err != nil {
		return err
	}

	tokenRegex := regexp.MustCompile(`^\d{8,10}:[A-Za-z0-9_-]{35}$`)
	if !tokenRegex.MatchString(token) {
		return fmt.Errorf("invalid Telegram bot token format")
	}

	return nil
}

func (nv *NotificationValidator) ValidateTelegramChatID(chatID string) error {
	if err := ValidateRequired(chatID); err != nil {
		return err
	}

	chatIDRegex := regexp.MustCompile(`^-?\d+$`)
	if !chatIDRegex.MatchString(chatID) {
		return fmt.Errorf("invalid Telegram chat ID format")
	}

	return nil
}

type RotationValidator struct{}

func NewRotationValidator() *RotationValidator {
	return &RotationValidator{}
}

func (rv *RotationValidator) ValidateRotationStrategy(strategy string) error {
	validStrategies := []string{"sequential", "random", "weighted", "time_based", "round_robin", "health_based"}
	return ValidateIn(strategy, validStrategies)
}

func (rv *RotationValidator) ValidateRotationWeights(weights map[string]int) error {
	if len(weights) == 0 {
		return fmt.Errorf("at least one weight must be specified")
	}

	totalWeight := 0
	for name, weight := range weights {
		if weight < 0 {
			return fmt.Errorf("weight for %s cannot be negative", name)
		}
		if weight > 100 {
			return fmt.Errorf("weight for %s cannot exceed 100", name)
		}
		totalWeight += weight
	}

	if totalWeight == 0 {
		return fmt.Errorf("total weight must be greater than 0")
	}

	return nil
}

func validateLocalPart(localPart string) error {
	if len(localPart) == 0 {
		return fmt.Errorf("local part cannot be empty")
	}

	if len(localPart) > 64 {
		return fmt.Errorf("local part cannot exceed 64 characters")
	}

	if strings.HasPrefix(localPart, ".") || strings.HasSuffix(localPart, ".") {
		return fmt.Errorf("local part cannot start or end with a dot")
	}

	if strings.Contains(localPart, "..") {
		return fmt.Errorf("local part cannot contain consecutive dots")
	}

	return nil
}

func validateDomainPart(domain string) error {
	if len(domain) == 0 {
		return fmt.Errorf("domain cannot be empty")
	}

	if len(domain) > 255 {
		return fmt.Errorf("domain cannot exceed 255 characters")
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return fmt.Errorf("domain must have at least two labels")
	}

	for _, label := range labels {
		if len(label) == 0 {
			return fmt.Errorf("domain label cannot be empty")
		}

		if len(label) > 63 {
			return fmt.Errorf("domain label cannot exceed 63 characters")
		}

		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("domain label cannot start or end with hyphen")
		}
	}

	return nil
}

func isValidVariableName(name string) bool {
	if name == "" {
		return false
	}

	validVarRegex := regexp.MustCompile(`^[A-Z_][A-Z0-9_]*(\.[A-Z_][A-Z0-9_]*)*$`)
	return validVarRegex.MatchString(name)
}

func ValidateFileUpload(filename string, sizeBytes int64, allowedExtensions []string, maxSizeBytes int64) error {
	if err := ValidateRequired(filename); err != nil {
		return fmt.Errorf("filename is required")
	}

	if err := ValidateFilePath(filename); err != nil {
		return err
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	if err := ValidateIn(ext, allowedExtensions); err != nil {
		return fmt.Errorf("invalid file extension: %s (allowed: %s)", ext, strings.Join(allowedExtensions, ", "))
	}

	if sizeBytes <= 0 {
		return fmt.Errorf("file size must be greater than 0")
	}

	if sizeBytes > maxSizeBytes {
		return fmt.Errorf("file size %d bytes exceeds maximum %d bytes", sizeBytes, maxSizeBytes)
	}

	return nil
}

func ValidateZIPStructure(files []string, requiredFiles []string) error {
	if len(files) == 0 {
		return fmt.Errorf("ZIP file is empty")
	}

	fileSet := make(map[string]bool)
	for _, file := range files {
		fileSet[file] = true
	}

	for _, required := range requiredFiles {
		if !fileSet[required] {
			return fmt.Errorf("required file not found in ZIP: %s", required)
		}
	}

	return nil
}

func ValidatePassword(password string) error {
	if err := ValidateMinLength(password, 8); err != nil {
		return err
	}

	if err := ValidateMaxLength(password, 128); err != nil {
		return err
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}

	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}

	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}

	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}

	return nil
}

func ValidateJSONString(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		return fmt.Errorf("JSON string cannot be empty")
	}

	var js interface{}
	if err := json.Unmarshal([]byte(jsonStr), &js); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return nil
}

func ValidateCronExpression(cron string) error {
	if err := ValidateRequired(cron); err != nil {
		return err
	}

	parts := strings.Fields(cron)
	if len(parts) != 5 && len(parts) != 6 {
		return fmt.Errorf("cron expression must have 5 or 6 fields")
	}

	return nil
}

func ValidateTimeRange(start, end time.Time) error {
	if start.IsZero() || end.IsZero() {
		return fmt.Errorf("start and end times cannot be zero")
	}

	if start.After(end) {
		return fmt.Errorf("start time must be before end time")
	}

	return nil
}

