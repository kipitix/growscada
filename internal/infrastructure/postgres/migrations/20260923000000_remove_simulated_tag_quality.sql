-- +goose Up
-- +goose StatementBegin
-- Quality describes how trustworthy a value is, never where it came from, so
-- "simulated" is no longer a Quality (see Quality in CONTEXT.md). Manually set
-- values were considered correct, so they become 'good'.
UPDATE tags
    SET quality = 'good'
    WHERE quality = 'simulated';
ALTER TABLE tags
    DROP CONSTRAINT chk_tags_quality,
    ADD CONSTRAINT chk_tags_quality CHECK (quality IN ('bad', 'uncertain', 'good'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Rows converted from 'simulated' cannot be identified, so data is left as is.
ALTER TABLE tags
    DROP CONSTRAINT chk_tags_quality,
    ADD CONSTRAINT chk_tags_quality CHECK (quality IN ('bad', 'uncertain', 'good', 'simulated'));
-- +goose StatementEnd
