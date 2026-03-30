CREATE TABLE IF NOT EXISTS recipient_lists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    recipient_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

ALTER TABLE recipients ADD COLUMN IF NOT EXISTS recipient_list_id UUID REFERENCES recipient_lists(id) ON DELETE SET NULL;
