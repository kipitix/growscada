-- +goose Up
-- +goose StatementBegin
-- Per-user settings (task 81; ADR 0024): the way one person sees the server's
-- content, stored apart from the aggregates it names and orders. The first key
-- is 'library_order' — the order of the libraries in the selection page; a
-- setting is one row, so changing one is one transaction over one aggregate
-- (ADR 0008). The payload is a document so a new key needs no migration.
--
-- user_id has no foreign key yet: the User aggregate does not exist (task 49),
-- and until authentication exists (task 50) every request acts as the one user
-- the code names. The migration that adds the Users backfills this row to the
-- first one. organization_id is the tenant the view belongs to: the same
-- person keeps a different order per Organization.
CREATE TABLE public.user_settings (
    user_id         UUID        NOT NULL,
    organization_id UUID        NOT NULL,
    key             TEXT        NOT NULL,
    payload         JSONB       NOT NULL,
    version         INTEGER     NOT NULL DEFAULT 1,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, organization_id, key),
    CONSTRAINT fk_user_settings_organization_id
        FOREIGN KEY (organization_id) REFERENCES organizations(id),
    CONSTRAINT chk_user_settings_version CHECK (version > 0),
    CONSTRAINT chk_user_settings_payload CHECK (jsonb_typeof(payload) = 'object')
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE public.user_settings;
-- +goose StatementEnd
