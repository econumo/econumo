package apiparity

// Read-only (lapsed trial) access. The seeded ReadonlyID user holds
// access_level "full" with an access_until in the past, which
// model.EffectiveAccessLevel collapses to read-only at request time.
//
// Two contracts are frozen here. First the 402 envelope itself — emitted by the
// auth middleware, so it is the one error shape no handler annotation covers.
// Second the access_until round-trip: get-user-data echoes the stored column,
// so the enginecompare suite asserts SQLite DATETIME and PostgreSQL TIMESTAMP
// serialize to the same bytes for it.
func init() {
	register(Scenario{Name: "readonly_access", Calls: func() []Call {
		return []Call{
			// Reads are never restricted, and the response carries the collapsed
			// level ("readonly") plus the raw expiry the SPA renders.
			{Label: "get-user-data", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "readonly", Body: map[string]any{}},
			{Label: "get-option-list", Method: "GET", Path: "/api/v1/user/get-option-list", Auth: "readonly", Body: map[string]any{}},

			// Two different modules, so the 402 is visibly a blanket POST rule
			// rather than one endpoint's own check.
			{Label: "err:create-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "readonly",
				Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000aa", "name": "Blocked", "type": "expense", "icon": "lock"}},
			{Label: "err:create-tag", Method: "POST", Path: "/api/v1/tag/create-tag", Auth: "readonly",
				Body: map[string]any{"id": "10000000-0000-0000-0000-0000000000aa", "name": "Blocked"}},

			// Allowlisted account-security POSTs stay reachable: a restricted user
			// must always be able to secure their account and leave it. logout
			// goes last — it revokes the presenting session.
			{Label: "revoke-other-sessions", Method: "POST", Path: "/api/v1/user/revoke-other-sessions", Auth: "readonly", Body: map[string]any{}},
			{Label: "logout-user", Method: "POST", Path: "/api/v1/user/logout-user", Auth: "readonly", Body: map[string]any{}},
		}
	}})
}
