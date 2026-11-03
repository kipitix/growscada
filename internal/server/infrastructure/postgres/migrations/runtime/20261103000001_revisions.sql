-- +goose Up
-- +goose StatementBegin
-- A Revision is a numbered, immutable snapshot of a Project's configuration,
-- stored as the ProjectFile document itself: config is the truth about what
-- the Project runs, and the copies of the Tag definitions, the Devices, the
-- Environments and their pairs are derived from it, rebuilt in the
-- transaction that creates the Revision (ADR 0011; the amendment recording it
-- lands with task 35, part 5; task 35, Q5, Q8). A row is never changed and
-- never removed while its Project lives; it goes with the Project by the
-- cascade.
--
-- The unique key is what numbers a Revision and the safety net of a Deploy
-- racing another: it indexes project_id first, so reading the current
-- Revision (ORDER BY n DESC LIMIT 1) needs no index of its own.
CREATE TABLE runtime.revisions (
    pk_id       SERIAL PRIMARY KEY,
    project_id  UUID        NOT NULL,
    n           INTEGER     NOT NULL,
    deployed_at TIMESTAMPTZ NOT NULL,
    config      JSONB       NOT NULL,
    CONSTRAINT fk_runtime_revisions_project_id FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT uq_runtime_revisions_project_n UNIQUE (project_id, n)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE runtime.revisions;
-- +goose StatementEnd
