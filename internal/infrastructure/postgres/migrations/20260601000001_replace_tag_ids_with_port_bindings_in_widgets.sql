-- +goose Up
-- +goose StatementBegin
ALTER TABLE widgets
    ADD COLUMN port_bindings JSONB NOT NULL DEFAULT '[]';
-- JSON element shape: {"port_name":"temperature","tag_id":"<uuid>"}
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE widgets
    DROP COLUMN tag_ids;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE widgets
    ADD COLUMN tag_ids UUID[] NOT NULL DEFAULT '{}';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE widgets
    DROP COLUMN port_bindings;
-- +goose StatementEnd
