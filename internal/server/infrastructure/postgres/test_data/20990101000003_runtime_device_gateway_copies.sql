-- +goose Up
-- +goose StatementBegin
-- The Runtime node's copies of the seeded Gateway fixtures (task 35, Q5): the
-- configuration of a Device Gateway is read from the copies the running
-- Revision fills, never from the Draft. The seeded Projects have no Revision
-- yet, so the copies stand where the Draft bridge would put them; the seed
-- writes them itself, as it already writes `runtime.tags`.
--
-- Devices:     sim (e0000000-…-000000000001), sim-2 (…0002), gateway (…0003)
-- Environments: bench (e1000000-…-000000000001), lab (…0002)
-- Pairs:       sim in bench, gateway in lab
INSERT INTO runtime.devices (id, project_id, name, comment, protocol, config, version) VALUES
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
     1),
    ('e0000000-0000-4000-8000-000000000003',
     'b0000000-0000-4000-8000-000000000001',
     'gateway',
     'the seeded dashboard Device Gateway',
     'simulation',
     '{}',
     1);

INSERT INTO runtime.environments (id, project_id, name, comment, version) VALUES
    ('e1000000-0000-4000-8000-000000000001',
     'b0000000-0000-4000-8000-000000000001',
     'bench',
     'the seeded test bench',
     1),
    ('e1000000-0000-4000-8000-000000000002',
     'b0000000-0000-4000-8000-000000000001',
     'lab',
     'the seeded dashboard test bench',
     1);

INSERT INTO runtime.environment_devices (environment_id, device_id, params) VALUES
    ('e1000000-0000-4000-8000-000000000001',
     'e0000000-0000-4000-8000-000000000001',
     '{"address":"127.0.0.1:1502"}'),
    ('e1000000-0000-4000-8000-000000000002',
     'e0000000-0000-4000-8000-000000000003',
     '{"address":"127.0.0.1:1503"}');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM runtime.environment_devices WHERE environment_id IN (
    'e1000000-0000-4000-8000-000000000001',
    'e1000000-0000-4000-8000-000000000002');
DELETE FROM runtime.devices WHERE id IN (
    'e0000000-0000-4000-8000-000000000001',
    'e0000000-0000-4000-8000-000000000002',
    'e0000000-0000-4000-8000-000000000003');
DELETE FROM runtime.environments WHERE id IN (
    'e1000000-0000-4000-8000-000000000001',
    'e1000000-0000-4000-8000-000000000002');
-- +goose StatementEnd
