-- +goose Up
-- +goose StatementBegin
-- A Device Token names its Device, and the Device names its Project
-- (GLOSSARY.md): a Gateway presents the token alone, so the Runtime node must
-- resolve it to both without asking the Engineering node (ADR 0011). The
-- token outlives Deploys and sleeps while its Device is not in the running
-- Revision — its copy in runtime.devices gone — so the Project is stored with
-- the token itself. Tokens go with their Project's row (CASCADE): a deleted
-- Project has nothing left to authenticate.
--
-- The column is NOT NULL from the start: the token management ships with this
-- migration, so no token predates it.
ALTER TABLE runtime.device_tokens ADD COLUMN project_id UUID NOT NULL;
ALTER TABLE runtime.device_tokens
    ADD CONSTRAINT fk_runtime_device_tokens_project_id
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE runtime.device_tokens DROP CONSTRAINT fk_runtime_device_tokens_project_id;
ALTER TABLE runtime.device_tokens DROP COLUMN project_id;
-- +goose StatementEnd
