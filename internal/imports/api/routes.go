package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/web/middleware"
	"github.com/econumo/econumo/internal/web/router"
)

func RegisterAPI(h *Handlers, authn middleware.TokenAuthenticator) router.RegisterAPI {
	return func(mux *http.ServeMux) {
		authMw := middleware.Auth(authn)
		auth := func(fn http.HandlerFunc) http.Handler { return authMw(fn) }

		mux.Handle("POST /api/v1/import/create-source", auth(h.CreateSource))
		mux.Handle("GET /api/v1/import/get-source-list", auth(h.GetSourceList))
		mux.Handle("POST /api/v1/import/delete-source", auth(h.DeleteSource))
		mux.Handle("POST /api/v1/import/link-account", auth(h.LinkAccount))
		mux.Handle("POST /api/v1/import/ignore-account", auth(h.IgnoreAccount))
		mux.Handle("POST /api/v1/import/unlink-account", auth(h.UnlinkAccount))
		// The only route an ingest-scoped PAT may call (middleware.IngestPathPrefix).
		mux.Handle("POST /api/v1/import/ingest-apple-wallet-event", auth(h.IngestAppleWalletEvent))
		mux.Handle("GET /api/v1/import/get-queued-event-list", auth(h.GetQueuedEventList))
		mux.Handle("POST /api/v1/import/import-queued-event", auth(h.ImportQueuedEvent))
		mux.Handle("POST /api/v1/import/skip-queued-event", auth(h.SkipQueuedEvent))
		mux.Handle("POST /api/v1/import/unskip-queued-event", auth(h.UnskipQueuedEvent))
		mux.Handle("POST /api/v1/import/retry-event", auth(h.RetryEvent))
		mux.Handle("POST /api/v1/import/discard-event", auth(h.DiscardEvent))
		mux.Handle("GET /api/v1/import/get-transaction-import-list", auth(h.GetTransactionImportList))
		mux.Handle("POST /api/v1/import/claim-setup-token", auth(h.ClaimSetupToken))
		mux.Handle("GET /api/v1/import/get-credential-key", auth(h.GetCredentialKey))
		mux.Handle("POST /api/v1/import/set-credential-key", auth(h.SetCredentialKey))
		// POST, not GET: the access URL rides in the body, never a query string.
		mux.Handle("POST /api/v1/import/list-external-accounts", auth(h.ListExternalAccounts))
		mux.Handle("POST /api/v1/import/sync-source", auth(h.SyncSource))
		mux.Handle("GET /api/v1/import/get-run-list", auth(h.GetRunList))
		mux.Handle("GET /api/v1/import/get-run", auth(h.GetRun))
		mux.Handle("GET /api/v1/import/get-rule-list", auth(h.GetRuleList))
		mux.Handle("POST /api/v1/import/create-rule", auth(h.CreateRule))
		mux.Handle("POST /api/v1/import/update-rule", auth(h.UpdateRule))
		mux.Handle("POST /api/v1/import/delete-rule", auth(h.DeleteRule))
		// POST, not GET: the whole rule spec travels in the body, like list-external-accounts.
		mux.Handle("POST /api/v1/import/preview-rule", auth(h.PreviewRule))
		mux.Handle("POST /api/v1/import/apply-rule", auth(h.ApplyRule))
		mux.Handle("POST /api/v1/import/suggest-rules", auth(h.SuggestRules))
	}
}
