package apiparity

import "net/url"

// MCP authorization-server consent + connected-apps scenarios. The seeded
// client (OAuthClientID, redirecting to claude.ai) and the owner's seeded grant
// (OAuthGrantID) are the fixed targets; the one-time code in an approve
// redirect is redacted by the normalizer.
func init() {
	const callback = "https://claude.ai/api/mcp/auth_callback"
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	authBody := func(clientID, redirect string) map[string]any {
		return map[string]any{"clientId": clientID, "redirectUri": redirect, "responseType": "code",
			"codeChallenge": challenge, "codeChallengeMethod": "S256", "state": "s1"}
	}
	query := func(clientID, redirect, challenge string) string {
		return "/api/v1/authserver/get-authorization-request?clientId=" + clientID + "&redirectUri=" + url.QueryEscape(redirect) +
			"&responseType=code&codeChallenge=" + challenge + "&codeChallengeMethod=S256&state=s1"
	}

	const shareCallback = "https://share.example.test/auth/callback"
	register(Scenario{Name: "authserver_consent", Calls: func() []Call {
		return []Call{
			{Label: "get-authorization-request-share-client", Method: "GET", Path: query(ShareClientID, shareCallback, challenge), Auth: "owner"},
			{Label: "approve-authorization-share-client-readonly", Method: "POST", Path: "/api/v1/authserver/approve-authorization", Auth: "readonly", Body: authBody(ShareClientID, shareCallback)},
			{Label: "get-authorization-request", Method: "GET", Path: query(OAuthClientID, callback, challenge), Auth: "owner"},
			{Label: "get-authorization-request-bad-challenge-redirects-the-error", Method: "GET", Path: query(OAuthClientID, callback, "short"), Auth: "owner"},
			{Label: "err:get-authorization-request-unknown-client", Method: "GET", Path: query("00000000-0000-0000-0000-00000000dead", callback, challenge), Auth: "owner"},
			{Label: "err:get-authorization-request-redirect-mismatch", Method: "GET", Path: query(OAuthClientID, "https://evil.example.test/cb", challenge), Auth: "owner"},
			{Label: "err:get-authorization-request-unauthenticated", Method: "GET", Path: query(OAuthClientID, callback, challenge)},
			{Label: "approve-authorization", Method: "POST", Path: "/api/v1/authserver/approve-authorization", Auth: "owner", Body: authBody(OAuthClientID, callback)},
			{Label: "err:approve-authorization-unknown-client", Method: "POST", Path: "/api/v1/authserver/approve-authorization", Auth: "owner", Body: authBody("00000000-0000-0000-0000-00000000dead", callback)},
			{Label: "err:approve-authorization-readonly", Method: "POST", Path: "/api/v1/authserver/approve-authorization", Auth: "readonly", Body: authBody(OAuthClientID, callback)},
			{Label: "decline-authorization", Method: "POST", Path: "/api/v1/authserver/decline-authorization", Auth: "owner", Body: authBody(OAuthClientID, callback)},
			{Label: "decline-authorization-readonly", Method: "POST", Path: "/api/v1/authserver/decline-authorization", Auth: "readonly", Body: authBody(OAuthClientID, callback)},
			{Label: "err:decline-authorization-redirect-mismatch", Method: "POST", Path: "/api/v1/authserver/decline-authorization", Auth: "owner", Body: authBody(OAuthClientID, "https://evil.example.test/cb")},
		}
	}})

	register(Scenario{Name: "authserver_connected_apps", Calls: func() []Call {
		revoke := "/api/v1/authserver/revoke-connected-app"
		return []Call{
			{Label: "get-connected-app-list", Method: "GET", Path: "/api/v1/authserver/get-connected-app-list", Auth: "owner"},
			{Label: "get-connected-app-list-guest-empty", Method: "GET", Path: "/api/v1/authserver/get-connected-app-list", Auth: "guest"},
			{Label: "err:revoke-connected-app-blank", Method: "POST", Path: revoke, Auth: "owner", Body: map[string]any{"id": ""}},
			{Label: "err:revoke-connected-app-invalid-id", Method: "POST", Path: revoke, Auth: "owner", Body: map[string]any{"id": "nope"}},
			{Label: "err:revoke-connected-app-foreign", Method: "POST", Path: revoke, Auth: "guest", Body: map[string]any{"id": OAuthGrantID}},
			{Label: "err:revoke-connected-app-readonly-reaches-the-handler", Method: "POST", Path: revoke, Auth: "readonly", Body: map[string]any{"id": OAuthGrantID}},
			{Label: "revoke-connected-app", Method: "POST", Path: revoke, Auth: "owner", Body: map[string]any{"id": OAuthGrantID}},
			{Label: "err:revoke-connected-app-again", Method: "POST", Path: revoke, Auth: "owner", Body: map[string]any{"id": OAuthGrantID}},
			{Label: "get-connected-app-list-after", Method: "GET", Path: "/api/v1/authserver/get-connected-app-list", Auth: "owner"},
		}
	}})

	// The share app's read-scoped token through the real handler: every
	// allowlisted GET answers; anything else (a write, a non-allowlisted read)
	// gets the frozen 401, never a 402 or 200.
	register(Scenario{Name: "read_scope", Calls: func() []Call {
		return []Call{
			{Label: "get-user-data", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "read"},
			{Label: "get-account-list", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "read"},
			{Label: "get-category-list", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "read"},
			{Label: "get-tag-list", Method: "GET", Path: "/api/v1/tag/get-tag-list", Auth: "read"},
			{Label: "get-label-list", Method: "GET", Path: "/api/v1/label/get-label-list", Auth: "read"},
			{Label: "get-currency-list", Method: "GET", Path: "/api/v1/currency/get-currency-list", Auth: "read"},
			{Label: "get-currency-rate-list", Method: "GET", Path: "/api/v1/currency/get-currency-rate-list", Auth: "read"},
			{Label: "get-currency-rate-list-ranged", Method: "GET", Path: "/api/v1/currency/get-currency-rate-list?from=2026-03-01&to=2026-03-31", Auth: "read"},
			{Label: "get-account-transaction-list", Method: "GET", Path: "/api/v1/transaction/get-account-transaction-list", Auth: "read"},
			{Label: "get-transaction-list", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "read"},
			{Label: "err:get-session-list", Method: "GET", Path: "/api/v1/user/get-session-list", Auth: "read"},
			{Label: "err:create-tag", Method: "POST", Path: "/api/v1/tag/create-tag", Auth: "read", Body: map[string]any{"id": "10000000-0000-0000-0000-0000000000fe", "name": "Urgent"}},
			{Label: "err:update-name", Method: "POST", Path: "/api/v1/user/update-name", Auth: "read", Body: map[string]any{"name": "Renamed"}},
			{Label: "err:get-budget-list", Method: "GET", Path: "/api/v1/budget/get-budget-list", Auth: "read"},
		}
	}})
}
