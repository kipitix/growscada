package indicator_type

import "fmt"

// ScriptLanguage - enumeration for the scripting language used in Script.
// Value Object.
type ScriptLanguage int

// Interfaces for ScriptLanguage.
var _ fmt.Stringer = ScriptLanguage(0)

const (
	ScriptLanguageUnknown    ScriptLanguage = iota
	ScriptLanguageJavaScript
	ScriptLanguagePython
	ScriptLanguageLua
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

// IsValid checks whether the ScriptLanguage value is a known language.
func (l ScriptLanguage) IsValid() bool {
	return l >= ScriptLanguageJavaScript && l <= ScriptLanguageLua
}
