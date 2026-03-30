-- Increase character varying(100) columns to 255 in logs table to prevent truncation errors
ALTER TABLE logs ALTER COLUMN session_id TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN campaign_id TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN account_id TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN recipient_id TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN proxy_id TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN template_id TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN error_class TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN request_id TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN trace_id TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN span_id TYPE character varying(255);
ALTER TABLE logs ALTER COLUMN node_id TYPE character varying(255);
