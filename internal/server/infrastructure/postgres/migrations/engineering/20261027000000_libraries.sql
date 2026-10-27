-- +goose Up
-- +goose StatementBegin
-- WidgetTypes move into Libraries (ADR 0007, ADR 0012): a Library is owned by
-- an Organization, outside every Project; its Draft is its WidgetTypes, each
-- with a name unique in it; a Library Release is a numbered copy of the
-- Draft, never changed nor removed but with its Library; a Library Pin is the
-- release of one Library a Project uses. A Widget keeps only its
-- WidgetType's ID: its WidgetType is the copy of the release its Project
-- pins, so the foreign key to the Draft goes.
--
-- The WidgetTypes stored so far become the Library "Main" of the first
-- Organization (its ID fixed, so the seed data can name it), released once
-- and pinned in every live Project. Names taken twice get a suffix.

-- A deleted library is only marked (deleted_at) until the policy that removes
-- it; its name is free at once. discarding_to is the release a Discard is
-- resetting the Draft to, NULL when none is under way.
CREATE TABLE engineering.libraries (
    pk_id           SERIAL PRIMARY KEY,
    id              UUID        NOT NULL UNIQUE,
    organization_id UUID        NOT NULL,
    name            TEXT        NOT NULL,
    discarding_to   INTEGER,
    version         INTEGER     NOT NULL DEFAULT 0,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_libraries_organization_id FOREIGN KEY (organization_id) REFERENCES organizations(id)
);
CREATE UNIQUE INDEX uq_libraries_organization_live_name ON engineering.libraries (organization_id, name) WHERE deleted_at IS NULL;
CREATE TRIGGER update_libraries_updated_at
    BEFORE UPDATE ON engineering.libraries
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
INSERT INTO engineering.libraries (id, organization_id, name, version)
    SELECT 'd0000000-0000-4000-8000-000000000001', id, 'Main', 1 FROM organizations ORDER BY pk_id LIMIT 1;

ALTER TABLE engineering.widget_types ADD COLUMN library_id UUID;
UPDATE engineering.widget_types SET library_id = 'd0000000-0000-4000-8000-000000000001';
-- A suffix already taken by another name is skipped.
DO $$
DECLARE
    d RECORD;
    k INTEGER;
BEGIN
    FOR d IN SELECT id, name, n FROM (SELECT id, name, pk_id, row_number() OVER (PARTITION BY name ORDER BY pk_id) AS n
                                     FROM engineering.widget_types) t
             WHERE n > 1 ORDER BY pk_id LOOP
        k := d.n;
        WHILE EXISTS (SELECT 1 FROM engineering.widget_types WHERE name = d.name || ' (' || k || ')') LOOP
            k := k + 1;
        END LOOP;
        UPDATE engineering.widget_types SET name = d.name || ' (' || k || ')' WHERE id = d.id;
    END LOOP;
END $$;
ALTER TABLE engineering.widget_types
    ALTER COLUMN library_id SET NOT NULL,
    ADD CONSTRAINT fk_widget_types_library_id FOREIGN KEY (library_id) REFERENCES engineering.libraries(id) ON DELETE CASCADE,
    ADD CONSTRAINT uq_widget_types_library_name UNIQUE (library_id, name);

-- A release's number is the latest of its Library plus one: the unique key
-- makes a concurrent Release lose and retry.
CREATE TABLE engineering.library_releases (
    pk_id      SERIAL PRIMARY KEY,
    library_id UUID        NOT NULL,
    number     INTEGER     NOT NULL,
    note       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_library_releases_library_id FOREIGN KEY (library_id) REFERENCES engineering.libraries(id) ON DELETE CASCADE,
    CONSTRAINT uq_library_releases_library_number UNIQUE (library_id, number),
    CONSTRAINT chk_library_releases_number CHECK (number > 0)
);

-- The copies of the WidgetTypes of a release, under their own IDs.
CREATE TABLE engineering.released_widget_types (
    pk_id           SERIAL PRIMARY KEY,
    library_id      UUID        NOT NULL,
    release_number  INTEGER     NOT NULL,
    id              UUID        NOT NULL,
    name            TEXT        NOT NULL,
    html_template   TEXT        NOT NULL,
    script          TEXT        NOT NULL,
    script_language VARCHAR(50) NOT NULL,
    default_width   INTEGER     NOT NULL,
    default_height  INTEGER     NOT NULL,
    input_ports     JSONB       NOT NULL DEFAULT '[]',
    CONSTRAINT fk_released_widget_types_release FOREIGN KEY (library_id, release_number)
        REFERENCES engineering.library_releases(library_id, number) ON DELETE CASCADE,
    CONSTRAINT uq_released_widget_types_release_id UNIQUE (library_id, release_number, id)
);
CREATE INDEX idx_released_widget_types_id ON engineering.released_widget_types(id);

-- A pin is content of its Project's Draft: no CASCADE from projects, so a
-- project is removed only once its pins are; none from libraries either, so
-- a library is removed only once no project pins it.
CREATE TABLE engineering.library_pins (
    pk_id          SERIAL PRIMARY KEY,
    project_id     UUID    NOT NULL,
    library_id     UUID    NOT NULL,
    release_number INTEGER NOT NULL,
    version        INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT fk_library_pins_project_id FOREIGN KEY (project_id) REFERENCES projects(id),
    CONSTRAINT fk_library_pins_library_id FOREIGN KEY (library_id) REFERENCES engineering.libraries(id),
    CONSTRAINT fk_library_pins_release FOREIGN KEY (library_id, release_number)
        REFERENCES engineering.library_releases(library_id, number),
    CONSTRAINT uq_library_pins_project_library UNIQUE (project_id, library_id)
);

INSERT INTO engineering.library_releases (library_id, number, note)
    SELECT id, 1, 'WidgetTypes before Libraries' FROM engineering.libraries;
INSERT INTO engineering.released_widget_types
        (library_id, release_number, id, name, html_template, script, script_language, default_width, default_height, input_ports)
    SELECT library_id, 1, id, name, html_template, script, script_language, default_width, default_height, input_ports
    FROM engineering.widget_types ORDER BY pk_id;
INSERT INTO engineering.library_pins (project_id, library_id, release_number, version)
    SELECT p.id, l.id, 1, 1 FROM projects p CROSS JOIN engineering.libraries l
    WHERE p.deleted_at IS NULL ORDER BY p.pk_id;

ALTER TABLE engineering.widgets DROP CONSTRAINT fk_widgets_type_id;
CREATE INDEX idx_widgets_project_id ON engineering.widgets(project_id);

-- The payload of a WidgetType's event names its Library.
UPDATE outbox
    SET payload = payload || jsonb_build_object('library_id', 'd0000000-0000-4000-8000-000000000001')
    WHERE type LIKE 'widget\_type\_%';

-- The Widgets are brought into line with the pinned releases, no longer with
-- a WidgetType of the Draft: a waiting job of one Scene keeps its Scene, one
-- of every Scene of a WidgetType becomes a job of every Scene of each live
-- Project holding a Widget of it (the release copies the WidgetType as it
-- is, but checks no binding). The jobs of one Scene or Project of several
-- WidgetTypes merge into one.
INSERT INTO policy_jobs (policy, job_key)
    SELECT 'reconcile_widgets_with_pins', regexp_replace(job_key, '^widget_type:[0-9a-f-]+/', '')
    FROM policy_jobs WHERE policy = 'reconcile_widgets_with_widget_type' AND job_key LIKE '%/scene:%'
    UNION
    SELECT 'reconcile_widgets_with_pins', 'project:' || w.project_id
    FROM policy_jobs j
    JOIN engineering.widgets w ON j.job_key = 'widget_type:' || w.type_id
    JOIN projects p ON p.id = w.project_id AND p.deleted_at IS NULL
    WHERE j.policy = 'reconcile_widgets_with_widget_type'
ON CONFLICT (policy, job_key) DO UPDATE
    SET generation = policy_jobs.generation + 1, next_attempt_at = now();
DELETE FROM policy_jobs WHERE policy = 'reconcile_widgets_with_widget_type';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The WidgetTypes of every Library go back to one set: names unique per
-- Library may collide, and the down fails then. A Widget whose WidgetType is
-- no longer in the Draft (removed since its release) goes, as the foreign key
-- coming back refuses it; its Scene's version is bumped first.
DELETE FROM outbox WHERE type LIKE 'library\_%';
-- The older server reconciles a Scene on the next event of its Widgets.
DELETE FROM policy_jobs WHERE policy IN ('remove_deleted_library', 'discard_library', 'reconcile_widgets_with_pins');
UPDATE outbox SET payload = payload - 'library_id' WHERE type LIKE 'widget\_type\_%';

UPDATE engineering.scenes SET version = version + 1
    WHERE id IN (SELECT scene_id FROM engineering.widgets w
                 WHERE NOT EXISTS (SELECT 1 FROM engineering.widget_types t WHERE t.id = w.type_id));
DELETE FROM engineering.widgets w
    WHERE NOT EXISTS (SELECT 1 FROM engineering.widget_types t WHERE t.id = w.type_id);
DROP INDEX engineering.idx_widgets_project_id;
ALTER TABLE engineering.widgets
    ADD CONSTRAINT fk_widgets_type_id FOREIGN KEY (type_id) REFERENCES engineering.widget_types(id) ON DELETE RESTRICT;

DROP TABLE engineering.library_pins;
DROP TABLE engineering.released_widget_types;
DROP TABLE engineering.library_releases;
DELETE FROM engineering.widget_types
    WHERE library_id IN (SELECT id FROM engineering.libraries WHERE deleted_at IS NOT NULL);
ALTER TABLE engineering.widget_types
    DROP CONSTRAINT uq_widget_types_library_name,
    DROP CONSTRAINT fk_widget_types_library_id,
    DROP COLUMN library_id;
DROP TABLE engineering.libraries;
-- +goose StatementEnd
