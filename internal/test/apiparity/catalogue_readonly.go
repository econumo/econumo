package apiparity

// Read-only (lapsed trial) access, exercised as the seeded ReadonlyID user
// (see its fixture.go comment for why the row is full-with-past-expiry).
//
// Two contracts are frozen here. First the 402 envelope, emitted by the auth
// middleware rather than any handler (the per-endpoint @Failure 402
// annotations it corresponds to are kept honest by
// TestGuard_EveryRestrictedPostDocuments402). Second the access_until
// round-trip: get-user-data echoes the stored column, so the enginecompare
// suite asserts both engines' stored values (SQLite DATETIME vs PostgreSQL
// TIMESTAMP) come back as identical response bytes.
func init() {
	register(Scenario{Name: "readonly_access", Calls: func() []Call {
		return []Call{
			// Reads are never restricted, and the response carries the collapsed
			// level ("readonly") plus the raw expiry the SPA renders.
			{Label: "get-user-data", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "readonly", Body: map[string]any{}},

			// Two different modules, so the 402 is visibly a blanket POST rule
			// rather than one endpoint's own check.
			{Label: "err:create-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "readonly",
				Body: map[string]any{"id": "c0000000-0000-0000-0000-0000000000aa", "name": "Blocked", "type": "expense", "icon": "lock"}},
			{Label: "err:create-tag", Method: "POST", Path: "/api/v1/tag/create-tag", Auth: "readonly",
				Body: map[string]any{"id": "10000000-0000-0000-0000-0000000000aa", "name": "Blocked"}},

			// Allowlisted account-security POSTs stay reachable: a restricted user
			// must always be able to secure their account and leave it. logout
			// goes last — it revokes the presenting session.
			{Label: "revoke-other-sessions", Method: "POST", Path: "/api/v1/user/revoke-other-sessions", Auth: "readonly"},
			{Label: "logout-user", Method: "POST", Path: "/api/v1/user/logout-user", Auth: "readonly"},
		}
	}})
}
