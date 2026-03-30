package provider

import (
        "context"
        "crypto/tls"
        "encoding/base64"
        "fmt"
        "net"
        "net/smtp"
        "strings"
        "sync"
        "time"

        "email-campaign-system/internal/models"
        "email-campaign-system/pkg/logger"
)

type GmailProvider struct {
        config       *ProviderConfig
        log          logger.Logger
        mu           sync.RWMutex
        health       *ProviderHealth
        rateLimiter  *GmailRateLimiter
        lastSend     time.Time
        messageCount int64
        errorCount   int64
}

type GmailRateLimiter struct {
        hourlyLimit   int
        dailyLimit    int
        hourlySent    int
        dailySent     int
        lastHourReset time.Time
        lastDayReset  time.Time
        mu            sync.Mutex
}

func NewGmailProvider(config *ProviderConfig, log logger.Logger) (Provider, error) {
        if config == nil {
                return nil, ErrInvalidConfig
        }

        if config.Password == "" {
                return nil, fmt.Errorf("app password is required for gmail")
        }

        if config.Username == "" {
                return nil, fmt.Errorf("username/email is required")
        }

        provider := &GmailProvider{
                config: config,
                log:    log,
                health: &ProviderHealth{
                        Status:      HealthStatusUnknown,
                        LastCheck:   time.Now(),
                        IsConnected: false,
                },
                rateLimiter: &GmailRateLimiter{
                        hourlyLimit:   config.RateLimitPerHour,
                        dailyLimit:    config.RateLimitPerDay,
                        lastHourReset: time.Now(),
                        lastDayReset:  time.Now(),
                },
                lastSend: time.Now(),
        }

        if config.Host == "" {
                config.Host = "smtp.gmail.com"
        }
        if config.Port == 0 {
                config.Port = 587
        }

        if err := provider.TestConnection(context.Background()); err != nil {
                if log != nil {
                        (log).Warn(fmt.Sprintf("initial connection test failed: %v", err))
                }
        }

        provider.health.Status = HealthStatusHealthy

        return provider, nil
}

func (p *GmailProvider) Send(ctx context.Context, message *models.Email) (string, error) {
        if message == nil {
                return "", fmt.Errorf("message cannot be nil")
        }

        if err := p.checkRateLimit(); err != nil {
                return "", err
        }

        startTime := time.Now()

        rawMessage, err := p.buildMessage(message)
        if err != nil {
                p.incrementErrorCount()
                return "", fmt.Errorf("failed to build message: %w", err)
        }

        if p.config.MaxMessageSize > 0 && int64(len(rawMessage)) > p.config.MaxMessageSize {
                return "", fmt.Errorf("message exceeds size limit")
        }

        client, err := p.getClient()
        if err != nil {
                p.incrementErrorCount()
                p.updateHealth(false, time.Since(startTime))
                return "", fmt.Errorf("failed to get smtp client: %w", err)
        }
        defer client.Quit()

        if err := client.Mail(message.From.Address); err != nil {
                p.incrementErrorCount()
                p.updateHealth(false, time.Since(startTime))
                return "", fmt.Errorf("failed to set sender: %w", err)
        }

        recipients := []string{message.To.Address}
        for _, cc := range message.CC {
                recipients = append(recipients, cc.Address)
        }
        for _, bcc := range message.BCC {
                recipients = append(recipients, bcc.Address)
        }

        for _, recipient := range recipients {
                if err := client.Rcpt(recipient); err != nil {
                        p.incrementErrorCount()
                        p.updateHealth(false, time.Since(startTime))
                        return "", fmt.Errorf("failed to add recipient %s: %w", recipient, err)
                }
        }

        writer, err := client.Data()
        if err != nil {
                p.incrementErrorCount()
                p.updateHealth(false, time.Since(startTime))
                return "", fmt.Errorf("failed to get data writer: %w", err)
        }

        _, err = writer.Write([]byte(rawMessage))
        if err != nil {
                writer.Close()
                p.incrementErrorCount()
                p.updateHealth(false, time.Since(startTime))
                return "", fmt.Errorf("failed to write message: %w", err)
        }

        if err := writer.Close(); err != nil {
                p.incrementErrorCount()
                p.updateHealth(false, time.Since(startTime))
                return "", fmt.Errorf("failed to close writer: %w", err)
        }

        messageID := p.generateMessageID(message)

        p.incrementMessageCount()
        p.updateHealth(true, time.Since(startTime))
        p.lastSend = time.Now()

        if p.log != nil {
                (p.log).Debug(fmt.Sprintf("email sent via gmail smtp: message_id=%s, to=%s, duration=%v", messageID, message.To.Address, time.Since(startTime)))
        }

        return messageID, nil
}

func (p *GmailProvider) getClient() (*smtp.Client, error) {
        addr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)

        conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
        if err != nil {
                return nil, fmt.Errorf("connection failed: %w", err)
        }

        client, err := smtp.NewClient(conn, p.config.Host)
        if err != nil {
                conn.Close()
                return nil, fmt.Errorf("failed to create smtp client: %w", err)
        }

        tlsConfig := &tls.Config{
                ServerName:         p.config.Host,
                InsecureSkipVerify: false,
                MinVersion:         tls.VersionTLS12,
        }

        if err := client.StartTLS(tlsConfig); err != nil {
                client.Close()
                return nil, fmt.Errorf("failed to start tls: %w", err)
        }

        auth := smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.Host)
        if err := client.Auth(auth); err != nil {
                client.Close()
                return nil, fmt.Errorf("authentication failed: %w", err)
        }

        return client, nil
}

func (p *GmailProvider) buildMessage(message *models.Email) (string, error) {
        var msg strings.Builder

        msg.WriteString(fmt.Sprintf("From: %s\r\n", message.From.FormatAddress()))
        msg.WriteString(fmt.Sprintf("To: %s\r\n", message.To.FormatAddress()))

        if len(message.CC) > 0 {
                ccAddresses := make([]string, len(message.CC))
                for i, cc := range message.CC {
                        ccAddresses[i] = cc.FormatAddress()
                }
                msg.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(ccAddresses, ", ")))
        }

        if message.ReplyTo != nil && message.ReplyTo.Address != "" {
                msg.WriteString(fmt.Sprintf("Reply-To: %s\r\n", message.ReplyTo.FormatAddress()))
        }

        msg.WriteString(fmt.Sprintf("Subject: %s\r\n", message.Subject))

        messageID := p.generateMessageID(message)
        msg.WriteString(fmt.Sprintf("Message-ID: <%s>\r\n", messageID))

        msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
        msg.WriteString("MIME-Version: 1.0\r\n")

        if message.Headers != nil {
                for key, value := range message.Headers {
                        msg.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
                }
        }

        if len(message.Attachments) > 0 {
                boundary := p.generateBoundary()
                msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary))

                msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
                msg.WriteString(p.buildBodyPart(message))

                for _, attachment := range message.Attachments {
                        msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
                        msg.WriteString(p.buildAttachmentPart(&attachment))
                }

                msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
        } else {
                msg.WriteString(p.buildBodyPart(message))
        }

        return msg.String(), nil
}

func (p *GmailProvider) buildBodyPart(message *models.Email) string {
        var body strings.Builder

        if message.HTMLBody != "" && message.PlainTextBody != "" {
                boundary := p.generateBoundary()
                body.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))

                body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
                body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
                body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
                body.WriteString(message.PlainTextBody)
                body.WriteString("\r\n\r\n")

                body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
                body.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
                body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
                body.WriteString(message.HTMLBody)
                body.WriteString("\r\n\r\n")

                body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
        } else if message.HTMLBody != "" {
                body.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
                body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
                body.WriteString(message.HTMLBody)
                body.WriteString("\r\n")
        } else {
                body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
                body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
                body.WriteString(message.PlainTextBody)
                body.WriteString("\r\n")
        }

        return body.String()
}

func (p *GmailProvider) buildAttachmentPart(attachment *models.EmailAttachment) string {
        var part strings.Builder

        part.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", attachment.ContentType, attachment.Filename))
        part.WriteString("Content-Transfer-Encoding: base64\r\n")
        part.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", attachment.Filename))

        encoded := base64.StdEncoding.EncodeToString(attachment.Content)
        for i := 0; i < len(encoded); i += 76 {
                end := i + 76
                if end > len(encoded) {
                        end = len(encoded)
                }
                part.WriteString(encoded[i:end])
                part.WriteString("\r\n")
        }

        return part.String()
}

func (p *GmailProvider) generateBoundary() string {
        return fmt.Sprintf("----=_Part_%d_%d", time.Now().Unix(), time.Now().UnixNano())
}

func (p *GmailProvider) generateMessageID(message *models.Email) string {
        domain := "gmail.com"
        if strings.Contains(message.From.Address, "@") {
                parts := strings.Split(message.From.Address, "@")
                if len(parts) == 2 {
                        domain = parts[1]
                }
        }
        return fmt.Sprintf("%d.%d@%s", time.Now().Unix(), time.Now().UnixNano(), domain)
}

func (p *GmailProvider) checkRateLimit() error {
        p.rateLimiter.mu.Lock()
        defer p.rateLimiter.mu.Unlock()

        now := time.Now()

        if now.Sub(p.rateLimiter.lastHourReset) >= time.Hour {
                p.rateLimiter.hourlySent = 0
                p.rateLimiter.lastHourReset = now
        }

        if now.Sub(p.rateLimiter.lastDayReset) >= 24*time.Hour {
                p.rateLimiter.dailySent = 0
                p.rateLimiter.lastDayReset = now
        }

        if p.rateLimiter.hourlyLimit > 0 && p.rateLimiter.hourlySent >= p.rateLimiter.hourlyLimit {
                return fmt.Errorf("hourly rate limit exceeded: %d/%d", p.rateLimiter.hourlySent, p.rateLimiter.hourlyLimit)
        }

        if p.rateLimiter.dailyLimit > 0 && p.rateLimiter.dailySent >= p.rateLimiter.dailyLimit {
                return fmt.Errorf("daily rate limit exceeded: %d/%d", p.rateLimiter.dailySent, p.rateLimiter.dailyLimit)
        }

        p.rateLimiter.hourlySent++
        p.rateLimiter.dailySent++

        return nil
}

func (p *GmailProvider) TestConnection(ctx context.Context) error {
        startTime := time.Now()

        client, err := p.getClient()
        if err != nil {
                p.updateHealth(false, time.Since(startTime))
                return fmt.Errorf("connection test failed: %w", err)
        }
        defer client.Quit()

        p.updateHealth(true, time.Since(startTime))
        p.health.IsConnected = true

        if p.log != nil {
                (p.log).Debug(fmt.Sprintf("gmail connection test successful: email=%s", p.config.Username))
        }

        return nil
}

func (p *GmailProvider) Validate() error {
        if p.config == nil {
                return ErrInvalidConfig
        }

        if p.config.Username == "" {
                return fmt.Errorf("username is required")
        }

        if p.config.Password == "" {
                return fmt.Errorf("app password is required")
        }

        if p.config.Host == "" {
                return fmt.Errorf("smtp host is required")
        }

        if p.config.Port == 0 {
                return fmt.Errorf("smtp port is required")
        }

        return nil
}

func (p *GmailProvider) Close() error {
        p.mu.Lock()
        defer p.mu.Unlock()

        p.health.IsConnected = false

        if p.log != nil {
                (p.log).Info("gmail provider closed")
        }

        return nil
}

func (p *GmailProvider) Name() string {
        return fmt.Sprintf("Gmail (%s)", p.config.Username)
}

func (p *GmailProvider) Type() ProviderType {
        return ProviderTypeGmail
}

func (p *GmailProvider) GetHealth() *ProviderHealth {
        p.mu.RLock()
        defer p.mu.RUnlock()

        healthCopy := *p.health
        return &healthCopy
}

func (p *GmailProvider) SupportedFeatures() []Feature {
        return []Feature{
                FeatureSMTP,
                FeatureHTMLBody,
                FeatureTextBody,
                FeatureAttachments,
                FeatureTLS,
        }
}

func (p *GmailProvider) GetConfig() *ProviderConfig {
        p.mu.RLock()
        defer p.mu.RUnlock()
        return p.config
}

func (p *GmailProvider) updateHealth(success bool, responseTime time.Duration) {
        p.mu.Lock()
        defer p.mu.Unlock()

        now := time.Now()

        if success {
                p.health.LastSuccess = now
                p.health.ConsecutiveFails = 0
                p.health.Status = HealthStatusHealthy
                p.health.Message = "operational"
        } else {
                p.health.LastFailure = now
                p.health.ConsecutiveFails++

                if p.health.ConsecutiveFails >= 5 {
                        p.health.Status = HealthStatusUnhealthy
                        p.health.Message = "multiple consecutive failures"
                } else if p.health.ConsecutiveFails >= 3 {
                        p.health.Status = HealthStatusDegraded
                        p.health.Message = "experiencing issues"
                }
        }

        p.health.LastCheck = now

        if responseTime > 0 {
                if p.health.AvgResponseTime == 0 {
                        p.health.AvgResponseTime = responseTime
                } else {
                        p.health.AvgResponseTime = (p.health.AvgResponseTime + responseTime) / 2
                }
        }

        total := p.health.TotalSent + p.health.TotalFailed
        if total > 0 {
                p.health.ErrorRate = float64(p.health.TotalFailed) / float64(total) * 100
        }
}

func (p *GmailProvider) incrementMessageCount() {
        p.mu.Lock()
        defer p.mu.Unlock()
        p.messageCount++
        p.health.TotalSent++
}

func (p *GmailProvider) incrementErrorCount() {
        p.mu.Lock()
        defer p.mu.Unlock()
        p.errorCount++
        p.health.TotalFailed++
}

func (p *GmailProvider) GetStats() map[string]interface{} {
        p.mu.RLock()
        defer p.mu.RUnlock()

        p.rateLimiter.mu.Lock()
        hourlySent := p.rateLimiter.hourlySent
        dailySent := p.rateLimiter.dailySent
        p.rateLimiter.mu.Unlock()

        return map[string]interface{}{
                "total_sent":    p.messageCount,
                "total_errors":  p.errorCount,
                "hourly_sent":   hourlySent,
                "daily_sent":    dailySent,
                "last_send":     p.lastSend,
                "health_status": p.health.Status,
        }
}
