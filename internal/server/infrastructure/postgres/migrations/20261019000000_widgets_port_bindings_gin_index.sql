-- +goose Up
-- +goose StatementBegin
-- unbind_deleted_tag finds the Scenes bound to a deleted Tag by containment
-- on port_bindings (SceneRepository.FindByTagID); without an index every
-- deleted Tag scans all widgets.
CREATE INDEX idx_widgets_port_bindings ON widgets USING GIN (port_bindings jsonb_path_ops);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_widgets_port_bindings;
-- +goose StatementEnd
