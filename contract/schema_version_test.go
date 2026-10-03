package contract

import (
	"encoding/json"
	"testing"
)

func TestParseSchemaVersion(t *testing.T) {
	valid := map[string]SchemaVersion{
		"0.1":   {0, 1},
		"1.0":   {1, 0},
		"12.34": {12, 34},
	}
	for text, want := range valid {
		got, err := ParseSchemaVersion(text)
		if err != nil || got != want {
			t.Errorf("ParseSchemaVersion(%q) = %v, %v; want %v", text, got, err, want)
		}
		if got.String() != text {
			t.Errorf("%v.String() = %q, want %q", got, got.String(), text)
		}
	}

	for _, text := range []string{"", "1", "1.", ".1", "v1.0", "1.0.0", "01.0", "1.01", "-1.0", "+1.0", "1.a"} {
		if v, err := ParseSchemaVersion(text); err == nil {
			t.Errorf("ParseSchemaVersion(%q) = %v, want error", text, v)
		}
	}
}

func TestSchemaVersionOrder(t *testing.T) {
	tests := []struct {
		a, b      SchemaVersion
		newer     bool
		sameMajor bool
	}{
		{SchemaVersion{0, 2}, SchemaVersion{0, 1}, true, true},
		{SchemaVersion{0, 1}, SchemaVersion{0, 1}, false, true},
		{SchemaVersion{0, 1}, SchemaVersion{0, 2}, false, true},
		{SchemaVersion{1, 0}, SchemaVersion{0, 9}, true, false},
		{SchemaVersion{0, 9}, SchemaVersion{1, 0}, false, false},
	}
	for _, tt := range tests {
		if got := tt.a.NewerThan(tt.b); got != tt.newer {
			t.Errorf("%v.NewerThan(%v) = %v, want %v", tt.a, tt.b, got, tt.newer)
		}
		if got := tt.a.SameMajor(tt.b); got != tt.sameMajor {
			t.Errorf("%v.SameMajor(%v) = %v, want %v", tt.a, tt.b, got, tt.sameMajor)
		}
	}
}

func TestSchemaVersionJSON(t *testing.T) {
	type document struct {
		SchemaVersion SchemaVersion `json:"schema_version"`
	}
	data, err := json.Marshal(document{SchemaVersion{0, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"schema_version":"0.3"}` {
		t.Fatalf("marshal = %s", data)
	}
	var back document
	if err := json.Unmarshal(data, &back); err != nil || back.SchemaVersion != (SchemaVersion{0, 3}) {
		t.Fatalf("unmarshal = %v, %v", back, err)
	}
	if err := json.Unmarshal([]byte(`{"schema_version":"3"}`), &back); err == nil {
		t.Fatal("unmarshal of a bare MAJOR succeeded, want error")
	}
}
