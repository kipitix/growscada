-- +goose Up
-- +goose StatementBegin
-- The Draft is one aggregate per scope (ADR 0016): one row per Project or per
-- Library, carrying the one Version of the scope's content, the EditLock of
-- the Draft and the position of its DraftChange stack (ADR 0017). The content
-- stays in its own tables; the Draft composes it.
CREATE TABLE engineering.drafts (
    id             UUID        PRIMARY KEY,
    project_id     UUID        NULL,
    library_id     UUID        NULL,
    version        BIGINT      NOT NULL DEFAULT 1,
    -- The EditLock's own counter, raised only when its holder changes, so a
    -- Take over stays possible while the holder edits (ADR 0015, ADR 0016).
    lock_version   BIGINT      NOT NULL DEFAULT 1,
    holder_key     UUID        NULL,
    holder_name    TEXT        NULL,
    held_since     TIMESTAMPTZ NULL,
    last_seen      TIMESTAMPTZ NULL,
    stack_position BIGINT      NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_drafts_project_id FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT fk_drafts_library_id FOREIGN KEY (library_id) REFERENCES engineering.libraries(id) ON DELETE CASCADE,
    -- Exactly one scope: a Draft belongs to one Project or one Library.
    CONSTRAINT ck_drafts_one_scope CHECK ((project_id IS NOT NULL) <> (library_id IS NOT NULL)),
    -- The holder is either absent as a whole, or present as a whole.
    CONSTRAINT ck_drafts_holder CHECK (
        (holder_key IS NULL AND holder_name IS NULL AND held_since IS NULL AND last_seen IS NULL)
        OR (holder_key IS NOT NULL AND holder_name IS NOT NULL AND held_since IS NOT NULL AND last_seen IS NOT NULL)
    )
);
CREATE UNIQUE INDEX uq_drafts_project ON engineering.drafts(project_id) WHERE project_id IS NOT NULL;
CREATE UNIQUE INDEX uq_drafts_library ON engineering.drafts(library_id) WHERE library_id IS NOT NULL;
-- The reaper reads the held Drafts alone.
CREATE INDEX idx_drafts_held ON engineering.drafts(last_seen) WHERE holder_key IS NOT NULL;
CREATE TRIGGER update_drafts_updated_at
    BEFORE UPDATE ON engineering.drafts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Every existing Project and Library gets its Draft; a new one gets its row on
-- first access (the repository inserts it when it writes).
INSERT INTO engineering.drafts (id, project_id)
    SELECT gen_random_uuid(), id FROM projects;
INSERT INTO engineering.drafts (id, library_id)
    SELECT gen_random_uuid(), id FROM engineering.libraries;

-- One DraftChange per successful write of the Draft's content (ADR 0017):
-- the full before and after states of the entities it touched, an internal
-- encoding with a format version, and a short description for undo/redo.
CREATE TABLE engineering.draft_changes (
    draft_id    UUID        NOT NULL,
    seq         BIGINT      NOT NULL,
    description TEXT        NOT NULL,
    before      JSONB       NOT NULL,
    after       JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_draft_changes_draft_id FOREIGN KEY (draft_id) REFERENCES engineering.drafts(id) ON DELETE CASCADE,
    CONSTRAINT ck_draft_changes_seq CHECK (seq > 0),
    PRIMARY KEY (draft_id, seq)
);

-- The EditLock is a field of the Draft's row now: its rows are not moved (the
-- lock data is not carried over — every Draft is free after this migration
-- and its tab checks the lock out again).
DROP TABLE engineering.edit_locks;

-- The content is versioned as a whole: its entities lose their own versions.
ALTER TABLE engineering.tags DROP COLUMN version;
ALTER TABLE engineering.scenes DROP COLUMN version;
ALTER TABLE engineering.devices DROP COLUMN version;
ALTER TABLE engineering.environments DROP COLUMN version;
ALTER TABLE engineering.environment_devices DROP COLUMN version;
ALTER TABLE engineering.library_pins DROP COLUMN version;

-- The transitional policies are done by the Draft's own transaction; their
-- waiting jobs have nothing left to do.
DELETE FROM policy_jobs WHERE policy IN ('reconcile_widgets_with_pins', 'unbind_deleted_tag', 'discard_project');
DELETE FROM outbox WHERE type = 'project_discard_started';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE engineering.library_pins ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE engineering.environment_devices ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE engineering.environments ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE engineering.devices ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE engineering.scenes ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE engineering.tags ADD COLUMN version INTEGER NOT NULL DEFAULT 0;

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
    CONSTRAINT ck_edit_locks_one_scope CHECK ((project_id IS NOT NULL) <> (library_id IS NOT NULL)),
    CONSTRAINT ck_edit_locks_holder CHECK (
        (holder_key IS NULL AND holder_name IS NULL AND since IS NULL AND last_seen IS NULL)
        OR (holder_key IS NOT NULL AND holder_name IS NOT NULL AND since IS NOT NULL AND last_seen IS NOT NULL)
    )
);
CREATE UNIQUE INDEX uq_edit_locks_project ON engineering.edit_locks(project_id) WHERE project_id IS NOT NULL;
CREATE UNIQUE INDEX uq_edit_locks_library ON engineering.edit_locks(library_id) WHERE library_id IS NOT NULL;
CREATE INDEX idx_edit_locks_held ON engineering.edit_locks(last_seen) WHERE holder_key IS NOT NULL;
CREATE TRIGGER update_edit_locks_updated_at
    BEFORE UPDATE ON engineering.edit_locks
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TABLE engineering.draft_changes;
DROP TABLE engineering.drafts;
-- +goose StatementEnd
