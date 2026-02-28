-- ============================================================
-- Fix proxies table: align column names with repository queries
-- ============================================================

-- Step 1: Convert enum columns to TEXT so they work as plain strings
ALTER TABLE proxies ALTER COLUMN proxy_type TYPE TEXT;
ALTER TABLE proxies ALTER COLUMN status     TYPE TEXT;

-- Step 2: Rename mismatched existing columns
ALTER TABLE proxies RENAME COLUMN proxy_type           TO type;
ALTER TABLE proxies RENAME COLUMN is_enabled           TO isactive;
ALTER TABLE proxies RENAME COLUMN health_score         TO healthscore;
ALTER TABLE proxies RENAME COLUMN avg_latency_ms       TO latencyms;
ALTER TABLE proxies RENAME COLUMN successful_requests  TO successcount;
ALTER TABLE proxies RENAME COLUMN failed_requests      TO failurecount;
ALTER TABLE proxies RENAME COLUMN consecutive_failures TO consecutivefails;
ALTER TABLE proxies RENAME COLUMN last_failure_at      TO last_error_at;
ALTER TABLE proxies RENAME COLUMN created_at           TO createdat;
ALTER TABLE proxies RENAME COLUMN updated_at           TO updatedat;
ALTER TABLE proxies RENAME COLUMN deleted_at           TO deletedat;

-- Step 3: Add columns that are entirely missing
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS region          VARCHAR(100);
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS provider        VARCHAR(100);
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS inuse           BOOLEAN        DEFAULT false;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS assignedaccounts INTEGER       DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS maxaccounts     INTEGER        DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS rotationgroup   VARCHAR(255);
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS maxconnections  INTEGER        DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS currentconns    INTEGER        DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS bandwidthmb     FLOAT          DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS bandwidthlimitmb FLOAT         DEFAULT 0;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS lasterror       TEXT;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS tags            TEXT[];
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS rotationweight  INTEGER        DEFAULT 1;
ALTER TABLE proxies ADD COLUMN IF NOT EXISTS last_healthy_at   TIMESTAMPTZ;

-- Step 4: Migrate data from old columns into new ones where semantics overlap
UPDATE proxies SET region = country     WHERE region IS NULL AND country IS NOT NULL;
UPDATE proxies SET lasterror = suspension_reason WHERE lasterror IS NULL;
