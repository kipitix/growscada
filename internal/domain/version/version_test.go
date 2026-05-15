package version

import "testing"

func TestNew_ValidValues(t *testing.T) {
	cases := []int{0, 1, 100}
	for _, n := range cases {
		got, err := New(WithNumber(n))
		if err != nil {
			t.Errorf("New(%d): unexpected error: %v", n, err)
		}
		if got.Number() != n {
			t.Errorf("New(%d): got %d", n, got.Number())
		}
	}
}

func TestNew_NegativeReturnsError(t *testing.T) {
	_, err := New(WithNumber(-1))
	if err == nil {
		t.Error("expected error for negative version, got nil")
	}
}

func TestVersion_Constants(t *testing.T) {
	if Initial.Number() != 0 {
		t.Errorf("Initial: expected 0, got %d", Initial.Number())
	}
	if Committed.Number() != 1 {
		t.Errorf("Committed: expected 1, got %d", Committed.Number())
	}
}

func TestVersion_Next(t *testing.T) {
	v, _ := New(WithNumber(5))
	if v.Next().Number() != 6 {
		t.Errorf("Next(): expected 6, got %d", v.Next().Number())
	}
	if Initial.Next() != Committed {
		t.Errorf("Initial.Next() should equal Committed")
	}
}

func TestVersion_IsCommitted(t *testing.T) {
	if Initial.IsCommitted() {
		t.Error("Initial should not be committed")
	}
	if !Committed.IsCommitted() {
		t.Error("Committed should be committed")
	}
	v, _ := New(WithNumber(5))
	if !v.IsCommitted() {
		t.Error("version 5 should be committed")
	}
}

func TestVersion_String(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
	}
	for _, c := range cases {
		got, _ := New(WithNumber(c.n))
		if got.String() != c.want {
			t.Errorf("String(): expected %q, got %q", c.want, got.String())
		}
	}
}
