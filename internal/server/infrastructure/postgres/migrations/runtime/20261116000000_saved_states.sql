-- +goose Up
-- +goose StatementBegin
-- A Deployment is made from a saved state (task 85_4), and the saved state is
-- called so (task 88_1; ADR 0026): the column and the constraint the code
-- names follow. The Journal and the Checkpoint name the number in their
-- payloads, which change with the records (task 88_2).
ALTER TABLE runtime.deployments RENAME COLUMN commit_number TO saved_state_number;
ALTER TABLE runtime.deployments RENAME CONSTRAINT ck_runtime_deployments_commit_number TO ck_runtime_deployments_saved_state_number;
ALTER INDEX runtime.idx_runtime_deployments_project_commit RENAME TO idx_runtime_deployments_project_saved_state;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER INDEX runtime.idx_runtime_deployments_project_saved_state RENAME TO idx_runtime_deployments_project_commit;
ALTER TABLE runtime.deployments RENAME CONSTRAINT ck_runtime_deployments_saved_state_number TO ck_runtime_deployments_commit_number;
ALTER TABLE runtime.deployments RENAME COLUMN saved_state_number TO commit_number;
-- +goose StatementEnd
