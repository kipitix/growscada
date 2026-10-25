package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

const testPolicyName = "test_policy"

func cleanPolicyJobs(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM policy_jobs"); err != nil {
		t.Fatalf("cleanPolicyJobs: %v", err)
	}
}

func countPolicyJobs(t *testing.T) int {
	t.Helper()
	var n int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM policy_jobs WHERE policy = $1`, testPolicyName).Scan(&n); err != nil {
		t.Fatalf("count policy_jobs: %v", err)
	}
	return n
}

// testPolicy keys a job by the Tag of each tag_created event and records its
// runs; run decides how each one ends.
type testPolicy struct {
	mu   sync.Mutex
	runs map[string][]time.Time
	run  func(ctx context.Context, key string, attempt int) error
}

var _ application.Policy = (*testPolicy)(nil)

func newTestPolicy(run func(ctx context.Context, key string, attempt int) error) *testPolicy {
	return &testPolicy{runs: map[string][]time.Time{}, run: run}
}

func (p *testPolicy) Name() string { return testPolicyName }

func (p *testPolicy) JobKey(e event.Event) (string, bool) {
	if ev, ok := e.(tag.TagEvent); ok && e.Type() == event.EventTypeTagCreated {
		return ev.TagID().String(), true
	}
	return "", false
}

func (p *testPolicy) Run(ctx context.Context, key string) error {
	p.mu.Lock()
	p.runs[key] = append(p.runs[key], time.Now())
	attempt := len(p.runs[key])
	p.mu.Unlock()
	return p.run(ctx, key, attempt)
}

func (p *testPolicy) runsOf(key string) []time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time(nil), p.runs[key]...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPolicyRunner_FailedJob_RetriedAfterPause_DeliveryNotHeldBack(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	cleanPolicyJobs(t)
	const pause = 500 * time.Millisecond
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	first := createTag(t, repo, "first")
	second := createTag(t, repo, "second")
	firstKey, secondKey := first.ID().String(), second.ID().String()

	policy := newTestPolicy(func(_ context.Context, key string, attempt int) error {
		if key == firstKey && attempt == 1 {
			return errors.New("first attempt fails")
		}
		return nil
	})
	runner := outbox.NewPolicyRunner(testDB, []application.Policy{policy}, outbox.WithRetryPause(pause, pause))
	var delivered recorder
	d := outbox.NewDispatcher(testDB, delivered.deliver, outbox.WithPolicyRunner(runner), outbox.WithPollInterval(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); d.Run(ctx) }()
	go func() { defer wg.Done(); runner.Run(ctx) }()
	defer func() { cancel(); wg.Wait() }()
	repo = repositories.NewTagRepositoryPostgres(testDB, mainProject, repositories.NotifyOnCommit(d.Notify))

	mustSaveTag(t, repo, first)
	waitFor(t, "the first attempt", func() bool { return len(policy.runsOf(firstKey)) == 1 })
	failedAt := policy.runsOf(firstKey)[0]

	mustSaveTag(t, repo, second)
	waitFor(t, "the second event", func() bool { return len(delivered.types()) == 2 })
	if waited := time.Since(failedAt); waited >= pause {
		t.Errorf("the next event was delivered %s after the failure, not before the pause %s", waited, pause)
	}
	waitFor(t, "the job of the second event", func() bool { return len(policy.runsOf(secondKey)) == 1 })
	if n := len(policy.runsOf(firstKey)); n != 1 {
		t.Errorf("the failed job ran %d times before its pause ended, want 1", n)
	}

	waitFor(t, "the retry", func() bool { return len(policy.runsOf(firstKey)) == 2 })
	runs := policy.runsOf(firstKey)
	if gap := runs[1].Sub(runs[0]); gap < pause-50*time.Millisecond {
		t.Errorf("retried %s after the failure, want after the pause %s", gap, pause)
	}
	waitFor(t, "the jobs to be done", func() bool { return countPolicyJobs(t) == 0 })
}

func TestPolicyRunner_StoppedWhileRunning_JobRunsAgainAfterStart(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	cleanPolicyJobs(t)
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	mustSaveTag(t, repo, createTag(t, repo, "stopped"))

	started := make(chan struct{})
	blocking := newTestPolicy(func(ctx context.Context, _ string, _ int) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	runner := outbox.NewPolicyRunner(testDB, []application.Policy{blocking})
	if _, err := outbox.NewDispatcher(testDB, func(event.Event) {}, outbox.WithPolicyRunner(runner)).DrainOnce(context.Background()); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runner.Run(ctx); close(done) }()
	<-started
	cancel()
	<-done

	succeeding := newTestPolicy(func(context.Context, string, int) error { return nil })
	n, err := outbox.NewPolicyRunner(testDB, []application.Policy{succeeding}).RunDueOnce(context.Background())
	if err != nil {
		t.Fatalf("RunDueOnce: %v", err)
	}
	if n != 1 {
		t.Errorf("after the restart ran %d jobs, want the stopped one", n)
	}
	if left := countPolicyJobs(t); left != 0 {
		t.Errorf("%d jobs left", left)
	}
}

// policyJobState is the row of a testPolicy job.
type policyJobState struct {
	generation int64
	attempts   int
	due        bool
	lastError  string
}

func policyJobOf(t *testing.T, key string) (policyJobState, bool) {
	t.Helper()
	var j policyJobState
	err := testDB.QueryRowContext(context.Background(),
		`SELECT generation, attempts, next_attempt_at <= now(), coalesce(last_error, '')
		 FROM policy_jobs WHERE policy = $1 AND job_key = $2`, testPolicyName, key,
	).Scan(&j.generation, &j.attempts, &j.due, &j.lastError)
	if errors.Is(err, sql.ErrNoRows) {
		return j, false
	}
	if err != nil {
		t.Fatalf("read policy job: %v", err)
	}
	return j, true
}

// enqueueTagCreated saves a new Tag and delivers its tag_created event, which
// enqueues a job of runner's policies.
func enqueueTagCreated(t *testing.T, runner *outbox.PolicyRunner, name string) tag.Tag {
	t.Helper()
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	saved := mustSaveTag(t, repo, createTag(t, repo, name))
	if _, err := outbox.NewDispatcher(testDB, func(event.Event) {}, outbox.WithPolicyRunner(runner)).DrainOnce(context.Background()); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	return saved
}

func runDueOnce(t *testing.T, runner *outbox.PolicyRunner) int {
	t.Helper()
	n, err := runner.RunDueOnce(context.Background())
	if err != nil {
		t.Fatalf("RunDueOnce: %v", err)
	}
	return n
}

func TestPolicyRunner_PolicyPanics_FailureRecordedRunnerGoesOn(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	cleanPolicyJobs(t)
	policy := newTestPolicy(func(context.Context, string, int) error { panic("boom") })
	runner := outbox.NewPolicyRunner(testDB, []application.Policy{policy}, outbox.WithRetryPause(time.Hour, time.Hour))
	key := enqueueTagCreated(t, runner, "panicking").ID().String()

	if n := runDueOnce(t, runner); n != 1 {
		t.Errorf("ran %d jobs, want 1", n)
	}

	job, ok := policyJobOf(t, key)
	if !ok || job.attempts != 1 || job.due || !strings.Contains(job.lastError, "boom") {
		t.Errorf("job %+v (found %t), want one failed once with the panic, waiting", job, ok)
	}
}

func TestPolicyRunner_AttemptOutlastsTimeout_FailsAndOtherJobsRun(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	cleanPolicyJobs(t)
	var hangingKey string
	policy := newTestPolicy(func(ctx context.Context, key string, _ int) error {
		if key == hangingKey {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	})
	runner := outbox.NewPolicyRunner(testDB, []application.Policy{policy},
		outbox.WithRetryPause(time.Hour, time.Hour), outbox.WithAttemptTimeout(50*time.Millisecond))
	hangingKey = enqueueTagCreated(t, runner, "hanging").ID().String()
	otherKey := enqueueTagCreated(t, runner, "other").ID().String()

	if n := runDueOnce(t, runner); n != 2 {
		t.Errorf("ran %d jobs, want 2", n)
	}

	job, ok := policyJobOf(t, hangingKey)
	if !ok || job.attempts != 1 || job.due || !strings.Contains(job.lastError, context.DeadlineExceeded.Error()) {
		t.Errorf("hanging job %+v (found %t), want one failed once on the timeout, waiting", job, ok)
	}
	if _, ok := policyJobOf(t, otherKey); ok || len(policy.runsOf(otherKey)) != 1 {
		t.Errorf("the other job: left %t, ran %d times; want done once", ok, len(policy.runsOf(otherKey)))
	}
}

// sameKeyPolicy keys every tag_created event by one key.
type sameKeyPolicy struct{ *testPolicy }

const sameKey = "same"

func (p sameKeyPolicy) JobKey(e event.Event) (string, bool) {
	if _, ok := p.testPolicy.JobKey(e); ok {
		return sameKey, true
	}
	return "", false
}

func TestPolicyRunner_EventMergesIntoFailedJob_KeepsItsPause(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	cleanPolicyJobs(t)
	policy := sameKeyPolicy{newTestPolicy(func(context.Context, string, int) error { return errors.New("fails") })}
	runner := outbox.NewPolicyRunner(testDB, []application.Policy{policy}, outbox.WithRetryPause(time.Hour, time.Hour))
	enqueueTagCreated(t, runner, "first")
	runDueOnce(t, runner)

	enqueueTagCreated(t, runner, "second")

	job, ok := policyJobOf(t, sameKey)
	if !ok || job.generation != 2 || job.attempts != 1 || job.due {
		t.Errorf("job %+v (found %t), want generation 2, failed once, still waiting", job, ok)
	}
	if n := runDueOnce(t, runner); n != 0 {
		t.Errorf("ran %d jobs before the pause was over, want none", n)
	}
}

func TestPolicyRunner_EventMergesWhileRetrySucceeds_RunsAgainWithFailuresForgotten(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	cleanPolicyJobs(t)
	var runner *outbox.PolicyRunner
	var beforeLastRun policyJobState
	policy := sameKeyPolicy{newTestPolicy(func(_ context.Context, _ string, attempt int) error {
		switch attempt {
		case 1:
			return errors.New("first attempt fails")
		case 2: // the retry succeeds, an event merging in meanwhile
			enqueueTagCreated(t, runner, "second")
		case 3:
			beforeLastRun, _ = policyJobOf(t, sameKey)
		}
		return nil
	})}
	runner = outbox.NewPolicyRunner(testDB, []application.Policy{policy}, outbox.WithRetryPause(time.Hour, time.Hour))
	enqueueTagCreated(t, runner, "first")
	runDueOnce(t, runner)
	if _, err := testDB.ExecContext(context.Background(), `UPDATE policy_jobs SET next_attempt_at = now()`); err != nil {
		t.Fatalf("make the job due: %v", err)
	}

	if n := runDueOnce(t, runner); n != 2 {
		t.Errorf("ran %d attempts, want the retry and one more for the merged event", n)
	}

	if beforeLastRun.attempts != 0 || beforeLastRun.lastError != "" {
		t.Errorf("before the last run the job was %+v, want its failures forgotten", beforeLastRun)
	}
	if _, ok := policyJobOf(t, sameKey); ok {
		t.Error("the job is left, want it done")
	}
}
