package devicesim

import (
	"testing"

	"github.com/kipitix/growscada/internal/brunotest"
)

// requestBodies maps the pattern of every route that takes a body to the type
// its handler decodes; a POST/PUT/PATCH route without a body maps to nil.
var requestBodies = map[string]any{
	"POST /control/device/start":               nil,
	"POST /control/device/stop":                nil,
	"POST /control/device/tags/{name}/pattern": PatternSpec{},
	"POST /control/device/tags/{name}/quality": qualityBody{},
}

// TestBrunoCollection checks that the device_simulator Bruno collection keeps
// up with the control API: every request hits a route, every route has a
// request and every body decodes as its handler decodes it.
func TestBrunoCollection(t *testing.T) {
	var patterns []string
	for _, rt := range (&Device{}).routes() {
		patterns = append(patterns, rt.pattern)
	}
	brunotest.Check(t, brunotest.Collection{
		Dir:      brunotest.CollectionDir("device_simulator"),
		Patterns: patterns,
		Bodies:   requestBodies,
		Decode:   decodeStrict,
	})
}
