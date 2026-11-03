-- +goose Up
-- +goose StatementBegin
-- The Engineering node's copy of the last ProjectFile it sent to the Runtime
-- node (task 35, Q6): one row per Project, replaced by every Deploy. It is
-- secondary — the truth is the Revision on the Runtime node — and it is what
-- tells an unchanged Draft from a changed one, and what the Project mode shows
-- as "Revision N · deployed at T". It goes with its Project by the cascade.
CREATE TABLE engineering.project_files (
    project_id UUID        PRIMARY KEY,
    revision_n INTEGER     NOT NULL,
    deployed_at TIMESTAMPTZ NOT NULL,
    config     JSONB       NOT NULL,
    CONSTRAINT fk_project_files_project_id FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE engineering.project_files;
-- +goose StatementEnd
