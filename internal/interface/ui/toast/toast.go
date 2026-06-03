package toast

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ActionAdd is the action name for pushing a new toast notification.
const ActionAdd = "toast.add"

// Problem mirrors the RFC 9457 fields displayed in a toast.
type Problem struct {
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// NetworkError creates a Problem for a transport-level failure.
func NetworkError(err error) Problem {
	return Problem{Title: "Network Error", Status: 0, Detail: err.Error()}
}

// FromHTTPError reads and closes resp.Body, parses it as RFC 9457,
// and returns a Problem. Falls back to a synthetic one on parse failure.
func FromHTTPError(resp *http.Response) Problem {
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var p Problem
	if json.Unmarshal(body, &p) == nil && p.Title != "" {
		if p.Status == 0 {
			p.Status = resp.StatusCode
		}
		return p
	}
	detail := strings.TrimSpace(string(body))
	return Problem{
		Title:  http.StatusText(resp.StatusCode),
		Status: resp.StatusCode,
		Detail: detail,
	}
}

// ── Component ─────────────────────────────────────────────────────────────────

type toastPhase string

const (
	phaseEntering toastPhase = "entering"
	phaseVisible  toastPhase = "visible"
	phaseExiting  toastPhase = "exiting"
)

type toastItem struct {
	id        string
	problem   Problem
	createdAt time.Time
	phase     toastPhase
}

// Container is the fixed-position overlay that renders toast notifications.
// Render it once at the root level.
type Container struct {
	app.Compo
	items []toastItem
	seq   int
}

func (c *Container) OnMount(ctx app.Context) {
	ctx.Handle(ActionAdd, func(ctx app.Context, a app.Action) {
		prob, ok := a.Value.(Problem)
		if !ok {
			return
		}
		c.seq++
		id := fmt.Sprintf("gs-toast-%d", c.seq)

		// Append so go-app reconciler sees only one new DOM node.
		c.items = append(c.items, toastItem{
			id:        id,
			problem:   prob,
			createdAt: time.Now(),
			phase:     phaseEntering,
		})

		// Timer starts after entry animation (400 ms), not when queued.
		ctx.After(400*time.Millisecond, func(ctx app.Context) {
			c.setPhase(id, phaseVisible)
			ctx.After(5*time.Second, func(ctx app.Context) {
				c.setPhase(id, phaseExiting)
				ctx.After(560*time.Millisecond, func(ctx app.Context) {
					c.remove(id)
				})
			})
		})
	})
}

func (c *Container) setPhase(id string, phase toastPhase) {
	for i := range c.items {
		if c.items[i].id == id {
			c.items[i].phase = phase
			return
		}
	}
}

func (c *Container) remove(id string) {
	for i, t := range c.items {
		if t.id == id {
			c.items = append(c.items[:i], c.items[i+1:]...)
			return
		}
	}
}

// ── Render ────────────────────────────────────────────────────────────────────

func (c *Container) Render() app.UI {
	if len(c.items) == 0 {
		return app.Div().Style("display", "none")
	}

	toasts := make([]app.UI, len(c.items))
	for i, t := range c.items {
		toasts[i] = c.renderToast(t)
	}

	return app.Div().
		Style("position", "fixed").
		Style("bottom", "16px").
		Style("right", "16px").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("align-items", "flex-end").
		Style("z-index", "9999").
		Style("pointer-events", "none").
		Body(toasts...)
}

func (c *Container) renderToast(t toastItem) app.UI {
	borderColor, titleColor := statusColors(t.problem.Status)

	var wrapperAnim, cardAnim string
	switch t.phase {
	case phaseEntering:
		wrapperAnim = "gs-toast-wrap-enter 0.4s ease-out forwards"
		cardAnim = "gs-toast-card-enter 0.4s ease-out forwards"
	case phaseExiting:
		wrapperAnim = "gs-toast-wrap-exit 0.55s ease-in forwards"
		cardAnim = "gs-toast-card-exit 0.55s ease-in forwards"
	}

	// Header: status badge + timestamp
	header := app.Div().
		Style("display", "flex").
		Style("justify-content", "space-between").
		Style("align-items", "baseline").
		Style("margin-bottom", "6px").
		Body(
			app.Span().
				Style("font-size", "11px").
				Style("font-weight", "700").
				Style("color", titleColor).
				Style("letter-spacing", "0.04em").
				Text(c.statusLabel(t.problem)),
			app.Span().
				Style("font-size", "10px").
				Style("color", "#666666").
				Style("margin-left", "12px").
				Style("white-space", "nowrap").
				Text(t.createdAt.Format("15:04:05")),
		)

	children := []app.UI{header}

	if t.problem.Detail != "" {
		children = append(children, app.Div().
			Style("font-size", "11px").
			Style("color", "#cccccc").
			Style("margin-bottom", "4px").
			Style("word-break", "break-word").
			Text(t.problem.Detail))
	}

	if t.problem.Instance != "" {
		children = append(children, app.Div().
			Style("font-size", "10px").
			Style("color", "#666666").
			Style("font-family", "monospace").
			Style("word-break", "break-all").
			Text(t.problem.Instance))
	}

	card := app.Div().
		Style("background", "rgba(18,18,18,0.97)").
		Style("border", "1px solid "+borderColor).
		Style("border-radius", "6px").
		Style("padding", "10px 14px").
		Style("min-width", "340px").
		Style("max-width", "520px").
		Style("pointer-events", "auto").
		Style("box-shadow", "0 4px 18px rgba(0,0,0,0.55)").
		Style("box-sizing", "border-box").
		Body(children...)

	if cardAnim != "" {
		card = card.Style("animation", cardAnim)
	}

	wrapper := app.Div().
		Style("overflow", "hidden").
		Style("pointer-events", "none").
		Style("padding-bottom", "8px").
		Body(card)

	if wrapperAnim != "" {
		wrapper = wrapper.Style("animation", wrapperAnim)
	}

	return wrapper
}

func (c *Container) statusLabel(p Problem) string {
	if p.Status == 0 {
		return p.Title
	}
	return fmt.Sprintf("%d  %s", p.Status, p.Title)
}

func statusColors(status int) (border, title string) {
	switch {
	case status >= 500:
		return "#cc3333", "#ff6666"
	case status >= 400:
		return "#bb8800", "#ffcc33"
	case status >= 300:
		return "#2266cc", "#66aaff"
	default:
		return "#555555", "#aaaaaa"
	}
}
