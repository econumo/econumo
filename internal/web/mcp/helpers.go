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
// SPA receives: a message plus the machine-readable code(s). Unlike REST
// (frozen English, translated client-side), MapErr renders message text in
// the caller's language server-side, since MCP clients are LLMs with no
// catalogue of their own; codes stay for machine use either way. Field
// validations populate errors/errorCodes exactly as the envelope's
// errors{}/errorCodes{} maps do.
type errPayload struct {
	Message       string               `json:"message"`
	MessageCode   string               `json:"messageCode,omitempty"`
	MessageParams map[string]any       `json:"messageParams,omitempty"`
	Errors        map[string][]string  `json:"errors,omitempty"`
	ErrorCodes    map[string][]codeRef `json:"errorCodes,omitempty"`
}

type codeRef struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// formValidationErrorLiteral is the field-validation generic label. The
// catalogue defines no key for it (checked against locales/en.json), so it
// stays frozen English rather than inventing a key; i18n.T's missing-key
// fallback (return the key verbatim) would otherwise leak a dotted key into
// the message.
const formValidationErrorLiteral = "Form validation error"

// MapErr shapes a use-case error into MCP tool-error text. Domain errors are
// surfaced as a JSON object mirroring the REST error envelope (message plus
// code(s)), so a model receives the same actionable signal the SPA does and
// can self-correct. Message text is rendered in the caller's language
// (reqctx.Language) via the shared i18n catalogue, keyed "errors."+code — a
// deliberate MCP-only divergence from REST, which keeps frozen English and
// leaves translation to the SPA. Errors with no code (nothing in the
// catalogue to look up) keep their literal Go-side text unchanged. Anything
// else is infrastructure: logged here and replaced with a static message so
// no internals leak. Typed SDK handlers turn the returned error into an
// isError tool result whose text is this JSON.
func MapErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	lang := reqctx.Language(ctx)
	if v, ok := errs.AsValidation(err); ok {
		p := errPayload{Message: v.Msg, MessageCode: v.MsgCode, MessageParams: v.MsgParams}
		if v.MsgCode != "" {
			p.Message = i18n.T(lang, "errors."+v.MsgCode, v.MsgParams)
		}
		if len(v.Fields) > 0 {
			// Field-level validation mirrors the envelope: the generic label
			// plus the actionable per-field errors/errorCodes maps.
			p.Message = formValidationErrorLiteral
			p.Errors = fieldsToMessages(lang, v.Fields)
			p.ErrorCodes = fieldsToCodes(v.Fields)
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

func fieldsToMessages(lang string, fields []errs.FieldError) map[string][]string {
	out := make(map[string][]string, len(fields))
	for _, f := range fields {
		msg := f.Message
		if f.Code != "" {
			msg = i18n.T(lang, "errors."+f.Code, f.Params)
		}
		out[f.Key] = append(out[f.Key], msg)
	}
	return out
}

func fieldsToCodes(fields []errs.FieldError) map[string][]codeRef {
	var out map[string][]codeRef
	for _, f := range fields {
		if f.Code == "" {
			continue
		}
		if out == nil {
			out = map[string][]codeRef{}
		}
		out[f.Key] = append(out[f.Key], codeRef{Code: f.Code, Params: f.Params})
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
