package root

import (
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
)

// modeProbe stands in for a mode and counts its mounts (on the host go-app
// mounts through OnPreRender) and dismounts.
type modeProbe struct {
	app.Compo
	mounts, dismounts int
}

func (p *modeProbe) OnPreRender(app.Context) { p.mounts++ }
func (p *modeProbe) OnDismount()             { p.dismounts++ }
func (p *modeProbe) Render() app.UI          { return app.Div() }

// TestModes_MountedOnceOpenedAndKeptWhileHidden: a mode is mounted when first
// opened, never before, and stays mounted while another mode is shown, so
// it keeps what it loaded (shown while the server is down) and its edits.
func TestModes_MountedOnceOpenedAndKeptWhileHidden(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	probes := map[Mode]*modeProbe{}
	for i := range r.modes {
		p := &modeProbe{}
		probes[r.modes[i].mode] = p
		r.modes[i].ui = p
	}
	e := uitest.Load(t, mountOnHost{r})
	show := func(mode Mode) {
		t.Helper()
		r.compoCtx.Dispatch(func(ctx app.Context) { r.SetMode(mode) })
		e.ConsumeAll()
		if r.currentMode != mode {
			t.Fatalf("shown %q, want %q", r.currentMode, mode)
		}
	}
	// want checks each mode's mounts; no mode is ever dismounted.
	want := func(mounts map[Mode]int) {
		t.Helper()
		for mode, p := range probes {
			if p.mounts != mounts[mode] || p.dismounts != 0 {
				t.Errorf("%s: %d mounts, %d dismounts; want %d mounts, none dismounted",
					mode, p.mounts, p.dismounts, mounts[mode])
			}
		}
	}

	want(map[Mode]int{ModeLibrary: 1}) // the default mode, nothing else

	// Opening a mode in a slot before an opened one does not remount that one.
	show(ModeOperation)
	show(ModeProject)
	want(map[Mode]int{ModeLibrary: 1, ModeProject: 1, ModeOperation: 1})

	// Back and forth: the modes are shown again, not mounted anew.
	show(ModeLibrary)
	show(ModeOperation)
	show(ModeProject)
	show(ModeHistory)
	show(ModeOperation)
	want(map[Mode]int{ModeLibrary: 1, ModeProject: 1, ModeOperation: 1, ModeHistory: 1})
}
