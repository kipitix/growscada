-- +goose Up
-- +goose StatementBegin
-- The outbox holds the domain events of committed changes until the
-- dispatcher has delivered them (ADR 0008). A repository writes a change and
-- its events in one transaction; the dispatcher reads rows in (tx_id, seq)
-- order and deletes each one once delivered. tx_id is the id of the
-- transaction that wrote the row: the dispatcher reads a row only once every
-- older transaction has ended, so writers never wait for one another and a
-- change is delivered after every change it was made on top of. payload is an
-- internal format of the server, not a contract: a row lives seconds.
CREATE TABLE outbox (
    seq        BIGSERIAL PRIMARY KEY,
    tx_id      XID8        NOT NULL DEFAULT pg_current_xact_id(),
    type       TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbox;
-- +goose StatementEnd
