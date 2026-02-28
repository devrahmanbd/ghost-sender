package deliverability

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"email-campaign-system/internal/models"
	"email-campaign-system/pkg/logger"
)

type UnsubscribeManager struct {
	unsubscribed map[string]*UnsubscribeRecord
	mu           sync.RWMutex
	config       *UnsubscribeConfig
	logger       logger.Logger
	secretKey    []byte
}

type UnsubscribeConfig struct {
	BaseURL              string
	SecretKey            string
	TokenExpiry          time.Duration
	EnableOneClick       bool
	EnableMailto         bool
	MailtoAddress        string
	TrackingEnabled      bool
	AutoRemoveFromList   bool
	ConfirmationRequired bool
	CustomPageURL        string
}

type UnsubscribeRecord struct {
	Email           string
	CampaignID      string
	RecipientID     string
	Token           string
	UnsubscribedAt  time.Time
	IPAddress       string
	UserAgent       string
	Method          UnsubscribeMethod
	Confirmed       bool
	Reason          string
	PreferenceType  PreferenceType
}

type UnsubscribeMethod string

const (
	MethodOneClick UnsubscribeMethod = "one_click"
	MethodLink     UnsubscribeMethod = "link"
	MethodMailto   UnsubscribeMethod = "mailto"
	MethodManual   UnsubscribeMethod = "manual"
)

type PreferenceType string

const (
	PreferenceUnsubscribeAll      PreferenceType = "all"
	PreferenceUnsubscribeCampaign PreferenceType = "campaign"
	PreferenceUnsubscribeCategory PreferenceType = "category"
)

type UnsubscribeToken struct {
	Email      string
	CampaignID string
	Timestamp  int64
	Signature  string
}

type UnsubscribeRequest struct {
	Email       string
	CampaignID  string
	Token       string
	IPAddress   string
	UserAgent   string
	Method      UnsubscribeMethod
	Reason      string
	Preference  PreferenceType
}

type UnsubscribeResponse struct {
	Success   bool
	Message   string
	Email     string
	Timestamp time.Time
}

func NewUnsubscribeManager(logger logger.Logger) *UnsubscribeManager {
	return &UnsubscribeManager{
		unsubscribed: make(map[string]*UnsubscribeRecord),
		config:       DefaultUnsubscribeConfig(),
		logger:       logger,
		secretKey:    generateSecretKey(),
	}
}

func DefaultUnsubscribeConfig() *UnsubscribeConfig {
	return &UnsubscribeConfig{
		BaseURL:              "https://example.com/unsubscribe",
		TokenExpiry:          90 * 24 * time.Hour,
		EnableOneClick:       true,
		EnableMailto:         false,
		TrackingEnabled:      true,
		AutoRemoveFromList:   true,
		ConfirmationRequired: false,
	}
}

func (um *UnsubscribeManager) GenerateUnsubscribeURL(email, campaignID string) (string, error) {
	token, err := um.GenerateToken(email, campaignID)
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimSuffix(um.config.BaseURL, "/")
	params := url.Values{}
	params.Add("token", token)
	params.Add("email", email)

	if campaignID != "" {
		params.Add("campaign", campaignID)
	}

	return fmt.Sprintf("%s?%s", baseURL, params.Encode()), nil
}

func (um *UnsubscribeManager) GenerateToken(email, campaignID string) (string, error) {
	timestamp := time.Now().Unix()
	
	data := fmt.Sprintf("%s:%s:%d", email, campaignID, timestamp)
	signature := um.generateHMAC(data)
	
	tokenData := fmt.Sprintf("%s:%s:%d:%s", email, campaignID, timestamp, signature)
	token := base64.URLEncoding.EncodeToString([]byte(tokenData))
	
	return token, nil
}

func (um *UnsubscribeManager) ValidateToken(token string) (*UnsubscribeToken, error) {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid token format: %w", err)
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid token structure")
	}

	email := parts[0]
	campaignID := parts[1]
	var timestamp int64
	fmt.Sscanf(parts[2], "%d", &timestamp)
	signature := parts[3]

	data := fmt.Sprintf("%s:%s:%d", email, campaignID, timestamp)
	expectedSignature := um.generateHMAC(data)

	if signature != expectedSignature {
		return nil, fmt.Errorf("invalid token signature")
	}

	tokenTime := time.Unix(timestamp, 0)
	if time.Since(tokenTime) > um.config.TokenExpiry {
		return nil, fmt.Errorf("token expired")
	}

	return &UnsubscribeToken{
		Email:      email,
		CampaignID: campaignID,
		Timestamp:  timestamp,
		Signature:  signature,
	}, nil
}

func (um *UnsubscribeManager) generateHMAC(data string) string {
	h := hmac.New(sha256.New, um.secretKey)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func (um *UnsubscribeManager) ProcessUnsubscribe(req *UnsubscribeRequest) (*UnsubscribeResponse, error) {
	if req.Token != "" {
		tokenData, err := um.ValidateToken(req.Token)
		if err != nil {
			return &UnsubscribeResponse{
				Success: false,
				Message: "Invalid or expired unsubscribe link",
			}, err
		}
		req.Email = tokenData.Email
		req.CampaignID = tokenData.CampaignID
	}

	if req.Email == "" {
		return &UnsubscribeResponse{
			Success: false,
			Message: "Email address is required",
		}, fmt.Errorf("email is required")
	}

	um.mu.Lock()
	defer um.mu.Unlock()

	key := um.getUnsubscribeKey(req.Email, req.CampaignID)
	
	if _, exists := um.unsubscribed[key]; exists {
		return &UnsubscribeResponse{
			Success: true,
			Message: "Already unsubscribed",
			Email:   req.Email,
		}, nil
	}

	record := &UnsubscribeRecord{
		Email:          req.Email,
		CampaignID:     req.CampaignID,
		Token:          req.Token,
		UnsubscribedAt: time.Now(),
		IPAddress:      req.IPAddress,
		UserAgent:      req.UserAgent,
		Method:         req.Method,
		Confirmed:      !um.config.ConfirmationRequired,
		Reason:         req.Reason,
		PreferenceType: req.Preference,
	}

	um.unsubscribed[key] = record

	return &UnsubscribeResponse{
		Success:   true,
		Message:   "Successfully unsubscribed",
		Email:     req.Email,
		Timestamp: record.UnsubscribedAt,
	}, nil
}

func (um *UnsubscribeManager) IsUnsubscribed(email, campaignID string) bool {
	um.mu.RLock()
	defer um.mu.RUnlock()

	globalKey := um.getUnsubscribeKey(email, "")
	if _, exists := um.unsubscribed[globalKey]; exists {
		return true
	}

	if campaignID != "" {
		campaignKey := um.getUnsubscribeKey(email, campaignID)
		if _, exists := um.unsubscribed[campaignKey]; exists {
			return true
		}
	}

	return false
}

func (um *UnsubscribeManager) getUnsubscribeKey(email, campaignID string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if campaignID == "" {
		return fmt.Sprintf("global:%s", email)
	}
	return fmt.Sprintf("campaign:%s:%s", campaignID, email)
}

func (um *UnsubscribeManager) GenerateListUnsubscribeHeader(email, campaignID string) (string, error) {
	unsubscribeURL, err := um.GenerateUnsubscribeURL(email, campaignID)
	if err != nil {
		return "", err
	}

	parts := make([]string, 0, 2)
	parts = append(parts, fmt.Sprintf("<%s>", unsubscribeURL))

	if um.config.EnableMailto && um.config.MailtoAddress != "" {
		mailtoURL := fmt.Sprintf("mailto:%s?subject=unsubscribe", um.config.MailtoAddress)
		parts = append(parts, fmt.Sprintf("<%s>", mailtoURL))
	}

	return strings.Join(parts, ", "), nil
}

func (um *UnsubscribeManager) GenerateOneClickUnsubscribeURL(email, campaignID string) (string, error) {
	if !um.config.EnableOneClick {
		return "", fmt.Errorf("one-click unsubscribe not enabled")
	}

	token, err := um.GenerateToken(email, campaignID)
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimSuffix(um.config.BaseURL, "/")
	return fmt.Sprintf("%s/one-click?token=%s", baseURL, url.QueryEscape(token)), nil
}

func (um *UnsubscribeManager) ConfirmUnsubscribe(email, campaignID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	key := um.getUnsubscribeKey(email, campaignID)
	record, exists := um.unsubscribed[key]
	if !exists {
		return fmt.Errorf("unsubscribe record not found")
	}

	record.Confirmed = true
	return nil
}

func (um *UnsubscribeManager) GetUnsubscribeRecord(email, campaignID string) *UnsubscribeRecord {
	um.mu.RLock()
	defer um.mu.RUnlock()

	key := um.getUnsubscribeKey(email, campaignID)
	return um.unsubscribed[key]
}

func (um *UnsubscribeManager) GetAllUnsubscribed() []*UnsubscribeRecord {
	um.mu.RLock()
	defer um.mu.RUnlock()

	records := make([]*UnsubscribeRecord, 0, len(um.unsubscribed))
	for _, record := range um.unsubscribed {
		records = append(records, record)
	}
	return records
}

func (um *UnsubscribeManager) GetUnsubscribedByCampaign(campaignID string) []*UnsubscribeRecord {
	um.mu.RLock()
	defer um.mu.RUnlock()

	records := make([]*UnsubscribeRecord, 0)
	for _, record := range um.unsubscribed {
		if record.CampaignID == campaignID || record.PreferenceType == PreferenceUnsubscribeAll {
			records = append(records, record)
		}
	}
	return records
}

func (um *UnsubscribeManager) RemoveUnsubscribe(email, campaignID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	key := um.getUnsubscribeKey(email, campaignID)
	if _, exists := um.unsubscribed[key]; !exists {
		return fmt.Errorf("unsubscribe record not found")
	}

	delete(um.unsubscribed, key)
	return nil
}

func (um *UnsubscribeManager) ExportUnsubscribeList() []string {
	um.mu.RLock()
	defer um.mu.RUnlock()

	emails := make([]string, 0, len(um.unsubscribed))
	uniqueEmails := make(map[string]bool)

	for _, record := range um.unsubscribed {
		if !uniqueEmails[record.Email] {
			emails = append(emails, record.Email)
			uniqueEmails[record.Email] = true
		}
	}

	return emails
}

func (um *UnsubscribeManager) GetStatistics() map[string]interface{} {
	um.mu.RLock()
	defer um.mu.RUnlock()

	stats := map[string]interface{}{
		"total_unsubscribed": len(um.unsubscribed),
		"by_method":          make(map[string]int),
		"by_preference":      make(map[string]int),
		"confirmed":          0,
		"unconfirmed":        0,
	}

	methodCounts := make(map[UnsubscribeMethod]int)
	preferenceCounts := make(map[PreferenceType]int)
	confirmed := 0
	unconfirmed := 0

	for _, record := range um.unsubscribed {
		methodCounts[record.Method]++
		preferenceCounts[record.PreferenceType]++
		if record.Confirmed {
			confirmed++
		} else {
			unconfirmed++
		}
	}

	stats["by_method"] = map[string]int{
		string(MethodOneClick): methodCounts[MethodOneClick],
		string(MethodLink):     methodCounts[MethodLink],
		string(MethodMailto):   methodCounts[MethodMailto],
		string(MethodManual):   methodCounts[MethodManual],
	}

	stats["by_preference"] = map[string]int{
		string(PreferenceUnsubscribeAll):      preferenceCounts[PreferenceUnsubscribeAll],
		string(PreferenceUnsubscribeCampaign): preferenceCounts[PreferenceUnsubscribeCampaign],
		string(PreferenceUnsubscribeCategory): preferenceCounts[PreferenceUnsubscribeCategory],
	}

	stats["confirmed"] = confirmed
	stats["unconfirmed"] = unconfirmed

	return stats
}

func (um *UnsubscribeManager) SetConfig(config *UnsubscribeConfig) {
	um.config = config
}

func (um *UnsubscribeManager) GetConfig() *UnsubscribeConfig {
	return um.config
}

func (um *UnsubscribeManager) Clear() {
	um.mu.Lock()
	defer um.mu.Unlock()
	um.unsubscribed = make(map[string]*UnsubscribeRecord)
}

func (um *UnsubscribeManager) ImportUnsubscribeList(emails []string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}

		key := um.getUnsubscribeKey(email, "")
		if _, exists := um.unsubscribed[key]; !exists {
			um.unsubscribed[key] = &UnsubscribeRecord{
				Email:          email,
				CampaignID:     "",
				UnsubscribedAt: time.Now(),
				Method:         MethodManual,
				Confirmed:      true,
				PreferenceType: PreferenceUnsubscribeAll,
			}
		}
	}

	return nil
}

func (um *UnsubscribeManager) FilterRecipients(recipients []*models.Recipient, campaignID string) []*models.Recipient {
	um.mu.RLock()
	defer um.mu.RUnlock()

	filtered := make([]*models.Recipient, 0, len(recipients))
	for _, recipient := range recipients {
		if !um.IsUnsubscribed(recipient.Email, campaignID) {
			filtered = append(filtered, recipient)
		}
	}

	return filtered
}

func generateSecretKey() []byte {
	key := make([]byte, 32)
	rand.Read(key)
	return key
}

func (um *UnsubscribeManager) SetSecretKey(key string) error {
	if len(key) < 16 {
		return fmt.Errorf("secret key must be at least 16 characters")
	}
	um.secretKey = []byte(key)
	return nil
}

func BuildUnsubscribeHTML(email, unsubscribeURL string) string {
	return fmt.Sprintf(`
<div style="text-align: center; padding: 20px; font-family: Arial, sans-serif; color: #666;">
	<p style="font-size: 12px;">
		If you no longer wish to receive these emails, you can 
		<a href="%s" style="color: #0066cc; text-decoration: underline;">unsubscribe here</a>.
	</p>
</div>
`, unsubscribeURL)
}

func BuildUnsubscribeText(unsubscribeURL string) string {
	return fmt.Sprintf("\n\nTo unsubscribe from this mailing list, visit: %s\n", unsubscribeURL)
}

func ValidateUnsubscribeEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email is required")
	}

	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email format")
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

func GenerateUnsubscribeConfirmationPage(email string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<title>Unsubscribe Confirmation</title>
	<style>
		body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
		.container { max-width: 600px; margin: 0 auto; }
		h1 { color: #333; }
		p { color: #666; line-height: 1.6; }
		.success { color: #28a745; font-weight: bold; }
	</style>
</head>
<body>
	<div class="container">
		<h1>Unsubscribe Successful</h1>
		<p class="success">✓ You have been successfully unsubscribed</p>
		<p>The email address <strong>%s</strong> has been removed from our mailing list.</p>
		<p>You will no longer receive emails from us.</p>
		<p>If you unsubscribed by mistake, please contact us.</p>
	</div>
</body>
</html>
`, email)
}

