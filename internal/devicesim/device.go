// Package devicesim simulates one lower-level Device: it generates Tag values
// by patterns and delivers them to the server through a devicelink.Link, with
// a control API to start/stop it and change patterns and qualities at runtime.
package devicesim

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/kipitix/growscada/internal/devicelink"
)

var (
	// ErrUnknownTag means the Device does not simulate a tag with that name.
	ErrUnknownTag = errors.New("tag is not simulated by this device")
	// ErrConnecting means the Device has not resolved its tags on the server
	// yet, so their types — and which patterns fit them — are unknown.
	ErrConnecting = errors.New("device is still connecting to the server")
)

// Device states, as reported by Status.
const (
	StateConnecting = "connecting"
	StateConnected  = "connected"
)

// Options tune a Device.
type Options struct {
	// Logger receives warnings about failed writes; default slog.Default().
	Logger *slog.Logger
	// RetryDelay is the pause between attempts to reach the server while
	// connecting; default 1s.
	RetryDelay time.Duration
}

// Device is one simulated Device. Its methods are safe for concurrent use.
type Device struct {
	link       devicelink.Link
	logger     *slog.Logger
	retryDelay time.Duration

	mu        sync.Mutex
	connected bool // tags resolved: types and generators are known
	running   bool
	// inFlight counts writes on their way to the server; Stop waits on idle
	// until they are done.
	inFlight int
	idle     *sync.Cond
	// Pattern time runs only while the Device is running: runTotal is the
	// running time before resumedAt, the moment of the last start.
	runTotal  time.Duration
	resumedAt time.Time
	tags      []*simTag
	byName    map[string]*simTag
}

// simTag is one simulated tag; its fields are guarded by Device.mu.
type simTag struct {
	tag      devicelink.Tag // only the name until the Device is connected
	interval time.Duration
	quality  devicelink.Quality  // from the config
	override *devicelink.Quality // set by OverrideQuality, nil if none
	pattern  PatternSpec
	gen      generator     // nil until the Device is connected
	setAt    time.Duration // Device running time when the pattern was set
	rnd      *rand.Rand
	lost     bool          // deleted on the server: no longer written
	kick     chan struct{} // write now instead of waiting for the tick
	// A kick repeats the last value (a random_walk must not take an extra
	// step), unless regen asks for a fresh one after a pattern change.
	regen   bool
	last    string
	written bool // last holds a generated value
}

// New creates the Device of a config. It does not talk to the server: Run
// connects first, so the control API can serve the Device right away.
func New(cfg Config, link devicelink.Link, opts Options) *Device {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = time.Second
	}
	d := &Device{
		link:       link,
		logger:     opts.Logger,
		retryDelay: opts.RetryDelay,
		running:    *cfg.Autostart,
		resumedAt:  time.Now(),
		byName:     make(map[string]*simTag, len(cfg.Tags)),
	}
	d.idle = sync.NewCond(&d.mu)
	for _, tc := range cfg.Tags {
		t := &simTag{
			tag:      devicelink.Tag{Name: tc.Tag},
			interval: tc.Interval.D,
			quality:  tc.Quality,
			pattern:  tc.Pattern,
			rnd:      rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
			kick:     make(chan struct{}, 1),
		}
		d.tags = append(d.tags, t)
		d.byName[tc.Tag] = t
	}
	return d
}

// connect resolves the tags on the server and checks each pattern against the
// tag's type. An unreachable or failing server is waited for without limit, as
// a real Device would; a missing tag or an incompatible pattern is an error.
func (d *Device) connect(ctx context.Context) error {
	resolved := make([]devicelink.Tag, len(d.tags))
	for i, t := range d.tags {
		tag, err := d.resolve(ctx, t.tag.Name)
		if err != nil {
			return err
		}
		resolved[i] = tag
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	for i, t := range d.tags {
		t.tag = resolved[i]
		if err := t.setPattern(t.pattern, 0); err != nil {
			return fmt.Errorf("tag %q: %w", t.tag.Name, err)
		}
	}
	d.connected = true
	// Pattern time starts now, not while waiting for the server.
	d.runTotal, d.resumedAt = 0, time.Now()
	return nil
}

// resolve looks a tag up, retrying while the server is unreachable or failing.
func (d *Device) resolve(ctx context.Context, name string) (devicelink.Tag, error) {
	for warned := false; ; {
		tag, err := d.link.Resolve(ctx, name)
		if err == nil {
			return tag, nil
		}
		if !devicelink.IsTransient(err) {
			return devicelink.Tag{}, err
		}
		if !warned {
			d.logger.Warn("server unavailable, waiting for it", "tag", name, "error", err)
			warned = true
		}
		select {
		case <-ctx.Done():
			return devicelink.Tag{}, fmt.Errorf("tag %q: %w", name, ctx.Err())
		case <-time.After(d.retryDelay):
		}
	}
}

func (t *simTag) setPattern(p PatternSpec, at time.Duration) error {
	gen, err := newGenerator(p, t.tag.Type, t.rnd)
	if err != nil {
		return err
	}
	t.pattern, t.gen, t.setAt = p, gen, at
	return nil
}

func (t *simTag) currentQuality() devicelink.Quality {
	if t.override != nil {
		return *t.override
	}
	return t.quality
}

func (t *simTag) poke() {
	select {
	case t.kick <- struct{}{}:
	default: // a write is already pending
	}
}

// Run connects to the server, then writes every tag at its interval while the
// Device is running, until ctx is done. Each tag is written right away, then
// on every tick. It returns an error only if connecting fails for a reason
// other than ctx being done (a missing tag, an incompatible pattern).
func (d *Device) Run(ctx context.Context) error {
	if err := d.connect(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	d.logger.Info("connected to the server", "tags", len(d.tags))

	var wg sync.WaitGroup
	for _, t := range d.tags {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.runTag(ctx, t)
		}()
	}
	wg.Wait()
	return nil
}

func (d *Device) runTag(ctx context.Context, t *simTag) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	kicked := false
	for {
		d.write(ctx, t, kicked)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			kicked = false
		case <-t.kick:
			kicked = true
		}
	}
}

// write generates the tag's next value and delivers it, if the Device is
// running and the tag still exists. A kicked write repeats the last value
// unless the pattern has changed since.
func (d *Device) write(ctx context.Context, t *simTag, kicked bool) {
	d.mu.Lock()
	if !d.running || t.lost {
		d.mu.Unlock()
		return
	}
	if !kicked || t.regen || !t.written {
		t.last, t.written = t.gen.next(d.runningTime()-t.setAt), true
	}
	t.regen = false
	value := t.last
	quality := t.currentQuality()
	d.inFlight++
	d.mu.Unlock()

	err := d.link.Write(ctx, t.tag, value, quality)

	d.mu.Lock()
	lost := err != nil && ctx.Err() == nil && errors.Is(err, devicelink.ErrTagNotFound)
	t.lost = t.lost || lost
	d.inFlight--
	if d.inFlight == 0 {
		d.idle.Broadcast()
	}
	d.mu.Unlock()

	switch {
	case err == nil, ctx.Err() != nil:
	case lost:
		d.logger.Warn("tag deleted on the server, no longer simulated", "tag", t.tag.Name, "error", err)
	default:
		d.logger.Warn("write failed, will retry on the next tick", "tag", t.tag.Name, "error", err)
	}
}

// runningTime is how long the Device has been running in total; d.mu held.
func (d *Device) runningTime() time.Duration {
	if !d.running {
		return d.runTotal
	}
	return d.runTotal + time.Since(d.resumedAt)
}

// Start resumes writing; pattern time continues where Stop left it.
func (d *Device) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		return
	}
	d.running, d.resumedAt = true, time.Now()
	for _, t := range d.tags {
		t.poke()
	}
}

// Stop pauses writing; the Device and its control API stay up. It returns
// once the writes already on their way to the server are done: from then on
// the Device is silent.
func (d *Device) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		d.runTotal = d.runningTime()
		d.running = false
	}
	for d.inFlight > 0 {
		d.idle.Wait()
	}
}

// SetPattern changes a tag's pattern; its time starts from zero.
func (d *Device) SetPattern(name string, p PatternSpec) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.tagLocked(name)
	if err != nil {
		return err
	}
	if err := t.setPattern(p, d.runningTime()); err != nil {
		return err
	}
	t.regen = true
	t.poke()
	return nil
}

// OverrideQuality makes every write of the tag carry this quality until
// ResetQuality.
func (d *Device) OverrideQuality(name string, quality devicelink.Quality) error {
	if !quality.IsValid() {
		return errors.New("quality is required")
	}
	return d.setOverride(name, &quality)
}

// ResetQuality returns the tag to its configured quality.
func (d *Device) ResetQuality(name string) error {
	return d.setOverride(name, nil)
}

func (d *Device) setOverride(name string, quality *devicelink.Quality) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.tagLocked(name)
	if err != nil {
		return err
	}
	t.override = quality
	t.poke()
	return nil
}

// tagLocked returns a simulated tag once the Device is connected; d.mu held.
func (d *Device) tagLocked(name string) (*simTag, error) {
	t, ok := d.byName[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTag, name)
	}
	if !d.connected {
		return nil, ErrConnecting
	}
	return t, nil
}

// Status is a snapshot of the Device.
type Status struct {
	// State is StateConnecting until the tags are resolved on the server.
	State   string      `json:"state"`
	Running bool        `json:"running"`
	Tags    []TagStatus `json:"tags"`
}

// TagStatus is a snapshot of one simulated tag.
type TagStatus struct {
	Name string `json:"name"`
	// Type is empty while the Device is connecting.
	Type     string             `json:"type"`
	Interval Duration           `json:"interval"`
	Quality  devicelink.Quality `json:"quality"`
	// Overridden is set while OverrideQuality holds the quality.
	Overridden bool        `json:"overridden"`
	Pattern    PatternSpec `json:"pattern"`
	// Lost is set once the tag was deleted on the server.
	Lost bool `json:"lost"`
}

// Status returns a snapshot of the Device.
func (d *Device) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := Status{State: StateConnecting, Running: d.running, Tags: make([]TagStatus, len(d.tags))}
	if d.connected {
		s.State = StateConnected
	}
	for i, t := range d.tags {
		s.Tags[i] = TagStatus{
			Name:       t.tag.Name,
			Interval:   Duration{D: t.interval},
			Quality:    t.currentQuality(),
			Overridden: t.override != nil,
			Pattern:    t.pattern,
			Lost:       t.lost,
		}
		if d.connected {
			s.Tags[i].Type = t.tag.Type.String()
		}
	}
	return s
}
