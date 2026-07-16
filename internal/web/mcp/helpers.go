package mcp

import (
	"context"
	"errors"
	"log/slog"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

// UserID returns the authenticated caller. /mcp sits behind the auth
// middleware, so absence is a programming error, not a client condition.
func UserID(ctx context.Context) (vo.Id, error) {
	id, ok := middleware.UserIDFromCtx(ctx)
	if !ok {
		slog.ErrorContext(ctx, "mcp user missing from context")
		return vo.Id{}, errors.New("Internal error")
	}
	return id, nil
}

// MapErr shapes a use-case error for the model: domain errors keep their
// message (typed SDK handlers turn any returned error into an isError tool
// result the model can read and self-correct on); everything else is
// infrastructure — logged here, replaced by a static message so no internals
// leak.
func MapErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.AsValidation(err); ok {
		return err
	}
	if _, ok := errs.AsNotFound(err); ok {
		return err
	}
	if _, ok := errs.AsAccessDenied(err); ok {
		return err
	}
	slog.ErrorContext(ctx, "mcp internal error", slog.Any("err", err))
	return errors.New("Internal error")
}
