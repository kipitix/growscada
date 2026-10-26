-- +goose Up
-- +goose StatementBegin

-- Widgets without a scene are semantically invalid (a widget is always placed on a scene).
-- Remove any orphaned rows before enforcing the constraint.
DELETE FROM widgets WHERE scene_id IS NULL;

ALTER TABLE widgets ALTER COLUMN scene_id SET NOT NULL;

ALTER TABLE widgets
    ADD CONSTRAINT fk_widgets_scene_id
    FOREIGN KEY (scene_id) REFERENCES scenes(id) ON DELETE CASCADE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE widgets DROP CONSTRAINT IF EXISTS fk_widgets_scene_id;
ALTER TABLE widgets ALTER COLUMN scene_id DROP NOT NULL;

-- +goose StatementEnd
