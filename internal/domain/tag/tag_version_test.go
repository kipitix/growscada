package tag

import "testing"

func TestNewTagVersion_ValidValues(t *testing.T) {
	cases := []int{0, 1, 100}
	for _, v := range cases {
		got, err := NewTagVersion(TagVersionWithNumber(v))
		if err != nil {
			t.Errorf("NewTagVersion(%d): unexpected error: %v", v, err)
		}
		if got.Number() != v {
			t.Errorf("NewTagVersion(%d): got %d", v, got.Number())
		}
	}
}

func TestNewTagVersion_NegativeReturnsError(t *testing.T) {
	_, err := NewTagVersion(TagVersionWithNumber(-1))
	if err == nil {
		t.Error("expected error for negative version, got nil")
	}
}

func TestTagVersion_Constants(t *testing.T) {
	if TagVersionInitial.Number() != 0 {
		t.Errorf("TagVersionInitial: expected 0, got %d", TagVersionInitial.Number())
	}
	if TagVersionCommitted.Number() != 1 {
		t.Errorf("TagVersionCommitted: expected 1, got %d", TagVersionCommitted.Number())
	}
}

func TestTagVersion_Next(t *testing.T) {
	v, _ := NewTagVersion(TagVersionWithNumber(5))
	if v.Next().Number() != 6 {
		t.Errorf("Next(): expected 6, got %d", v.Next().Number())
	}
	if TagVersionInitial.Next() != TagVersionCommitted {
		t.Errorf("TagVersionInitial.Next() should equal TagVersionCommitted")
	}
}

func TestTagVersion_String(t *testing.T) {
	cases := []struct {
		v    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
	}
	for _, c := range cases {
		got, _ := NewTagVersion(TagVersionWithNumber(c.v))
		if got.String() != c.want {
			t.Errorf("String(): expected %q, got %q", c.want, got.String())
		}
	}
}
