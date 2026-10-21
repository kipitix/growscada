package library

import (
	"slices"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestMergeEditFields_KeepsEditedFieldsAndFollowsServerForOthers(t *testing.T) {
	old := editFieldsOf(apiv0.WidgetTypeResponse{Name: "Gauge", HtmlTemplate: "<p>a</p>", Script: "a()", ScriptLanguage: "javascript"})
	current := editFieldsOf(apiv0.WidgetTypeResponse{Name: "Gauge renamed", HtmlTemplate: "<p>b</p>", Script: "b()", ScriptLanguage: "javascript"})
	local := old
	local.name = "Mine" // unsaved edit

	got := mergeEditFields(local, old, current)

	if got.name != "Mine" {
		t.Errorf("edited name: got %q, want the unsaved %q", got.name, "Mine")
	}
	if got.html != "<p>b</p>" || got.script != "b()" {
		t.Errorf("unedited fields must follow the server, got html=%q script=%q", got.html, got.script)
	}
}

func TestMergeEditFields_PortsMergedAsOneList(t *testing.T) {
	old := editFieldsOf(apiv0.WidgetTypeResponse{InputPorts: []apiv0.InputPort{{Name: "value"}}})
	current := editFieldsOf(apiv0.WidgetTypeResponse{InputPorts: []apiv0.InputPort{{Name: "value"}, {Name: "max"}}})

	if got := mergeEditFields(old, old, current); !slices.Equal(got.ports, current.ports) {
		t.Errorf("unedited ports: got %+v, want the server's %+v", got.ports, current.ports)
	}

	local := old
	local.ports = []apiv0.InputPort{{Name: "value"}, {Name: "on"}} // unsaved edit
	if got := mergeEditFields(local, old, current); !slices.Equal(got.ports, local.ports) {
		t.Errorf("edited ports: got %+v, want the unsaved %+v", got.ports, local.ports)
	}
}

func TestEditFieldsOf_CopiesPorts(t *testing.T) {
	it := apiv0.WidgetTypeResponse{InputPorts: []apiv0.InputPort{{Name: "value"}}}
	f := editFieldsOf(it)

	f.ports[0].Name = "changed" // the editor changes its ports

	if it.InputPorts[0].Name != "value" {
		t.Errorf("the WidgetType follows the editor: %+v", it.InputPorts)
	}
}
