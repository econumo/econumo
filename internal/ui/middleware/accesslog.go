package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/reqctx"
)

// healthPath is logged at DEBUG (transport) only — no INFO operation line — so
// frequent liveness probes don't drown the operation log.
const healthPath = "/health"

// logResponseWriter wraps http.ResponseWriter to capture the final status code
// and any error recorded by the handler/error layer (httpx.WriteError type-
// asserts to the exported SetError). The access-log middleware reads both back
// after the handler returns.
type logResponseWriter struct {
	http.ResponseWriter
	status  int
	written bool
	err     error
}

func (w *logResponseWriter) WriteHeader(code int) {
	if !w.written {
		w.status = code
		w.written = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *logResponseWriter) Write(b []byte) (int, error) {
	if !w.written {
		// Mirror net/http: a Write without an explicit WriteHeader implies 200.
		w.status = http.StatusOK
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

// Flush forwards to the underlying writer when it supports flushing.
func (w *logResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// SetError records the error that produced this response so the access log can
// surface its message and type. Implements the errorRecorder contract that the
// httpx package (and the panic recovery middleware) type-assert against.
func (w *logResponseWriter) SetError(err error) { w.err = err }

// AccessLog emits structured logging for each request. It installs a request-
// scoped log accumulator (so the JWT middleware, handlers, and app services can
// attach custom dimensions via reqctx.AddLogAttr) and a status-capturing writer,
// then on the way out emits two lines:
//
//   - an operation-result line — message = the static operation name (the last
//     path segment, e.g. "create-category") — at INFO (2xx), WARN (4xx), or
//     ERROR (5xx), carrying request_id, status, route, the accumulated
//     dimensions (user_id, timezone, …), and err/err_type on failure;
//   - a DEBUG "http request" transport line with method, route, status, and
//     duration_ms.
//
// OPTIONS preflight requests are skipped entirely; /health emits only the DEBUG
// transport line.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r) // CORS handles preflight; nothing to log
			return
		}

		ctx := reqctx.WithLogAttrs(r.Context())
		lw := &logResponseWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()

		next.ServeHTTP(lw, r.WithContext(ctx))

		durMs := time.Since(start).Milliseconds()
		reqID := RequestIDFromCtx(ctx)
		dims := reqctx.LogAttrs(ctx) // user_id, timezone, domain params

		if r.URL.Path != healthPath {
			attrs := []slog.Attr{
				slog.String("request_id", reqID),
				slog.Int("status", lw.status),
				slog.String("route", r.URL.Path),
			}
			if lw.err != nil {
				attrs = append(attrs,
					slog.String("err", lw.err.Error()),
					slog.String("err_type", fmt.Sprintf("%T", lw.err)),
				)
			}
			attrs = append(attrs, dims...)
			slog.LogAttrs(ctx, levelForStatus(lw.status), operationName(r.URL.Path), attrs...)
		}

		transport := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("route", r.URL.Path),
			slog.Int("status", lw.status),
			slog.Int64("duration_ms", durMs),
			slog.String("request_id", reqID),
		}
		transport = append(transport, dims...)
		slog.LogAttrs(ctx, slog.LevelDebug, "http request", transport...)
	})
}

// levelForStatus maps an HTTP status to a log level: server errors are ERROR,
// client errors WARN, everything else INFO.
func levelForStatus(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// operationName derives the static operation identifier from the request path:
// the last non-empty path segment (e.g. "/api/v1/category/create-category" ->
// "create-category"). It is intentionally free of ids/PII (these routes carry
// ids in the POST body, not the path).
func operationName(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "/"
	}
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
}
