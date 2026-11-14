-- +goose Up
-- +goose StatementBegin
-- Releases are gone (task 85_7): a Deploy makes a saved state — a Commit — the
-- running Deployment, and a Project's import names the saved state of a
-- Library, so nothing reads either table any more. A Release is replaced by a
-- Commit with a label, a Deploy of that Commit and an Import of a Library's
-- Commit; the history of a Project or a Library is its Commits, read from
-- engineering.commits.
--
-- The drop lives in the Runtime node's chain although the tables are the
-- Engineering node's: the runtime migration 20261114000002 read them to give
-- the Deployments their saved states, and the chains are applied one after the
-- other, the Engineering one first. Dropping the tables there would leave that
-- migration without a table on a fresh database. This one runs after it.
--
-- This drops data: the Releases themselves, their notes and the copies of the
-- WidgetTypes they held. The Down migration recreates the empty tables and
-- brings nothing back, and nothing can: the rows are not recoverable from the
-- Commits, because a Release was a copy with a note, an Author and a time of
-- its own, not a reference.
DROP TABLE engineering.project_release_library_pins;
DROP TABLE engineering.project_releases;
DROP TABLE engineering.released_widget_types;
DROP TABLE engineering.library_releases;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The tables come back empty: see the Up comment.
CREATE TABLE engineering.library_releases (
    pk_id      SERIAL PRIMARY KEY,
    library_id UUID        NOT NULL,
    number     INTEGER     NOT NULL,
    note       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    commit_number INTEGER,
    author        TEXT NOT NULL DEFAULT 'system',
    origin        TEXT NOT NULL DEFAULT 'legacy',
    CONSTRAINT fk_library_releases_library_id FOREIGN KEY (library_id) REFERENCES engineering.libraries(id) ON DELETE CASCADE,
    CONSTRAINT uq_library_releases_library_number UNIQUE (library_id, number),
    CONSTRAINT chk_library_releases_number CHECK (number > 0),
    CONSTRAINT ck_library_releases_origin CHECK (origin IN ('commit', 'legacy')),
    CONSTRAINT ck_library_releases_commit CHECK (origin <> 'commit' OR commit_number IS NOT NULL)
);

CREATE UNIQUE INDEX uq_library_releases_commit
    ON engineering.library_releases (library_id, commit_number)
    WHERE commit_number IS NOT NULL AND origin = 'commit';

CREATE TABLE engineering.released_widget_types (
    pk_id           SERIAL PRIMARY KEY,
    library_id      UUID        NOT NULL,
    release_number  INTEGER     NOT NULL,
    id              UUID        NOT NULL,
    name            TEXT        NOT NULL,
    html_template   TEXT        NOT NULL,
    script          TEXT        NOT NULL,
    script_language VARCHAR(50) NOT NULL,
    default_width   INTEGER     NOT NULL,
    default_height  INTEGER     NOT NULL,
    input_ports     JSONB       NOT NULL DEFAULT '[]',
    CONSTRAINT fk_released_widget_types_release FOREIGN KEY (library_id, release_number)
        REFERENCES engineering.library_releases(library_id, number) ON DELETE CASCADE,
    CONSTRAINT uq_released_widget_types_release_id UNIQUE (library_id, release_number, id)
);
CREATE INDEX idx_released_widget_types_id ON engineering.released_widget_types(id);

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
    CONSTRAINT ck_project_releases_commit CHECK (origin <> 'commit' OR (commit_number IS NOT NULL AND commit_title IS NOT NULL)),
    CONSTRAINT uq_project_releases_project_number UNIQUE (project_id, number)
);
CREATE INDEX idx_project_releases_project_number ON engineering.project_releases(project_id, number DESC);

CREATE UNIQUE INDEX uq_project_releases_project_commit
    ON engineering.project_releases(project_id, commit_number)
    WHERE origin = 'commit';

CREATE TABLE engineering.project_release_library_pins (
    release_id     UUID NOT NULL,
    library_id     UUID NOT NULL,
    release_number INT  NOT NULL,
    CONSTRAINT fk_project_release_library_pins_release_id FOREIGN KEY (release_id) REFERENCES engineering.project_releases(id) ON DELETE CASCADE,
    PRIMARY KEY (release_id, library_id)
);
-- +goose StatementEnd
