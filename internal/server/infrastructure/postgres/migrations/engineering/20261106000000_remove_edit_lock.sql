-- +goose Up
-- +goose StatementBegin
-- The Draft is edited without an EditLock (ADR 0022): the Version of its
-- content is the only concurrency token, so the lock fields of its row are
-- gone. A Draft locked before the upgrade is free after it.
ALTER TABLE engineering.drafts DROP CONSTRAINT IF EXISTS ck_drafts_holder;
DROP INDEX IF EXISTS engineering.idx_drafts_held;
ALTER TABLE engineering.drafts
    DROP COLUMN holder_key,
    DROP COLUMN holder_name,
    DROP COLUMN held_since,
    DROP COLUMN last_seen,
    DROP COLUMN lock_version;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE engineering.drafts
    ADD COLUMN lock_version BIGINT      NOT NULL DEFAULT 1,
    ADD COLUMN holder_key   UUID        NULL,
    ADD COLUMN holder_name  TEXT        NULL,
    ADD COLUMN held_since   TIMESTAMPTZ NULL,
    ADD COLUMN last_seen    TIMESTAMPTZ NULL;
ALTER TABLE engineering.drafts
    ADD CONSTRAINT ck_drafts_holder CHECK (
        (holder_key IS NULL AND holder_name IS NULL AND held_since IS NULL AND last_seen IS NULL)
        OR (holder_key IS NOT NULL AND holder_name IS NOT NULL AND held_since IS NOT NULL AND last_seen IS NOT NULL)
    );
CREATE INDEX idx_drafts_held ON engineering.drafts(last_seen) WHERE holder_key IS NOT NULL;
-- +goose StatementEnd
