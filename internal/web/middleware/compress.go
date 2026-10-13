package middleware

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// compressMinSize is the body size below which gzip costs more than it saves.
const compressMinSize = 1024

// Level 5: on a 4 MB transaction list it comes within 2% of the default
// level's size (10x smaller than the JSON) in 80% of its time.
var gzipPool = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(nil, 5)
	return w
}}

// Compress gzips responses for clients that accept it. A transaction list runs
// to megabytes of JSON, and on a phone the transfer, not the server, is the
// wait. Bodies under 1 KB, types that are already compressed, and responses
// carrying their own Content-Encoding pass through untouched. Range requests
// pass through too: a byte range of the uncompressed file must not be gzipped.
//
// Auth is a bearer header, never a cookie, so a cross-site page cannot make
// the browser send authenticated requests whose compressed size it could
// measure (BREACH).
func Compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		cw := &compressWriter{ResponseWriter: w}
		defer cw.finish()
		next.ServeHTTP(cw, r)
	})
}

func acceptsGzip(header string) bool {
	for part := range strings.SplitSeq(header, ",") {
		coding, params, _ := strings.Cut(part, ";")
		coding = strings.ToLower(strings.TrimSpace(coding))
		if coding != "gzip" && coding != "*" {
			continue
		}
		q := 1.0
		for p := range strings.SplitSeq(params, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
			if ok && strings.EqualFold(k, "q") {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = f
				}
			}
		}
		if q > 0 {
			return true
		}
	}
	return false
}

func compressible(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch {
	case mt == "text/event-stream":
		return false
	case strings.HasPrefix(mt, "text/"):
		return true
	case mt == "application/json", mt == "application/javascript", mt == "application/xml",
		mt == "application/manifest+json", mt == "image/svg+xml", strings.HasSuffix(mt, "+json"):
		return true
	}
	return false
}

// compressWriter holds the status and the first bytes back until it knows the
// body is big enough to compress, then commits to gzip or to passthrough.
type compressWriter struct {
	http.ResponseWriter
	status  int
	buf     []byte
	decided bool
	gz      *gzip.Writer
}

func (w *compressWriter) WriteHeader(code int) {
	if w.decided || w.status != 0 {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if code >= 100 && code < 200 {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.status = code
	if code == http.StatusNoContent || code == http.StatusNotModified {
		_ = w.decide()
	}
}

func (w *compressWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.decided {
		w.buf = append(w.buf, b...)
		if len(w.buf) >= compressMinSize {
			if err := w.decide(); err != nil {
				return 0, err
			}
		}
		return len(b), nil
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *compressWriter) decide() error {
	w.decided = true
	h := w.ResponseWriter.Header()
	if h.Get("Content-Type") == "" && len(w.buf) > 0 {
		h.Set("Content-Type", http.DetectContentType(w.buf))
	}
	if len(w.buf) >= compressMinSize && h.Get("Content-Encoding") == "" && compressible(h.Get("Content-Type")) {
		h.Del("Content-Length")
		h.Set("Content-Encoding", "gzip")
		if etag := h.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
			h.Set("ETag", "W/"+etag)
		}
		w.ResponseWriter.WriteHeader(w.status)
		w.gz = gzipPool.Get().(*gzip.Writer)
		w.gz.Reset(w.ResponseWriter)
		_, err := w.gz.Write(w.buf)
		w.buf = nil
		return err
	}
	if w.status != 0 {
		w.ResponseWriter.WriteHeader(w.status)
	}
	if len(w.buf) == 0 {
		return nil
	}
	_, err := w.ResponseWriter.Write(w.buf)
	w.buf = nil
	return err
}

func (w *compressWriter) Flush() {
	if !w.decided {
		_ = w.decide()
	}
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *compressWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *compressWriter) finish() {
	if !w.decided {
		_ = w.decide()
	}
	if w.gz != nil {
		_ = w.gz.Close()
		w.gz.Reset(nil)
		gzipPool.Put(w.gz)
		w.gz = nil
	}
}
