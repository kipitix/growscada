-- +goose Up
-- +goose StatementBegin
-- The Journal: the Runtime node's append-only record of a Project's
-- operational state (ADR 0019). One sequence of the server numbers every
-- entry, so seq is a position: the total order a reader catches up by. An
-- entry belongs to exactly one Project and goes with it (ADR 0006). The
-- record format is contract/record/v0; schema_version is written into every
-- row, so a reader never guesses the format of an entry.
CREATE SEQUENCE runtime.journal_seq;

CREATE TABLE runtime.journal (
    seq            BIGINT      PRIMARY KEY DEFAULT nextval('runtime.journal_seq'),
    project_id     UUID        NOT NULL,
    ts             TIMESTAMPTZ NOT NULL,
    type           TEXT        NOT NULL,
    payload        JSONB       NOT NULL,
    schema_version TEXT        NOT NULL,
    CONSTRAINT fk_runtime_journal_project_id FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT ck_runtime_journal_type CHECK (type IN ('tag_state', 'revision'))
);
CREATE INDEX idx_runtime_journal_project_seq ON runtime.journal(project_id, seq);

-- A state's seq becomes a Journal position, so the sequence continues above
-- every number already handed out: a client holding a state of the old
-- numbering would else drop the next change of its Tag as the older one.
-- is_called false makes the next nextval return this value.
SELECT setval('runtime.journal_seq', COALESCE(MAX(seq), 0) + 1, false) FROM runtime.tag_states;
DROP SEQUENCE runtime.tag_state_seq;

-- When the state was last written, changed or not: a write that repeats the
-- stored value and Quality appends no entry, and this is what keeps a stable
-- Device visibly alive (ADR 0019). NULL is the initial state of a Tag no
-- supplier has written yet.
ALTER TABLE runtime.tag_states ADD COLUMN last_write_at TIMESTAMPTZ;

-- The positions of the server-side subscribers that react to the Journal
-- (alerts, computations): they are the Runtime node's, as the Journal is, and
-- write their own position.
CREATE TABLE runtime.journal_cursors (
    subscriber TEXT   PRIMARY KEY,
    last_seq   BIGINT NOT NULL DEFAULT 0
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The Journal cannot be read by a schema that has no table for it: the
-- entries are removed with it, and a Tag state's seq goes back to being a
-- number of its own sequence, set above every seq already stored.
DROP TABLE runtime.journal_cursors;
DROP TABLE runtime.journal;
DROP SEQUENCE runtime.journal_seq;

ALTER TABLE runtime.tag_states DROP COLUMN last_write_at;

CREATE SEQUENCE runtime.tag_state_seq;
SELECT setval('runtime.tag_state_seq', COALESCE(MAX(seq), 0) + 1, false) FROM runtime.tag_states;
-- +goose StatementEnd
