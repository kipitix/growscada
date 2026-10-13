package devicelink

import (
	"log/slog"

	"github.com/kipitix/growscada/contract"
	"github.com/kipitix/growscada/internal/apiclient"
)

// WarnOlderServer is the option of a Device's REST API client that logs a
// warning when the server speaks an older API than the Device: once per
// change of the server's version, as a Device runs for long and the server may
// be replaced under it. A newer server is fine: responses are read tolerantly.
func WarnOlderServer(logger *slog.Logger) apiclient.Option {
	return apiclient.WarnOlderServer(apiclient.WarnOnChange, func(server, device contract.SchemaVersion) {
		logger.Warn("The server speaks an older API than this Device: update the server",
			"server_api", server.String(), "device_api", device.String())
	})
}
