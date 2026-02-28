-- ============================================================
-- 000012_fix_schema_columns.sql  (FINAL AUTHORITATIVE VERSION)
-- Renames all no-underscore column names produced by migrations
-- 000001-000011 to the snake_case names the Go repository code
-- expects.  Every step is idempotent (safe to re-run).
-- ============================================================

-- ── Helpers ──────────────────────────────────────────────────
CREATE OR REPLACE FUNCTION _rename_col(tbl text, old_col text, new_col text)
RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = tbl AND column_name = old_col
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = tbl AND column_name = new_col
    ) THEN
        EXECUTE format('ALTER TABLE %I RENAME COLUMN %I TO %I', tbl, old_col, new_col);
    END IF;
END $$;

-- ENUM → TEXT requires explicit USING cast
CREATE OR REPLACE FUNCTION _to_text(tbl text, col text)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE v_udt text;
BEGIN
    SELECT udt_name INTO v_udt
    FROM information_schema.columns
    WHERE table_name = tbl AND column_name = col;
    IF v_udt IS NOT NULL AND v_udt NOT IN ('text','varchar','bpchar') THEN
        EXECUTE format(
            'ALTER TABLE %I ALTER COLUMN %I TYPE TEXT USING %I::TEXT',
            tbl, col, col);
    END IF;
END $$;

-- ── TABLE: proxies ───────────────────────────────────────────
-- Convert ENUM columns → TEXT
SELECT _to_text('proxies', 'proxy_type');
SELECT _to_text('proxies', 'proxytype');
SELECT _to_text('proxies', 'status');

-- Rename to what proxy.go expects
SELECT _rename_col('proxies', 'proxy_type',           'type');
SELECT _rename_col('proxies', 'proxytype',            'type');
SELECT _rename_col('proxies', 'is_enabled',           'is_active');
SELECT _rename_col('proxies', 'isenabled',            'is_active');
SELECT _rename_col('proxies', 'isactive',             'is_active');
SELECT _rename_col('proxies', 'avg_latency_ms',       'latency_ms');
SELECT _rename_col('proxies', 'avglatencyms',         'latency_ms');
SELECT _rename_col('proxies', 'latencyms',            'latency_ms');
SELECT _rename_col('proxies', 'successful_requests',  'success_count');
SELECT _rename_col('proxies', 'successfulrequests',   'success_count');
SELECT _rename_col('proxies', 'successcount',         'success_count');
SELECT _rename_col('proxies', 'failed_requests',      'failure_count');
SELECT _rename_col('proxies', 'failedrequests',       'failure_count');
SELECT _rename_col('proxies', 'failurecount',         'failure_count');
SELECT _rename_col('proxies', 'consecutive_failures', 'consecutive_fails');
SELECT _rename_col('proxies', 'consecutivefailures',  'consecutive_fails');
SELECT _rename_col('proxies', 'consecutivefails',     'consecutive_fails');
SELECT _rename_col('proxies', 'max_failures_threshold','max_consecutive');
SELECT _rename_col('proxies', 'maxfailuresthreshold', 'max_consecutive');
SELECT _rename_col('proxies', 'maxconsecutive',       'max_consecutive');
-- last_failure_at → last_error_at  (proxy.go uses last_error_at)
SELECT _rename_col('proxies', 'last_failure_at',      'last_error_at');
SELECT _rename_col('proxies', 'lastfailureat',        'last_error_at');
SELECT _rename_col('proxies', 'lasterrorat',          'last_error_at');
-- last_success_at → last_healthy_at  (proxy.go uses last_healthy_at)
SELECT _rename_col('proxies', 'last_success_at',      'last_healthy_at');
SELECT _rename_col('proxies', 'lastsuccessat',        'last_healthy_at');
SELECT _rename_col('proxies', 'lasthealthyat',        'last_healthy_at');
SELECT _rename_col('proxies', 'last_checked_at',      'last_checked_at');   -- already correct
SELECT _rename_col('proxies', 'lastcheckedat',        'last_checked_at');
SELECT _rename_col('proxies', 'last_used_at',         'last_used_at');      -- already correct
SELECT _rename_col('proxies', 'lastusedat',           'last_used_at');
SELECT _rename_col('proxies', 'weight',               'rotation_weight');
SELECT _rename_col('proxies', 'rotationweight',       'rotation_weight');
SELECT _rename_col('proxies', 'lasterror',            'last_error');
SELECT _rename_col('proxies', 'inuse',                'in_use');
SELECT _rename_col('proxies', 'assignedaccounts',     'assigned_accounts');
SELECT _rename_col('proxies', 'maxaccounts',          'max_accounts');
SELECT _rename_col('proxies', 'rotationgroup',        'rotation_group');
SELECT _rename_col('proxies', 'maxconnections',       'max_connections');
-- current_conns is the exact name proxy.go uses (NOT current_connections)
SELECT _rename_col('proxies', 'currentconns',         'current_conns');
SELECT _rename_col('proxies', 'current_connections',  'current_conns');
SELECT _rename_col('proxies', 'bandwidthmb',          'bandwidth_mb');
SELECT _rename_col('proxies', 'bandwidthlimitmb',     'bandwidth_limit_mb');
SELECT _rename_col('proxies', 'resetat',              'reset_at');
SELECT _rename_col('proxies', 'createdat',            'created_at');
SELECT _rename_col('proxies', 'updatedat',            'updated_at');
SELECT _rename_col('proxies', 'deletedat',            'deleted_at');

-- Add new proxy columns needed by proxy.go
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS region         VARCHAR(100);
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS provider       VARCHAR(100);
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS in_use         BOOLEAN       DEFAULT false;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS assigned_accounts INTEGER    DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS max_accounts   INTEGER       DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS rotation_group VARCHAR(255);
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS max_connections INTEGER      DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS current_conns  INTEGER       DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS bandwidth_mb   FLOAT         DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS bandwidth_limit_mb FLOAT     DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS last_error     TEXT;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS last_healthy_at TIMESTAMPTZ;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS reset_at       TIMESTAMPTZ;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS rotation_weight INTEGER      DEFAULT 1;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS tags           TEXT[];
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS created_by     VARCHAR(100);
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS updated_by     VARCHAR(100);

UPDATE proxies SET region = country WHERE region IS NULL AND country IS NOT NULL;

DROP INDEX IF EXISTS idx_proxies_proxy_type;
DROP INDEX IF EXISTS idx_proxies_proxytype;
DROP INDEX IF EXISTS idx_proxies_is_enabled;
DROP INDEX IF EXISTS idx_proxies_isenabled;
DROP INDEX IF EXISTS idx_proxies_isactive;
DROP INDEX IF EXISTS idx_proxies_health_score;
DROP INDEX IF EXISTS idx_proxies_healthscore;
DROP INDEX IF EXISTS idx_proxies_last_checked_at;
DROP INDEX IF EXISTS idx_proxies_lastcheckedat;
DROP INDEX IF EXISTS idx_proxies_last_used_at;
DROP INDEX IF EXISTS idx_proxies_lastusedat;
CREATE INDEX IF NOT EXISTS idx_proxies_type           ON proxies(type);
CREATE INDEX IF NOT EXISTS idx_proxies_status         ON proxies(status);
CREATE INDEX IF NOT EXISTS idx_proxies_is_active      ON proxies(is_active);
CREATE INDEX IF NOT EXISTS idx_proxies_health_score   ON proxies(health_score);
CREATE INDEX IF NOT EXISTS idx_proxies_last_checked   ON proxies(last_checked_at);
CREATE INDEX IF NOT EXISTS idx_proxies_last_used      ON proxies(last_used_at);

-- ── TABLE: accounts ──────────────────────────────────────────
-- Convert ENUM → TEXT (with USING cast, the key fix vs original)
SELECT _to_text('accounts', 'status');
SELECT _to_text('accounts', 'provider');

-- Rename no-underscore → snake_case  (account.go db:"health_score" etc.)
SELECT _rename_col('accounts', 'healthscore',          'health_score');
SELECT _rename_col('accounts', 'issuspended',          'is_suspended');
SELECT _rename_col('accounts', 'suspendedat',          'suspended_at');
SELECT _rename_col('accounts', 'suspensionreason',     'suspension_reason');
SELECT _rename_col('accounts', 'encryptedpassword',    'encrypted_password');
SELECT _rename_col('accounts', 'oauthtoken',           'oauth_token');
SELECT _rename_col('accounts', 'oauthrefreshtoken',    'oauth_refresh_token');
SELECT _rename_col('accounts', 'oauthexpiry',          'oauth_expiry');
SELECT _rename_col('accounts', 'oauthtokenexpiry',     'oauth_expiry');
ALTER TABLE accounts DROP COLUMN IF EXISTS oauthtokenexpiry;
SELECT _rename_col('accounts', 'smtphost',             'smtp_host');
SELECT _rename_col('accounts', 'smtpport',             'smtp_port');
SELECT _rename_col('accounts', 'smtpusername',         'smtp_username');
SELECT _rename_col('accounts', 'smtpusetls',           'smtp_use_tls');
SELECT _rename_col('accounts', 'smtpusessl',           'smtp_use_ssl');
SELECT _rename_col('accounts', 'dailylimit',           'daily_limit');
SELECT _rename_col('accounts', 'rotationlimit',        'rotation_limit');
SELECT _rename_col('accounts', 'hourlylimit',          'hourly_limit');
-- daily_sent and sent_today are the same; keep sent_today
ALTER TABLE accounts DROP COLUMN IF EXISTS daily_sent;
SELECT _rename_col('accounts', 'senttoday',            'sent_today');
SELECT _rename_col('accounts', 'sentthishour',         'sent_this_hour');
SELECT _rename_col('accounts', 'rotationsent',         'rotation_sent');
SELECT _rename_col('accounts', 'totalsent',            'total_sent');
SELECT _rename_col('accounts', 'totalfailed',          'total_failed');
SELECT _rename_col('accounts', 'successrate',          'success_rate');
SELECT _rename_col('accounts', 'lastusedat',           'last_used_at');
SELECT _rename_col('accounts', 'lastsuccessat',        'last_success_at');
SELECT _rename_col('accounts', 'lastreset',            'last_reset');
SELECT _rename_col('accounts', 'lasthealthcheck',      'last_health_check');
SELECT _rename_col('accounts', 'consecutivefailures',  'consecutive_failures');
SELECT _rename_col('accounts', 'cooldownuntil',        'cooldown_until');
SELECT _rename_col('accounts', 'proxyid',              'proxy_id');
SELECT _rename_col('accounts', 'useproxy',             'use_proxy');
SELECT _rename_col('accounts', 'isactive',             'is_active');
SELECT _rename_col('accounts', 'createdat',            'created_at');
SELECT _rename_col('accounts', 'updatedat',            'updated_at');
SELECT _rename_col('accounts', 'deletedat',            'deleted_at');
SELECT _rename_col('accounts', 'lastfailureat',        'last_failure_at');
SELECT _rename_col('accounts', 'lasterrormessage',     'last_error_message');

-- Add any missing account columns
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS smtp_use_tls       BOOLEAN DEFAULT true;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS smtp_use_ssl       BOOLEAN DEFAULT false;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS smtp_username      VARCHAR(255) DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS oauth_token        TEXT DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS oauth_refresh_token TEXT DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS oauth_expiry       TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS encrypted_password TEXT DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_used_at       TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_success_at    TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_failure_at    TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS total_failed       INTEGER DEFAULT 0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS success_rate       DECIMAL(5,2) DEFAULT 100.0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS cooldown_until     TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS is_active          BOOLEAN DEFAULT true;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS is_suspended       BOOLEAN DEFAULT false;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_error_message TEXT DEFAULT '';

DROP INDEX IF EXISTS idx_accounts_health_score;
DROP INDEX IF EXISTS idx_accounts_healthscore;
DROP INDEX IF EXISTS idx_accounts_last_used_at;
DROP INDEX IF EXISTS idx_accounts_lastusedat;
DROP INDEX IF EXISTS idx_accounts_is_suspended;
DROP INDEX IF EXISTS idx_accounts_issuspended;
DROP INDEX IF EXISTS idx_accounts_isactive;
CREATE INDEX IF NOT EXISTS idx_accounts_health_score  ON accounts(health_score);
CREATE INDEX IF NOT EXISTS idx_accounts_last_used_at  ON accounts(last_used_at);
CREATE INDEX IF NOT EXISTS idx_accounts_is_suspended  ON accounts(is_suspended);
CREATE INDEX IF NOT EXISTS idx_accounts_is_active     ON accounts(is_active);

-- ── TABLE: campaigns ─────────────────────────────────────────
SELECT _to_text('campaigns', 'status');
SELECT _to_text('campaigns', 'rotation_strategy');
SELECT _to_text('campaigns', 'rotationstrategy');

SELECT _rename_col('campaigns', 'sessionid',                 'session_id');
SELECT _rename_col('campaigns', 'totalrecipients',           'total_recipients');
SELECT _rename_col('campaigns', 'sentcount',                 'sent_count');
SELECT _rename_col('campaigns', 'failedcount',               'failed_count');
SELECT _rename_col('campaigns', 'pendingcount',              'pending_count');
-- progress_percentage / progresspercentage / progress → progress (campaign.go uses `progress`)
SELECT _rename_col('campaigns', 'progresspercentage',        'progress');
SELECT _rename_col('campaigns', 'progress_percentage',       'progress');
-- starttime / start_time → started_at  (campaign.go uses started_at)
SELECT _rename_col('campaigns', 'starttime',                 'started_at');
SELECT _rename_col('campaigns', 'start_time',                'started_at');
SELECT _rename_col('campaigns', 'startedat',                 'started_at');
-- endtime / end_time → completed_at  (campaign.go uses completed_at)
SELECT _rename_col('campaigns', 'endtime',                   'completed_at');
SELECT _rename_col('campaigns', 'end_time',                  'completed_at');
SELECT _rename_col('campaigns', 'completedat',               'completed_at');
SELECT _rename_col('campaigns', 'scheduledat',               'scheduled_at');
SELECT _rename_col('campaigns', 'pausedat',                  'paused_at');
SELECT _rename_col('campaigns', 'resumedat',                 'resumed_at');
SELECT _rename_col('campaigns', 'workercount',               'worker_count');
SELECT _rename_col('campaigns', 'batchsize',                 'batch_size');
SELECT _rename_col('campaigns', 'ratelimit',                 'rate_limit');
SELECT _rename_col('campaigns', 'subjecttemplate',           'subject_template');
SELECT _rename_col('campaigns', 'sendername',                'sender_name');
SELECT _rename_col('campaigns', 'replyto',                   'reply_to');
SELECT _rename_col('campaigns', 'usetemplates',              'use_templates');
SELECT _rename_col('campaigns', 'useattachments',            'use_attachments');
SELECT _rename_col('campaigns', 'usepersonalization',        'use_personalization');
SELECT _rename_col('campaigns', 'rotationstrategy',          'rotation_strategy');
SELECT _rename_col('campaigns', 'errormessage',              'error_message');
SELECT _rename_col('campaigns', 'lasterrorat',               'last_error_at');
SELECT _rename_col('campaigns', 'retrycount',                'retry_count');
SELECT _rename_col('campaigns', 'maxretries',                'max_retries');
SELECT _rename_col('campaigns', 'createdby',                 'created_by');
SELECT _rename_col('campaigns', 'createdat',                 'created_at');
SELECT _rename_col('campaigns', 'updatedat',                 'updated_at');
SELECT _rename_col('campaigns', 'deletedat',                 'deleted_at');
-- Rotation columns (added by migration 000004)
SELECT _rename_col('campaigns', 'sendernamerotationenabled', 'sender_name_rotation_enabled');
SELECT _rename_col('campaigns', 'subjectrotationenabled',    'subject_rotation_enabled');
SELECT _rename_col('campaigns', 'templaterotationenabled',   'template_rotation_enabled');
SELECT _rename_col('campaigns', 'attachmentrotationenabled', 'attachment_rotation_enabled');
SELECT _rename_col('campaigns', 'customfieldrotationenabled','custom_field_rotation_enabled');
-- Extended columns (added/renamed by migration 000009)
SELECT _rename_col('campaigns', 'settings',                  'config');
SELECT _rename_col('campaigns', 'updatedby',                 'updated_by');
SELECT _rename_col('campaigns', 'isarchived',                'is_archived');
SELECT _rename_col('campaigns', 'archivedat',                'archived_at');
SELECT _rename_col('campaigns', 'lastcheckpoint',            'last_checkpoint');
SELECT _rename_col('campaigns', 'checkpointdata',            'checkpoint_data');
SELECT _rename_col('campaigns', 'failedat',                  'failed_at');
SELECT _rename_col('campaigns', 'successrate',               'success_rate');
SELECT _rename_col('campaigns', 'estimatedeta',              'estimated_eta');
SELECT _rename_col('campaigns', 'templateids',               'template_ids');
SELECT _rename_col('campaigns', 'accountids',                'account_ids');
SELECT _rename_col('campaigns', 'recipientlistid',           'recipient_list_id');
SELECT _rename_col('campaigns', 'proxyids',                  'proxy_ids');
SELECT _rename_col('campaigns', 'throughput',                'throughput');  -- same
SELECT _rename_col('campaigns', 'isarchived',                'is_archived');

-- Add back columns that migration 000009 accidentally dropped
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS state           VARCHAR(50)   DEFAULT 'idle';
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS priority        INTEGER       DEFAULT 0;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS failed_at       TIMESTAMP;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS started_at      TIMESTAMP;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS completed_at    TIMESTAMP;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS success_rate    DECIMAL(5,2)  DEFAULT 0.00;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS progress        DECIMAL(5,2)  DEFAULT 0.00;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS throughput      DECIMAL(10,2) DEFAULT 0.00;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS estimated_eta   TIMESTAMP;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS config          JSONB         DEFAULT '{}';
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS template_ids    UUID[]        DEFAULT '{}';
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS account_ids     UUID[]        DEFAULT '{}';
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS recipient_list_id UUID;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS proxy_ids       UUID[]        DEFAULT '{}';
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS tags            TEXT[]        DEFAULT '{}';
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS last_checkpoint TIMESTAMP;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS checkpoint_data JSONB         DEFAULT '{}';
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS updated_by      VARCHAR(100)  DEFAULT '';
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS is_archived     BOOLEAN       DEFAULT false;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS archived_at     TIMESTAMP;

-- Back-fill config from settings if config is empty
UPDATE campaigns
SET config = settings
WHERE (config IS NULL OR config = '{}')
  AND settings IS NOT NULL
  AND settings::text != '{}';

DROP INDEX IF EXISTS idx_campaigns_sessionid;
DROP INDEX IF EXISTS idx_campaigns_createdat;
DROP INDEX IF EXISTS idx_campaigns_scheduledat;
DROP INDEX IF EXISTS idx_campaigns_isarchived;
CREATE INDEX IF NOT EXISTS idx_campaigns_session_id    ON campaigns(session_id);
CREATE INDEX IF NOT EXISTS idx_campaigns_created_at    ON campaigns(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaigns_scheduled_at  ON campaigns(scheduled_at) WHERE scheduled_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_campaigns_is_archived   ON campaigns(is_archived);
CREATE INDEX IF NOT EXISTS idx_campaigns_state         ON campaigns(state);

-- ── TABLE: recipients ────────────────────────────────────────
SELECT _to_text('recipients', 'status');

-- campaign_id → list_id  (the FK was renamed to list_id in the new code)
SELECT _rename_col('recipients', 'campaignid',       'list_id');
SELECT _rename_col('recipients', 'campaign_id',      'list_id');
SELECT _rename_col('recipients', 'listid',           'list_id');
SELECT _rename_col('recipients', 'firstname',        'first_name');
SELECT _rename_col('recipients', 'lastname',         'last_name');
SELECT _rename_col('recipients', 'customfields',     'custom_fields');
SELECT _rename_col('recipients', 'sentat',           'sent_at');
SELECT _rename_col('recipients', 'failedat',         'failed_at');
SELECT _rename_col('recipients', 'bouncedat',        'bounced_at');
SELECT _rename_col('recipients', 'accountid',        'account_id');
SELECT _rename_col('recipients', 'templateid',       'template_id');
SELECT _rename_col('recipients', 'retrycount',       'retry_count');
SELECT _rename_col('recipients', 'maxretries',       'max_retries');
SELECT _rename_col('recipients', 'errormessage',     'error_message');
SELECT _rename_col('recipients', 'errorcode',        'error_code');
SELECT _rename_col('recipients', 'messageid',        'message_id');
SELECT _rename_col('recipients', 'openedat',         'opened_at');
SELECT _rename_col('recipients', 'clickedat',        'clicked_at');
SELECT _rename_col('recipients', 'unsubscribedat',   'unsubscribed_at');
SELECT _rename_col('recipients', 'createdat',        'created_at');
SELECT _rename_col('recipients', 'updatedat',        'updated_at');
SELECT _rename_col('recipients', 'sendernameused',   'sender_used');
SELECT _rename_col('recipients', 'subjectused',      'subject_used');
SELECT _rename_col('recipients', 'customfieldsused', 'custom_fields_used');

DROP INDEX IF EXISTS idx_recipients_campaign_id;
DROP INDEX IF EXISTS idx_recipients_campaignid;
DROP INDEX IF EXISTS idx_recipients_account_id;
DROP INDEX IF EXISTS idx_recipients_accountid;
CREATE INDEX IF NOT EXISTS idx_recipients_list_id   ON recipients(list_id);
CREATE INDEX IF NOT EXISTS idx_recipients_account_id ON recipients(account_id);

-- ── TABLE: templates ─────────────────────────────────────────
SELECT _rename_col('templates', 'spamscore',        'spam_score');
SELECT _rename_col('templates', 'spamcheckresult',  'spam_check_result');
SELECT _rename_col('templates', 'spamcheckedat',    'spam_checked_at');
SELECT _rename_col('templates', 'templatetype',     'template_type');
SELECT _rename_col('templates', 'parenttemplateid', 'parent_template_id');
SELECT _rename_col('templates', 'plaintext',        'plain_text');
SELECT _rename_col('templates', 'isactive',         'is_active');
SELECT _rename_col('templates', 'isdefault',        'is_default');
SELECT _rename_col('templates', 'usagecount',       'usage_count');
SELECT _rename_col('templates', 'lastusedat',       'last_used_at');
SELECT _rename_col('templates', 'createdby',        'created_by');
SELECT _rename_col('templates', 'createdat',        'created_at');
SELECT _rename_col('templates', 'updatedat',        'updated_at');
SELECT _rename_col('templates', 'deletedat',        'deleted_at');

DROP INDEX IF EXISTS idx_templates_isactive;
DROP INDEX IF EXISTS idx_templates_spamscore;
CREATE INDEX IF NOT EXISTS idx_templates_is_active  ON templates(is_active);
CREATE INDEX IF NOT EXISTS idx_templates_spam_score ON templates(spam_score);

-- ── TABLE: logs (CREATE — log.go uses no-underscore names) ───
CREATE TABLE IF NOT EXISTS logs (
    id            UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    time          TIMESTAMP    NOT NULL DEFAULT NOW(),
    level         VARCHAR(20)  NOT NULL DEFAULT 'info',
    category      VARCHAR(50),
    sessionid     VARCHAR(100),
    campaignid    VARCHAR(100),
    accountid     VARCHAR(100),
    recipientid   VARCHAR(100),
    proxyid       VARCHAR(100),
    templateid    VARCHAR(100),
    message       TEXT         NOT NULL DEFAULT '',
    details       JSONB        DEFAULT '{}',
    errorcode     VARCHAR(50),
    errorclass    VARCHAR(100),
    stacktrace    TEXT,
    requestid     VARCHAR(100),
    traceid       VARCHAR(100),
    spanid        VARCHAR(100),
    httpmethod    VARCHAR(10),
    httppath      TEXT,
    httpstatus    INTEGER,
    durationms    BIGINT,
    clientip      VARCHAR(45),
    useragent     TEXT,
    nodeid        VARCHAR(100),
    hostname      VARCHAR(255),
    environment   VARCHAR(50),
    shard         VARCHAR(50),
    metricname    VARCHAR(255),
    metricvalue   DOUBLE PRECISION,
    metricunit    VARCHAR(50),
    metriclabels  JSONB        DEFAULT '{}',
    userid        VARCHAR(100),
    username      VARCHAR(255),
    tenantid      VARCHAR(100),
    source        VARCHAR(100),
    subsystem     VARCHAR(100),
    component     VARCHAR(100),
    version       VARCHAR(50),
    correlationid VARCHAR(100),
    archived      BOOLEAN      DEFAULT false,
    createdat     TIMESTAMP    NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_logs_time       ON logs(time DESC);
CREATE INDEX IF NOT EXISTS idx_logs_level      ON logs(level);
CREATE INDEX IF NOT EXISTS idx_logs_campaignid ON logs(campaignid);
CREATE INDEX IF NOT EXISTS idx_logs_archived   ON logs(archived) WHERE archived = false;
CREATE INDEX IF NOT EXISTS idx_logs_createdat  ON logs(createdat DESC);

-- ── Rename log columns if they were previously snake_case ────
SELECT _rename_col('logs', 'session_id',    'sessionid');
SELECT _rename_col('logs', 'campaign_id',   'campaignid');
SELECT _rename_col('logs', 'account_id',    'accountid');
SELECT _rename_col('logs', 'recipient_id',  'recipientid');
SELECT _rename_col('logs', 'proxy_id',      'proxyid');
SELECT _rename_col('logs', 'template_id',   'templateid');
SELECT _rename_col('logs', 'error_code',    'errorcode');
SELECT _rename_col('logs', 'error_class',   'errorclass');
SELECT _rename_col('logs', 'stack_trace',   'stacktrace');
SELECT _rename_col('logs', 'request_id',    'requestid');
SELECT _rename_col('logs', 'trace_id',      'traceid');
SELECT _rename_col('logs', 'span_id',       'spanid');
SELECT _rename_col('logs', 'http_method',   'httpmethod');
SELECT _rename_col('logs', 'http_path',     'httppath');
SELECT _rename_col('logs', 'http_status',   'httpstatus');
SELECT _rename_col('logs', 'duration_ms',   'durationms');
SELECT _rename_col('logs', 'client_ip',     'clientip');
SELECT _rename_col('logs', 'user_agent',    'useragent');
SELECT _rename_col('logs', 'node_id',       'nodeid');
SELECT _rename_col('logs', 'metric_name',   'metricname');
SELECT _rename_col('logs', 'metric_value',  'metricvalue');
SELECT _rename_col('logs', 'metric_unit',   'metricunit');
SELECT _rename_col('logs', 'metric_labels', 'metriclabels');
SELECT _rename_col('logs', 'user_id',       'userid');
SELECT _rename_col('logs', 'tenant_id',     'tenantid');
SELECT _rename_col('logs', 'correlation_id','correlationid');
SELECT _rename_col('logs', 'created_at',    'createdat');

-- ── Cleanup ──────────────────────────────────────────────────
DROP FUNCTION IF EXISTS _rename_col(text,text,text);
DROP FUNCTION IF EXISTS _to_text(text,text);
