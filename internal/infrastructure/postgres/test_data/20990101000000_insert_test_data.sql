-- +goose Up
-- +goose StatementBegin
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
-- version: default committed version is 1
INSERT INTO widget_types (id, name, html_template, script, script_language, version) VALUES
    (
        'a1b2c3d4-0001-4000-8000-000000000001',
        'Pressure Gauge',
        '<div class="gauge pressure"><div class="gauge__label">Pressure</div><div class="gauge__value"></div><div class="gauge__unit">bar</div></div>',
        'function render(value) { document.querySelector(''.gauge__value'').textContent = value; }',
        'javascript',
        1
    ),
    (
        'a1b2c3d4-0002-4000-8000-000000000002',
        'Temperature Indicator',
        '<div class="indicator temperature"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40" fill="none" stroke="#e74c3c" stroke-width="4"/><text x="50" y="55" text-anchor="middle" font-size="20" fill="#e74c3c" class="temp-value">--</text></svg></div>',
        'function render(value) { document.querySelector(''.temp-value'').textContent = value + ''°C''; }',
        'javascript',
        1
    ),
    (
        'a1b2c3d4-0003-4000-8000-000000000003',
        'Boolean Lamp',
        '<div class="lamp"><div class="lamp__bulb"></div><div class="lamp__label">Status</div></div>',
        'function render(value) { var bulb = document.querySelector(''.lamp__bulb''); bulb.style.background = value ? ''#2ecc71'' : ''#e74c3c''; }',
        'javascript',
        1
    ),
    (
        'a1b2c3d4-0004-4000-8000-000000000004',
        'Flow Meter',
        '<div class="flow-meter"><div class="flow-meter__bar"><div class="flow-meter__fill"></div></div><div class="flow-meter__value"></div></div>',
        'def render(value):\n    print(f"Flow: {value} m3/h")',
        'python',
        1
    ),
    (
        'a1b2c3d4-0005-4000-8000-000000000005',
        'Level Sensor',
        '<div class="level-sensor"><div class="level-sensor__tank"><div class="level-sensor__liquid"></div></div><div class="level-sensor__value"></div></div>',
        'function render(value)\n  local pct = math.min(100, math.max(0, value))\n  return pct\nend',
        'lua',
        2
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
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
