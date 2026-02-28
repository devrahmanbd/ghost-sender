-- =============================================================================
-- 000012_add_stats_tables.up.sql
-- Creates the six analytics/reporting tables required by stats.go, plus the
-- two missing boolean columns on recipients used by GetGlobalStats.emailQuery.
-- =============================================================================

-- ─── 1. campaign_recipient_stats ─────────────────────────────────────────────
-- Used by: GetCampaignStats
-- Query:   SELECT COUNT(*), SUM(sent::int), SUM(delivered::int), ...
--          FROM campaign_recipient_stats
--          WHERE campaign_id = $1 AND time >= $2 AND time <= $3
-- Note:    Column is named "time" (not created_at) to match the WHERE clause.
-- Note:    "complaint" is singular – the query aliases SUM to "complaints".
-- Note:    "unsubscribed" is a BOOLEAN here (different from recipients.unsubscribed
--          which is also BOOLEAN, but used elsewhere as a flag, not cast to int).
-- =============================================================================
CREATE TABLE IF NOT EXISTS campaign_recipient_stats (
    id              UUID          PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id     UUID          NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    recipient_id    UUID          REFERENCES recipients(id) ON DELETE SET NULL,
    recipient_email VARCHAR(255)  NOT NULL DEFAULT '',

    -- Timestamp used directly in WHERE time >= $2 AND time <= $3
    time            TIMESTAMP     NOT NULL DEFAULT NOW(),

    -- Boolean event flags; queries cast these to int via ::int for SUM
    sent            BOOLEAN       NOT NULL DEFAULT FALSE,
    delivered       BOOLEAN       NOT NULL DEFAULT FALSE,
    failed          BOOLEAN       NOT NULL DEFAULT FALSE,
    hard_bounced    BOOLEAN       NOT NULL DEFAULT FALSE,
    soft_bounced    BOOLEAN       NOT NULL DEFAULT FALSE,
    complaint       BOOLEAN       NOT NULL DEFAULT FALSE,
    unsubscribed    BOOLEAN       NOT NULL DEFAULT FALSE,

    -- Integer counters; queried with SUM(opens) – no cast needed
    opens           INTEGER       NOT NULL DEFAULT 0,
    unique_opens    INTEGER       NOT NULL DEFAULT 0,
    clicks          INTEGER       NOT NULL DEFAULT 0,
    unique_clicks   INTEGER       NOT NULL DEFAULT 0,

    metadata        JSONB         NOT NULL DEFAULT '{}',
    created_at      TIMESTAMP     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_crs_campaign_id
    ON campaign_recipient_stats(campaign_id);
CREATE INDEX IF NOT EXISTS idx_crs_time
    ON campaign_recipient_stats(time DESC);
CREATE INDEX IF NOT EXISTS idx_crs_campaign_time
    ON campaign_recipient_stats(campaign_id, time DESC);


-- ─── 2. campaign_delivery_metrics ────────────────────────────────────────────
-- Used by: GetCampaignStats (latencyQuery, throughputQuery)
-- latencyQuery:    SELECT AVG(latency_ms), percentile_disc(0.95/0.99)
--                  FROM campaign_delivery_metrics
--                  WHERE campaign_id=$1 AND created_at>=$2 AND created_at<=$3
--                  AND latency_ms > 0
-- throughputQuery: SELECT MAX(sent_per_minute)
--                  FROM campaign_delivery_metrics
--                  WHERE campaign_id=$1 AND created_at>=$2 AND created_at<=$3
-- =============================================================================
CREATE TABLE IF NOT EXISTS campaign_delivery_metrics (
    id              UUID          PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id     UUID          NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    account_id      UUID          REFERENCES accounts(id) ON DELETE SET NULL,

    -- Milliseconds for one send attempt; used in AVG + percentile_disc
    latency_ms      DECIMAL(10,2) NOT NULL DEFAULT 0,

    -- Throughput snapshot: emails dispatched in the current one-minute window
    sent_per_minute DECIMAL(10,2) NOT NULL DEFAULT 0,

    metadata        JSONB         NOT NULL DEFAULT '{}',
    created_at      TIMESTAMP     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cdm_campaign_id
    ON campaign_delivery_metrics(campaign_id);
CREATE INDEX IF NOT EXISTS idx_cdm_created_at
    ON campaign_delivery_metrics(created_at DESC);
-- Partial index: latency queries always filter latency_ms > 0
CREATE INDEX IF NOT EXISTS idx_cdm_latency_nonzero
    ON campaign_delivery_metrics(latency_ms)
    WHERE latency_ms > 0;


-- ─── 3. account_delivery_stats ───────────────────────────────────────────────
-- Used by: GetAccountStats
-- Query:   SELECT provider, SUM(sent::int), SUM(delivered::int), ...,
--                 AVG(spam_score), percentile_disc(0.95/0.99) WITHIN GROUP ...,
--                 AVG(health_score), AVG(latency_ms)
--          FROM account_delivery_stats
--          WHERE account_id=$1 AND created_at>=$2 AND created_at<=$3
--          GROUP BY provider
-- Note:    percentile_disc on spam_score requires the column to be sortable –
--          DECIMAL(5,2) satisfies this.
-- =============================================================================
CREATE TABLE IF NOT EXISTS account_delivery_stats (
    id           UUID          PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id   UUID          NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,

    -- Denormalised so GROUP BY provider requires no JOIN back to accounts
    provider     VARCHAR(50)   NOT NULL DEFAULT '',

    -- Boolean event flags
    sent         BOOLEAN       NOT NULL DEFAULT FALSE,
    delivered    BOOLEAN       NOT NULL DEFAULT FALSE,
    failed       BOOLEAN       NOT NULL DEFAULT FALSE,
    hard_bounced BOOLEAN       NOT NULL DEFAULT FALSE,
    soft_bounced BOOLEAN       NOT NULL DEFAULT FALSE,
    complaint    BOOLEAN       NOT NULL DEFAULT FALSE,
    unsubscribed BOOLEAN       NOT NULL DEFAULT FALSE,

    -- Snapshot scores captured at send time
    spam_score   DECIMAL(5,2)  NOT NULL DEFAULT 0.00,
    health_score DECIMAL(3,2)  NOT NULL DEFAULT 1.00,

    -- Round-trip latency for this send attempt
    latency_ms   DECIMAL(10,2) NOT NULL DEFAULT 0,

    metadata     JSONB         NOT NULL DEFAULT '{}',
    created_at   TIMESTAMP     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ads_account_id
    ON account_delivery_stats(account_id);
CREATE INDEX IF NOT EXISTS idx_ads_created_at
    ON account_delivery_stats(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ads_account_created
    ON account_delivery_stats(account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ads_provider
    ON account_delivery_stats(provider);


-- ─── 4. account_limits  (VIEW – no new data storage needed) ──────────────────
-- Used by: GetAccountStats
-- Query:   SELECT daily_limit, remaining_daily
--          FROM account_limits WHERE account_id = $1
-- Both values already live on accounts; a view keeps them always consistent.
-- "remaining_daily" = daily_limit − sent_today, clamped to 0.
-- =============================================================================
CREATE OR REPLACE VIEW account_limits AS
SELECT
    id                                           AS account_id,
    daily_limit,
    GREATEST(daily_limit - sent_today, 0)        AS remaining_daily
FROM accounts;


-- ─── 5. account_rotation_metrics ─────────────────────────────────────────────
-- Used by: GetAccountStats
-- Query:   SELECT COALESCE(AVG(rotation_index), 0)
--          FROM account_rotation_metrics
--          WHERE account_id=$1 AND created_at>=$2 AND created_at<=$3
-- rotation_index: 0-based ordinal position of this account in the rotation
--                 sequence at the time it was selected.
-- =============================================================================
CREATE TABLE IF NOT EXISTS account_rotation_metrics (
    id             UUID      PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id     UUID      NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    campaign_id    UUID      REFERENCES campaigns(id) ON DELETE SET NULL,

    -- Ordinal slot used when this account was rotated into service
    rotation_index INTEGER   NOT NULL DEFAULT 0,

    metadata       JSONB     NOT NULL DEFAULT '{}',
    created_at     TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_arm_account_id
    ON account_rotation_metrics(account_id);
CREATE INDEX IF NOT EXISTS idx_arm_created_at
    ON account_rotation_metrics(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_arm_account_created
    ON account_rotation_metrics(account_id, created_at DESC);


-- ─── 6. delivery_time_series ─────────────────────────────────────────────────
-- Used by: GetTimeSeriesStats
-- Query:   SELECT date_trunc($1, created_at) AS bucket,
--                 SUM(sent::int), SUM(delivered::int), SUM(failed::int),
--                 SUM(hard_bounced::int), SUM(soft_bounced::int),
--                 SUM(complaint::int), SUM(unsubscribed::int),
--                 SUM(opens), SUM(clicks)
--          FROM delivery_time_series
--          WHERE created_at >= $2 AND created_at <= $3
--          [AND campaign_id = $N]  [AND account_id = $M]
--          GROUP BY bucket ORDER BY bucket ASC
-- Note:    campaign_id / account_id are NULLABLE – rows without them represent
--          system-wide events (the Go code only adds these filters when the
--          StatsFilter fields are non-empty).
-- =============================================================================
CREATE TABLE IF NOT EXISTS delivery_time_series (
    id           UUID      PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Nullable foreign keys; absent = system-wide aggregate row
    campaign_id  UUID      REFERENCES campaigns(id) ON DELETE SET NULL,
    account_id   UUID      REFERENCES accounts(id)  ON DELETE SET NULL,

    -- Boolean event flags; queries cast to int via ::int for SUM
    sent         BOOLEAN   NOT NULL DEFAULT FALSE,
    delivered    BOOLEAN   NOT NULL DEFAULT FALSE,
    failed       BOOLEAN   NOT NULL DEFAULT FALSE,
    hard_bounced BOOLEAN   NOT NULL DEFAULT FALSE,
    soft_bounced BOOLEAN   NOT NULL DEFAULT FALSE,
    complaint    BOOLEAN   NOT NULL DEFAULT FALSE,
    unsubscribed BOOLEAN   NOT NULL DEFAULT FALSE,

    -- Integer counters; queried with SUM(opens) / SUM(clicks) – no cast
    opens        INTEGER   NOT NULL DEFAULT 0,
    clicks       INTEGER   NOT NULL DEFAULT 0,

    metadata     JSONB     NOT NULL DEFAULT '{}',
    created_at   TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Primary lookup index for the time-window WHERE clause
CREATE INDEX IF NOT EXISTS idx_dts_created_at
    ON delivery_time_series(created_at DESC);

-- Partial indexes accelerate the optional campaign_id / account_id filters
CREATE INDEX IF NOT EXISTS idx_dts_campaign_id
    ON delivery_time_series(campaign_id)
    WHERE campaign_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_dts_account_id
    ON delivery_time_series(account_id)
    WHERE account_id IS NOT NULL;

-- Composite for the common (bucket + campaign) path
CREATE INDEX IF NOT EXISTS idx_dts_created_campaign
    ON delivery_time_series(created_at, campaign_id);


-- ─── 7. recipients — add missing boolean columns for stats.go emailQuery ──────
-- GetGlobalStats runs:
--   SELECT COALESCE(SUM(sent::int), 0), COALESCE(SUM(failed::int), 0), ...
--   FROM recipients
-- The RecipientRepository struct has SentCount/FailedCount (INTEGER counters)
-- but not `sent` / `failed` boolean flags. The emailQuery needs the flags.
-- `hard_bounced`, `soft_bounced`, `opens`, `clicks` already exist on recipients
-- (added by earlier migrations used by RecipientRepository.Create/Update).
-- =============================================================================
ALTER TABLE recipients
    ADD COLUMN IF NOT EXISTS sent    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS failed  BOOLEAN NOT NULL DEFAULT FALSE;

-- Back-fill from the existing timestamp sentinels so historical rows are
-- correct without a full table rewrite on large deployments.
UPDATE recipients SET sent   = TRUE WHERE sent_at   IS NOT NULL AND sent   = FALSE;
UPDATE recipients SET failed = TRUE WHERE failed_at  IS NOT NULL AND failed = FALSE;
