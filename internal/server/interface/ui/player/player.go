package player

import (
	"errors"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

const (
	// redrawInterval is the shortest interval between two redraws a burst of
	// entries can ask for: applying an entry is immediate, drawing it is not
	// (ADR 0020).
	redrawInterval = 50 * time.Millisecond
	// initialRetryDelay and maxRetryDelay bound the full resync a lost
	// connection asks for: 2 s, doubling to 30 s.
	initialRetryDelay = 2 * time.Second
	maxRetryDelay     = 30 * time.Second
)

// Events are the callbacks a Source reports to. They run on the UI goroutine
// and in the order of the entries' positions; the Player keeps that order.
type Events struct {
	// Entry delivers one Journal entry.
	Entry func(recordv0.Entry)
	// Opened reports that the stream is up: a native retry succeeded.
	Opened func()
	// Lost reports that the stream is down. exhausted is true when the
	// browser will not retry: the Player then resyncs from a new starting
	// state after a backoff, since the entries published while disconnected
	// are not replayed (ADR 0020). A native retry needs none: the stream
	// catches up from the table by its position.
	Lost func(exhausted bool)
}

// Source is where a Player's entries come from. It does not say whether they
// are live or replayed: Operation's is the live Journal stream, History's
// (task 45) will be a playback (ADR 0020). A Source reports on the UI
// goroutine through the callbacks it is opened with, and is done with when
// Close is called.
type Source interface {
	// Open starts delivering the entries after position from, replacing any
	// stream it had open. from is exclusive: the first entry is the one after
	// it.
	Open(ctx app.Context, from int64, events Events)
	// Close ends the source for good; safe on nil and more than once.
	Close()
}

// Player is one Project's operational model over a Source: Operation's is
// live, History's (task 45) replayed. It reads the starting state
// (GET …/operation), applies the entries that follow and asks its owner for a
// redraw in batches. Every method must be called on the UI goroutine, and so
// do the Source's callbacks.
type Player struct {
	ctx      app.Context
	project  *apiclient.ProjectClient
	source   Source
	onChange func()

	model   Model
	updater updateBatcher

	// opens counts the explicit opens; a starting state or a Deployment
	// document read by an older one is dropped.
	opens uint64
	// openSeq counts the Source.Open calls: a callback of a replaced stream
	// is ignored.
	openSeq uint64
	// streamFrom is the position the open stream started after.
	streamFrom int64

	closed         bool
	retryScheduled bool
	retryDelay     time.Duration
	// after schedules a delayed callback on the UI goroutine; ctx.After by
	// default, replaceable by a test.
	after func(time.Duration, func())
}

// New creates the Player of project over source. ctx is the owning component's
// own context: the HTTP reads and the redraws go through it.
func New(ctx app.Context, project *apiclient.ProjectClient, source Source) *Player {
	p := &Player{
		ctx:        ctx,
		project:    project,
		source:     source,
		streamFrom: -1,
		retryDelay: initialRetryDelay,
	}
	p.after = func(d time.Duration, f func()) {
		ctx.After(d, func(app.Context) { f() })
	}
	p.updater = updateBatcher{
		interval: redrawInterval,
		after:    func(d time.Duration, f func()) { p.after(d, f) },
		request: func() {
			if p.closed || p.onChange == nil {
				return
			}
			p.onChange()
		},
	}
	return p
}

// WithOnChange sets what a redraw calls; without it the Player applies the
// entries silently.
func (p *Player) WithOnChange(onChange func()) *Player {
	p.onChange = onChange
	return p
}

// Close ends the stream and drops whatever a read in flight still delivers;
// safe on nil and more than once.
func (p *Player) Close() {
	if p == nil || p.closed {
		return
	}
	p.closed = true
	p.opens++
	if p.source != nil {
		p.source.Close()
	}
}

// Position returns the Journal position the Player has consumed.
func (p *Player) Position() int64 { return p.model.Position() }

// Deployed reports whether the Project runs a Deployment.
func (p *Player) Deployed() bool { return p.model.Deployed() }

// Connected reports whether the stream is up.
func (p *Player) Connected() bool { return p.model.Connected() }

// View returns the picture Operation renders.
func (p *Player) View() View { return p.model.View() }

// Resync is the explicit open: it reads a starting state (GET …/operation)
// and opens the stream after its position. It runs on mount, on a switch of
// Project and on a reconnect the browser has given up on (ADR 0020). The
// requests are read in the Runtime node's order, so a Deploy committed in
// between has a position above the snapshot and reaches the stream.
func (p *Player) Resync() {
	if p == nil || p.closed || p.project == nil {
		return
	}
	p.retryScheduled = false
	p.opens++
	open := p.opens
	p.ctx.Async(func() {
		snapshot, err := p.project.Operation(p.ctx)
		p.ctx.Dispatch(func(ctx app.Context) {
			if p.closed || open != p.opens {
				return
			}
			if err != nil {
				p.resyncFailed(ctx, err)
				return
			}
			p.Start(snapshot)
		})
	})
}

// Start applies a starting state and opens the stream after its position. It
// is what Resync does once the state is read, and what a playback begins
// with (task 45). A starting state is current until the stream says
// otherwise, so the picture stops showing disconnected at once.
func (p *Player) Start(snapshot apiv0.OperationStateResponse) {
	if p.closed {
		return
	}
	p.retryDelay = initialRetryDelay
	p.model.MergeSnapshot(snapshot)
	p.model.SetConnected(true)
	p.updater.Request()
	p.open()
}

// resyncFailed handles a starting state the server refused: a Project that is
// gone is announced (the switcher moves to another one), any other failure is
// a lost connection, retried with a backoff.
func (p *Player) resyncFailed(ctx app.Context, err error) {
	p.model.SetConnected(false)
	p.updater.Request()
	if errors.Is(err, apiclient.ErrNotFound) {
		ctx.NewActionWithValue(uiutil.ActionProjectGone, p.project.ID())
		return
	}
	p.scheduleResync()
}

// open starts the Source after the model's position, replacing the stream it
// had. Callbacks of the replaced stream are ignored.
func (p *Player) open() {
	if p.closed || p.source == nil {
		return
	}
	p.openSeq++
	open := p.openSeq
	p.streamFrom = p.model.Position()
	p.source.Open(p.ctx, p.streamFrom, Events{
		Entry: func(entry recordv0.Entry) {
			if p.closed || open != p.openSeq {
				return
			}
			p.apply(entry)
		},
		Opened: func() {
			if p.closed || open != p.openSeq {
				return
			}
			p.streamOpened()
		},
		Lost: func(exhausted bool) {
			if p.closed || open != p.openSeq {
				return
			}
			p.streamLost(exhausted)
		},
	})
}

// apply applies one entry. A deployment entry names a document the Runtime node
// stores: it is read asynchronously and applied by its number.
func (p *Player) apply(entry recordv0.Entry) {
	result := p.model.Apply(entry)
	if result.Changed {
		p.updater.Request()
	}
	if result.Deployment > 0 {
		p.loadDeployment(result.Deployment)
	}
}

// loadDeployment reads the document of Deployment number and swaps the
// configuration to it. A failure is lost connectivity: the old configuration
// stays on screen, the view shows disconnected and the full resync retries
// (ADR 0020).
func (p *Player) loadDeployment(number int) {
	p.ctx.Async(func() {
		deployment, err := p.project.DeploymentByNumber(p.ctx, number)
		p.ctx.Dispatch(func(ctx app.Context) {
			if p.closed {
				return
			}
			if err != nil {
				// A read the Player has overtaken (a newer Deployment is
				// already on screen) says nothing about the connection.
				if p.deploymentNeeded(number) {
					p.streamLost(true)
				}
				return
			}
			if p.model.SetDeployment(deployment) {
				p.updater.Request()
			}
		})
	})
}

// deploymentNeeded reports whether the document of Deployment number is still the
// one the Player runs: a read of an older Deployment may arrive after a newer
// one was applied, and its failure is not a lost connection.
func (p *Player) deploymentNeeded(number int) bool {
	deployment := p.model.Deployment()
	return deployment == nil || number > deployment.Number
}

// streamOpened marks the connection up. If the starting state has advanced
// since the stream was opened from, the stream is reopened from the newer
// position: the entries between the two are not replayed by it.
func (p *Player) streamOpened() {
	if p.model.Position() > p.streamFrom {
		p.open()
		return
	}
	p.retryDelay = initialRetryDelay
	if p.model.SetConnected(true) {
		p.updater.Request()
	}
}

// streamLost marks the connection down. A stream the browser will retry needs
// nothing else: it reads the table from its position. One it has given up on
// gets a full resync after a backoff.
func (p *Player) streamLost(exhausted bool) {
	if p.model.SetConnected(false) {
		p.updater.Request()
	}
	if exhausted {
		p.scheduleResync()
	}
}

// scheduleResync schedules one full resync after the current backoff, which
// then doubles up to maxRetryDelay. Repeated losses share the one retry.
func (p *Player) scheduleResync() {
	if p.closed || p.retryScheduled {
		return
	}
	p.retryScheduled = true
	delay := p.retryDelay
	if p.retryDelay < maxRetryDelay {
		p.retryDelay *= 2
		if p.retryDelay > maxRetryDelay {
			p.retryDelay = maxRetryDelay
		}
	}
	// An explicit open meanwhile (a switch of Project, a mount) has already
	// resynced: the scheduled one would only duplicate it.
	open := p.opens
	p.after(delay, func() {
		p.retryScheduled = false
		if p.opens != open {
			return
		}
		p.Resync()
	})
}
