package uiutil

import (
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// KeyAction is the Draft command a keydown asks for: the Undo, Redo and Save
// buttons of the Project and Library modes, pressed on the keyboard (ADR 0017,
// ADR 0021).
type KeyAction int

const (
	// KeyNone is a keydown the shortcuts do not act on.
	KeyNone KeyAction = iota
	// KeyUndo is Ctrl+Z (Cmd+Z on macOS).
	KeyUndo
	// KeyRedo is Ctrl+Shift+Z (Cmd+Shift+Z on macOS).
	KeyRedo
	// KeySave is Ctrl+S (Cmd+S on macOS): Save the Draft as a Commit.
	KeySave
)

// ShortcutKey returns the command a keydown of key — the event's code, the
// physical key — with its modifiers asks for: Ctrl+Z (or Cmd+Z on macOS) undo,
// Ctrl+Shift+Z (or Cmd+Shift+Z) redo, Ctrl+S (or Cmd+S) save. key follows the
// keyboard layout, so Ctrl+Z on a Cyrillic layout is "я"; code stays "KeyZ" and
// is what makes the shortcut work on every layout (a QWERTZ or AZERTY board
// keeps its own z at another code, and its key is what is read then). typing is
// a keydown that comes from a field the user types in — those keep the
// browser's own undo, so they ask for nothing.
func ShortcutKey(key, code string, ctrl, meta, shift, typing bool) KeyAction {
	if typing || (!ctrl && !meta) {
		return KeyNone
	}
	switch {
	case strings.EqualFold(key, "z") || code == "KeyZ":
		if shift {
			return KeyRedo
		}
		return KeyUndo
	case !shift && (strings.EqualFold(key, "s") || code == "KeyS"):
		return KeySave
	}
	return KeyNone
}

// KeyEventTyping reports whether a keydown event ev comes from a field the
// user types in: an input, a textarea, a select or editable content. The
// shortcuts leave those alone, so the browser's own undo works in them.
func KeyEventTyping(ev app.Value) bool {
	target := ev.Get("target")
	switch strings.ToUpper(target.Get("tagName").String()) {
	case "INPUT", "TEXTAREA", "SELECT":
		return true
	}
	return target.Get("isContentEditable").Bool()
}

// KeyEventAction returns the command the keyboard event ev asks for: its key,
// its code and modifiers are read from the event, and a keydown in a field the
// user types in asks for none.
func KeyEventAction(ev app.Value) KeyAction {
	return ShortcutKey(
		ev.Get("key").String(),
		ev.Get("code").String(),
		ev.Get("ctrlKey").Bool(),
		ev.Get("metaKey").Bool(),
		ev.Get("shiftKey").Bool(),
		KeyEventTyping(ev),
	)
}
