-- +goose Up
-- +goose StatementBegin
-- The EditLock: the marker that one holder — today a Client, later a User —
-- currently holds one Draft for editing (ADR 0015). One row per Draft: the
-- Draft of a Project, or the Draft of a Library. The row stays when the lock
-- is free, so its version grows monotonically across holders (a taking, a
-- giving back, a takeover, an expiry each raise it) and a client can tell a
-- stale observation from a fresh one. Refreshing last_seen raises nothing.
--
-- The holder is a key the tab keeps for itself plus the name it declared:
-- coordination, not authentication. Both are NULL while the lock is free.
CREATE TABLE engineering.edit_locks (
    pk_id       SERIAL PRIMARY KEY,
    project_id  UUID        NULL,
    library_id  UUID        NULL,
    holder_key  UUID        NULL,
    holder_name TEXT        NULL,
    since       TIMESTAMPTZ NULL,
    last_seen   TIMESTAMPTZ NULL,
    version     BIGINT      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_edit_locks_project_id FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT fk_edit_locks_library_id FOREIGN KEY (library_id) REFERENCES engineering.libraries(id) ON DELETE CASCADE,
    -- Exactly one scope: a lock belongs to one Draft.
    CONSTRAINT ck_edit_locks_one_scope CHECK ((project_id IS NOT NULL) <> (library_id IS NOT NULL)),
    -- The holder is either absent as a whole, or present as a whole.
    CONSTRAINT ck_edit_locks_holder CHECK (
        (holder_key IS NULL AND holder_name IS NULL AND since IS NULL AND last_seen IS NULL)
        OR (holder_key IS NOT NULL AND holder_name IS NOT NULL AND since IS NOT NULL AND last_seen IS NOT NULL)
    )
);
CREATE UNIQUE INDEX uq_edit_locks_project ON engineering.edit_locks(project_id) WHERE project_id IS NOT NULL;
CREATE UNIQUE INDEX uq_edit_locks_library ON engineering.edit_locks(library_id) WHERE library_id IS NOT NULL;
-- The reaper reads the held locks alone.
CREATE INDEX idx_edit_locks_held ON engineering.edit_locks(last_seen) WHERE holder_key IS NOT NULL;
CREATE TRIGGER update_edit_locks_updated_at
    BEFORE UPDATE ON engineering.edit_locks
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE engineering.edit_locks;
-- +goose StatementEnd
