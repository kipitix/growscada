-- +goose Up
-- +goose StatementBegin
-- The copy of an Environment's name is unique in its Project, but a Deploy may
-- swap the names of two Environments that both stay: the diff writes the copies
-- one row at a time, so an order exists in which the first write collides with
-- the name the second one is about to leave. The constraint is checked at the
-- commit of the transaction that defers it (task 35, Q5), where the swap is
-- already complete; a document that really repeats a name still fails there.
--
-- The unique key is rebuilt, not added beside: the index behind it is the one
-- a Gateway's lookup by name uses.
ALTER TABLE runtime.environments
    DROP CONSTRAINT uq_runtime_environments_project_name;
ALTER TABLE runtime.environments
    ADD CONSTRAINT uq_runtime_environments_project_name UNIQUE (project_id, name) DEFERRABLE INITIALLY IMMEDIATE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE runtime.environments
    DROP CONSTRAINT uq_runtime_environments_project_name;
ALTER TABLE runtime.environments
    ADD CONSTRAINT uq_runtime_environments_project_name UNIQUE (project_id, name);
-- +goose StatementEnd
