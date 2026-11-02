-- +goose Up
-- +goose StatementBegin
-- Devices and the Environments that hold their connection parameters (task 34,
-- ADR 0004 amendment 2026-11-01). A Device is what a thing is — its protocol
-- and the configuration read under it; an Environment is where it is — the
-- address, the port, the credentials; an Environment Device is the
-- parameters of one pair (Environment, Device). Both are content of a
-- Project's Draft: names unique in the Project, a Version of their own.
--
-- A Tag's supplier is part of its definition: device_id is NULL until a
-- Device is made its supplier. At most one Device supplies a Tag; two of them
-- is a Draft validation error (task 35 refuses the Deploy).
CREATE TABLE engineering.devices (
    pk_id      SERIAL PRIMARY KEY,
    id         UUID        NOT NULL UNIQUE,
    project_id UUID        NOT NULL,
    name       TEXT        NOT NULL,
    comment    TEXT        NOT NULL DEFAULT '',
    protocol   VARCHAR(50) NOT NULL,
    config     JSONB       NOT NULL DEFAULT '{}',
    version    INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_devices_project_id FOREIGN KEY (project_id) REFERENCES projects(id),
    CONSTRAINT uq_devices_project_name UNIQUE (project_id, name)
);
CREATE TRIGGER update_devices_updated_at
    BEFORE UPDATE ON engineering.devices
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE engineering.environments (
    pk_id      SERIAL PRIMARY KEY,
    id         UUID        NOT NULL UNIQUE,
    project_id UUID        NOT NULL,
    name       TEXT        NOT NULL,
    comment    TEXT        NOT NULL DEFAULT '',
    version    INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_environments_project_id FOREIGN KEY (project_id) REFERENCES projects(id),
    CONSTRAINT uq_environments_project_name UNIQUE (project_id, name)
);
CREATE TRIGGER update_environments_updated_at
    BEFORE UPDATE ON engineering.environments
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- The pair (Environment, Device): its parameters are read under the Device's
-- protocol. The pair is what identifies it (as a Library Pin is identified by
-- its Library in its Project), so it has no identifier of its own. No CASCADE
-- from either side, so a Device or an Environment is removed only once no
-- pair holds it.
CREATE TABLE engineering.environment_devices (
    pk_id          SERIAL PRIMARY KEY,
    environment_id UUID    NOT NULL,
    device_id      UUID    NOT NULL,
    params         JSONB   NOT NULL DEFAULT '{}',
    version        INTEGER NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_environment_devices_environment_id FOREIGN KEY (environment_id) REFERENCES engineering.environments(id),
    CONSTRAINT fk_environment_devices_device_id FOREIGN KEY (device_id) REFERENCES engineering.devices(id),
    CONSTRAINT uq_environment_devices_pair UNIQUE (environment_id, device_id)
);
CREATE TRIGGER update_environment_devices_updated_at
    BEFORE UPDATE ON engineering.environment_devices
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE engineering.tags ADD COLUMN device_id UUID;
ALTER TABLE engineering.tags
    ADD CONSTRAINT fk_tags_device_id FOREIGN KEY (device_id) REFERENCES engineering.devices(id);
CREATE INDEX idx_tags_device_id ON engineering.tags(device_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Dropping the column takes its index and its foreign key with it; dropping
-- a table takes its triggers, so neither is named here.
ALTER TABLE engineering.tags DROP COLUMN device_id;

DROP TABLE engineering.environment_devices;
DROP TABLE engineering.environments;
DROP TABLE engineering.devices;
-- +goose StatementEnd
