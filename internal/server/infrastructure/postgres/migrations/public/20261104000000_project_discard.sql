-- +goose Up
-- +goose StatementBegin
-- The mark of a Discard (task 35, Q10): while it stands the Project's Draft is
-- read-only, and the policy of NewDiscardProjectPolicy brings it back to the
-- Revision the Project runs, clearing the mark last. It is a flag and not a
-- target: the Revision is read from the Runtime node when the policy runs.
ALTER TABLE projects ADD COLUMN discarding BOOLEAN NOT NULL DEFAULT false;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE projects DROP COLUMN discarding;
-- +goose StatementEnd
