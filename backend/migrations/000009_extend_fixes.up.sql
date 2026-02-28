ALTER TABLE campaigns
  ADD COLUMN IF NOT EXISTS state               VARCHAR(50)   DEFAULT 'idle',
  ADD COLUMN IF NOT EXISTS priority            INTEGER       DEFAULT 0,
  ADD COLUMN IF NOT EXISTS started_at          TIMESTAMP,
  ADD COLUMN IF NOT EXISTS completed_at        TIMESTAMP,
  ADD COLUMN IF NOT EXISTS failed_at           TIMESTAMP,
  ADD COLUMN IF NOT EXISTS success_rate        DECIMAL(5,2)  DEFAULT 0.00,
  ADD COLUMN IF NOT EXISTS progress            DECIMAL(5,2)  DEFAULT 0.00,
  ADD COLUMN IF NOT EXISTS throughput          DECIMAL(10,2) DEFAULT 0.00,
  ADD COLUMN IF NOT EXISTS estimated_eta       TIMESTAMP,
  ADD COLUMN IF NOT EXISTS config              JSONB         DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS template_ids        UUID[]        DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS account_ids         UUID[]        DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS recipient_list_id   UUID,
  ADD COLUMN IF NOT EXISTS proxy_ids           UUID[]        DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS tags                TEXT[]        DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS last_checkpoint     TIMESTAMP,
  ADD COLUMN IF NOT EXISTS checkpoint_data     JSONB         DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS is_archived         BOOLEAN       DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS archived_at         TIMESTAMP,
  ADD COLUMN IF NOT EXISTS updated_by          VARCHAR(100)  DEFAULT '';

-- Back-fill from existing columns
UPDATE campaigns SET progress = progress_percentage WHERE progress = 0;
UPDATE campaigns SET config   = settings            WHERE config  = '{}';
ALTER TABLE campaigns
  DROP COLUMN IF EXISTS state,
  DROP COLUMN IF EXISTS priority,
  DROP COLUMN IF EXISTS started_at,
  DROP COLUMN IF EXISTS completed_at,
  DROP COLUMN IF EXISTS failed_at,
  DROP COLUMN IF EXISTS success_rate,
  DROP COLUMN IF EXISTS progress,
  DROP COLUMN IF EXISTS throughput,
  DROP COLUMN IF EXISTS estimated_eta,
  DROP COLUMN IF EXISTS config,
  DROP COLUMN IF EXISTS template_ids,
  DROP COLUMN IF EXISTS account_ids,
  DROP COLUMN IF EXISTS recipient_list_id,
  DROP COLUMN IF EXISTS proxy_ids,
  DROP COLUMN IF EXISTS tags,
  DROP COLUMN IF EXISTS last_checkpoint,
  DROP COLUMN IF EXISTS checkpoint_data,
  DROP COLUMN IF EXISTS is_archived,
  DROP COLUMN IF EXISTS archived_at,
  DROP COLUMN IF EXISTS updated_by;
CREATE TABLE IF NOT EXISTS configs (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    section      VARCHAR(100) NOT NULL,
    key          VARCHAR(255) NOT NULL,
    value        TEXT         NOT NULL DEFAULT '',
    type         VARCHAR(50)  NOT NULL DEFAULT 'string',
    default_value TEXT        DEFAULT '',
    description  TEXT,
    validation   TEXT,
    is_encrypted BOOLEAN      DEFAULT FALSE,
    is_sensitive BOOLEAN      DEFAULT FALSE,
    is_readonly  BOOLEAN      DEFAULT FALSE,
    tags         TEXT[]       DEFAULT '{}',
    metadata     JSONB        DEFAULT '{}',
    version      INTEGER      DEFAULT 1,
    created_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_by   VARCHAR(100) DEFAULT '',
    UNIQUE(section, key)
);

CREATE TABLE IF NOT EXISTS config_history (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    config_id     UUID REFERENCES configs(id) ON DELETE CASCADE,
    section       VARCHAR(100) NOT NULL,
    key           VARCHAR(255) NOT NULL,
    old_value     TEXT,
    new_value     TEXT,
    changed_by    VARCHAR(100),
    change_reason TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_configs_section ON configs(section);
CREATE INDEX IF NOT EXISTS idx_configs_key     ON configs(key);
CREATE TABLE IF NOT EXISTS logs (
    id             UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    time           TIMESTAMP    NOT NULL DEFAULT NOW(),
    level          VARCHAR(20)  NOT NULL DEFAULT 'info',
    category       VARCHAR(50),
    session_id     VARCHAR(100),
    campaign_id    VARCHAR(100),
    account_id     VARCHAR(100),
    recipient_id   VARCHAR(100),
    proxy_id       VARCHAR(100),
    template_id    VARCHAR(100),
    message        TEXT         NOT NULL,
    details        JSONB        DEFAULT '{}',
    error_code     VARCHAR(50),
    error_class    VARCHAR(100),
    stack_trace    TEXT,
    request_id     VARCHAR(100),
    trace_id       VARCHAR(100),
    span_id        VARCHAR(100),
    http_method    VARCHAR(10),
    http_path      TEXT,
    http_status    INTEGER,
    duration_ms    BIGINT,
    client_ip      VARCHAR(45),
    user_agent     TEXT,
    node_id        VARCHAR(100),
    hostname       VARCHAR(255),
    environment    VARCHAR(50),
    shard          VARCHAR(50),
    metric_name    VARCHAR(255),
    metric_value   DOUBLE PRECISION,
    metric_unit    VARCHAR(50),
    metric_labels  JSONB        DEFAULT '{}',
    user_id        VARCHAR(100),
    username       VARCHAR(255),
    tenant_id      VARCHAR(100),
    source         VARCHAR(100),
    subsystem      VARCHAR(100),
    component      VARCHAR(100),
    version        VARCHAR(50),
    correlation_id VARCHAR(100),
    archived       BOOLEAN      DEFAULT FALSE,
    created_at     TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_logs_level      ON logs(level);
CREATE INDEX IF NOT EXISTS idx_logs_campaign_id ON logs(campaign_id);
CREATE INDEX IF NOT EXISTS idx_logs_created_at  ON logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_logs_archived    ON logs(archived) WHERE archived = FALSE;
