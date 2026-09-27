-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS widgets (
    pk_id SERIAL PRIMARY KEY,
    id UUID NOT NULL UNIQUE,
    name TEXT NOT NULL,
    x DOUBLE PRECISION NOT NULL DEFAULT 0,
    y DOUBLE PRECISION NOT NULL DEFAULT 0,
    z DOUBLE PRECISION NOT NULL DEFAULT 0,
    type_id UUID NOT NULL,
    labels TEXT[] NOT NULL DEFAULT '{}',
    tag_ids UUID[] NOT NULL DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_widgets_id ON widgets(id);
CREATE INDEX idx_widgets_name ON widgets(name);
CREATE INDEX idx_widgets_type_id ON widgets(type_id);
CREATE INDEX idx_widgets_version ON widgets(version);
CREATE OR REPLACE FUNCTION update_widgets_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';
CREATE TRIGGER update_widgets_updated_at
    BEFORE UPDATE ON widgets
    FOR EACH ROW
    EXECUTE FUNCTION update_widgets_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS update_widgets_updated_at ON widgets;
DROP FUNCTION IF EXISTS update_widgets_updated_at();
DROP TABLE IF EXISTS widgets;
-- +goose StatementEnd
