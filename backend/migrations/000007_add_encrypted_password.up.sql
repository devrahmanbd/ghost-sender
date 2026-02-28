-- Add encrypted_password column and handle NULL values

-- Add missing columns
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS encrypted_password TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS smtp_use_tls BOOLEAN DEFAULT TRUE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS smtp_use_ssl BOOLEAN DEFAULT FALSE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS smtp_username VARCHAR(255);
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS oauth_token TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS oauth_refresh_token TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS oauth_expiry TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_success_at TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS last_failure_at TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS total_failed INTEGER DEFAULT 0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS success_rate DECIMAL(5,2) DEFAULT 100.0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS cooldown_until TIMESTAMP;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE;

-- Convert NULL values to empty strings for text columns
UPDATE accounts SET encrypted_password = '' WHERE encrypted_password IS NULL;
UPDATE accounts SET smtp_username = '' WHERE smtp_username IS NULL;
UPDATE accounts SET oauth_token = '' WHERE oauth_token IS NULL;
UPDATE accounts SET oauth_refresh_token = '' WHERE oauth_refresh_token IS NULL;
UPDATE accounts SET suspension_reason = '' WHERE suspension_reason IS NULL;

-- Set default values for text columns
ALTER TABLE accounts 
  ALTER COLUMN encrypted_password SET DEFAULT '',
  ALTER COLUMN smtp_username SET DEFAULT '',
  ALTER COLUMN oauth_token SET DEFAULT '',
  ALTER COLUMN oauth_refresh_token SET DEFAULT '',
  ALTER COLUMN suspension_reason SET DEFAULT '';
