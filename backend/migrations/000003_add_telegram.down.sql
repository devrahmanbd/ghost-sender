DELETE FROM system_config WHERE config_key IN (
    'notification.telegram.enabled',
    'notification.telegram.retry_enabled',
    'notification.telegram.max_retries',
    'notification.telegram.retry_delay',
    'notification.telegram.rate_limit',
    'notification.telegram.rate_window',
    'notification.quiet_hours_enabled',
    'notification.quiet_hours_start',
    'notification.quiet_hours_end'
);

DROP TRIGGER IF EXISTS update_notification_stats_updated_at ON notification_stats;
DROP TRIGGER IF EXISTS update_notification_queue_updated_at ON notification_queue;
DROP TRIGGER IF EXISTS update_notification_subscriptions_updated_at ON notification_subscriptions;
DROP TRIGGER IF EXISTS update_notification_templates_updated_at ON notification_templates;
DROP TRIGGER IF EXISTS update_telegram_config_updated_at ON telegram_config;

DROP INDEX IF EXISTS idx_notification_stats_channel;
DROP INDEX IF EXISTS idx_notification_stats_event_type;
DROP INDEX IF EXISTS idx_notification_stats_hour;
DROP INDEX IF EXISTS idx_notification_stats_date;

DROP INDEX IF EXISTS idx_notification_history_sent_at;
DROP INDEX IF EXISTS idx_notification_history_created_at;
DROP INDEX IF EXISTS idx_notification_history_campaign_id;
DROP INDEX IF EXISTS idx_notification_history_status;
DROP INDEX IF EXISTS idx_notification_history_channel;
DROP INDEX IF EXISTS idx_notification_history_event_type;

DROP INDEX IF EXISTS idx_notification_queue_next_retry_at;
DROP INDEX IF EXISTS idx_notification_queue_campaign_id;
DROP INDEX IF EXISTS idx_notification_queue_scheduled_at;
DROP INDEX IF EXISTS idx_notification_queue_priority;
DROP INDEX IF EXISTS idx_notification_queue_status;

DROP INDEX IF EXISTS idx_notification_subscriptions_is_enabled;
DROP INDEX IF EXISTS idx_notification_subscriptions_channel;
DROP INDEX IF EXISTS idx_notification_subscriptions_event_type;

DROP INDEX IF EXISTS idx_notification_templates_is_enabled;
DROP INDEX IF EXISTS idx_notification_templates_template_key;
DROP INDEX IF EXISTS idx_notification_templates_channel;
DROP INDEX IF EXISTS idx_notification_templates_event_type;

DROP INDEX IF EXISTS idx_telegram_config_is_enabled;

DROP TABLE IF EXISTS notification_stats;
DROP TABLE IF EXISTS notification_history;
DROP TABLE IF EXISTS notification_queue;
DROP TABLE IF EXISTS notification_subscriptions;
DROP TABLE IF EXISTS notification_templates;
DROP TABLE IF EXISTS telegram_config;

DROP TYPE IF EXISTS notification_status;
DROP TYPE IF EXISTS notification_priority;
DROP TYPE IF EXISTS notification_event;
DROP TYPE IF EXISTS notification_channel;
