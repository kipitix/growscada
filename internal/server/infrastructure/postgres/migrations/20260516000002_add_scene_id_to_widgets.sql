-- +goose Up
-- +goose StatementBegin
ALTER TABLE widgets ADD COLUMN IF NOT EXISTS scene_id UUID NULL;
CREATE INDEX idx_widgets_scene_id ON widgets(scene_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_widgets_scene_id;
ALTER TABLE widgets DROP COLUMN IF EXISTS scene_id;
-- +goose StatementEnd
