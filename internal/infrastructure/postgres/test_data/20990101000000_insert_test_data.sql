-- +goose Up
-- +goose StatementBegin
-- IDs used across tables:
--   Scenes:       c1d2e3f4-000N-4000-8000-000000000000
--   Widgets:      b1c2d3e4-000N-4000-8000-000000000000
--   Widget types: a1b2c3d4-000N-4000-8000-000000000000 (see below)
--   Tags:         6ba7b81N-9dad-11d1-80b4-00c04fd430c8 (see below)

-- Add test data for tags table
-- type: string | boolean | integer
-- quality: bad | uncertain | good | simulated
--- default version for database is 1 - tag.TagVersionCommitted
INSERT INTO tags (id, name, type, value, quality, version) VALUES
    ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'is_active', 'boolean', 'true', 'good', 1),
    ('6ba7b811-9dad-11d1-80b4-00c04fd430c8', 'max_connections', 'integer', '100', 'good', 1),
    ('6ba7b812-9dad-11d1-80b4-00c04fd430c8', 'is_debug', 'boolean', 'false', 'bad', 1),
    ('6ba7b813-9dad-11d1-80b4-00c04fd430c8', 'timeout_seconds', 'integer', '30', 'uncertain', 2),
    ('6ba7b814-9dad-11d1-80b4-00c04fd430c8', 'retry_count', 'integer', '3', 'good', 1),
    ('6ba7b815-9dad-11d1-80b4-00c04fd430c8', 'is_cached', 'boolean', 'true', 'simulated', 3),
    ('6ba7b816-9dad-11d1-80b4-00c04fd430c8', 'pool_size', 'integer', '20', 'good', 1),
    ('6ba7b817-9dad-11d1-80b4-00c04fd430c8', 'enable_tls', 'boolean', 'false', 'bad', 4),
    ('6ba7b818-9dad-11d1-80b4-00c04fd430c8', 'creator', 'string', 'Alexander', 'good', 1);

-- Add test data for widget_types table
-- script_language: javascript | python | lua
-- input_ports: JSON array of {name, description, type_hint}; type_hint: "" | "string" | "boolean" | "integer"
-- version: default committed version is 1
INSERT INTO widget_types (id, name, html_template, script, script_language, input_ports, version) VALUES
    (
        'a1b2c3d4-0001-4000-8000-000000000001',
        'Pressure Gauge',
        '<div class="gauge pressure"><div class="gauge__label">Pressure</div><div class="gauge__value"></div><div class="gauge__unit">bar</div></div>',
        'function render(inputs) { document.querySelector(''.gauge__value'').textContent = inputs.pressure; }',
        'javascript',
        '[{"name":"pressure","description":"Line pressure in bar","type_hint":"integer"}]',
        1
    ),
    (
        'a1b2c3d4-0002-4000-8000-000000000002',
        'Temperature Indicator',
        '<div class="indicator temperature"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40" fill="none" stroke="#e74c3c" stroke-width="4"/><text x="50" y="55" text-anchor="middle" font-size="20" fill="#e74c3c" class="temp-value">--</text></svg></div>',
        'function render(inputs) { document.querySelector(''.temp-value'').textContent = inputs.temperature + ''°C''; }',
        'javascript',
        '[{"name":"temperature","description":"Temperature in degrees Celsius","type_hint":"integer"}]',
        1
    ),
    (
        'a1b2c3d4-0003-4000-8000-000000000003',
        'Boolean Lamp',
        '<div class="lamp"><div class="lamp__bulb"></div><div class="lamp__label">Status</div></div>',
        'function render(inputs) { var bulb = document.querySelector(''.lamp__bulb''); bulb.style.background = inputs.state ? ''#2ecc71'' : ''#e74c3c''; }',
        'javascript',
        '[{"name":"state","description":"Active state of the lamp","type_hint":"boolean"}]',
        1
    ),
    (
        'a1b2c3d4-0004-4000-8000-000000000004',
        'Flow Meter',
        '<div class="flow-meter"><div class="flow-meter__bar"><div class="flow-meter__fill"></div></div><div class="flow-meter__value"></div></div>',
        'def render(inputs):\n    print(f"Flow: {inputs[''flow_rate'']} m3/h")',
        'python',
        '[{"name":"flow_rate","description":"Flow rate in m³/h","type_hint":"integer"}]',
        1
    ),
    (
        'a1b2c3d4-0005-4000-8000-000000000005',
        'Level Sensor',
        '<div class="level-sensor"><div class="level-sensor__tank"><div class="level-sensor__liquid"></div></div><div class="level-sensor__value"></div></div>',
        'function render(inputs)\n  local pct = math.min(100, math.max(0, inputs.level))\n  return pct\nend',
        'lua',
        '[{"name":"level","description":"Fill level in percent","type_hint":"integer"}]',
        2
    );

-- Add test data for scenes table
-- version: default committed version is 1
INSERT INTO scenes (id, name, width, height, background_html, version) VALUES
    (
        'c1d2e3f4-0001-4000-8000-000000000001',
        'Main Dashboard',
        1920,
        1080,
        '<div class="background main-dashboard"><svg xmlns="http://www.w3.org/2000/svg" width="1920" height="1080"><rect width="100%" height="100%" fill="#1a1a2e"/></svg></div>',
        1
    ),
    (
        'c1d2e3f4-0002-4000-8000-000000000002',
        'Overview Panel',
        1280,
        720,
        '<div class="background overview"><svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720"><rect width="100%" height="100%" fill="#16213e"/></svg></div>',
        1
    );

-- Add test data for widgets table
-- scene_id references scenes above; type_id references widget_types above
-- port_bindings: JSON array of {port_name, tag_id} mapping InputPort names to tag IDs above
-- origin_x/origin_y: anchor point in [0,1] (0.5 = center)
-- rotation_degrees: rotation angle, normalized to [0, 360)
INSERT INTO widgets (id, name, x, y, z, width, height, origin_x, origin_y, rotation_degrees, type_id, scene_id, labels, port_bindings, version) VALUES
    (
        'b1c2d3e4-0001-4000-8000-000000000001',
        'pressure-gauge-main',
        100.0, 200.0, 0,
        100, 100,
        0.5, 0.5, 0.0,
        'a1b2c3d4-0001-4000-8000-000000000001',
        'c1d2e3f4-0001-4000-8000-000000000001',
        '{"sensor","pressure"}',
        '[{"port_name":"pressure","tag_id":"6ba7b813-9dad-11d1-80b4-00c04fd430c8"}]',
        1
    ),
    (
        'b1c2d3e4-0002-4000-8000-000000000002',
        'temperature-indicator-main',
        350.0, 150.0, 1,
        100, 100,
        0.5, 0.5, 0.0,
        'a1b2c3d4-0002-4000-8000-000000000002',
        'c1d2e3f4-0001-4000-8000-000000000001',
        '{"sensor","temperature"}',
        '[{"port_name":"temperature","tag_id":"6ba7b811-9dad-11d1-80b4-00c04fd430c8"}]',
        1
    ),
    (
        'b1c2d3e4-0003-4000-8000-000000000003',
        'boolean-lamp-overview',
        50.0, 400.0, 0,
        100, 100,
        0.5, 0.5, 0.0,
        'a1b2c3d4-0003-4000-8000-000000000003',
        'c1d2e3f4-0002-4000-8000-000000000002',
        '{"indicator","status"}',
        '[]',
        1
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Delete test data for widgets table by their ids
DELETE FROM widgets WHERE id IN (
    'b1c2d3e4-0001-4000-8000-000000000001',
    'b1c2d3e4-0002-4000-8000-000000000002',
    'b1c2d3e4-0003-4000-8000-000000000003'
);

-- Delete test data for scenes table by their ids
DELETE FROM scenes WHERE id IN (
    'c1d2e3f4-0001-4000-8000-000000000001',
    'c1d2e3f4-0002-4000-8000-000000000002'
);

-- Delete test data for widget_types table by their ids
DELETE FROM widget_types WHERE id IN (
    'a1b2c3d4-0001-4000-8000-000000000001',
    'a1b2c3d4-0002-4000-8000-000000000002',
    'a1b2c3d4-0003-4000-8000-000000000003',
    'a1b2c3d4-0004-4000-8000-000000000004',
    'a1b2c3d4-0005-4000-8000-000000000005'
);

-- Delete test data for tags table by their ids
DELETE FROM tags WHERE id IN (
    '6ba7b810-9dad-11d1-80b4-00c04fd430c8',
    '6ba7b811-9dad-11d1-80b4-00c04fd430c8',
    '6ba7b812-9dad-11d1-80b4-00c04fd430c8',
    '6ba7b813-9dad-11d1-80b4-00c04fd430c8',
    '6ba7b814-9dad-11d1-80b4-00c04fd430c8',
    '6ba7b815-9dad-11d1-80b4-00c04fd430c8',
    '6ba7b816-9dad-11d1-80b4-00c04fd430c8',
    '6ba7b817-9dad-11d1-80b4-00c04fd430c8',
    '6ba7b818-9dad-11d1-80b4-00c04fd430c8'
);
-- +goose StatementEnd
