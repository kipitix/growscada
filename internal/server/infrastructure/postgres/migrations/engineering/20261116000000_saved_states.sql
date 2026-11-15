-- +goose Up
-- +goose StatementBegin
-- A Commit is a saved state (task 88_1; ADR 0026): the tables, the columns and
-- the constraints the code names take the name the domain uses. Nothing
-- changes shape: the rows, the numbers and the documents stay as they are, and
-- the JSONB documents (`saved_states.content`, the Deployment documents, the
-- Journal and the Checkpoint payloads) are rewritten with the contracts
-- (task 88_2).
ALTER TABLE engineering.commits RENAME TO saved_states;
ALTER TABLE engineering.saved_states RENAME CONSTRAINT fk_commits_draft_id TO fk_saved_states_draft_id;
ALTER TABLE engineering.saved_states RENAME CONSTRAINT ck_commits_number TO ck_saved_states_number;
ALTER TABLE engineering.saved_states RENAME CONSTRAINT uq_commits_draft_number TO uq_saved_states_draft_number;
ALTER TABLE engineering.saved_states RENAME CONSTRAINT commits_pkey TO saved_states_pkey;
ALTER INDEX engineering.idx_commits_draft_number RENAME TO idx_saved_states_draft_number;

ALTER TABLE engineering.drafts
    RENAME COLUMN last_commit_number TO last_saved_state_number;
ALTER TABLE engineering.drafts
    RENAME COLUMN last_commit_draft_version TO last_saved_state_draft_version;

-- The labels a saved state carries (ADR 0025): the code names the table and
-- the saved state's column, so they follow; the label's own column keeps its
-- name.
ALTER TABLE engineering.commit_labels RENAME TO saved_state_labels;
ALTER TABLE engineering.saved_state_labels RENAME CONSTRAINT commit_labels_pkey TO saved_state_labels_pkey;
ALTER TABLE engineering.saved_state_labels RENAME CONSTRAINT commit_labels_label_id_fkey TO saved_state_labels_label_id_fkey;
ALTER TABLE engineering.saved_state_labels RENAME CONSTRAINT commit_labels_commit_id_fkey TO saved_state_labels_saved_state_id_fkey;
ALTER TABLE engineering.saved_state_labels RENAME COLUMN commit_id TO saved_state_id;
ALTER INDEX engineering.idx_commit_labels_commit RENAME TO idx_saved_state_labels_saved_state;

-- The imports a Project's saved state holds: its ProjectFile carries them as
-- provenance.
ALTER TABLE engineering.commit_library_imports RENAME TO saved_state_library_imports;
ALTER TABLE engineering.saved_state_library_imports RENAME CONSTRAINT commit_library_pins_pkey TO saved_state_library_imports_pkey;
ALTER TABLE engineering.saved_state_library_imports RENAME CONSTRAINT fk_commit_library_imports_commit_id TO fk_saved_state_library_imports_saved_state_id;
ALTER TABLE engineering.saved_state_library_imports RENAME COLUMN commit_id TO saved_state_id;
ALTER TABLE engineering.saved_state_library_imports RENAME COLUMN commit_number TO saved_state_number;

-- A Project's Draft imports a saved state of a Library by its identity and its
-- number.
ALTER TABLE engineering.library_imports RENAME COLUMN commit_id TO saved_state_id;
ALTER TABLE engineering.library_imports RENAME COLUMN commit_number TO saved_state_number;
ALTER TABLE engineering.library_imports RENAME CONSTRAINT fk_library_imports_commit_id TO fk_library_imports_saved_state_id;
ALTER INDEX engineering.idx_library_imports_commit_id RENAME TO idx_library_imports_saved_state_id;
COMMENT ON TABLE engineering.library_imports IS
    'A Project''s Library Imports (task 85_6): the saved state of a Library its Draft uses';
COMMENT ON TABLE engineering.saved_states IS
    'The saved states of the Drafts (task 72_2; ADR 0021, ADR 0026): numbered, immutable snapshots';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
COMMENT ON TABLE engineering.saved_states IS NULL;
COMMENT ON TABLE engineering.library_imports IS
    'A Project''s Library Imports (task 85_6): the saved state — the Commit — of a Library its Draft uses';
ALTER INDEX engineering.idx_library_imports_saved_state_id RENAME TO idx_library_imports_commit_id;
ALTER TABLE engineering.library_imports RENAME CONSTRAINT fk_library_imports_saved_state_id TO fk_library_imports_commit_id;
ALTER TABLE engineering.library_imports RENAME COLUMN saved_state_number TO commit_number;
ALTER TABLE engineering.library_imports RENAME COLUMN saved_state_id TO commit_id;

ALTER TABLE engineering.saved_state_library_imports RENAME COLUMN saved_state_number TO commit_number;
ALTER TABLE engineering.saved_state_library_imports RENAME COLUMN saved_state_id TO commit_id;
ALTER TABLE engineering.saved_state_library_imports RENAME CONSTRAINT fk_saved_state_library_imports_saved_state_id TO fk_commit_library_imports_commit_id;
ALTER TABLE engineering.saved_state_library_imports RENAME CONSTRAINT saved_state_library_imports_pkey TO commit_library_pins_pkey;
ALTER TABLE engineering.saved_state_library_imports RENAME TO commit_library_imports;

ALTER INDEX engineering.idx_saved_state_labels_saved_state RENAME TO idx_commit_labels_commit;
ALTER TABLE engineering.saved_state_labels RENAME COLUMN saved_state_id TO commit_id;
ALTER TABLE engineering.saved_state_labels RENAME CONSTRAINT saved_state_labels_saved_state_id_fkey TO commit_labels_commit_id_fkey;
ALTER TABLE engineering.saved_state_labels RENAME CONSTRAINT saved_state_labels_label_id_fkey TO commit_labels_label_id_fkey;
ALTER TABLE engineering.saved_state_labels RENAME CONSTRAINT saved_state_labels_pkey TO commit_labels_pkey;
ALTER TABLE engineering.saved_state_labels RENAME TO commit_labels;

ALTER TABLE engineering.drafts
    RENAME COLUMN last_saved_state_draft_version TO last_commit_draft_version;
ALTER TABLE engineering.drafts
    RENAME COLUMN last_saved_state_number TO last_commit_number;

ALTER INDEX engineering.idx_saved_states_draft_number RENAME TO idx_commits_draft_number;
ALTER TABLE engineering.saved_states RENAME CONSTRAINT saved_states_pkey TO commits_pkey;
ALTER TABLE engineering.saved_states RENAME CONSTRAINT uq_saved_states_draft_number TO uq_commits_draft_number;
ALTER TABLE engineering.saved_states RENAME CONSTRAINT ck_saved_states_number TO ck_commits_number;
ALTER TABLE engineering.saved_states RENAME CONSTRAINT fk_saved_states_draft_id TO fk_commits_draft_id;
ALTER TABLE engineering.saved_states RENAME TO commits;
-- +goose StatementEnd
