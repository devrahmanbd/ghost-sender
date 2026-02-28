CREATE TYPE notification_channel AS ENUM (
    'telegram',
    'email',
    'webhook',
    'slack'
);

CREATE TYPE notification_event AS ENUM (
    'campaign_started',
    'campaign_completed',
    'campaign_failed',
    'campaign_paused',
    'campaign_resumed',
    'account_suspended',
    'account_recovered',
    'daily_limit_reached',
    'error_threshold_reached',
    'system_error',
    'low_health_score',
    'proxy_failed',
    'high_spam_score'
);

CREATE TYPE notification_priority AS ENUM (
    'low',
    'normal',
    'high',
    'critical'
);

CREATE TYPE notification_status AS ENUM (
    'pending',
    'sent',
    'failed',
    'cancelled'
);

CREATE TABLE telegram_config (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    bot_token TEXT NOT NULL,
    bot_username VARCHAR(255),
    
    chat_ids TEXT[] DEFAULT '{}',
    
    is_enabled BOOLEAN DEFAULT true,
    
    parse_mode VARCHAR(50) DEFAULT 'Markdown',
    
    disable_notification BOOLEAN DEFAULT false,
    disable_web_page_preview BOOLEAN DEFAULT true,
    
    retry_enabled BOOLEAN DEFAULT true,
    max_retries INTEGER DEFAULT 3,
    retry_delay_seconds INTEGER DEFAULT 5,
    
    rate_limit_messages INTEGER DEFAULT 30,
    rate_limit_window_seconds INTEGER DEFAULT 60,
    
    last_test_at TIMESTAMP,
    test_success BOOLEAN,
    test_error TEXT,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE notification_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    template_name VARCHAR(255) NOT NULL UNIQUE,
    template_key VARCHAR(100) NOT NULL UNIQUE,
    
    event_type notification_event NOT NULL,
    channel notification_channel NOT NULL DEFAULT 'telegram',
    
    title VARCHAR(500),
    message_template TEXT NOT NULL,
    
    format VARCHAR(50) DEFAULT 'markdown',
    
    variables JSONB DEFAULT '[]',
    
    priority notification_priority DEFAULT 'normal',
    
    is_enabled BOOLEAN DEFAULT true,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE notification_subscriptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    event_type notification_event NOT NULL,
    channel notification_channel NOT NULL DEFAULT 'telegram',
    
    is_enabled BOOLEAN DEFAULT true,
    
    chat_ids TEXT[] DEFAULT '{}',
    
    priority notification_priority DEFAULT 'normal',
    
    throttle_enabled BOOLEAN DEFAULT false,
    throttle_minutes INTEGER DEFAULT 5,
    
    quiet_hours_enabled BOOLEAN DEFAULT false,
    quiet_hours_start TIME,
    quiet_hours_end TIME,
    
    filters JSONB DEFAULT '{}',
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(event_type, channel)
);

CREATE TABLE notification_queue (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    event_type notification_event NOT NULL,
    channel notification_channel NOT NULL DEFAULT 'telegram',
    
    priority notification_priority DEFAULT 'normal',
    
    title VARCHAR(500),
    message TEXT NOT NULL,
    
    chat_ids TEXT[] DEFAULT '{}',
    
    campaign_id UUID REFERENCES campaigns(id) ON DELETE SET NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    
    scheduled_at TIMESTAMP NOT NULL DEFAULT NOW(),
    next_retry_at TIMESTAMP,
    
    status notification_status NOT NULL DEFAULT 'pending',
    
    error_message TEXT,
    
    payload JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE notification_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    event_type notification_event NOT NULL,
    channel notification_channel NOT NULL DEFAULT 'telegram',
    
    priority notification_priority DEFAULT 'normal',
    
    title VARCHAR(500),
    message TEXT NOT NULL,
    
    chat_id VARCHAR(255),
    
    campaign_id UUID REFERENCES campaigns(id) ON DELETE SET NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    
    status notification_status NOT NULL DEFAULT 'sent',
    
    sent_at TIMESTAMP,
    delivered_at TIMESTAMP,
    failed_at TIMESTAMP,
    
    retry_count INTEGER DEFAULT 0,
    
    error_message TEXT,
    error_code VARCHAR(50),
    
    message_id VARCHAR(255),
    
    response_data JSONB,
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE notification_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    hour INTEGER CHECK (hour >= 0 AND hour <= 23),
    
    event_type notification_event,
    channel notification_channel DEFAULT 'telegram',
    
    total_sent INTEGER DEFAULT 0,
    total_failed INTEGER DEFAULT 0,
    total_pending INTEGER DEFAULT 0,
    
    success_rate DECIMAL(5,2) DEFAULT 0.00,
    
    avg_delivery_time_ms DECIMAL(10,2) DEFAULT 0.00,
    
    by_priority JSONB DEFAULT '{}',
    by_event JSONB DEFAULT '{}',
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(date, hour, event_type, channel)
);

CREATE INDEX idx_telegram_config_is_enabled ON telegram_config(is_enabled);

CREATE INDEX idx_notification_templates_event_type ON notification_templates(event_type);
CREATE INDEX idx_notification_templates_channel ON notification_templates(channel);
CREATE INDEX idx_notification_templates_template_key ON notification_templates(template_key);
CREATE INDEX idx_notification_templates_is_enabled ON notification_templates(is_enabled);

CREATE INDEX idx_notification_subscriptions_event_type ON notification_subscriptions(event_type);
CREATE INDEX idx_notification_subscriptions_channel ON notification_subscriptions(channel);
CREATE INDEX idx_notification_subscriptions_is_enabled ON notification_subscriptions(is_enabled);

CREATE INDEX idx_notification_queue_status ON notification_queue(status);
CREATE INDEX idx_notification_queue_priority ON notification_queue(priority);
CREATE INDEX idx_notification_queue_scheduled_at ON notification_queue(scheduled_at);
CREATE INDEX idx_notification_queue_campaign_id ON notification_queue(campaign_id);
CREATE INDEX idx_notification_queue_next_retry_at ON notification_queue(next_retry_at) WHERE status = 'pending';

CREATE INDEX idx_notification_history_event_type ON notification_history(event_type);
CREATE INDEX idx_notification_history_channel ON notification_history(channel);
CREATE INDEX idx_notification_history_status ON notification_history(status);
CREATE INDEX idx_notification_history_campaign_id ON notification_history(campaign_id);
CREATE INDEX idx_notification_history_created_at ON notification_history(created_at DESC);
CREATE INDEX idx_notification_history_sent_at ON notification_history(sent_at DESC);

CREATE INDEX idx_notification_stats_date ON notification_stats(date DESC);
CREATE INDEX idx_notification_stats_hour ON notification_stats(hour);
CREATE INDEX idx_notification_stats_event_type ON notification_stats(event_type);
CREATE INDEX idx_notification_stats_channel ON notification_stats(channel);

CREATE TRIGGER update_telegram_config_updated_at BEFORE UPDATE ON telegram_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_notification_templates_updated_at BEFORE UPDATE ON notification_templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_notification_subscriptions_updated_at BEFORE UPDATE ON notification_subscriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_notification_queue_updated_at BEFORE UPDATE ON notification_queue
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_notification_stats_updated_at BEFORE UPDATE ON notification_stats
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

INSERT INTO notification_templates (template_name, template_key, event_type, channel, title, message_template, format, priority) VALUES
('Campaign Started', 'campaign_started', 'campaign_started', 'telegram', '🚀 Campaign Started', '🚀 *Campaign Started*\n\n📋 Name: {{campaign_name}}\n📊 Recipients: {{total_recipients}}\n⏰ Started: {{start_time}}', 'markdown', 'normal'),
('Campaign Completed', 'campaign_completed', 'campaign_completed', 'telegram', '✅ Campaign Completed', '✅ *Campaign Completed*\n\n📋 Name: {{campaign_name}}\n✉️ Sent: {{sent_count}}\n❌ Failed: {{failed_count}}\n📈 Success Rate: {{success_rate}}%\n⏱ Duration: {{duration}}', 'markdown', 'normal'),
('Campaign Failed', 'campaign_failed', 'campaign_failed', 'telegram', '❌ Campaign Failed', '❌ *Campaign Failed*\n\n📋 Name: {{campaign_name}}\n⚠️ Error: {{error_message}}\n⏰ Failed At: {{failed_time}}', 'markdown', 'high'),
('Account Suspended', 'account_suspended', 'account_suspended', 'telegram', '⚠️ Account Suspended', '⚠️ *Account Suspended*\n\n📧 Email: {{account_email}}\n🔒 Reason: {{suspension_reason}}\n⏰ Time: {{suspended_at}}', 'markdown', 'high'),
('System Error', 'system_error', 'system_error', 'telegram', '🔴 System Error', '🔴 *System Error*\n\n⚠️ Error: {{error_message}}\n📍 Location: {{error_location}}\n⏰ Time: {{error_time}}', 'markdown', 'critical'),
('Daily Limit Reached', 'daily_limit_reached', 'daily_limit_reached', 'telegram', '📊 Daily Limit Reached', '📊 *Daily Limit Reached*\n\n📧 Account: {{account_email}}\n📈 Sent Today: {{sent_today}}\n🎯 Limit: {{daily_limit}}', 'markdown', 'normal');

INSERT INTO system_config (config_key, config_value, config_type, category, description) VALUES
('notification.telegram.enabled', 'false', 'boolean', 'notification', 'Enable Telegram notifications'),
('notification.telegram.retry_enabled', 'true', 'boolean', 'notification', 'Enable retry for failed notifications'),
('notification.telegram.max_retries', '3', 'integer', 'notification', 'Maximum retry attempts'),
('notification.telegram.retry_delay', '5', 'integer', 'notification', 'Delay between retries in seconds'),
('notification.telegram.rate_limit', '30', 'integer', 'notification', 'Maximum messages per window'),
('notification.telegram.rate_window', '60', 'integer', 'notification', 'Rate limit window in seconds'),
('notification.quiet_hours_enabled', 'false', 'boolean', 'notification', 'Enable quiet hours'),
('notification.quiet_hours_start', '22:00', 'string', 'notification', 'Quiet hours start time'),
('notification.quiet_hours_end', '08:00', 'string', 'notification', 'Quiet hours end time');
