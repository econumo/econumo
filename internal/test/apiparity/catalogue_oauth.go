package apiparity

import "net/url"

// OAuth-module scenarios. The success callback needs a live consent step and is
// covered by internal/oauth's own suite; here the deterministic edges are
// pinned: provider list, start-login/link URLs (random params and the per-flow
// secret redacted), the error redirects, the raw handoff 401, and the identity
// list/unlink envelopes.
func init() {
	register(Scenario{Name: "oauth_flows", Calls: func() []Call {
		return []Call{
			{Label: "get-provider-list", Method: "GET", Path: "/api/v1/oauth/get-provider-list"},
			{Label: "start-login", Method: "POST", Path: "/api/v1/oauth/start-login", Body: map[string]any{"provider": "oidc", "client": "web"}},
			{Label: "err:start-login-unconfigured", Method: "POST", Path: "/api/v1/oauth/start-login", Body: map[string]any{"provider": "apple", "client": "web"}},
			{Label: "err:start-login-bad-client", Method: "POST", Path: "/api/v1/oauth/start-login", Body: map[string]any{"provider": "oidc", "client": "desktop"}},
			{Label: "start-link", Method: "POST", Path: "/api/v1/oauth/start-link", Auth: "owner", Body: map[string]any{"provider": "oidc", "client": "app"}},
			{Label: "start-link-readonly-allowed", Method: "POST", Path: "/api/v1/oauth/start-link", Auth: "readonly", Body: map[string]any{"provider": "oidc", "client": "web"}},
			{Label: "err:callback-oidc-bad-state", Method: "GET", Path: "/api/v1/oauth/callback-oidc?code=x&state=bogus"},
			{Label: "err:callback-google-bad-state", Method: "GET", Path: "/api/v1/oauth/callback-google?code=x&state=bogus"},
			{Label: "err:callback-apple-bad-state", Method: "POST", Path: "/api/v1/oauth/callback-apple", Form: url.Values{"code": {"x"}, "state": {"bogus"}}},
			{Label: "err:exchange-handoff-unknown", Method: "POST", Path: "/api/v1/oauth/exchange-handoff", Body: map[string]any{"code": "nope", "flow": "x"}},
			{Label: "err:exchange-handoff-blank", Method: "POST", Path: "/api/v1/oauth/exchange-handoff", Body: map[string]any{"code": "", "flow": "x"}},
			{Label: "err:exchange-handoff-blank-flow", Method: "POST", Path: "/api/v1/oauth/exchange-handoff", Body: map[string]any{"code": "nope", "flow": ""}},
			{Label: "get-identity-list-empty", Method: "GET", Path: "/api/v1/oauth/get-identity-list", Auth: "owner"},
			{Label: "err:unlink-identity-missing", Method: "POST", Path: "/api/v1/oauth/unlink-identity", Auth: "owner", Body: map[string]any{"provider": "google"}},
			{Label: "get-identity-list-seeded", Method: "GET", Path: "/api/v1/oauth/get-identity-list", Auth: "guest"},
			{Label: "unlink-identity", Method: "POST", Path: "/api/v1/oauth/unlink-identity", Auth: "guest", Body: map[string]any{"provider": "google"}},
			{Label: "get-identity-list-after-unlink", Method: "GET", Path: "/api/v1/oauth/get-identity-list", Auth: "guest"},
		}
	}})
}
