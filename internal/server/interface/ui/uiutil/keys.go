package uiutil

import (
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// KeyAction is the DraftChange command a keydown asks for: the Undo and Redo
// buttons of the Project and Library modes, pressed on the keyboard (ADR 0017).
type KeyAction int

const (
	// KeyNone is a keydown the shortcuts do not act on.
	KeyNone KeyAction = iota
	// KeyUndo is Ctrl+Z (Cmd+Z on macOS).
	KeyUndo
	// KeyRedo is Ctrl+Shift+Z (Cmd+Shift+Z on macOS).
	KeyRedo
)

// UndoRedoKey returns the command a keydown of key — the event's code, the
// physical key — with its modifiers asks for: Ctrl+Z (or Cmd+Z on macOS) undo,
// Ctrl+Shift+Z (or Cmd+Shift+Z) redo. key follows the keyboard layout, so
// Ctrl+Z on a Cyrillic layout is "я"; code stays "KeyZ" and is what makes the
// shortcut work on every layout (a QWERTZ or AZERTY board keeps its own z at
// another code, and its key is what is read then). typing is a keydown that
// comes from a field the user types in — those keep the browser's own undo, so
// they ask for nothing.
func UndoRedoKey(key, code string, ctrl, meta, shift, typing bool) KeyAction {
	if typing || (!ctrl && !meta) || (!strings.EqualFold(key, "z") && code != "KeyZ") {
		return KeyNone
	}
	if shift {
		return KeyRedo
	}
	return KeyUndo
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
	return UndoRedoKey(
		ev.Get("key").String(),
		ev.Get("code").String(),
		ev.Get("ctrlKey").Bool(),
		ev.Get("metaKey").Bool(),
		ev.Get("shiftKey").Bool(),
		KeyEventTyping(ev),
	)
}
