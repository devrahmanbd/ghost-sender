package provider

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"email-campaign-system/internal/models"
	"email-campaign-system/pkg/logger"
)

var (
	ErrSMTPConnectionFailed = errors.New("smtp connection failed")
	ErrSMTPAuthFailed       = errors.New("smtp authentication failed")
	ErrSMTPSendFailed       = errors.New("smtp send failed")
	ErrConnectionClosed     = errors.New("smtp connection closed")
	ErrMessageTooLarge      = errors.New("message exceeds size limit")
)

type SMTPProvider struct {
	config       *ProviderConfig
	log          *logger.Logger
	client       *smtp.Client
	conn         net.Conn
	mu           sync.RWMutex
	health       *ProviderHealth
	rateLimiter  *SMTPRateLimiter
	connPool     *SMTPConnectionPool
	lastSend     time.Time
	messageCount int64
	errorCount   int64
}

type SMTPRateLimiter struct {
	hourlyLimit   int
	dailyLimit    int
	hourlySent    int
	dailySent     int
	lastHourReset time.Time
	lastDayReset  time.Time
	mu            sync.Mutex
}

type SMTPConnection struct {
	client    *smtp.Client
	conn      net.Conn
	createdAt time.Time
	lastUsed  time.Time
	useCount  int
}

func (p *SMTPProvider) Send(ctx context.Context, message *models.Email) (string, error) {
	if message == nil {
		return "", errors.New("message cannot be nil")
	}

	if err := p.checkRateLimit(); err != nil {
		return "", err
	}

	startTime := time.Now()

	mimeMessage, err := p.buildMIMEMessage(message)
	if err != nil {
		p.incrementErrorCount()
		return "", fmt.Errorf("failed to build MIME message: %w", err)
	}

	if int64(len(mimeMessage)) > p.config.MaxMessageSize {
		return "", ErrMessageTooLarge
	}

	var client *smtp.Client
	var pooledConn *PooledConnection

	if p.connPool != nil {
		pooledConn, err = p.connPool.Acquire(ctx)
		if err != nil {
			p.incrementErrorCount()
			p.updateHealth(false, time.Since(startTime))
			return "", fmt.Errorf("failed to acquire connection: %w", err)
		}
		defer p.connPool.Release(pooledConn)
		client = pooledConn.GetSMTPClient()
	} else {
		smtpConn, err := p.createConnection()
		if err != nil {
			p.incrementErrorCount()
			p.updateHealth(false, time.Since(startTime))
			return "", fmt.Errorf("failed to create connection: %w", err)
		}
		defer p.closeConnection(smtpConn)
		client = smtpConn.client
	}

	if err := p.sendMessageWithClient(client, message, mimeMessage); err != nil {
		p.incrementErrorCount()
		p.updateHealth(false, time.Since(startTime))
		return "", fmt.Errorf("%w: %v", ErrSMTPSendFailed, err)
	}

	p.incrementMessageCount()
	p.updateHealth(true, time.Since(startTime))
	p.lastSend = time.Now()

	messageID := p.extractMessageID(message)

	if p.log != nil {
		(*p.log).Debug(fmt.Sprintf("email sent via smtp: to=%s, duration=%v", message.To.Address, time.Since(startTime)))
	}

	return messageID, nil
}

func (p *SMTPProvider) createConnection() (*SMTPConnection, error) {
	addr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)

	var conn net.Conn
	var err error

	dialer := &net.Dialer{
		Timeout: p.config.TimeoutConfig.Connect,
	}

	if p.config.TLSConfig != nil && p.config.TLSConfig.Enabled {
		tlsConfig := &tls.Config{
			ServerName:         p.config.Host,
			InsecureSkipVerify: p.config.TLSConfig.InsecureSkipVerify,
		}

		if p.config.TLSConfig.MinVersion > 0 {
			tlsConfig.MinVersion = p.config.TLSConfig.MinVersion
		}

		if p.config.Port == 465 {
			conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		} else {
			conn, err = dialer.Dial("tcp", addr)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrSMTPConnectionFailed, err)
			}
		}
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSMTPConnectionFailed, err)
	}

	client, err := smtp.NewClient(conn, p.config.Host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrSMTPConnectionFailed, err)
	}

	if p.config.TLSConfig != nil && p.config.TLSConfig.Enabled && p.config.Port != 465 {
		tlsConfig := &tls.Config{
			ServerName:         p.config.Host,
			InsecureSkipVerify: p.config.TLSConfig.InsecureSkipVerify,
		}

		if err := client.StartTLS(tlsConfig); err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	if p.config.Password != "" {
		auth := smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.Host)
		if err := client.Auth(auth); err != nil {
			client.Close()
			return nil, fmt.Errorf("%w: %v", ErrSMTPAuthFailed, err)
		}
	}

	return &SMTPConnection{
		client:    client,
		conn:      conn,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		useCount:  0,
	}, nil
}

func (p *SMTPProvider) sendMessageWithClient(client *smtp.Client, message *models.Email, mimeMessage []byte) error {
	if err := client.Mail(message.From.Address); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}

	if err := client.Rcpt(message.To.Address); err != nil {
		return fmt.Errorf("RCPT TO failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA command failed: %w", err)
	}

	if _, err := w.Write(mimeMessage); err != nil {
		w.Close()
		return fmt.Errorf("write message failed: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close DATA writer failed: %w", err)
	}

	return nil
}

func (p *SMTPProvider) buildMIMEMessage(message *models.Email) ([]byte, error) {
	var msg strings.Builder

	if message.Headers != nil {
		for key, value := range message.Headers {
			msg.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}

	msg.WriteString(fmt.Sprintf("From: %s\r\n", message.From.FormatAddress()))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", message.To.FormatAddress()))

	if message.ReplyTo != nil && message.ReplyTo.Address != "" {
		msg.WriteString(fmt.Sprintf("Reply-To: %s\r\n", message.ReplyTo.FormatAddress()))
	}

	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", message.Subject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString("MIME-Version: 1.0\r\n")

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

	return []byte(msg.String()), nil
}

func (p *SMTPProvider) buildBodyPart(message *models.Email) string {
	var body strings.Builder

	if message.HTMLBody != "" && message.PlainTextBody != "" {
		boundary := p.generateBoundary()
		body.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))

		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		body.WriteString(message.PlainTextBody)
		body.WriteString("\r\n\r\n")

		body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		body.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		body.WriteString(message.HTMLBody)
		body.WriteString("\r\n\r\n")

		body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else if message.HTMLBody != "" {
		body.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		body.WriteString(message.HTMLBody)
		body.WriteString("\r\n")
	} else {
		body.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
		body.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		body.WriteString(message.PlainTextBody)
		body.WriteString("\r\n")
	}

	return body.String()
}

func (p *SMTPProvider) buildAttachmentPart(attachment *models.EmailAttachment) string {
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

func (p *SMTPProvider) closeConnection(conn *SMTPConnection) {
	if conn != nil {
		if conn.client != nil {
			conn.client.Quit()
			conn.client.Close()
		}
		if conn.conn != nil {
			conn.conn.Close()
		}
	}
}

func (p *SMTPProvider) TestConnection(ctx context.Context) error {
	conn, err := p.createConnection()
	if err != nil {
		p.updateHealth(false, 0)
		return err
	}
	defer p.closeConnection(conn)

	if err := conn.client.Noop(); err != nil {
		p.updateHealth(false, 0)
		return fmt.Errorf("NOOP command failed: %w", err)
	}

	p.updateHealth(true, 0)
	if p.log != nil {
		(*p.log).Debug("smtp connection test successful")
	}

	return nil
}

func (p *SMTPProvider) Validate() error {
	if p.config == nil {
		return ErrInvalidConfig
	}

	if p.config.Host == "" {
		return fmt.Errorf("smtp host is required")
	}

	if p.config.Port <= 0 || p.config.Port > 65535 {
		return fmt.Errorf("invalid smtp port")
	}

	if p.config.Username == "" {
		return fmt.Errorf("smtp username is required")
	}

	return nil
}

func (p *SMTPProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.connPool != nil {
		p.connPool.Close()
	}

	if p.client != nil {
		p.client.Quit()
		p.client.Close()
	}

	p.health.IsConnected = false
	if p.log != nil {
		(*p.log).Info("smtp provider closed")
	}

	return nil
}

func (p *SMTPProvider) Name() string {
	return fmt.Sprintf("SMTP (%s)", p.config.Username)
}

func (p *SMTPProvider) Type() ProviderType {
	return ProviderTypeSMTP
}

func (p *SMTPProvider) SupportedFeatures() []Feature {
	return []Feature{
		FeatureTLS,
		FeatureSSL,
		FeatureSTARTTLS,
		FeatureHTMLBody,
		FeatureTextBody,
		FeatureAttachments,
		FeatureConnectionPool,
	}
}

func (p *SMTPProvider) GetConfig() *ProviderConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *SMTPProvider) GetHealth() *ProviderHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()

	health := *p.health
	return &health
}

func (p *SMTPProvider) generateBoundary() string {
	return fmt.Sprintf("----=_Part_%d_%d", time.Now().Unix(), time.Now().UnixNano())
}

func (p *SMTPProvider) extractMessageID(message *models.Email) string {
	if message.Headers != nil {
		if msgID, ok := message.Headers["Message-ID"]; ok {
			return msgID
		}
	}
	return fmt.Sprintf("<%d.%s@%s>", time.Now().Unix(), p.config.Username, p.config.Host)
}

func (p *SMTPProvider) checkRateLimit() error {
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

	if p.rateLimiter.hourlySent >= p.rateLimiter.hourlyLimit {
		return fmt.Errorf("hourly rate limit exceeded: %d/%d", p.rateLimiter.hourlySent, p.rateLimiter.hourlyLimit)
	}

	if p.rateLimiter.dailySent >= p.rateLimiter.dailyLimit {
		return fmt.Errorf("daily rate limit exceeded: %d/%d", p.rateLimiter.dailySent, p.rateLimiter.dailyLimit)
	}

	p.rateLimiter.hourlySent++
	p.rateLimiter.dailySent++

	return nil
}

func (p *SMTPProvider) updateHealth(success bool, responseTime time.Duration) {
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

func (p *SMTPProvider) incrementMessageCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messageCount++
	p.health.TotalSent++
}

func (p *SMTPProvider) incrementErrorCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errorCount++
	p.health.TotalFailed++
}
