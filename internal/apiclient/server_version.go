package apiclient

import (
	"sync"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// Repeat says how often WarnOlderServer warns about an older server.
type Repeat int

const (
	// WarnOnce warns once for the client's lifetime: a short-lived tool, such
	// as a run of growctl.
	WarnOnce Repeat = iota
	// WarnOnChange warns again each time the server's version changes: a
	// long-running Device, whose server may be replaced under it.
	WarnOnChange
)

// WarnOlderServer makes the client call warn when the server speaks an older
// API than the client (apiv0.SchemaVersion()), as often as repeat says. A
// newer server is fine: responses are read tolerantly (ADR 0005). warn runs on
// the goroutine of the call.
func WarnOlderServer(repeat Repeat, warn func(server, client contract.SchemaVersion)) Option {
	w := &olderServerWatch{repeat: repeat, warn: warn}
	return OnServerVersion(w.observe)
}

type olderServerWatch struct {
	repeat Repeat
	warn   func(server, client contract.SchemaVersion)

	mu     sync.Mutex
	last   contract.SchemaVersion
	seen   bool
	warned bool
}

func (w *olderServerWatch) observe(server contract.SchemaVersion) {
	own := apiv0.SchemaVersion()
	w.mu.Lock()
	changed := !w.seen || w.last != server
	w.last, w.seen = server, true
	tell := changed && own.NewerThan(server) && (w.repeat == WarnOnChange || !w.warned)
	w.warned = w.warned || tell
	w.mu.Unlock()
	if tell {
		w.warn(server, own)
	}
}
