package devicelink_test

import (
	"encoding/json"
	"testing"

	"github.com/kipitix/growscada/internal/devicelink"
)

func TestParseQuality(t *testing.T) {
	for _, q := range []devicelink.Quality{devicelink.QualityBad, devicelink.QualityUncertain, devicelink.QualityGood} {
		got, err := devicelink.ParseQuality(q.String())
		if err != nil || got != q {
			t.Errorf("ParseQuality(%q): got %v, %v", q, got, err)
		}
	}
	for _, s := range []string{"", "unknown", "invalid", "Good"} {
		if _, err := devicelink.ParseQuality(s); err == nil {
			t.Errorf("ParseQuality(%q): want an error", s)
		}
	}
}

func TestQuality_JSON(t *testing.T) {
	var body struct {
		Quality devicelink.Quality `json:"quality"`
	}
	if err := json.Unmarshal([]byte(`{"quality":"uncertain"}`), &body); err != nil || body.Quality != devicelink.QualityUncertain {
		t.Errorf("unmarshal: got %v, %v", body.Quality, err)
	}
	if err := json.Unmarshal([]byte(`{"quality":"great"}`), &body); err == nil {
		t.Error("unmarshal an unknown quality: want an error")
	}
	if out, err := json.Marshal(body); err != nil || string(out) != `{"quality":"uncertain"}` {
		t.Errorf("marshal: got %s, %v", out, err)
	}
	if _, err := json.Marshal(devicelink.Quality{}); err == nil {
		t.Error("marshal the zero value: want an error")
	}
}

func TestParseTagType(t *testing.T) {
	for _, tt := range []devicelink.TagType{devicelink.TagTypeString, devicelink.TagTypeBoolean, devicelink.TagTypeInteger} {
		got, err := devicelink.ParseTagType(tt.String())
		if err != nil || got != tt {
			t.Errorf("ParseTagType(%q): got %v, %v", tt, got, err)
		}
	}
	for _, s := range []string{"", "unknown", "invalid", "float"} {
		if _, err := devicelink.ParseTagType(s); err == nil {
			t.Errorf("ParseTagType(%q): want an error", s)
		}
	}
}
