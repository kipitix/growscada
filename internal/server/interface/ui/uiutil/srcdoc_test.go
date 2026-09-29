package uiutil

import (
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/server/interface/ui/uidto"
)

func TestNewInput(t *testing.T) {
	cases := []struct {
		raw, typ string
		want     any
	}{
		{"42", "integer", int64(42)},
		{"", "integer", int64(0)},
		{"abc", "integer", int64(0)},
		{"true", "boolean", true},
		{"false", "boolean", false},
		{"", "boolean", false},
		{"hi", "string", "hi"},
		{"", "string", ""},
		{"42", "", "42"},
		{"", "", nil},
	}
	for _, c := range cases {
		if got := NewInput(c.raw, c.typ).Value; got != c.want {
			t.Errorf("NewInput(%q, %q) = %#v, want %#v", c.raw, c.typ, got, c.want)
		}
	}
}

func TestInputsJSON(t *testing.T) {
	got := InputsJSON(Inputs{
		"speed": {Value: int64(7)},
		"label": {},
		"text":  {Value: "</script>"},
	})
	want := `{"label":{},"speed":{"value":7},"text":{"value":"\u003c/script\u003e"}}`
	if got != want {
		t.Errorf("InputsJSON = %s, want %s", got, want)
	}
	if got := InputsJSON(nil); got != "{}" {
		t.Errorf("InputsJSON(nil) = %s, want {}", got)
	}
}

func TestBuildSrcdoc_RendersOnceWithInputs(t *testing.T) {
	ports := []uidto.InputPortDTO{{Name: "value", TypeHint: "integer"}}
	doc := BuildSrcdoc("<div></div>", "function render(inputs){}", PreviewInputs(map[string]string{"value": "5"}, ports), ports, "#fff")
	if !strings.Contains(doc, `render({"value":{"value":5}})`) {
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
	doc := BuildLiveSrcdoc("<div></div>", "function render(inputs){}", "#fff")
	for _, want := range []string{
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
