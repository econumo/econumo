package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/httpx"
)

const idempotencyKeyHeader = "Idempotency-Key"

// IdempotencyStore keeps one row per (user, key).
type IdempotencyStore interface {
	Find(ctx context.Context, userID vo.Id, key string) (resourceID string, found bool, err error)
	Insert(ctx context.Context, userID vo.Id, key, resourceID string) error
	Delete(ctx context.Context, userID vo.Id, key string) error
	MaybePurge(ctx context.Context)
}

// IdempotencyProvider is an optional capability of the wired
// TokenAuthenticator; without it no request is guarded.
type IdempotencyProvider interface {
	IdempotencyStore() IdempotencyStore
}

// idempotencyGuard returns the store and canonical key when the request is
// guarded. An absent or malformed key means no guard, so clients that do not
// send one keep their existing behaviour.
func idempotencyGuard(authn TokenAuthenticator, r *http.Request, scope model.TokenScope) (IdempotencyStore, string, bool) {
	if r.Method != http.MethodPost || r.URL.Path == MCPPath || scope != model.TokenScopeFull {
		return nil, "", false
	}
	provider, ok := authn.(IdempotencyProvider)
	if !ok {
		return nil, "", false
	}
	store := provider.IdempotencyStore()
	if store == nil {
		return nil, "", false
	}
	key, err := uuid.Parse(r.Header.Get(idempotencyKeyHeader))
	if err != nil {
		return nil, "", false
	}
	return store, key.String(), true
}

// serveIdempotent refuses a request whose key was already used, before any
// lookup or access check: the replay of a delete that landed would otherwise
// answer "not found". The key is written inside the handler's own transaction
// so the two commit together. A non-2xx releases it so the operation can be sent
// again, unless the handler pinned it: the write is in, and repeating it would
// duplicate it.
func serveIdempotent(w http.ResponseWriter, r *http.Request, next http.Handler, store IdempotencyStore, userID vo.Id, key string) {
	ctx := r.Context()
	resourceID, found, err := store.Find(ctx, userID, key)
	if err != nil {
		slog.WarnContext(ctx, "find idempotency key", slog.Any("err", err))
		next.ServeHTTP(w, r)
		return
	}
	if found {
		httpx.WriteReplay(ctx, w, resourceID)
		return
	}

	guarded := reqctx.WithIdempotencyResource(ctx)
	guarded = backend.WithCommitHook(guarded, func(txCtx context.Context) error {
		return store.Insert(txCtx, userID, key, reqctx.IdempotencyResource(txCtx))
	})
	sw := &idempotencyStatusWriter{ResponseWriter: w, status: http.StatusOK}
	served := false
	// A deferred call so a panic releases the key too; Recover, further out,
	// still answers it. Detached from cancellation because a client that dropped
	// the connection after a successful write is exactly the one that retries.
	defer func() {
		settle := context.WithoutCancel(guarded)
		succeeded := served && sw.status >= 200 && sw.status < 300
		recorded := backend.CommitHookDone(guarded)
		switch {
		case succeeded && !recorded:
			if err := store.Insert(settle, userID, key, reqctx.IdempotencyResource(guarded)); err != nil {
				slog.WarnContext(settle, "record idempotency key", slog.Any("err", err))
			}
		case !succeeded && recorded && !reqctx.IdempotencyPinned(guarded):
			if err := store.Delete(settle, userID, key); err != nil {
				slog.WarnContext(settle, "release idempotency key", slog.Any("err", err))
			}
		}
		if succeeded {
			store.MaybePurge(settle)
		}
	}()
	next.ServeHTTP(sw, r.WithContext(guarded))
	served = true
}

// idempotencyStatusWriter captures the response status. It forwards SetError
// because httpx type-asserts the writer it is handed, and hiding that method
// would strip the error from the access log.
type idempotencyStatusWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *idempotencyStatusWriter) WriteHeader(code int) {
	if !w.written {
		w.status = code
		w.written = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *idempotencyStatusWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}

func (w *idempotencyStatusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *idempotencyStatusWriter) SetError(err error) {
	if r, ok := w.ResponseWriter.(interface{ SetError(error) }); ok {
		r.SetError(err)
	}
}

func (w *idempotencyStatusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
