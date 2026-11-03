// Package devicesim simulates one lower-level Device: it reads the
// configuration of the Device a Device Token names, generates Tag values by
// patterns and delivers them to the server through a devicelink.Link, with a
// control API to start/stop it and change patterns and qualities at runtime.
package devicesim

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/devicelink"
)

var (
	// ErrUnknownTag means the Device does not simulate a tag with that name.
	ErrUnknownTag = errors.New("tag is not simulated by this device")
	// ErrConnecting means the Device has not read its configuration on the
	// server yet, so their types — and which patterns fit them — are unknown.
	ErrConnecting = errors.New("device is still connecting to the server")
	// ErrConfig means the local config cannot be run against the server's
	// configuration of the Device: a Tag it does not supply, a TagType that is
	// not known, or a pattern that does not fit its Tag.
	ErrConfig = errors.New("device configuration error")
)

// Device states, as reported by Status.
const (
	StateConnecting = "connecting"
	StateConnected  = "connected"
)

// DefaultConfigReload is how often a Device rereads its configuration when
// the event stream says nothing.
const DefaultConfigReload = 30 * time.Second

// Options tune a Device.
type Options struct {
	// Logger receives warnings about failed writes; default slog.Default().
	Logger *slog.Logger
	// RetryDelay is the pause between attempts to reach the server while
	// connecting, and between reconnections of the event stream; default 1s.
	RetryDelay time.Duration
	// ConfigReload is the fallback interval of rereading the Device's
	// configuration, besides the Gateway's event stream; default
	// DefaultConfigReload.
	ConfigReload time.Duration
}

// Device is one simulated Device, the Device Gateway of one Environment. Its
// methods are safe for concurrent use.
type Device struct {
	link         devicelink.Link
	source       Source
	logger       *slog.Logger
	retryDelay   time.Duration
	configReload time.Duration

	mu        sync.Mutex
	connected bool // configuration read: types and generators are known
	running   bool
	// inFlight counts writes on their way to the server; Stop waits on idle
	// until they are done.
	inFlight int
	idle     *sync.Cond
	// The server's configuration of the Device, as last read.
	deviceID    uuid.UUID
	deviceName  string
	protocol    string
	environment string
	// Pattern time runs only while the Device is running: runTotal is the
	// running time before resumedAt, the moment of the last start.
	runTotal  time.Duration
	resumedAt time.Time
	tags      []*simTag
	byName    map[string]*simTag
}

// simTag is one simulated tag; its fields are guarded by Device.mu.
type simTag struct {
	tag      devicelink.Tag // only the name until the configuration is read
	interval time.Duration
	quality  devicelink.Quality  // from the config
	override *devicelink.Quality // set by OverrideQuality, nil if none
	pattern  PatternSpec
	gen      generator     // nil until the Device is connected, and while no pattern fits the tag's type
	setAt    time.Duration // Device running time when the pattern was set
	rnd      *rand.Rand
	lost     bool          // not supplied by the Device any more: not written
	kick     chan struct{} // write now instead of waiting for the tick
	// A kick repeats the last value (a random_walk must not take an extra
	// step), unless regen asks for a fresh one after a pattern change.
	regen   bool
	last    string
	written bool // last holds a generated value
}

// New creates the Device of a config. It does not talk to the server: Run
// connects first, so the control API can serve the Device right away.
func New(cfg Config, link devicelink.Link, source Source, opts Options) *Device {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = time.Second
	}
	if opts.ConfigReload <= 0 {
		opts.ConfigReload = DefaultConfigReload
	}
	d := &Device{
		link:         link,
		source:       source,
		logger:       opts.Logger,
		retryDelay:   opts.RetryDelay,
		configReload: opts.ConfigReload,
		running:      *cfg.Autostart,
		resumedAt:    time.Now(),
		byName:       make(map[string]*simTag, len(cfg.Tags)),
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

// connect reads the Device's configuration on the server and checks each
// simulated tag against it: the tag must be one the Device supplies, and its
// pattern must fit the tag's type. An unreachable or failing server is waited
// for without limit, as a real Device would; a token the server refuses, a
// missing tag or an incompatible pattern is an error.
func (d *Device) connect(ctx context.Context) error {
	cfg, err := d.fetchConfiguration(ctx)
	if err != nil {
		return err
	}
	if err := d.applyConfiguration(cfg, true); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = true
	// Pattern time starts now, not while waiting for the server.
	d.runTotal, d.resumedAt = 0, time.Now()
	return nil
}

// fetchConfiguration asks the server for the Device's configuration,
// retrying while the server is unreachable or failing, and while it says the
// Device is not in the configuration the Gateway asks about (403): a Deploy or
// an operator brings it back.
func (d *Device) fetchConfiguration(ctx context.Context) (apiv0.DeviceConfigResponse, error) {
	for warned := false; ; {
		cfg, err := d.source.Configuration(ctx)
		if err == nil {
			return cfg, nil
		}
		if !devicelink.IsTransient(err) && !devicelink.IsForbidden(err) {
			return apiv0.DeviceConfigResponse{}, err
		}
		if !warned {
			d.logger.Warn("cannot read the device's configuration yet, waiting", "error", err)
			warned = true
		}
		select {
		case <-ctx.Done():
			return apiv0.DeviceConfigResponse{}, ctx.Err()
		case <-time.After(d.retryDelay):
		}
	}
}

// applyConfiguration brings the simulated tags in line with the Device's
// configuration on the server: the Tag each simulated tag writes, its type and
// whether the Device supplies it at all. At connect (strict) a tag the Device
// does not supply, or a pattern that does not fit its type, is an error; on a
// reload the same stops the tag from being written, so the Device keeps
// running whatever its configuration became.
func (d *Device) applyConfiguration(cfg apiv0.DeviceConfigResponse, strict bool) error {
	supplied := make(map[string]apiv0.DeviceConfigTagResponse, len(cfg.Tags))
	for _, tag := range cfg.Tags {
		supplied[tag.Name] = tag
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.deviceID, d.deviceName, d.protocol = cfg.Device.ID, cfg.Device.Name, cfg.Device.Protocol
	d.environment = cfg.Environment.Name
	// Pattern time is counted from the moment the Device connects: before
	// that, the time spent waiting for the server does not move the patterns.
	at := d.runningTime()
	if !d.connected {
		at = 0
	}

	for _, t := range d.tags {
		name := t.tag.Name
		definition, ok := supplied[name]
		if !ok {
			if strict {
				return fmt.Errorf("%w: tag %q is not supplied by the device", ErrConfig, name)
			}
			if !t.lost {
				d.logger.Warn("tag is not supplied by the device any more, no longer simulated", "tag", name)
			}
			t.lost = true
			continue
		}
		tagType, err := devicelink.ParseTagType(definition.Type)
		if err != nil {
			if strict {
				return fmt.Errorf("%w: tag %q: %w", ErrConfig, name, err)
			}
			d.logger.Warn("tag has an unknown type, no longer simulated", "tag", name, "error", err)
			t.lost = true
			continue
		}
		if t.tag.ID == definition.ID && t.tag.Type == tagType && t.gen != nil {
			t.lost = false // supplied again, or never lost
			continue
		}
		t.tag = devicelink.Tag{ID: definition.ID, Name: name, Type: tagType}
		// The tag's identity or type changed: build its generator for the new
		// type. A pattern that does not fit it leaves the tag without a
		// generator, so the next reload tries again and the tag stays lost
		// meanwhile — never the old type's generator under the new identity.
		gen, err := newGenerator(t.pattern, tagType, t.rnd)
		if err != nil {
			t.gen = nil
			if strict {
				return fmt.Errorf("%w: tag %q: %w", ErrConfig, name, err)
			}
			d.logger.Warn("pattern does not fit the tag, no longer simulated", "tag", name, "error", err)
			t.lost = true
			continue
		}
		t.gen, t.setAt = gen, at
		t.lost = false
		if d.connected {
			// A reload: the tag's identity or type changed, so its next value
			// is fresh rather than the last one again.
			t.regen = true
			t.poke()
		}
	}
	return nil
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
// Device is running, until ctx is done. The Device's configuration is kept
// current in the background: on the Gateway's configuration events and, as a
// fallback, on a timer. It returns an error only if connecting fails for a
// reason other than ctx being done, or the server refuses the Device's token
// while running (a revoked one): a Gateway cannot fix either. A server that
// merely refuses the configuration (the Device is not in it) is waited for.
func (d *Device) Run(ctx context.Context) error {
	if err := d.connect(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	d.mu.Lock()
	device, environment := d.deviceName, d.environment
	d.mu.Unlock()
	d.logger.Info("connected to the server", "device", device, "environment", environment, "tags", len(d.tags))

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	var wg sync.WaitGroup
	fatal := make(chan error, 1)
	for _, t := range d.tags {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.runTag(runCtx, t); err != nil {
				select {
				case fatal <- err:
				default: // the first fatal error is the one reported
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.watch(runCtx, fatal)
	}()

	select {
	case <-ctx.Done():
		stop()
		wg.Wait()
		return nil
	case err := <-fatal:
		stop()
		wg.Wait()
		return err
	}
}

// runTag writes the tag every interval until ctx is done. A token the server
// refuses is returned: the Gateway ends rather than retrying.
func (d *Device) runTag(ctx context.Context, t *simTag) error {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	kicked := false
	for {
		err := d.write(ctx, t, kicked)
		if devicelink.IsUnauthenticated(err) {
			return fmt.Errorf("tag %q: %w", t.tag.Name, err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			kicked = false
		case <-t.kick:
			kicked = true
		}
	}
}

// write generates the tag's next value and delivers it, if the Device is
// running and the Device still supplies the tag. A kicked write repeats the
// last value unless the pattern has changed since. A tag the server refuses to
// write (403) is no longer simulated until the configuration is read again.
func (d *Device) write(ctx context.Context, t *simTag, kicked bool) error {
	d.mu.Lock()
	if !d.running || t.lost || t.gen == nil {
		d.mu.Unlock()
		return nil
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
	gone := err != nil && ctx.Err() == nil && errors.Is(err, devicelink.ErrTagNotFound)
	refused := err != nil && ctx.Err() == nil && devicelink.IsForbidden(err)
	t.lost = t.lost || gone || refused
	d.inFlight--
	if d.inFlight == 0 {
		d.idle.Broadcast()
	}
	d.mu.Unlock()

	switch {
	case err == nil, ctx.Err() != nil:
	case gone:
		d.logger.Warn("tag deleted on the server, no longer simulated", "tag", t.tag.Name, "error", err)
	case refused:
		d.logger.Warn("the server refuses writes of the tag, no longer simulated", "tag", t.tag.Name, "error", err)
	default:
		d.logger.Warn("write failed, will retry on the next tick", "tag", t.tag.Name, "error", err)
	}
	return err
}

// watch keeps the Device's configuration current: it follows the Gateway's
// configuration event stream, and rereads the configuration on a timer as a
// fallback. A token the server refuses is fatal; a configuration the server
// refuses to serve (403) is waited for.
func (d *Device) watch(ctx context.Context, fatal chan<- error) {
	kick := make(chan struct{}, 1)
	poke := func() {
		select {
		case kick <- struct{}{}:
		default: // a reload is already pending
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		d.followEvents(ctx, poke, fatal)
	}()
	go func() {
		defer wg.Done()
		d.reloadOnSchedule(ctx, kick, fatal)
	}()
	wg.Wait()
}

// followEvents subscribes to the Gateway's configuration events, reconnecting
// after a break and rereading the configuration then: the stream replays
// nothing, so the configuration may have changed while it was down.
func (d *Device) followEvents(ctx context.Context, poke func(), fatal chan<- error) {
	for {
		err := d.source.Events(ctx, func(apiv0.Event) { poke() })
		if ctx.Err() != nil {
			return
		}
		if devicelink.IsUnauthenticated(err) {
			fatal <- fmt.Errorf("the server refused the device: %w", err)
			return
		}
		d.logger.Warn("configuration event stream ended, reconnecting", "error", err)
		poke()
		select {
		case <-ctx.Done():
			return
		case <-time.After(d.retryDelay):
		}
	}
}

// reloadOnSchedule rereads the configuration when the event stream asks for
// it and, as a fallback, every ConfigReload.
func (d *Device) reloadOnSchedule(ctx context.Context, kick <-chan struct{}, fatal chan<- error) {
	ticker := time.NewTicker(d.configReload)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-kick:
		}
		err := d.reload(ctx)
		switch {
		case err == nil, ctx.Err() != nil:
		case devicelink.IsUnauthenticated(err):
			fatal <- err
			return
		case devicelink.IsForbidden(err):
			// The Device is not in the configuration the Gateway asked about:
			// its Device may come back, so the refusal is waited for, as on the
			// first read.
			d.logger.Warn("the server refuses the device's configuration, waiting", "error", err)
		default:
			d.logger.Warn("configuration reload failed, will retry", "error", err)
		}
	}
}

// reload rereads the Device's configuration and applies it.
func (d *Device) reload(ctx context.Context) error {
	cfg, err := d.source.Configuration(ctx)
	if err != nil {
		return err
	}
	if err := d.applyConfiguration(cfg, false); err != nil {
		return err
	}
	d.logger.Info("configuration reloaded", "device", cfg.Device.Name, "environment", cfg.Environment.Name, "tags", len(cfg.Tags))
	return nil
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
	// State is StateConnecting until the configuration is read on the server.
	State string `json:"state"`
	// DeviceID, DeviceName, Protocol and Environment are the server's
	// configuration of the Device; empty until it is read.
	DeviceID    uuid.UUID   `json:"device_id"`
	DeviceName  string      `json:"device_name"`
	Protocol    string      `json:"protocol"`
	Environment string      `json:"environment"`
	Running     bool        `json:"running"`
	Tags        []TagStatus `json:"tags"`
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
	// Lost is set once the Device no longer supplies the tag: it was deleted
	// on the server or taken out of the Device's configuration.
	Lost bool `json:"lost"`
}

// Status returns a snapshot of the Device.
func (d *Device) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := Status{
		State:       StateConnecting,
		DeviceID:    d.deviceID,
		DeviceName:  d.deviceName,
		Protocol:    d.protocol,
		Environment: d.environment,
		Running:     d.running,
		Tags:        make([]TagStatus, len(d.tags)),
	}
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
