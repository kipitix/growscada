-- +goose Up
-- +goose StatementBegin
-- A Checkpoint is an immutable snapshot of a Project's operational state at a
-- Journal position (task 44, ADR 0023). It sits beside the Journal, not in it:
-- seq is the position of the entry the snapshot describes — the state after
-- that entry. The payload carries the Deployment in force (its number, cause,
-- rolled_back_from and release_number, never the document) and every Tag state
-- with its own position. The record goes with its Project, as the Journal does.
-- No foreign key to the Deployment: its number is written in the same
-- transaction, a Deployment is immutable and is removed only with the Project.
CREATE TABLE runtime.checkpoints (
    project_id     UUID        NOT NULL,
    seq            BIGINT      NOT NULL,
    taken_at       TIMESTAMPTZ NOT NULL,
    schema_version TEXT        NOT NULL,
    payload        JSONB       NOT NULL,
    PRIMARY KEY (project_id, seq),
    CONSTRAINT fk_runtime_checkpoints_project_id
        FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);
CREATE INDEX idx_runtime_checkpoints_project_seq
    ON runtime.checkpoints(project_id, seq DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE runtime.checkpoints;
-- +goose StatementEnd
