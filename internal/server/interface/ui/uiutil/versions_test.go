package uiutil

import (
	"testing"

	"github.com/google/uuid"
)

func TestVersions_KeepTheNewest(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	v := Versions{}

	if got := v.Of(a); got != 0 {
		t.Errorf("unknown id: got %d, want 0", got)
	}
	v.Observe(a, 3)
	v.Observe(a, 5)
	v.Observe(a, 4) // a list read before the write that made 5
	v.Observe(b, 1)

	if got := v.Of(a); got != 5 {
		t.Errorf("a: got %d, want the newest 5", got)
	}
	if got := v.Of(b); got != 1 {
		t.Errorf("b: got %d, want 1", got)
	}
}

func TestVersions_ForgetLetsAVersionGoBack(t *testing.T) {
	id := uuid.New()
	v := Versions{}
	v.Observe(id, 15)

	v.Forget() // e.g. the server's database was recreated
	v.Observe(id, 1)

	if got := v.Of(id); got != 1 {
		t.Errorf("got %d, want the server's 1", got)
	}
}

func TestVersions_ForgetIDKeepsTheOthers(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	v := Versions{}
	v.Observe(a, 15)
	v.Observe(b, 7)

	v.ForgetID(a) // a conflict on a
	v.Observe(a, 1)

	if got := v.Of(a); got != 1 {
		t.Errorf("a: got %d, want the server's 1", got)
	}
	if got := v.Of(b); got != 7 {
		t.Errorf("b: got %d, want the untouched 7", got)
	}
}
