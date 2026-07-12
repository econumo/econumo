package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/middleware"
)

// UserID returns the authenticated caller. /mcp sits behind the auth
// middleware, so absence is a programming error, not a client condition.
func UserID(ctx context.Context) (vo.Id, error) {
	id, ok := middleware.UserIDFromCtx(ctx)
	if !ok {
		return vo.Id{}, errors.New("Internal error")
	}
	return id, nil
}

// JSONText marshals for MCP payloads with the same HTML-escaping-off policy
// as the REST envelope.
func JSONText(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
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

// AddJSONResource registers a per-user JSON resource with the shared
// load-marshal-wrap plumbing.
func AddJSONResource[T any](s *sdk.Server, uri, name, description string, load func(ctx context.Context, userID vo.Id) (T, error)) {
	s.AddResource(
		&sdk.Resource{URI: uri, Name: name, Description: description, MIMEType: "application/json"},
		func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			reqctx.AddLogAttr(ctx, "resource", uri)
			userID, err := UserID(ctx)
			if err != nil {
				return nil, err
			}
			v, err := load(ctx, userID)
			if err != nil {
				return nil, MapErr(ctx, err)
			}
			text, err := JSONText(v)
			if err != nil {
				return nil, MapErr(ctx, err)
			}
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{
				{URI: req.Params.URI, MIMEType: "application/json", Text: text},
			}}, nil
		})
}
