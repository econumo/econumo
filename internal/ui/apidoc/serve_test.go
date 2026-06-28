package apidoc_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/ui/apidoc"
	"github.com/econumo/econumo/internal/ui/router"
)

// TestSwaggerRoutes confirms the spec is served publicly (no JWT) and is valid
// JSON, and that the UI routes are reachable — through the REAL router with the
// global middleware chain applied.
func TestSwaggerRoutes(t *testing.T) {
	h := router.New(router.Deps{
		Cfg:         config.Config{CORSAllowedOrigins: []string{"*"}},
		RegisterAPI: apidoc.RegisterAPI(),
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	// /api/doc.json: public, 200, valid OpenAPI JSON.
	resp, err := http.Get(srv.URL + "/api/doc.json")
	if err != nil {
		t.Fatalf("GET /api/doc.json: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/doc.json status = %d, want 200", resp.StatusCode)
	}
	var spec map[string]any
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("/api/doc.json is not valid JSON: %v", err)
	}
	if spec["swagger"] != "2.0" {
		t.Fatalf("spec swagger = %v, want 2.0", spec["swagger"])
	}
	paths, _ := spec["paths"].(map[string]any)
	if _, ok := paths["/api/v1/category/create-category"]; !ok {
		t.Fatalf("spec missing /api/v1/category/create-category; keys: %v", keys(paths))
	}

	// /api/doc redirects into the UI.
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r2, err := c.Get(srv.URL + "/api/doc")
	if err != nil {
		t.Fatalf("GET /api/doc: %v", err)
	}
	r2.Body.Close()
	if r2.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("/api/doc status = %d, want 301", r2.StatusCode)
	}

	// /api/doc/ serves the Swagger UI (non-empty body, 200).
	r3, err := http.Get(srv.URL + "/api/doc/")
	if err != nil {
		t.Fatalf("GET /api/doc/: %v", err)
	}
	ui, _ := io.ReadAll(r3.Body)
	r3.Body.Close()
	if r3.StatusCode != http.StatusOK || len(ui) == 0 {
		t.Fatalf("/api/doc/ status = %d, len = %d", r3.StatusCode, len(ui))
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
