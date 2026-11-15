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
// its data, e.g. a recreated database): the one in conflict on a conflict
// (ForgetID), all of them on a resync of the event stream (Forget), before
// the lists are reread.
//
// A response of a request sent before a forget may still state the old
// version, so each request takes a Mark when it is sent and its versions are
// observed with it: those of a forgotten id are dropped, the reread sets them.
//
// A Mark also orders the requests: it grows with every request and every
// forget, and Versions remembers the mark of the last response that named an
// id (or of the write that touched it). A list requested before that mark may
// miss the id without meaning it was deleted — it was created or written
// after the list was sent (KnownAfter).
//
// The zero value is empty and ready to use, on the UI goroutine only.
type Versions struct {
	known map[uuid.UUID]int
	// seq numbers the marks; a Mark is the seq of the moment it was taken.
	seq uint64
	// forgotAll is the mark of the last Forget, forgotID the mark of the last
	// ForgetID of an id since then.
	forgotAll uint64
	forgotID  map[uuid.UUID]uint64
	// seen is the mark of the last response that named an id, or of the last
	// Touch: an id seen after a list was requested exists, whatever the list
	// says.
	seen map[uuid.UUID]Mark
}

// Mark is the moment a request was sent, as Versions.Mark took it.
type Mark uint64

// Mark returns the moment of a request sent now, later than every mark taken
// before; take it before sending.
func (v *Versions) Mark() Mark {
	v.seq++
	return Mark(v.seq)
}

// Observe records version of id, stated by the response to the request sent
// at m, if it is newer than the known one and id was not forgotten since m.
func (v *Versions) Observe(m Mark, id uuid.UUID, version int) {
	v.Advance(m, id, version)
}

// Advance is Observe reporting whether it recorded version: whether the
// response states a newer state of id than any known, so a copy of that state
// kept along with the version is to be replaced by the response's.
func (v *Versions) Advance(m Mark, id uuid.UUID, version int) bool {
	if v.Forgotten(m, id) {
		return false
	}
	// The response names the id whatever version it states: the id exists as
	// of m, so an older list that misses it does not say it was deleted.
	v.see(m, id)
	if version <= v.known[id] {
		return false
	}
	if v.known == nil {
		v.known = make(map[uuid.UUID]int)
	}
	v.known[id] = version
	return true
}

// Touch records that id exists as of now: a write created it or wrote it. A
// list requested before that moment and missing it does not say it was
// deleted (KnownAfter).
func (v *Versions) Touch(id uuid.UUID) {
	v.see(v.Mark(), id)
}

// KnownAfter reports whether id was known to exist after m was taken: a
// response sent at m and missing it is older than its creation or its last
// write, so it does not say id was deleted.
func (v *Versions) KnownAfter(m Mark, id uuid.UUID) bool {
	return v.seen[id] > m
}

// Forgotten reports whether the version of id was forgotten since m: the
// response to a request sent at m may state the server's old one.
func (v *Versions) Forgotten(m Mark, id uuid.UUID) bool {
	return uint64(m) < v.forgotAll || uint64(m) < v.forgotID[id]
}

// Of returns the known version of id; 0 if none is known.
func (v *Versions) Of(id uuid.UUID) int {
	return v.known[id]
}

// ForgetID drops the known version of id alone; the next response sets it
// anew. The other aggregates' versions stay: their writes are not in doubt.
func (v *Versions) ForgetID(id uuid.UUID) {
	v.seq++
	delete(v.known, id)
	delete(v.seen, id)
	if v.forgotID == nil {
		v.forgotID = make(map[uuid.UUID]uint64)
	}
	v.forgotID[id] = v.seq
}

// Forget drops every known version; the next responses set them anew.
func (v *Versions) Forget() {
	v.seq++
	clear(v.known)
	clear(v.forgotID) // forgotAll covers them
	clear(v.seen)
	v.forgotAll = v.seq
}

// see records that the response sent at m named id.
func (v *Versions) see(m Mark, id uuid.UUID) {
	if v.seen == nil {
		v.seen = make(map[uuid.UUID]Mark)
	}
	if m > v.seen[id] {
		v.seen[id] = m
	}
}
