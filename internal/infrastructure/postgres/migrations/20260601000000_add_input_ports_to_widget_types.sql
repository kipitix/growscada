-- +goose Up
-- +goose StatementBegin
ALTER TABLE widget_types
    ADD COLUMN input_ports JSONB NOT NULL DEFAULT '[]';
-- JSON element shape: {"name":"temperature","description":"Process temperature","type_hint":"integer"}
-- type_hint "" or "unknown" means any tag type is accepted.
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE widget_types
    DROP COLUMN input_ports;
-- +goose StatementEnd
