-- +goose Up
-- +goose StatementBegin
ALTER TABLE widget_types
    ADD COLUMN default_width  INTEGER NOT NULL DEFAULT 100,
    ADD COLUMN default_height INTEGER NOT NULL DEFAULT 100;
ALTER TABLE widget_types
    ADD CONSTRAINT chk_widget_types_default_size CHECK (default_width > 0 AND default_height > 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE widget_types
    DROP CONSTRAINT IF EXISTS chk_widget_types_default_size;
ALTER TABLE widget_types
    DROP COLUMN IF EXISTS default_width,
    DROP COLUMN IF EXISTS default_height;
-- +goose StatementEnd
