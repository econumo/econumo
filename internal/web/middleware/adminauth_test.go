package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func adminRequest(t *testing.T, header string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	ran := false
	h := AdminAuth(strings.Repeat("k", 32))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/admin/user-context", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, ran
}

func TestAdminAuthAcceptsValidToken(t *testing.T) {
	rec, ran := adminRequest(t, "Bearer "+strings.Repeat("k", 32))
	if !ran || rec.Code != http.StatusOK {
		t.Fatalf("status %d ran %v", rec.Code, ran)
	}
}

func TestAdminAuthAcceptsCaseInsensitiveScheme(t *testing.T) {
	rec, ran := adminRequest(t, "bearer "+strings.Repeat("k", 32))
	if !ran || rec.Code != http.StatusOK {
		t.Fatalf("status %d ran %v", rec.Code, ran)
	}
}

func TestAdminAuthRejects(t *testing.T) {
	cases := map[string]string{
		"missing":      "",
		"wrong token":  "Bearer " + strings.Repeat("x", 32),
		"short token":  "Bearer k",
		"wrong scheme": "Basic " + strings.Repeat("k", 32),
		"empty bearer": "Bearer ",
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			rec, ran := adminRequest(t, header)
			if ran {
				t.Fatal("handler ran despite failed auth")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}
