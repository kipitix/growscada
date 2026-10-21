package uiutil

import (
	"math"
	"strconv"
	"strings"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	widgetscriptv0 "github.com/kipitix/growscada/contract/widgetscript/v0"
)

func TestNewInput(t *testing.T) {
	cases := []struct {
		text, typ string
		value     any
		wantText  string
		ok        bool
	}{
		{"42", "integer", int64(42), "42", true},
		{"+007", "integer", int64(7), "7", true},
		{"", "integer", nil, "", false},
		{"abc", "integer", nil, "", false},
		{"9223372036854775808", "integer", nil, "", false},
		{"true", "boolean", true, "true", true},
		{"false", "boolean", false, "false", true},
		{"yes", "boolean", nil, "", false},
		{"", "boolean", nil, "", false},
		{"hi", "string", "hi", "hi", true},
		{"", "string", "", "", true},
		{"42", "", "42", "42", true},
	}
	for _, c := range cases {
		in, ok := NewInput(c.text, c.typ)
		if ok != c.ok {
			t.Errorf("NewInput(%q, %q) ok = %v, want %v", c.text, c.typ, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if in.Value != c.value || in.Text == nil || *in.Text != c.wantText {
			t.Errorf("NewInput(%q, %q) = %#v, %v; want %#v, %q", c.text, c.typ, in.Value, textOf(in), c.value, c.wantText)
		}
	}
}

// TestNewInput_IntegerBeyondSafeRange: text keeps an Integer exactly, value
// stays the int64 it was (rounded only by the script's JSON.parse).
func TestNewInput_IntegerBeyondSafeRange(t *testing.T) {
	for _, n := range []int64{9007199254740991, 9007199254740993, -9007199254740993, math.MaxInt64, math.MinInt64} {
		text := strconv.FormatInt(n, 10)
		in, ok := NewInput(text, "integer")
		if !ok || in.Value != n || in.Text == nil || *in.Text != text {
			t.Errorf("NewInput(%q) = %#v, %v, %v", text, in.Value, textOf(in), ok)
		}
		want := `{"n":{"value":` + text + `,"text":"` + text + `"}}`
		if got := InputsJSON(widgetscriptv0.Inputs{"n": in}); got != want {
			t.Errorf("InputsJSON = %s, want %s", got, want)
		}
	}
}

func TestNoValueInput(t *testing.T) {
	for hint, want := range map[string]any{"integer": int64(0), "boolean": false, "string": "", "": nil} {
		in := NoValueInput(hint)
		if in.Value != want || in.Text != nil {
			t.Errorf("NoValueInput(%q) = %#v, %v; want %#v, no text", hint, in.Value, textOf(in), want)
		}
	}
}

func TestPreviewInputs(t *testing.T) {
	ports := []apiv0.InputPort{
		{Name: "count", TypeHint: "integer"},
		{Name: "bad", TypeHint: "integer"},
		{Name: "flag", TypeHint: "boolean"},
		{Name: "label", TypeHint: "string"},
		{Name: "anything", TypeHint: ""},
		{Name: "unset", TypeHint: "integer"},
	}
	got := InputsJSON(PreviewInputs(map[string]string{
		"count":    "+007",
		"bad":      "abc",
		"flag":     "yes",
		"label":    "",
		"anything": "x",
	}, ports))
	want := `{"anything":{"value":"x","text":"x"},"bad":{"value":0},"count":{"value":7,"text":"7"},` +
		`"flag":{"value":false},"label":{"value":""},"unset":{"value":0}}`
	if got != want {
		t.Errorf("PreviewInputs = %s, want %s", got, want)
	}
}

func textOf(in widgetscriptv0.Input) any {
	if in.Text == nil {
		return nil
	}
	return *in.Text
}

func TestInputsJSON(t *testing.T) {
	text := "</script>"
	got := InputsJSON(widgetscriptv0.Inputs{
		"speed": {Value: int64(7)},
		"label": {},
		"text":  {Value: text, Text: &text},
	})
	want := `{"label":{},"speed":{"value":7},"text":{"value":"\u003c/script\u003e","text":"\u003c/script\u003e"}}`
	if got != want {
		t.Errorf("InputsJSON = %s, want %s", got, want)
	}
	if got := InputsJSON(nil); got != "{}" {
		t.Errorf("InputsJSON(nil) = %s, want {}", got)
	}
}

func TestBuildSrcdoc_RendersOnceWithInputs(t *testing.T) {
	ports := []apiv0.InputPort{{Name: "value", TypeHint: "integer"}}
	doc := BuildSrcdoc("<div></div>", "function render(inputs){}", PreviewInputs(map[string]string{"value": "5"}, ports), ports, "#fff")
	if !strings.Contains(doc, `render({"value":{"value":5,"text":"5"}})`) {
		t.Errorf("expected a render call with the inputs, got %s", doc)
	}
	if strings.Contains(doc, `addEventListener("message"`) {
		t.Error("a preview must not listen for messages")
	}
}

func TestBuildSrcdoc_EscapesScriptClose(t *testing.T) {
	doc := BuildSrcdoc("", `var s="</script><b>x</b>";`, nil, nil, "#fff")
	if strings.Contains(doc, `"</script><b>`) {
		t.Errorf("user script closed the script element: %s", doc)
	}
}

func TestBuildLiveSrcdoc_ListensAndAnnouncesReady(t *testing.T) {
	doc := BuildLiveSrcdoc("<div></div>", "function render(inputs){}")
	for _, want := range []string{
		// Drawn right on the Scene's background.
		`html,body{background:transparent;`,
		`if(e.source!==window.parent)return;`,
		`m.type!=="inputs"`,
		`window.parent.postMessage({type:"ready"},"*");`,
		`connect-src 'none'`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("expected live srcdoc to contain %q, got %s", want, doc)
		}
	}
	if strings.Contains(doc, "try{render({") {
		t.Error("a live widget must wait for its inputs message instead of rendering inline")
	}
}

func TestWidgetNamePlaceholder_EscapesNameAndColor(t *testing.T) {
	got := WidgetNamePlaceholder(`<b>pump</b>`, `red"><script>`)
	if strings.Contains(got, "<b>") || strings.Contains(got, "<script>") {
		t.Errorf("name and colour must be escaped, got %s", got)
	}
	if !strings.Contains(got, "&lt;b&gt;pump&lt;/b&gt;") {
		t.Errorf("the escaped name must be shown, got %s", got)
	}
}
