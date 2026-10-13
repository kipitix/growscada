package devicelink

import (
	"log/slog"
	"sync"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// WarnOlderServer is the option of a Device's REST API client that logs a
// warning when the server speaks an older API than the Device: once per
// change of the server's version, as a Device runs for long and the server may
// be replaced under it. A newer server is fine: responses are read tolerantly.
func WarnOlderServer(logger *slog.Logger) apiclient.Option {
	return apiclient.OnServerVersion(newServerVersionWatch(logger).observe)
}

type serverVersionWatch struct {
	logger *slog.Logger

	mu   sync.Mutex
	last contract.SchemaVersion
	seen bool
}

func newServerVersionWatch(logger *slog.Logger) *serverVersionWatch {
	return &serverVersionWatch{logger: logger}
}

func (w *serverVersionWatch) observe(server contract.SchemaVersion) {
	w.mu.Lock()
	changed := !w.seen || w.last != server
	w.last, w.seen = server, true
	w.mu.Unlock()
	if changed && apiv0.SchemaVersion().NewerThan(server) {
		w.logger.Warn("The server speaks an older API than this Device: update the server",
			"server_api", server.String(), "device_api", apiv0.SchemaVersion().String())
	}
}
