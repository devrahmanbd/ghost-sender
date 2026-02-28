-- Fix templates table - add missing columns and fix NULLs

-- Add missing columns
ALTER TABLE templates ADD COLUMN IF NOT EXISTS is_archived BOOLEAN DEFAULT FALSE;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS html_content TEXT;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS plain_text TEXT DEFAULT '';

-- Fix NULL values
UPDATE templates SET html_content = content WHERE html_content IS NULL AND content IS NOT NULL;
UPDATE templates SET html_content = '' WHERE html_content IS NULL;
UPDATE templates SET plain_text = '' WHERE plain_text IS NULL;

-- Set defaults
ALTER TABLE templates 
  ALTER COLUMN html_content SET DEFAULT '',
  ALTER COLUMN plain_text SET DEFAULT '';

-- Clean up empty/broken templates
DELETE FROM templates WHERE content = '' OR name IS NULL OR name = '';

-- Add indexes
CREATE INDEX IF NOT EXISTS idx_templates_is_archived ON templates(is_archived);

-- Template Manager will now work!
-- Complete templates table fix based on repository code analysis

-- 1. Add ALL missing columns the TemplateRepository expects
ALTER TABLE templates ADD COLUMN IF NOT EXISTS slug VARCHAR(255);
ALTER TABLE templates ADD COLUMN IF NOT EXISTS language VARCHAR(50) DEFAULT 'en';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS type VARCHAR(50) DEFAULT 'html';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS from_name VARCHAR(255) DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS from_email VARCHAR(255) DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS reply_to VARCHAR(255) DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS html_content TEXT DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS text_content TEXT DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS preheader VARCHAR(500) DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS variables TEXT[] DEFAULT '{}';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS custom_headers JSONB DEFAULT '{}';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS attachments TEXT[] DEFAULT '{}';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS tags TEXT[] DEFAULT '{}';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS spam_details JSONB DEFAULT '{}';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS last_spam_check_at TIMESTAMP;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS render_count BIGINT DEFAULT 0;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS last_rendered_at TIMESTAMP;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS failure_count BIGINT DEFAULT 0;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS last_failure_at TIMESTAMP;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS last_failure_msg TEXT DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS engine VARCHAR(50) DEFAULT 'go';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rendering_config JSONB DEFAULT '{}';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rotation_group VARCHAR(255) DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rotation_weight INTEGER DEFAULT 1;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rotation_index INTEGER DEFAULT 0;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS is_archived BOOLEAN DEFAULT FALSE;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS updated_by VARCHAR(100) DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS success_rate DECIMAL(5,2) DEFAULT 0.00;

-- 2. Generate slugs for existing templates (slug = lowercase name with hyphens)
UPDATE templates 
SET slug = LOWER(REGEXP_REPLACE(name, '[^a-zA-Z0-9]+', '-', 'g'))
WHERE slug IS NULL OR slug = '';

-- 3. Make slug unique (required by GetBySlug)
ALTER TABLE templates ADD CONSTRAINT templates_slug_unique UNIQUE (slug);

-- 4. Copy existing content to html_content if empty
UPDATE templates SET html_content = content WHERE html_content = '' AND content IS NOT NULL;
UPDATE templates SET html_content = '' WHERE html_content IS NULL;

-- 5. Fix all NULL text values
UPDATE templates SET text_content = '' WHERE text_content IS NULL;
UPDATE templates SET preheader = '' WHERE preheader IS NULL;
UPDATE templates SET from_name = '' WHERE from_name IS NULL;
UPDATE templates SET from_email = '' WHERE from_email IS NULL;
UPDATE templates SET reply_to = '' WHERE reply_to IS NULL;
UPDATE templates SET last_failure_msg = '' WHERE last_failure_msg IS NULL;
UPDATE templates SET engine = 'go' WHERE engine IS NULL;
UPDATE templates SET updated_by = '' WHERE updated_by IS NULL;
UPDATE templates SET rotation_group = '' WHERE rotation_group IS NULL;
UPDATE templates SET language = 'en' WHERE language IS NULL;

-- 6. Fix NULL integers
UPDATE templates SET render_count = 0 WHERE render_count IS NULL;
UPDATE templates SET failure_count = 0 WHERE failure_count IS NULL;
UPDATE templates SET rotation_weight = 1 WHERE rotation_weight IS NULL;
UPDATE templates SET rotation_index = 0 WHERE rotation_index IS NULL;
UPDATE templates SET is_archived = FALSE WHERE is_archived IS NULL;

-- 7. Set defaults to prevent future NULLs
ALTER TABLE templates
  ALTER COLUMN slug SET DEFAULT '',
  ALTER COLUMN language SET DEFAULT 'en',
  ALTER COLUMN type SET DEFAULT 'html',
  ALTER COLUMN from_name SET DEFAULT '',
  ALTER COLUMN from_email SET DEFAULT '',
  ALTER COLUMN reply_to SET DEFAULT '',
  ALTER COLUMN html_content SET DEFAULT '',
  ALTER COLUMN text_content SET DEFAULT '',
  ALTER COLUMN preheader SET DEFAULT '',
  ALTER COLUMN last_failure_msg SET DEFAULT '',
  ALTER COLUMN engine SET DEFAULT 'go',
  ALTER COLUMN rotation_group SET DEFAULT '',
  ALTER COLUMN updated_by SET DEFAULT '',
  ALTER COLUMN render_count SET DEFAULT 0,
  ALTER COLUMN failure_count SET DEFAULT 0,
  ALTER COLUMN rotation_weight SET DEFAULT 1,
  ALTER COLUMN rotation_index SET DEFAULT 0,
  ALTER COLUMN success_rate SET DEFAULT 0.00;

-- 8. Add indexes
CREATE INDEX IF NOT EXISTS idx_templates_slug ON templates(slug);
CREATE INDEX IF NOT EXISTS idx_templates_is_archived ON templates(is_archived);
CREATE INDEX IF NOT EXISTS idx_templates_rotation_group ON templates(rotation_group) WHERE rotation_group != '';
CREATE INDEX IF NOT EXISTS idx_templates_language ON templates(language);
CREATE INDEX IF NOT EXISTS idx_templates_type ON templates(type);
