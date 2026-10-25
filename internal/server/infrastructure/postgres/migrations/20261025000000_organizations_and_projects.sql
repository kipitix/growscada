-- +goose Up
-- +goose StatementBegin
-- Every operational entity is keyed by Project, every Project by
-- Organization (ADR 0006). Until Users exist (task 49) the server runs with
-- the one Organization created here; the content stored so far becomes the
-- project "Main" of it. Both IDs are fixed, so the seed data can name them.
CREATE TABLE organizations (
    pk_id      SERIAL PRIMARY KEY,
    id         UUID        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO organizations (id, name) VALUES ('a0000000-0000-4000-8000-000000000001', 'Default');

-- A deleted project is only marked (deleted_at) until the policy that empties
-- it removes its row last; its name is free at once: unique among the live
-- projects of the organization.
CREATE TABLE projects (
    pk_id           SERIAL PRIMARY KEY,
    id              UUID        NOT NULL UNIQUE,
    organization_id UUID        NOT NULL,
    name            TEXT        NOT NULL,
    version         INTEGER     NOT NULL DEFAULT 0,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_projects_organization_id FOREIGN KEY (organization_id) REFERENCES organizations(id)
);
CREATE UNIQUE INDEX uq_projects_organization_live_name ON projects (organization_id, name) WHERE deleted_at IS NULL;
CREATE TRIGGER update_projects_updated_at
    BEFORE UPDATE ON projects
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
INSERT INTO projects (id, organization_id, name, version)
    VALUES ('b0000000-0000-4000-8000-000000000001', 'a0000000-0000-4000-8000-000000000001', 'Main', 1);

-- The content of a project: no CASCADE from projects, so removing a project
-- that still has Tags or Scenes is refused.
ALTER TABLE tags ADD COLUMN project_id UUID;
UPDATE tags SET project_id = 'b0000000-0000-4000-8000-000000000001';
ALTER TABLE tags
    ALTER COLUMN project_id SET NOT NULL,
    ADD CONSTRAINT fk_tags_project_id FOREIGN KEY (project_id) REFERENCES projects(id),
    DROP CONSTRAINT uq_tags_name,
    ADD CONSTRAINT uq_tags_project_name UNIQUE (project_id, name);

ALTER TABLE scenes ADD COLUMN project_id UUID;
UPDATE scenes SET project_id = 'b0000000-0000-4000-8000-000000000001';
ALTER TABLE scenes
    ALTER COLUMN project_id SET NOT NULL,
    ADD CONSTRAINT fk_scenes_project_id FOREIGN KEY (project_id) REFERENCES projects(id),
    ADD CONSTRAINT uq_scenes_id_project_id UNIQUE (id, project_id);
CREATE INDEX idx_scenes_project_id ON scenes(project_id);

-- A widget is of its scene's project: the foreign key to the scene includes
-- the project, so the two cannot differ.
ALTER TABLE widgets ADD COLUMN project_id UUID;
UPDATE widgets SET project_id = scenes.project_id FROM scenes WHERE scenes.id = widgets.scene_id;
ALTER TABLE widgets
    ALTER COLUMN project_id SET NOT NULL,
    DROP CONSTRAINT fk_widgets_scene_id,
    ADD CONSTRAINT fk_widgets_scene_id_project_id
        FOREIGN KEY (scene_id, project_id) REFERENCES scenes(id, project_id) ON DELETE CASCADE;
CREATE INDEX idx_widgets_project_id ON widgets(project_id);

-- The outbox row names the project of a content event; a row still waiting
-- here is of the project "Main" (its payload says so too, for the codec).
ALTER TABLE outbox ADD COLUMN project_id UUID;
UPDATE outbox
    SET project_id = 'b0000000-0000-4000-8000-000000000001',
        payload = payload || jsonb_build_object('project_id', 'b0000000-0000-4000-8000-000000000001')
    WHERE type LIKE 'tag\_%' OR type LIKE 'scene\_%' OR (type LIKE 'widget\_%' AND type NOT LIKE 'widget\_type\_%');

-- The job keys of a project's content name the project.
UPDATE policy_jobs
    SET job_key = 'project:b0000000-0000-4000-8000-000000000001/' || job_key
    WHERE policy = 'unbind_deleted_tag';
UPDATE policy_jobs
    SET job_key = replace(job_key, '/scene:', '/project:b0000000-0000-4000-8000-000000000001/scene:')
    WHERE policy = 'reconcile_widgets_with_widget_type' AND job_key LIKE '%/scene:%';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Only the content of one project can go back: names unique per project may
-- collide once the projects are merged, and the down fails then.
UPDATE policy_jobs
    SET job_key = regexp_replace(job_key, '/project:[0-9a-f-]+/scene:', '/scene:')
    WHERE policy = 'reconcile_widgets_with_widget_type';
UPDATE policy_jobs
    SET job_key = regexp_replace(job_key, '^project:[0-9a-f-]+/', '')
    WHERE policy = 'unbind_deleted_tag';
ALTER TABLE outbox DROP COLUMN project_id;

DROP INDEX IF EXISTS idx_widgets_project_id;
ALTER TABLE widgets
    DROP CONSTRAINT fk_widgets_scene_id_project_id,
    ADD CONSTRAINT fk_widgets_scene_id FOREIGN KEY (scene_id) REFERENCES scenes(id) ON DELETE CASCADE,
    DROP COLUMN project_id;

DROP INDEX IF EXISTS idx_scenes_project_id;
ALTER TABLE scenes
    DROP CONSTRAINT uq_scenes_id_project_id,
    DROP CONSTRAINT fk_scenes_project_id,
    DROP COLUMN project_id;

ALTER TABLE tags
    DROP CONSTRAINT uq_tags_project_name,
    DROP CONSTRAINT fk_tags_project_id,
    DROP COLUMN project_id,
    ADD CONSTRAINT uq_tags_name UNIQUE (name);

DROP TABLE projects;
DROP TABLE organizations;
-- +goose StatementEnd
