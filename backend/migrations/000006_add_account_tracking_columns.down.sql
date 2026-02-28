-- Rollback: Remove account tracking columns

DROP INDEX IF EXISTS idx_accounts_status;
DROP INDEX IF EXISTS idx_accounts_is_suspended;

ALTER TABLE accounts 
DROP COLUMN IF EXISTS last_reset,
DROP COLUMN IF EXISTS rotation_sent,
DROP COLUMN IF EXISTS daily_sent,
DROP COLUMN IF EXISTS last_health_check,
DROP COLUMN IF EXISTS health_score,
DROP COLUMN IF EXISTS consecutive_failures,
DROP COLUMN IF EXISTS suspended_at,
DROP COLUMN IF EXISTS suspension_reason,
DROP COLUMN IF EXISTS is_suspended;
