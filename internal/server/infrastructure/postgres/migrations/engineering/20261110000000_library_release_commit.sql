-- +goose Up
-- +goose StatementBegin
-- A Library Release is made from a Commit (task 72_5; ADR 0007, ADR 0021): it
-- names the Commit it published, the Author who published it, and its origin.
-- A Release older than Commits has no commit_number and origin 'legacy'; the
-- released_widget_types copies stay exactly as they were, because a Project
-- reads the pinned Release through them.
ALTER TABLE engineering.library_releases
    ADD COLUMN commit_number INTEGER,
    ADD COLUMN author        TEXT NOT NULL DEFAULT 'system',
    ADD COLUMN origin        TEXT NOT NULL DEFAULT 'legacy',
    ADD CONSTRAINT ck_library_releases_origin CHECK (origin IN ('commit', 'legacy')),
    ADD CONSTRAINT ck_library_releases_commit CHECK (origin <> 'commit' OR commit_number IS NOT NULL);

-- One Commit is published at most once. A legacy Release names no Commit, so
-- the index leaves it out.
CREATE UNIQUE INDEX uq_library_releases_commit
    ON engineering.library_releases(library_id, commit_number)
    WHERE commit_number IS NOT NULL AND origin = 'commit';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX engineering.uq_library_releases_commit;
ALTER TABLE engineering.library_releases
    DROP CONSTRAINT ck_library_releases_commit,
    DROP CONSTRAINT ck_library_releases_origin,
    DROP COLUMN origin,
    DROP COLUMN author,
    DROP COLUMN commit_number;
-- +goose StatementEnd
