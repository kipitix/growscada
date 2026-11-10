-- +goose Up
-- +goose StatementBegin
-- The Clean Version of a Draft: the Version at which its content last equalled
-- its last Commit's (task 74; ADR 0016, amendment 2026-11-10). The
-- unsaved-changes flag is Version above it, and a Save is refused at it. 0 means
-- the content never equalled a Commit's: the Draft was never saved.
ALTER TABLE engineering.drafts ADD COLUMN clean_version INTEGER NOT NULL DEFAULT 0;

-- A Draft that sits at the Version its last Commit fixed is equal to it: its
-- Clean Version is that Version. Every other one keeps 0 — "there was no Clean
-- Version" — and honestly reports unsaved changes until a Save or a Discard.
UPDATE engineering.drafts SET clean_version = version WHERE last_commit_draft_version = version;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE engineering.drafts DROP COLUMN clean_version;
-- +goose StatementEnd
