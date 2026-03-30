package main

import (
        "context"
        "fmt"
        "strings"
        "sync"
        "time"

        "email-campaign-system/internal/core/provider"
        "email-campaign-system/internal/core/sender"
        "email-campaign-system/internal/models"
        "email-campaign-system/internal/storage/repository"
        "email-campaign-system/pkg/logger"
)

// ─────────────────────────────────────────────────────────
// sender.TemplateManager adapter
// ─────────────────────────────────────────────────────────

type senderTemplateAdapter struct {
        repo       repository.TemplateRepository
        log        logger.Logger
        mu         sync.Mutex
        idx        int
        cachedList []*repository.Template
        lastLoad   time.Time
}

func newSenderTemplateAdapter(repo repository.TemplateRepository, log logger.Logger) sender.TemplateManager {
        return &senderTemplateAdapter{repo: repo, log: log}
}

func (a *senderTemplateAdapter) GetNextTemplate(ctx context.Context, templateIDs []string) (*models.Template, error) {
        a.mu.Lock()
        defer a.mu.Unlock()

        if a.cachedList == nil || time.Since(a.lastLoad) > 60*time.Second {
                templates, _, err := a.repo.List(ctx, nil)
                if err != nil || len(templates) == 0 {
                        return nil, fmt.Errorf("no templates available")
                }
                a.cachedList = templates
                a.lastLoad = time.Now()
        }

        if len(a.cachedList) == 0 {
                return nil, fmt.Errorf("no templates available")
        }

        candidates := a.cachedList
        if len(templateIDs) > 0 {
                idSet := make(map[string]bool, len(templateIDs))
                for _, id := range templateIDs {
                        idSet[id] = true
                }
                var filtered []*repository.Template
                for _, t := range a.cachedList {
                        if idSet[t.ID] {
                                filtered = append(filtered, t)
                        }
                }
                if len(filtered) == 0 {
                        return nil, fmt.Errorf("no templates matching campaign IDs: %v", templateIDs)
                }
                candidates = filtered
        }

        t := candidates[a.idx%len(candidates)]
        a.idx++
        return repoToModel(t), nil
}

func repoToModel(t *repository.Template) *models.Template {
        var subjects []string
        if t.Subject != "" {
                subjects = []string{t.Subject}
        }
        status := models.TemplateStatusInactive
        if t.IsActive {
                status = models.TemplateStatusActive
        }
        return &models.Template{
                ID:               t.ID,
                Name:             t.Name,
                Description:      t.Description,
                HTMLContent:      t.HtmlContent,
                PlainTextContent: t.TextContent,
                Subjects:         subjects,
                Tags:             t.Tags,
                Status:           status,
                SpamScore:        t.SpamScore,
                Metadata:         t.Metadata,
                CustomVariables:  t.CustomVariables,
                CreatedAt:        t.CreatedAt,
                UpdatedAt:        t.UpdatedAt,
        }
}

func (a *senderTemplateAdapter) Render(ctx context.Context, tmpl *models.Template, data map[string]interface{}) (string, error) {
        if body, ok := data["body"].(string); ok && body != "" {
                return body, nil
        }
        content := tmpl.HTMLContent
        for k, v := range data {
                if str, ok := v.(string); ok {
                        content = strings.ReplaceAll(content, "{"+k+"}", str)
                        content = strings.ReplaceAll(content, "{{"+k+"}}", str)
                        content = strings.ReplaceAll(content, "{"+strings.ToLower(k)+"}", str)
                        content = strings.ReplaceAll(content, "{{"+strings.ToLower(k)+"}}", str)
                }
        }
        if vars, ok := data["variables"].(map[string]string); ok {
                for k, v := range vars {
                        content = strings.ReplaceAll(content, "{"+k+"}", v)
                        content = strings.ReplaceAll(content, "{{"+k+"}}", v)
                }
        }
        return content, nil
}

// ─────────────────────────────────────────────────────────
// sender.DeliverabilityManager adapter
// ─────────────────────────────────────────────────────────

type simpleDeliverabilityManager struct{}

func (d *simpleDeliverabilityManager) BuildMessage(_ context.Context, req *sender.MessageRequest) (*sender.EmailMessage, error) {
        return &sender.EmailMessage{
                From:        req.From,
                FromName:    req.FromName,
                To:          req.To,
                ToName:      req.ToName,
                Subject:     req.Subject,
                Body:        req.HTMLBody,
                Attachments: req.Attachments,
        }, nil
}

// ─────────────────────────────────────────────────────────
// sender.ProviderFactory adapter
// ─────────────────────────────────────────────────────────

type providerFactoryAdapter struct {
        factory *provider.ProviderFactory
        log     logger.Logger
}

func newProviderFactoryAdapter(f *provider.ProviderFactory, log logger.Logger) sender.ProviderFactory {
        return &providerFactoryAdapter{factory: f, log: log}
}

func (p *providerFactoryAdapter) GetProvider(ctx context.Context, acc *models.Account) (sender.EmailProvider, error) {
        if acc.Credentials == nil || acc.SMTPConfig == nil {
                return nil, fmt.Errorf("account %s missing credentials or SMTP config", acc.Email)
        }
        cfg := &provider.ProviderConfig{
                Type:     provider.ProviderType(acc.Provider),
                Username: acc.Email,
                Password: acc.Credentials.Password,
                Host:     acc.SMTPConfig.Host,
                Port:     acc.SMTPConfig.Port,
                TLSConfig: &provider.TLSConfig{
                        Enabled:    acc.SMTPConfig.UseTLS || acc.SMTPConfig.UseSSL,
                        ServerName: acc.SMTPConfig.Host,
                },
                TimeoutConfig: &provider.TimeoutConfig{
                        Connect: 30 * time.Second,
                        Send:    60 * time.Second,
                },
                RetryConfig: &provider.RetryConfig{
                        MaxRetries:   3,
                        InitialDelay: 1 * time.Second,
                },
        }
        prov, err := provider.NewProvider(cfg, p.log)
        if err != nil {
                return nil, fmt.Errorf("failed to create provider for %s: %w", acc.Email, err)
        }
        return &smtpEmailProvider{provider: prov, name: string(acc.Provider)}, nil
}

// ─────────────────────────────────────────────────────────
// sender.EmailProvider wrapper around provider.Provider
// ─────────────────────────────────────────────────────────

type smtpEmailProvider struct {
        provider provider.Provider
        name     string
}

func (s *smtpEmailProvider) Send(ctx context.Context, msg *sender.EmailMessage) (string, error) {
        if msg.ProxyHost != "" && msg.ProxyPort > 0 {
                if smtpProv, ok := s.provider.(interface{ SetProxy(*provider.ProxyConfig) }); ok {
                        smtpProv.SetProxy(&provider.ProxyConfig{
                                Enabled:  true,
                                Host:     msg.ProxyHost,
                                Port:     msg.ProxyPort,
                                Type:     msg.ProxyType,
                        })
                        defer func() {
                                smtpProv.SetProxy(nil)
                        }()
                }
        }

        var emailAttachments []models.EmailAttachment
        fmt.Printf("[PROVIDER-DEBUG] Send called with %d attachments for %s\n", len(msg.Attachments), msg.To)
        for _, att := range msg.Attachments {
                if att != nil {
                        fmt.Printf("[PROVIDER-DEBUG] mapping attachment: filename=%s, contentType=%s, dataLen=%d\n",
                                att.Filename, att.ContentType, len(att.Data))
                        emailAttachments = append(emailAttachments, models.EmailAttachment{
                                ID:          att.ID,
                                Filename:    att.Filename,
                                ContentType: att.ContentType,
                                Content:     att.Data,
                                Size:        att.Size,
                        })
                }
        }
        fmt.Printf("[PROVIDER-DEBUG] emailAttachments count after mapping: %d\n", len(emailAttachments))

        email := &models.Email{
                From:        models.EmailAddress{Address: msg.From, Name: msg.FromName},
                To:          models.EmailAddress{Address: msg.To, Name: msg.ToName},
                Subject:     msg.Subject,
                HTMLBody:    msg.Body,
                ReplyTo:     &models.EmailAddress{Address: msg.From, Name: msg.FromName},
                Attachments: emailAttachments,
        }
        messageID, err := s.provider.Send(ctx, email)
        if err != nil {
                return "", err
        }
        return messageID, nil
}

func (s *smtpEmailProvider) Name() string { return s.name }

// ─────────────────────────────────────────────────────────
// sender.RateLimiter — no-op
// ─────────────────────────────────────────────────────────

type noopRateLimiter struct{}

func (n *noopRateLimiter) Wait(_ context.Context) error { return nil }

// Compile-time interface checks
var _ sender.TemplateManager       = (*senderTemplateAdapter)(nil)
var _ sender.DeliverabilityManager = (*simpleDeliverabilityManager)(nil)
var _ sender.ProviderFactory       = (*providerFactoryAdapter)(nil)
var _ sender.EmailProvider         = (*smtpEmailProvider)(nil)
var _ sender.RateLimiter           = (*noopRateLimiter)(nil)
