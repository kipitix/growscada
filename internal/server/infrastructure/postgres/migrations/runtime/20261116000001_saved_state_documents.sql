-- +goose Up
-- +goose StatementBegin
-- A Commit is a saved state (task 88_2; ADR 0026): the documents of the Runtime
-- node name the saved state a Deployment made running "saved_state_number", not
-- "commit_number", and their schema_version follows the record (0.7) and the
-- project (0.4) contracts. The rows, the numbers and the documents' content
-- stay as they are; only the key changes.
--
-- schema_version is raised on the rows whose payload changed alone: a Journal
-- entry or a Checkpoint of a Deployment from a file names no saved state, and
-- its shape is the same as before.

-- The running Deployment, and every Deployment document: the imports of the
-- ProjectFile name a saved state, and the document says project 0.4.
-- A fresh database has no Runtime node yet (its chain runs after the
-- engineering one) and so no Deployment to rewrite; a running server has them
-- all.
CREATE OR REPLACE FUNCTION pg_temp.saved_state_project_file(pf jsonb) RETURNS jsonb AS $$
    SELECT jsonb_set(
        jsonb_set(
            pf,
            '{library_imports}',
            COALESCE((
                SELECT jsonb_agg(jsonb_build_object(
                    'library_id', e -> 'library_id',
                    'saved_state', e -> 'commit'))
                FROM jsonb_array_elements(COALESCE(pf -> 'library_imports', '[]'::jsonb)) e
            ), '[]'::jsonb),
            true),
        '{schema_version}', to_jsonb('0.4'::text), true)
$$ LANGUAGE sql STABLE;

UPDATE runtime.deployments SET config = pg_temp.saved_state_project_file(config);

-- The Journal entries of a Deployment: the record names the saved state it
-- deployed.
UPDATE runtime.journal
SET payload = (payload - 'commit_number'::text)
        || jsonb_build_object('saved_state_number', payload -> 'commit_number'),
    schema_version = '0.7'
WHERE type = 'deployment' AND payload ? 'commit_number';

-- A Checkpoint holds the same Deployment reference: it is rewritten with the
-- Journal entry that names it, so the two never disagree.
UPDATE runtime.checkpoints
SET payload = jsonb_set(
        payload,
        '{deployment}',
        ((payload -> 'deployment') - 'commit_number'::text)
            || jsonb_build_object('saved_state_number', payload -> 'deployment' -> 'commit_number'),
        true),
    schema_version = '0.7'
WHERE jsonb_typeof(payload -> 'deployment') = 'object'
  AND payload -> 'deployment' ? 'commit_number';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The old names come back over the same rows: this rename is a rename, nothing
-- about the records is lost.
CREATE OR REPLACE FUNCTION pg_temp.commit_project_file(pf jsonb) RETURNS jsonb AS $$
    SELECT jsonb_set(
        jsonb_set(
            pf,
            '{library_imports}',
            COALESCE((
                SELECT jsonb_agg(jsonb_build_object(
                    'library_id', e -> 'library_id',
                    'commit', e -> 'saved_state'))
                FROM jsonb_array_elements(COALESCE(pf -> 'library_imports', '[]'::jsonb)) e
            ), '[]'::jsonb),
            true),
        '{schema_version}', to_jsonb('0.3'::text), true)
$$ LANGUAGE sql STABLE;

UPDATE runtime.deployments SET config = pg_temp.commit_project_file(config);

UPDATE runtime.journal
SET payload = (payload - 'saved_state_number'::text)
        || jsonb_build_object('commit_number', payload -> 'saved_state_number'),
    schema_version = '0.6'
WHERE type = 'deployment' AND payload ? 'saved_state_number';

UPDATE runtime.checkpoints
SET payload = jsonb_set(
        payload,
        '{deployment}',
        ((payload -> 'deployment') - 'saved_state_number'::text)
            || jsonb_build_object('commit_number', payload -> 'deployment' -> 'saved_state_number'),
        true),
    schema_version = '0.6'
WHERE jsonb_typeof(payload -> 'deployment') = 'object'
  AND payload -> 'deployment' ? 'saved_state_number';
-- +goose StatementEnd
