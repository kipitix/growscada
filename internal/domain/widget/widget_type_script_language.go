package widget

import "fmt"

// ScriptLanguage - enumeration for the scripting language used in Script.
// The zero value is invalid (see docs/adr/0001-no-unknown-enum-sentinels.md).
// Value Object.
type ScriptLanguage struct {
	language int
}

var _ fmt.Stringer = ScriptLanguage{}

// Sentinel values for ScriptLanguage. Must not be reassigned.
var (
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
		return ScriptLanguage{}, fmt.Errorf("unknown script language: %q", s)
	}
}

// IsValid reports whether the language is one of the known values (not the zero value).
func (l ScriptLanguage) IsValid() bool {
	return l != ScriptLanguage{}
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
		return "invalid"
	}
}
