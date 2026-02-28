package main

import (
	"context"
	"fmt"
	"strings"
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
	repo repository.TemplateRepository
	log  logger.Logger
}

func newSenderTemplateAdapter(repo repository.TemplateRepository, log logger.Logger) sender.TemplateManager {
	return &senderTemplateAdapter{repo: repo, log: log}
}

// GetNextTemplate returns *models.Template — matches the sender.TemplateManager interface
func (a *senderTemplateAdapter) GetNextTemplate(ctx context.Context) (*models.Template, error) {
	templates, _, err := a.repo.List(ctx, nil)
	if err != nil || len(templates) == 0 {
		return nil, fmt.Errorf("no templates available")
	}
	t := templates[0]
	tmpl := &models.Template{
		ID:          t.ID,
		HTMLContent: t.HtmlContent,
	}
	return tmpl, nil
}

func (a *senderTemplateAdapter) Render(ctx context.Context, tmpl models.Template, data map[string]interface{}) (string, error) {
	content := tmpl.HTMLContent
	for k, v := range data {
		content = strings.ReplaceAll(content, "{{"+k+"}}", fmt.Sprintf("%v", v))
		content = strings.ReplaceAll(content, "{{"+strings.ToLower(k)+"}}", fmt.Sprintf("%v", v))
	}
	return content, nil
}

// ─────────────────────────────────────────────────────────
// sender.DeliverabilityManager adapter
// ─────────────────────────────────────────────────────────

type simpleDeliverabilityManager struct{}

// BuildMessage takes *sender.MessageRequest and returns *sender.EmailMessage
func (d *simpleDeliverabilityManager) BuildMessage(_ context.Context, req *sender.MessageRequest) (*sender.EmailMessage, error) {
	return &sender.EmailMessage{
		From:    req.From,
		To:      req.To,
		Subject: req.Subject,
		Body:    req.HTMLBody,
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

// GetProvider takes *models.Account — matches the sender.ProviderFactory interface
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

// Send takes *sender.EmailMessage — matches the sender.EmailProvider interface
func (s *smtpEmailProvider) Send(ctx context.Context, msg *sender.EmailMessage) (string, error) {
	email := models.Email{
		From:     models.EmailAddress{Address: msg.From},
		To:       models.EmailAddress{Address: msg.To},
		Subject:  msg.Subject,
		HTMLBody: msg.Body,
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
var _ sender.TemplateManager      = (*senderTemplateAdapter)(nil)
var _ sender.DeliverabilityManager = (*simpleDeliverabilityManager)(nil)
var _ sender.ProviderFactory      = (*providerFactoryAdapter)(nil)
var _ sender.EmailProvider        = (*smtpEmailProvider)(nil)
var _ sender.RateLimiter          = (*noopRateLimiter)(nil)
