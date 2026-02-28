CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TYPE campaign_status AS ENUM (
    'created',
    'scheduled',
    'running',
    'paused',
    'completed',
    'failed',
    'cancelled'
);

CREATE TYPE account_status AS ENUM (
    'active',
    'suspended',
    'failed',
    'cooldown',
    'disabled'
);

CREATE TYPE account_provider AS ENUM (
    'gmail',
    'smtp',
    'yahoo',
    'outlook',
    'hotmail',
    'icloud',
    'workspace'
);

CREATE TYPE recipient_status AS ENUM (
    'pending',
    'sent',
    'failed',
    'bounced',
    'skipped',
    'unsubscribed'
);

CREATE TYPE log_level AS ENUM (
    'debug',
    'info',
    'warning',
    'error',
    'critical'
);

CREATE TYPE rotation_strategy AS ENUM (
    'round_robin',
    'random',
    'weighted',
    'health_based',
    'least_used',
    'time_based'
);

CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status campaign_status NOT NULL DEFAULT 'created',
    session_id VARCHAR(100),
    
    total_recipients INTEGER DEFAULT 0,
    sent_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    pending_count INTEGER DEFAULT 0,
    
    progress_percentage DECIMAL(5,2) DEFAULT 0.00,
    
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    scheduled_at TIMESTAMP,
    paused_at TIMESTAMP,
    resumed_at TIMESTAMP,
    
    worker_count INTEGER DEFAULT 1,
    batch_size INTEGER DEFAULT 100,
    rate_limit INTEGER DEFAULT 10,
    
    subject_template TEXT,
    sender_name VARCHAR(255),
    reply_to VARCHAR(255),
    
    use_templates BOOLEAN DEFAULT true,
    use_attachments BOOLEAN DEFAULT false,
    use_personalization BOOLEAN DEFAULT true,
    
    rotation_strategy rotation_strategy DEFAULT 'round_robin',
    
    error_message TEXT,
    last_error_at TIMESTAMP,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_by VARCHAR(100),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255),
    provider account_provider NOT NULL,
    status account_status NOT NULL DEFAULT 'active',
    
    smtp_host VARCHAR(255),
    smtp_port INTEGER,
    smtp_username VARCHAR(255),
    smtp_password TEXT,
    
    oauth_token TEXT,
    oauth_refresh_token TEXT,
    oauth_token_expiry TIMESTAMP,
    oauth_client_id TEXT,
    oauth_client_secret TEXT,
    
    credentials_encrypted TEXT,
    encryption_key_id VARCHAR(100),
    
    daily_limit INTEGER DEFAULT 500,
    rotation_limit INTEGER DEFAULT 50,
    hourly_limit INTEGER DEFAULT 50,
    
    sent_today INTEGER DEFAULT 0,
    sent_this_hour INTEGER DEFAULT 0,
    total_sent INTEGER DEFAULT 0,
    total_failed INTEGER DEFAULT 0,
    
    consecutive_failures INTEGER DEFAULT 0,
    last_failure_at TIMESTAMP,
    last_success_at TIMESTAMP,
    last_used_at TIMESTAMP,
    
    health_score DECIMAL(3,2) DEFAULT 1.00,
    spam_score DECIMAL(3,2) DEFAULT 0.00,
    
    suspended_at TIMESTAMP,
    suspended_until TIMESTAMP,
    suspension_reason TEXT,
    
    cooldown_until TIMESTAMP,
    
    weight INTEGER DEFAULT 1,
    priority INTEGER DEFAULT 0,
    
    use_proxy BOOLEAN DEFAULT false,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    
    subject VARCHAR(500),
    content TEXT NOT NULL,
    plain_text TEXT,
    
    template_type VARCHAR(50) DEFAULT 'html',
    category VARCHAR(100),
    
    variables JSONB DEFAULT '[]',
    
    spam_score DECIMAL(5,2) DEFAULT 0.00,
    spam_check_result JSONB,
    spam_checked_at TIMESTAMP,
    
    version INTEGER DEFAULT 1,
    parent_template_id UUID REFERENCES templates(id) ON DELETE SET NULL,
    
    is_active BOOLEAN DEFAULT true,
    is_default BOOLEAN DEFAULT false,
    
    usage_count INTEGER DEFAULT 0,
    last_used_at TIMESTAMP,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_by VARCHAR(100),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);
-- Fix smtp_host NULL values
UPDATE accounts SET smtp_host = '' WHERE smtp_host IS NULL;
ALTER TABLE accounts ALTER COLUMN smtp_host SET DEFAULT '';

CREATE TABLE recipients (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    
    status recipient_status NOT NULL DEFAULT 'pending',
    
    custom_fields JSONB DEFAULT '{}',
    
    sent_at TIMESTAMP,
    failed_at TIMESTAMP,
    bounced_at TIMESTAMP,
    
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    template_id UUID REFERENCES templates(id) ON DELETE SET NULL,
    
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    
    error_message TEXT,
    error_code VARCHAR(50),
    
    message_id VARCHAR(255),
    
    opened_at TIMESTAMP,
    clicked_at TIMESTAMP,
    unsubscribed_at TIMESTAMP,
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(campaign_id, email)
);

CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID REFERENCES campaigns(id) ON DELETE CASCADE,
    template_id UUID REFERENCES templates(id) ON DELETE SET NULL,
    
    filename VARCHAR(255) NOT NULL,
    original_filename VARCHAR(255),
    
    file_path TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    file_hash VARCHAR(64),
    
    mime_type VARCHAR(100),
    file_format VARCHAR(50),
    
    is_inline BOOLEAN DEFAULT false,
    content_id VARCHAR(255),
    
    converted_from VARCHAR(50),
    conversion_backend VARCHAR(50),
    
    cache_key VARCHAR(255),
    cached_at TIMESTAMP,
    cache_hit_count INTEGER DEFAULT 0,
    
    usage_count INTEGER DEFAULT 0,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

CREATE TABLE email_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    recipient_id UUID REFERENCES recipients(id) ON DELETE SET NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    
    recipient_email VARCHAR(255) NOT NULL,
    sender_email VARCHAR(255),
    
    subject VARCHAR(500),
    
    status VARCHAR(50) NOT NULL,
    
    message_id VARCHAR(255),
    
    sent_at TIMESTAMP,
    delivered_at TIMESTAMP,
    failed_at TIMESTAMP,
    bounced_at TIMESTAMP,
    
    error_message TEXT,
    error_code VARCHAR(50),
    error_type VARCHAR(50),
    
    smtp_response TEXT,
    http_status_code INTEGER,
    
    retry_attempt INTEGER DEFAULT 0,
    
    send_duration_ms INTEGER,
    
    template_used UUID REFERENCES templates(id) ON DELETE SET NULL,
    
    personalization_data JSONB,
    
    has_attachments BOOLEAN DEFAULT false,
    attachment_count INTEGER DEFAULT 0,
    
    proxy_used VARCHAR(255),
    
    log_level log_level DEFAULT 'info',
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    indexed_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE campaign_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    snapshot_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    total_recipients INTEGER DEFAULT 0,
    sent_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    pending_count INTEGER DEFAULT 0,
    bounced_count INTEGER DEFAULT 0,
    
    success_rate DECIMAL(5,2) DEFAULT 0.00,
    failure_rate DECIMAL(5,2) DEFAULT 0.00,
    
    emails_per_minute DECIMAL(10,2) DEFAULT 0.00,
    emails_per_hour DECIMAL(10,2) DEFAULT 0.00,
    
    avg_send_time_ms DECIMAL(10,2) DEFAULT 0.00,
    
    active_accounts INTEGER DEFAULT 0,
    suspended_accounts INTEGER DEFAULT 0,
    
    elapsed_time_seconds INTEGER DEFAULT 0,
    estimated_time_remaining_seconds INTEGER,
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE account_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    hour INTEGER,
    
    sent_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    bounced_count INTEGER DEFAULT 0,
    
    success_rate DECIMAL(5,2) DEFAULT 0.00,
    
    avg_response_time_ms DECIMAL(10,2) DEFAULT 0.00,
    
    consecutive_failures INTEGER DEFAULT 0,
    
    health_score DECIMAL(3,2) DEFAULT 1.00,
    spam_score DECIMAL(3,2) DEFAULT 0.00,
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(account_id, date, hour)
);

CREATE TABLE system_config (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    config_key VARCHAR(255) NOT NULL UNIQUE,
    config_value TEXT NOT NULL,
    
    config_type VARCHAR(50) DEFAULT 'string',
    
    category VARCHAR(100),
    
    description TEXT,
    
    is_encrypted BOOLEAN DEFAULT false,
    is_sensitive BOOLEAN DEFAULT false,
    
    validation_rule TEXT,
    
    default_value TEXT,
    
    is_active BOOLEAN DEFAULT true,
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id VARCHAR(100) NOT NULL UNIQUE,
    session_name VARCHAR(255),
    
    campaign_ids UUID[] DEFAULT '{}',
    
    status VARCHAR(50) DEFAULT 'active',
    
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMP,
    expires_at TIMESTAMP,
    
    total_campaigns INTEGER DEFAULT 0,
    active_campaigns INTEGER DEFAULT 0,
    
    state_data JSONB DEFAULT '{}',
    
    created_by VARCHAR(100),
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE rotation_tracking (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    rotation_type VARCHAR(50) NOT NULL,
    
    current_index INTEGER DEFAULT 0,
    total_items INTEGER DEFAULT 0,
    
    rotation_count INTEGER DEFAULT 0,
    
    strategy rotation_strategy DEFAULT 'round_robin',
    
    items JSONB DEFAULT '[]',
    
    usage_stats JSONB DEFAULT '{}',
    
    last_rotated_at TIMESTAMP,
    
    settings JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(campaign_id, rotation_type)
);

CREATE TABLE unsubscribe_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    email VARCHAR(255) NOT NULL,
    campaign_id UUID REFERENCES campaigns(id) ON DELETE SET NULL,
    
    unsubscribe_token VARCHAR(255) NOT NULL UNIQUE,
    
    unsubscribed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    ip_address INET,
    user_agent TEXT,
    
    reason TEXT,
    
    verified BOOLEAN DEFAULT false,
    verified_at TIMESTAMP,
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    key_name VARCHAR(255) NOT NULL,
    api_key VARCHAR(255) NOT NULL UNIQUE,
    api_secret VARCHAR(255),
    
    key_hash VARCHAR(64) NOT NULL,
    
    permissions JSONB DEFAULT '[]',
    
    rate_limit INTEGER DEFAULT 100,
    rate_limit_window INTEGER DEFAULT 60,
    
    is_active BOOLEAN DEFAULT true,
    
    last_used_at TIMESTAMP,
    usage_count INTEGER DEFAULT 0,
    
    expires_at TIMESTAMP,
    
    created_by VARCHAR(100),
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    entity_type VARCHAR(100) NOT NULL,
    entity_id UUID,
    
    action VARCHAR(100) NOT NULL,
    
    actor VARCHAR(255),
    actor_ip INET,
    
    changes JSONB,
    
    old_values JSONB,
    new_values JSONB,
    
    success BOOLEAN DEFAULT true,
    error_message TEXT,
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_campaigns_status ON campaigns(status);
CREATE INDEX idx_campaigns_session_id ON campaigns(session_id);
CREATE INDEX idx_campaigns_created_at ON campaigns(created_at DESC);
CREATE INDEX idx_campaigns_scheduled_at ON campaigns(scheduled_at) WHERE scheduled_at IS NOT NULL;

CREATE INDEX idx_accounts_email ON accounts(email);
CREATE INDEX idx_accounts_provider ON accounts(provider);
CREATE INDEX idx_accounts_status ON accounts(status);
CREATE INDEX idx_accounts_health_score ON accounts(health_score);
CREATE INDEX idx_accounts_last_used_at ON accounts(last_used_at);

CREATE INDEX idx_templates_name ON templates(name);
CREATE INDEX idx_templates_is_active ON templates(is_active);
CREATE INDEX idx_templates_spam_score ON templates(spam_score);

CREATE INDEX idx_recipients_campaign_id ON recipients(campaign_id);
CREATE INDEX idx_recipients_email ON recipients(email);
CREATE INDEX idx_recipients_status ON recipients(status);
CREATE INDEX idx_recipients_account_id ON recipients(account_id);

CREATE INDEX idx_attachments_campaign_id ON attachments(campaign_id);
CREATE INDEX idx_attachments_file_hash ON attachments(file_hash);
CREATE INDEX idx_attachments_cache_key ON attachments(cache_key);

CREATE INDEX idx_email_logs_campaign_id ON email_logs(campaign_id);
CREATE INDEX idx_email_logs_recipient_id ON email_logs(recipient_id);
CREATE INDEX idx_email_logs_account_id ON email_logs(account_id);
CREATE INDEX idx_email_logs_status ON email_logs(status);
CREATE INDEX idx_email_logs_created_at ON email_logs(created_at DESC);
CREATE INDEX idx_email_logs_recipient_email ON email_logs(recipient_email);

CREATE INDEX idx_campaign_stats_campaign_id ON campaign_stats(campaign_id);
CREATE INDEX idx_campaign_stats_snapshot_at ON campaign_stats(snapshot_at DESC);

CREATE INDEX idx_account_stats_account_id ON account_stats(account_id);
CREATE INDEX idx_account_stats_date ON account_stats(date DESC);

CREATE INDEX idx_system_config_config_key ON system_config(config_key);
CREATE INDEX idx_system_config_category ON system_config(category);

CREATE INDEX idx_sessions_session_id ON sessions(session_id);
CREATE INDEX idx_sessions_status ON sessions(status);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

CREATE INDEX idx_rotation_tracking_campaign_id ON rotation_tracking(campaign_id);
CREATE INDEX idx_rotation_tracking_rotation_type ON rotation_tracking(rotation_type);

CREATE INDEX idx_unsubscribe_requests_email ON unsubscribe_requests(email);
CREATE INDEX idx_unsubscribe_requests_token ON unsubscribe_requests(unsubscribe_token);

CREATE INDEX idx_api_keys_api_key ON api_keys(api_key);
CREATE INDEX idx_api_keys_is_active ON api_keys(is_active);

CREATE INDEX idx_audit_logs_entity_type ON audit_logs(entity_type);
CREATE INDEX idx_audit_logs_entity_id ON audit_logs(entity_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_campaigns_updated_at BEFORE UPDATE ON campaigns
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_accounts_updated_at BEFORE UPDATE ON accounts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_templates_updated_at BEFORE UPDATE ON templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_recipients_updated_at BEFORE UPDATE ON recipients
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_attachments_updated_at BEFORE UPDATE ON attachments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_account_stats_updated_at BEFORE UPDATE ON account_stats
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_system_config_updated_at BEFORE UPDATE ON system_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_sessions_updated_at BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_rotation_tracking_updated_at BEFORE UPDATE ON rotation_tracking
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_api_keys_updated_at BEFORE UPDATE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

INSERT INTO system_config (config_key, config_value, config_type, category, description) VALUES
('system.version', '1.0.0', 'string', 'system', 'System version'),
('system.timezone', 'UTC', 'string', 'system', 'Default timezone'),
('email.max_workers', '4', 'integer', 'email', 'Maximum concurrent workers'),
('email.batch_size', '100', 'integer', 'email', 'Default batch size'),
('email.rate_limit', '10', 'integer', 'email', 'Default rate limit per second'),
('account.daily_limit', '500', 'integer', 'account', 'Default daily sending limit'),
('account.rotation_limit', '50', 'integer', 'account', 'Default rotation limit'),
('account.suspension_threshold', '5', 'integer', 'account', 'Consecutive failures before suspension'),
('template.max_spam_score', '5.0', 'decimal', 'template', 'Maximum acceptable spam score'),
('cleanup.retention_days', '30', 'integer', 'cleanup', 'Log retention period in days');
-- Fix all account table columns, NULL values, and add missing columns

-- 1. Add any missing columns
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS smtp_use_tls BOOLEAN DEFAULT TRUE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS smtp_use_ssl BOOLEAN DEFAULT FALSE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS smtp_username VARCHAR(255);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS oauth_token TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS oauth_refresh_token TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS oauth_expiry TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS encrypted_password TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_success_at TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_failure_at TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS total_failed INTEGER DEFAULT 0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS success_rate DECIMAL(5,2) DEFAULT 100.0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS cooldown_until TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS is_suspended BOOLEAN DEFAULT FALSE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS daily_sent INTEGER DEFAULT 0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS rotation_sent INTEGER DEFAULT 0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_reset TIMESTAMP DEFAULT NOW();
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_health_check TIMESTAMP;

-- 2. Fix TEXT/VARCHAR NULLs → empty strings
UPDATE accounts SET smtp_host = '' WHERE smtp_host IS NULL;
UPDATE accounts SET smtp_username = '' WHERE smtp_username IS NULL;
UPDATE accounts SET oauth_token = '' WHERE oauth_token IS NULL;
UPDATE accounts SET oauth_refresh_token = '' WHERE oauth_refresh_token IS NULL;
UPDATE accounts SET encrypted_password = '' WHERE encrypted_password IS NULL;
UPDATE accounts SET suspension_reason = '' WHERE suspension_reason IS NULL;

-- 3. Fix INTEGER NULLs → defaults
UPDATE accounts SET smtp_port = 587 WHERE smtp_port IS NULL;
UPDATE accounts SET daily_limit = 500 WHERE daily_limit IS NULL;
UPDATE accounts SET rotation_limit = 50 WHERE rotation_limit IS NULL;
UPDATE accounts SET hourly_limit = 50 WHERE hourly_limit IS NULL;
UPDATE accounts SET sent_today = 0 WHERE sent_today IS NULL;
UPDATE accounts SET sent_this_hour = 0 WHERE sent_this_hour IS NULL;
UPDATE accounts SET total_sent = 0 WHERE total_sent IS NULL;
UPDATE accounts SET total_failed = 0 WHERE total_failed IS NULL;
UPDATE accounts SET consecutive_failures = 0 WHERE consecutive_failures IS NULL;
UPDATE accounts SET daily_sent = 0 WHERE daily_sent IS NULL;
UPDATE accounts SET rotation_sent = 0 WHERE rotation_sent IS NULL;
UPDATE accounts SET weight = 1 WHERE weight IS NULL;
UPDATE accounts SET priority = 0 WHERE priority IS NULL;

-- 4. Set DEFAULT values to prevent future NULLs
ALTER TABLE accounts 
  ALTER COLUMN smtp_host SET DEFAULT '',
  ALTER COLUMN smtp_username SET DEFAULT '',
  ALTER COLUMN oauth_token SET DEFAULT '',
  ALTER COLUMN oauth_refresh_token SET DEFAULT '',
  ALTER COLUMN encrypted_password SET DEFAULT '',
  ALTER COLUMN suspension_reason SET DEFAULT '',
  
  ALTER COLUMN smtp_port SET DEFAULT 587,
  ALTER COLUMN daily_limit SET DEFAULT 500,
  ALTER COLUMN rotation_limit SET DEFAULT 50,
  ALTER COLUMN hourly_limit SET DEFAULT 50,
  ALTER COLUMN sent_today SET DEFAULT 0,
  ALTER COLUMN sent_this_hour SET DEFAULT 0,
  ALTER COLUMN total_sent SET DEFAULT 0,
  ALTER COLUMN total_failed SET DEFAULT 0,
  ALTER COLUMN consecutive_failures SET DEFAULT 0,
  ALTER COLUMN daily_sent SET DEFAULT 0,
  ALTER COLUMN rotation_sent SET DEFAULT 0,
  ALTER COLUMN weight SET DEFAULT 1,
  ALTER COLUMN priority SET DEFAULT 0;

-- 5. Add indexes
CREATE INDEX IF NOT EXISTS idx_accounts_is_suspended ON accounts(is_suspended);
CREATE INDEX IF NOT EXISTS idx_accounts_status ON accounts(status);
