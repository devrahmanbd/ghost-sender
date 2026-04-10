# Email Campaign System

## Overview
Go-based email campaign management system with a web frontend. Manages email campaigns, accounts, templates, attachments, proxies, and recipient lists.

## Architecture
- **Backend**: Go (1.25), located at `./backend/`
- **Frontend**: Server-rendered HTML templates with Tailwind CSS (served by Go backend)
- **Database**: PostgreSQL 16 (Replit-managed, accessed via `DATABASE_URL`)
- **Cache**: Redis (optional, falls back to in-memory)

## Key Configuration
- **Config file**: `./backend/configs/config.yaml`
- **Server port**: 5000 (set via `SERVER_PORT` env var, overriding default 8080)
- **Admin credentials**: admin / admin123
- **JWT_SECRET**: Set in shared env vars
- **ENCRYPTION_KEY**: Hardcoded in config.yaml

## Project Structure
```
./backend/
├── cmd/server/main.go          # Entry point
├── configs/config.yaml         # App configuration
├── internal/
│   ├── api/                    # HTTP handlers, middleware, router, server
│   ├── config/                 # Config loading and validation
│   ├── core/                   # Business logic
│   │   ├── account/            # Email account management
│   │   ├── attachment/         # Attachment handling (NativeConverter, manager)
│   │   ├── campaign/           # Campaign execution
│   │   ├── provider/           # SMTP providers
│   │   ├── proxy/              # Proxy management
│   │   ├── sender/             # Send engine
│   │   └── template/           # Template management
│   ├── storage/repository/     # Database repositories
│   └── pkg/                    # Shared utilities
├── web/                        # Frontend templates and static assets
└── migrations/                 # DB migration files (000001-000017)
```

## Implemented Fixes
1. **Hot-reload gap**: `RefreshTemplates()` triggers on both templates and attachments category uploads
2. **Job-level attachment isolation**: Deep-copied `AttachmentTemplateIDs` per job
3. **Attachment filtering**: `Prepare()` filters by templateID
4. **NativeConverter**: WYSIWYG HTML-to-PDF/image conversion using wkhtmltopdf/wkhtmltoimage (lightweight Qt WebKit, ~30MB per process, no browser daemon). Supports PDF, JPG, PNG, WebP formats. Concurrency-safe via unique temp files.
5. **Streaming attachments**: Base64 streaming via lineBreaker writer
6. **Worker count**: Defaults to `runtime.NumCPU()`
7. **Template adapter**: Round-robin rotation with 60s cache TTL, campaign-aware filtering by templateIDs
8. **Campaign-aware template selection**: `GetNextTemplate(ctx, templateIDs)` filters to campaign-specific templates; returns error if no match
9. **Concurrent campaign safety**: `SendJob.Campaign` snapshot deep-copied at queue time; `processJob` uses job-scoped campaign data, not shared `e.campaign`
10. **Cache key correctness**: `buildCacheKey` sorts personalization keys, uses null-byte delimiters, includes CampaignID for cross-campaign isolation
11. **Pre-compiled SVG template**: Cached `text/template` for SVG generation via `sync.Once`
12. **AttachmentTemplateIDs source fix**: `QueueEmail()` now extracts attachment template IDs from `campaign.Config.Metadata["attachment_template_ids"]` (handles both `[]string` and `[]interface{}` JSON types) instead of incorrectly using `campaign.TemplateIDs` (email body templates)
13. **GetNextTemplate fix**: Email template selection in `processJob` now uses `job.Campaign.TemplateIDs` (email body templates) instead of `job.AttachmentTemplateIDs`, with nil-safe guard for campaigns
14. **Personalization fix**: `personalizeHTML()` now replaces both `{key}` (single-brace, used by templates) and `{{key}}` (double-brace) formats
15. **Silent error masking removed**: `doGenerate()` now returns errors on conversion failure instead of silently falling back to raw HTML bytes; debug logging added throughout format selection and conversion pipeline
16. **Format moved to attachment template**: Format selection (PDF/JPEG/PNG/WebP) is now on the Attachment Template page, not the campaign creation form. Format is stored in `.meta.json` alongside the HTML template and loaded by `LoadTemplates()`.
17. **Security hardening**: Removed `--enable-local-file-access` flag from wkhtmltopdf; JavaScript disabled in conversion subprocess
18. **Attachment filename support**: Optional filename field on attachment templates; if set, attachment is named `filename.ext` instead of `templateID.ext`. Stored in `.meta.json`.
19. **Variable fix in attachments**: `Prepare()` now properly extracts variables from the nested `variables` map in personalizedData, and directly reads recipient fields (First_Name, Last_Name, Email, etc.) via type assertion on `*models.Recipient`
20. **Image quality upgrade**: wkhtmltoimage default width increased to 1920px with 2x zoom for retina-quality output; JPEG quality 95; PDF uses 300 DPI with `--image-quality 100`

## Database
- 41 tables imported from SQL backup
- Schema uses snake_case column naming throughout
- Key tables: accounts, campaigns, templates, proxies, recipients, email_logs, attachments

## SMTP Provider Fixes (factory.go)
- **STARTTLS capability check**: `createConnection()` now checks `client.Extension("STARTTLS")` before calling `StartTLS()` — fails fast if server doesn't advertise it
- **AUTH PLAIN + LOGIN fallback**: `factoryConnectionPool.authenticate()` inspects server's advertised AUTH mechanisms, tries PLAIN first, falls back to AUTH LOGIN (required for Office 365)
- **`smtpLoginAuth`**: Custom `smtp.Auth` implementation for AUTH LOGIN challenge-response (Office 365 / Exchange servers)
- **Proxy send path fixed**: `sendMessageViaProxy()` also checks `client.Extension("STARTTLS")` and uses `p.pool.authenticate()` for consistent auth behavior

## Office 365 Provider
- **Provider type**: `ProviderTypeOffice365 = "office365"` added to interface.go constants
- **Default config**: `smtp.office365.com:587`, TLS 1.2+, STARTTLS required, 10k/day, 1k/hr limits
- **`office365.go`**: Full provider wrapper with rate limiting, retry logic, error categorization, health tracking
- **UI**: "Microsoft — Office 365 (smtp.office365.com:587)" option added to account creation provider dropdown

## Other Fixes
- **Log column truncation**: Migration `000018_increase_logs_column_sizes.up.sql` increases 11 log table columns from varchar(100) → varchar(255)
- **Proxy list UI**: Removed Success/Fail column; added "Remove Unhealthy Proxies" button
- **`POST /api/v1/proxies/bulk/delete-unhealthy`**: Endpoint implemented in proxy handler and repository

## Workflow
- **Start application**: `cd ./backend && go build -o server ./cmd/server/ && ./server`
- Listens on port 5000 (webview)
