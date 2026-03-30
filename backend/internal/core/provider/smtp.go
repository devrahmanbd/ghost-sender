package provider

import (
        "context"
        "crypto/tls"
        "encoding/base64"
        "errors"
        "fmt"
        "net"
        "net/smtp"
        "os"
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
        proxyConfig  *ProxyConfig
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

func (p *SMTPProvider) SetProxy(proxy *ProxyConfig) {
        p.mu.Lock()
        defer p.mu.Unlock()
        p.proxyConfig = proxy
}

func (p *SMTPProvider) dialWithProxy(addr string) (net.Conn, error) {
        pc := p.proxyConfig
        proxyAddr := fmt.Sprintf("%s:%d", pc.Host, pc.Port)
        timeout := p.config.TimeoutConfig.Connect

        dialer := &net.Dialer{Timeout: timeout}

        switch strings.ToLower(pc.Type) {
        case "socks5":
                var auth *socksAuth
                if pc.Username != "" {
                        auth = &socksAuth{User: pc.Username, Password: pc.Password}
                }
                _ = auth
                rawConn, err := dialer.Dial("tcp", proxyAddr)
                if err != nil {
                        return nil, fmt.Errorf("socks5 proxy connect failed: %w", err)
                }
                if err := p.socks5Handshake(rawConn, addr, pc.Username, pc.Password); err != nil {
                        rawConn.Close()
                        return nil, fmt.Errorf("socks5 handshake failed: %w", err)
                }
                return rawConn, nil
        default:
                rawConn, err := dialer.Dial("tcp", proxyAddr)
                if err != nil {
                        return nil, fmt.Errorf("http proxy connect failed: %w", err)
                }
                connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
                if pc.Username != "" {
                        creds := pc.Username + ":" + pc.Password
                        encoded := base64.StdEncoding.EncodeToString([]byte(creds))
                        connectReq += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", encoded)
                }
                connectReq += "\r\n"
                if _, err := rawConn.Write([]byte(connectReq)); err != nil {
                        rawConn.Close()
                        return nil, fmt.Errorf("proxy CONNECT write failed: %w", err)
                }
                buf := make([]byte, 4096)
                n, err := rawConn.Read(buf)
                if err != nil {
                        rawConn.Close()
                        return nil, fmt.Errorf("proxy CONNECT read failed: %w", err)
                }
                resp := string(buf[:n])
                if !strings.Contains(resp, "200") {
                        rawConn.Close()
                        return nil, fmt.Errorf("proxy CONNECT rejected: %s", strings.TrimSpace(resp))
                }
                return rawConn, nil
        }
}

type socksAuth struct {
        User     string
        Password string
}

func (p *SMTPProvider) socks5Handshake(conn net.Conn, targetAddr, user, pass string) error {
        hasAuth := user != ""
        var methods []byte
        if hasAuth {
                methods = []byte{0x05, 0x02, 0x00, 0x02}
        } else {
                methods = []byte{0x05, 0x01, 0x00}
        }
        if _, err := conn.Write(methods); err != nil {
                return err
        }
        resp := make([]byte, 2)
        if _, err := conn.Read(resp); err != nil {
                return err
        }
        if resp[0] != 0x05 {
                return fmt.Errorf("invalid socks5 version: %d", resp[0])
        }
        if resp[1] == 0x02 && hasAuth {
                authReq := []byte{0x01, byte(len(user))}
                authReq = append(authReq, []byte(user)...)
                authReq = append(authReq, byte(len(pass)))
                authReq = append(authReq, []byte(pass)...)
                if _, err := conn.Write(authReq); err != nil {
                        return err
                }
                authResp := make([]byte, 2)
                if _, err := conn.Read(authResp); err != nil {
                        return err
                }
                if authResp[1] != 0x00 {
                        return fmt.Errorf("socks5 auth failed")
                }
        }
        host, portStr, err := net.SplitHostPort(targetAddr)
        if err != nil {
                return err
        }
        port := 0
        fmt.Sscanf(portStr, "%d", &port)
        req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
        req = append(req, []byte(host)...)
        req = append(req, byte(port>>8), byte(port&0xff))
        if _, err := conn.Write(req); err != nil {
                return err
        }
        connResp := make([]byte, 256)
        if _, err := conn.Read(connResp); err != nil {
                return err
        }
        if connResp[1] != 0x00 {
                return fmt.Errorf("socks5 connect failed: status %d", connResp[1])
        }
        return nil
}

func (p *SMTPProvider) createConnection() (*SMTPConnection, error) {
        addr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)

        var conn net.Conn
        var err error

        useProxy := p.proxyConfig != nil && p.proxyConfig.Enabled

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
                        if useProxy {
                                rawConn, proxyErr := p.dialWithProxy(addr)
                                if proxyErr != nil {
                                        return nil, fmt.Errorf("%w: proxy: %v", ErrSMTPConnectionFailed, proxyErr)
                                }
                                conn = tls.Client(rawConn, tlsConfig)
                        } else {
                                conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
                        }
                } else {
                        if useProxy {
                                conn, err = p.dialWithProxy(addr)
                        } else {
                                conn, err = dialer.Dial("tcp", addr)
                        }
                        if err != nil {
                                return nil, fmt.Errorf("%w: %v", ErrSMTPConnectionFailed, err)
                        }
                }
        } else {
                if useProxy {
                        conn, err = p.dialWithProxy(addr)
                } else {
                        conn, err = dialer.Dial("tcp", addr)
                }
        }

        if err != nil {
                return nil, fmt.Errorf("%w: %v", ErrSMTPConnectionFailed, err)
        }

        client, err := smtp.NewClient(conn, p.config.Host)
        if err != nil {
                conn.Close()
                return nil, fmt.Errorf("%w: %v", ErrSMTPConnectionFailed, err)
        }

        // Handle STARTTLS only for non-465 ports
        if p.config.Port != 465 {
                tlsConfig := &tls.Config{
                        ServerName:         p.config.Host,
                        InsecureSkipVerify: p.config.TLSConfig != nil && p.config.TLSConfig.InsecureSkipVerify,
                }

                // Check if server supports STARTTLS
                if ok, _ := client.Extension("STARTTLS"); ok {
                        if err := client.StartTLS(tlsConfig); err != nil {
                                client.Close()
                                return nil, fmt.Errorf("STARTTLS failed: %w", err)
                        }
                } else if p.config.TLSConfig != nil && p.config.TLSConfig.Enabled {
                        // TLS is required but server doesn't support STARTTLS
                        client.Close()
                        return nil, fmt.Errorf("server does not support STARTTLS on port %d", p.config.Port)
                }
        }

        // Authenticate only AFTER TLS is established
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

        if p.log != nil {
                (*p.log).Debug(fmt.Sprintf("[SMTP-DEBUG] buildMIME: attachments=%d, to=%s", len(message.Attachments), message.To.Address))
                for i, a := range message.Attachments {
                        (*p.log).Debug(fmt.Sprintf("[SMTP-DEBUG] attachment[%d]: filename=%s, contentType=%s, contentLen=%d, filePath=%s",
                                i, a.Filename, a.ContentType, len(a.Content), a.FilePath))
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

        if attachment.FilePath != "" && len(attachment.Content) == 0 {
                f, err := os.Open(attachment.FilePath)
                if err != nil {
                        if p.log != nil {
                                (*p.log).Error(fmt.Sprintf("failed to open attachment file %s: %v", attachment.FilePath, err))
                        }
                        return part.String()
                }
                defer f.Close()

                encoder := base64.NewEncoder(base64.StdEncoding, &lineBreaker{w: &part})
                buf := make([]byte, 4096)
                for {
                        n, readErr := f.Read(buf)
                        if n > 0 {
                                encoder.Write(buf[:n])
                        }
                        if readErr != nil {
                                break
                        }
                }
                encoder.Close()
                part.WriteString("\r\n")
        } else {
                encoded := base64.StdEncoding.EncodeToString(attachment.Content)
                for i := 0; i < len(encoded); i += 76 {
                        end := i + 76
                        if end > len(encoded) {
                                end = len(encoded)
                        }
                        part.WriteString(encoded[i:end])
                        part.WriteString("\r\n")
                }
        }

        return part.String()
}

type lineBreaker struct {
        w   *strings.Builder
        col int
}

func (lb *lineBreaker) Write(p []byte) (int, error) {
        total := 0
        for len(p) > 0 {
                remaining := 76 - lb.col
                if remaining <= 0 {
                        lb.w.WriteString("\r\n")
                        lb.col = 0
                        remaining = 76
                }
                chunk := remaining
                if chunk > len(p) {
                        chunk = len(p)
                }
                lb.w.Write(p[:chunk])
                lb.col += chunk
                total += chunk
                p = p[chunk:]
        }
        return total, nil
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
