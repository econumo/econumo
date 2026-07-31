package endpoint

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
)

type numericAmountReq struct {
	Balance vo.FlexString  `json:"balance"`
	Amount  *vo.FlexString `json:"amount"`
	Name    string         `json:"name"`
}

func captureWarns(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestWarnNumericAmounts_NumberFields(t *testing.T) {
	buf := captureWarns(t)
	var req numericAmountReq
	if err := json.Unmarshal([]byte(`{"balance": 100.5, "amount": 9.99, "name": "x"}`), &req); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/account/create-account", nil)
	warnNumericAmounts(r, &req)

	out := buf.String()
	if !strings.Contains(out, "deprecated numeric amount") {
		t.Fatalf("expected the WARN line, got: %s", out)
	}
	if !strings.Contains(out, "fields=balance,amount") {
		t.Errorf("expected fields=balance,amount, got: %s", out)
	}
	if strings.Contains(out, "100.5") || strings.Contains(out, "9.99") {
		t.Errorf("amount VALUES must never be logged: %s", out)
	}
}

func TestWarnNumericAmounts_SilentForStrings(t *testing.T) {
	buf := captureWarns(t)
	var req numericAmountReq
	if err := json.Unmarshal([]byte(`{"balance": "100.5", "name": "x"}`), &req); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/account/create-account", nil)
	warnNumericAmounts(r, &req)
	if buf.Len() != 0 {
		t.Errorf("string amounts must not warn, got: %s", buf.String())
	}
}
