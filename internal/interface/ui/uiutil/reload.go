package uiutil

// Reloader coalesces reloads of one server-backed list. While a load is in
// flight, further requests are not started in parallel (their responses could
// arrive out of order); instead exactly one more load is queued to run after
// the current one. A burst of server events therefore costs at most one extra
// request.
//
// Use it on the UI goroutine only:
//
//	if !r.Start() { return }       // a load is already running; it will rerun
//	... fetch asynchronously, then on the UI goroutine:
//	if r.Done() { reload again }
type Reloader struct {
	loading bool
	pending bool
}

// Start reports whether the caller should start a load now. If a load is
// already in flight it queues a rerun and returns false.
func (r *Reloader) Start() bool {
	if r.loading {
		r.pending = true
		return false
	}
	r.loading = true
	return true
}

// Done marks the in-flight load finished and reports whether a rerun was
// requested meanwhile; the caller should then load again.
func (r *Reloader) Done() bool {
	r.loading = false
	again := r.pending
	r.pending = false
	return again
}

// MergeField decides what an editor field shows after fresh server data
// arrives: if the field still holds the previous server value (the user has
// not edited it), it follows the server; otherwise the user's unsaved edit is kept.
func MergeField[T comparable](local, oldServer, newServer T) T {
	if local == oldServer {
		return newServer
	}
	return local
}
