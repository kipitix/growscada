-- +goose Up
-- +goose StatementBegin
-- A Commit is a saved state (task 88_2; ADR 0026): the documents the Drafts
-- store name the saved state of a Library an Import uses "saved_state", not
-- "commit", and their schema_version follows the project contract (0.4). The
-- rows, the numbers and the WidgetType copies stay as they are; only the key
-- changes.
--
-- The document's schema_version is stamped with the version of the shape it now
-- has (project 0.4): a document that carries saved_state but still says 0.3 is
-- refused when it is opened from a file — the validator rebuilds it at the
-- current version and compares the two documents whole, header included.
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

UPDATE engineering.saved_states
SET content = jsonb_set(content, '{project_file}', pg_temp.saved_state_project_file(content -> 'project_file'), true)
WHERE content ->> 'kind' = 'project' AND content -> 'project_file' IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The old key comes back over the same rows: this rename is a rename, nothing
-- about the documents' content is lost.
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

UPDATE engineering.saved_states
SET content = jsonb_set(content, '{project_file}', pg_temp.commit_project_file(content -> 'project_file'), true)
WHERE content ->> 'kind' = 'project' AND content -> 'project_file' IS NOT NULL;
-- +goose StatementEnd
