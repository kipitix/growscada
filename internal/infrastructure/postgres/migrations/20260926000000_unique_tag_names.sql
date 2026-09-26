-- +goose Up
-- +goose StatementBegin
-- A Tag's name is its unique natural key (ADR 0002). The unique constraint's
-- index replaces the plain name index.
DROP INDEX IF EXISTS idx_tags_name;
ALTER TABLE tags ADD CONSTRAINT uq_tags_name UNIQUE (name);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE tags DROP CONSTRAINT IF EXISTS uq_tags_name;
CREATE INDEX idx_tags_name ON tags(name);
-- +goose StatementEnd
