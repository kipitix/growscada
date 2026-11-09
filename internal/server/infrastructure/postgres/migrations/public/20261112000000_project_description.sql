-- +goose Up
-- +goose StatementBegin
-- A Project's description is an attribute of the aggregate, shown on the
-- selection page and edited with its name (task 73). It is never content: a
-- ProjectFile, a Deployment and a Commit do not carry it.
ALTER TABLE projects ADD COLUMN description TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE projects DROP COLUMN description;
-- +goose StatementEnd
