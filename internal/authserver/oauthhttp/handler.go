// Package oauthhttp is the public HTTP edge of the OAuth authorization server:
// discovery metadata, dynamic client registration and the token endpoint.
// Responses are RFC 6749/7591/8414 JSON, not the API envelope.
package oauthhttp

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/econumo/econumo/internal/authserver"
	"github.com/econumo/econumo/internal/model"
)

const (
	ProtectedResourcePath    = "/.well-known/oauth-protected-resource"
	ProtectedResourceMCPPath = "/.well-known/oauth-protected-resource/mcp"
	AuthServerMetadataPath   = "/.well-known/oauth-authorization-server"
	RegisterPath             = "/oauth/register"
	TokenPath                = "/oauth/token"
)

const maxBodyBytes = 64 << 10

// Challenge is the WWW-Authenticate value for /mcp 401s; "" when the server is disabled.
func Challenge(svc *authserver.Service) string {
	if !svc.Enabled() {
		return ""
	}
	return `Bearer resource_metadata="` + svc.Issuer() + ProtectedResourceMCPPath + `"`
}

func Handler(svc *authserver.Service) http.Handler {
	h := &handler{svc: svc}
	mux := http.NewServeMux()
	for _, p := range []string{ProtectedResourcePath, ProtectedResourceMCPPath, AuthServerMetadataPath} {
		mux.HandleFunc("OPTIONS "+p, h.preflight)
	}
	mux.HandleFunc("GET "+ProtectedResourcePath, h.protectedResource)
	mux.HandleFunc("GET "+ProtectedResourceMCPPath, h.protectedResource)
	mux.HandleFunc("GET "+AuthServerMetadataPath, h.authServerMetadata)
	mux.HandleFunc("OPTIONS "+RegisterPath, h.preflight)
	mux.HandleFunc("POST "+RegisterPath, h.register)
	mux.HandleFunc("OPTIONS "+TokenPath, h.preflight)
	mux.HandleFunc("POST "+TokenPath, h.token)
	return mux
}

type handler struct{ svc *authserver.Service }

func (h *handler) preflight(w http.ResponseWriter, _ *http.Request) {
	setCORS(w)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) protectedResource(w http.ResponseWriter, _ *http.Request) {
	if !h.enabled(w) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 h.svc.ResourceURL(),
		"authorization_servers":    []string{h.svc.Issuer()},
		"scopes_supported":         []string{authserver.Scope},
		"bearer_methods_supported": []string{"header"},
	})
}

func (h *handler) authServerMetadata(w http.ResponseWriter, _ *http.Request) {
	if !h.enabled(w) {
		return
	}
	iss := h.svc.Issuer()
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                iss,
		"authorization_endpoint":                iss + "/oauth/authorize",
		"token_endpoint":                        iss + TokenPath,
		"registration_endpoint":                 iss + RegisterPath,
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post", "client_secret_basic"},
		"scopes_supported":                      []string{authserver.Scope},
	})
}

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	var req model.ClientRegistrationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeOAuthError(w, r, &authserver.OAuthError{Status: http.StatusBadRequest, Code: "invalid_client_metadata", Description: "malformed JSON body"})
		return
	}
	res, err := h.svc.Register(r.Context(), req)
	if err != nil {
		writeOAuthError(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusCreated, res)
}

func (h *handler) token(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, r, &authserver.OAuthError{Status: http.StatusBadRequest, Code: "invalid_request", Description: "malformed form body"})
		return
	}
	// PostForm, not Form: credentials and grants must not be accepted from the
	// query string, where they would land in access logs.
	f := r.PostForm
	req := model.TokenRequest{
		GrantType: f.Get("grant_type"), Code: f.Get("code"), RedirectURI: f.Get("redirect_uri"),
		CodeVerifier: f.Get("code_verifier"), RefreshToken: f.Get("refresh_token"),
		Resource: f.Get("resource"), Scope: f.Get("scope"),
		ClientID: f.Get("client_id"), ClientSecret: f.Get("client_secret"),
	}
	if id, secret, ok := r.BasicAuth(); ok {
		// RFC 6749 2.3.1: both parts are form-urlencoded before Basic encoding.
		var err1, err2 error
		req.ClientID, err1 = url.QueryUnescape(id)
		req.ClientSecret, err2 = url.QueryUnescape(secret)
		if err := errors.Join(err1, err2); err != nil {
			writeOAuthError(w, r, &authserver.OAuthError{Status: http.StatusUnauthorized, Code: "invalid_client", Description: "malformed client credentials"})
			return
		}
	}
	res, err := h.svc.Token(r.Context(), req)
	if err != nil {
		writeOAuthError(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, res)
}

func (h *handler) enabled(w http.ResponseWriter) bool {
	if h.svc.Enabled() {
		return true
	}
	setCORS(w)
	writeJSON(w, http.StatusNotFound, errorBody{Error: "invalid_request", Description: "not available"})
	return false
}

type errorBody struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

func writeOAuthError(w http.ResponseWriter, r *http.Request, err error) {
	noStore(w)
	var oe *authserver.OAuthError
	if !errors.As(err, &oe) {
		slog.ErrorContext(r.Context(), "oauth endpoint failed", "err", err.Error())
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "server_error"})
		return
	}
	if oe.Status == http.StatusUnauthorized && oe.Code == "invalid_client" {
		w.Header().Set("WWW-Authenticate", `Basic realm="econumo"`)
	}
	writeJSON(w, oe.Status, errorBody{Error: oe.Code, Description: oe.Description})
}

func setCORS(w http.ResponseWriter) { w.Header().Set("Access-Control-Allow-Origin", "*") }

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	setCORS(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
