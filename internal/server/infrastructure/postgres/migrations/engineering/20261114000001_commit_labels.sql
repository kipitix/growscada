-- +goose Up
-- +goose StatementBegin
-- Label assignments: the saved states a label marks (task 85_3; ADR 0025). An
-- assignment is an entity of the Label aggregate, not an aggregate of its own,
-- so the label row owns the set and the pair is the key: assigning it twice
-- changes nothing.
CREATE TABLE engineering.commit_labels (
    label_id  UUID NOT NULL REFERENCES engineering.labels(id) ON DELETE CASCADE,
    commit_id UUID NOT NULL REFERENCES engineering.commits(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (label_id, commit_id)
);

-- The read of a batch of saved states' labels goes by Commit; the primary key
-- serves the read by label.
CREATE INDEX idx_commit_labels_commit ON engineering.commit_labels (commit_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE engineering.commit_labels;
-- +goose StatementEnd
