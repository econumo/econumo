package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func decodeErrPayload(t *testing.T, err error) map[string]any {
	t.Helper()
	if err == nil {
		t.Fatal("MapErr returned nil")
	}
	var out map[string]any
	if uerr := json.Unmarshal([]byte(err.Error()), &out); uerr != nil {
		t.Fatalf("payload not JSON: %v (%q)", uerr, err.Error())
	}
	return out
}

func TestMapErrTranslatesCodedMessageToCallerLanguage(t *testing.T) {
	code := errs.CodeTransactionItemNotAvailable
	domainErr := &errs.ValidationError{Msg: "Transaction is not available", MsgCode: code}

	enCtx := reqctx.WithLanguage(context.Background(), "en")
	enPayload := decodeErrPayload(t, webmcp.MapErr(enCtx, domainErr))
	wantEn := i18n.T("en", "errors."+code, nil)
	if enPayload["message"] != wantEn {
		t.Fatalf("en message = %v, want %q", enPayload["message"], wantEn)
	}

	ruCtx := reqctx.WithLanguage(context.Background(), "ru")
	ruPayload := decodeErrPayload(t, webmcp.MapErr(ruCtx, domainErr))
	wantRu := i18n.T("ru", "errors."+code, nil)
	if wantRu == wantEn {
		t.Fatalf("ru catalogue text for %q equals en text; test cannot distinguish translation", code)
	}
	if ruPayload["message"] != wantRu {
		t.Fatalf("ru message = %v, want %q", ruPayload["message"], wantRu)
	}
}

func TestMapErrKeepsLiteralMessageWhenNoCode(t *testing.T) {
	domainErr := errs.NewValidation("month must be YYYY-MM")
	ruCtx := reqctx.WithLanguage(context.Background(), "ru")
	payload := decodeErrPayload(t, webmcp.MapErr(ruCtx, domainErr))
	if payload["message"] != "month must be YYYY-MM" {
		t.Fatalf("message = %v, want literal unchanged", payload["message"])
	}
}

func TestMapErrKeepsLiteralMessageWhenCodeUnknown(t *testing.T) {
	domainErr := &errs.ValidationError{Msg: "Something is wrong", MsgCode: "no.such.code"}
	ruCtx := reqctx.WithLanguage(context.Background(), "ru")
	payload := decodeErrPayload(t, webmcp.MapErr(ruCtx, domainErr))
	if payload["message"] != "Something is wrong" {
		t.Fatalf("message = %v, want literal (never the dotted key)", payload["message"])
	}
}

func TestMapErrTranslatesFieldMessages(t *testing.T) {
	fieldErr := errs.NewValidation("", errs.FieldError{
		Key:     "name",
		Message: "This value should not be blank.",
		Code:    "common.is_blank",
	})
	ruCtx := reqctx.WithLanguage(context.Background(), "ru")
	payload := decodeErrPayload(t, webmcp.MapErr(ruCtx, fieldErr))
	if payload["message"] != "Form validation error" {
		t.Fatalf("top-level message = %v, want literal (no catalogue key exists)", payload["message"])
	}
	errsMap, ok := payload["errors"].(map[string]any)
	if !ok {
		t.Fatalf("errors field missing or wrong shape: %v", payload["errors"])
	}
	got, ok := errsMap["name"].([]any)
	wantRu := i18n.T("ru", "errors.common.is_blank", nil)
	if !ok || len(got) != 1 || got[0] != wantRu {
		t.Fatalf("errors[name] = %v, want [%q]", errsMap["name"], wantRu)
	}
}

func TestMapErrNotFoundKeepsMsgUnchanged(t *testing.T) {
	ruCtx := reqctx.WithLanguage(context.Background(), "ru")
	payload := decodeErrPayload(t, webmcp.MapErr(ruCtx, errs.NewNotFound("Category not found")))
	if payload["message"] != "Category not found" {
		t.Fatalf("message = %v, want literal unchanged", payload["message"])
	}
}
