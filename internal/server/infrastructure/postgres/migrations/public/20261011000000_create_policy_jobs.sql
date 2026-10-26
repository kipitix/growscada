-- +goose Up
-- +goose StatementBegin
-- A policy job asks a policy to bring into line what job_key names (ADR 0008).
-- The outbox dispatcher enqueues one in the transaction that deletes the
-- delivered outbox row; a further event for the same key merges into the
-- waiting job and raises its generation, so a job taken before that event is
-- run again. The policy runner deletes the job once the policy has succeeded
-- at the generation it took, or else counts the attempt, keeps the error and
-- puts the next attempt off (next_attempt_at).
CREATE TABLE policy_jobs (
    policy          TEXT        NOT NULL,
    job_key         TEXT        NOT NULL,
    generation      BIGINT      NOT NULL DEFAULT 1,
    attempts        INT         NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error      TEXT,
    PRIMARY KEY (policy, job_key)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS policy_jobs;
-- +goose StatementEnd
