package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var bigJSON = `{"items":[` + strings.Repeat(`{"id":"0190a0e2-0000-7000-8000-000000000000","amount":"12.50"},`, 100) + `{}]}`

func serveCompressed(t *testing.T, h http.HandlerFunc, method, acceptEncoding string, extra http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/x", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	for k, v := range extra {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	Compress(h).ServeHTTP(rec, req)
	return rec
}

func writeJSON(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "999999")
		w.WriteHeader(status)
		// in pieces, so the size threshold is crossed mid-body
		for i := 0; i < len(body); i += 300 {
			_, _ = io.WriteString(w, body[i:min(i+300, len(body))])
		}
	}
}

func gunzip(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	return string(out)
}

func TestCompress_GzipsLargeJSON(t *testing.T) {
	rec := serveCompressed(t, writeJSON(http.StatusBadRequest, bigJSON), http.MethodGet, "br, gzip;q=0.8", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding=%q want gzip", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length=%q must be dropped", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary=%q want Accept-Encoding", got)
	}
	if rec.Body.Len() >= len(bigJSON) {
		t.Fatalf("compressed %d bytes, not smaller than %d", rec.Body.Len(), len(bigJSON))
	}
	if got := gunzip(t, rec.Body.Bytes()); got != bigJSON {
		t.Fatalf("round trip mismatch")
	}
}

func TestCompress_PassesThrough(t *testing.T) {
	cases := []struct {
		name   string
		h      http.HandlerFunc
		method string
		accept string
		extra  http.Header
		body   string
	}{
		{"client without gzip", writeJSON(http.StatusOK, bigJSON), http.MethodGet, "", nil, bigJSON},
		{"gzip refused with q=0", writeJSON(http.StatusOK, bigJSON), http.MethodGet, "gzip;q=0, identity", nil, bigJSON},
		{"gzip refused despite a wildcard", writeJSON(http.StatusOK, bigJSON), http.MethodGet, "gzip;q=0, *;q=1", nil, bigJSON},
		{"small body", writeJSON(http.StatusOK, `{"success":true}`), http.MethodGet, "gzip", nil, `{"success":true}`},
		{"range request", writeJSON(http.StatusOK, bigJSON), http.MethodGet, "gzip", http.Header{"Range": {"bytes=0-10"}}, bigJSON},
		{"already compressed type", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, bigJSON)
		}, http.MethodGet, "gzip", nil, bigJSON},
		{"own content encoding", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "br")
			_, _ = io.WriteString(w, bigJSON)
		}, http.MethodGet, "gzip", nil, bigJSON},
		{"event stream", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, bigJSON)
		}, http.MethodGet, "gzip", nil, bigJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveCompressed(t, tc.h, tc.method, tc.accept, tc.extra)
			if got := rec.Header().Get("Content-Encoding"); got == "gzip" {
				t.Fatalf("Content-Encoding=gzip, want passthrough")
			}
			if rec.Body.String() != tc.body {
				t.Fatalf("body=%q want %q", rec.Body.String(), tc.body)
			}
		})
	}
}

func TestCompress_StatusWithoutBody(t *testing.T) {
	rec := serveCompressed(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}, http.MethodGet, "gzip", nil)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("status=%d body=%d encoding=%q want 304, empty, none", rec.Code, rec.Body.Len(), rec.Header().Get("Content-Encoding"))
	}
}

func TestCompress_SniffsUntypedBodyAndWeakensETag(t *testing.T) {
	html := "<!doctype html><html><body>" + strings.Repeat("<p>econumo</p>", 200) + "</body></html>"
	rec := serveCompressed(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		_, _ = io.WriteString(w, html)
	}, http.MethodGet, "gzip", nil)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("untyped html not compressed")
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type=%q want sniffed text/html", got)
	}
	if got := rec.Header().Get("ETag"); got != `W/"abc"` {
		t.Fatalf("ETag=%q want weak", got)
	}
	if gunzip(t, rec.Body.Bytes()) != html {
		t.Fatalf("round trip mismatch")
	}
}

func TestCompress_FlushCommitsAndStreams(t *testing.T) {
	rec := serveCompressed(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"a":`)
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, `1}`)
	}, http.MethodGet, "gzip", nil)
	if !rec.Flushed {
		t.Fatalf("flush not forwarded")
	}
	if rec.Body.String() != `{"a":1}` {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestCompress_KeepsErrorRecorderReachable(t *testing.T) {
	// AccessLog sits inside Compress, so httpx's SetError type assertion still
	// lands on the logging writer.
	var recorded bool
	h := AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, recorded = w.(interface{ SetError(error) })
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	Compress(h).ServeHTTP(httptest.NewRecorder(), req)
	if !recorded {
		t.Fatalf("SetError not reachable through Compress")
	}
}

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{
		"":                   false,
		"gzip":               true,
		"GZIP":               true,
		"deflate, gzip":      true,
		"gzip;q=0":           false,
		"gzip; q=0.001":      true,
		"*":                  true,
		"*;q=0":              false,
		"br, identity":       false,
		"x-gzip":             false,
		"gzip;level=1;q=0.5": true,
		"gzip;q=0, *;q=1":    false,
		"*;q=1, gzip;q=0":    false,
		"*;q=0, gzip":        true,
		"br, *;q=0.5":        true,
	} {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q)=%v want %v", header, got, want)
		}
	}
}
