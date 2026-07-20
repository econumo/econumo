package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/econumo/econumo/internal/infra/i18n"
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
		slog.ErrorContext(ctx, "mcp user missing from context")
		return vo.Id{}, structuredErr(ctx, errPayload{Message: "Internal error"})
	}
	return id, nil
}

// errPayload mirrors the client-facing fields of the REST error envelope
// (internal/web/httpx) so an MCP tool error carries the same signal the web
// SPA receives: message and per-field errors rendered in the caller's
// language, exactly as WriteError emits them.
type errPayload struct {
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors,omitempty"`
}

// formValidationErrorLiteral is the field-validation generic label. The
// catalogue defines no key for it (checked against locales/en.json), so it
// stays literal English — matching the REST envelope, where clients read the
// per-field errors map instead.
const formValidationErrorLiteral = "Form validation error"

// MapErr shapes a use-case error into MCP tool-error text. Domain errors are
// surfaced as a JSON object mirroring the REST error envelope: message and
// per-field errors rendered in the caller's language (reqctx.Language) via
// the shared i18n catalogue, keyed "errors."+code — so a model receives the
// same actionable signal the SPA does and can self-correct. Errors with no
// code (nothing in the catalogue to look up) keep their literal Go-side text
// unchanged. Anything else is infrastructure: logged here and replaced with a
// static message so no internals leak. Typed SDK handlers turn the returned
// error into an isError tool result whose text is this JSON.
func MapErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	lang := reqctx.Language(ctx)
	if v, ok := errs.AsValidation(err); ok {
		p := errPayload{Message: v.Msg}
		if v.MsgCode != "" {
			if msg, found := i18n.Lookup(lang, "errors."+v.MsgCode, v.MsgParams); found {
				p.Message = msg
			}
		}
		if len(v.Fields) > 0 {
			// Field-level validation mirrors the envelope: the generic label
			// plus the actionable per-field errors map.
			p.Message = formValidationErrorLiteral
			p.Errors = fieldsToMessages(lang, v.Fields)
		}
		return structuredErr(ctx, p)
	}
	if v, ok := errs.AsNotFound(err); ok {
		return structuredErr(ctx, errPayload{Message: v.Msg})
	}
	if v, ok := errs.AsAccessDenied(err); ok {
		return structuredErr(ctx, errPayload{Message: v.Msg})
	}
	slog.ErrorContext(ctx, "mcp internal error", slog.Any("err", err))
	return structuredErr(ctx, errPayload{Message: "Internal error"})
}

// fieldsToMessages renders each field entry in the caller's language; a field
// with no code — or a code absent from every catalogue — keeps its literal
// English message rather than leaking a dotted key.
func fieldsToMessages(lang string, fields []errs.FieldError) map[string][]string {
	out := make(map[string][]string, len(fields))
	for _, f := range fields {
		msg := f.Message
		if f.Code != "" {
			if translated, ok := i18n.Lookup(lang, "errors."+f.Code, f.Params); ok {
				msg = translated
			}
		}
		out[f.Key] = append(out[f.Key], msg)
	}
	return out
}

// structuredErr renders the payload as compact JSON (HTML escaping disabled, as
// everywhere on the wire) and returns it as an error, so the typed SDK handler
// surfaces it as the isError tool-result text. A marshal failure degrades to
// the plain message rather than leaking the encode error.
func structuredErr(ctx context.Context, p errPayload) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(p); err != nil {
		slog.ErrorContext(ctx, "mcp error payload marshal failed", slog.Any("err", err))
		return errors.New(p.Message)
	}
	return errors.New(strings.TrimRight(buf.String(), "\n"))
}
