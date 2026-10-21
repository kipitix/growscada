package library

import (
	"slices"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Editor fields ─────────────────────────────────────────────────────────────

// editFields is the WidgetType editor's state that comes from the server.
// Preview input values and the add-port form are editor-only state.
type editFields struct {
	name, html, script, scriptLang string
	ports                          []apiv0.InputPort
}

// editFieldsOf returns the editor fields of it; the ports are a copy.
func editFieldsOf(it apiv0.WidgetTypeResponse) editFields {
	ports := make([]apiv0.InputPort, len(it.InputPorts))
	copy(ports, it.InputPorts)
	return editFields{
		name:       it.Name,
		html:       it.HtmlTemplate,
		script:     it.Script,
		scriptLang: it.ScriptLanguage,
		ports:      ports,
	}
}

// mergeEditFields updates each field of local that still holds the old server
// value to current. The ports are merged as one list; current's ports are
// taken as they are, so current must own them (editFieldsOf copies).
func mergeEditFields(local, old, current editFields) editFields {
	m := uiutil.MergeField[string]
	merged := editFields{
		name:       m(local.name, old.name, current.name),
		html:       m(local.html, old.html, current.html),
		script:     m(local.script, old.script, current.script),
		scriptLang: m(local.scriptLang, old.scriptLang, current.scriptLang),
		ports:      local.ports,
	}
	if slices.Equal(local.ports, old.ports) {
		merged.ports = current.ports
	}
	return merged
}

// selectItem selects the WidgetType id and fills the editor with it.
func (l *Library) selectItem(id uuid.UUID) {
	it, ok := l.findItem(id)
	if !ok {
		return
	}
	l.selectedID = id
	l.edit = editFieldsOf(it)
	l.clearPortForm()
}

// clearPortForm empties the add-port form.
func (l *Library) clearPortForm() {
	l.newPortName = ""
	l.newPortDesc = ""
	l.newPortType = ""
}

// mergeSelected updates each editor field that still holds the old server value.
// Preview input values and the add-port form are editor-only state and kept.
func (l *Library) mergeSelected(old, current apiv0.WidgetTypeResponse) {
	l.edit = mergeEditFields(l.edit, editFieldsOf(old), editFieldsOf(current))
}
