package picker

import "github.com/maxence-charriere/go-app/v11/pkg/app"

// installTouch starts the pointer drag of the rows on a phone: the browser's
// own drag never fires from a touch, and go-app exposes no touch events
// (task 82). A build without a browser installs nothing.
func (p *Picker) installTouch() {
	if _, ok := p.source.(OrderSource); !ok {
		return
	}
	p.releaseTouch = installTouchDrag(p.source.Singular(), func(from, to int) {
		p.compoCtx.Dispatch(func(ctx app.Context) { p.move(ctx, from, to) })
	})
}

// uninstallTouch drops the listeners of the phone's drag.
func (p *Picker) uninstallTouch() {
	if p.releaseTouch == nil {
		return
	}
	p.releaseTouch()
	p.releaseTouch = nil
}
