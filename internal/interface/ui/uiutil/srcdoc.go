package uiutil

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/interface/ui/uidto"
)

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

// BuildSrcdoc constructs the iframe srcdoc for sandboxed widget preview.
// It injects the background colour, applies a connect-src CSP, and escapes
// any </script> in user-authored code to prevent early script-tag termination.
func BuildSrcdoc(htmlTemplate, script string, inputValues map[string]string, ports []uidto.InputPortDTO, bgColor string) string {
	// Prevent a CSS value containing </style> from breaking the HTML structure.
	bgColor = strings.ReplaceAll(bgColor, "</style>", "")

	callRender := ""
	if len(ports) > 0 {
		parts := make([]string, 0, len(ports))
		for _, p := range ports {
			parts = append(parts, p.Name+":"+PortValueToJS(inputValues[p.Name], p.TypeHint))
		}
		callRender = fmt.Sprintf("\nvar inputs={%s};\ntry{render(inputs);}catch(e){}", strings.Join(parts, ","))
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
