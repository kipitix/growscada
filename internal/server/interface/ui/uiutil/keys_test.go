package uiutil

import (
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestUndoRedoKey(t *testing.T) {
	tests := []struct {
		name                      string
		key                       string
		ctrl, meta, shift, typing bool
		want                      KeyAction
	}{
		{name: "ctrl+z undoes", key: "z", ctrl: true, want: KeyUndo},
		{name: "cmd+z undoes", key: "z", meta: true, want: KeyUndo},
		{name: "ctrl+shift+z redoes", key: "z", ctrl: true, shift: true, want: KeyRedo},
		{name: "cmd+shift+z redoes", key: "z", meta: true, shift: true, want: KeyRedo},
		{name: "the upper case key", key: "Z", ctrl: true, want: KeyUndo},
		{name: "z alone types", key: "z"},
		{name: "shift+z types a capital", key: "Z", shift: true},
		{name: "another key with ctrl", key: "y", ctrl: true},
		{name: "a field keeps the browser's undo", key: "z", ctrl: true, typing: true},
		{name: "a field keeps the browser's redo", key: "z", ctrl: true, shift: true, typing: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UndoRedoKey(tt.key, tt.ctrl, tt.meta, tt.shift, tt.typing); got != tt.want {
				t.Errorf("UndoRedoKey(%q, ctrl %t, meta %t, shift %t, typing %t) = %v, want %v",
					tt.key, tt.ctrl, tt.meta, tt.shift, tt.typing, got, tt.want)
			}
		})
	}
}

// TestKeyEventAction_OnTheHost: the host has no DOM, so an event reads as a
// keydown without a key and asks for nothing.
func TestKeyEventAction_OnTheHost(t *testing.T) {
	if got := KeyEventAction(app.ValueOf(nil)); got != KeyNone {
		t.Errorf("KeyEventAction of an empty event = %v, want none", got)
	}
}
