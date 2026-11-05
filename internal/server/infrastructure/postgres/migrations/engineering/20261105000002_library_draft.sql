-- +goose Up
-- +goose StatementBegin
-- A Library's Draft is the Draft aggregate too (ADR 0016): its WidgetTypes are
-- entities of it and state its one Version, so they lose their own; the
-- discarding_to mark of the policy-driven Discard (ADR 0012) goes with the
-- policy, the Discard now being one transaction over the Draft.
ALTER TABLE engineering.widget_types DROP COLUMN version;
ALTER TABLE engineering.libraries DROP COLUMN discarding_to;

-- The Discard policy is gone: its queued jobs have nothing left to do, and a
-- job keyed by a library_discard_started event would fail forever.
DELETE FROM policy_jobs WHERE policy = 'discard_library';
DELETE FROM outbox WHERE type = 'library_discard_started';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE engineering.libraries ADD COLUMN discarding_to INTEGER;
ALTER TABLE engineering.widget_types ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd
