package player

import (
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/apiclient"
)

// TestLiveSource_OpenOutsideABrowser: the host test engine has no
// EventSource, so Open follows nothing and Close is still safe — before the
// first stream the source holds no JavaScript value at all.
func TestLiveSource_OpenOutsideABrowser(t *testing.T) {
	client := apiclient.New("http://localhost:9090", nil)
	source := NewLiveSource(client.Project(uuid.New()))
	source.Open(app.Context{}, 0, Events{})
	source.Close()
	source.Close()
}

// TestLiveSource_CloseWithoutAStream: a source that was never opened closes
// cleanly.
func TestLiveSource_CloseWithoutAStream(t *testing.T) {
	source := NewLiveSource(nil)
	source.Close()
	source.Close()
}
