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
    ('6ba7b818-9dad-11d1-80b4-00c04fd430c8', 'creator', 'string', 'Alexander', 'good', 1),
    ('6ba7b819-9dad-11d1-80b4-00c04fd430c8', 'status_message', 'string', 'System running normally - all checks passed', 'good', 1);

-- Add test data for widget_types table
-- script_language: javascript | python | lua
-- input_ports: JSON array of {name, description, type_hint}; type_hint: "" | "string" | "boolean" | "integer"
-- version: default committed version is 1
INSERT INTO widget_types (id, name, html_template, script, script_language, input_ports, default_width, default_height, version) VALUES
    (
        'a1b2c3d4-0001-4000-8000-000000000001',
        'Boolean Circle',
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="100%" height="100%" style="display:block"><circle class="circle-indicator" cx="50" cy="50" r="44" fill="#888888" stroke="#555555" stroke-width="2"/></svg>',
        'function render(inputs) { var c = document.querySelector(''.circle-indicator''); c.setAttribute(''fill'', inputs.state ? ''#2ecc71'' : ''#888888''); }',
        'javascript',
        '[{"name":"state","description":"Active state (true = green, false = gray)","type_hint":"boolean"}]',
        80,
        80,
        1
    ),
    (
        'a1b2c3d4-0002-4000-8000-000000000002',
        'Integer Speedometer',
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="100%" height="100%" style="display:block"><path class="spd-bg" d="M 18.82 73 A 36 36 0 1 1 81.18 73" fill="none" stroke="#333" stroke-width="6" stroke-linecap="round"/><path class="spd-arc" d="M 18.82 73 A 36 36 0 1 1 81.18 73" fill="none" stroke="#2ecc71" stroke-width="6" stroke-linecap="round" stroke-dasharray="0 1000"/><line class="spd-needle" x1="50" y1="55" x2="27.48" y2="68" stroke="#ddd" stroke-width="2.5" stroke-linecap="round"/><circle cx="50" cy="55" r="3.5" fill="#aaa"/><text class="spd-val" x="50" y="71" text-anchor="middle" font-size="11" font-family="monospace" font-weight="bold" fill="#ddd">0</text><text x="14" y="81" text-anchor="middle" font-size="7" font-family="sans-serif" fill="#666">0</text><text x="86" y="81" text-anchor="middle" font-size="7" font-family="sans-serif" fill="#666">100</text></svg>',
        'function render(inputs) { var value = Math.max(0, Math.min(100, Number(inputs.value) || 0)); var angleRad = (150 + value / 100 * 240) * Math.PI / 180; var x2 = (50 + 26 * Math.cos(angleRad)).toFixed(2); var y2 = (55 + 26 * Math.sin(angleRad)).toFixed(2); document.querySelector(''.spd-needle'').setAttribute(''x2'', x2); document.querySelector(''.spd-needle'').setAttribute(''y2'', y2); var arcLen = (150.8 * value / 100).toFixed(2); document.querySelector(''.spd-arc'').setAttribute(''stroke-dasharray'', arcLen + '' 1000''); var color = value < 70 ? ''#2ecc71'' : value < 90 ? ''#f39c12'' : ''#e74c3c''; document.querySelector(''.spd-arc'').setAttribute(''stroke'', color); document.querySelector(''.spd-val'').textContent = Math.round(value); }',
        'javascript',
        '[{"name":"value","description":"Integer value (0-100)","type_hint":"integer"}]',
        120,
        120,
        1
    ),
    (
        'a1b2c3d4-0005-4000-8000-000000000003',
        'String Ticker',
        '<style>@keyframes mq{0%{transform:translateX(0)}100%{transform:translateX(-50%)}}</style><div style="width:100%;height:100%;display:flex;align-items:center;"><div style="width:100%;height:100%;border-radius:6px;background:#0a0a0f;border:1px solid #222a33;overflow:hidden;display:flex;align-items:center;box-sizing:border-box;"><div class="st-track" style="overflow:hidden;flex:1;white-space:nowrap;padding:0 10px;"><span class="st-txt" style="display:inline-block;font-family:monospace;font-size:12px;color:#7fb3d3;letter-spacing:0.05em;"></span></div></div></div>',
        'function render(inputs){var t=String(inputs.value||'''');var sp=document.querySelector(''.st-txt'');var tr=document.querySelector(''.st-track'');sp.style.animation=''none'';sp.textContent=t;requestAnimationFrame(function(){if(sp.scrollWidth>tr.clientWidth){sp.textContent=t+''  ●  ''+t;sp.style.animation=''mq ''+(t.length*0.1+1.5)+''s linear infinite'';}});}',
        'javascript',
        '[{"name":"value","description":"String value to display","type_hint":"string"}]',
        280,
        32,
        1
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
        'status-circle-main',
        100.0, 200.0, 0,
        80, 80,
        0.5, 0.5, 0.0,
        'a1b2c3d4-0001-4000-8000-000000000001',
        'c1d2e3f4-0001-4000-8000-000000000001',
        '{"indicator","status"}',
        '[{"port_name":"state","tag_id":"6ba7b810-9dad-11d1-80b4-00c04fd430c8"}]',
        1
    ),
    (
        'b1c2d3e4-0002-4000-8000-000000000002',
        'speedometer-max-connections',
        300.0, 200.0, 0,
        120, 120,
        0.5, 0.5, 0.0,
        'a1b2c3d4-0002-4000-8000-000000000002',
        'c1d2e3f4-0001-4000-8000-000000000001',
        '{"speedometer","gauge"}',
        '[{"port_name":"value","tag_id":"6ba7b811-9dad-11d1-80b4-00c04fd430c8"}]',
        1
    ),
    (
        'b1c2d3e4-0005-4000-8000-000000000005',
        'string-ticker-status',
        100.0, 360.0, 0,
        280, 32,
        0.5, 0.5, 0.0,
        'a1b2c3d4-0005-4000-8000-000000000003',
        'c1d2e3f4-0001-4000-8000-000000000001',
        '{"string","ticker"}',
        '[{"port_name":"value","tag_id":"6ba7b819-9dad-11d1-80b4-00c04fd430c8"}]',
        1
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Delete test data for widgets table by their ids
DELETE FROM widgets WHERE id IN (
    'b1c2d3e4-0001-4000-8000-000000000001',
    'b1c2d3e4-0002-4000-8000-000000000002',
    'b1c2d3e4-0003-4000-8000-000000000003',
    'b1c2d3e4-0005-4000-8000-000000000005'
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
    'a1b2c3d4-0005-4000-8000-000000000003',
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
    '6ba7b818-9dad-11d1-80b4-00c04fd430c8',
    '6ba7b819-9dad-11d1-80b4-00c04fd430c8'
);
-- +goose StatementEnd
