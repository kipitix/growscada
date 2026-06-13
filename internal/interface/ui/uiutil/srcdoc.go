package uiutil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/interface/ui/uidto"
)

// styleCloseRE matches </style> in any capitalisation or with trailing whitespace.
var styleCloseRE = regexp.MustCompile(`(?i)</style\s*>`)

var jsIdentRE = regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z0-9_$]*$`)

// IsValidJSIdentifier reports whether s can safely be used as an unquoted JS
// object key. Call this before accepting user-supplied port names.
func IsValidJSIdentifier(s string) bool {
	return jsIdentRE.MatchString(s)
}

// IframeBgColor reads --bg from the parent document's computed CSS.
func IframeBgColor() string {
	color := strings.TrimSpace(
		app.Window().Call("getComputedStyle",
			app.Window().Get("document").Get("documentElement"),
		).Call("getPropertyValue", "--bg").String(),
	)
	if color == "" {
		return "#ffffff"
	}
	return color
}

// IframeTextMuted reads --text-muted from the parent document's computed CSS.
func IframeTextMuted() string {
	color := strings.TrimSpace(
		app.Window().Call("getComputedStyle",
			app.Window().Get("document").Get("documentElement"),
		).Call("getPropertyValue", "--text-muted").String(),
	)
	if color == "" {
		return "#888888"
	}
	return color
}

// SetIframeSrcdoc sets the srcdoc property of the iframe identified by id.
func SetIframeSrcdoc(id, srcdoc string) {
	elem := app.Window().Get("document").Call("getElementById", id)
	if !elem.IsNull() && !elem.IsUndefined() {
		elem.Set("srcdoc", srcdoc)
	}
}

// BuildSrcdoc constructs the iframe srcdoc for sandboxed widget preview.
// It injects the background colour, applies a connect-src CSP, and escapes
// any </script> in user-authored code to prevent early script-tag termination.
func BuildSrcdoc(htmlTemplate, script string, inputValues map[string]string, ports []uidto.InputPortDTO, bgColor string) string {
	// Strip </style> (any capitalisation/spacing) so a CSS variable value cannot
	// close the injected <style> block prematurely.
	bgColor = styleCloseRE.ReplaceAllString(bgColor, "")

	callRender := ""
	if len(ports) > 0 {
		parts := make([]string, 0, len(ports))
		for _, p := range ports {
			parts = append(parts, p.Name+":"+PortValueToJS(inputValues[p.Name], p.TypeHint))
		}
		callRender = fmt.Sprintf("\nvar inputs={%s};\ntry{render(inputs);}catch(e){}", strings.Join(parts, ","))
		// Escape </script> in callRender (built from port names) the same way
		// safeScript is escaped below — port names have no character restriction.
		callRender = strings.ReplaceAll(callRender, "</script>", `<\/script>`)
	}
	// Escape </script> so a literal occurrence in user-authored JS cannot
	// terminate the enclosing <script> element and inject new HTML.
	safeScript := strings.ReplaceAll(script, "</script>", `<\/script>`)
	return fmt.Sprintf(
		`<!DOCTYPE html><html><head><meta http-equiv="Content-Security-Policy" content="connect-src 'none'"><style>html,body{background:%s;margin:0;padding:0}</style></head><body>%s<script>%s%s</script></body></html>`,
		bgColor, htmlTemplate, safeScript, callRender)
}

// PortValueToJS converts a user-entered string to a JS literal based on type hint.
// Values are validated/encoded to prevent JS injection in the preview srcdoc.
func PortValueToJS(val, typeHint string) string {
	if val == "" {
		switch typeHint {
		case "integer":
			return "0"
		case "boolean":
			return "false"
		case "string":
			return `""`
		default:
			return "undefined"
		}
	}
	switch typeHint {
	case "integer":
		if _, err := strconv.ParseInt(val, 10, 64); err == nil {
			return val
		}
		return "0"
	case "boolean":
		if val == "true" || val == "false" {
			return val
		}
		return "false"
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}
