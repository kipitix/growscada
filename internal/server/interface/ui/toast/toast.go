package toast

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// ActionAdd is the action name for pushing a new toast notification.
const ActionAdd = "toast.add"

// Problem is what a toast shows: a server's refusal (its Problem Details as
// they are, so a field added there reaches the toast), or a synthetic problem
// for a failure on this side, with Status 0.
type Problem struct {
	apiv0.Problem
	Method string // HTTP method of the refused request
}

// synthetic is the Problem of a failure on this side: the server did not
// answer, or not as expected.
func synthetic(title string, err error) Problem {
	return Problem{Problem: apiv0.Problem{Title: title, Detail: err.Error()}}
}

// NetworkError creates a Problem for a transport-level failure.
func NetworkError(err error) Problem {
	return synthetic("Network Error", err)
}

// ClientError creates a Problem for a request the UI could not build from its
// own state, e.g. invalid input; nothing was sent to the server.
func ClientError(err error) Problem {
	return synthetic("Client Error", err)
}

// BadResponse creates a Problem for a server's answer the UI cannot read.
func BadResponse(err error) Problem {
	return synthetic("Bad Response", err)
}

// FromError creates the Problem for an error of an apiclient call: the
// server's Problem Details if it answered with one (*apiclient.StatusError),
// a Bad Response if its answer could not be read, a Network Error if the
// request did not reach it (*url.Error), and a Client Error for a request
// this side could not even build.
func FromError(err error) Problem {
	var se *apiclient.StatusError
	var ue *url.Error
	switch {
	case errors.As(err, &se):
		return Problem{Problem: se.Problem, Method: se.Method}
	case errors.Is(err, apiclient.ErrBadResponse):
		return BadResponse(err)
	case errors.As(err, &ue):
		return NetworkError(err)
	default:
		return ClientError(err)
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
			if c.items[i].phase == phaseExiting {
				return // don't regress from an exit in progress
			}
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

	// Header: [METHOD]  STATUS LABEL  ·····  timestamp
	headerLeft := []app.UI{}
	if t.problem.Method != "" {
		headerLeft = append(headerLeft,
			app.Span().
				Style("font-size", "10px").
				Style("font-family", "monospace").
				Style("font-weight", "700").
				Style("color", "var(--toast-text-meta)").
				Style("margin-right", "8px").
				Text(t.problem.Method),
		)
	}
	headerLeft = append(headerLeft,
		app.Span().
			Style("font-size", "11px").
			Style("font-weight", "700").
			Style("color", titleColor).
			Style("letter-spacing", "0.04em").
			Text(c.statusLabel(t.problem)),
	)

	header := app.Div().
		Style("display", "flex").
		Style("align-items", "baseline").
		Style("margin-bottom", "6px").
		Body(append(headerLeft,
			app.Span().
				Style("font-size", "10px").
				Style("color", "var(--toast-text-meta)").
				Style("margin-left", "auto").
				Style("white-space", "nowrap").
				Style("padding-left", "12px").
				Text(t.createdAt.Format("15:04:05")),
		)...)

	children := []app.UI{header}

	if t.problem.Detail != "" {
		children = append(children, app.Div().
			Style("font-size", "11px").
			Style("color", "var(--toast-text)").
			Style("margin-bottom", "4px").
			Style("word-break", "break-word").
			Text(t.problem.Detail))
	}

	if t.problem.Instance != "" {
		children = append(children, app.Div().
			Style("font-size", "10px").
			Style("color", "var(--toast-text-instance)").
			Style("font-family", "monospace").
			Style("word-break", "break-all").
			Text(t.problem.Instance))
	}

	card := app.Div().
		Style("background", "var(--toast-bg)").
		Style("border", "1px solid "+borderColor).
		Style("border-radius", "6px").
		Style("padding", "10px 14px").
		Style("min-width", "340px").
		Style("max-width", "520px").
		Style("pointer-events", "auto").
		Style("box-shadow", "var(--toast-shadow)").
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
		return "var(--toast-err-border)", "var(--toast-err-title)"
	case status >= 400:
		return "var(--toast-warn-border)", "var(--toast-warn-title)"
	case status >= 300:
		return "var(--toast-info-border)", "var(--toast-info-title)"
	default:
		return "var(--toast-muted-border)", "var(--toast-muted-title)"
	}
}
