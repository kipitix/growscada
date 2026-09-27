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

// ErrUnknownTag means the Device does not simulate a tag with that name.
var ErrUnknownTag = errors.New("tag is not simulated by this device")

// Options tune how a Device is opened.
type Options struct {
	// Logger receives warnings about failed writes; default slog.Default().
	Logger *slog.Logger
	// StartupTimeout bounds waiting for the server while resolving tags;
	// default 30s.
	StartupTimeout time.Duration
	// RetryDelay is the pause between resolve attempts; default 1s.
	RetryDelay time.Duration
}

// Device is one simulated Device. Its methods are safe for concurrent use.
type Device struct {
	link   devicelink.Link
	logger *slog.Logger

	mu      sync.Mutex
	running bool
	// Pattern time runs only while the Device is running: runTotal is the
	// running time before resumedAt, the moment of the last start.
	runTotal  time.Duration
	resumedAt time.Time
	tags      []*simTag
	byName    map[string]*simTag
}

// simTag is one simulated tag; its fields are guarded by Device.mu.
type simTag struct {
	tag      devicelink.Tag
	interval time.Duration
	quality  string // from the config
	override string // forced quality, "" if none
	pattern  PatternSpec
	gen      generator
	setAt    time.Duration // Device running time when the pattern was set
	rnd      *rand.Rand
	lost     bool          // deleted on the server: no longer written
	kick     chan struct{} // write now instead of waiting for the tick
}

// Open resolves the config's tags on the server and checks each pattern against
// the tag's type. Network errors and server failures are retried until
// StartupTimeout; a missing tag or an incompatible pattern fails at once.
// Nothing is written before Run.
func Open(ctx context.Context, cfg Config, link devicelink.Link, opts Options) (*Device, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.StartupTimeout <= 0 {
		opts.StartupTimeout = 30 * time.Second
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.StartupTimeout)
	defer cancel()

	d := &Device{
		link:    link,
		logger:  opts.Logger,
		running: *cfg.Autostart,
		byName:  make(map[string]*simTag, len(cfg.Tags)),
	}
	d.resumedAt = time.Now()
	for _, tc := range cfg.Tags {
		resolved, err := resolve(ctx, link, tc.Tag, opts)
		if err != nil {
			return nil, err
		}
		t := &simTag{
			tag:      resolved,
			interval: tc.Interval.D,
			quality:  tc.Quality,
			rnd:      rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
			kick:     make(chan struct{}, 1),
		}
		if err := t.setPattern(tc.Pattern, 0); err != nil {
			return nil, fmt.Errorf("tag %q: %w", tc.Tag, err)
		}
		d.tags = append(d.tags, t)
		d.byName[tc.Tag] = t
	}
	return d, nil
}

// resolve looks a tag up, retrying while the server is unreachable or failing.
func resolve(ctx context.Context, link devicelink.Link, name string, opts Options) (devicelink.Tag, error) {
	for {
		tag, err := link.Resolve(ctx, name)
		if err == nil {
			return tag, nil
		}
		if !devicelink.IsTransient(err) {
			return devicelink.Tag{}, err
		}
		opts.Logger.Warn("server unavailable, retrying", "tag", name, "error", err)
		select {
		case <-ctx.Done():
			return devicelink.Tag{}, fmt.Errorf("tag %q: server did not become available: %w", name, err)
		case <-time.After(opts.RetryDelay):
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

func (t *simTag) currentQuality() string {
	if t.override != "" {
		return t.override
	}
	return t.quality
}

func (t *simTag) poke() {
	select {
	case t.kick <- struct{}{}:
	default: // a write is already pending
	}
}

// Run writes every tag at its interval while the Device is running, until ctx
// is done. Each tag is written right away, then on every tick.
func (d *Device) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, t := range d.tags {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.runTag(ctx, t)
		}()
	}
	wg.Wait()
}

func (d *Device) runTag(ctx context.Context, t *simTag) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		d.write(ctx, t)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-t.kick:
		}
	}
}

// write generates the tag's next value and delivers it, if the Device is
// running and the tag still exists.
func (d *Device) write(ctx context.Context, t *simTag) {
	d.mu.Lock()
	if !d.running || t.lost {
		d.mu.Unlock()
		return
	}
	value := t.gen.next(d.runningTime() - t.setAt)
	quality := t.currentQuality()
	d.mu.Unlock()

	err := d.link.Write(ctx, t.tag, value, quality)
	switch {
	case err == nil, ctx.Err() != nil:
	case errors.Is(err, devicelink.ErrTagNotFound):
		d.mu.Lock()
		t.lost = true
		d.mu.Unlock()
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

// Stop pauses writing; the Device and its control API stay up.
func (d *Device) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running {
		return
	}
	d.runTotal = d.runningTime()
	d.running = false
}

// SetPattern changes a tag's pattern; its time starts from zero.
func (d *Device) SetPattern(name string, p PatternSpec) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.byName[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownTag, name)
	}
	if err := t.setPattern(p, d.runningTime()); err != nil {
		return err
	}
	t.poke()
	return nil
}

// ForceQuality makes every write of the tag carry this quality until
// ResetQuality.
func (d *Device) ForceQuality(name, quality string) error {
	if err := validateQuality(quality); err != nil {
		return err
	}
	return d.setOverride(name, quality)
}

// ResetQuality returns the tag to its configured quality.
func (d *Device) ResetQuality(name string) error {
	return d.setOverride(name, "")
}

func (d *Device) setOverride(name, quality string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.byName[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownTag, name)
	}
	t.override = quality
	t.poke()
	return nil
}

// Status is a snapshot of the Device.
type Status struct {
	Running bool        `json:"running"`
	Tags    []TagStatus `json:"tags"`
}

// TagStatus is a snapshot of one simulated tag.
type TagStatus struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Interval Duration    `json:"interval"`
	Quality  string      `json:"quality"`
	Forced   bool        `json:"forced"`
	Pattern  PatternSpec `json:"pattern"`
	// Lost is set once the tag was deleted on the server.
	Lost bool `json:"lost"`
}

// Status returns a snapshot of the Device.
func (d *Device) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := Status{Running: d.running, Tags: make([]TagStatus, len(d.tags))}
	for i, t := range d.tags {
		s.Tags[i] = TagStatus{
			Name:     t.tag.Name,
			Type:     t.tag.Type,
			Interval: Duration{D: t.interval},
			Quality:  t.currentQuality(),
			Forced:   t.override != "",
			Pattern:  t.pattern,
			Lost:     t.lost,
		}
	}
	return s
}
