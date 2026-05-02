-- +goose Up
-- +goose StatementBegin
-- Add test data for tags table
-- kind: string | boolean | integer
-- quality: bad | uncertain | good | simulated
INSERT INTO tags (id, name, kind, value, quality, version) VALUES
    ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'is_active', 'boolean', 'true', 'good', 1),
    ('6ba7b811-9dad-11d1-80b4-00c04fd430c8', 'max_connections', 'integer', '100', 'good', 1),
    ('6ba7b812-9dad-11d1-80b4-00c04fd430c8', 'is_debug', 'boolean', 'false', 'bad', 1),
    ('6ba7b813-9dad-11d1-80b4-00c04fd430c8', 'timeout_seconds', 'integer', '30', 'uncertain', 2),
    ('6ba7b814-9dad-11d1-80b4-00c04fd430c8', 'retry_count', 'integer', '3', 'good', 1),
    ('6ba7b815-9dad-11d1-80b4-00c04fd430c8', 'is_cached', 'boolean', 'true', 'simulated', 3),
    ('6ba7b816-9dad-11d1-80b4-00c04fd430c8', 'pool_size', 'integer', '20', 'good', 1),
    ('6ba7b817-9dad-11d1-80b4-00c04fd430c8', 'enable_tls', 'boolean', 'false', 'bad', 4),
    ('6ba7b818-9dad-11d1-80b4-00c04fd430c8', 'creator', 'string', 'Alexander', 'good', 1);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
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
