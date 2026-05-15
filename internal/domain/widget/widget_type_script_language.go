package widget

import "fmt"

// ScriptLanguage - enumeration for the scripting language used in Script.
// Value Object.
type ScriptLanguage struct {
	language int
}

var _ fmt.Stringer = ScriptLanguage{}

// Script language enumeration values.
var (
	ScriptLanguageUnknown    = ScriptLanguage{language: 0}
	ScriptLanguageJavaScript = ScriptLanguage{language: 1}
	ScriptLanguagePython     = ScriptLanguage{language: 2}
	ScriptLanguageLua        = ScriptLanguage{language: 3}
)

// NewScriptLanguage parses a string into the ScriptLanguage enum.
func NewScriptLanguage(s string) (ScriptLanguage, error) {
	switch s {
	case "javascript":
		return ScriptLanguageJavaScript, nil
	case "python":
		return ScriptLanguagePython, nil
	case "lua":
		return ScriptLanguageLua, nil
	default:
		return ScriptLanguageUnknown, fmt.Errorf("unknown script language: %s", s)
	}
}

// String returns the string representation of the ScriptLanguage.
func (l ScriptLanguage) String() string {
	switch l {
	case ScriptLanguageJavaScript:
		return "javascript"
	case ScriptLanguagePython:
		return "python"
	case ScriptLanguageLua:
		return "lua"
	default:
		return "unknown"
	}
}
