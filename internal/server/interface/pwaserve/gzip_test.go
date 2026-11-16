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

// TestGzipHandler_VaryOnEveryForm: both forms of an answer are stated as such
// whatever this request asked for, so a shared cache never stores one for the
// other (task 91).
func TestGzipHandler_VaryOnEveryForm(t *testing.T) {
	body := strings.Repeat("growscada wasm body ", 256)
	serve := func(t *testing.T, method, accept string) *httptest.ResponseRecorder {
		t.Helper()
		handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/wasm")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			if r.Method != http.MethodHead {
				_, _ = w.Write([]byte(body))
			}
		}))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/app.wasm", nil)
		if accept != "" {
			req.Header.Set("Accept-Encoding", accept)
		}
		handler.ServeHTTP(rec, req)
		return rec
	}

	for _, tc := range []struct {
		name   string
		method string
		accept string
	}{
		{"a refusal", http.MethodGet, "identity"},
		{"no Accept-Encoding", http.MethodGet, ""},
		{"a HEAD", http.MethodHead, "gzip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(t, tc.method, tc.accept)
			if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
				t.Errorf("Vary lacks Accept-Encoding: %q", rec.Header().Get("Vary"))
			}
			if got := rec.Header().Get("Content-Encoding"); got != "" {
				t.Errorf("an answer that is not compressed says %q", got)
			}
		})
	}
	if rec := serve(t, http.MethodHead, "gzip"); rec.Body.Len() != 0 {
		t.Errorf("a HEAD carries a body of %d bytes", rec.Body.Len())
	}
}

// TestGzipHandler_ParsesTheQuality: a quality the client writes in any of its
// accepted forms is read as a number, and a wildcard covers gzip when nothing
// names it (task 91).
func TestGzipHandler_ParsesTheQuality(t *testing.T) {
	for _, tc := range []struct {
		accept string
		want   bool
	}{
		{"gzip", true},
		{"gzip;q=0.5", true},
		{"gzip;q=1.0", true},
		{"gzip;q=0", false},
		{"gzip;q=0.0", false},
		{"gzip;q=0.000", false},
		{"gzip; q=0.00", false},
		{"gzip; Q=0", false},
		{"gzip;q=nonsense", false},
		{"deflate, gzip;q=0.001", true},
		{"deflate", false},
		{"identity", false},
		{"", false},
		{"*", true},
		{"*;q=0", false},
		{"deflate, *;q=0.2", true},
		{"gzip;q=0, *", false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
		if tc.accept != "" {
			r.Header.Set("Accept-Encoding", tc.accept)
		}
		if got := acceptsGzip(r); got != tc.want {
			t.Errorf("%q: acceptsGzip %v, want %v", tc.accept, got, tc.want)
		}
	}
}

// TestGzipHandler_StatesTheLengthOfTheWasmOnly: the header the go-app loader
// reads belongs to the wasm answer alone; the page and the other resources
// carry none (task 91).
func TestGzipHandler_StatesTheLengthOfTheWasmOnly(t *testing.T) {
	body := strings.Repeat("growscada body ", 128)
	for _, tc := range []struct {
		contentType string
		want        bool
	}{
		{"application/wasm", true},
		{"text/html; charset=utf-8", false},
		{"application/javascript", false},
	} {
		handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", tc.contentType)
			_, _ = w.Write([]byte(body))
		}))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/resource", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		handler.ServeHTTP(rec, req)

		got := rec.Header().Get(UncompressedLengthHeader)
		if tc.want && got != strconv.Itoa(len(body)) {
			t.Errorf("%s: %s %q, want %d", tc.contentType, UncompressedLengthHeader, got, len(body))
		}
		if !tc.want && got != "" {
			t.Errorf("%s: the wasm length is stated on another answer: %q", tc.contentType, got)
		}
	}
}

// TestGzipHandler_LeavesAnEncodedAnswerAlone: an answer that already states its
// Content-Encoding is not compressed again, and no wasm length is claimed for
// bytes that are encoded (task 91).
func TestGzipHandler_LeavesAnEncodedAnswerAlone(t *testing.T) {
	body := strings.Repeat("encoded wasm ", 128)
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		w.Header().Set("Content-Encoding", "br")
		_, _ = w.Write([]byte(body))
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Errorf("Content-Encoding %q, want the answer's own br", got)
	}
	if got := rec.Header().Get(UncompressedLengthHeader); got != "" {
		t.Errorf("%s %q on an encoded answer", UncompressedLengthHeader, got)
	}
	if rec.Body.String() != body {
		t.Errorf("the body changed")
	}
}

// TestGzipHandler_LeavesANotModifiedAnswerAlone: a 304 has no body to compress
// and keeps none, with Vary still stated (task 91).
func TestGzipHandler_LeavesANotModifiedAnswerAlone(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		w.WriteHeader(http.StatusNotModified)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Errorf("status %d, want 304", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("a 304 was compressed: %q", got)
	}
	if got := rec.Header().Get(UncompressedLengthHeader); got != "" {
		t.Errorf("a 304 states a length: %q", got)
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("a 304 lacks Vary: %q", rec.Header().Get("Vary"))
	}
}
