package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/web/httpx"
)

// capRec is one captured log record (level, message, flattened attrs).
type capRec struct {
	level slog.Level
	msg   string
	attrs map[string]string
}

type capHandler struct{ recs *[]capRec }

func (h capHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h capHandler) Handle(_ context.Context, r slog.Record) error {
	m := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.String()
		return true
	})
	*h.recs = append(*h.recs, capRec{level: r.Level, msg: r.Message, attrs: m})
	return nil
}
func (h capHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h capHandler) WithGroup(string) slog.Handler      { return h }

// captureLogs swaps in a capturing default logger for the duration of a test.
func captureLogs(t *testing.T) *[]capRec {
	t.Helper()
	prev := slog.Default()
	recs := &[]capRec{}
	slog.SetDefault(slog.New(capHandler{recs: recs}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return recs
}

func find(recs []capRec, msg string) (capRec, bool) {
	for _, r := range recs {
		if r.msg == msg {
			return r, true
		}
	}
	return capRec{}, false
}

const createCategoryPath = "/api/v1/category/create-category"

func TestAccessLog_Success_OpAndTransportLines(t *testing.T) {
	recs := captureLogs(t)
	h := AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqctx.AddLogAttr(r.Context(), "user_id", "u-9")
		reqctx.AddLogAttr(r.Context(), "timezone", "UTC")
		reqctx.AddLogAttr(r.Context(), "category_id", "c-1")
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, createCategoryPath, nil))

	op, ok := find(*recs, "create-category")
	if !ok {
		t.Fatalf("no operation line; records=%+v", *recs)
	}
	if op.level != slog.LevelInfo {
		t.Errorf("op level=%v want INFO", op.level)
	}
	for _, k := range []string{"user_id", "timezone", "category_id", "request_id", "status", "route"} {
		if _, has := op.attrs[k]; !has {
			t.Errorf("op line missing attr %q; got %v", k, op.attrs)
		}
	}
	if op.attrs["status"] != "200" {
		t.Errorf("status=%q want 200", op.attrs["status"])
	}

	tr, ok := find(*recs, "http request")
	if !ok {
		t.Fatalf("no transport line; records=%+v", *recs)
	}
	if tr.level != slog.LevelDebug {
		t.Errorf("transport level=%v want DEBUG", tr.level)
	}
	if _, has := tr.attrs["duration_ms"]; !has {
		t.Errorf("transport line missing duration_ms; got %v", tr.attrs)
	}
}

func TestAccessLog_ClientError_WarnWithErr(t *testing.T) {
	recs := captureLogs(t)
	h := AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(r.Context(), w, errs.NewValidation("bad input"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, createCategoryPath, nil))

	op, ok := find(*recs, "create-category")
	if !ok {
		t.Fatalf("no operation line; records=%+v", *recs)
	}
	if op.level != slog.LevelWarn {
		t.Errorf("op level=%v want WARN", op.level)
	}
	if op.attrs["status"] != "400" {
		t.Errorf("status=%q want 400", op.attrs["status"])
	}
	if op.attrs["err"] == "" {
		t.Errorf("op line missing err attr; got %v", op.attrs)
	}
	if op.attrs["err_type"] == "" {
		t.Errorf("op line missing err_type attr; got %v", op.attrs)
	}
}

func TestAccessLog_ServerError_Error(t *testing.T) {
	recs := captureLogs(t)
	h := AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(r.Context(), w, errors.New("boom")) // unhandled -> 500
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, createCategoryPath, nil))

	op, ok := find(*recs, "create-category")
	if !ok {
		t.Fatalf("no operation line; records=%+v", *recs)
	}
	if op.level != slog.LevelError {
		t.Errorf("op level=%v want ERROR", op.level)
	}
	if op.attrs["status"] != "500" {
		t.Errorf("status=%q want 500", op.attrs["status"])
	}
	if op.attrs["err"] != "boom" {
		t.Errorf("err=%q want boom", op.attrs["err"])
	}
}

func TestAccessLog_Options_Skipped(t *testing.T) {
	recs := captureLogs(t)
	var ran bool
	h := AccessLog(okHandler(&ran))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodOptions, createCategoryPath, nil))

	if !ran {
		t.Fatal("OPTIONS must still reach the downstream handler")
	}
	if len(*recs) != 0 {
		t.Fatalf("OPTIONS must not log anything; got %+v", *recs)
	}
}

func TestAccessLog_Health_TransportOnly(t *testing.T) {
	recs := captureLogs(t)
	h := AccessLog(okHandler(nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, healthPath, nil))

	if _, ok := find(*recs, "http request"); !ok {
		t.Error("health should emit the transport line")
	}
	if _, ok := find(*recs, "health"); ok {
		t.Error("health must NOT emit an operation line")
	}
}
