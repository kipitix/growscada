-- +goose Up
-- +goose StatementBegin
-- Add test data for tags table
-- type: Boolean | Integer
-- quality: Unknown | Bad | Uncertain | Good | Simulated
INSERT INTO tags (id, name, type, value, quality, version) VALUES
    ('6ba7b810-9dad-11d1-80b4-00c04fd430c8', 'is_active', 'Boolean', 'true', 'Good', 1),
    ('6ba7b811-9dad-11d1-80b4-00c04fd430c8', 'max_connections', 'Integer', '100', 'Good', 1),
    ('6ba7b812-9dad-11d1-80b4-00c04fd430c8', 'is_debug', 'Boolean', 'false', 'Bad', 1),
    ('6ba7b813-9dad-11d1-80b4-00c04fd430c8', 'timeout_seconds', 'Integer', '30', 'Uncertain', 1),
    ('6ba7b814-9dad-11d1-80b4-00c04fd430c8', 'retry_count', 'Integer', '3', 'Unknown', 1),
    ('6ba7b815-9dad-11d1-80b4-00c04fd430c8', 'is_cached', 'Boolean', 'true', 'Simulated', 1),
    ('6ba7b816-9dad-11d1-80b4-00c04fd430c8', 'pool_size', 'Integer', '20', 'Good', 1),
    ('6ba7b817-9dad-11d1-80b4-00c04fd430c8', 'enable_tls', 'Boolean', 'false', 'Bad', 1);
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
    '6ba7b817-9dad-11d1-80b4-00c04fd430c8'
);
-- +goose StatementEnd
