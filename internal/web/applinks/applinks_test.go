package applinks_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/web/applinks"
)

func serve(t *testing.T, cfg config.Config, path string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	applinks.Handler(cfg).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr
}

func TestAASA(t *testing.T) {
	rr := serve(t, config.Config{AppLinksIOSAppIDs: []string{"ABCDE12345.com.econumo.app"}}, applinks.AASAPath)
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q", got)
	}
	want := `{"applinks":{"details":[{"appIDs":["ABCDE12345.com.econumo.app"],"components":[{"/":"/oauth/app-return","comment":"OAuth return to the app"}]}]}}`
	if strings.TrimSpace(rr.Body.String()) != want {
		t.Fatalf("got %s", rr.Body.String())
	}
}

func TestAssetLinks(t *testing.T) {
	cfg := config.Config{AppLinksAndroid: []config.AndroidAppLink{{Package: "com.econumo.app", Fingerprints: []string{"AA:BB"}}}}
	rr := serve(t, cfg, applinks.AssetLinksPath)
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q", got)
	}
	want := `[{"relation":["delegate_permission/common.handle_all_urls"],"target":{"namespace":"android_app","package_name":"com.econumo.app","sha256_cert_fingerprints":["AA:BB"]}}]`
	if strings.TrimSpace(rr.Body.String()) != want {
		t.Fatalf("got %s", rr.Body.String())
	}
}

// A platform with no configured app is not associated with this domain, so its
// document must be absent rather than an empty list.
func TestPlatformWithoutAppsIs404(t *testing.T) {
	ios := config.Config{AppLinksIOSAppIDs: []string{"ABCDE12345.com.econumo.app"}}
	if rr := serve(t, ios, applinks.AssetLinksPath); rr.Code != http.StatusNotFound {
		t.Fatalf("assetlinks.json with no android app = %d, want 404", rr.Code)
	}
	android := config.Config{AppLinksAndroid: []config.AndroidAppLink{{Package: "com.econumo.app", Fingerprints: []string{"AA:BB"}}}}
	if rr := serve(t, android, applinks.AASAPath); rr.Code != http.StatusNotFound {
		t.Fatalf("AASA with no ios app = %d, want 404", rr.Code)
	}
}

func TestMultipleAndroidPackages(t *testing.T) {
	cfg := config.Config{AppLinksAndroid: []config.AndroidAppLink{
		{Package: "com.econumo.app", Fingerprints: []string{"AA:BB", "CC:DD"}},
		{Package: "com.econumo.app.dev", Fingerprints: []string{"EE:FF"}},
	}}
	rr := serve(t, cfg, applinks.AssetLinksPath)
	want := `[{"relation":["delegate_permission/common.handle_all_urls"],"target":{"namespace":"android_app","package_name":"com.econumo.app","sha256_cert_fingerprints":["AA:BB","CC:DD"]}},` +
		`{"relation":["delegate_permission/common.handle_all_urls"],"target":{"namespace":"android_app","package_name":"com.econumo.app.dev","sha256_cert_fingerprints":["EE:FF"]}}]`
	if strings.TrimSpace(rr.Body.String()) != want {
		t.Fatalf("got %s", rr.Body.String())
	}
}
