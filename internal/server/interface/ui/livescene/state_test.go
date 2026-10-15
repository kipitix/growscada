package livescene

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// id is a stable ID named for readability.
func id(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

func tag(name, tagType, value, quality string) apiv0.TagResponse {
	return apiv0.TagResponse{ID: id(name), Type: tagType, Value: value, Quality: quality}
}

func tagsOf(tags ...apiv0.TagResponse) map[uuid.UUID]apiv0.TagResponse {
	byID := make(map[uuid.UUID]apiv0.TagResponse, len(tags))
	for _, t := range tags {
		byID[t.ID] = t
	}
	return byID
}

func bind(port, tagName string) apiv0.PortBinding {
	return apiv0.PortBinding{PortName: port, TagID: id(tagName)}
}

func testWidgetType() apiv0.WidgetTypeResponse {
	return apiv0.WidgetTypeResponse{
		ID: id("wt"),
		InputPorts: []apiv0.InputPort{
			{Name: "speed", TypeHint: "integer"},
			{Name: "any", TypeHint: ""},
			{Name: "on", TypeHint: "boolean"},
		},
	}
}

func testWidget(bindings ...apiv0.PortBinding) apiv0.WidgetResponse {
	return apiv0.WidgetResponse{ID: id("w"), TypeID: id("wt"), PortBindings: bindings}
}

func TestWidgetInputs(t *testing.T) {
	tags := tagsOf(tag("t-speed", "integer", "42", "good"), tag("t-count", "integer", "7", "good"))
	w := testWidget(bind("speed", "t-speed"), bind("any", "t-count"), bind("on", "t-deleted"))

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
	tags := tagsOf(
		tag("g1", "integer", "1", "good"), tag("g2", "integer", "1", "good"), tag("g3", "integer", "1", "good"),
		tag("u", "integer", "1", "uncertain"),
		tag("b", "integer", "1", "bad"),
	)
	bindAll := func(speed, anyTag, on string) apiv0.WidgetResponse {
		return testWidget(bind("speed", speed), bind("any", anyTag), bind("on", on))
	}

	cases := []struct {
		name      string
		w         apiv0.WidgetResponse
		connected bool
		want      Quality
	}{
		{"all good", bindAll("g1", "g2", "g3"), true, QualityGood},
		{"one uncertain", bindAll("g1", "u", "g3"), true, QualityUncertain},
		{"bad beats uncertain", bindAll("b", "u", "g3"), true, QualityBad},
		{"deleted tag is bad", bindAll("g1", "g2", "gone"), true, QualityBad},
		{"unbound port is bad", testWidget(bind("speed", "g1")), true, QualityBad},
		{"disconnected is uncertain", bindAll("g1", "g2", "g3"), false, QualityUncertain},
		{"disconnected keeps bad", bindAll("b", "g2", "g3"), false, QualityBad},
	}
	for _, c := range cases {
		if got := WidgetQuality(c.w, testWidgetType(), tags, c.connected); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestWidgetQuality_NoPorts(t *testing.T) {
	wt := apiv0.WidgetTypeResponse{ID: id("wt")}
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

// findIn finds a Tag in tags, as tagstate.Tags.Find does.
func findIn(tags map[uuid.UUID]apiv0.TagResponse) func(uuid.UUID) (apiv0.TagResponse, bool) {
	return func(id uuid.UUID) (apiv0.TagResponse, bool) {
		t, ok := tags[id]
		return t, ok
	}
}

func TestSceneTags_KeepsOnlyBoundTags(t *testing.T) {
	tags := tagsOf(tag("t-speed", "integer", "42", "good"), tag("t-other", "integer", "1", "good"))
	widgets := []apiv0.WidgetResponse{
		testWidget(bind("speed", "t-speed")),
		testWidget(bind("on", "t-deleted")),
	}

	got := SceneTags(widgets, findIn(tags))

	if len(got) != 1 || got[id("t-speed")].Value != "42" {
		t.Errorf("want only the existing bound Tag, got %+v", got)
	}

	// A change of an unbound Tag must leave the result equal, so the View is
	// not re-rendered.
	tags[id("t-other")] = tag("t-other", "integer", "2", "good")
	if again := SceneTags(widgets, findIn(tags)); !reflect.DeepEqual(again, got) {
		t.Errorf("an unbound Tag changed the result: %+v vs %+v", again, got)
	}
}
