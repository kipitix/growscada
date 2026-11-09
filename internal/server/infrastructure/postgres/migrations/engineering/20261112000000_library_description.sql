-- +goose Up
-- +goose StatementBegin
-- A Library's description is an attribute of the aggregate, shown on the
-- selection page and edited with its name (task 73). It is never content: a
-- Library Release and a pin carry the WidgetType copies alone.
ALTER TABLE engineering.libraries ADD COLUMN description TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE engineering.libraries DROP COLUMN description;
-- +goose StatementEnd
