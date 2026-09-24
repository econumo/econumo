package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/infra/oidc"
	"github.com/econumo/econumo/internal/infra/oidc/oidctest"
	"github.com/econumo/econumo/internal/oauth"
)

// captureLogs redirects the default logger for the duration of the test, which
// is where probeProviders writes (it reports, it never returns anything).
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func TestProbeProviders_WarnsPerUnreachableProvider(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	live := oidctest.New(t)

	buf := captureLogs(t)
	probeProviders(context.Background(), []oauth.Provider{
		{Name: "Dead", Client: oidc.NewClient(oidc.Issuer{ID: "dead", IssuerURL: deadURL}, http.DefaultClient)},
		{Name: "Live", Client: oidc.NewClient(live.Issuer("oidc", false), live.Server.Client())},
	})

	out := buf.String()
	if got := strings.Count(out, "oauth provider discovery failed at boot"); got != 1 {
		t.Fatalf("want exactly one warning, got %d: %s", got, out)
	}
	if !strings.Contains(out, "provider=dead") {
		t.Fatalf("warning does not name the unreachable provider: %s", out)
	}
}

func TestProbeProviders_ReachableProviderIsSilent(t *testing.T) {
	live := oidctest.New(t)
	buf := captureLogs(t)
	probeProviders(context.Background(), []oauth.Provider{
		{Name: "Live", Client: oidc.NewClient(live.Issuer("oidc", false), live.Server.Client())},
	})
	if buf.Len() != 0 {
		t.Fatalf("reachable provider logged: %s", buf)
	}
}
