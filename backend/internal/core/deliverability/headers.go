package deliverability

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "net/mail"
    "strings"
    "time"

    "email-campaign-system/internal/models"
)

type HeaderBuilder struct {
    headers        map[string]string
    config         *HeaderConfig
    campaignID     string
    accountEmail   string
    recipientEmail string
}

type HeaderConfig struct {
    EnableFBL                bool
    EnableListUnsubscribe    bool
    EnableOneClickUnsubscribe bool
    FBLIdentifier            string
    UnsubscribeURL           string
    Domain                   string
    Organization             string
    CustomHeaders            map[string]string
    EnableXMailer            bool
    MailerName               string
    EnablePrecedence         bool
    PrecedenceValue          string
}

type EmailHeaders struct {
    MessageID            string
    Date                 string
    From                 string
    To                   string
    Subject              string
    ReplyTo              string
    ReturnPath           string
    FeedbackID           string
    ListUnsubscribe      string
    ListUnsubscribePost  string
    MIMEVersion          string
    ContentType          string
    ContentTransferEncoding string
    XMailer              string
    XPriority            string
    Precedence           string
    Custom               map[string]string
}

func NewHeaderBuilder() *HeaderBuilder {
    return &HeaderBuilder{
        headers: make(map[string]string),
        config:  DefaultHeaderConfig(),
    }
}

func DefaultHeaderConfig() *HeaderConfig {
    return &HeaderConfig{
        EnableFBL:                true,
        EnableListUnsubscribe:    true,
        EnableOneClickUnsubscribe: true,
        FBLIdentifier:            "campaign",
        Domain:                   "example.com",
        Organization:             "Campaign System",
        CustomHeaders:            make(map[string]string),
        EnableXMailer:            false,
        MailerName:               "Email Campaign System v1.0",
        EnablePrecedence:         true,
        PrecedenceValue:          "bulk",
    }
}

func (hb *HeaderBuilder) WithConfig(config *HeaderConfig) *HeaderBuilder {
    hb.config = config
    return hb
}

func (hb *HeaderBuilder) WithCampaign(campaignID string) *HeaderBuilder {
    hb.campaignID = campaignID
    return hb
}

func (hb *HeaderBuilder) WithAccount(email string) *HeaderBuilder {
    hb.accountEmail = email
    return hb
}

func (hb *HeaderBuilder) WithRecipient(email string) *HeaderBuilder {
    hb.recipientEmail = email
    return hb
}

func (hb *HeaderBuilder) Build(email *models.Email) (*EmailHeaders, error) {
    headers := &EmailHeaders{
        Custom: make(map[string]string),
    }

    headers.MessageID = hb.generateMessageID()
    headers.Date = hb.generateDateHeader()
    
    // ✅ Fixed: From and To are structs, not pointers or arrays
    if email.From.Address != "" {
        headers.From = hb.formatFromHeader(email.From.Address, email.From.Name)
    }
    
    if email.To.Address != "" {
        headers.To = hb.formatToHeader(email.To.Address, email.To.Name)
    }
    
    headers.Subject = email.Subject
    headers.MIMEVersion = "1.0"

    // ✅ Fixed: ReplyTo is a pointer
    if email.ReplyTo != nil && email.ReplyTo.Address != "" {
        headers.ReplyTo = email.ReplyTo.Address
    }

    if hb.accountEmail != "" {
        headers.ReturnPath = fmt.Sprintf("<%s>", hb.accountEmail)
    }

    if hb.config.EnableFBL {
        headers.FeedbackID = hb.generateFBLHeader()
    }

    if hb.config.EnableListUnsubscribe {
        unsubscribeURL := hb.generateUnsubscribeURL(email)
        if unsubscribeURL != "" {
            headers.ListUnsubscribe = fmt.Sprintf("<%s>", unsubscribeURL)
            
            if hb.config.EnableOneClickUnsubscribe {
                headers.ListUnsubscribePost = "List-Unsubscribe=One-Click"
            }
        }
    }

    if hb.config.EnableXMailer {
        headers.XMailer = hb.config.MailerName
    }

    if hb.config.EnablePrecedence {
        headers.Precedence = hb.config.PrecedenceValue
    }

    for key, value := range hb.config.CustomHeaders {
        headers.Custom[key] = value
    }

    return headers, nil
}

func (hb *HeaderBuilder) generateMessageID() string {
    timestamp := time.Now().Unix()
    randomBytes := make([]byte, 16)
    rand.Read(randomBytes)
    randomHex := hex.EncodeToString(randomBytes)
    
    domain := hb.config.Domain
    if domain == "" {
        domain = "localhost"
    }

    if hb.campaignID != "" && len(hb.campaignID) >= 8 {
        return fmt.Sprintf("<%d.%s.%s@%s>", timestamp, hb.campaignID[:8], randomHex[:16], domain)
    }

    return fmt.Sprintf("<%d.%s@%s>", timestamp, randomHex[:16], domain)
}

func (hb *HeaderBuilder) generateFBLHeader() string {
    parts := make([]string, 0, 4)

    if hb.campaignID != "" {
        parts = append(parts, fmt.Sprintf("%s:%s", hb.config.FBLIdentifier, hb.campaignID))
    }

    if hb.accountEmail != "" {
        parts = append(parts, fmt.Sprintf("sender:%s", hb.accountEmail))
    }

    if hb.recipientEmail != "" {
        recipientHash := hb.hashEmail(hb.recipientEmail)
        if len(recipientHash) >= 16 {
            parts = append(parts, fmt.Sprintf("recipient:%s", recipientHash[:16]))
        }
    }

    timestamp := time.Now().Unix()
    parts = append(parts, fmt.Sprintf("ts:%d", timestamp))

    return strings.Join(parts, ":")
}

func (hb *HeaderBuilder) hashEmail(email string) string {
    email = strings.ToLower(strings.TrimSpace(email))
    randomBytes := make([]byte, 8)
    rand.Read(randomBytes)
    return hex.EncodeToString(randomBytes) + hex.EncodeToString([]byte(email))
}

func (hb *HeaderBuilder) generateUnsubscribeURL(email *models.Email) string {
    if hb.config.UnsubscribeURL == "" {
        return ""
    }

    baseURL := strings.TrimSuffix(hb.config.UnsubscribeURL, "/")
    
    token := hb.generateUnsubscribeToken(email)
    
    // ✅ Fixed: To is a struct, not an array
    recipientEmail := email.To.Address
    
    return fmt.Sprintf("%s?token=%s&email=%s", baseURL, token, recipientEmail)
}

func (hb *HeaderBuilder) generateUnsubscribeToken(email *models.Email) string {
    // ✅ Fixed: To is a struct
    recipientEmail := email.To.Address
    
    data := fmt.Sprintf("%s:%s:%d", recipientEmail, hb.campaignID, time.Now().Unix())
    randomBytes := make([]byte, 16)
    rand.Read(randomBytes)
    return hex.EncodeToString([]byte(data)) + hex.EncodeToString(randomBytes)
}

func (hb *HeaderBuilder) generateDateHeader() string {
    return time.Now().Format(time.RFC1123Z)
}

func (hb *HeaderBuilder) formatFromHeader(email, name string) string {
    if name == "" {
        return fmt.Sprintf("<%s>", email)
    }
    
    addr := mail.Address{
        Name:    name,
        Address: email,
    }
    return addr.String()
}

func (hb *HeaderBuilder) formatToHeader(email, name string) string {
    if name == "" {
        return fmt.Sprintf("<%s>", email)
    }
    
    addr := mail.Address{
        Name:    name,
        Address: email,
    }
    return addr.String()
}

func (hb *HeaderBuilder) AddCustomHeader(key, value string) *HeaderBuilder {
    hb.headers[key] = value
    return hb
}

func (hb *HeaderBuilder) SetHeader(key, value string) *HeaderBuilder {
    hb.headers[key] = value
    return hb
}

func (hb *HeaderBuilder) GetHeaders() map[string]string {
    return hb.headers
}

func (eh *EmailHeaders) ToMap() map[string]string {
    headers := make(map[string]string)

    headers["Message-ID"] = eh.MessageID
    headers["Date"] = eh.Date
    headers["From"] = eh.From
    headers["To"] = eh.To
    headers["Subject"] = eh.Subject
    headers["MIME-Version"] = eh.MIMEVersion

    if eh.ReplyTo != "" {
        headers["Reply-To"] = eh.ReplyTo
    }

    if eh.ReturnPath != "" {
        headers["Return-Path"] = eh.ReturnPath
    }

    if eh.FeedbackID != "" {
        headers["Feedback-ID"] = eh.FeedbackID
    }

    if eh.ListUnsubscribe != "" {
        headers["List-Unsubscribe"] = eh.ListUnsubscribe
    }

    if eh.ListUnsubscribePost != "" {
        headers["List-Unsubscribe-Post"] = eh.ListUnsubscribePost
    }

    if eh.ContentType != "" {
        headers["Content-Type"] = eh.ContentType
    }

    if eh.ContentTransferEncoding != "" {
        headers["Content-Transfer-Encoding"] = eh.ContentTransferEncoding
    }

    if eh.XMailer != "" {
        headers["X-Mailer"] = eh.XMailer
    }

    if eh.XPriority != "" {
        headers["X-Priority"] = eh.XPriority
    }

    if eh.Precedence != "" {
        headers["Precedence"] = eh.Precedence
    }

    for key, value := range eh.Custom {
        headers[key] = value
    }

    return headers
}

func (eh *EmailHeaders) ToString() string {
    var sb strings.Builder

    for key, value := range eh.ToMap() {
        sb.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
    }

    return sb.String()
}

func (eh *EmailHeaders) Validate() error {
    if eh.MessageID == "" {
        return fmt.Errorf("message-id is required")
    }

    if eh.From == "" {
        return fmt.Errorf("from header is required")
    }

    if eh.To == "" {
        return fmt.Errorf("to header is required")
    }

    if eh.Subject == "" {
        return fmt.Errorf("subject is required")
    }

    if eh.Date == "" {
        return fmt.Errorf("date header is required")
    }

    return nil
}

func GenerateMessageID(domain string) string {
    timestamp := time.Now().Unix()
    randomBytes := make([]byte, 16)
    rand.Read(randomBytes)
    randomHex := hex.EncodeToString(randomBytes)
    
    if domain == "" {
        domain = "localhost"
    }

    return fmt.Sprintf("<%d.%s@%s>", timestamp, randomHex, domain)
}

func GenerateFeedbackID(campaignID, senderEmail, recipientEmail string) string {
    parts := []string{
        fmt.Sprintf("campaign:%s", campaignID),
        fmt.Sprintf("sender:%s", senderEmail),
        fmt.Sprintf("ts:%d", time.Now().Unix()),
    }

    return strings.Join(parts, ":")
}

func GenerateListUnsubscribeHeader(unsubscribeURL, unsubscribeEmail string) string {
    parts := make([]string, 0, 2)

    if unsubscribeURL != "" {
        parts = append(parts, fmt.Sprintf("<%s>", unsubscribeURL))
    }

    if unsubscribeEmail != "" {
        parts = append(parts, fmt.Sprintf("<mailto:%s>", unsubscribeEmail))
    }

    if len(parts) == 0 {
        return ""
    }

    return strings.Join(parts, ", ")
}

func ValidateEmailAddress(address string) error {
    _, err := mail.ParseAddress(address)
    return err
}

func ParseEmailAddress(address string) (*mail.Address, error) {
    return mail.ParseAddress(address)
}

func FormatEmailAddress(email, name string) string {
    if name == "" {
        return fmt.Sprintf("<%s>", email)
    }
    
    addr := mail.Address{
        Name:    name,
        Address: email,
    }
    return addr.String()
}

func SanitizeHeaderValue(value string) string {
    value = strings.TrimSpace(value)
    value = strings.ReplaceAll(value, "\r", "")
    value = strings.ReplaceAll(value, "\n", " ")
    return value
}

func AddSecurityHeaders(headers map[string]string) {
    headers["X-Content-Type-Options"] = "nosniff"
    headers["X-Frame-Options"] = "DENY"
}

func SetPriority(headers map[string]string, priority int) {
    if priority < 1 {
        priority = 1
    }
    if priority > 5 {
        priority = 5
    }
    
    headers["X-Priority"] = fmt.Sprintf("%d", priority)
    
    switch priority {
    case 1:
        headers["Importance"] = "high"
    case 2:
        headers["Importance"] = "high"
    case 3:
        headers["Importance"] = "normal"
    case 4:
        headers["Importance"] = "low"
    case 5:
        headers["Importance"] = "low"
    }
}

func SetBulkPrecedence(headers map[string]string) {
    headers["Precedence"] = "bulk"
    headers["Auto-Submitted"] = "auto-generated"
}

func GenerateReferencesHeader(previousMessageIDs []string) string {
    if len(previousMessageIDs) == 0 {
        return ""
    }
    return strings.Join(previousMessageIDs, " ")
}

func GenerateInReplyToHeader(messageID string) string {
    if messageID == "" {
        return ""
    }
    return messageID
}

func ExtractDomain(email string) string {
    parts := strings.Split(email, "@")
    if len(parts) != 2 {
        return ""
    }
    return parts[1]
}

func IsValidMessageID(messageID string) bool {
    if messageID == "" {
        return false
    }
    
    if !strings.HasPrefix(messageID, "<") || !strings.HasSuffix(messageID, ">") {
        return false
    }
    
    content := messageID[1 : len(messageID)-1]
    return strings.Contains(content, "@")
}

func CleanHeaderValue(value string) string {
    value = strings.TrimSpace(value)
    value = strings.ReplaceAll(value, "\r\n", " ")
    value = strings.ReplaceAll(value, "\r", " ")
    value = strings.ReplaceAll(value, "\n", " ")
    
    for strings.Contains(value, "  ") {
        value = strings.ReplaceAll(value, "  ", " ")
    }
    
    return value
}
