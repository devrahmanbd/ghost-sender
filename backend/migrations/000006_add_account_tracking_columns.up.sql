-- Migration: Add missing columns to accounts table

ALTER TABLE accounts 
ADD COLUMN IF NOT EXISTS is_suspended BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS suspension_reason TEXT,
ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMP,
ADD COLUMN IF NOT EXISTS consecutive_failures INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS health_score FLOAT DEFAULT 100.0,
ADD COLUMN IF NOT EXISTS last_health_check TIMESTAMP,
ADD COLUMN IF NOT EXISTS daily_sent INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS rotation_sent INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_reset TIMESTAMP DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_accounts_is_suspended ON accounts(is_suspended);
CREATE INDEX IF NOT EXISTS idx_accounts_status ON accounts(status);
