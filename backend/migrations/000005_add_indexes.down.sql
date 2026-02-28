DROP INDEX CONCURRENTLY IF EXISTS idx_rotation_performance_stats_rotation_type_date;
DROP INDEX CONCURRENTLY IF EXISTS idx_rotation_performance_stats_campaign_id_date;

DROP INDEX CONCURRENTLY IF EXISTS idx_rotation_history_recipient_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_rotation_history_rotation_type_rotated_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_rotation_history_campaign_id_rotated_at;

DROP INDEX CONCURRENTLY IF EXISTS idx_attachment_rotation_state_campaign_id_is_active;
DROP INDEX CONCURRENTLY IF EXISTS idx_template_rotation_state_campaign_id_is_active;
DROP INDEX CONCURRENTLY IF EXISTS idx_custom_field_rotation_campaign_id_field_name;
DROP INDEX CONCURRENTLY IF EXISTS idx_subject_rotation_campaign_id_is_active;
DROP INDEX CONCURRENTLY IF EXISTS idx_sender_name_rotation_campaign_id_is_active;

DROP INDEX CONCURRENTLY IF EXISTS idx_notification_history_sent_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_notification_history_status_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_notification_history_event_type_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_notification_history_campaign_id_created_at;

DROP INDEX CONCURRENTLY IF EXISTS idx_notification_queue_next_retry_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_notification_queue_campaign_id_status;
DROP INDEX CONCURRENTLY IF EXISTS idx_notification_queue_priority_scheduled_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_notification_queue_status_scheduled_at;

DROP INDEX CONCURRENTLY IF EXISTS idx_proxy_usage_logs_success;
DROP INDEX CONCURRENTLY IF EXISTS idx_proxy_usage_logs_account_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_proxy_usage_logs_campaign_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_proxy_usage_logs_proxy_id_used_at;

DROP INDEX CONCURRENTLY IF EXISTS idx_proxy_health_history_is_healthy;
DROP INDEX CONCURRENTLY IF EXISTS idx_proxy_health_history_checked_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_proxy_health_history_proxy_id_checked_at;

DROP INDEX CONCURRENTLY IF EXISTS idx_proxies_last_health_check;
DROP INDEX CONCURRENTLY IF EXISTS idx_proxies_last_used_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_proxies_health_score;
DROP INDEX CONCURRENTLY IF EXISTS idx_proxies_status_is_active;
DROP INDEX CONCURRENTLY IF EXISTS idx_proxies_proxy_type_status;

DROP INDEX CONCURRENTLY IF EXISTS idx_system_config_is_active;
DROP INDEX CONCURRENTLY IF EXISTS idx_system_config_config_key;
DROP INDEX CONCURRENTLY IF EXISTS idx_system_config_category;

DROP INDEX CONCURRENTLY IF EXISTS idx_account_stats_account_id_hour;
DROP INDEX CONCURRENTLY IF EXISTS idx_account_stats_date_hour;
DROP INDEX CONCURRENTLY IF EXISTS idx_account_stats_account_id_date;

DROP INDEX CONCURRENTLY IF EXISTS idx_campaign_stats_campaign_id_hour;
DROP INDEX CONCURRENTLY IF EXISTS idx_campaign_stats_date_hour;
DROP INDEX CONCURRENTLY IF EXISTS idx_campaign_stats_campaign_id_date;

DROP INDEX CONCURRENTLY IF EXISTS idx_logs_message_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_logs_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_logs_account_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_logs_recipient_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_logs_level_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_logs_campaign_id_created_at;

DROP INDEX CONCURRENTLY IF EXISTS idx_recipients_campaign_id_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_recipients_status_processed_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_recipients_email;
DROP INDEX CONCURRENTLY IF EXISTS idx_recipients_campaign_id_status;

DROP INDEX CONCURRENTLY IF EXISTS idx_templates_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_templates_spam_score;
DROP INDEX CONCURRENTLY IF EXISTS idx_templates_is_active;
DROP INDEX CONCURRENTLY IF EXISTS idx_templates_name_trgm;

DROP INDEX CONCURRENTLY IF EXISTS idx_accounts_daily_sent_count;
DROP INDEX CONCURRENTLY IF EXISTS idx_accounts_last_used_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_accounts_suspended_until;
DROP INDEX CONCURRENTLY IF EXISTS idx_accounts_health_score;
DROP INDEX CONCURRENTLY IF EXISTS idx_accounts_status_is_active;
DROP INDEX CONCURRENTLY IF EXISTS idx_accounts_provider_status;
DROP INDEX CONCURRENTLY IF EXISTS idx_accounts_email;

DROP INDEX CONCURRENTLY IF EXISTS idx_campaigns_name_trgm;
DROP INDEX CONCURRENTLY IF EXISTS idx_campaigns_completed_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_campaigns_started_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_campaigns_status_scheduled_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_campaigns_status_created_at;

DROP EXTENSION IF EXISTS pg_trgm;
