package uiutil

import "github.com/google/uuid"

// Versions is the one place a mode keeps the version of each aggregate it
// writes with an optimistic lock. Every response that carries a version — a
// list, the result of a write — is observed here, and every write takes the
// version from here.
//
// A version never goes back: a response that raced with a later write (a
// list read before it) must not undo the version that write recorded. The
// versions are forgotten instead when the server's may have gone back (it lost
// its data, e.g. a recreated database): on a conflict and on a resync of the
// event stream, before the lists are reread.
type Versions map[uuid.UUID]int

// Observe records version of id if it is newer than the known one.
func (v Versions) Observe(id uuid.UUID, version int) {
	if version > v[id] {
		v[id] = version
	}
}

// Of returns the known version of id; 0 if none is known.
func (v Versions) Of(id uuid.UUID) int {
	return v[id]
}

// Forget drops every known version; the next responses set them anew.
func (v Versions) Forget() {
	clear(v)
}
