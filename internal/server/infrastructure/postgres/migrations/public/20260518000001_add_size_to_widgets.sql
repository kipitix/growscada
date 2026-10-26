-- +goose Up
-- +goose StatementBegin
ALTER TABLE widgets
    ADD COLUMN width  INTEGER NOT NULL DEFAULT 100,
    ADD COLUMN height INTEGER NOT NULL DEFAULT 100;
ALTER TABLE widgets
    ADD CONSTRAINT chk_widgets_size CHECK (width > 0 AND height > 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE widgets
    DROP CONSTRAINT IF EXISTS chk_widgets_size;
ALTER TABLE widgets
    DROP COLUMN IF EXISTS width,
    DROP COLUMN IF EXISTS height;
-- +goose StatementEnd
