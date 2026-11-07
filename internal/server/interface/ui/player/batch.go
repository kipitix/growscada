package player

import "time"

// updateBatcher coalesces the redraws a Player asks for: the first request
// schedules one redraw after interval and further requests join it, so a burst
// of Journal entries costs one Update (ADR 0020).
type updateBatcher struct {
	interval time.Duration
	// after runs f after d on the UI goroutine. It is replaceable so a test
	// need not wait for a timer.
	after   func(time.Duration, func())
	request func()

	scheduled bool
}

// Request asks for a redraw; requests that arrive before the scheduled one
// runs are folded into it.
func (b *updateBatcher) Request() {
	if b.scheduled {
		return
	}
	b.scheduled = true
	b.after(b.interval, func() {
		b.scheduled = false
		if b.request != nil {
			b.request()
		}
	})
}
