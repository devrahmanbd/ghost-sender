DELETE FROM system_config WHERE config_key IN (
    'rotation.sender_name.enabled',
    'rotation.sender_name.strategy',
    'rotation.subject.enabled',
    'rotation.subject.strategy',
    'rotation.template.enabled',
    'rotation.template.strategy',
    'rotation.attachment.enabled',
    'rotation.attachment.formats',
    'rotation.custom_field.enabled',
    'rotation.track_history'
);

DROP TRIGGER IF EXISTS update_rotation_performance_stats_updated_at ON rotation_performance_stats;
DROP TRIGGER IF EXISTS update_attachment_rotation_state_updated_at ON attachment_rotation_state;
DROP TRIGGER IF EXISTS update_template_rotation_state_updated_at ON template_rotation_state;
DROP TRIGGER IF EXISTS update_custom_field_rotation_updated_at ON custom_field_rotation;
DROP TRIGGER IF EXISTS update_subject_rotation_updated_at ON subject_rotation;
DROP TRIGGER IF EXISTS update_sender_name_rotation_updated_at ON sender_name_rotation;

DROP INDEX IF EXISTS idx_rotation_history_recipient_id;
DROP INDEX IF EXISTS idx_rotation_history_rotated_at;
DROP INDEX IF EXISTS idx_rotation_history_rotation_type;
DROP INDEX IF EXISTS idx_rotation_history_campaign_id;

DROP INDEX IF EXISTS idx_rotation_performance_stats_hour;
DROP INDEX IF EXISTS idx_rotation_performance_stats_date;
DROP INDEX IF EXISTS idx_rotation_performance_stats_rotation_type;
DROP INDEX IF EXISTS idx_rotation_performance_stats_campaign_id;

DROP INDEX IF EXISTS idx_attachment_rotation_state_last_rotated_at;
DROP INDEX IF EXISTS idx_attachment_rotation_state_is_active;
DROP INDEX IF EXISTS idx_attachment_rotation_state_campaign_id;

DROP INDEX IF EXISTS idx_template_rotation_state_last_rotated_at;
DROP INDEX IF EXISTS idx_template_rotation_state_is_active;
DROP INDEX IF EXISTS idx_template_rotation_state_campaign_id;

DROP INDEX IF EXISTS idx_custom_field_rotation_is_active;
DROP INDEX IF EXISTS idx_custom_field_rotation_field_name;
DROP INDEX IF EXISTS idx_custom_field_rotation_campaign_id;

DROP INDEX IF EXISTS idx_subject_rotation_last_rotated_at;
DROP INDEX IF EXISTS idx_subject_rotation_is_active;
DROP INDEX IF EXISTS idx_subject_rotation_campaign_id;

DROP INDEX IF EXISTS idx_sender_name_rotation_last_rotated_at;
DROP INDEX IF EXISTS idx_sender_name_rotation_is_active;
DROP INDEX IF EXISTS idx_sender_name_rotation_campaign_id;

ALTER TABLE recipients DROP COLUMN IF EXISTS custom_fields_used;
ALTER TABLE recipients DROP COLUMN IF EXISTS subject_used;
ALTER TABLE recipients DROP COLUMN IF EXISTS sender_name_used;

ALTER TABLE campaigns DROP COLUMN IF EXISTS custom_field_rotation_enabled;
ALTER TABLE campaigns DROP COLUMN IF EXISTS attachment_rotation_enabled;
ALTER TABLE campaigns DROP COLUMN IF EXISTS template_rotation_enabled;
ALTER TABLE campaigns DROP COLUMN IF EXISTS subject_rotation_enabled;
ALTER TABLE campaigns DROP COLUMN IF EXISTS sender_name_rotation_enabled;

DROP TABLE IF EXISTS rotation_history;
DROP TABLE IF EXISTS rotation_performance_stats;
DROP TABLE IF EXISTS attachment_rotation_state;
DROP TABLE IF EXISTS template_rotation_state;
DROP TABLE IF EXISTS custom_field_rotation;
DROP TABLE IF EXISTS subject_rotation;
DROP TABLE IF EXISTS sender_name_rotation;
