package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// maxIngestBody caps one shortcut push; a real Apple Wallet event is < 1 KB.
const maxIngestBody = 64 << 10

// IngestAppleWalletEvent handles POST /api/v1/import/ingest-apple-wallet-event (auth; ingest- or full-scope token).
//
// @Summary     Push an Apple Wallet transaction event
// @Description Accepts one raw Apple Wallet event as pushed by the "econumo-wallet-v1" shortcut, stores it, and processes it synchronously: created (a transaction was written), duplicate (already seen), queued (card unmapped / no rate / account deleted), skipped (card ignored) or failed (unparsable payload). The body is never logged.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     object true "Raw Apple Wallet event: {account, payee, amount, currency, occurredAt?, type?, eventId?}"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.IngestEventResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/ingest-apple-wallet-event [post]
func (h *Handlers) IngestAppleWalletEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxIngestBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(ctx, w, errs.NewValidation("Request body is too large"))
			return
		}
		httpx.WriteError(ctx, w, errs.NewValidation("Could not read request body"))
		return
	}
	res, err := h.svc.IngestAppleWallet(ctx, userID, body)
	if err != nil {
		httpx.WriteError(ctx, w, err)
		return
	}
	reqctx.AddLogAttr(ctx, "ingest_status", res.Status)
	httpx.OK(w, res)
}
