DELETE FROM system_config WHERE config_key IN (
    'proxy.health_check_enabled',
    'proxy.health_check_interval',
    'proxy.health_check_timeout',
    'proxy.min_health_score',
    'proxy.max_failures',
    'proxy.suspension_duration',
    'proxy.rotation_strategy',
    'proxy.max_concurrent',
    'proxy.test_url',
    'proxy.verify_anonymity'
);

DROP TRIGGER IF EXISTS update_proxy_rotation_config_updated_at ON proxy_rotation_config;
DROP TRIGGER IF EXISTS update_account_proxy_mapping_updated_at ON account_proxy_mapping;
DROP TRIGGER IF EXISTS update_proxy_stats_updated_at ON proxy_stats;
DROP TRIGGER IF EXISTS update_proxies_updated_at ON proxies;

DROP INDEX IF EXISTS idx_proxy_rotation_config_enabled;
DROP INDEX IF EXISTS idx_proxy_rotation_config_config_name;

DROP INDEX IF EXISTS idx_account_proxy_mapping_is_active;
DROP INDEX IF EXISTS idx_account_proxy_mapping_proxy_id;
DROP INDEX IF EXISTS idx_account_proxy_mapping_account_id;

DROP INDEX IF EXISTS idx_proxy_stats_hour;
DROP INDEX IF EXISTS idx_proxy_stats_date;
DROP INDEX IF EXISTS idx_proxy_stats_proxy_id;

DROP INDEX IF EXISTS idx_proxy_health_checks_success;
DROP INDEX IF EXISTS idx_proxy_health_checks_check_time;
DROP INDEX IF EXISTS idx_proxy_health_checks_proxy_id;

DROP INDEX IF EXISTS idx_proxies_last_used_at;
DROP INDEX IF EXISTS idx_proxies_last_checked_at;
DROP INDEX IF EXISTS idx_proxies_is_enabled;
DROP INDEX IF EXISTS idx_proxies_health_score;
DROP INDEX IF EXISTS idx_proxies_proxy_type;
DROP INDEX IF EXISTS idx_proxies_status;
DROP INDEX IF EXISTS idx_proxies_host_port;

ALTER TABLE email_logs DROP COLUMN IF EXISTS proxy_port;
ALTER TABLE email_logs DROP COLUMN IF EXISTS proxy_host;
ALTER TABLE email_logs DROP COLUMN IF EXISTS proxy_id;

ALTER TABLE accounts DROP COLUMN IF EXISTS proxy_rotation_enabled;
ALTER TABLE accounts DROP COLUMN IF EXISTS use_proxy;
ALTER TABLE accounts DROP COLUMN IF EXISTS proxy_id;

DROP TABLE IF EXISTS proxy_rotation_config;
DROP TABLE IF EXISTS account_proxy_mapping;
DROP TABLE IF EXISTS proxy_stats;
DROP TABLE IF EXISTS proxy_health_checks;
DROP TABLE IF EXISTS proxies;

DROP TYPE IF EXISTS proxy_status;
DROP TYPE IF EXISTS proxy_type;
