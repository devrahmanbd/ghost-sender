CREATE TABLE sender_name_rotation (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    rotation_strategy rotation_strategy DEFAULT 'round_robin',
    
    sender_names JSONB DEFAULT '[]',
    
    current_index INTEGER DEFAULT 0,
    total_names INTEGER DEFAULT 0,
    
    rotation_count INTEGER DEFAULT 0,
    
    weights JSONB DEFAULT '{}',
    
    time_based_rules JSONB DEFAULT '{}',
    
    usage_stats JSONB DEFAULT '{}',
    
    last_rotated_at TIMESTAMP,
    last_used_name VARCHAR(255),
    
    is_active BOOLEAN DEFAULT true,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(campaign_id)
);

CREATE TABLE subject_rotation (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    rotation_strategy rotation_strategy DEFAULT 'round_robin',
    
    subject_lines JSONB DEFAULT '[]',
    
    current_index INTEGER DEFAULT 0,
    total_subjects INTEGER DEFAULT 0,
    
    rotation_count INTEGER DEFAULT 0,
    
    weights JSONB DEFAULT '{}',
    
    time_based_rules JSONB DEFAULT '{}',
    
    usage_stats JSONB DEFAULT '{}',
    
    last_rotated_at TIMESTAMP,
    last_used_subject TEXT,
    
    is_active BOOLEAN DEFAULT true,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(campaign_id)
);

CREATE TABLE custom_field_rotation (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    field_name VARCHAR(255) NOT NULL,
    
    rotation_strategy rotation_strategy DEFAULT 'round_robin',
    
    field_values JSONB DEFAULT '[]',
    
    current_index INTEGER DEFAULT 0,
    total_values INTEGER DEFAULT 0,
    
    rotation_count INTEGER DEFAULT 0,
    
    weights JSONB DEFAULT '{}',
    
    time_based_rules JSONB DEFAULT '{}',
    
    usage_stats JSONB DEFAULT '{}',
    
    last_rotated_at TIMESTAMP,
    last_used_value TEXT,
    
    is_active BOOLEAN DEFAULT true,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(campaign_id, field_name)
);

CREATE TABLE template_rotation_state (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    rotation_strategy rotation_strategy DEFAULT 'round_robin',
    
    template_ids UUID[] DEFAULT '{}',
    
    current_index INTEGER DEFAULT 0,
    total_templates INTEGER DEFAULT 0,
    
    rotation_count INTEGER DEFAULT 0,
    
    weights JSONB DEFAULT '{}',
    
    usage_stats JSONB DEFAULT '{}',
    
    last_rotated_at TIMESTAMP,
    last_used_template_id UUID REFERENCES templates(id) ON DELETE SET NULL,
    
    is_active BOOLEAN DEFAULT true,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(campaign_id)
);

CREATE TABLE attachment_rotation_state (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    rotation_strategy rotation_strategy DEFAULT 'round_robin',
    
    format_rotation_enabled BOOLEAN DEFAULT false,
    
    formats JSONB DEFAULT '["pdf", "jpg", "png", "webp"]',
    
    current_format_index INTEGER DEFAULT 0,
    total_formats INTEGER DEFAULT 0,
    
    attachment_template_ids UUID[] DEFAULT '{}',
    current_template_index INTEGER DEFAULT 0,
    
    rotation_count INTEGER DEFAULT 0,
    
    usage_stats JSONB DEFAULT '{}',
    
    last_rotated_at TIMESTAMP,
    last_used_format VARCHAR(50),
    
    is_active BOOLEAN DEFAULT true,
    
    settings JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(campaign_id)
);

CREATE TABLE rotation_performance_stats (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    rotation_type VARCHAR(100) NOT NULL,
    
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    hour INTEGER CHECK (hour >= 0 AND hour <= 23),
    
    total_rotations INTEGER DEFAULT 0,
    
    items_used JSONB DEFAULT '{}',
    
    strategy_used rotation_strategy,
    
    avg_rotation_time_ms DECIMAL(10,2) DEFAULT 0.00,
    
    success_count INTEGER DEFAULT 0,
    failure_count INTEGER DEFAULT 0,
    
    performance_data JSONB DEFAULT '{}',
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(campaign_id, rotation_type, date, hour)
);

CREATE TABLE rotation_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    
    rotation_type VARCHAR(100) NOT NULL,
    
    rotated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    previous_value TEXT,
    current_value TEXT,
    
    previous_index INTEGER,
    current_index INTEGER,
    
    strategy_used rotation_strategy,
    
    recipient_id UUID REFERENCES recipients(id) ON DELETE SET NULL,
    recipient_email VARCHAR(255),
    
    rotation_reason VARCHAR(255),
    
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS sender_name_rotation_enabled BOOLEAN DEFAULT false;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS subject_rotation_enabled BOOLEAN DEFAULT false;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS template_rotation_enabled BOOLEAN DEFAULT false;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS attachment_rotation_enabled BOOLEAN DEFAULT false;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS custom_field_rotation_enabled BOOLEAN DEFAULT false;

ALTER TABLE recipients ADD COLUMN IF NOT EXISTS sender_name_used VARCHAR(255);
ALTER TABLE recipients ADD COLUMN IF NOT EXISTS subject_used TEXT;
ALTER TABLE recipients ADD COLUMN IF NOT EXISTS custom_fields_used JSONB DEFAULT '{}';

CREATE INDEX idx_sender_name_rotation_campaign_id ON sender_name_rotation(campaign_id);
CREATE INDEX idx_sender_name_rotation_is_active ON sender_name_rotation(is_active);
CREATE INDEX idx_sender_name_rotation_last_rotated_at ON sender_name_rotation(last_rotated_at);

CREATE INDEX idx_subject_rotation_campaign_id ON subject_rotation(campaign_id);
CREATE INDEX idx_subject_rotation_is_active ON subject_rotation(is_active);
CREATE INDEX idx_subject_rotation_last_rotated_at ON subject_rotation(last_rotated_at);

CREATE INDEX idx_custom_field_rotation_campaign_id ON custom_field_rotation(campaign_id);
CREATE INDEX idx_custom_field_rotation_field_name ON custom_field_rotation(field_name);
CREATE INDEX idx_custom_field_rotation_is_active ON custom_field_rotation(is_active);

CREATE INDEX idx_template_rotation_state_campaign_id ON template_rotation_state(campaign_id);
CREATE INDEX idx_template_rotation_state_is_active ON template_rotation_state(is_active);
CREATE INDEX idx_template_rotation_state_last_rotated_at ON template_rotation_state(last_rotated_at);

CREATE INDEX idx_attachment_rotation_state_campaign_id ON attachment_rotation_state(campaign_id);
CREATE INDEX idx_attachment_rotation_state_is_active ON attachment_rotation_state(is_active);
CREATE INDEX idx_attachment_rotation_state_last_rotated_at ON attachment_rotation_state(last_rotated_at);

CREATE INDEX idx_rotation_performance_stats_campaign_id ON rotation_performance_stats(campaign_id);
CREATE INDEX idx_rotation_performance_stats_rotation_type ON rotation_performance_stats(rotation_type);
CREATE INDEX idx_rotation_performance_stats_date ON rotation_performance_stats(date DESC);
CREATE INDEX idx_rotation_performance_stats_hour ON rotation_performance_stats(hour);

CREATE INDEX idx_rotation_history_campaign_id ON rotation_history(campaign_id);
CREATE INDEX idx_rotation_history_rotation_type ON rotation_history(rotation_type);
CREATE INDEX idx_rotation_history_rotated_at ON rotation_history(rotated_at DESC);
CREATE INDEX idx_rotation_history_recipient_id ON rotation_history(recipient_id);

CREATE TRIGGER update_sender_name_rotation_updated_at BEFORE UPDATE ON sender_name_rotation
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_subject_rotation_updated_at BEFORE UPDATE ON subject_rotation
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_custom_field_rotation_updated_at BEFORE UPDATE ON custom_field_rotation
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_template_rotation_state_updated_at BEFORE UPDATE ON template_rotation_state
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_attachment_rotation_state_updated_at BEFORE UPDATE ON attachment_rotation_state
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_rotation_performance_stats_updated_at BEFORE UPDATE ON rotation_performance_stats
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

INSERT INTO system_config (config_key, config_value, config_type, category, description) VALUES
('rotation.sender_name.enabled', 'false', 'boolean', 'rotation', 'Enable sender name rotation'),
('rotation.sender_name.strategy', 'round_robin', 'string', 'rotation', 'Default sender name rotation strategy'),
('rotation.subject.enabled', 'false', 'boolean', 'rotation', 'Enable subject line rotation'),
('rotation.subject.strategy', 'round_robin', 'string', 'rotation', 'Default subject rotation strategy'),
('rotation.template.enabled', 'false', 'boolean', 'rotation', 'Enable template rotation'),
('rotation.template.strategy', 'round_robin', 'string', 'rotation', 'Default template rotation strategy'),
('rotation.attachment.enabled', 'false', 'boolean', 'rotation', 'Enable attachment format rotation'),
('rotation.attachment.formats', 'pdf,jpg,png,webp', 'string', 'rotation', 'Supported attachment formats'),
('rotation.custom_field.enabled', 'false', 'boolean', 'rotation', 'Enable custom field rotation'),
('rotation.track_history', 'true', 'boolean', 'rotation', 'Track rotation history');
