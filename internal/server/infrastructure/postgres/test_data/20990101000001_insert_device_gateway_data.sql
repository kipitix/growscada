-- +goose Up
-- +goose StatementBegin
-- The Device Gateway test data (task 34): one Device, one Environment it
-- stands in, a Tag it supplies and its token. The collection of the server API
-- drives them; the device simulator's own config does not use them, and none
-- of the seeded dashboard Tags is supplied by a Device, so the simulator
-- (which carries no token until task 34, part 5) keeps writing them.
--
-- IDs used below:
--   Device:      e0000000-0000-4000-8000-000000000001
--   Environment: e1000000-0000-4000-8000-000000000001
--   Tag:         6ba7b820-9dad-11d1-80b4-00c04fd430c8 (device.speed)
--
-- The token is a fixture, not a secret: gsc_dev_0123…cdef. Only its SHA-256
-- and its fingerprint are stored, as the server stores any token.
INSERT INTO engineering.devices (id, project_id, name, comment, protocol, config, version) VALUES
    ('e0000000-0000-4000-8000-000000000001',
     'b0000000-0000-4000-8000-000000000001',
     'sim',
     'the seeded Device Gateway fixture',
     'simulation',
     '{"patterns":[]}',
     1),
    ('e0000000-0000-4000-8000-000000000002',
     'b0000000-0000-4000-8000-000000000001',
     'sim-2',
     'a second Device, without a token or a tag',
     'simulation',
     '{}',
     1);

INSERT INTO engineering.environments (id, project_id, name, comment, version) VALUES
    ('e1000000-0000-4000-8000-000000000001',
     'b0000000-0000-4000-8000-000000000001',
     'bench',
     'the seeded test bench',
     1);

INSERT INTO engineering.environment_devices (environment_id, device_id, params, version) VALUES
    ('e1000000-0000-4000-8000-000000000001',
     'e0000000-0000-4000-8000-000000000001',
     '{"address":"127.0.0.1:1502"}',
     1);

-- The Tag the seeded Device supplies, in the Draft and in the Runtime node's
-- copy of its definition, with a state written by the Device.
INSERT INTO engineering.tags (id, project_id, name, type, version, device_id) VALUES
    ('6ba7b820-9dad-11d1-80b4-00c04fd430c8', 'b0000000-0000-4000-8000-000000000001', 'device.speed', 'integer', 1,
     'e0000000-0000-4000-8000-000000000001');
INSERT INTO runtime.tags (id, project_id, name, type, version, device_id) VALUES
    ('6ba7b820-9dad-11d1-80b4-00c04fd430c8', 'b0000000-0000-4000-8000-000000000001', 'device.speed', 'integer', 1,
     'e0000000-0000-4000-8000-000000000001');
INSERT INTO runtime.tag_states (tag_id, value, quality, seq) VALUES
    ('6ba7b820-9dad-11d1-80b4-00c04fd430c8', '12', 'good', nextval('runtime.tag_state_seq'));

INSERT INTO runtime.device_tokens (device_id, project_id, token_hash, fingerprint, issued_at) VALUES
    ('e0000000-0000-4000-8000-000000000001',
     'b0000000-0000-4000-8000-000000000001',
     '7431b63bcf29106da937b94ccb71914457b4d213de61b3dff07221367b57bc86',
     'gsc_dev_7431b63b',
     '2026-11-01T12:00:00Z');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM runtime.device_tokens WHERE device_id = 'e0000000-0000-4000-8000-000000000001';
DELETE FROM runtime.tags WHERE id = '6ba7b820-9dad-11d1-80b4-00c04fd430c8';
DELETE FROM engineering.tags WHERE id = '6ba7b820-9dad-11d1-80b4-00c04fd430c8';
DELETE FROM engineering.environment_devices WHERE environment_id = 'e1000000-0000-4000-8000-000000000001';
DELETE FROM engineering.devices WHERE id IN ('e0000000-0000-4000-8000-000000000001', 'e0000000-0000-4000-8000-000000000002');
DELETE FROM engineering.environments WHERE id = 'e1000000-0000-4000-8000-000000000001';
-- +goose StatementEnd
