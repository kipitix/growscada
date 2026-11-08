-- +goose Up
-- +goose StatementBegin
-- A Commit is an immutable snapshot of a whole Draft (task 72_2; ADR 0021):
-- an entity of the Draft, like a DraftChange, numbered within it. It changes
-- no content: writing it raises no Version and writes no DraftChange. content
-- is the internal encoding of the Draft's ProjectFile (a Project) or of its
-- WidgetTypes (a Library), with a format version.
CREATE TABLE engineering.commits (
    id            UUID        PRIMARY KEY,
    draft_id      UUID        NOT NULL,
    number        BIGINT      NOT NULL,
    title         TEXT        NOT NULL,
    author        TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    draft_version BIGINT      NOT NULL,
    content       JSONB       NOT NULL,
    CONSTRAINT fk_commits_draft_id FOREIGN KEY (draft_id) REFERENCES engineering.drafts(id) ON DELETE CASCADE,
    CONSTRAINT ck_commits_number CHECK (number > 0),
    CONSTRAINT uq_commits_draft_number UNIQUE (draft_id, number)
);
CREATE INDEX idx_commits_draft_number ON engineering.commits(draft_id, number DESC);

-- The Draft remembers what its last Save fixed, so a Save without changes is
-- refused and the next number needs no read of the commits.
ALTER TABLE engineering.drafts
    ADD COLUMN last_commit_number        BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN last_commit_draft_version BIGINT NOT NULL DEFAULT 0;

-- The Library Pins of a Project's Commit: its ProjectFile carries them as
-- provenance, and task 71 keeps a Library from being deleted while a live
-- Project pins one of its Releases. A Library's Commit has no pins.
CREATE TABLE engineering.commit_library_pins (
    commit_id      UUID NOT NULL,
    library_id     UUID NOT NULL,
    release_number INT  NOT NULL,
    CONSTRAINT fk_commit_library_pins_commit_id FOREIGN KEY (commit_id) REFERENCES engineering.commits(id) ON DELETE CASCADE,
    PRIMARY KEY (commit_id, library_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE engineering.commit_library_pins;
ALTER TABLE engineering.drafts
    DROP COLUMN last_commit_draft_version,
    DROP COLUMN last_commit_number;
DROP TABLE engineering.commits;
-- +goose StatementEnd
