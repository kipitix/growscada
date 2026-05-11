-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS indicator_types (
    pk_id SERIAL PRIMARY KEY,
    id UUID NOT NULL UNIQUE,
    name TEXT NOT NULL,
    svg_template TEXT NOT NULL,
    script TEXT NOT NULL,
    script_language VARCHAR(50) NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_indicator_types_id ON indicator_types(id);
CREATE INDEX idx_indicator_types_name ON indicator_types(name);
CREATE INDEX idx_indicator_types_script_language ON indicator_types(script_language);
CREATE INDEX idx_indicator_types_version ON indicator_types(version);
CREATE OR REPLACE FUNCTION update_indicator_types_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';
CREATE TRIGGER update_indicator_types_updated_at
    BEFORE UPDATE ON indicator_types
    FOR EACH ROW
    EXECUTE FUNCTION update_indicator_types_updated_at();
ALTER TABLE indicator_types
    ADD CONSTRAINT chk_indicator_types_script_language CHECK (script_language IN ('javascript', 'python', 'lua'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE indicator_types
    DROP CONSTRAINT IF EXISTS chk_indicator_types_script_language;
DROP TRIGGER IF EXISTS update_indicator_types_updated_at ON indicator_types;
DROP FUNCTION IF EXISTS update_indicator_types_updated_at();
DROP TABLE IF EXISTS indicator_types;
-- +goose StatementEnd
