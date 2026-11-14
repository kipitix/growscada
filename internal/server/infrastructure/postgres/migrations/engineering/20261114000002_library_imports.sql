-- +goose Up
-- +goose StatementBegin
-- A Project no longer pins a Library Release but imports a saved state — a
-- Commit — of the Library (task 85_6). The rows, the provenance and the
-- documents are rewritten; a legacy Release (one without a Commit) is dropped
-- with the imports and the Widgets of its WidgetTypes. This is a deliberate
-- loss of data, recorded in the CHANGELOG.

-- The saved state each commit-backed Release published: (library, release) →
-- commit. A legacy Release is one without a Commit.
CREATE TEMP TABLE release_commits ON COMMIT DROP AS
    SELECT library_id, number AS release_number, commit_number
    FROM engineering.library_releases
    WHERE commit_number IS NOT NULL;

CREATE TEMP TABLE legacy_releases ON COMMIT DROP AS
    SELECT library_id, number AS release_number
    FROM engineering.library_releases
    WHERE commit_number IS NULL;

-- The WidgetTypes of the legacy Releases: the copies go with the Releases and
-- so do the Widgets of them, in a Draft and in a document.
CREATE TEMP TABLE legacy_release_types ON COMMIT DROP AS
    SELECT lr.library_id, lr.number AS release_number, rwt.id AS type_id
    FROM engineering.library_releases lr
    JOIN engineering.released_widget_types rwt
      ON rwt.library_id = lr.library_id AND rwt.release_number = lr.number
    WHERE lr.commit_number IS NULL;

-- The Drafts whose content changes: those importing a legacy Release, and those
-- holding a Widget of a WidgetType only a legacy Release held. Their Version is
-- raised below, so they report unsaved changes against their last saved state
-- (task 74).
CREATE TEMP TABLE changed_project_drafts ON COMMIT DROP AS
    SELECT DISTINCT li.project_id
    FROM engineering.library_pins li
    JOIN legacy_releases lr
      ON lr.library_id = li.library_id AND lr.release_number = li.release_number
    UNION
    SELECT DISTINCT w.project_id
    FROM engineering.widgets w
    WHERE w.type_id IN (SELECT type_id FROM legacy_release_types);

-- The Widgets of a WidgetType no saved state holds any more: a Draft cannot
-- keep them, and a Deploy of it would fail. They go, with their PortBindings
-- (which are columns of the Widget's row).
DELETE FROM engineering.widgets w
WHERE w.type_id IN (SELECT type_id FROM legacy_release_types);

-- The imports: a pin becomes the import of the Commit its Release published.
ALTER TABLE engineering.library_pins RENAME TO library_imports;
ALTER TABLE engineering.library_imports RENAME COLUMN release_number TO commit_number;
ALTER TABLE engineering.library_imports ADD COLUMN commit_id UUID;
UPDATE engineering.library_imports li
SET commit_id = c.id
FROM engineering.library_releases lr,
     engineering.drafts d,
     engineering.commits c
WHERE lr.library_id = li.library_id
  AND lr.number = li.commit_number
  AND lr.commit_number IS NOT NULL
  AND d.library_id = lr.library_id
  AND c.draft_id = d.id
  AND c.number = lr.commit_number;
DELETE FROM engineering.library_imports WHERE commit_id IS NULL;
ALTER TABLE engineering.library_imports ALTER COLUMN commit_id SET NOT NULL;
CREATE INDEX idx_library_imports_commit_id ON engineering.library_imports(commit_id);
ALTER TABLE engineering.library_imports RENAME CONSTRAINT fk_library_pins_project_id TO fk_library_imports_project_id;
ALTER TABLE engineering.library_imports RENAME CONSTRAINT fk_library_pins_library_id TO fk_library_imports_library_id;
ALTER TABLE engineering.library_imports RENAME CONSTRAINT uq_library_pins_project_library TO uq_library_imports_project_library;
ALTER TABLE engineering.library_imports DROP CONSTRAINT fk_library_pins_release;
ALTER TABLE engineering.library_imports ADD CONSTRAINT fk_library_imports_commit_id
    FOREIGN KEY (commit_id) REFERENCES engineering.commits(id);
COMMENT ON TABLE engineering.library_imports IS
    'A Project''s Library Imports (task 85_6): the saved state — the Commit — of a Library its Draft uses';

-- The provenance of a Project's Commit: its ProjectFile carries the imports.
ALTER TABLE engineering.commit_library_pins RENAME TO commit_library_imports;
ALTER TABLE engineering.commit_library_imports RENAME CONSTRAINT fk_commit_library_pins_commit_id TO fk_commit_library_imports_commit_id;
DELETE FROM engineering.commit_library_imports cli
USING legacy_releases lr
WHERE lr.library_id = cli.library_id AND lr.release_number = cli.release_number;
UPDATE engineering.commit_library_imports cli
SET release_number = rc.commit_number
FROM release_commits rc
WHERE rc.library_id = cli.library_id AND rc.release_number = cli.release_number;
ALTER TABLE engineering.commit_library_imports RENAME COLUMN release_number TO commit_number;

-- The provenance of a Project's Release: the table goes with the Release in
-- task 85_7, so only its rows follow the new numbering here and a legacy one
-- is dropped.
DELETE FROM engineering.project_release_library_pins prp
USING legacy_releases lr
WHERE lr.library_id = prp.library_id AND lr.release_number = prp.release_number;
UPDATE engineering.project_release_library_pins prp
SET release_number = rc.commit_number
FROM release_commits rc
WHERE rc.library_id = prp.library_id AND rc.release_number = prp.release_number;

-- The documents: every ProjectFile names library_imports (a library and the
-- number of its saved state) rather than library_pins (a library and the number
-- of its Release), the copies of the dropped WidgetTypes go and so do the
-- Widgets of them. It applies to the saved states of the Drafts, to the
-- Releases that copied them and to every Deployment — the running one included:
-- Operation stops showing those Widgets at once. This is the one rewrite of
-- Commit and Deployment documents, an exception recorded in ADR 0021.
--
-- The document's schema_version is stamped with the version of the shape it now
-- has (project 0.3): a document that carries library_imports but still says 0.2
-- is refused when it is opened from a file — the validator rebuilds it at the
-- current version and compares the two documents whole, header included.
CREATE FUNCTION pg_temp.imported_project_file(pf jsonb) RETURNS jsonb AS $$
    SELECT jsonb_set(
        jsonb_set(
            jsonb_set(
                jsonb_set(
                    pf #- '{library_pins}',
                    '{library_imports}',
                    COALESCE((
                        SELECT jsonb_agg(jsonb_build_object(
                            'library_id', e ->> 'library_id',
                            'commit', rc.commit_number))
                        FROM jsonb_array_elements(COALESCE(pf -> 'library_pins', '[]'::jsonb)) e
                        JOIN release_commits rc
                          ON rc.library_id = (e ->> 'library_id')::uuid
                         AND rc.release_number = (e ->> 'release')::int
                    ), '[]'::jsonb),
                    true),
                '{widget_types}',
                COALESCE((
                    SELECT jsonb_agg(wt)
                    FROM jsonb_array_elements(COALESCE(pf -> 'widget_types', '[]'::jsonb)) wt
                    WHERE NOT EXISTS (
                        SELECT 1 FROM legacy_release_types lrt
                        WHERE lrt.type_id = (wt ->> 'id')::uuid)
                ), '[]'::jsonb),
                true),
            '{scenes}',
            COALESCE((
                SELECT jsonb_agg(jsonb_set(sc, '{widgets}', COALESCE((
                    SELECT jsonb_agg(w)
                    FROM jsonb_array_elements(COALESCE(sc -> 'widgets', '[]'::jsonb)) w
                    WHERE NOT EXISTS (
                        SELECT 1 FROM legacy_release_types lrt
                        WHERE lrt.type_id = (w ->> 'type_id')::uuid)
                ), '[]'::jsonb), true))
                FROM jsonb_array_elements(COALESCE(pf -> 'scenes', '[]'::jsonb)) sc
            ), '[]'::jsonb),
            true),
        '{schema_version}', to_jsonb('0.3'::text), true)
$$ LANGUAGE sql STABLE;

UPDATE engineering.commits
SET content = jsonb_set(content, '{project_file}', pg_temp.imported_project_file(content -> 'project_file'), true)
WHERE content ->> 'kind' = 'project' AND content -> 'project_file' IS NOT NULL;

UPDATE engineering.project_releases
SET content = jsonb_set(content, '{project_file}', pg_temp.imported_project_file(content -> 'project_file'), true)
WHERE content -> 'project_file' IS NOT NULL;

-- A fresh database has no Runtime node yet (its chain runs after this one) and
-- so no Deployment to rewrite; a running server has them all.
DO $$
BEGIN
    IF to_regclass('runtime.deployments') IS NOT NULL THEN
        UPDATE runtime.deployments SET config = pg_temp.imported_project_file(config);
    END IF;
END $$;

-- A legacy Release goes with its copies (the cascade) and its provenance (the
-- rows above).
DELETE FROM engineering.library_releases lr
USING legacy_releases l
WHERE lr.library_id = l.library_id AND lr.number = l.release_number;

-- The DraftChange stack names Library Pins by number of a Release: no snapshot
-- applies after this change, so the stacks go. Draft content, saved states and
-- Deployments are untouched.
DELETE FROM engineering.draft_changes;
UPDATE engineering.drafts SET stack_position = 0;

-- A Draft whose content changed (an import went, or Widgets of a dropped
-- WidgetType) reports unsaved changes: its Version is above its Clean Version
-- (task 74) and its clients do not take it for its last saved state.
UPDATE engineering.drafts d
SET version = d.version + 1, clean_version = 0, updated_at = now()
WHERE d.project_id IN (SELECT project_id FROM changed_project_drafts);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The shape comes back; the data does not. The legacy Releases this migration
-- dropped, the imports of them and the Widgets of their WidgetTypes are gone,
-- and the documents keep the library_imports form they were rewritten to: a
-- Down returns no lost row, and an older server reads no document of it. A
-- database that must go back is restored from a backup.
ALTER TABLE engineering.library_imports DROP CONSTRAINT fk_library_imports_commit_id;
ALTER TABLE engineering.library_imports DROP COLUMN commit_id;
UPDATE engineering.library_imports li
SET commit_number = lr.number
FROM engineering.library_releases lr
WHERE lr.library_id = li.library_id AND lr.commit_number = li.commit_number;
-- An import of a saved state this server never published as a Release has no
-- pin to become: it goes before the foreign key to the Releases comes back, or
-- a Down of a database that imported such a state would fail on that key.
DELETE FROM engineering.library_imports li
WHERE NOT EXISTS (
    SELECT 1 FROM engineering.library_releases lr
    WHERE lr.library_id = li.library_id AND lr.number = li.commit_number);
ALTER TABLE engineering.library_imports RENAME CONSTRAINT uq_library_imports_project_library TO uq_library_pins_project_library;
ALTER TABLE engineering.library_imports RENAME CONSTRAINT fk_library_imports_library_id TO fk_library_pins_library_id;
ALTER TABLE engineering.library_imports RENAME CONSTRAINT fk_library_imports_project_id TO fk_library_pins_project_id;
ALTER TABLE engineering.library_imports ADD CONSTRAINT fk_library_pins_release
    FOREIGN KEY (library_id, commit_number) REFERENCES engineering.library_releases(library_id, number);
ALTER TABLE engineering.library_imports RENAME COLUMN commit_number TO release_number;
ALTER TABLE engineering.library_imports RENAME TO library_pins;
COMMENT ON TABLE engineering.library_pins IS NULL;
ALTER TABLE engineering.commit_library_imports RENAME COLUMN commit_number TO release_number;
UPDATE engineering.commit_library_imports cli
SET release_number = lr.number
FROM engineering.library_releases lr
WHERE lr.library_id = cli.library_id AND lr.commit_number = cli.release_number;
ALTER TABLE engineering.commit_library_imports RENAME CONSTRAINT fk_commit_library_imports_commit_id TO fk_commit_library_pins_commit_id;
ALTER TABLE engineering.commit_library_imports RENAME TO commit_library_pins;
UPDATE engineering.project_release_library_pins prp
SET release_number = lr.number
FROM engineering.library_releases lr
WHERE lr.library_id = prp.library_id AND lr.commit_number = prp.release_number;
-- +goose StatementEnd
