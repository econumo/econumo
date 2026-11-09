package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

const idemKey = "01a1be30-30af-7e10-8b56-9b545fbe3b34"

type fakeIdemStore struct {
	findErr   error
	insertErr error
	rows      map[string]string
	inserts   int
	deletes   int
	purges    int
	settleErr []error
}

func (s *fakeIdemStore) Find(_ context.Context, _ vo.Id, key string) (string, bool, error) {
	if s.findErr != nil {
		return "", false, s.findErr
	}
	res, ok := s.rows[key]
	return res, ok, nil
}

func (s *fakeIdemStore) Insert(ctx context.Context, _ vo.Id, key, res string) error {
	s.inserts++
	s.settleErr = append(s.settleErr, ctx.Err())
	if s.insertErr != nil {
		return s.insertErr
	}
	if s.rows == nil {
		s.rows = map[string]string{}
	}
	s.rows[key] = res
	return nil
}

func (s *fakeIdemStore) Delete(ctx context.Context, _ vo.Id, key string) error {
	s.deletes++
	s.settleErr = append(s.settleErr, ctx.Err())
	delete(s.rows, key)
	return nil
}

func (s *fakeIdemStore) MaybePurge(context.Context) { s.purges++ }

type idemAuthn struct {
	store middleware.IdempotencyStore
	scope model.TokenScope
}

func (a idemAuthn) Authenticate(context.Context, string) (model.Principal, error) {
	id := vo.NewId()
	scope := a.scope
	if scope == "" {
		scope = model.TokenScopeFull
	}
	return model.Principal{UserID: id, TokenID: id, Level: model.AccessLevelFull, Scope: scope}, nil
}

func (a idemAuthn) IdempotencyStore() middleware.IdempotencyStore { return a.store }

func serveIdem(t *testing.T, authn middleware.TokenAuthenticator, method, path, key string, next http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer token")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	middleware.AuthWith(authn, middleware.AuthOptions{AllowMCPScope: true})(next).ServeHTTP(rec, req)
	return rec
}

func TestIdempotency_FindErrorProceedsUnguarded(t *testing.T) {
	store := &fakeIdemStore{findErr: errors.New("db down")}
	called := false
	rec := serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", idemKey, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	if !called || rec.Code != http.StatusOK {
		t.Fatalf("called=%v status=%d", called, rec.Code)
	}
	if store.inserts != 0 || store.deletes != 0 {
		t.Fatalf("inserts=%d deletes=%d, want none", store.inserts, store.deletes)
	}
}

func TestIdempotency_NoTransactionRecordsWithTheResourceAndPurges(t *testing.T) {
	store := &fakeIdemStore{}
	rec := serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", idemKey, func(w http.ResponseWriter, r *http.Request) {
		reqctx.SetIdempotencyResource(r.Context(), "res-1")
		w.WriteHeader(http.StatusOK)
	})
	if rec.Code != http.StatusOK || store.rows[idemKey] != "res-1" || store.purges != 1 {
		t.Fatalf("status=%d rows=%v purges=%d", rec.Code, store.rows, store.purges)
	}
}

func TestIdempotency_NoTransactionFailureRecordsNothing(t *testing.T) {
	store := &fakeIdemStore{}
	serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", idemKey, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	if store.inserts != 0 || store.deletes != 0 || store.purges != 0 {
		t.Fatalf("inserts=%d deletes=%d purges=%d, want none", store.inserts, store.deletes, store.purges)
	}
}

func TestIdempotency_ReplayNeverReachesTheHandler(t *testing.T) {
	store := &fakeIdemStore{rows: map[string]string{idemKey: "res-1"}}
	rec := serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", idemKey, func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler ran on a replay")
	})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"resourceId":"res-1"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
}

func TestIdempotency_KeyIsCanonicalized(t *testing.T) {
	store := &fakeIdemStore{}
	serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", "urn:uuid:"+strings.ToUpper(idemKey), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if _, ok := store.rows[idemKey]; !ok {
		t.Fatalf("rows=%v, want the canonical key", store.rows)
	}
}

func TestIdempotency_OnlyFullScopePostsOutsideMCPAreGuarded(t *testing.T) {
	cases := []struct {
		name   string
		scope  model.TokenScope
		method string
		path   string
		key    string
	}{
		{"read", model.TokenScopeFull, http.MethodGet, "/api/v1/x", idemKey},
		{"mcp path", model.TokenScopeFull, http.MethodPost, middleware.MCPPath, idemKey},
		{"mcp scope", model.TokenScopeMCP, http.MethodPost, middleware.MCPPath, idemKey},
		{"ingest scope", model.TokenScopeIngest, http.MethodPost, middleware.IngestPathPrefix + "apple-wallet-event", idemKey},
		{"no key", model.TokenScopeFull, http.MethodPost, "/api/v1/x", ""},
		{"malformed key", model.TokenScopeFull, http.MethodPost, "/api/v1/x", "nope"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeIdemStore{findErr: errors.New("must not be consulted")}
			rec := serveIdem(t, idemAuthn{store: store, scope: c.scope}, c.method, c.path, c.key, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			if rec.Code != http.StatusOK || store.inserts != 0 {
				t.Fatalf("status=%d inserts=%d", rec.Code, store.inserts)
			}
		})
	}
}

func TestIdempotency_NilStoreIsUnguarded(t *testing.T) {
	rec := serveIdem(t, idemAuthn{}, http.MethodPost, "/api/v1/x", idemKey, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestIdempotency_TransactionRecordsOnceAcrossTopLevelTransactions(t *testing.T) {
	m := dbtest.NewSQLite(t).TX
	store := &fakeIdemStore{}
	serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", idemKey, func(w http.ResponseWriter, r *http.Request) {
		for range 2 {
			if err := m.WithTx(r.Context(), func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	if store.inserts != 1 || store.deletes != 0 {
		t.Fatalf("inserts=%d deletes=%d, want 1 and 0", store.inserts, store.deletes)
	}
}

func TestIdempotency_FailureAfterACommitReleasesTheKey(t *testing.T) {
	m := dbtest.NewSQLite(t).TX
	store := &fakeIdemStore{}
	serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", idemKey, func(w http.ResponseWriter, r *http.Request) {
		if err := m.WithTx(r.Context(), func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	if store.inserts != 1 || store.deletes != 1 || len(store.rows) != 0 {
		t.Fatalf("inserts=%d deletes=%d rows=%v", store.inserts, store.deletes, store.rows)
	}
}

// A duplicate that lost the primary-key race must leave the winner's row alone.
func TestIdempotency_LostKeyRaceDoesNotReleaseTheKey(t *testing.T) {
	m := dbtest.NewSQLite(t).TX
	conflict := errors.New("primary key conflict")
	store := &fakeIdemStore{insertErr: conflict}
	rec := serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", idemKey, func(w http.ResponseWriter, r *http.Request) {
		err := m.WithTx(r.Context(), func(context.Context) error { return nil })
		if !errors.Is(err, conflict) {
			t.Fatalf("err=%v, want the conflict", err)
		}
		httpx.WriteError(r.Context(), w, err)
	})
	if rec.Code != http.StatusInternalServerError || store.deletes != 0 || store.inserts != 1 {
		t.Fatalf("status=%d deletes=%d inserts=%d", rec.Code, store.deletes, store.inserts)
	}
}

// The commit failed after the hook ran, so the key row is gone with it and the
// row now under that key, if any, is another request's.
func TestIdempotency_RolledBackHookDoesNotReleaseTheKey(t *testing.T) {
	m := dbtest.NewSQLite(t).TX
	store := &fakeIdemStore{}
	boom := errors.New("boom")
	serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", idemKey, func(w http.ResponseWriter, r *http.Request) {
		_ = m.WithTx(r.Context(), func(ctx context.Context) error {
			m.BeforeCommit(ctx, "k", func(context.Context) error { return boom })
			return nil
		})
		w.WriteHeader(http.StatusInternalServerError)
	})
	if store.deletes != 0 {
		t.Fatalf("deletes=%d, want 0", store.deletes)
	}
}

func TestIdempotency_CancelledClientStillRecords(t *testing.T) {
	store := &fakeIdemStore{}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/x", strings.NewReader(`{}`)).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Idempotency-Key", idemKey)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(http.StatusOK)
	})
	middleware.AuthWith(idemAuthn{store: store}, middleware.AuthOptions{})(next).ServeHTTP(httptest.NewRecorder(), req)
	if store.inserts != 1 || store.settleErr[0] != nil {
		t.Fatalf("inserts=%d settleErr=%v", store.inserts, store.settleErr)
	}
}

func TestIdempotency_PanicReleasesACommittedKey(t *testing.T) {
	m := dbtest.NewSQLite(t).TX
	store := &fakeIdemStore{}
	defer func() {
		if recover() == nil {
			t.Fatal("the panic must keep propagating to Recover")
		}
		if store.deletes != 1 {
			t.Fatalf("deletes=%d, want 1", store.deletes)
		}
	}()
	serveIdem(t, idemAuthn{store: store}, http.MethodPost, "/api/v1/x", idemKey, func(w http.ResponseWriter, r *http.Request) {
		if err := m.WithTx(r.Context(), func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		panic("boom")
	})
}

func TestIdempotency_AccessLogKeepsTheHandlerError(t *testing.T) {
	store := &fakeIdemStore{}
	recorder := &errorRecordingWriter{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/x", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	want := &errs.ValidationError{Msg: "nope"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(r.Context(), w, want)
	})
	middleware.AuthWith(idemAuthn{store: store}, middleware.AuthOptions{})(next).ServeHTTP(recorder, req)
	if recorder.err != error(want) {
		t.Fatalf("recorded err = %v", recorder.err)
	}
}

type errorRecordingWriter struct {
	*httptest.ResponseRecorder
	err error
}

func (w *errorRecordingWriter) SetError(err error) { w.err = err }
