package root

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
	"github.com/kipitix/growscada/internal/versiontest"
)

// mountOnHost lets go-app mount a Root on the host (see uitest).
type mountOnHost struct{ *Root }

func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }

func TestVersionBanner_ShownWhileTheServerSpeaksAnotherVersion(t *testing.T) {
	var serverVersion atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contract.SchemaVersionHeader, serverVersion.Load().(string))
		w.Write([]byte(`{"tags":[]}`))
	}))
	defer srv.Close()
	r := NewRoot(srv.URL)
	e := uitest.Load(t, mountOnHost{r})
	// request makes a request through the UI's client, as any mode does.
	request := func(version string) {
		t.Helper()
		serverVersion.Store(version)
		if _, err := r.client.ListTags(context.Background()); err != nil {
			t.Fatal(err)
		}
		e.ConsumeAll()
	}

	if r.versionMismatch() {
		t.Fatal("banner shown before any response")
	}

	older := versiontest.Older(apiv0.SchemaVersion())
	request(older.String())
	if !r.versionMismatch() {
		t.Fatal("no banner for an older server")
	}
	if err := app.Match(app.Span().Text(versionBannerText(older)), r.renderVersionBanner(), 0); err != nil {
		t.Errorf("banner: %v", err)
	}
	if html := app.HTMLString(r.renderVersionBanner()); !strings.Contains(html, "data-version-banner") {
		t.Errorf("the banner lacks its data-version-banner attribute: %s", html)
	}
	text := versionBannerText(r.serverVersion)
	if !strings.Contains(text, "API "+older.String()) || !strings.Contains(text, apiv0.SchemaVersion().String()) ||
		!strings.Contains(text, "Reload") {
		t.Errorf("banner %q: want both versions and what to do", text)
	}

	newer := versiontest.Newer(apiv0.SchemaVersion())
	request(newer.String())
	if !r.versionMismatch() || r.serverVersion != newer {
		t.Errorf("a newer server: mismatch %v, version %v", r.versionMismatch(), r.serverVersion)
	}

	request(apiv0.SchemaVersion().String())
	if r.versionMismatch() {
		t.Error("the banner stays after the versions match again")
	}
	if err := app.Match(app.Div().Style("display", "none"), r.renderVersionBanner()); err != nil {
		t.Errorf("hidden banner: %v", err)
	}

	request("") // a response without the header changes nothing
	if r.versionMismatch() || !r.serverVersionKnown {
		t.Errorf("a response without the version: mismatch %v, known %v", r.versionMismatch(), r.serverVersionKnown)
	}
}
