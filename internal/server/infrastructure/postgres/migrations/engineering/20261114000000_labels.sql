-- +goose Up
-- +goose StatementBegin
-- Labels: a named mark with an optional color an Engineer puts on saved states
-- to group and filter them (task 85_2; ADR 0025). Logically a label belongs to
-- the Organization; physically the table sits in engineering because only the
-- Engineering node reads and writes it and, from task 85_3 on, its assignments
-- name engineering.commits. The API is server-wide while one Organization
-- exists: `organization_id` is written from the first day, so a second tenant
-- changes only the route and the lookup.
CREATE TABLE engineering.labels (
    id              UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    color           TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_labels_name CHECK (name = btrim(name) AND length(name) BETWEEN 1 AND 100),
    CONSTRAINT ck_labels_color CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$')
);

-- A name is unique in the Organization without regard to case (`Prod` and
-- `prod` are one name). The expression index is both the constraint and the
-- read order's index.
CREATE UNIQUE INDEX uq_labels_organization_name
    ON engineering.labels (organization_id, lower(name));
CREATE INDEX idx_labels_organization_name
    ON engineering.labels (organization_id, lower(name));

-- Two starter labels, ordinary in every way: they can be renamed, recolored
-- and deleted, and a later migration never recreates them. They only name the
-- habit of marking a saved state ready for Import or Deploy; no operation
-- depends on them (ADR 0025).
INSERT INTO engineering.labels (id, organization_id, name, color, created_at)
SELECT v.id, o.id, v.name, NULL, now()
FROM organizations o
CROSS JOIN (VALUES
    ('f1000000-0000-4000-8000-000000000001'::uuid, 'ready_for_import'),
    ('f1000000-0000-4000-8000-000000000002'::uuid, 'ready_for_deploy')
) AS v(id, name)
ON CONFLICT DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE engineering.labels;
-- +goose StatementEnd
