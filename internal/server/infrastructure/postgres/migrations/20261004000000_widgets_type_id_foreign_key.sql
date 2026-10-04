-- +goose Up
-- +goose StatementBegin
-- A Widget's WidgetType always exists: a type used by Widgets cannot be
-- deleted. Before this, deleting a type left its Widgets pointing nowhere;
-- they rendered as a placeholder and could not be bound, so they are dropped.
DELETE FROM widgets
    WHERE NOT EXISTS (SELECT 1 FROM widget_types WHERE widget_types.id = widgets.type_id);
ALTER TABLE widgets
    ADD CONSTRAINT fk_widgets_type_id
    FOREIGN KEY (type_id) REFERENCES widget_types(id) ON DELETE RESTRICT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Dropped orphan Widgets cannot be restored.
ALTER TABLE widgets DROP CONSTRAINT IF EXISTS fk_widgets_type_id;
-- +goose StatementEnd
