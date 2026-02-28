CREATE TYPE proxy_type AS ENUM (
    'http',
    'https',
    'socks5'
);

CREATE TYPE proxy_status AS ENUM (
    'healthy',
    'unhealthy',
    'untested',
    'suspended',
    'disabled'
);

CREATE TABLE proxies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255),
    
    proxy_type proxy_type NOT NULL DEFAULT 'http',
    status proxy_status NOT NULL DEFAULT 'untested',
    
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL CHECK (port > 0 AND port <= 65535),
    
    username VARCHAR(255),
    password TEXT,
    
    proxy_url TEXT NOT NULL,
    
    is_authenticated BOOLEAN DEFAULT false,
    
    health_score DECIMAL(3,2) DEFAULT 1.00,
    
    last_checked_at TIMESTAMP,
    last_used_at TIMESTAMP,
    last_success_at TIMESTAMP,
    last_failure_at TIMESTAMP,
    
    total_requests INTEGER DEFAULT 0,
    successful_requests INTEGER DEFAULT 0,
    failed_requests INTEGER DEFAULT 0,
    
    consecutive_failures INTEGER DEFAULT 0,
    max_failures_threshold INTEGER DEFAULT 5,
    
    avg_latency_ms DECIMAL(10,2) DEFAULT 0.00,
    min_latency_ms DECIMAL(10,2),
    max_latency_ms DECIMAL(10,2),
    
    response_times JSONB DEFAULT '[]',
    
    is_anonymous BOOLEAN DEFAULT false,
    anonymity_verified_at TIMESTAMP,
    
    country VARCHAR(100),
    city VARCHAR(100),
    isp VARCHAR(255),
    location_checked_at TIMESTAMP,
    
    weight INTEGER DEFAULT 1,
    priority INTEGER DEFAULT 0,
    
    suspended_at TIMESTAMP,
    suspended_until TIMESTAMP,
    suspension_reason TEXT,
    
    is_enabled BOOLEAN DEFAULT true,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    
    UNIQUE(host, port)
);

CREATE TABLE proxy_health_checks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    proxy_id UUID NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
    
    check_time TIMESTAMP NOT NULL DEFAULT NOW(),
    
    success BOOLEAN NOT NULL DEFAULT false,
    
    latency_ms DECIMAL(10,2),
    
    status_code INTEGER,
    
    error_message TEXT,
    error_type VARCHAR(100),
    
    test_url VARCHAR(500),
    test_target VARCHAR(255),
    
    anonymity_test BOOLEAN DEFAULT false,
    is_anonymous BOOLEAN,
    
    location_detected BOOLEAN DEFAULT false,
    detected_country VARCHAR(100),
    detected_city VARCHAR(100),
    
    connection_time_ms DECIMAL(10,2),
    response_time_ms DECIMAL(10,2),
    
    health_score_after DECIMAL(3,2),
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE proxy_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    proxy_id UUID NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
    
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    hour INTEGER CHECK (hour >= 0 AND hour <= 23),
    
    total_requests INTEGER DEFAULT 0,
    successful_requests INTEGER DEFAULT 0,
    failed_requests INTEGER DEFAULT 0,
    
    success_rate DECIMAL(5,2) DEFAULT 0.00,
    
    avg_latency_ms DECIMAL(10,2) DEFAULT 0.00,
    min_latency_ms DECIMAL(10,2),
    max_latency_ms DECIMAL(10,2),
    
    total_bytes_sent BIGINT DEFAULT 0,
    total_bytes_received BIGINT DEFAULT 0,
    
    active_connections INTEGER DEFAULT 0,
    max_concurrent_connections INTEGER DEFAULT 0,
    
    health_score DECIMAL(3,2) DEFAULT 1.00,
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(proxy_id, date, hour)
);

CREATE TABLE account_proxy_mapping (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    proxy_id UUID NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
    
    is_active BOOLEAN DEFAULT true,
    is_preferred BOOLEAN DEFAULT false,
    
    assigned_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMP,
    
    usage_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    failure_count INTEGER DEFAULT 0,
    
    notes TEXT,
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(account_id, proxy_id)
);

CREATE TABLE proxy_rotation_config (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    config_name VARCHAR(255) NOT NULL UNIQUE,
    
    strategy rotation_strategy DEFAULT 'round_robin',
    
    enabled BOOLEAN DEFAULT true,
    
    min_health_score DECIMAL(3,2) DEFAULT 0.50,
    max_failures_allowed INTEGER DEFAULT 5,
    
    sticky_sessions BOOLEAN DEFAULT false,
    session_timeout_minutes INTEGER DEFAULT 30,
    
    health_check_enabled BOOLEAN DEFAULT true,
    health_check_interval_minutes INTEGER DEFAULT 5,
    
    auto_disable_failed BOOLEAN DEFAULT true,
    auto_enable_recovered BOOLEAN DEFAULT true,
    
    load_balancing_enabled BOOLEAN DEFAULT false,
    max_concurrent_per_proxy INTEGER DEFAULT 100,
    
    proxy_pool JSONB DEFAULT '[]',
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS proxy_id UUID REFERENCES proxies(id) ON DELETE SET NULL;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS use_proxy BOOLEAN DEFAULT false;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS proxy_rotation_enabled BOOLEAN DEFAULT false;

ALTER TABLE email_logs ADD COLUMN IF NOT EXISTS proxy_id UUID REFERENCES proxies(id) ON DELETE SET NULL;
ALTER TABLE email_logs ADD COLUMN IF NOT EXISTS proxy_host VARCHAR(255);
ALTER TABLE email_logs ADD COLUMN IF NOT EXISTS proxy_port INTEGER;

CREATE INDEX idx_proxies_host_port ON proxies(host, port);
CREATE INDEX idx_proxies_status ON proxies(status);
CREATE INDEX idx_proxies_proxy_type ON proxies(proxy_type);
CREATE INDEX idx_proxies_health_score ON proxies(health_score);
CREATE INDEX idx_proxies_is_enabled ON proxies(is_enabled);
CREATE INDEX idx_proxies_last_checked_at ON proxies(last_checked_at);
CREATE INDEX idx_proxies_last_used_at ON proxies(last_used_at);

CREATE INDEX idx_proxy_health_checks_proxy_id ON proxy_health_checks(proxy_id);
CREATE INDEX idx_proxy_health_checks_check_time ON proxy_health_checks(check_time DESC);
CREATE INDEX idx_proxy_health_checks_success ON proxy_health_checks(success);

CREATE INDEX idx_proxy_stats_proxy_id ON proxy_stats(proxy_id);
CREATE INDEX idx_proxy_stats_date ON proxy_stats(date DESC);
CREATE INDEX idx_proxy_stats_hour ON proxy_stats(hour);

CREATE INDEX idx_account_proxy_mapping_account_id ON account_proxy_mapping(account_id);
CREATE INDEX idx_account_proxy_mapping_proxy_id ON account_proxy_mapping(proxy_id);
CREATE INDEX idx_account_proxy_mapping_is_active ON account_proxy_mapping(is_active);

CREATE INDEX idx_proxy_rotation_config_config_name ON proxy_rotation_config(config_name);
CREATE INDEX idx_proxy_rotation_config_enabled ON proxy_rotation_config(enabled);

CREATE TRIGGER update_proxies_updated_at BEFORE UPDATE ON proxies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_proxy_stats_updated_at BEFORE UPDATE ON proxy_stats
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_account_proxy_mapping_updated_at BEFORE UPDATE ON account_proxy_mapping
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_proxy_rotation_config_updated_at BEFORE UPDATE ON proxy_rotation_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

INSERT INTO system_config (config_key, config_value, config_type, category, description) VALUES
('proxy.health_check_enabled', 'true', 'boolean', 'proxy', 'Enable automatic proxy health checking'),
('proxy.health_check_interval', '5', 'integer', 'proxy', 'Health check interval in minutes'),
('proxy.health_check_timeout', '10', 'integer', 'proxy', 'Health check timeout in seconds'),
('proxy.min_health_score', '0.5', 'decimal', 'proxy', 'Minimum health score for proxy usage'),
('proxy.max_failures', '5', 'integer', 'proxy', 'Maximum consecutive failures before suspension'),
('proxy.suspension_duration', '30', 'integer', 'proxy', 'Proxy suspension duration in minutes'),
('proxy.rotation_strategy', 'round_robin', 'string', 'proxy', 'Default proxy rotation strategy'),
('proxy.max_concurrent', '100', 'integer', 'proxy', 'Maximum concurrent connections per proxy'),
('proxy.test_url', 'https://www.google.com', 'string', 'proxy', 'Default URL for proxy testing'),
('proxy.verify_anonymity', 'false', 'boolean', 'proxy', 'Verify proxy anonymity on health check');
