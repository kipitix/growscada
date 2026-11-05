-- +goose Up
-- +goose StatementBegin
-- A second seeded Device, `gateway`, and the Environment `lab` it stands in:
-- the one the dashboard simulator config drives (task 34, part 5). The
-- Brno collections revoke the token of the Device `sim` and delete its Tag,
-- so the simulator that runs beside them must be a Device of its own — a
-- Gateway whose token is revoked stops, as a real one would.
--
-- `gateway` supplies the Tags of the "Main Dashboard" scene:
--   is_active        6ba7b810-9dad-11d1-80b4-00c04fd430c8
--   max_connections  6ba7b811-9dad-11d1-80b4-00c04fd430c8
--   status_message   6ba7b819-9dad-11d1-80b4-00c04fd430c8
-- The Device `sim`, its Environment `bench` and its token (supplying
-- `device.speed`) are inserted by 20990101000001_insert_device_gateway_data.sql.
--
-- The token is a fixture, not a secret:
-- gsc_dev_2222…2222. Only its SHA-256 and its fingerprint are stored, as the
-- server stores any token.
INSERT INTO engineering.devices (id, project_id, name, comment, protocol, config) VALUES
    ('e0000000-0000-4000-8000-000000000003',
     'b0000000-0000-4000-8000-000000000001',
     'gateway',
     'the seeded dashboard Device Gateway',
     'simulation',
     '{}');
INSERT INTO engineering.environments (id, project_id, name, comment) VALUES
    ('e1000000-0000-4000-8000-000000000002',
     'b0000000-0000-4000-8000-000000000001',
     'lab',
     'the seeded dashboard test bench');
INSERT INTO engineering.environment_devices (environment_id, device_id, params) VALUES
    ('e1000000-0000-4000-8000-000000000002',
     'e0000000-0000-4000-8000-000000000003',
     '{"address":"127.0.0.1:1503"}');
INSERT INTO runtime.device_tokens (device_id, project_id, token_hash, fingerprint, issued_at) VALUES
    ('e0000000-0000-4000-8000-000000000003',
     'b0000000-0000-4000-8000-000000000001',
     'b62f9fd9091d584bdd19036c97d6e19605c6e6d244d729c748a6ea977a169846',
     'gsc_dev_b62f9fd9',
     '2026-11-03T12:00:00Z');

UPDATE engineering.tags SET device_id = 'e0000000-0000-4000-8000-000000000003'
 WHERE id IN ('6ba7b810-9dad-11d1-80b4-00c04fd430c8',
              '6ba7b811-9dad-11d1-80b4-00c04fd430c8',
              '6ba7b819-9dad-11d1-80b4-00c04fd430c8');
UPDATE runtime.tags SET device_id = 'e0000000-0000-4000-8000-000000000003'
 WHERE id IN ('6ba7b810-9dad-11d1-80b4-00c04fd430c8',
              '6ba7b811-9dad-11d1-80b4-00c04fd430c8',
              '6ba7b819-9dad-11d1-80b4-00c04fd430c8');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE engineering.tags SET device_id = NULL
 WHERE id IN ('6ba7b810-9dad-11d1-80b4-00c04fd430c8',
              '6ba7b811-9dad-11d1-80b4-00c04fd430c8',
              '6ba7b819-9dad-11d1-80b4-00c04fd430c8');
UPDATE runtime.tags SET device_id = NULL
 WHERE id IN ('6ba7b810-9dad-11d1-80b4-00c04fd430c8',
              '6ba7b811-9dad-11d1-80b4-00c04fd430c8',
              '6ba7b819-9dad-11d1-80b4-00c04fd430c8');
DELETE FROM runtime.device_tokens WHERE device_id = 'e0000000-0000-4000-8000-000000000003';
DELETE FROM engineering.environment_devices WHERE environment_id = 'e1000000-0000-4000-8000-000000000002';
DELETE FROM engineering.devices WHERE id = 'e0000000-0000-4000-8000-000000000003';
DELETE FROM engineering.environments WHERE id = 'e1000000-0000-4000-8000-000000000002';
-- +goose StatementEnd
