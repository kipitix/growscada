-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS scenes (
    pk_id SERIAL PRIMARY KEY,
    id UUID NOT NULL UNIQUE,
    name TEXT NOT NULL,
    width INTEGER NOT NULL DEFAULT 1920,
    height INTEGER NOT NULL DEFAULT 1080,
    background_html TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_scenes_id ON scenes(id);
CREATE INDEX idx_scenes_name ON scenes(name);
CREATE OR REPLACE FUNCTION update_scenes_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';
CREATE TRIGGER update_scenes_updated_at
    BEFORE UPDATE ON scenes
    FOR EACH ROW
    EXECUTE FUNCTION update_scenes_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS update_scenes_updated_at ON scenes;
DROP FUNCTION IF EXISTS update_scenes_updated_at();
DROP TABLE IF EXISTS scenes;
-- +goose StatementEnd
