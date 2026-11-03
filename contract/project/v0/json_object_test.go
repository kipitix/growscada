package projectv0

import (
	"encoding/json"
	"testing"
)

func TestJSONObject_MarshalZeroIsTheEmptyObject(t *testing.T) {
	var zero JSONObject
	got, err := json.Marshal(zero)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{}" {
		t.Errorf("json.Marshal(zero JSONObject) = %s, want {}", got)
	}
}

func TestJSONObject_MarshalKeepsTheObjectAsItIs(t *testing.T) {
	raw := `{"port": 502, "nested": {"channels": [1, 2]}}`
	got, err := json.Marshal(JSONObject(raw))
	if err != nil {
		t.Fatal(err)
	}
	// encoding/json compacts what a Marshaler returns, but keeps the keys as
	// they were: the object survives a read and a write.
	want := `{"port":502,"nested":{"channels":[1,2]}}`
	if string(got) != want {
		t.Errorf("json.Marshal(JSONObject) = %s, want %s", got, want)
	}
}

func TestDevice_ZeroConfigIsTheEmptyObject(t *testing.T) {
	got, err := json.Marshal(Device{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["config"]) != "{}" {
		t.Errorf("a Device without a config carries config = %s, want {}", fields["config"])
	}
}

func TestJSONObject_UnmarshalTrimsAndKeepsTheObject(t *testing.T) {
	var o JSONObject
	if err := json.Unmarshal([]byte("  {\"b\": 2, \"a\": 1}\n"), &o); err != nil {
		t.Fatal(err)
	}
	if string(o) != `{"b": 2, "a": 1}` {
		t.Errorf("JSONObject = %s, want the object without surrounding space", o)
	}
}

func TestJSONObject_UnmarshalRejectsWhatIsNotAnObject(t *testing.T) {
	for _, raw := range []string{
		``, `null`, `[]`, `[1]`, `"x"`, `42`, `true`, `{`, `{"a":}`,
	} {
		var o JSONObject
		if err := json.Unmarshal([]byte(raw), &o); err == nil {
			t.Errorf("json.Unmarshal(%q) accepted, want an error", raw)
		}
	}
}
