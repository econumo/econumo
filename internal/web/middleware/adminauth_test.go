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
	h := AdminAuth(strings.Repeat("k", 32), "")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestAdminAuthShareToken(t *testing.T) {
	main, share := strings.Repeat("M", 32), strings.Repeat("S", 32)
	cases := []struct {
		name, shareCfg, method, path, bearer string
		want                                 int
	}{
		{"share on user-status GET", share, http.MethodGet, "/admin/user-status", share, 200},
		{"share on user-context", share, http.MethodGet, "/admin/user-context", share, 401},
		{"share on set-access", share, http.MethodPost, "/admin/set-access", share, 401},
		{"share on user-status POST", share, http.MethodPost, "/admin/user-status", share, 401},
		{"main on user-status", share, http.MethodGet, "/admin/user-status", main, 200},
		{"main on user-context", share, http.MethodGet, "/admin/user-context", main, 200},
		{"unset share, empty bearer", "", http.MethodGet, "/admin/user-status", "", 401},
		{"unset share, share-like bearer", "", http.MethodGet, "/admin/user-status", share, 401},
		{"wrong bearer", share, http.MethodGet, "/admin/user-status", strings.Repeat("x", 32), 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := AdminAuth(main, tc.shareCfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == 401 && !strings.Contains(rec.Body.String(), `"message":"Invalid access token"`) {
				t.Fatalf("body = %s", rec.Body.String())
			}
		})
	}
}
