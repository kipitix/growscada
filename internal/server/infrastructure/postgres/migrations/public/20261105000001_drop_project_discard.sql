-- +goose Up
-- +goose StatementBegin
-- Discard is one transaction over the Draft (ADR 0016): the mark of a Discard
-- that a policy later carried out has no place any more. Its event and policy
-- went with the code.
ALTER TABLE projects DROP COLUMN discarding;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE projects ADD COLUMN discarding BOOLEAN NOT NULL DEFAULT false;
-- +goose StatementEnd
