package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/fixture"
)

func TestIngest_BodyIsNeverLogged(t *testing.T) {
	h := newHarness(t)
	h.f.ImportAccountLink(fixture.ImportAccountLink{SourceID: source, ExternalAccountID: "Apple Card", AccountID: acct1})

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	const secretPayee = "Dr. Very Private Clinic"
	body := `{"account":"Apple Card","payee":"` + secretPayee + `","amount":"120.00","currency":"USD","eventId":"evt-secret"}`
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, h.srv.URL+"/api/v1/import/ingest-apple-wallet-event", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userA)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.StatusCode != 200 || !env.Success || env.Data.Status != "created" {
		t.Fatalf("status=%d success=%v ingest=%q", res.StatusCode, env.Success, env.Data.Status)
	}
	if h.txns.created != 1 {
		t.Fatalf("created=%d, want 1", h.txns.created)
	}

	out := logs.String()
	if strings.Contains(out, secretPayee) || strings.Contains(out, "evt-secret") || strings.Contains(out, "120.00") {
		t.Fatalf("payload leaked into logs:\n%s", out)
	}
	if !strings.Contains(out, "ingest_status=created") {
		t.Fatalf("operation line missing ingest_status:\n%s", out)
	}
}

func TestIngest_BodyTooLarge(t *testing.T) {
	h := newHarness(t)
	body := `{"account":"Apple Card","payee":"` + strings.Repeat("x", 70<<10) + `","amount":"1","currency":"USD"}`
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, h.srv.URL+"/api/v1/import/ingest-apple-wallet-event", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userA)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("status=%d, want 400", res.StatusCode)
	}
}
