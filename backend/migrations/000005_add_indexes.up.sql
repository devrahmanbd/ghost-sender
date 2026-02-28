-- ============================================================================
-- CAMPAIGNS INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_campaigns_status_created_at ON campaigns(status, created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_campaigns_status_scheduled_at ON campaigns(status, scheduled_at) WHERE scheduled_at IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_campaigns_start_time ON campaigns(start_time DESC) WHERE start_time IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_campaigns_end_time ON campaigns(end_time DESC) WHERE end_time IS NOT NULL;
-- Requires pg_trgm extension (commented out by default)
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_campaigns_name_trgm ON campaigns USING gin(name gin_trgm_ops);

-- ============================================================================
-- ACCOUNTS INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_email ON accounts(email);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_provider_status ON accounts(provider, status);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_status_active ON accounts(status) WHERE status = 'active';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_health_score ON accounts(health_score DESC) WHERE status = 'active';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_suspended_until ON accounts(suspended_until) WHERE status = 'suspended';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_last_used_at ON accounts(last_used_at DESC) WHERE status = 'active';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_daily_sent_count ON accounts(sent_today, daily_limit) WHERE status = 'active';

-- ============================================================================
-- TEMPLATES INDEXES
-- ============================================================================
-- Requires pg_trgm extension (commented out by default)
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_templates_name_trgm ON templates USING gin(name gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_templates_is_active ON templates(is_active);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_templates_spam_score ON templates(spam_score) WHERE is_active = true;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_templates_created_at ON templates(created_at DESC);

-- ============================================================================
-- RECIPIENTS INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recipients_campaign_id_status ON recipients(campaign_id, status);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recipients_email ON recipients(email);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recipients_status_sent_at ON recipients(status, sent_at) WHERE sent_at IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recipients_campaign_id_created_at ON recipients(campaign_id, created_at DESC);

-- ============================================================================
-- EMAIL LOGS INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_logs_campaign_id_created_at ON email_logs(campaign_id, created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_logs_level_created_at ON email_logs(log_level, created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_logs_recipient_id ON email_logs(recipient_id) WHERE recipient_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_logs_account_id ON email_logs(account_id) WHERE account_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_logs_created_at ON email_logs(created_at DESC);
-- Requires pg_trgm extension (commented out by default)
-- CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_logs_subject_trgm ON email_logs USING gin(subject gin_trgm_ops);

-- ============================================================================
-- CAMPAIGN STATS INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_campaign_stats_campaign_id_snapshot ON campaign_stats(campaign_id, snapshot_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_campaign_stats_snapshot_at ON campaign_stats(snapshot_at DESC);

-- ============================================================================
-- ACCOUNT STATS INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_stats_account_id_date ON account_stats(account_id, date DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_stats_date_hour ON account_stats(date DESC, hour);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_stats_account_id_hour ON account_stats(account_id, date DESC, hour);

-- ============================================================================
-- SYSTEM CONFIG INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_system_config_category ON system_config(category);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_system_config_config_key ON system_config(config_key);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_system_config_active ON system_config(is_active) WHERE is_active = true;

-- ============================================================================
-- PROXIES INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_proxies_proxy_type_status ON proxies(proxy_type, status);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_proxies_status_enabled ON proxies(status, is_enabled);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_proxies_health_score ON proxies(health_score DESC) WHERE is_enabled = true;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_proxies_last_used_at ON proxies(last_used_at DESC) WHERE is_enabled = true;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_proxies_last_checked_at ON proxies(last_checked_at) WHERE is_enabled = true;

-- ============================================================================
-- PROXY HEALTH CHECKS INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_proxy_health_checks_proxy_id_time ON proxy_health_checks(proxy_id, check_time DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_proxy_health_checks_time ON proxy_health_checks(check_time DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_proxy_health_checks_success ON proxy_health_checks(success, check_time DESC);

-- ============================================================================
-- NOTIFICATION QUEUE INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_queue_status_scheduled_at ON notification_queue(status, scheduled_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_queue_priority_scheduled_at ON notification_queue(priority DESC, scheduled_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_queue_campaign_id_status ON notification_queue(campaign_id, status);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_queue_next_retry_at ON notification_queue(next_retry_at) WHERE status = 'pending' AND next_retry_at IS NOT NULL;

-- ============================================================================
-- NOTIFICATION HISTORY INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_history_campaign_id_created_at ON notification_history(campaign_id, created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_history_event_type_created_at ON notification_history(event_type, created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_history_status_created_at ON notification_history(status, created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_history_sent_at ON notification_history(sent_at DESC) WHERE sent_at IS NOT NULL;

-- ============================================================================
-- ROTATION INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sender_name_rotation_campaign_id_active ON sender_name_rotation(campaign_id, is_active);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_subject_rotation_campaign_id_active ON subject_rotation(campaign_id, is_active);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_custom_field_rotation_campaign_id_field ON custom_field_rotation(campaign_id, field_name);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_template_rotation_state_campaign_id_active ON template_rotation_state(campaign_id, is_active);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_attachment_rotation_state_campaign_id_active ON attachment_rotation_state(campaign_id, is_active);

-- ============================================================================
-- ROTATION HISTORY INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_rotation_history_campaign_id_rotated_at ON rotation_history(campaign_id, rotated_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_rotation_history_rotation_type_rotated_at ON rotation_history(rotation_type, rotated_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_rotation_history_recipient_id ON rotation_history(recipient_id) WHERE recipient_id IS NOT NULL;

-- ============================================================================
-- ROTATION PERFORMANCE STATS INDEXES
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_rotation_performance_stats_campaign_date ON rotation_performance_stats(campaign_id, date DESC, hour);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_rotation_performance_stats_type_date ON rotation_performance_stats(rotation_type, date DESC);

-- ============================================================================
-- ANALYZE TABLES FOR QUERY PLANNER OPTIMIZATION
-- ============================================================================
ANALYZE campaigns;
ANALYZE accounts;
ANALYZE templates;
ANALYZE recipients;
ANALYZE email_logs;
ANALYZE campaign_stats;
ANALYZE account_stats;
ANALYZE system_config;
ANALYZE proxies;
ANALYZE proxy_health_checks;
ANALYZE telegram_config;
ANALYZE notification_templates;
ANALYZE notification_subscriptions;
ANALYZE notification_queue;
ANALYZE notification_history;
ANALYZE notification_stats;
ANALYZE sender_name_rotation;
ANALYZE subject_rotation;
ANALYZE custom_field_rotation;
ANALYZE template_rotation_state;
ANALYZE attachment_rotation_state;
ANALYZE rotation_performance_stats;
ANALYZE rotation_history;
