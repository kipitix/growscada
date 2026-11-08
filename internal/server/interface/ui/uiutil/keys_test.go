package uiutil

import (
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestShortcutKey(t *testing.T) {
	tests := []struct {
		name                      string
		key, code                 string
		ctrl, meta, shift, typing bool
		want                      KeyAction
	}{
		{name: "ctrl+z undoes", key: "z", code: "KeyZ", ctrl: true, want: KeyUndo},
		{name: "cmd+z undoes", key: "z", code: "KeyZ", meta: true, want: KeyUndo},
		{name: "ctrl+shift+z redoes", key: "z", code: "KeyZ", ctrl: true, shift: true, want: KeyRedo},
		{name: "cmd+shift+z redoes", key: "z", code: "KeyZ", meta: true, shift: true, want: KeyRedo},
		{name: "ctrl+s saves", key: "s", code: "KeyS", ctrl: true, want: KeySave},
		{name: "cmd+s saves", key: "s", code: "KeyS", meta: true, want: KeySave},
		{name: "the upper case key", key: "Z", code: "KeyZ", ctrl: true, want: KeyUndo},
		// The physical key saves the shortcut on a Cyrillic layout, where
		// Ctrl+Z states the letter "я" and no "z" at all.
		{name: "ctrl+z on a Cyrillic layout", key: "я", code: "KeyZ", ctrl: true, want: KeyUndo},
		{name: "ctrl+shift+z on a Cyrillic layout", key: "Я", code: "KeyZ", ctrl: true, shift: true, want: KeyRedo},
		{name: "ctrl+s on a Cyrillic layout", key: "ы", code: "KeyS", ctrl: true, want: KeySave},
		// And the key saves it on a layout whose z stands at another code
		// (AZERTY, QWERTZ).
		{name: "ctrl+z on AZERTY", key: "z", code: "KeyW", ctrl: true, want: KeyUndo},
		{name: "z alone types", key: "z", code: "KeyZ"},
		{name: "shift+z types a capital", key: "Z", code: "KeyZ", shift: true},
		{name: "ctrl+shift+s is save as, not ours", key: "s", code: "KeyS", ctrl: true, shift: true},
		{name: "another key with ctrl", key: "y", code: "KeyY", ctrl: true},
		{name: "a field keeps the browser's undo", key: "z", code: "KeyZ", ctrl: true, typing: true},
		{name: "a field keeps the browser's redo", key: "z", code: "KeyZ", ctrl: true, shift: true, typing: true},
		{name: "a field keeps the browser's save", key: "s", code: "KeyS", ctrl: true, typing: true},
		{name: "a Cyrillic key of another code", key: "я", code: "KeyQ", ctrl: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShortcutKey(tt.key, tt.code, tt.ctrl, tt.meta, tt.shift, tt.typing); got != tt.want {
				t.Errorf("ShortcutKey(%q, %q, ctrl %t, meta %t, shift %t, typing %t) = %v, want %v",
					tt.key, tt.code, tt.ctrl, tt.meta, tt.shift, tt.typing, got, tt.want)
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
