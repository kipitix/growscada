-- +goose Up
-- +goose StatementBegin
-- The Engineering node's copy of the last ProjectFile it sent made no sense
-- once a Rollback could create a Revision on the Runtime node alone: the copy
-- would name a Revision the Project no longer runs, and its readers went with
-- it — Project mode reads deploy/preview, and DeployService.State had no
-- caller (ADR 0014, the amendment to ADR 0013).
DROP TABLE engineering.project_files;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE engineering.project_files (
    project_id UUID        PRIMARY KEY,
    revision_n INTEGER     NOT NULL,
    deployed_at TIMESTAMPTZ NOT NULL,
    config     JSONB       NOT NULL,
    CONSTRAINT fk_project_files_project_id FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);
-- +goose StatementEnd
