-- +goose Up
-- +goose StatementBegin
-- A Deployment is made from a Project Release (task 72_4): the column names the
-- Release it deployed, and the Release's number is what a repeated Deploy
-- compares with. It is nullable — a Deployment from a project file states no
-- Release, and every Deployment made before this migration keeps NULL. The
-- index is not unique: one Release may be deployed again after a Rollback or
-- after another Release ran in between.
ALTER TABLE runtime.deployments ADD COLUMN release_number INTEGER;
ALTER TABLE runtime.deployments
    ADD CONSTRAINT ck_runtime_deployments_release_number
    CHECK (release_number IS NULL OR release_number > 0);
CREATE INDEX idx_runtime_deployments_project_release
    ON runtime.deployments (project_id, release_number);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX runtime.idx_runtime_deployments_project_release;
ALTER TABLE runtime.deployments DROP CONSTRAINT ck_runtime_deployments_release_number;
ALTER TABLE runtime.deployments DROP COLUMN release_number;
-- +goose StatementEnd
