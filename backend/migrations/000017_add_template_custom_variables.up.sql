-- Add custom_variables column to templates table
ALTER TABLE templates
ADD COLUMN IF NOT EXISTS custom_variables JSONB DEFAULT '{}';

-- Create index for faster queries
CREATE INDEX IF NOT EXISTS idx_templates_custom_variables ON templates USING GIN(custom_variables);
