package uiutil

import "testing"

func TestReloader_CoalescesRequestsDuringLoad(t *testing.T) {
	var r Reloader

	if !r.Start() {
		t.Fatal("first Start must start a load")
	}
	if r.Start() || r.Start() {
		t.Fatal("Start during a load must not start another one")
	}
	if !r.Done() {
		t.Fatal("Done must request exactly one rerun after queued requests")
	}
	if !r.Start() {
		t.Fatal("the rerun must be allowed to start")
	}
	if r.Done() {
		t.Fatal("Done without queued requests must not request a rerun")
	}
	if !r.Start() {
		t.Fatal("Start after Done must start a new load")
	}
}

func TestMergeField(t *testing.T) {
	tests := []struct {
		name                       string
		local, oldServer, newValue string
		want                       string
	}{
		{"unedited field follows the server", "a", "a", "b", "b"},
		{"edited field keeps the local value", "mine", "a", "b", "mine"},
		{"edit equal to the new server value stays", "b", "a", "b", "b"},
		{"unchanged server keeps the field", "a", "a", "a", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MergeField(tt.local, tt.oldServer, tt.newValue); got != tt.want {
				t.Errorf("MergeField(%q, %q, %q) = %q, want %q", tt.local, tt.oldServer, tt.newValue, got, tt.want)
			}
		})
	}
}
