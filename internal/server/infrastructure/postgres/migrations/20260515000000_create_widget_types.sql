-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS widget_types (
    pk_id SERIAL PRIMARY KEY,
    id UUID NOT NULL UNIQUE,
    name TEXT NOT NULL,
    html_template TEXT NOT NULL,
    script TEXT NOT NULL,
    script_language VARCHAR(50) NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_widget_types_id ON widget_types(id);
CREATE INDEX idx_widget_types_name ON widget_types(name);
CREATE INDEX idx_widget_types_script_language ON widget_types(script_language);
CREATE INDEX idx_widget_types_version ON widget_types(version);
CREATE OR REPLACE FUNCTION update_widget_types_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';
CREATE TRIGGER update_widget_types_updated_at
    BEFORE UPDATE ON widget_types
    FOR EACH ROW
    EXECUTE FUNCTION update_widget_types_updated_at();
ALTER TABLE widget_types
    ADD CONSTRAINT chk_widget_types_script_language CHECK (script_language IN ('javascript', 'python', 'lua'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE widget_types
    DROP CONSTRAINT IF EXISTS chk_widget_types_script_language;
DROP TRIGGER IF EXISTS update_widget_types_updated_at ON widget_types;
DROP FUNCTION IF EXISTS update_widget_types_updated_at();
DROP TABLE IF EXISTS widget_types;
-- +goose StatementEnd
