package uiutil

import (
	"testing"

	"github.com/google/uuid"
)

func TestVersions_KeepTheNewest(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	var v Versions

	if got := v.Of(a); got != 0 {
		t.Errorf("unknown id: got %d, want 0", got)
	}
	m := v.Mark()
	v.Observe(m, a, 3)
	v.Observe(m, a, 5)
	v.Observe(m, a, 4) // a list read before the write that made 5
	v.Observe(m, b, 1)

	if got := v.Of(a); got != 5 {
		t.Errorf("a: got %d, want the newest 5", got)
	}
	if got := v.Of(b); got != 1 {
		t.Errorf("b: got %d, want 1", got)
	}
}

func TestVersions_ForgetLetsAVersionGoBack(t *testing.T) {
	id := uuid.New()
	var v Versions
	v.Observe(v.Mark(), id, 15)

	v.Forget() // e.g. the server's database was recreated
	v.Observe(v.Mark(), id, 1)

	if got := v.Of(id); got != 1 {
		t.Errorf("got %d, want the server's 1", got)
	}
}

func TestVersions_ForgetIDKeepsTheOthers(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	var v Versions
	v.Observe(v.Mark(), a, 15)
	v.Observe(v.Mark(), b, 7)

	v.ForgetID(a) // a conflict on a
	v.Observe(v.Mark(), a, 1)

	if got := v.Of(a); got != 1 {
		t.Errorf("a: got %d, want the server's 1", got)
	}
	if got := v.Of(b); got != 7 {
		t.Errorf("b: got %d, want the untouched 7", got)
	}
}

func TestVersions_ResponseSentBeforeAForgetIsDropped(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	var v Versions

	inFlight := v.Mark() // a list read from the server before it lost its data
	v.Forget()
	reread := v.Mark()
	v.Observe(inFlight, a, 12) // its old response arrives after the forget
	v.Observe(reread, a, 1)

	if got := v.Of(a); got != 1 {
		t.Errorf("after Forget: got %d, want the reread 1", got)
	}

	inFlight = v.Mark()
	v.ForgetID(a) // a conflict on a
	v.Observe(inFlight, a, 9)
	v.Observe(inFlight, b, 4) // b was not forgotten

	if got := v.Of(a); got != 0 {
		t.Errorf("after ForgetID: got %d, want 0 until the reread", got)
	}
	if got := v.Of(b); got != 4 {
		t.Errorf("b: got %d, want 4", got)
	}
}

// A mark orders the requests: the later a request is sent, the greater its
// mark, whatever was forgotten between them.
func TestVersions_MarksGrowWithEachRequest(t *testing.T) {
	id := uuid.New()
	var v Versions

	first := v.Mark()
	second := v.Mark()
	v.Observe(first, id, 1)
	v.ForgetID(id)
	third := v.Mark()

	if second <= first {
		t.Errorf("marks: %d then %d, want the second greater", first, second)
	}
	if third <= second {
		t.Errorf("marks: %d then a forget then %d, want it greater", second, third)
	}
}

// KnownAfter: an id seen after the list was requested exists, whatever the
// list says; a later list that names it takes the protection away.
func TestVersions_KnownAfter(t *testing.T) {
	id := uuid.New()
	var v Versions
	stale := v.Mark() // a list sent before the id was created

	v.Touch(id)
	if !v.KnownAfter(stale, id) {
		t.Error("after Touch: not known after the stale list")
	}
	if v.KnownAfter(v.Mark(), id) {
		t.Error("a list requested after the Touch: known after it")
	}

	later := v.Mark()
	v.Advance(later, id, 3) // a list naming it
	if !v.KnownAfter(stale, id) {
		t.Error("a list naming the id: not known after an older one")
	}

	// A response that states no newer version still names the id.
	again := v.Mark()
	v.Advance(again, id, 3)
	if !v.KnownAfter(later, id) {
		t.Error("the response named the id: not known after the earlier list")
	}

	v.ForgetID(id)
	if v.KnownAfter(again, id) {
		t.Error("after ForgetID: the id is still protected")
	}
}

// A forgotten id is not protected by a response sent before the forget: it
// may have been lost with the server's old data.
func TestVersions_KnownAfter_DropsAForgottenIDsOldResponse(t *testing.T) {
	id := uuid.New()
	var v Versions
	inFlight := v.Mark()
	v.Forget()
	v.Advance(inFlight, id, 7) // the old response arrives after the forget

	if v.KnownAfter(inFlight, id) {
		t.Error("a response sent before a Forget: the id is protected by it")
	}
}

func TestVersions_AdvanceReportsANewerState(t *testing.T) {
	id := uuid.New()
	var v Versions
	m := v.Mark()

	if !v.Advance(m, id, 2) {
		t.Error("first version: not advanced")
	}
	if v.Advance(m, id, 2) {
		t.Error("the same version: advanced")
	}
	if v.Advance(m, id, 1) {
		t.Error("an older version: advanced")
	}

	v.ForgetID(id)
	if v.Advance(m, id, 9) {
		t.Error("a response sent before the forget: advanced")
	}
	if !v.Advance(v.Mark(), id, 1) {
		t.Error("after the forget: not advanced")
	}
}
