-- +goose Up
-- +goose StatementBegin
-- Change z from DOUBLE PRECISION to INTEGER (semantic: layer order / z-index).
ALTER TABLE widgets ALTER COLUMN z TYPE INTEGER USING z::INTEGER;

-- Add origin anchor and rotation angle.
ALTER TABLE widgets
    ADD COLUMN origin_x          DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    ADD COLUMN origin_y          DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    ADD COLUMN rotation_degrees  DOUBLE PRECISION NOT NULL DEFAULT 0.0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE widgets
    DROP COLUMN IF EXISTS rotation_degrees,
    DROP COLUMN IF EXISTS origin_y,
    DROP COLUMN IF EXISTS origin_x;

ALTER TABLE widgets ALTER COLUMN z TYPE DOUBLE PRECISION USING z::DOUBLE PRECISION;
-- +goose StatementEnd
