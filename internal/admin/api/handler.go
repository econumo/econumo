// Package api is the admin listener's HTTP edge. These routes are registered on
// a separate mux served by a separate http.Server; they are never reachable on
// the public API mux (asserted by internal/test/apiparity).
package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/admin"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/httpx"
)

type Handlers struct {
	svc *admin.Service
}

func NewHandlers(svc *admin.Service) *Handlers {
	return &Handlers{svc: svc}
}

// writeErr maps a not-found to a real 404 before delegating. The public API
// collapses not-found into 400 (frozen wire contract), but this listener is
// private and single-consumer, and its consumer is a machine: "no such user,
// stop retrying" and "your request was malformed" call for different handling,
// so they get different statuses here. The admin chain runs no Language
// middleware, so WriteError renders the frozen English strings.
func (h *Handlers) writeErr(ctx context.Context, w http.ResponseWriter, err error) {
	if nf, ok := errs.AsNotFound(err); ok {
		httpx.Err(w, nf.Error(), 0, nil, http.StatusNotFound)
		return
	}
	httpx.WriteError(ctx, w, err)
}

func (h *Handlers) SetAccess(w http.ResponseWriter, r *http.Request) {
	var req model.AdminSetAccessRequest
	// Decode, not DecodeValidate: SetAccess calls Validate itself, and running
	// it twice would report the same failure from two places.
	if err := httpx.Decode(r, &req); err != nil {
		h.writeErr(r.Context(), w, err)
		return
	}
	res, err := h.svc.SetAccess(r.Context(), req)
	if err != nil {
		h.writeErr(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

func (h *Handlers) UserContext(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("userId")
	if raw == "" {
		h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
			errs.FieldError{Key: "userId", Message: "This value should not be blank."}))
		return
	}
	id, err := vo.ParseId(raw)
	if err != nil {
		h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
			errs.FieldError{Key: "userId", Message: "Invalid user id"}))
		return
	}
	res, err := h.svc.UserContext(r.Context(), id)
	if err != nil {
		h.writeErr(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
