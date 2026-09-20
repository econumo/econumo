// Package applinks serves the two platform association documents that let the
// mobile app claim <ECONUMO_URL>/oauth/app-return as a verified Universal Link
// (iOS) / App Link (Android): Apple's apple-app-site-association and Google's
// assetlinks.json, both at the fixed /.well-known/ paths their operating
// systems fetch. Only an app whose Team ID + bundle id (Apple) or package +
// signing certificate (Android) appears here may receive that URL, which is
// what makes the OAuth app return unspoofable by a look-alike app.
package applinks

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/econumo/econumo/internal/config"
)

const (
	AASAPath       = "/.well-known/apple-app-site-association"
	AssetLinksPath = "/.well-known/assetlinks.json"

	// appReturnPath is the single path the app is associated with. Claiming the
	// whole origin would hand every link to the SPA over to the app.
	appReturnPath = "/oauth/app-return"
)

// The documents are encoded from structs rather than maps so the field order is
// the one Apple and Google document, byte for byte.

type aasaComponent struct {
	Path    string `json:"/"`
	Comment string `json:"comment"`
}

type aasaDetail struct {
	AppIDs     []string        `json:"appIDs"`
	Components []aasaComponent `json:"components"`
}

type aasaLinks struct {
	Details []aasaDetail `json:"details"`
}

type aasaDocument struct {
	Applinks aasaLinks `json:"applinks"`
}

type assetLinkTarget struct {
	Namespace    string   `json:"namespace"`
	PackageName  string   `json:"package_name"`
	Fingerprints []string `json:"sha256_cert_fingerprints"`
}

type assetLinkStatement struct {
	Relation []string        `json:"relation"`
	Target   assetLinkTarget `json:"target"`
}

// Handler serves both documents from the configured app associations. The
// bodies are fixed for the process lifetime, so they are built once here.
func Handler(cfg config.Config) http.Handler {
	var aasa, assetLinks []byte
	if len(cfg.AppLinksIOSAppIDs) > 0 {
		aasa = mustJSON(aasaDocument{Applinks: aasaLinks{Details: []aasaDetail{{
			AppIDs:     cfg.AppLinksIOSAppIDs,
			Components: []aasaComponent{{Path: appReturnPath, Comment: "OAuth return to the app"}},
		}}}})
	}
	if len(cfg.AppLinksAndroid) > 0 {
		statements := make([]assetLinkStatement, 0, len(cfg.AppLinksAndroid))
		for _, a := range cfg.AppLinksAndroid {
			statements = append(statements, assetLinkStatement{
				Relation: []string{"delegate_permission/common.handle_all_urls"},
				Target:   assetLinkTarget{Namespace: "android_app", PackageName: a.Package, Fingerprints: a.Fingerprints},
			})
		}
		assetLinks = mustJSON(statements)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		switch r.URL.Path {
		case AASAPath:
			body = aasa
		case AssetLinksPath:
			body = assetLinks
		}
		// A platform with no configured app is not associated with this domain
		// at all, so its document must be absent. An empty one is a valid
		// "no app may claim these links" answer that the OS would cache.
		if body == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(body)
	})
}

// mustJSON panics on an unmarshalable document. Both shapes are plain strings
// and string slices, so this can only fire on a programming error at boot,
// never on a request.
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("applinks: unmarshalable document: %v", err))
	}
	return b
}
