package recipient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"email-campaign-system/internal/models"
)

var (
	ErrInvalidEmailFormat  = errors.New("invalid email format")
	ErrInvalidEmailSyntax  = errors.New("invalid email syntax")
	ErrDomainNotFound      = errors.New("domain not found")
	ErrNoMXRecords         = errors.New("no MX records found")
	ErrDisposableEmail     = errors.New("disposable email address")
	ErrBlacklistedEmail    = errors.New("email is blacklisted")
	ErrBlacklistedDomain   = errors.New("domain is blacklisted")
	ErrInvalidDomain       = errors.New("invalid domain")
	ErrEmptyEmail          = errors.New("email address is empty")
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	localRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+$`)
	domainRegex = regexp.MustCompile(`^[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
)

type Validator struct {
	config          *ValidationConfig
	dnsCache        map[string]*DNSCacheEntry
	dnsCacheMu      sync.RWMutex
	blacklist       map[string]bool
	blacklistMu     sync.RWMutex
	whitelist       map[string]bool
	whitelistMu     sync.RWMutex
	disposableDomains map[string]bool
	disposableMu    sync.RWMutex
}

type ValidationConfig struct {
	EnableDNSCheck        bool
	EnableMXCheck         bool
	EnableDisposableCheck bool
	EnableBlacklist       bool
	EnableWhitelist       bool
	StrictMode            bool
	DNSTimeout            time.Duration
	CacheDNSResults       bool
	CacheTTL              time.Duration
	MaxEmailLength        int
	MaxLocalLength        int
	MaxDomainLength       int
	AllowIPAddress        bool
	AllowSubaddressing    bool
	ConcurrentValidation  int
}

type DNSCacheEntry struct {
	Domain     string
	HasMX      bool
	MXRecords  []*net.MX
	ValidatedAt time.Time
	ExpiresAt  time.Time
}

type ValidationResult struct {
	Valid          bool
	Email          string
	Errors         []error
	Warnings       []string
	FormatValid    bool
	SyntaxValid    bool
	DNSValid       bool
	MXValid        bool
	IsDisposable   bool
	IsBlacklisted  bool
	IsWhitelisted  bool
	Domain         string
	LocalPart      string
	ValidatedAt    time.Time
}

type BatchValidationResult struct {
	TotalEmails    int
	ValidEmails    int
	InvalidEmails  int
	Results        map[string]*ValidationResult
	Duration       time.Duration
	StartedAt      time.Time
	CompletedAt    time.Time
}

func NewValidator() *Validator {
	return &Validator{
		config:            DefaultValidationConfig(),
		dnsCache:          make(map[string]*DNSCacheEntry),
		blacklist:         make(map[string]bool),
		whitelist:         make(map[string]bool),
		disposableDomains: LoadDisposableDomains(),
	}
}

func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		EnableDNSCheck:        true,
		EnableMXCheck:         true,
		EnableDisposableCheck: true,
		EnableBlacklist:       true,
		EnableWhitelist:       false,
		StrictMode:            false,
		DNSTimeout:            5 * time.Second,
		CacheDNSResults:       true,
		CacheTTL:              1 * time.Hour,
		MaxEmailLength:        254,
		MaxLocalLength:        64,
		MaxDomainLength:       255,
		AllowIPAddress:        false,
		AllowSubaddressing:    true,
		ConcurrentValidation:  10,
	}
}

func (v *Validator) ValidateEmail(email string) error {
	result := v.Validate(email)
	if !result.Valid {
		if len(result.Errors) > 0 {
			return result.Errors[0]
		}
		return ErrInvalidEmailFormat
	}
	return nil
}

func (v *Validator) Validate(email string) *ValidationResult {
	result := &ValidationResult{
		Email:       email,
		Valid:       true,
		Errors:      make([]error, 0),
		Warnings:    make([]string, 0),
		ValidatedAt: time.Now(),
	}

	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ErrEmptyEmail)
		return result
	}

	if err := v.validateFormat(email, result); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
		return result
	}

	if err := v.validateSyntax(email, result); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err)
		return result
	}

	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		result.LocalPart = parts[0]
		result.Domain = parts[1]
	}

	if v.config.EnableWhitelist && v.IsWhitelisted(email) {
		result.IsWhitelisted = true
		return result
	}

	if v.config.EnableBlacklist {
		if v.IsBlacklisted(email) {
			result.Valid = false
			result.IsBlacklisted = true
			result.Errors = append(result.Errors, ErrBlacklistedEmail)
			return result
		}
		if v.IsDomainBlacklisted(result.Domain) {
			result.Valid = false
			result.IsBlacklisted = true
			result.Errors = append(result.Errors, ErrBlacklistedDomain)
			return result
		}
	}

	if v.config.EnableDisposableCheck {
		if v.IsDisposableEmail(result.Domain) {
			result.IsDisposable = true
			if v.config.StrictMode {
				result.Valid = false
				result.Errors = append(result.Errors, ErrDisposableEmail)
				return result
			}
			result.Warnings = append(result.Warnings, "disposable email domain")
		}
	}

	if v.config.EnableDNSCheck || v.config.EnableMXCheck {
		if err := v.validateDNS(result.Domain, result); err != nil {
			if v.config.StrictMode {
				result.Valid = false
				result.Errors = append(result.Errors, err)
				return result
			}
			result.Warnings = append(result.Warnings, err.Error())
		}
	}

	return result
}

func (v *Validator) validateFormat(email string, result *ValidationResult) error {
	if len(email) > v.config.MaxEmailLength {
		return fmt.Errorf("email exceeds maximum length: %d", v.config.MaxEmailLength)
	}

	if !emailRegex.MatchString(email) {
		return ErrInvalidEmailFormat
	}

	result.FormatValid = true
	return nil
}

func (v *Validator) validateSyntax(email string, result *ValidationResult) error {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ErrInvalidEmailSyntax
	}

	localPart := parts[0]
	domain := parts[1]

	if len(localPart) == 0 || len(localPart) > v.config.MaxLocalLength {
		return fmt.Errorf("invalid local part length: %d", len(localPart))
	}

	if len(domain) == 0 || len(domain) > v.config.MaxDomainLength {
		return fmt.Errorf("invalid domain length: %d", len(domain))
	}

	if !v.config.AllowSubaddressing && strings.Contains(localPart, "+") {
		return errors.New("subaddressing not allowed")
	}

	if strings.HasPrefix(localPart, ".") || strings.HasSuffix(localPart, ".") {
		return errors.New("local part cannot start or end with dot")
	}

	if strings.Contains(localPart, "..") {
		return errors.New("local part cannot contain consecutive dots")
	}

	if !domainRegex.MatchString(domain) {
		return ErrInvalidDomain
	}

	result.SyntaxValid = true
	return nil
}

func (v *Validator) validateDNS(domain string, result *ValidationResult) error {
	if cached := v.getCachedDNS(domain); cached != nil {
		result.DNSValid = true
		result.MXValid = cached.HasMX
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), v.config.DNSTimeout)
	defer cancel()

	resolver := &net.Resolver{}

	if v.config.EnableMXCheck {
		mxRecords, err := resolver.LookupMX(ctx, domain)
		if err != nil {
			if v.config.CacheDNSResults {
				v.cacheDNS(domain, false, nil)
			}
			result.DNSValid = false
			result.MXValid = false
			return ErrNoMXRecords
		}

		if len(mxRecords) == 0 {
			if v.config.CacheDNSResults {
				v.cacheDNS(domain, false, nil)
			}
			result.DNSValid = false
			result.MXValid = false
			return ErrNoMXRecords
		}

		if v.config.CacheDNSResults {
			v.cacheDNS(domain, true, mxRecords)
		}

		result.DNSValid = true
		result.MXValid = true
		return nil
	}

	ips, err := resolver.LookupHost(ctx, domain)
	if err != nil || len(ips) == 0 {
		result.DNSValid = false
		return ErrDomainNotFound
	}

	result.DNSValid = true
	return nil
}

func (v *Validator) ValidateRecipient(recipient *models.Recipient) error {
	if recipient == nil {
		return errors.New("recipient is nil")
	}

	if recipient.Email == "" {
		return ErrEmptyEmail
	}

	if err := v.ValidateEmail(recipient.Email); err != nil {
		return err
	}

	if len(recipient.Email) > v.config.MaxEmailLength {
		return fmt.Errorf("email exceeds maximum length")
	}

	return nil
}

func (v *Validator) ValidateBatch(emails []string) *BatchValidationResult {
	startTime := time.Now()

	result := &BatchValidationResult{
		TotalEmails: len(emails),
		Results:     make(map[string]*ValidationResult),
		StartedAt:   startTime,
	}

	sem := make(chan struct{}, v.config.ConcurrentValidation)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, email := range emails {
		wg.Add(1)
		go func(e string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			validationResult := v.Validate(e)

			mu.Lock()
			result.Results[e] = validationResult
			if validationResult.Valid {
				result.ValidEmails++
			} else {
				result.InvalidEmails++
			}
			mu.Unlock()
		}(email)
	}

	wg.Wait()

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	return result
}

func (v *Validator) IsDisposableEmail(domain string) bool {
	v.disposableMu.RLock()
	defer v.disposableMu.RUnlock()

	domain = strings.ToLower(domain)
	return v.disposableDomains[domain]
}

func (v *Validator) IsBlacklisted(email string) bool {
	v.blacklistMu.RLock()
	defer v.blacklistMu.RUnlock()

	email = strings.ToLower(email)
	return v.blacklist[email]
}

func (v *Validator) IsDomainBlacklisted(domain string) bool {
	v.blacklistMu.RLock()
	defer v.blacklistMu.RUnlock()

	domain = strings.ToLower(domain)
	return v.blacklist[domain]
}

func (v *Validator) IsWhitelisted(email string) bool {
	v.whitelistMu.RLock()
	defer v.whitelistMu.RUnlock()

	email = strings.ToLower(email)
	return v.whitelist[email]
}

func (v *Validator) AddToBlacklist(email string) {
	v.blacklistMu.Lock()
	defer v.blacklistMu.Unlock()

	v.blacklist[strings.ToLower(email)] = true
}

func (v *Validator) RemoveFromBlacklist(email string) {
	v.blacklistMu.Lock()
	defer v.blacklistMu.Unlock()

	delete(v.blacklist, strings.ToLower(email))
}

func (v *Validator) AddToWhitelist(email string) {
	v.whitelistMu.Lock()
	defer v.whitelistMu.Unlock()

	v.whitelist[strings.ToLower(email)] = true
}

func (v *Validator) RemoveFromWhitelist(email string) {
	v.whitelistMu.Lock()
	defer v.whitelistMu.Unlock()

	delete(v.whitelist, strings.ToLower(email))
}

func (v *Validator) cacheDNS(domain string, hasMX bool, mxRecords []*net.MX) {
	if !v.config.CacheDNSResults {
		return
	}

	v.dnsCacheMu.Lock()
	defer v.dnsCacheMu.Unlock()

	entry := &DNSCacheEntry{
		Domain:      domain,
		HasMX:       hasMX,
		MXRecords:   mxRecords,
		ValidatedAt: time.Now(),
		ExpiresAt:   time.Now().Add(v.config.CacheTTL),
	}

	v.dnsCache[domain] = entry
}

func (v *Validator) getCachedDNS(domain string) *DNSCacheEntry {
	if !v.config.CacheDNSResults {
		return nil
	}

	v.dnsCacheMu.RLock()
	defer v.dnsCacheMu.RUnlock()

	entry, exists := v.dnsCache[domain]
	if !exists {
		return nil
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil
	}

	return entry
}

func (v *Validator) ClearDNSCache() {
	v.dnsCacheMu.Lock()
	defer v.dnsCacheMu.Unlock()

	v.dnsCache = make(map[string]*DNSCacheEntry)
}

func (v *Validator) SetConfig(config *ValidationConfig) {
	v.config = config
}

func (v *Validator) GetConfig() *ValidationConfig {
	return v.config
}

func (v *Validator) GetCacheStats() map[string]interface{} {
	v.dnsCacheMu.RLock()
	defer v.dnsCacheMu.RUnlock()

	return map[string]interface{}{
		"dns_cache_size": len(v.dnsCache),
		"blacklist_size": len(v.blacklist),
		"whitelist_size": len(v.whitelist),
	}
}

func LoadDisposableDomains() map[string]bool {
	domains := map[string]bool{
		"tempmail.com":     true,
		"guerrillamail.com": true,
		"10minutemail.com": true,
		"throwaway.email":  true,
		"temp-mail.org":    true,
		"getnada.com":      true,
		"mailinator.com":   true,
		"maildrop.cc":      true,
		"mailnesia.com":    true,
		"trashmail.com":    true,
		"sharklasers.com":  true,
		"yopmail.com":      true,
		"fakeinbox.com":    true,
		"dispostable.com":  true,
		"tempr.email":      true,
		"emailondeck.com":  true,
		"mohmal.com":       true,
		"mytemp.email":     true,
		"harakirimail.com": true,
		"spambog.com":      true,
	}

	return domains
}

func (v *Validator) AddDisposableDomain(domain string) {
	v.disposableMu.Lock()
	defer v.disposableMu.Unlock()

	v.disposableDomains[strings.ToLower(domain)] = true
}

func (v *Validator) RemoveDisposableDomain(domain string) {
	v.disposableMu.Lock()
	defer v.disposableMu.Unlock()

	delete(v.disposableDomains, strings.ToLower(domain))
}

func (v *Validator) ExtractDomain(email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", ErrInvalidEmailFormat
	}
	return parts[1], nil
}

func (v *Validator) ExtractLocalPart(email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", ErrInvalidEmailFormat
	}
	return parts[0], nil
}

func (v *Validator) NormalizeEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))

	if !v.config.AllowSubaddressing {
		parts := strings.Split(email, "@")
		if len(parts) == 2 {
			localPart := parts[0]
			if idx := strings.Index(localPart, "+"); idx != -1 {
				localPart = localPart[:idx]
			}
			email = localPart + "@" + parts[1]
		}
	}

	return email
}

