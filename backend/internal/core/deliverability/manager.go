package deliverability

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"mime"
	"net/textproto"
	"os"
	"strings"
	"time"
	"email-campaign-system/pkg/logger"
)

var (
	ErrInvalidRequest     = errors.New("invalid message request")
	ErrMissingRecipient   = errors.New("missing recipient")
	ErrMissingSender      = errors.New("missing sender")
	ErrInvalidAttachment  = errors.New("invalid attachment")
	ErrDKIMSigningFailed  = errors.New("DKIM signing failed")
	ErrMessageTooLarge    = errors.New("message size exceeds limit")
)

type Manager struct {
	config          *Config
	log             logger.Logger
	dkimPrivateKey  *rsa.PrivateKey
	spamScoreEngine *SpamScoreEngine
}

type Config struct {
	EnableDKIM           bool
	DKIMDomain           string
	DKIMSelector         string
	DKIMPrivateKeyPath   string
	EnableTracking       bool
	TrackingDomain       string
	EnableUnsubscribe    bool
	UnsubscribeURL       string
	MaxMessageSize       int64
	DefaultCharset       string
	EnableSpamCheck      bool
	SpamScoreThreshold   float64
	EnableAuthentication bool
}

type MessageRequest struct {
	CampaignID    string
	TemplateID    string
	RecipientID   string
	From          EmailAddress
	To            EmailAddress
	ReplyTo       *EmailAddress
	CC            []EmailAddress
	BCC           []EmailAddress
	Subject       string
	HTMLBody      string
	TextBody      string
	Headers       map[string]string
	Attachments   []Attachment
	Personalize   map[string]string
	TrackOpens    bool
	TrackClicks   bool
	Unsubscribe   bool
	Priority      Priority
	SendAt        *time.Time
}

type EmailAddress struct {
	Name    string
	Address string
}

type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
	Inline      bool
	ContentID   string
}

type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
	PriorityUrgent
)

type EmailMessage struct {
	MessageID   string
	From        EmailAddress
	To          EmailAddress
	ReplyTo     *EmailAddress
	CC          []EmailAddress
	BCC         []EmailAddress
	Subject     string
	Headers     textproto.MIMEHeader
	HTMLBody    string
	TextBody    string
	Attachments []Attachment
	Raw         []byte
	Size        int64
	SpamScore   float64
	Metadata    MessageMetadata
}

type MessageMetadata struct {
	CampaignID       string
	TemplateID       string
	RecipientID      string
	TrackingID       string
	UnsubscribeToken string
	CreatedAt        time.Time
}

type SpamScoreEngine struct {
	threshold float64
}

func NewManager(config *Config, log logger.Logger) (*Manager, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if log == nil {
		return nil, errors.New("logger is required")
	}

	manager := &Manager{
		config: config,
		log:    log,
		spamScoreEngine: &SpamScoreEngine{
			threshold: config.SpamScoreThreshold,
		},
	}

	if config.EnableDKIM && config.DKIMPrivateKeyPath != "" {
		privateKey, err := loadDKIMPrivateKey(config.DKIMPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load DKIM private key: %w", err)
		}
		manager.dkimPrivateKey = privateKey
	}

	return manager, nil
}

func DefaultConfig() *Config {
	return &Config{
		EnableDKIM:           false,
		EnableTracking:       true,
		EnableUnsubscribe:    true,
		MaxMessageSize:       25 * 1024 * 1024,
		DefaultCharset:       "UTF-8",
		EnableSpamCheck:      true,
		SpamScoreThreshold:   5.0,
		EnableAuthentication: true,
	}
}

func (m *Manager) BuildMessage(ctx context.Context, req *MessageRequest) (*EmailMessage, error) {
	if req == nil {
		return nil, ErrInvalidRequest
	}

	if err := m.validateRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	msg := &EmailMessage{
		MessageID: m.generateMessageID(req),
		From:      req.From,
		To:        req.To,
		ReplyTo:   req.ReplyTo,
		CC:        req.CC,
		BCC:       req.BCC,
		Subject:   req.Subject,
		Headers:   make(textproto.MIMEHeader),
		Metadata: MessageMetadata{
			CampaignID:  req.CampaignID,
			TemplateID:  req.TemplateID,
			RecipientID: req.RecipientID,
			CreatedAt:   time.Now(),
		},
	}

	if req.TrackOpens && m.config.EnableTracking {
		trackingID := m.generateTrackingID(req)
		msg.Metadata.TrackingID = trackingID
		req.HTMLBody = m.insertTrackingPixel(req.HTMLBody, trackingID)
	}

	if req.TrackClicks && m.config.EnableTracking {
		req.HTMLBody = m.wrapLinksForTracking(req.HTMLBody, msg.Metadata.TrackingID)
	}

	if req.Unsubscribe && m.config.EnableUnsubscribe {
		token := m.generateUnsubscribeToken(req)
		msg.Metadata.UnsubscribeToken = token
		unsubLink := m.buildUnsubscribeLink(token)
		req.HTMLBody = m.insertUnsubscribeLink(req.HTMLBody, unsubLink)
	}

	msg.HTMLBody = req.HTMLBody
	msg.TextBody = req.TextBody
	msg.Attachments = req.Attachments

	m.buildHeaders(msg, req)

	raw, err := m.buildRawMessage(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to build raw message: %w", err)
	}

	msg.Raw = raw
	msg.Size = int64(len(raw))

	if msg.Size > m.config.MaxMessageSize {
		return nil, fmt.Errorf("%w: %d bytes", ErrMessageTooLarge, msg.Size)
	}

	if m.config.EnableSpamCheck {
		spamScore := m.spamScoreEngine.CalculateScore(msg)
		msg.SpamScore = spamScore
		
		if spamScore > m.config.SpamScoreThreshold {
			m.log.Warn(fmt.Sprintf("high spam score detected: score=%.2f, threshold=%.2f, message_id=%s",
				spamScore, m.config.SpamScoreThreshold, msg.MessageID))
		}
	}

	if m.config.EnableDKIM && m.dkimPrivateKey != nil {
		if err := m.signMessageDKIM(msg); err != nil {
			m.log.Error(fmt.Sprintf("DKIM signing failed: error=%v, message_id=%s", err, msg.MessageID))
		}
	}

	m.log.Debug(fmt.Sprintf("message built: message_id=%s, size=%d, spam_score=%.2f",
		msg.MessageID, msg.Size, msg.SpamScore))

	return msg, nil
}

func (m *Manager) validateRequest(req *MessageRequest) error {
	if req.To.Address == "" {
		return ErrMissingRecipient
	}

	if req.From.Address == "" {
		return ErrMissingSender
	}

	if req.Subject == "" {
		return errors.New("subject is required")
	}

	if req.HTMLBody == "" && req.TextBody == "" {
		return errors.New("message body is required")
	}

	if !isValidEmail(req.From.Address) {
		return fmt.Errorf("invalid sender email: %s", req.From.Address)
	}

	if !isValidEmail(req.To.Address) {
		return fmt.Errorf("invalid recipient email: %s", req.To.Address)
	}

	for _, attachment := range req.Attachments {
		if attachment.Filename == "" {
			return fmt.Errorf("%w: missing filename", ErrInvalidAttachment)
		}
		if len(attachment.Content) == 0 {
			return fmt.Errorf("%w: empty content for %s", ErrInvalidAttachment, attachment.Filename)
		}
	}

	return nil
}

func (m *Manager) buildHeaders(msg *EmailMessage, req *MessageRequest) {
	msg.Headers.Set("Message-ID", fmt.Sprintf("<%s>", msg.MessageID))
	msg.Headers.Set("Date", time.Now().Format(time.RFC1123Z))
	msg.Headers.Set("From", m.formatEmailAddress(msg.From))
	msg.Headers.Set("To", m.formatEmailAddress(msg.To))
	msg.Headers.Set("Subject", mime.QEncoding.Encode(m.config.DefaultCharset, msg.Subject))
	msg.Headers.Set("MIME-Version", "1.0")

	if msg.ReplyTo != nil {
		msg.Headers.Set("Reply-To", m.formatEmailAddress(*msg.ReplyTo))
	}

	if len(msg.CC) > 0 {
		ccAddrs := make([]string, len(msg.CC))
		for i, addr := range msg.CC {
			ccAddrs[i] = m.formatEmailAddress(addr)
		}
		msg.Headers.Set("CC", strings.Join(ccAddrs, ", "))
	}

	switch req.Priority {
	case PriorityHigh:
		msg.Headers.Set("X-Priority", "1")
		msg.Headers.Set("Importance", "high")
	case PriorityUrgent:
		msg.Headers.Set("X-Priority", "1")
		msg.Headers.Set("Importance", "high")
		msg.Headers.Set("X-MSMail-Priority", "High")
	case PriorityLow:
		msg.Headers.Set("X-Priority", "5")
		msg.Headers.Set("Importance", "low")
	default:
		msg.Headers.Set("X-Priority", "3")
		msg.Headers.Set("Importance", "normal")
	}

	msg.Headers.Set("X-Mailer", "Email-Campaign-System/1.0")
	
	if req.CampaignID != "" {
		msg.Headers.Set("X-Campaign-ID", req.CampaignID)
	}

	if m.config.EnableUnsubscribe && msg.Metadata.UnsubscribeToken != "" {
		unsubLink := m.buildUnsubscribeLink(msg.Metadata.UnsubscribeToken)
		msg.Headers.Set("List-Unsubscribe", fmt.Sprintf("<%s>", unsubLink))
		msg.Headers.Set("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}

	for key, value := range req.Headers {
		msg.Headers.Set(key, value)
	}
}

func (m *Manager) buildRawMessage(msg *EmailMessage) ([]byte, error) {
	var builder strings.Builder

	for key, values := range msg.Headers {
		for _, value := range values {
			builder.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}

	if len(msg.Attachments) == 0 && msg.HTMLBody != "" && msg.TextBody != "" {
		boundary := generateBoundary()
		msg.Headers.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%s", boundary))
		
		builder.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
		builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		builder.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		builder.WriteString(msg.TextBody)
		builder.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
		builder.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		builder.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		builder.WriteString(msg.HTMLBody)
		builder.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))
	} else if len(msg.Attachments) > 0 {
		boundary := generateBoundary()
		msg.Headers.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))

		contentBoundary := generateBoundary()
		builder.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
		builder.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s\r\n\r\n", contentBoundary))

		if msg.TextBody != "" {
			builder.WriteString(fmt.Sprintf("--%s\r\n", contentBoundary))
			builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
			builder.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
			builder.WriteString(msg.TextBody)
			builder.WriteString("\r\n")
		}

		if msg.HTMLBody != "" {
			builder.WriteString(fmt.Sprintf("--%s\r\n", contentBoundary))
			builder.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
			builder.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
			builder.WriteString(msg.HTMLBody)
			builder.WriteString("\r\n")
		}

		builder.WriteString(fmt.Sprintf("--%s--\r\n", contentBoundary))

		for _, att := range msg.Attachments {
			builder.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			
			if att.Inline {
				builder.WriteString(fmt.Sprintf("Content-Type: %s\r\n", att.ContentType))
				builder.WriteString(fmt.Sprintf("Content-Disposition: inline; filename=\"%s\"\r\n", att.Filename))
				if att.ContentID != "" {
					builder.WriteString(fmt.Sprintf("Content-ID: <%s>\r\n", att.ContentID))
				}
			} else {
				builder.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", att.ContentType, att.Filename))
				builder.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", att.Filename))
			}
			
			builder.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
			encoded := base64.StdEncoding.EncodeToString(att.Content)
			builder.WriteString(encoded)
			builder.WriteString("\r\n")
		}

		builder.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else if msg.HTMLBody != "" {
		msg.Headers.Set("Content-Type", "text/html; charset=UTF-8")
		msg.Headers.Set("Content-Transfer-Encoding", "quoted-printable")
		builder.WriteString("\r\n")
		builder.WriteString(msg.HTMLBody)
	} else {
		msg.Headers.Set("Content-Type", "text/plain; charset=UTF-8")
		msg.Headers.Set("Content-Transfer-Encoding", "quoted-printable")
		builder.WriteString("\r\n")
		builder.WriteString(msg.TextBody)
	}

	return []byte(builder.String()), nil
}

func (m *Manager) signMessageDKIM(msg *EmailMessage) error {
	if m.dkimPrivateKey == nil {
		return errors.New("DKIM private key not loaded")
	}

	canonicalizedHeaders := m.canonicalizeHeaders(msg.Headers)
	canonicalizedBody := m.canonicalizeBody(msg.HTMLBody)

	bodyHash := sha256.Sum256([]byte(canonicalizedBody))
	bodyHashB64 := base64.StdEncoding.EncodeToString(bodyHash[:])

	dkimHeader := fmt.Sprintf("v=1; a=rsa-sha256; c=relaxed/relaxed; d=%s; s=%s; h=from:to:subject:date:message-id; bh=%s; b=",
		m.config.DKIMDomain,
		m.config.DKIMSelector,
		bodyHashB64,
	)

	signData := canonicalizedHeaders + dkimHeader

	hashed := sha256.Sum256([]byte(signData))
	signature, err := rsa.SignPKCS1v15(rand.Reader, m.dkimPrivateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDKIMSigningFailed, err)
	}

	signatureB64 := base64.StdEncoding.EncodeToString(signature)
	dkimHeader += signatureB64

	msg.Headers.Set("DKIM-Signature", dkimHeader)

	return nil
}

func (m *Manager) canonicalizeHeaders(headers textproto.MIMEHeader) string {
	var builder strings.Builder
	
	headerOrder := []string{"From", "To", "Subject", "Date", "Message-ID"}
	
	for _, key := range headerOrder {
		if values := headers[key]; len(values) > 0 {
			builder.WriteString(strings.ToLower(key))
			builder.WriteString(":")
			builder.WriteString(strings.TrimSpace(values[0]))
			builder.WriteString("\r\n")
		}
	}
	
	return builder.String()
}

func (m *Manager) canonicalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	body = strings.TrimRight(body, "\r\n")
	return body + "\r\n"
}

func (m *Manager) generateMessageID(req *MessageRequest) string {
	timestamp := time.Now().UnixNano()
	domain := strings.Split(req.From.Address, "@")
	if len(domain) == 2 {
		return fmt.Sprintf("%d.%s.%s@%s", timestamp, req.CampaignID, req.RecipientID, domain[1])
	}
	return fmt.Sprintf("%d.%s.%s@localhost", timestamp, req.CampaignID, req.RecipientID)
}

func (m *Manager) generateTrackingID(req *MessageRequest) string {
	data := fmt.Sprintf("%s:%s:%s:%d", req.CampaignID, req.RecipientID, req.To.Address, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:16])
}

func (m *Manager) generateUnsubscribeToken(req *MessageRequest) string {
	data := fmt.Sprintf("%s:%s:%d", req.RecipientID, req.To.Address, time.Now().Unix())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:])
}

func (m *Manager) insertTrackingPixel(html string, trackingID string) string {
	if m.config.TrackingDomain == "" {
		return html
	}

	trackingURL := fmt.Sprintf("%s/track/open/%s", m.config.TrackingDomain, trackingID)
	pixel := fmt.Sprintf(`<img src="%s" alt="" width="1" height="1" style="display:none" />`, trackingURL)

	if strings.Contains(html, "</body>") {
		return strings.Replace(html, "</body>", pixel+"</body>", 1)
	}

	return html + pixel
}

func (m *Manager) wrapLinksForTracking(html string, trackingID string) string {
	if m.config.TrackingDomain == "" {
		return html
	}

	return html
}

func (m *Manager) buildUnsubscribeLink(token string) string {
	if m.config.UnsubscribeURL == "" {
		return ""
	}
	return fmt.Sprintf("%s?token=%s", m.config.UnsubscribeURL, token)
}

func (m *Manager) insertUnsubscribeLink(html string, unsubLink string) string {
	if unsubLink == "" {
		return html
	}

	unsubHTML := fmt.Sprintf(`<p style="text-align:center;font-size:12px;color:#999;"><a href="%s" style="color:#999;">Unsubscribe</a></p>`, unsubLink)

	if strings.Contains(html, "</body>") {
		return strings.Replace(html, "</body>", unsubHTML+"</body>", 1)
	}

	return html + unsubHTML
}

func (m *Manager) formatEmailAddress(addr EmailAddress) string {
	if addr.Name != "" {
		return fmt.Sprintf("%s <%s>", mime.QEncoding.Encode(m.config.DefaultCharset, addr.Name), addr.Address)
	}
	return addr.Address
}

func (e *SpamScoreEngine) CalculateScore(msg *EmailMessage) float64 {
	score := 0.0

	if msg.Subject == "" {
		score += 1.0
	}

	if strings.Contains(strings.ToUpper(msg.Subject), "FREE") {
		score += 0.5
	}

	if strings.Contains(strings.ToUpper(msg.Subject), "!!!") {
		score += 0.5
	}

	if msg.TextBody == "" && msg.HTMLBody != "" {
		score += 0.3
	}

	if len(msg.Attachments) > 5 {
		score += 0.5
	}

	if msg.HTMLBody != "" {
		htmlUpper := strings.ToUpper(msg.HTMLBody)
		spamWords := []string{"CLICK HERE", "BUY NOW", "LIMITED TIME", "ACT NOW", "URGENT"}
		for _, word := range spamWords {
			if strings.Contains(htmlUpper, word) {
				score += 0.3
			}
		}
	}

	return score
}

func loadDKIMPrivateKey(keyPath string) (*rsa.PrivateKey, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an RSA private key")
		}
	}

	return privateKey, nil
}


func isValidEmail(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	if parts[0] == "" || parts[1] == "" {
		return false
	}
	if !strings.Contains(parts[1], ".") {
		return false
	}
	return true
}

func (m *Manager) ValidateMessage(msg *EmailMessage) error {
	if msg.MessageID == "" {
		return errors.New("message ID is required")
	}

	if msg.From.Address == "" {
		return ErrMissingSender
	}

	if msg.To.Address == "" {
		return ErrMissingRecipient
	}

	if msg.Subject == "" {
		return errors.New("subject is required")
	}

	if msg.Size > m.config.MaxMessageSize {
		return fmt.Errorf("%w: %d bytes", ErrMessageTooLarge, msg.Size)
	}

	return nil
}

func (m *Manager) GetMessageInfo(msg *EmailMessage) map[string]interface{} {
	return map[string]interface{}{
		"message_id":   msg.MessageID,
		"from":         msg.From.Address,
		"to":           msg.To.Address,
		"subject":      msg.Subject,
		"size":         msg.Size,
		"spam_score":   msg.SpamScore,
		"attachments":  len(msg.Attachments),
		"campaign_id":  msg.Metadata.CampaignID,
		"template_id":  msg.Metadata.TemplateID,
		"recipient_id": msg.Metadata.RecipientID,
		"created_at":   msg.Metadata.CreatedAt,
	}
}

func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityUrgent:
		return "urgent"
	default:
		return "normal"
	}
}
