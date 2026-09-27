-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS tags (
    pk_id SERIAL PRIMARY KEY,
    id UUID NOT NULL UNIQUE,
    name TEXT NOT NULL,
    type VARCHAR(50) NOT NULL,
    value TEXT NOT NULL,
    quality VARCHAR(50) NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
-- Create indexes for common query patterns
CREATE INDEX idx_tags_id ON tags(id);
CREATE INDEX idx_tags_name ON tags(name);
CREATE INDEX idx_tags_type ON tags(type);
CREATE INDEX idx_tags_quality ON tags(quality);
CREATE INDEX idx_tags_version ON tags(version);
-- Add composite indexes if needed
CREATE INDEX idx_tags_name_type ON tags(name, type);
-- Add a trigger to automatically update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';
-- Add trigger
CREATE TRIGGER update_tags_updated_at
    BEFORE UPDATE ON tags
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
-- Add check constraints for enums
ALTER TABLE tags
    ADD CONSTRAINT chk_tags_type CHECK (type IN ('string', 'boolean', 'integer')),
    ADD CONSTRAINT chk_tags_quality CHECK (quality IN ('bad', 'uncertain', 'good', 'simulated'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Drop check constraints
ALTER TABLE tags
    DROP CONSTRAINT IF EXISTS chk_tags_type,
    DROP CONSTRAINT IF EXISTS chk_tags_quality;
-- Drop trigger
DROP TRIGGER IF EXISTS update_tags_updated_at ON tags;
DROP FUNCTION IF EXISTS update_updated_at_column();
-- Drop table
DROP TABLE IF EXISTS tags;
-- +goose StatementEnd
