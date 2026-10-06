-- +goose Up
-- +goose StatementBegin
-- The outbox holds the domain events of committed changes until the
-- dispatcher has delivered them (ADR 0008). A repository writes a change and
-- its events in one transaction; the dispatcher reads rows in seq order and
-- deletes each one once delivered. payload is an internal format of the
-- server, not a contract: a row lives seconds.
CREATE TABLE outbox (
    seq        BIGSERIAL PRIMARY KEY,
    type       TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbox;
-- +goose StatementEnd
