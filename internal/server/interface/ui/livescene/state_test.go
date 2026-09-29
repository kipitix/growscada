package livescene

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/interface/ui/uidto"
)

func testWidgetType() WidgetType {
	return WidgetType{
		ID: "wt",
		InputPorts: []uidto.InputPortDTO{
			{Name: "speed", TypeHint: "integer"},
			{Name: "any", TypeHint: ""},
			{Name: "on", TypeHint: "boolean"},
		},
	}
}

func testWidget(bindings ...PortBinding) Widget {
	return Widget{ID: "w", TypeID: "wt", PortBindings: bindings}
}

func TestWidgetInputs(t *testing.T) {
	tags := map[string]Tag{
		"t-speed": {ID: "t-speed", Type: "integer", Value: "42", Quality: QualityGood},
		"t-count": {ID: "t-count", Type: "integer", Value: "7", Quality: QualityGood},
	}
	w := testWidget(
		PortBinding{PortName: "speed", TagID: "t-speed"},
		PortBinding{PortName: "any", TagID: "t-count"},
		PortBinding{PortName: "on", TagID: "t-deleted"},
	)

	inputs := WidgetInputs(w, testWidgetType(), tags)

	if got := inputs["speed"].Value; got != int64(42) {
		t.Errorf("speed = %#v, want 42", got)
	}
	if got := inputs["any"].Value; got != int64(7) {
		t.Errorf("an \"any\" port takes its Tag's type: got %#v, want 7", got)
	}
	if got := inputs["on"].Value; got != false {
		t.Errorf("a port bound to a deleted Tag gets its default: got %#v, want false", got)
	}
}

func TestWidgetInputs_UnboundPortsGetDefaults(t *testing.T) {
	inputs := WidgetInputs(testWidget(), testWidgetType(), nil)

	if got := inputs["speed"].Value; got != int64(0) {
		t.Errorf("speed = %#v, want 0", got)
	}
	if got := inputs["any"].Value; got != nil {
		t.Errorf("any = %#v, want undefined", got)
	}
	if len(inputs) != 3 {
		t.Errorf("expected an input per port, got %v", inputs)
	}
}

func TestWidgetQuality(t *testing.T) {
	good := func(id string) Tag { return Tag{ID: id, Type: "integer", Value: "1", Quality: QualityGood} }
	tags := map[string]Tag{
		"g1": good("g1"), "g2": good("g2"), "g3": good("g3"),
		"u": {ID: "u", Type: "integer", Value: "1", Quality: QualityUncertain},
		"b": {ID: "b", Type: "integer", Value: "1", Quality: QualityBad},
	}
	bind := func(speed, anyTag, on string) Widget {
		return testWidget(
			PortBinding{PortName: "speed", TagID: speed},
			PortBinding{PortName: "any", TagID: anyTag},
			PortBinding{PortName: "on", TagID: on},
		)
	}

	cases := []struct {
		name      string
		w         Widget
		connected bool
		want      Quality
	}{
		{"all good", bind("g1", "g2", "g3"), true, QualityGood},
		{"one uncertain", bind("g1", "u", "g3"), true, QualityUncertain},
		{"bad beats uncertain", bind("b", "u", "g3"), true, QualityBad},
		{"deleted tag is bad", bind("g1", "g2", "gone"), true, QualityBad},
		{"unbound port is bad", testWidget(PortBinding{PortName: "speed", TagID: "g1"}), true, QualityBad},
		{"disconnected is uncertain", bind("g1", "g2", "g3"), false, QualityUncertain},
		{"disconnected keeps bad", bind("b", "g2", "g3"), false, QualityBad},
	}
	for _, c := range cases {
		if got := WidgetQuality(c.w, testWidgetType(), tags, c.connected); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestWidgetQuality_NoPorts(t *testing.T) {
	wt := WidgetType{ID: "wt"}
	if got := WidgetQuality(testWidget(), wt, nil, true); got != QualityGood {
		t.Errorf("connected: got %s, want good", got)
	}
	if got := WidgetQuality(testWidget(), wt, nil, false); got != QualityUncertain {
		t.Errorf("disconnected: got %s, want uncertain", got)
	}
}

func TestWorst(t *testing.T) {
	cases := []struct{ a, b, want Quality }{
		{QualityGood, QualityGood, QualityGood},
		{QualityGood, QualityUncertain, QualityUncertain},
		{QualityBad, QualityUncertain, QualityBad},
		{QualityGood, "weird", QualityBad},
	}
	for _, c := range cases {
		if got := Worst(c.a, c.b); got != c.want {
			t.Errorf("Worst(%s, %s) = %s, want %s", c.a, c.b, got, c.want)
		}
	}
}
