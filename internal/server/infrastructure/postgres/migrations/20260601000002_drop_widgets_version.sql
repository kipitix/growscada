-- +goose Up
-- +goose StatementBegin
-- Widget no longer has its own optimistic-concurrency version: it is an
-- entity of the Scene aggregate, and scenes.version is the sole consistency
-- boundary for a scene and all of its widgets.
DROP INDEX IF EXISTS idx_widgets_version;
ALTER TABLE widgets
    DROP COLUMN version;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE widgets
    ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_widgets_version ON widgets(version);
-- +goose StatementEnd
