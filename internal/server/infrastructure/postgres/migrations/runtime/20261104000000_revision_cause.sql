-- +goose Up
-- +goose StatementBegin
-- A Revision records why it exists: a Deploy made it, or a Rollback copied an
-- earlier one (ADR 0014). A Rollback names the Revision it copied in
-- rolled_back_from, which is a foreign key to the same Project's Revisions:
-- a Row is never removed while its Project lives, and one goes with the
-- Project by the cascade. The cause is NOT NULL without a default, so a new
-- writer has to name it.
ALTER TABLE runtime.revisions ADD COLUMN cause TEXT;
ALTER TABLE runtime.revisions ADD COLUMN rolled_back_from INTEGER;

UPDATE runtime.revisions SET cause = 'deploy';

ALTER TABLE runtime.revisions ALTER COLUMN cause SET NOT NULL;
ALTER TABLE runtime.revisions
    ADD CONSTRAINT ck_runtime_revisions_cause CHECK (cause IN ('deploy', 'rollback'));
ALTER TABLE runtime.revisions
    ADD CONSTRAINT ck_runtime_revisions_rolled_back_from
    CHECK ((cause = 'deploy' AND rolled_back_from IS NULL)
        OR (cause = 'rollback' AND rolled_back_from IS NOT NULL));
ALTER TABLE runtime.revisions
    ADD CONSTRAINT fk_runtime_revisions_rolled_back_from
    FOREIGN KEY (project_id, rolled_back_from)
    REFERENCES runtime.revisions (project_id, n) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE runtime.revisions DROP CONSTRAINT fk_runtime_revisions_rolled_back_from;
ALTER TABLE runtime.revisions DROP CONSTRAINT ck_runtime_revisions_rolled_back_from;
ALTER TABLE runtime.revisions DROP CONSTRAINT ck_runtime_revisions_cause;
ALTER TABLE runtime.revisions DROP COLUMN rolled_back_from;
ALTER TABLE runtime.revisions DROP COLUMN cause;
-- +goose StatementEnd
