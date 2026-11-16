package pwaserve

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestGzipHandler_CompressesWhenAccepted(t *testing.T) {
	body := strings.Repeat("growscada wasm body ", 256)
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		_, _ = w.Write([]byte(body))
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding %q, want gzip", got)
	}
	if got, err := strconv.Atoi(rec.Header().Get("Content-Length")); err != nil || got != rec.Body.Len() {
		t.Fatalf("Content-Length %q, body %d", rec.Header().Get("Content-Length"), rec.Body.Len())
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary lacks Accept-Encoding: %q", rec.Header().Get("Vary"))
	}
	if got := rec.Header().Get(UncompressedLengthHeader); got != strconv.Itoa(len(body)) {
		t.Errorf("%s %q, want the uncompressed length %d", UncompressedLengthHeader, got, len(body))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read the compressed body: %v", err)
	}
	if string(plain) != body {
		t.Errorf("the compressed body is not the answer")
	}
}

func TestGzipHandler_LeavesARefusalUncompressed(t *testing.T) {
	body := strings.Repeat("growscada wasm body ", 256)
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		_, _ = w.Write([]byte(body))
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip;q=0")

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("a refused gzip was compressed anyway: %q", got)
	}
	if rec.Body.String() != body {
		t.Errorf("the body changed")
	}
}

func TestGzipHandler_SkipsImagesAndSmallAnswers(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
	}{
		{"an image", "image/png", strings.Repeat("x", 4096)},
		{"a small answer", "text/plain", "ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write([]byte(tc.body))
			}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/asset", nil)
			req.Header.Set("Accept-Encoding", "gzip")

			handler.ServeHTTP(rec, req)

			if got := rec.Header().Get("Content-Encoding"); got != "" {
				t.Errorf("compressed %s: %q", tc.name, got)
			}
			if rec.Body.String() != tc.body {
				t.Errorf("the body changed")
			}
		})
	}
}
