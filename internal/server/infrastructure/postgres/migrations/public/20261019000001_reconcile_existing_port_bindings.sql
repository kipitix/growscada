-- +goose Up
-- +goose StatementBegin
-- Before task 25 nothing removed a PortBinding to a deleted Tag or to a Tag
-- whose TagType the port's type hint does not accept, so such bindings may be
-- stored (ADR 0010). A job of reconcile_widgets_with_widget_type for every
-- WidgetType used by Widgets removes them once the policy runner starts.
INSERT INTO policy_jobs (policy, job_key)
    SELECT 'reconcile_widgets_with_widget_type', 'widget_type:' || wt.id
    FROM widget_types wt
    WHERE EXISTS (SELECT 1 FROM widgets w WHERE w.type_id = wt.id)
ON CONFLICT (policy, job_key) DO UPDATE
    SET generation = policy_jobs.generation + 1, next_attempt_at = now();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The removed bindings cannot be restored; a job not yet run is harmless.
SELECT 1;
-- +goose StatementEnd
