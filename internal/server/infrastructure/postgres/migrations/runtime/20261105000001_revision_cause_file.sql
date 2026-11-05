-- +goose Up
-- +goose StatementBegin
-- A Revision records why it exists: a Deploy made it from the Draft, a
-- Rollback copied an earlier one, or a project file was deployed as it is
-- (ADR 0018). A file Revision names no Revision it copied, like a Deploy.
ALTER TABLE runtime.revisions DROP CONSTRAINT ck_runtime_revisions_rolled_back_from;
ALTER TABLE runtime.revisions DROP CONSTRAINT ck_runtime_revisions_cause;

ALTER TABLE runtime.revisions
    ADD CONSTRAINT ck_runtime_revisions_cause CHECK (cause IN ('deploy', 'rollback', 'file'));
ALTER TABLE runtime.revisions
    ADD CONSTRAINT ck_runtime_revisions_rolled_back_from
    CHECK ((cause IN ('deploy', 'file') AND rolled_back_from IS NULL)
        OR (cause = 'rollback' AND rolled_back_from IS NOT NULL));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- A file Revision cannot be stored by a schema that knows only deploy and
-- rollback: it is removed, and the constraint goes back to the two causes.
DELETE FROM runtime.revisions WHERE cause = 'file';
ALTER TABLE runtime.revisions DROP CONSTRAINT ck_runtime_revisions_rolled_back_from;
ALTER TABLE runtime.revisions DROP CONSTRAINT ck_runtime_revisions_cause;

ALTER TABLE runtime.revisions
    ADD CONSTRAINT ck_runtime_revisions_cause CHECK (cause IN ('deploy', 'rollback'));
ALTER TABLE runtime.revisions
    ADD CONSTRAINT ck_runtime_revisions_rolled_back_from
    CHECK ((cause = 'deploy' AND rolled_back_from IS NULL)
        OR (cause = 'rollback' AND rolled_back_from IS NOT NULL));
-- +goose StatementEnd
