package router_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/web/router"
)

// newServer builds a router with a temp SPA dir (containing index.html) and an
// optional API registration func.
func newServer(t *testing.T, reg router.RegisterAPI) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>spa</title>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "favicon.ico"), []byte("icon"), 0o644); err != nil {
		t.Fatalf("write favicon: %v", err)
	}
	h := router.New(router.Deps{
		Cfg:         config.Config{CORSAllowedOrigins: []string{"*"}, SPADir: dir},
		RegisterAPI: reg,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, method, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func TestHealthCheck_OK(t *testing.T) {
	srv := newServer(t, nil)
	resp := get(t, srv, http.MethodGet, "/health")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
	var env struct {
		Success bool            `json:"success"`
		Data    map[string]bool `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&env)
	if !env.Success || !env.Data["database"] {
		t.Fatalf("health envelope=%+v want success + database:true (nil Pinger)", env)
	}
}

func TestUnknownApiPath_404(t *testing.T) {
	// An /api/ route that no module registered. The API mux has no matching
	// pattern, so net/http's ServeMux returns 404.
	reg := func(mux *http.ServeMux) {
		mux.HandleFunc("POST /api/v1/known", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
	srv := newServer(t, reg)
	resp := get(t, srv, http.MethodGet, "/api/v1/does-not-exist")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown api path status=%d want 404", resp.StatusCode)
	}
}

func TestWrongMethodOnKnownPath_405(t *testing.T) {
	reg := func(mux *http.ServeMux) {
		mux.HandleFunc("POST /api/v1/known", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
	srv := newServer(t, reg)
	// GET on a path registered only for POST -> ServeMux yields 405.
	resp := get(t, srv, http.MethodGet, "/api/v1/known")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status=%d want 405", resp.StatusCode)
	}
}

func TestRegisteredApiRoute_Reached(t *testing.T) {
	var hit bool
	reg := func(mux *http.ServeMux) {
		mux.HandleFunc("POST /api/v1/known", func(w http.ResponseWriter, r *http.Request) {
			hit = true
			w.WriteHeader(http.StatusOK)
		})
	}
	srv := newServer(t, reg)
	resp := get(t, srv, http.MethodPost, "/api/v1/known")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !hit {
		t.Fatalf("registered route: status=%d hit=%v want 200/true", resp.StatusCode, hit)
	}
}

func TestSPAFallback_ServesIndex(t *testing.T) {
	srv := newServer(t, nil)
	// A client-side route that has no file on disk -> SPA index.html fallback.
	resp := get(t, srv, http.MethodGet, "/accounts/123")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("spa fallback status=%d want 200", resp.StatusCode)
	}
}

func TestSPA_ServesExistingFile(t *testing.T) {
	srv := newServer(t, nil)
	resp := get(t, srv, http.MethodGet, "/favicon.ico")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("static file status=%d want 200", resp.StatusCode)
	}
}

func TestComposeRegisterAPI_RunsAll(t *testing.T) {
	var a, b bool
	reg := router.Compose(
		func(mux *http.ServeMux) {
			mux.HandleFunc("GET /api/a", func(w http.ResponseWriter, r *http.Request) { a = true })
		},
		nil, // a nil entry must be skipped without panicking
		func(mux *http.ServeMux) {
			mux.HandleFunc("GET /api/b", func(w http.ResponseWriter, r *http.Request) { b = true })
		},
	)
	srv := newServer(t, reg)
	get(t, srv, http.MethodGet, "/api/a").Body.Close()
	get(t, srv, http.MethodGet, "/api/b").Body.Close()
	if !a || !b {
		t.Fatalf("compose did not run both fns: a=%v b=%v", a, b)
	}
}

func TestRouter_MountsMCP(t *testing.T) {
	var hit bool
	h := router.New(router.Deps{Cfg: config.Config{}, MCP: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	})})
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !hit {
		t.Fatal("POST /mcp did not reach the MCP handler")
	}
	// Without a handler the SPA fallback answers; just assert no panic.
	h = router.New(router.Deps{Cfg: config.Config{}})
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mcp", nil))
}

// stubPinger lets the health-check exercise the db-down branch.
type stubPinger struct{ err error }

func (s stubPinger) Ping(ctx context.Context) error { return s.err }

func TestHealthCheck_DBDown_ReportsFalse(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644)
	h := router.New(router.Deps{
		Cfg: config.Config{SPADir: dir},
		DB:  stubPinger{err: context.DeadlineExceeded},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp := get(t, srv, http.MethodGet, "/health")
	defer resp.Body.Close()
	var env struct {
		Data map[string]bool `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&env)
	if env.Data["database"] {
		t.Fatalf("database=true want false when Ping errors")
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRuntimeConfigOverrides(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "econumo-config.js"), []byte("window.econumoConfig={};"), 0o644); err != nil {
		t.Fatal(err)
	}
	allowCustom := false
	h := router.New(router.Deps{Cfg: config.Config{
		SPADir:         dir,
		Analytics:      true,
		APIURL:         "https://api.example.test",
		AllowCustomAPI: &allowCustom,
	}})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := get(t, srv, http.MethodGet, "/econumo-config.js")
	defer resp.Body.Close()
	body := readBody(t, resp)
	want := `Object.assign(window.econumoConfig, {"ALLOW_CUSTOM_API":false,"ALLOW_REGISTRATION":false,"ANALYTICS":true,"API_URL":"https://api.example.test"});`
	if !strings.Contains(body, want) {
		t.Fatalf("config body missing %q:\n%s", want, body)
	}
}

func TestRuntimeConfigOverrides_UnsetKeysNotMerged(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "econumo-config.js"), []byte("window.econumoConfig={};"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := router.New(router.Deps{Cfg: config.Config{SPADir: dir, Analytics: true}})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := get(t, srv, http.MethodGet, "/econumo-config.js")
	defer resp.Body.Close()
	body := readBody(t, resp)
	for _, absent := range []string{"API_URL", "ALLOW_CUSTOM_API"} {
		if strings.Contains(body, absent) {
			t.Fatalf("unset key %q must not be merged:\n%s", absent, body)
		}
	}
	if !strings.Contains(body, `"ANALYTICS":true`) {
		t.Fatalf("ANALYTICS missing:\n%s", body)
	}
}
