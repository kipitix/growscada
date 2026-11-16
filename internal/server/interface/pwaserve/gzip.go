package pwaserve

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
)

// minCompressSize is the answer below which gzip costs more than it saves: the
// PWA's app.wasm is far above it, the page and its small resources are not.
const minCompressSize = 1024

// UncompressedLengthHeader names the length of the answer before gzip. go-app's
// wasm loader counts the bytes its reader yields — the browser hands it the
// decompressed body — so it must read this length, not the compressed
// Content-Length, or its progress passes 100%. main.go gives the handler this
// name as app.Handler.WasmContentLengthHeader.
const UncompressedLengthHeader = "GrowSCADA-Wasm-Length"

// gzipHandler compresses the PWA's textual answers — app.wasm above all — when
// the browser accepts gzip. It buffers an answer, so Content-Length stays exact:
// go-app's wasm loader reads it for its progress bar. The API server is not
// behind it, so no event stream is delayed.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		buffered := &bufferedWriter{header: make(http.Header)}
		next.ServeHTTP(buffered, r)
		status := buffered.status
		if status == 0 {
			status = http.StatusOK
		}
		for key, values := range buffered.header {
			w.Header()[key] = values
		}
		w.Header().Add("Vary", "Accept-Encoding")
		body := buffered.body.Bytes()
		// The loader's progress counts the bytes it reads: the browser
		// decompresses them, so the length it has to compare with is this one,
		// never the compressed Content-Length (task 91).
		w.Header().Set(UncompressedLengthHeader, strconv.Itoa(len(body)))
		if status != http.StatusOK || len(body) < minCompressSize ||
			w.Header().Get("Content-Encoding") != "" ||
			!compressible(w.Header().Get("Content-Type")) {
			writeUncompressed(w, status, body)
			return
		}
		var compressed bytes.Buffer
		zw := gzip.NewWriter(&compressed)
		if _, err := zw.Write(body); err != nil || zw.Close() != nil || compressed.Len() >= len(body) {
			writeUncompressed(w, status, body)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(compressed.Len()))
		w.WriteHeader(status)
		_, _ = w.Write(compressed.Bytes())
	})
}

// writeUncompressed answers as the wrapped handler did, with the length the
// buffered body really has.
func writeUncompressed(w http.ResponseWriter, status int, body []byte) {
	if status != http.StatusNotModified && w.Header().Get("Content-Length") == "" {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// compressible reports whether an answer of that type gains from gzip. Images
// and fonts are already compressed and are left alone.
func compressible(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(ct, "text/"):
		return true
	case strings.HasPrefix(ct, "application/javascript"):
		return true
	case strings.HasPrefix(ct, "application/json"):
		return true
	case strings.HasPrefix(ct, "application/manifest+json"):
		return true
	case strings.HasPrefix(ct, "application/wasm"):
		return true
	case strings.HasPrefix(ct, "image/svg+xml"):
		return true
	default:
		return false
	}
}

// acceptsGzip reports whether the client accepts a gzip answer: “gzip” with a
// zero quality is a refusal.
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), "gzip") {
			continue
		}
		for _, param := range fields[1:] {
			if strings.TrimSpace(param) == "q=0" || strings.TrimSpace(param) == "q=0.0" {
				return false
			}
		}
		return true
	}
	return false
}

// bufferedWriter collects an answer so gzipHandler can give it its final
// Content-Length. It implements only http.ResponseWriter: the PWA handler
// streams nothing.
type bufferedWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedWriter) Header() http.Header { return b.header }

func (b *bufferedWriter) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedWriter) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}
