package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/event"
)

const (
	// defaultRetryPause is how long a job waits after its first failure; the
	// pause doubles with each further failure up to defaultMaxRetryPause.
	defaultRetryPause    = time.Second
	defaultMaxRetryPause = 5 * time.Minute
	// failedLookupRetry is how soon Run looks into the jobs again after it
	// failed to learn when the next one is due.
	failedLookupRetry = time.Second
	// finishTimeout bounds the write that ends a job's attempt, which goes on
	// after the runner's ctx is done: a success left unrecorded would run the
	// job again.
	finishTimeout = 5 * time.Second
)

// PolicyRunner runs the policy jobs (ADR 0008) one by one in one goroutine:
// the Dispatcher enqueues them in the policy_jobs table as it delivers the
// events, and wakes the runner. A job that succeeds is deleted, unless an
// event merged into it while it ran (its generation was raised): then it runs
// again. A job that fails is retried forever, after a pause that doubles with
// each failure, from the retry pause up to the maximum one; each failure is
// logged and kept in last_error, and a further event for the job's key makes
// it due at once. A job stopped by shutdown runs again after the next start.
// There is one PolicyRunner per database, like the Dispatcher.
type PolicyRunner struct {
	db            *sql.DB
	policies      []application.Policy
	byName        map[string]application.Policy
	retryPause    time.Duration
	maxRetryPause time.Duration
	wake          chan struct{}

	// mu makes RunDueOnce calls take turns, so no job runs twice at once.
	mu sync.Mutex
}

// PolicyRunnerOption configures a PolicyRunner.
type PolicyRunnerOption func(*PolicyRunner)

// WithRetryPause sets the pause after a job's first failure and the most it
// grows to.
func WithRetryPause(first, max time.Duration) PolicyRunnerOption {
	return func(r *PolicyRunner) {
		r.retryPause = first
		r.maxRetryPause = max
	}
}

// NewPolicyRunner returns a PolicyRunner running the jobs of policies in
// db's policy_jobs; jobs of a policy it does not know wait untouched. Policy
// names must be unique.
func NewPolicyRunner(db *sql.DB, policies []application.Policy, opts ...PolicyRunnerOption) *PolicyRunner {
	r := &PolicyRunner{
		db:            db,
		policies:      policies,
		byName:        make(map[string]application.Policy, len(policies)),
		retryPause:    defaultRetryPause,
		maxRetryPause: defaultMaxRetryPause,
		wake:          make(chan struct{}, 1),
	}
	for _, p := range policies {
		if _, dup := r.byName[p.Name()]; dup {
			panic(fmt.Sprintf("outbox: two policies named %q", p.Name()))
		}
		r.byName[p.Name()] = p
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Notify tells Run that jobs were enqueued. It never blocks.
func (r *PolicyRunner) Notify() {
	select {
	case r.wake <- struct{}{}:
	default: // a wake-up is already pending
	}
}

// policyJob is a job the Dispatcher enqueues: a policy and the key of what it
// brings into line.
type policyJob struct {
	policy string
	key    string
}

// jobsFor returns the jobs the policies need for e.
func (r *PolicyRunner) jobsFor(e event.Event) []policyJob {
	var jobs []policyJob
	for _, p := range r.policies {
		if key, ok := p.JobKey(e); ok {
			jobs = append(jobs, policyJob{policy: p.Name(), key: key})
		}
	}
	return jobs
}

// enqueue writes job in tx, the transaction deleting the delivered outbox
// rows: a waiting job with the same key takes the new event in, its
// generation raised, and becomes due at once.
func enqueue(ctx context.Context, tx *sql.Tx, job policyJob) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO policy_jobs (policy, job_key) VALUES ($1, $2)
		 ON CONFLICT (policy, job_key) DO UPDATE
		 SET generation = policy_jobs.generation + 1, attempts = 0, next_attempt_at = now()`,
		job.policy, job.key,
	); err != nil {
		return fmt.Errorf("cannot enqueue %s job %q: %w", job.policy, job.key, err)
	}
	return nil
}

// Run runs the due jobs until ctx is done: on every Notify and when the next
// waiting job becomes due. An error is logged and retried.
func (r *PolicyRunner) Run(ctx context.Context) {
	for {
		if _, err := r.RunDueOnce(ctx); err != nil && ctx.Err() == nil {
			slog.Error("policy jobs failed to run", "error", err)
		}
		var due <-chan time.Time // nil: no job waits
		wait, ok, err := r.untilNextDue(ctx)
		switch {
		case err != nil:
			if ctx.Err() == nil {
				slog.Error("cannot read policy jobs", "error", err)
			}
			due = time.After(failedLookupRetry)
		case ok:
			due = time.After(wait)
		}
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-due:
		}
	}
}

// RunDueOnce runs, one by one, every job that is due, including those that
// become due again while it runs (an event merged into them), and returns how
// many attempts it made. Once ctx is done no further job is taken.
func (r *PolicyRunner) RunDueOnce(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ran := 0
	for {
		if err := ctx.Err(); err != nil {
			return ran, err
		}
		job, ok, err := r.takeDue(ctx)
		if err != nil || !ok {
			return ran, err
		}
		ran++
		if err := r.attempt(ctx, job); err != nil {
			return ran, err
		}
	}
}

// takenJob is a due job as the runner took it: the generation it took is the
// one a success or failure is recorded for.
type takenJob struct {
	policyJob
	generation int64
	attempts   int
}

// takeDue reads the job that has been due longest.
func (r *PolicyRunner) takeDue(ctx context.Context) (takenJob, bool, error) {
	var job takenJob
	err := r.db.QueryRowContext(ctx,
		`SELECT policy, job_key, generation, attempts FROM policy_jobs
		 WHERE policy = ANY($1) AND next_attempt_at <= now()
		 ORDER BY next_attempt_at LIMIT 1`,
		pq.Array(r.policyNames()),
	).Scan(&job.policy, &job.key, &job.generation, &job.attempts)
	if err == sql.ErrNoRows {
		return job, false, nil
	}
	if err != nil {
		return job, false, fmt.Errorf("cannot read due policy jobs: %w", err)
	}
	return job, true, nil
}

// untilNextDue returns how soon the next waiting job is due (zero or less:
// due now), and false if no job waits.
func (r *PolicyRunner) untilNextDue(ctx context.Context) (time.Duration, bool, error) {
	var seconds sql.NullFloat64
	if err := r.db.QueryRowContext(ctx,
		`SELECT EXTRACT(EPOCH FROM min(next_attempt_at) - now()) FROM policy_jobs WHERE policy = ANY($1)`,
		pq.Array(r.policyNames()),
	).Scan(&seconds); err != nil {
		return 0, false, fmt.Errorf("cannot read when the next policy job is due: %w", err)
	}
	if !seconds.Valid {
		return 0, false, nil
	}
	return time.Duration(seconds.Float64 * float64(time.Second)), true, nil
}

// attempt runs job's policy and records the outcome for the generation it
// took. An attempt stopped by ctx records nothing: the job stays due.
func (r *PolicyRunner) attempt(ctx context.Context, job takenJob) error {
	runErr := r.byName[job.policy].Run(ctx, job.key)
	if runErr != nil && ctx.Err() != nil {
		return nil
	}

	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()
	if runErr == nil {
		if _, err := r.db.ExecContext(finishCtx,
			`DELETE FROM policy_jobs WHERE policy = $1 AND job_key = $2 AND generation = $3`,
			job.policy, job.key, job.generation,
		); err != nil {
			return fmt.Errorf("cannot delete the done %s job %q: %w", job.policy, job.key, err)
		}
		return nil
	}

	attempts := job.attempts + 1
	pause := r.pauseAfter(attempts)
	slog.Error("policy job failed", "policy", job.policy, "key", job.key,
		"attempts", attempts, "retry_in", pause, "error", runErr)
	if _, err := r.db.ExecContext(finishCtx,
		`UPDATE policy_jobs
		 SET attempts = $4, next_attempt_at = now() + $5 * interval '1 microsecond', last_error = $6
		 WHERE policy = $1 AND job_key = $2 AND generation = $3`,
		job.policy, job.key, job.generation, attempts, pause.Microseconds(), runErr.Error(),
	); err != nil {
		return fmt.Errorf("cannot record the failure of the %s job %q: %w", job.policy, job.key, err)
	}
	return nil
}

// pauseAfter returns how long a job waits after its attempts-th failure in a
// row: the retry pause, doubled for each failure before, at most the maximum
// pause.
func (r *PolicyRunner) pauseAfter(attempts int) time.Duration {
	pause := r.retryPause
	for i := 1; i < attempts && pause < r.maxRetryPause; i++ {
		pause *= 2
	}
	return min(pause, r.maxRetryPause)
}

func (r *PolicyRunner) policyNames() []string {
	names := make([]string, 0, len(r.policies))
	for _, p := range r.policies {
		names = append(names, p.Name())
	}
	return names
}
