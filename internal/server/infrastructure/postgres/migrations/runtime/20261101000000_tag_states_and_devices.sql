-- +goose Up
-- +goose StatementBegin
-- The Runtime schema copies the Draft's tables and keeps its own data apart
-- (ADR 0011, amendment 2026-11-01): runtime.tags becomes the copy of the Tag
-- definition alone, the Tag state moves to runtime.tag_states, and the
-- Devices with the parameters of their Environments are copied beside it.
-- A copy row keeps its original's ID, so the Runtime node never learns an ID
-- the Draft does not know; nothing here is read from the engineering schema.

-- The copy of engineering.devices: what a thing is. No unique name — that is
-- the Draft's rule, not the Runtime node's.
CREATE TABLE runtime.devices (
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
    CONSTRAINT fk_runtime_devices_project_id FOREIGN KEY (project_id) REFERENCES projects(id)
);

-- The copy of engineering.environments: the name a Gateway calls its
-- Environment by. Its name is unique in the Project here too, unlike the
-- Device's: an Environment arrives from the wire by name alone, and the
-- Runtime node resolves it to a pair with a Device without asking anyone.
CREATE TABLE runtime.environments (
    pk_id      SERIAL PRIMARY KEY,
    id         UUID        NOT NULL UNIQUE,
    project_id UUID        NOT NULL,
    name       TEXT        NOT NULL,
    comment    TEXT        NOT NULL DEFAULT '',
    version    INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_runtime_environments_project_id FOREIGN KEY (project_id) REFERENCES projects(id),
    CONSTRAINT uq_runtime_environments_project_name UNIQUE (project_id, name)
);

-- The copy of engineering.environment_devices: the parameters of one pair
-- (Environment, Device), which is also how the Gateway asks for them. The
-- Draft's Version does not come along — a copy has nothing to edit.
CREATE TABLE runtime.environment_devices (
    pk_id          SERIAL PRIMARY KEY,
    environment_id UUID  NOT NULL,
    device_id      UUID  NOT NULL,
    params         JSONB NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_runtime_environment_devices_pair UNIQUE (environment_id, device_id)
);

-- The live tokens, one per Device: only the hash of the token and its
-- fingerprint are here (ADR 0004, amendment 2026-11-01). A token never rides
-- into a Revision; it outlives Deploy and Rollback, and sleeps while its
-- Device is not in the running Revision. So its device_id names the Device
-- without a foreign key to the copy: Deploy removes the copy of a Device the
-- running Revision no longer has, and the token must stay behind, orphaned
-- until it is revoked. The unique index is the hot path of authentication: a
-- token is looked up by its hash.
CREATE TABLE runtime.device_tokens (
    device_id   UUID        PRIMARY KEY,
    token_hash  TEXT        NOT NULL,
    fingerprint TEXT        NOT NULL,
    issued_at   TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX uq_runtime_device_tokens_token_hash ON runtime.device_tokens(token_hash);

-- The Tag state apart from its definition: a state never outlives the copy of
-- its definition, so it goes with it. The states the Tags have now move here
-- as they are, their seqs unchanged.
CREATE TABLE runtime.tag_states (
    tag_id  UUID        PRIMARY KEY,
    value   TEXT        NOT NULL,
    quality VARCHAR(50) NOT NULL,
    seq     BIGINT      NOT NULL DEFAULT 0,
    CONSTRAINT fk_runtime_tag_states_tag_id FOREIGN KEY (tag_id) REFERENCES runtime.tags(id) ON DELETE CASCADE,
    CONSTRAINT chk_runtime_tag_states_quality CHECK (quality IN ('bad', 'uncertain', 'good'))
);
INSERT INTO runtime.tag_states (tag_id, value, quality, seq)
    SELECT id, value, quality, seq FROM runtime.tags;

-- What is left in runtime.tags is the copy of the Tag definition: the name,
-- the TagType, the Version and, once a Device supplies the Tag, its ID. The
-- seq goes with the state: a definition is never numbered. Until Deploy (task
-- 35) the bridge fills the copy from the Draft; the supplying Device is
-- copied with the definition.
ALTER TABLE runtime.tags
    ADD COLUMN device_id UUID,
    DROP COLUMN value,
    DROP COLUMN quality,
    DROP COLUMN seq;
CREATE INDEX idx_runtime_tags_device_id ON runtime.tags(device_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Each state goes back into its definition's row, its seq with it; a Tag
-- without a state gets its TagType's zero value with Quality bad, as the
-- Runtime node shows it.
ALTER TABLE runtime.tags
    ADD COLUMN value TEXT,
    ADD COLUMN quality VARCHAR(50),
    ADD COLUMN seq BIGINT NOT NULL DEFAULT 0;
UPDATE runtime.tags t
    SET value = s.value, quality = s.quality, seq = s.seq
    FROM runtime.tag_states s WHERE s.tag_id = t.id;
UPDATE runtime.tags
    SET value = CASE type WHEN 'boolean' THEN 'false' WHEN 'integer' THEN '0' ELSE '' END,
        quality = 'bad'
    WHERE value IS NULL;
ALTER TABLE runtime.tags
    ALTER COLUMN value SET NOT NULL,
    ALTER COLUMN quality SET NOT NULL,
    ADD CONSTRAINT chk_runtime_tags_quality CHECK (quality IN ('bad', 'uncertain', 'good'));

DROP TABLE runtime.tag_states;
DROP TABLE runtime.device_tokens;
DROP TABLE runtime.environment_devices;
DROP TABLE runtime.environments;
ALTER TABLE runtime.tags DROP COLUMN device_id;
DROP TABLE runtime.devices;
-- +goose StatementEnd
