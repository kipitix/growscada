-- +goose Up
-- +goose StatementBegin
-- A Release made of a Commit always names it, with the title it had at the
-- moment of publication (task 72_3; ADR 0021). The domain refuses the other
-- shape; the table refuses it too, so a row written around the domain cannot
-- claim a Commit origin without its provenance. A Release made from a project
-- file (task 72_4) keeps both columns NULL.
ALTER TABLE engineering.project_releases
    ADD CONSTRAINT ck_project_releases_commit
    CHECK (origin <> 'commit' OR (commit_number IS NOT NULL AND commit_title IS NOT NULL));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE engineering.project_releases DROP CONSTRAINT ck_project_releases_commit;
-- +goose StatementEnd
