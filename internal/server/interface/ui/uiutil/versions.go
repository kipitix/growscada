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
// The zero value is empty and ready to use, on the UI goroutine only.
type Versions struct {
	known map[uuid.UUID]int
	// epoch counts the forgets; forgotAll is the epoch of the last Forget,
	// forgotID the epoch of the last ForgetID of an id since then.
	epoch     uint64
	forgotAll uint64
	forgotID  map[uuid.UUID]uint64
}

// Mark is the moment a request was sent, as Versions.Mark took it.
type Mark uint64

// Mark returns the moment of a request sent now; take it before sending.
func (v *Versions) Mark() Mark {
	return Mark(v.epoch)
}

// Observe records version of id, stated by the response to the request sent
// at m, if it is newer than the known one and id was not forgotten since m.
func (v *Versions) Observe(m Mark, id uuid.UUID, version int) {
	if uint64(m) < v.forgotAll || uint64(m) < v.forgotID[id] {
		return
	}
	if version > v.known[id] {
		if v.known == nil {
			v.known = make(map[uuid.UUID]int)
		}
		v.known[id] = version
	}
}

// Of returns the known version of id; 0 if none is known.
func (v *Versions) Of(id uuid.UUID) int {
	return v.known[id]
}

// ForgetID drops the known version of id alone; the next response sets it
// anew. The other aggregates' versions stay: their writes are not in doubt.
func (v *Versions) ForgetID(id uuid.UUID) {
	v.epoch++
	delete(v.known, id)
	if v.forgotID == nil {
		v.forgotID = make(map[uuid.UUID]uint64)
	}
	v.forgotID[id] = v.epoch
}

// Forget drops every known version; the next responses set them anew.
func (v *Versions) Forget() {
	v.epoch++
	clear(v.known)
	clear(v.forgotID) // forgotAll covers them
	v.forgotAll = v.epoch
}
