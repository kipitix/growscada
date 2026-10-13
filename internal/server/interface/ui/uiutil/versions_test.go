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
