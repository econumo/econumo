// Package endpoint holds the generic request/response combinators the feature
// api packages build their handlers from. A handler METHOD stays named (swag
// annotations live on it); its body delegates here. This package lives outside
// httpx because it needs middleware.RequireUser, and middleware already
// imports httpx.
package endpoint

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/httpx"
	"github.com/econumo/econumo/internal/web/middleware"
)

// Handle serves an authenticated JSON endpoint: require user, decode+validate
// Req, call, write the OK envelope. dev gates 500 stack traces exactly as the
// hand-written handlers did.
func Handle[Req any, Res any](w http.ResponseWriter, r *http.Request, dev bool,
	call func(ctx context.Context, userID vo.Id, req Req) (Res, error),
) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	var req Req
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(r.Context(), w, err, dev)
		return
	}
	warnNumericAmounts(r, &req)
	res, err := call(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err, dev)
		return
	}
	httpx.OK(w, res)
}

// HandleNoBody serves an authenticated endpoint with no request body.
func HandleNoBody[Res any](w http.ResponseWriter, r *http.Request, dev bool,
	call func(ctx context.Context, userID vo.Id) (Res, error),
) {
	userID, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}
	res, err := call(r.Context(), userID)
	if err != nil {
		httpx.WriteError(r.Context(), w, err, dev)
		return
	}
	httpx.OK(w, res)
}

// HandlePublic serves an unauthenticated JSON endpoint (register, remind,
// reset). No user gate; decode+validate, call, OK envelope.
func HandlePublic[Req any, Res any](w http.ResponseWriter, r *http.Request, dev bool,
	call func(ctx context.Context, req Req) (Res, error),
) {
	var req Req
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(r.Context(), w, err, dev)
		return
	}
	warnNumericAmounts(r, &req)
	res, err := call(r.Context(), req)
	if err != nil {
		httpx.WriteError(r.Context(), w, err, dev)
		return
	}
	httpx.OK(w, res)
}
