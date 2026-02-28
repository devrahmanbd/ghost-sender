DROP TRIGGER IF EXISTS update_api_keys_updated_at ON api_keys;
DROP TRIGGER IF EXISTS update_rotation_tracking_updated_at ON rotation_tracking;
DROP TRIGGER IF EXISTS update_sessions_updated_at ON sessions;
DROP TRIGGER IF EXISTS update_system_config_updated_at ON system_config;
DROP TRIGGER IF EXISTS update_account_stats_updated_at ON account_stats;
DROP TRIGGER IF EXISTS update_attachments_updated_at ON attachments;
DROP TRIGGER IF EXISTS update_recipients_updated_at ON recipients;
DROP TRIGGER IF EXISTS update_templates_updated_at ON templates;
DROP TRIGGER IF EXISTS update_accounts_updated_at ON accounts;
DROP TRIGGER IF EXISTS update_campaigns_updated_at ON campaigns;

DROP FUNCTION IF EXISTS update_updated_at_column();

DROP INDEX IF EXISTS idx_audit_logs_created_at;
DROP INDEX IF EXISTS idx_audit_logs_action;
DROP INDEX IF EXISTS idx_audit_logs_entity_id;
DROP INDEX IF EXISTS idx_audit_logs_entity_type;

DROP INDEX IF EXISTS idx_api_keys_is_active;
DROP INDEX IF EXISTS idx_api_keys_api_key;

DROP INDEX IF EXISTS idx_unsubscribe_requests_token;
DROP INDEX IF EXISTS idx_unsubscribe_requests_email;

DROP INDEX IF EXISTS idx_rotation_tracking_rotation_type;
DROP INDEX IF EXISTS idx_rotation_tracking_campaign_id;

DROP INDEX IF EXISTS idx_sessions_expires_at;
DROP INDEX IF EXISTS idx_sessions_status;
DROP INDEX IF EXISTS idx_sessions_session_id;

DROP INDEX IF EXISTS idx_system_config_category;
DROP INDEX IF EXISTS idx_system_config_config_key;

DROP INDEX IF EXISTS idx_account_stats_date;
DROP INDEX IF EXISTS idx_account_stats_account_id;

DROP INDEX IF EXISTS idx_campaign_stats_snapshot_at;
DROP INDEX IF EXISTS idx_campaign_stats_campaign_id;

DROP INDEX IF EXISTS idx_email_logs_recipient_email;
DROP INDEX IF EXISTS idx_email_logs_created_at;
DROP INDEX IF EXISTS idx_email_logs_status;
DROP INDEX IF EXISTS idx_email_logs_account_id;
DROP INDEX IF EXISTS idx_email_logs_recipient_id;
DROP INDEX IF EXISTS idx_email_logs_campaign_id;

DROP INDEX IF EXISTS idx_attachments_cache_key;
DROP INDEX IF EXISTS idx_attachments_file_hash;
DROP INDEX IF EXISTS idx_attachments_campaign_id;

DROP INDEX IF EXISTS idx_recipients_account_id;
DROP INDEX IF EXISTS idx_recipients_status;
DROP INDEX IF EXISTS idx_recipients_email;
DROP INDEX IF EXISTS idx_recipients_campaign_id;

DROP INDEX IF EXISTS idx_templates_spam_score;
DROP INDEX IF EXISTS idx_templates_is_active;
DROP INDEX IF EXISTS idx_templates_name;

DROP INDEX IF EXISTS idx_accounts_last_used_at;
DROP INDEX IF EXISTS idx_accounts_health_score;
DROP INDEX IF EXISTS idx_accounts_status;
DROP INDEX IF EXISTS idx_accounts_provider;
DROP INDEX IF EXISTS idx_accounts_email;

DROP INDEX IF EXISTS idx_campaigns_scheduled_at;
DROP INDEX IF EXISTS idx_campaigns_created_at;
DROP INDEX IF EXISTS idx_campaigns_session_id;
DROP INDEX IF EXISTS idx_campaigns_status;

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS unsubscribe_requests;
DROP TABLE IF EXISTS rotation_tracking;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS system_config;
DROP TABLE IF EXISTS account_stats;
DROP TABLE IF EXISTS campaign_stats;
DROP TABLE IF EXISTS email_logs;
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS recipients;
DROP TABLE IF EXISTS templates;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS campaigns;

DROP TYPE IF EXISTS rotation_strategy;
DROP TYPE IF EXISTS log_level;
DROP TYPE IF EXISTS recipient_status;
DROP TYPE IF EXISTS account_provider;
DROP TYPE IF EXISTS account_status;
DROP TYPE IF EXISTS campaign_status;

DROP EXTENSION IF EXISTS "pgcrypto";
DROP EXTENSION IF EXISTS "uuid-ossp";
