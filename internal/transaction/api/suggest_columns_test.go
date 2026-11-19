package api_test

import (
	"net/http"
	"net/url"
	"testing"
)

const suggestColumnsPath = "/api/v1/transaction/suggest-import-columns"

func TestSuggestImportColumnsWithoutAIAnswersEmptyRoles(t *testing.T) {
	h := newHarness(t)
	columns := url.QueryEscape(`[{"name":"Date","type":"date","negatives":false},{"name":"Amount","type":"number","negatives":true}]`)
	status, env := h.do(t, http.MethodGet, suggestColumnsPath+"?columns="+columns, h.token(t), nil)
	if status != http.StatusOK || !env.Success {
		t.Fatalf("status = %d, body = %s", status, env.raw)
	}
	if string(env.Data) != `{"roles":{}}` {
		t.Fatalf("data = %s", env.Data)
	}
}

func TestSuggestImportColumnsRefusesBadColumns(t *testing.T) {
	h := newHarness(t)
	for name, query := range map[string]string{
		"malformed": "?columns=" + url.QueryEscape(`[{"name":`),
		"missing":   "",
		"bad_type":  "?columns=" + url.QueryEscape(`[{"name":"Amount","type":"money"}]`),
	} {
		t.Run(name, func(t *testing.T) {
			status, env := h.do(t, http.MethodGet, suggestColumnsPath+query, h.token(t), nil)
			if status != http.StatusBadRequest || env.Success {
				t.Fatalf("status = %d, body = %s", status, env.raw)
			}
			if len(env.errorsMap()["columns"]) == 0 {
				t.Fatalf("errors = %s", env.Errors)
			}
		})
	}
}

func TestSuggestImportColumnsRequiresToken(t *testing.T) {
	h := newHarness(t)
	status, env := h.do(t, http.MethodGet, suggestColumnsPath+"?columns=%5B%5D", "", nil)
	if status != http.StatusUnauthorized || env.Message != "Access token not found" {
		t.Fatalf("status = %d, body = %s", status, env.raw)
	}
}
