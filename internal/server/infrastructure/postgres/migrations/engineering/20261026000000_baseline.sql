-- +goose Up
-- The baseline of the engineering chain (ADR 0011): its tables were moved or
-- created by the public migration 20261026000000_engineering_and_runtime_schemas,
-- which also marks this version as applied. The chain's own migrations
-- follow it.

-- +goose Down
-- Rolled back with the public migration that made the schema.
