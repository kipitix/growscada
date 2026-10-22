package devicesim

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/kipitix/growscada/internal/devicelink"
)

// Handler serves the control API:
//
//	GET    /control/device                       Device status
//	POST   /control/device/start                 resume writing
//	POST   /control/device/stop                  pause writing (answers once silent)
//	POST   /control/device/tags/{name}/pattern   body: a pattern object
//	POST   /control/device/tags/{name}/quality   body: {"quality":"bad"}
//	DELETE /control/device/tags/{name}/quality   back to the configured quality
//
// The API is served from the start, while the Device is still connecting to
// the server; tag calls answer 503 until it is connected. Every successful
// call answers with the Device status; errors are RFC 7807 Problem Details.
func (d *Device) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range d.routes() {
		mux.HandleFunc(rt.pattern, rt.handler)
	}
	return mux
}

// route is one ServeMux pattern and its handler.
type route struct {
	pattern string
	handler http.HandlerFunc
}

// routes is every route of the control API. The Bruno collection check
// (bruno_collection_internal_test.go) matches the collection against it.
func (d *Device) routes() []route {
	return []route{
		{"POST /control/device/start", func(w http.ResponseWriter, r *http.Request) {
			d.Start()
			d.sendStatus(w)
		}},
		{"POST /control/device/stop", func(w http.ResponseWriter, r *http.Request) {
			d.Stop()
			d.sendStatus(w)
		}},
		{"GET /control/device", func(w http.ResponseWriter, r *http.Request) {
			d.sendStatus(w)
		}},
		{"POST /control/device/tags/{name}/pattern", func(w http.ResponseWriter, r *http.Request) {
			var p PatternSpec
			if !decodeBody(w, r, &p) {
				return
			}
			d.reply(w, r, d.SetPattern(r.PathValue("name"), p))
		}},
		{"POST /control/device/tags/{name}/quality", func(w http.ResponseWriter, r *http.Request) {
			var body qualityBody
			if !decodeBody(w, r, &body) {
				return
			}
			d.reply(w, r, d.OverrideQuality(r.PathValue("name"), body.Quality))
		}},
		{"DELETE /control/device/tags/{name}/quality", func(w http.ResponseWriter, r *http.Request) {
			d.reply(w, r, d.ResetQuality(r.PathValue("name")))
		}},
	}
}

// qualityBody is the body of POST /control/device/tags/{name}/quality.
type qualityBody struct {
	Quality devicelink.Quality `json:"quality"`
}

// reply sends the status, or the error as a problem: 404 for a tag the Device
// does not simulate, 503 while it is connecting, 400 for anything else.
func (d *Device) reply(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case err == nil:
		d.sendStatus(w)
	case errors.Is(err, ErrUnknownTag):
		sendProblem(w, r, http.StatusNotFound, err)
	case errors.Is(err, ErrConnecting):
		sendProblem(w, r, http.StatusServiceUnavailable, err)
	default:
		sendProblem(w, r, http.StatusBadRequest, err)
	}
}

func (d *Device) sendStatus(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.Status())
}

// decodeBody decodes a JSON body, rejecting unknown fields; on failure it
// sends a 400 problem and returns false.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := decodeStrict(r.Body, v); err != nil {
		sendProblem(w, r, http.StatusBadRequest, err)
		return false
	}
	return true
}

// decodeStrict decodes a JSON body into v, rejecting unknown fields.
func decodeStrict(body io.Reader, v any) error {
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// problem is an RFC 7807 Problem Details body.
type problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

func sendProblem(w http.ResponseWriter, r *http.Request, status int, err error) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(problem{
		Type:     "about:blank",
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   err.Error(),
		Instance: r.URL.Path,
	})
}
