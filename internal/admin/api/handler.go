// Package api is the admin listener's HTTP edge. These routes are registered on
// a separate mux served by a separate http.Server; they are never reachable on
// the public API mux (asserted by internal/test/apiparity).
package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/admin"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
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

const (
	defaultAffiliateLimit = 500
	maxAffiliateLimit     = 500
)

func (h *Handlers) AffiliateUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	program := q.Get("program")
	if program != "" && !model.ValidAffiliateProgram(program) {
		h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
			errs.FieldError{Key: "program", Message: "Invalid program"}))
		return
	}
	raw := q.Get("since")
	if raw == "" {
		h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
			errs.FieldError{Key: "since", Message: "This value should not be blank."}))
		return
	}
	since, err := time.ParseInLocation(datetime.Layout, raw, time.UTC)
	if err != nil {
		h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
			errs.FieldError{Key: "since", Message: "Invalid date"}))
		return
	}
	limit := defaultAffiliateLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxAffiliateLimit {
			h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
				errs.FieldError{Key: "limit", Message: "This value should be between 1 and 500."}))
			return
		}
		limit = n
	}
	after := q.Get("afterUserId")
	if after != "" {
		id, err := vo.ParseId(after)
		if err != nil {
			h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
				errs.FieldError{Key: "afterUserId", Message: "Invalid user id"}))
			return
		}
		after = id.String()
	}
	res, err := h.svc.AffiliateUsers(r.Context(), program, since, after, limit)
	if err != nil {
		h.writeErr(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}

const (
	defaultDeletedLimit = 500
	maxDeletedLimit     = 500
)

// DeletedUsers lets billing poll wiped users, oldest first, by the cursor
// (since, afterId): pass the last row's deletedAt and id to read the next page.
// Wipes are serialized and each writes its deleted row last, so on one server
// rows become visible in cursor order. A caller should still re-read a short
// overlap (start a little before the last deletedAt) and skip ids it already
// processed: a clock set back, or app servers whose clocks differ, can make a
// row appear behind the cursor.
func (h *Handlers) DeletedUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	raw := q.Get("since")
	if raw == "" {
		h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
			errs.FieldError{Key: "since", Message: "This value should not be blank."}))
		return
	}
	since, err := time.ParseInLocation(datetime.Layout, raw, time.UTC)
	if err != nil {
		h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
			errs.FieldError{Key: "since", Message: "Invalid date"}))
		return
	}
	limit := defaultDeletedLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxDeletedLimit {
			h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
				errs.FieldError{Key: "limit", Message: "This value should be between 1 and 500."}))
			return
		}
		limit = n
	}
	after := q.Get("afterId")
	if after != "" {
		id, err := vo.ParseId(after)
		if err != nil {
			h.writeErr(r.Context(), w, errs.NewValidation("Form validation error",
				errs.FieldError{Key: "afterId", Message: "Invalid id"}))
			return
		}
		after = id.String()
	}
	res, err := h.svc.DeletedUsers(r.Context(), since, after, limit)
	if err != nil {
		h.writeErr(r.Context(), w, err)
		return
	}
	httpx.OK(w, res)
}
