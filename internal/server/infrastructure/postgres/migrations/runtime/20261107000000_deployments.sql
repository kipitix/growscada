-- +goose Up
-- +goose StatementBegin
-- The Runtime node's numbered, immutable record of a Project's running
-- configuration was called a Revision before task 72_9: it is a Deployment
-- now, and the names of its table, its constraints, its indexes and its
-- sequence follow. The shape is unchanged — number, deployed_at, cause,
-- rolled_back_from and the ProjectFile in config stay — so old rows are read
-- as Deployments without a rewrite.
ALTER TABLE runtime.revisions RENAME TO deployments;
ALTER TABLE runtime.deployments RENAME COLUMN n TO number;

ALTER TABLE runtime.deployments
    RENAME CONSTRAINT uq_runtime_revisions_project_n TO uq_runtime_deployments_project_number;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT fk_runtime_revisions_project_id TO fk_runtime_deployments_project_id;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT ck_runtime_revisions_cause TO ck_runtime_deployments_cause;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT ck_runtime_revisions_rolled_back_from TO ck_runtime_deployments_rolled_back_from;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT fk_runtime_revisions_rolled_back_from TO fk_runtime_deployments_rolled_back_from;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT revisions_pkey TO deployments_pkey;

ALTER INDEX runtime.idx_runtime_revisions_rolled_back_from
    RENAME TO idx_runtime_deployments_rolled_back_from;
ALTER SEQUENCE runtime.revisions_pk_id_seq RENAME TO deployments_pk_id_seq;

-- A Journal entry of a new Deployment had the type "revision": it is a
-- "deployment" now. The payload (number, cause, rolled_back_from) is
-- unchanged, so an entry written before this migration reads as it is.
ALTER TABLE runtime.journal DROP CONSTRAINT ck_runtime_journal_type;
UPDATE runtime.journal SET type = 'deployment' WHERE type = 'revision';
ALTER TABLE runtime.journal
    ADD CONSTRAINT ck_runtime_journal_type CHECK (type IN ('tag_state', 'deployment'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The record goes back to "revision", and every entry of a Deployment is a
-- revision entry again.
ALTER TABLE runtime.journal DROP CONSTRAINT ck_runtime_journal_type;
UPDATE runtime.journal SET type = 'revision' WHERE type = 'deployment';
ALTER TABLE runtime.journal
    ADD CONSTRAINT ck_runtime_journal_type CHECK (type IN ('tag_state', 'revision'));

ALTER SEQUENCE runtime.deployments_pk_id_seq RENAME TO revisions_pk_id_seq;
ALTER INDEX runtime.idx_runtime_deployments_rolled_back_from
    RENAME TO idx_runtime_revisions_rolled_back_from;

ALTER TABLE runtime.deployments
    RENAME CONSTRAINT deployments_pkey TO revisions_pkey;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT fk_runtime_deployments_rolled_back_from TO fk_runtime_revisions_rolled_back_from;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT ck_runtime_deployments_rolled_back_from TO ck_runtime_revisions_rolled_back_from;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT ck_runtime_deployments_cause TO ck_runtime_revisions_cause;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT fk_runtime_deployments_project_id TO fk_runtime_revisions_project_id;
ALTER TABLE runtime.deployments
    RENAME CONSTRAINT uq_runtime_deployments_project_number TO uq_runtime_revisions_project_n;

ALTER TABLE runtime.deployments RENAME COLUMN number TO n;
ALTER TABLE runtime.deployments RENAME TO revisions;
-- +goose StatementEnd
