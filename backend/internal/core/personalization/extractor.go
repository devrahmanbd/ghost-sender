package personalization

import (
	"regexp"
	"strings"
	"unicode"

	"email-campaign-system/internal/models"
	"email-campaign-system/pkg/logger"
)

type NameExtractor struct {
	log    logger.Logger
	config *ExtractorConfig
}

type ExtractorConfig struct {
	EnableCleaning       bool
	EnableCapitalization bool
	MinNameLength        int
	MaxNameLength        int
	RemoveNumbers        bool
	CommonPrefixes       []string
	CommonSuffixes       []string
	StopWords            []string
}

type ExtractedName struct {
	FullName   string
	FirstName  string
	LastName   string
	Username   string
	Domain     string
	Confidence float64
}

func NewNameExtractor(log logger.Logger) *NameExtractor {
	return &NameExtractor{
		log:    log,
		config: DefaultExtractorConfig(),
	}
}

func DefaultExtractorConfig() *ExtractorConfig {
	return &ExtractorConfig{
		EnableCleaning:       true,
		EnableCapitalization: true,
		MinNameLength:        2,
		MaxNameLength:        50,
		RemoveNumbers:        true,
		CommonPrefixes:       []string{"mr", "mrs", "ms", "dr", "prof"},
		CommonSuffixes:       []string{"jr", "sr", "ii", "iii", "iv"},
		StopWords:            []string{"the", "and", "or", "but", "admin", "info", "support", "sales", "contact"},
	}
}

func (ne *NameExtractor) ExtractNameFromEmail(email string) string {
	if email == "" {
		return ""
	}

	extracted := ne.ExtractFullName(email)
	if extracted.FullName != "" {
		return extracted.FullName
	}

	return ne.extractUsernameFromEmail(email)
}

func (ne *NameExtractor) ExtractFullName(email string) *ExtractedName {
	if email == "" {
		return &ExtractedName{Confidence: 0.0}
	}

	username, domain := ne.splitEmail(email)
	if username == "" {
		return &ExtractedName{Confidence: 0.0}
	}

	parts := ne.parseUsername(username)
	if len(parts) == 0 {
		return &ExtractedName{
			Username:   username,
			Domain:     domain,
			Confidence: 0.0,
		}
	}

	firstName, lastName := ne.extractFirstAndLastName(parts)
	fullName := ne.buildFullName(firstName, lastName)

	confidence := ne.calculateConfidence(parts, firstName, lastName)

	return &ExtractedName{
		FullName:   fullName,
		FirstName:  firstName,
		LastName:   lastName,
		Username:   username,
		Domain:     domain,
		Confidence: confidence,
	}
}

func (ne *NameExtractor) ExtractFirstName(recipient *models.Recipient) string {
	if recipient == nil {
		return ""
	}

	// Check if recipient has FirstName field
	if recipient.FirstName != "" {
		return ne.cleanName(recipient.FirstName)
	}

	// Fallback to extracting from email
	extracted := ne.ExtractFullName(recipient.Email)
	return extracted.FirstName
}

func (ne *NameExtractor) ExtractLastName(recipient *models.Recipient) string {
	if recipient == nil {
		return ""
	}

	// Check if recipient has LastName field
	if recipient.LastName != "" {
		return ne.cleanName(recipient.LastName)
	}

	// Fallback to extracting from email
	extracted := ne.ExtractFullName(recipient.Email)
	return extracted.LastName
}

func (ne *NameExtractor) extractFirstNameFromFullName(fullName string) string {
	parts := strings.Fields(fullName)
	if len(parts) == 0 {
		return ""
	}

	firstName := parts[0]
	return ne.cleanName(firstName)
}

func (ne *NameExtractor) extractLastNameFromFullName(fullName string) string {
	parts := strings.Fields(fullName)
	if len(parts) < 2 {
		return ""
	}

	lastName := parts[len(parts)-1]
	return ne.cleanName(lastName)
}

func (ne *NameExtractor) splitEmail(email string) (string, string) {
	email = strings.TrimSpace(strings.ToLower(email))

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", ""
	}

	return parts[0], parts[1]
}

func (ne *NameExtractor) extractUsernameFromEmail(email string) string {
	username, _ := ne.splitEmail(email)
	return username
}

func (ne *NameExtractor) parseUsername(username string) []string {
	username = strings.ToLower(username)

	username = ne.removeNumbers(username)

	separators := []string{".", "_", "-", "+"}
	parts := []string{username}

	for _, sep := range separators {
		newParts := []string{}
		for _, part := range parts {
			split := strings.Split(part, sep)
			newParts = append(newParts, split...)
		}
		parts = newParts
	}

	cleaned := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if ne.isStopWord(part) {
			continue
		}

		if len(part) < ne.config.MinNameLength {
			continue
		}

		if len(part) > ne.config.MaxNameLength {
			continue
		}

		cleaned = append(cleaned, part)
	}

	return cleaned
}

func (ne *NameExtractor) extractFirstAndLastName(parts []string) (string, string) {
	if len(parts) == 0 {
		return "", ""
	}

	if len(parts) == 1 {
		firstName := ne.cleanName(parts[0])
		return firstName, ""
	}

	firstName := ne.cleanName(parts[0])
	lastName := ne.cleanName(parts[len(parts)-1])

	return firstName, lastName
}

func (ne *NameExtractor) buildFullName(firstName, lastName string) string {
	if firstName == "" && lastName == "" {
		return ""
	}

	if lastName == "" {
		return firstName
	}

	return firstName + " " + lastName
}

func (ne *NameExtractor) cleanName(name string) string {
	if !ne.config.EnableCleaning {
		return name
	}

	name = strings.TrimSpace(name)

	if ne.config.RemoveNumbers {
		name = ne.removeNumbers(name)
	}

	name = ne.removeSpecialCharacters(name)

	if ne.config.EnableCapitalization {
		name = ne.capitalizeName(name)
	}

	return name
}

func (ne *NameExtractor) removeNumbers(s string) string {
	if !ne.config.RemoveNumbers {
		return s
	}

	result := strings.Builder{}
	for _, char := range s {
		if !unicode.IsDigit(char) {
			result.WriteRune(char)
		}
	}

	return result.String()
}

func (ne *NameExtractor) removeSpecialCharacters(s string) string {
	result := strings.Builder{}

	for _, char := range s {
		if unicode.IsLetter(char) || unicode.IsSpace(char) {
			result.WriteRune(char)
		}
	}

	return result.String()
}

func (ne *NameExtractor) capitalizeName(name string) string {
	if name == "" {
		return ""
	}

	words := strings.Fields(name)
	capitalized := make([]string, len(words))

	for i, word := range words {
		if len(word) == 0 {
			continue
		}

		runes := []rune(word)
		runes[0] = unicode.ToUpper(runes[0])

		for j := 1; j < len(runes); j++ {
			runes[j] = unicode.ToLower(runes[j])
		}

		capitalized[i] = string(runes)
	}

	return strings.Join(capitalized, " ")
}

func (ne *NameExtractor) isStopWord(word string) bool {
	word = strings.ToLower(word)

	for _, stopWord := range ne.config.StopWords {
		if word == stopWord {
			return true
		}
	}

	return false
}

func (ne *NameExtractor) calculateConfidence(parts []string, firstName, lastName string) float64 {
	confidence := 0.5

	if len(parts) >= 2 {
		confidence += 0.2
	}

	if firstName != "" && len(firstName) >= ne.config.MinNameLength {
		confidence += 0.15
	}

	if lastName != "" && len(lastName) >= ne.config.MinNameLength {
		confidence += 0.15
	}

	if ne.looksLikeRealName(firstName) {
		confidence += 0.1
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

func (ne *NameExtractor) looksLikeRealName(name string) bool {
	if name == "" {
		return false
	}

	if len(name) < 2 {
		return false
	}

	digitCount := 0
	for _, char := range name {
		if unicode.IsDigit(char) {
			digitCount++
		}
	}

	if digitCount > 0 {
		return false
	}

	if strings.Contains(strings.ToLower(name), "noreply") {
		return false
	}

	if strings.Contains(strings.ToLower(name), "donotreply") {
		return false
	}

	return true
}

func (ne *NameExtractor) ExtractInitials(recipient *models.Recipient) string {
	if recipient == nil {
		return ""
	}

	firstName := ne.ExtractFirstName(recipient)
	lastName := ne.ExtractLastName(recipient)

	initials := ""

	if firstName != "" {
		initials += string(unicode.ToUpper(rune(firstName[0])))
	}

	if lastName != "" {
		initials += string(unicode.ToUpper(rune(lastName[0])))
	}

	return initials
}

func (ne *NameExtractor) ExtractDisplayName(recipient *models.Recipient) string {
	if recipient == nil {
		return ""
	}

	// Try to build name from FirstName and LastName
	fullName := ne.buildFullName(recipient.FirstName, recipient.LastName)
	if fullName != "" {
		return fullName
	}

	// Fallback to extracting from email
	extracted := ne.ExtractFullName(recipient.Email)
	if extracted.FullName != "" {
		return extracted.FullName
	}

	return extracted.Username
}

func (ne *NameExtractor) ParseFullName(fullName string) (string, string) {
	parts := strings.Fields(fullName)

	if len(parts) == 0 {
		return "", ""
	}

	if len(parts) == 1 {
		return ne.cleanName(parts[0]), ""
	}

	firstName := ne.cleanName(parts[0])
	lastName := ne.cleanName(parts[len(parts)-1])

	return firstName, lastName
}

func (ne *NameExtractor) ValidateName(name string) bool {
	if name == "" {
		return false
	}

	name = strings.TrimSpace(name)

	if len(name) < ne.config.MinNameLength {
		return false
	}

	if len(name) > ne.config.MaxNameLength {
		return false
	}

	digitPattern := regexp.MustCompile(`\d`)
	if digitPattern.MatchString(name) {
		return false
	}

	return true
}

func (ne *NameExtractor) SetConfig(config *ExtractorConfig) {
	ne.config = config
}

func (ne *NameExtractor) GetConfig() *ExtractorConfig {
	configCopy := *ne.config
	return &configCopy
}

func (ne *NameExtractor) ExtractMiddleName(fullName string) string {
	parts := strings.Fields(fullName)

	if len(parts) < 3 {
		return ""
	}

	middleParts := parts[1 : len(parts)-1]
	middleName := strings.Join(middleParts, " ")

	return ne.cleanName(middleName)
}

func (ne *NameExtractor) NormalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = ne.removeSpecialCharacters(name)
	name = ne.capitalizeName(name)
	return name
}

func (ne *NameExtractor) SplitCompoundName(name string) []string {
	separators := []string{"-", " "}
	parts := []string{name}

	for _, sep := range separators {
		newParts := []string{}
		for _, part := range parts {
			split := strings.Split(part, sep)
			newParts = append(newParts, split...)
		}
		parts = newParts
	}

	cleaned := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}

	return cleaned
}
