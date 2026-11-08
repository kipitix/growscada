-- +goose Up
-- +goose StatementBegin
-- A Project Release is the publication of a Commit (task 72_3; ADR 0021): an
-- immutable, numbered copy of the Commit's ProjectFile with a note, the author
-- and the time. It copies the content rather than naming the Commit, so it
-- outlives the Commit, the Draft and future retention; the canceled
-- commit_number is only the provenance of where it came from.
CREATE TABLE engineering.project_releases (
    id            UUID        PRIMARY KEY,
    project_id    UUID        NOT NULL,
    number        INT         NOT NULL,
    commit_number INT         NULL,
    commit_title  TEXT        NULL,
    note          TEXT        NULL,
    author        TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    origin        TEXT        NOT NULL,
    content       JSONB       NOT NULL,
    CONSTRAINT fk_project_releases_project_id FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT ck_project_releases_number CHECK (number > 0),
    CONSTRAINT ck_project_releases_origin CHECK (origin IN ('commit', 'file')),
    CONSTRAINT uq_project_releases_project_number UNIQUE (project_id, number)
);
CREATE INDEX idx_project_releases_project_number ON engineering.project_releases(project_id, number DESC);

-- One Commit is published at most once (a partial unique index: a Release
-- made from a file names no Commit).
CREATE UNIQUE INDEX uq_project_releases_project_commit
    ON engineering.project_releases(project_id, commit_number)
    WHERE origin = 'commit';

-- The Library Pins of a Project Release's ProjectFile: its provenance, and
-- what keeps a pinned Library from being deleted while a Release names it
-- (task 71). A Release made from a file (task 72_4) fills them from the file.
CREATE TABLE engineering.project_release_library_pins (
    release_id     UUID NOT NULL,
    library_id     UUID NOT NULL,
    release_number INT  NOT NULL,
    CONSTRAINT fk_project_release_library_pins_release_id FOREIGN KEY (release_id) REFERENCES engineering.project_releases(id) ON DELETE CASCADE,
    PRIMARY KEY (release_id, library_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE engineering.project_release_library_pins;
DROP TABLE engineering.project_releases;
-- +goose StatementEnd
