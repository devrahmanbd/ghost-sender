package validator

import (
    "encoding/base64"
    "encoding/json"
    "fmt"
    "net"
    "net/mail"
    "net/url"
    "path/filepath"
    "reflect"
    "regexp"
    "strings"
    "sync"
    "time"
    "github.com/go-playground/validator/v10"
)

type Validator struct {
	validate *validator.Validate
	mu       sync.RWMutex
}

var (
	defaultValidator     *Validator
	defaultValidatorOnce sync.Once
)

func New() *Validator {
	v := &Validator{
		validate: validator.New(),
	}

	v.validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	v.registerCustomValidators()

	return v
}

func Default() *Validator {
	defaultValidatorOnce.Do(func() {
		defaultValidator = New()
	})
	return defaultValidator
}

func (v *Validator) Validate(data interface{}) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	err := v.validate.Struct(data)
	if err == nil {
		return nil
	}

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		return NewValidationError(validationErrors)
	}

	return err
}

func (v *Validator) ValidateVar(field interface{}, tag string) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	err := v.validate.Var(field, tag)
	if err == nil {
		return nil
	}

	return err
}

func (v *Validator) RegisterValidation(tag string, fn validator.Func) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.validate.RegisterValidation(tag, fn)
}

func (v *Validator) RegisterAlias(alias, tags string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.validate.RegisterAlias(alias, tags)
}

func (v *Validator) registerCustomValidators() {
	v.validate.RegisterValidation("email_dns", validateEmailDNS)
	v.validate.RegisterValidation("email_format", validateEmailFormat)
	v.validate.RegisterValidation("provider", validateProvider)
	v.validate.RegisterValidation("proxy_type", validateProxyType)
	v.validate.RegisterValidation("campaign_status", validateCampaignStatus)
	v.validate.RegisterValidation("rotation_strategy", validateRotationStrategy)
	v.validate.RegisterValidation("port_number", validatePortNumber)
	v.validate.RegisterValidation("host", validateHost)
	v.validate.RegisterValidation("phone", validatePhone)
	v.validate.RegisterValidation("not_blank", validateNotBlank)
	v.validate.RegisterValidation("slug", validateSlug)
	v.validate.RegisterValidation("timezone", validateTimezone)
	v.validate.RegisterValidation("cron", validateCron)
	v.validate.RegisterValidation("telegram_token", validateTelegramToken)
	v.validate.RegisterValidation("ipv4_or_ipv6", validateIPv4OrIPv6)
}

func validateEmailDNS(fl validator.FieldLevel) bool {
	email := fl.Field().String()
	if email == "" {
		return true
	}

	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}

	parts := strings.Split(addr.Address, "@")
	if len(parts) != 2 {
		return false
	}

	domain := parts[1]
	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		return false
	}

	return true
}

func validateEmailFormat(fl validator.FieldLevel) bool {
	email := fl.Field().String()
	if email == "" {
		return true
	}

	_, err := mail.ParseAddress(email)
	return err == nil
}

func validateProvider(fl validator.FieldLevel) bool {
	provider := fl.Field().String()
	validProviders := []string{"gmail", "yahoo", "outlook", "hotmail", "icloud", "workspace", "smtp"}
	
	for _, valid := range validProviders {
		if provider == valid {
			return true
		}
	}
	
	return false
}

func validateProxyType(fl validator.FieldLevel) bool {
	proxyType := fl.Field().String()
	validTypes := []string{"http", "https", "socks5"}
	
	for _, valid := range validTypes {
		if proxyType == valid {
			return true
		}
	}
	
	return false
}

func validateCampaignStatus(fl validator.FieldLevel) bool {
	status := fl.Field().String()
	validStatuses := []string{"created", "scheduled", "running", "paused", "completed", "failed", "cancelled"}
	
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	
	return false
}

func validateRotationStrategy(fl validator.FieldLevel) bool {
	strategy := fl.Field().String()
	validStrategies := []string{"sequential", "random", "weighted", "time_based", "round_robin"}
	
	for _, valid := range validStrategies {
		if strategy == valid {
			return true
		}
	}
	
	return false
}

func validatePortNumber(fl validator.FieldLevel) bool {
	port := fl.Field().Int()
	return port >= 1 && port <= 65535
}


func validateHost(fl validator.FieldLevel) bool {
	host := fl.Field().String()
	if host == "" {
		return false
	}

	if net.ParseIP(host) != nil {
		return true
	}

	if _, err := net.LookupHost(host); err == nil {
		return true
	}

	return false
}

func validatePhone(fl validator.FieldLevel) bool {
	phone := fl.Field().String()
	phoneRegex := regexp.MustCompile(`^[\d\s\+\-\(\)]+$`)
	return phoneRegex.MatchString(phone)
}

func validateNotBlank(fl validator.FieldLevel) bool {
	return strings.TrimSpace(fl.Field().String()) != ""
}

func validateSlug(fl validator.FieldLevel) bool {
	slug := fl.Field().String()
	slugRegex := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	return slugRegex.MatchString(slug)
}

func validateTimezone(fl validator.FieldLevel) bool {
	tz := fl.Field().String()
	_, err := time.LoadLocation(tz)
	return err == nil
}

func validateCron(fl validator.FieldLevel) bool {
	cron := fl.Field().String()
	parts := strings.Fields(cron)
	return len(parts) == 5 || len(parts) == 6
}

func validateTelegramToken(fl validator.FieldLevel) bool {
	token := fl.Field().String()
	tokenRegex := regexp.MustCompile(`^\d+:[A-Za-z0-9_-]+$`)
	return tokenRegex.MatchString(token)
}

func validateIPv4OrIPv6(fl validator.FieldLevel) bool {
	ip := fl.Field().String()
	return net.ParseIP(ip) != nil
}
func ValidateHost(host string) error {
    if host == "" {
        return fmt.Errorf("host cannot be empty")
    }

    if net.ParseIP(host) != nil {
        return nil
    }

    if _, err := net.LookupHost(host); err == nil {
        return nil
    }

    return fmt.Errorf("invalid host: must be a valid IP address or hostname")
}

type ValidationError struct {
	Errors map[string][]string
}

func NewValidationError(validationErrors validator.ValidationErrors) *ValidationError {
	ve := &ValidationError{
		Errors: make(map[string][]string),
	}

	for _, err := range validationErrors {
		field := err.Field()
		message := formatErrorMessage(err)
		ve.Errors[field] = append(ve.Errors[field], message)
	}

	return ve
}

func (ve *ValidationError) Error() string {
	var messages []string
	for field, errs := range ve.Errors {
		for _, err := range errs {
			messages = append(messages, fmt.Sprintf("%s: %s", field, err))
		}
	}
	return strings.Join(messages, "; ")
}

func (ve *ValidationError) HasErrors() bool {
	return len(ve.Errors) > 0
}

func (ve *ValidationError) GetFieldErrors(field string) []string {
	return ve.Errors[field]
}

func formatErrorMessage(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "email_dns":
		return "email domain does not have valid MX records"
	case "email_format":
		return "must be a valid email format"
	case "min":
		return fmt.Sprintf("must be at least %s", err.Param())
	case "max":
		return fmt.Sprintf("must be at most %s", err.Param())
	case "len":
		return fmt.Sprintf("must be exactly %s characters", err.Param())
	case "gt":
		return fmt.Sprintf("must be greater than %s", err.Param())
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", err.Param())
	case "lt":
		return fmt.Sprintf("must be less than %s", err.Param())
	case "lte":
		return fmt.Sprintf("must be less than or equal to %s", err.Param())
	case "url":
		return "must be a valid URL"
	case "uri":
		return "must be a valid URI"
	case "alpha":
		return "must contain only alphabetic characters"
	case "alphanum":
		return "must contain only alphanumeric characters"
	case "numeric":
		return "must be numeric"
	case "uuid":
		return "must be a valid UUID"
	case "oneof":
		return fmt.Sprintf("must be one of: %s", err.Param())
	case "provider":
		return "must be a valid email provider (gmail, yahoo, outlook, etc.)"
	case "proxy_type":
		return "must be a valid proxy type (http, https, socks5)"
	case "campaign_status":
		return "must be a valid campaign status"
	case "rotation_strategy":
		return "must be a valid rotation strategy"
	case "port_number":
		return "must be a valid port number (1-65535)"
	case "host":
		return "must be a valid hostname or IP address"
	case "phone":
		return "must be a valid phone number"
	case "not_blank":
		return "must not be blank"
	case "slug":
		return "must be a valid slug (lowercase letters, numbers, and hyphens)"
	case "timezone":
		return "must be a valid timezone"
	case "cron":
		return "must be a valid cron expression"
	case "telegram_token":
		return "must be a valid Telegram bot token"
	case "ipv4_or_ipv6":
		return "must be a valid IPv4 or IPv6 address"
	default:
		return fmt.Sprintf("failed validation: %s", err.Tag())
	}
}

func ValidateStruct(data interface{}) error {
	return Default().Validate(data)
}

func ValidateVar(field interface{}, tag string) error {
	return Default().ValidateVar(field, tag)
}

func ValidateEmail(email string) error {
	return ValidateVar(email, "required,email")
}

func ValidateEmailWithDNS(email string) error {
	return ValidateVar(email, "required,email,email_dns")
}

func ValidateURL(urlStr string) error {
	if _, err := url.Parse(urlStr); err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	return nil
}

func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func ValidateIPAddress(ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address")
	}
	return nil
}

func ValidateRequired(value interface{}) error {
	if value == nil {
		return fmt.Errorf("value is required")
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		if v.String() == "" {
			return fmt.Errorf("value is required")
		}
	case reflect.Slice, reflect.Map, reflect.Array:
		if v.Len() == 0 {
			return fmt.Errorf("value is required")
		}
	case reflect.Ptr:
		if v.IsNil() {
			return fmt.Errorf("value is required")
		}
	}

	return nil
}

func ValidateMinLength(value string, min int) error {
	if len(value) < min {
		return fmt.Errorf("minimum length is %d characters", min)
	}
	return nil
}

func ValidateMaxLength(value string, max int) error {
	if len(value) > max {
		return fmt.Errorf("maximum length is %d characters", max)
	}
	return nil
}

func ValidateRange(value, min, max int) error {
	if value < min || value > max {
		return fmt.Errorf("value must be between %d and %d", min, max)
	}
	return nil
}

func ValidateIn(value string, allowed []string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("value must be one of: %s", strings.Join(allowed, ", "))
}

func ValidateNotIn(value string, disallowed []string) error {
	for _, d := range disallowed {
		if value == d {
			return fmt.Errorf("value cannot be: %s", value)
		}
	}
	return nil
}

func ValidateRegex(value, pattern string) error {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	if !regex.MatchString(value) {
		return fmt.Errorf("value does not match pattern")
	}

	return nil
}

func ValidateAlphanumeric(value string) error {
	alphanumRegex := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	if !alphanumRegex.MatchString(value) {
		return fmt.Errorf("value must be alphanumeric")
	}
	return nil
}

func ValidateAlpha(value string) error {
	alphaRegex := regexp.MustCompile(`^[a-zA-Z]+$`)
	if !alphaRegex.MatchString(value) {
		return fmt.Errorf("value must contain only letters")
	}
	return nil
}

func ValidateNumeric(value string) error {
	numericRegex := regexp.MustCompile(`^[0-9]+$`)
	if !numericRegex.MatchString(value) {
		return fmt.Errorf("value must be numeric")
	}
	return nil
}

func ValidateDomain(domain string) error {
	domainRegex := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	if !domainRegex.MatchString(domain) {
		return fmt.Errorf("invalid domain name")
	}
	return nil
}

func ValidateFilePath(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("path cannot contain '..'")
	}
	if strings.Contains(path, "~") {
		return fmt.Errorf("path cannot contain '~'")
	}
	return nil
}

func ValidateFileExtension(filename string, allowedExtensions []string) error {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	
	for _, allowed := range allowedExtensions {
		if ext == strings.ToLower(allowed) {
			return nil
		}
	}
	
	return fmt.Errorf("file extension must be one of: %s", strings.Join(allowedExtensions, ", "))
}

func ValidateUUID(uuid string) error {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !uuidRegex.MatchString(uuid) {
		return fmt.Errorf("invalid UUID format")
	}
	return nil
}

func ValidateJSON(data string) error {
	var js interface{}
	if err := json.Unmarshal([]byte(data), &js); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func ValidateBase64(data string) error {
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		return fmt.Errorf("invalid base64: %w", err)
	}
	return nil
}

func ValidateHexColor(color string) error {
	hexColorRegex := regexp.MustCompile(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`)
	if !hexColorRegex.MatchString(color) {
		return fmt.Errorf("invalid hex color format")
	}
	return nil
}

func ValidateDate(date string, layout string) error {
	if _, err := time.Parse(layout, date); err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}
	return nil
}

func ValidateDateAfter(date string, after time.Time, layout string) error {
	d, err := time.Parse(layout, date)
	if err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}
	
	if !d.After(after) {
		return fmt.Errorf("date must be after %s", after.Format(layout))
	}
	
	return nil
}

func ValidateDateBefore(date string, before time.Time, layout string) error {
	d, err := time.Parse(layout, date)
	if err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}
	
	if !d.Before(before) {
		return fmt.Errorf("date must be before %s", before.Format(layout))
	}
	
	return nil
}

func ValidateUnique(values []string) error {
	seen := make(map[string]bool)
	for _, value := range values {
		if seen[value] {
			return fmt.Errorf("duplicate value found: %s", value)
		}
		seen[value] = true
	}
	return nil
}

func ValidateHTMLContent(html string) error {
	if strings.TrimSpace(html) == "" {
		return fmt.Errorf("HTML content cannot be empty")
	}
	
	if !strings.Contains(html, "<") || !strings.Contains(html, ">") {
		return fmt.Errorf("invalid HTML content")
	}
	
	return nil
}

func ValidateSMTPHost(host string, port int) error {
	if err := ValidateHost(host); err != nil {
		return err
	}
	
	if err := ValidatePort(port); err != nil {
		return err
	}
	
	address := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("cannot connect to SMTP server: %w", err)
	}
	conn.Close()
	
	return nil
}

func ValidateProxyURL(proxyURL string) error {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}
	
	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" {
		return fmt.Errorf("proxy scheme must be http, https, or socks5")
	}
	
	if u.Host == "" {
		return fmt.Errorf("proxy host is required")
	}
	
	return nil
}

