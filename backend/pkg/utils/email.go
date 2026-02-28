package utils

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidEmail       = errors.New("invalid email address")
	ErrInvalidDomain      = errors.New("invalid email domain")
	ErrDisposableEmail    = errors.New("disposable email address")
	ErrDomainNotFound     = errors.New("domain does not exist")
	ErrNoMXRecords        = errors.New("no MX records found for domain")
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	
	disposableDomains = map[string]bool{
		"tempmail.com": true, "guerrillamail.com": true, "10minutemail.com": true,
		"mailinator.com": true, "throwaway.email": true, "temp-mail.org": true,
		"trashmail.com": true, "yopmail.com": true, "fakeinbox.com": true,
		"maildrop.cc": true, "getnada.com": true, "mohmal.com": true,
	}

	providerDomains = map[string]string{
		"gmail.com":       "gmail",
		"googlemail.com":  "gmail",
		"yahoo.com":       "yahoo",
		"yahoo.co.uk":     "yahoo",
		"yahoo.fr":        "yahoo",
		"hotmail.com":     "outlook",
		"outlook.com":     "outlook",
		"live.com":        "outlook",
		"msn.com":         "outlook",
		"icloud.com":      "icloud",
		"me.com":          "icloud",
		"mac.com":         "icloud",
		"protonmail.com":  "protonmail",
		"proton.me":       "protonmail",
	}
)

type EmailAddress struct {
	Local    string
	Domain   string
	Full     string
	Provider string
}

func ValidateEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 {
		return false
	}

	return emailRegex.MatchString(email)
}

func ValidateEmailStrict(email string) error {
	if !ValidateEmail(email) {
		return ErrInvalidEmail
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ErrInvalidEmail
	}

	local, domain := parts[0], parts[1]

	if len(local) > 64 {
		return ErrInvalidEmail
	}

	if len(domain) > 255 {
		return ErrInvalidDomain
	}

	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") {
		return ErrInvalidEmail
	}

	if strings.Contains(local, "..") {
		return ErrInvalidEmail
	}

	domainParts := strings.Split(domain, ".")
	if len(domainParts) < 2 {
		return ErrInvalidDomain
	}

	for _, part := range domainParts {
		if len(part) == 0 || len(part) > 63 {
			return ErrInvalidDomain
		}
	}

	return nil
}

func ParseEmail(email string) (*EmailAddress, error) {
	email = NormalizeEmail(email)

	if !ValidateEmail(email) {
		return nil, ErrInvalidEmail
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return nil, ErrInvalidEmail
	}

	local := parts[0]
	domain := strings.ToLower(parts[1])

	provider := DetectProvider(domain)

	return &EmailAddress{
		Local:    local,
		Domain:   domain,
		Full:     email,
		Provider: provider,
	}, nil
}

func NormalizeEmail(email string) string {
	email = strings.TrimSpace(email)
	email = strings.ToLower(email)
	return email
}

func ExtractUsername(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func ExtractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}

func ExtractNameFromEmail(email string) string {
	username := ExtractUsername(email)
	if username == "" {
		return ""
	}

	username = strings.ReplaceAll(username, ".", " ")
	username = strings.ReplaceAll(username, "_", " ")
	username = strings.ReplaceAll(username, "-", " ")
	username = strings.ReplaceAll(username, "+", " ")

	plusIndex := strings.Index(username, "+")
	if plusIndex > 0 {
		username = username[:plusIndex]
	}

	parts := strings.Fields(username)
	if len(parts) == 0 {
		return ""
	}

	var names []string
	for _, part := range parts {
		if len(part) > 0 && !isNumeric(part) {
			names = append(names, capitalize(part))
		}
	}

	if len(names) == 0 {
		return capitalize(parts[0])
	}

	return strings.Join(names, " ")
}

func ExtractFirstName(email string) string {
	fullName := ExtractNameFromEmail(email)
	parts := strings.Fields(fullName)
	if len(parts) > 0 {
		return parts[0]
	}
	return fullName
}

func ExtractLastName(email string) string {
	fullName := ExtractNameFromEmail(email)
	parts := strings.Fields(fullName)
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}

func DetectProvider(domain string) string {
	domain = strings.ToLower(domain)

	if provider, exists := providerDomains[domain]; exists {
		return provider
	}

	if strings.Contains(domain, "google") {
		return "gmail"
	}
	if strings.Contains(domain, "yahoo") {
		return "yahoo"
	}
	if strings.Contains(domain, "outlook") || strings.Contains(domain, "hotmail") {
		return "outlook"
	}
	if strings.Contains(domain, "icloud") {
		return "icloud"
	}

	return "other"
}

func IsDisposableEmail(email string) bool {
	domain := ExtractDomain(email)
	return disposableDomains[domain]
}

func ValidateDomainDNS(domain string) error {
	domain = strings.ToLower(domain)

	ips, err := net.LookupIP(domain)
	if err != nil || len(ips) == 0 {
		return ErrDomainNotFound
	}

	return nil
}

func ValidateEmailDNS(email string) error {
	domain := ExtractDomain(email)
	if domain == "" {
		return ErrInvalidDomain
	}

	return ValidateDomainDNS(domain)
}

func CheckMXRecords(domain string) (bool, error) {
	domain = strings.ToLower(domain)

	mxRecords, err := net.LookupMX(domain)
	if err != nil {
		return false, err
	}

	if len(mxRecords) == 0 {
		return false, ErrNoMXRecords
	}

	return true, nil
}

func ValidateEmailWithMX(email string) error {
	if err := ValidateEmailStrict(email); err != nil {
		return err
	}

	domain := ExtractDomain(email)
	hasMX, err := CheckMXRecords(domain)
	if err != nil {
		return err
	}

	if !hasMX {
		return ErrNoMXRecords
	}

	return nil
}

func ValidateEmailWithTimeout(email string, timeout time.Duration) error {
	type result struct {
		err error
	}

	ch := make(chan result, 1)

	go func() {
		err := ValidateEmailWithMX(email)
		ch <- result{err: err}
	}()

	select {
	case res := <-ch:
		return res.err
	case <-time.After(timeout):
		return errors.New("validation timeout")
	}
}

func ObfuscateEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}

	local := parts[0]
	domain := parts[1]

	if len(local) <= 2 {
		return local[0:1] + "***@" + domain
	}

	visible := len(local) / 3
	if visible < 1 {
		visible = 1
	}

	return local[0:visible] + "***@" + domain
}

func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***@***"
	}

	local := parts[0]
	domain := parts[1]

	maskedLocal := ""
	if len(local) > 0 {
		maskedLocal = string(local[0]) + strings.Repeat("*", len(local)-1)
	}

	domainParts := strings.Split(domain, ".")
	maskedDomain := ""
	for i, part := range domainParts {
		if len(part) > 0 {
			maskedDomain += string(part[0]) + strings.Repeat("*", len(part)-1)
		}
		if i < len(domainParts)-1 {
			maskedDomain += "."
		}
	}

	return maskedLocal + "@" + maskedDomain
}

func GetEmailHash(email string) string {
	normalized := NormalizeEmail(email)
	return CalculateHash([]byte(normalized))
}

func CompareEmails(email1, email2 string) bool {
	return NormalizeEmail(email1) == NormalizeEmail(email2)
}

func IsGmail(email string) bool {
	return DetectProvider(ExtractDomain(email)) == "gmail"
}

func IsYahoo(email string) bool {
	return DetectProvider(ExtractDomain(email)) == "yahoo"
}

func IsOutlook(email string) bool {
	return DetectProvider(ExtractDomain(email)) == "outlook"
}

func IsICloud(email string) bool {
	return DetectProvider(ExtractDomain(email)) == "icloud"
}

func IsCorporateEmail(email string) bool {
	domain := ExtractDomain(email)
	
	commonProviders := []string{"gmail", "yahoo", "outlook", "hotmail", "icloud", "aol", "protonmail"}
	
	for _, provider := range commonProviders {
		if strings.Contains(domain, provider) {
			return false
		}
	}

	return true
}

func ExtractEmails(text string) []string {
	matches := emailRegex.FindAllString(text, -1)
	
	uniqueEmails := make(map[string]bool)
	var result []string

	for _, email := range matches {
		normalized := NormalizeEmail(email)
		if !uniqueEmails[normalized] {
			uniqueEmails[normalized] = true
			result = append(result, normalized)
		}
	}

	return result
}

func SplitEmailList(emails string) []string {
	separators := []string{",", ";", "\n", "\t", " "}
	
	for _, sep := range separators {
		emails = strings.ReplaceAll(emails, sep, ",")
	}

	parts := strings.Split(emails, ",")
	
	var result []string
	for _, email := range parts {
		email = strings.TrimSpace(email)
		if email != "" && ValidateEmail(email) {
			result = append(result, NormalizeEmail(email))
		}
	}

	return result
}

func DeduplicateEmails(emails []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, email := range emails {
		normalized := NormalizeEmail(email)
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}

	return result
}

func FilterValidEmails(emails []string) []string {
	var result []string

	for _, email := range emails {
		if ValidateEmail(email) {
			result = append(result, NormalizeEmail(email))
		}
	}

	return result
}

func FilterInvalidEmails(emails []string) []string {
	var result []string

	for _, email := range emails {
		if !ValidateEmail(email) {
			result = append(result, email)
		}
	}

	return result
}

func FilterDisposableEmails(emails []string) []string {
	var result []string

	for _, email := range emails {
		if !IsDisposableEmail(email) {
			result = append(result, email)
		}
	}

	return result
}

func GroupEmailsByDomain(emails []string) map[string][]string {
	grouped := make(map[string][]string)

	for _, email := range emails {
		domain := ExtractDomain(email)
		grouped[domain] = append(grouped[domain], email)
	}

	return grouped
}

func GroupEmailsByProvider(emails []string) map[string][]string {
	grouped := make(map[string][]string)

	for _, email := range emails {
		provider := DetectProvider(ExtractDomain(email))
		grouped[provider] = append(grouped[provider], email)
	}

	return grouped
}

func GenerateEmailVariations(baseEmail string) []string {
	parsed, err := ParseEmail(baseEmail)
	if err != nil {
		return []string{baseEmail}
	}

	local := parsed.Local
	domain := parsed.Domain

	variations := []string{baseEmail}

	variations = append(variations, local+"+test@"+domain)
	variations = append(variations, local+"+spam@"+domain)
	variations = append(variations, local+"+newsletter@"+domain)

	if strings.Contains(local, ".") {
		noDots := strings.ReplaceAll(local, ".", "")
		variations = append(variations, noDots+"@"+domain)
	}

	return DeduplicateEmails(variations)
}

func CreateEmailFromName(firstName, lastName, domain string) string {
	if domain == "" {
		domain = "example.com"
	}

	firstName = strings.ToLower(strings.TrimSpace(firstName))
	lastName = strings.ToLower(strings.TrimSpace(lastName))

	firstName = strings.ReplaceAll(firstName, " ", "")
	lastName = strings.ReplaceAll(lastName, " ", "")

	if lastName == "" {
		return fmt.Sprintf("%s@%s", firstName, domain)
	}

	return fmt.Sprintf("%s.%s@%s", firstName, lastName, domain)
}

func SuggestEmailCorrections(email string) []string {
	if ValidateEmail(email) {
		return []string{email}
	}

	suggestions := []string{}

	email = strings.TrimSpace(email)
	email = strings.ToLower(email)

	commonTypos := map[string]string{
		"gmial.com":    "gmail.com",
		"gmai.com":     "gmail.com",
		"gmil.com":     "gmail.com",
		"yahooo.com":   "yahoo.com",
		"yaho.com":     "yahoo.com",
		"hotmial.com":  "hotmail.com",
		"hotmil.com":   "hotmail.com",
		"outlok.com":   "outlook.com",
		"outloo.com":   "outlook.com",
	}

	domain := ExtractDomain(email)
	if correction, exists := commonTypos[domain]; exists {
		corrected := strings.Replace(email, domain, correction, 1)
		suggestions = append(suggestions, corrected)
	}

	return suggestions
}

func IsRoleBasedEmail(email string) bool {
	roleKeywords := []string{
		"admin", "administrator", "info", "support", "sales", "contact",
		"help", "service", "noreply", "no-reply", "postmaster", "webmaster",
		"hostmaster", "abuse", "marketing", "hr", "jobs", "careers",
	}

	local := strings.ToLower(ExtractUsername(email))

	for _, keyword := range roleKeywords {
		if local == keyword || strings.HasPrefix(local, keyword) {
			return true
		}
	}

	return false
}

func GetTLD(email string) string {
	domain := ExtractDomain(email)
	parts := strings.Split(domain, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func HasPlusAddressing(email string) bool {
	local := ExtractUsername(email)
	return strings.Contains(local, "+")
}

func RemovePlusAddressing(email string) string {
	parsed, err := ParseEmail(email)
	if err != nil {
		return email
	}

	local := parsed.Local
	plusIndex := strings.Index(local, "+")
	
	if plusIndex > 0 {
		local = local[:plusIndex]
	}

	return local + "@" + parsed.Domain
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func ValidateBulkEmails(emails []string, maxConcurrent int) map[string]error {
	results := make(map[string]error)
	
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}

	semaphore := make(chan struct{}, maxConcurrent)
	resultChan := make(chan struct {
		email string
		err   error
	}, len(emails))

	for _, email := range emails {
		go func(e string) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			err := ValidateEmailStrict(e)
			resultChan <- struct {
				email string
				err   error
			}{e, err}
		}(email)
	}

	for range emails {
		result := <-resultChan
		results[result.email] = result.err
	}

	return results
}

func FormatEmailDisplay(email, name string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", name, email)
}

func ParseEmailDisplay(display string) (email, name string) {
	if strings.Contains(display, "<") && strings.Contains(display, ">") {
		start := strings.Index(display, "<")
		end := strings.Index(display, ">")
		
		if start < end {
			email = strings.TrimSpace(display[start+1 : end])
			name = strings.TrimSpace(display[:start])
			return
		}
	}

	email = strings.TrimSpace(display)
	return
}
