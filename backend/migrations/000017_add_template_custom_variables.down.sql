-- Drop custom_variables column from templates table
ALTER TABLE templates DROP COLUMN IF EXISTS custom_variables;

-- Drop index
DROP INDEX IF EXISTS idx_templates_custom_variables;
