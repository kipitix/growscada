-- +goose Up
-- +goose StatementBegin
-- A Deployment is made from a saved state — a Commit — and not from a Release
-- (task 85_4): the column names the Commit it deployed, and its number is what a
-- repeated Deploy compares with. It is nullable: a Deployment from a project
-- file states no Commit, and every Deployment made before this migration that
-- names no commit-backed Release keeps NULL. The index is not unique: one Commit
-- may be deployed again after a Rollback or after another Commit ran in between.
ALTER TABLE runtime.deployments ADD COLUMN commit_number INTEGER;
ALTER TABLE runtime.deployments
    ADD CONSTRAINT ck_runtime_deployments_commit_number
    CHECK (commit_number IS NULL OR commit_number > 0);

-- The Releases of before this migration know the Commit they published: the
-- Deployment that deployed one keeps that number. A Deployment from a file or
-- from a pre-model Release names none and stays NULL.
UPDATE runtime.deployments d
SET commit_number = pr.commit_number
FROM engineering.project_releases pr
WHERE pr.project_id = d.project_id
  AND pr.number = d.release_number
  AND pr.origin = 'commit'
  AND pr.commit_number IS NOT NULL;

-- The Journal entries of a Deployment carried the Release they deployed; the
-- record carries the saved state now (record/v0 0.6). The entries of a
-- Deployment from a file lose the field; the entries of a commit-backed one
-- take the Commit's number. schema_version is raised on those rows alone: a
-- tag_state entry's payload did not change.
UPDATE runtime.journal j
SET payload = (j.payload - 'release_number'::text) || COALESCE((
        SELECT jsonb_build_object('commit_number', pr.commit_number)
        FROM engineering.project_releases pr
        WHERE pr.project_id = j.project_id
          AND pr.number = (j.payload ->> 'release_number')::INT
          AND pr.origin = 'commit'
    ), '{}'::jsonb),
    schema_version = '0.6'
WHERE j.type = 'deployment';

-- A Checkpoint holds the same Deployment reference: it is rewritten with the
-- Journal entry that names it, so the two never disagree.
UPDATE runtime.checkpoints c
SET payload = jsonb_set(
        c.payload,
        '{deployment}',
        ((c.payload -> 'deployment') - 'release_number'::text) || COALESCE((
            SELECT jsonb_build_object('commit_number', pr.commit_number)
            FROM engineering.project_releases pr
            WHERE pr.project_id = c.project_id
              AND pr.number = (c.payload -> 'deployment' ->> 'release_number')::INT
              AND pr.origin = 'commit'
        ), '{}'::jsonb)),
    schema_version = '0.6'
WHERE jsonb_typeof(c.payload -> 'deployment') = 'object';

DROP INDEX runtime.idx_runtime_deployments_project_release;
ALTER TABLE runtime.deployments DROP CONSTRAINT ck_runtime_deployments_release_number;
ALTER TABLE runtime.deployments DROP COLUMN release_number;
CREATE INDEX idx_runtime_deployments_project_commit
    ON runtime.deployments (project_id, commit_number);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The Release comes back: a commit-backed Deployment names the Release that
-- published its Commit. The Releases are still here until task 85_7. A file
-- Deployment — one of a Release made from a file, and a Rollback of it — names
-- none: its Release cannot be told by a Commit, and a file Deploy leaves no
-- Release since task 85_4.
ALTER TABLE runtime.deployments ADD COLUMN release_number INTEGER;
ALTER TABLE runtime.deployments
    ADD CONSTRAINT ck_runtime_deployments_release_number
    CHECK (release_number IS NULL OR release_number > 0);
UPDATE runtime.deployments d
SET release_number = pr.number
FROM engineering.project_releases pr
WHERE pr.project_id = d.project_id
  AND pr.commit_number = d.commit_number
  AND pr.origin = 'commit';

UPDATE runtime.journal j
SET payload = (j.payload - 'commit_number'::text) || COALESCE((
        SELECT jsonb_build_object('release_number', pr.number)
        FROM engineering.project_releases pr
        WHERE pr.project_id = j.project_id
          AND pr.commit_number = (j.payload ->> 'commit_number')::INT
          AND pr.origin = 'commit'
    ), '{}'::jsonb),
    schema_version = '0.5'
WHERE j.type = 'deployment';

UPDATE runtime.checkpoints c
SET payload = jsonb_set(
        c.payload,
        '{deployment}',
        ((c.payload -> 'deployment') - 'commit_number'::text) || COALESCE((
            SELECT jsonb_build_object('release_number', pr.number)
            FROM engineering.project_releases pr
            WHERE pr.project_id = c.project_id
              AND pr.commit_number = (c.payload -> 'deployment' ->> 'commit_number')::INT
              AND pr.origin = 'commit'
        ), '{}'::jsonb)),
    schema_version = '0.5'
WHERE jsonb_typeof(c.payload -> 'deployment') = 'object';

DROP INDEX runtime.idx_runtime_deployments_project_commit;
ALTER TABLE runtime.deployments DROP CONSTRAINT ck_runtime_deployments_commit_number;
ALTER TABLE runtime.deployments DROP COLUMN commit_number;
CREATE INDEX idx_runtime_deployments_project_release
    ON runtime.deployments (project_id, release_number);
-- +goose StatementEnd
