-- +goose Up
-- +goose StatementBegin
ALTER TABLE widgets
    ADD COLUMN port_bindings JSONB NOT NULL DEFAULT '[]';
-- JSON element shape: {"port_name":"temperature","tag_id":"<uuid>"}
-- +goose StatementEnd

-- Migrate existing tag_ids to synthetic port names (port_0, port_1, …).
-- Widgets with no tag bindings keep the empty array default.
-- +goose StatementBegin
UPDATE widgets
SET port_bindings = (
    SELECT COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'port_name', 'port_' || (ord - 1)::text,
                'tag_id',    tid::text
            )
            ORDER BY ord
        ),
        '[]'::jsonb
    )
    FROM unnest(tag_ids) WITH ORDINALITY AS t(tid, ord)
)
WHERE array_length(tag_ids, 1) > 0;
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
