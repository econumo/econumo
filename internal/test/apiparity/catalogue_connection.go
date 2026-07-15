package apiparity

// Connection-module write scenarios: the invite/delete-connection flow.
// Account-access grants/revokes moved to the account module (see
// catalogue_account.go's account_access_writes scenario). Owner and guest are
// already Connect()ed in the shared fixture; generate-invite mints a fresh code
// every run (see NormalizeGolden's inviteCodeRe redaction), so the happy accept
// path can't be replayed statically here — it's covered end to end by the
// build-tagged internal/test/enginecompare/connection_invite_test.go.

func init() {
	register(Scenario{Name: "connection_invite_flows", Calls: func() []Call {
		return []Call{
			// Owner mints an invite (response carries a fresh, non-UUID 5-char code
			// each run; redacted by NormalizeGolden's inviteCodeRe).
			{Label: "generate-invite", Method: "POST", Path: "/api/v1/connection/generate-invite", Auth: "owner", Body: map[string]any{}},
			// The code is only in the response body, so the happy accept path can't
			// be replayed statically; pin the error envelope instead. The full
			// accept flow stays covered by the tagged connection_invite test.
			{Label: "err:accept-invite-bad-code", Method: "POST", Path: "/api/v1/connection/accept-invite", Auth: "guest",
				Body: map[string]any{"code": "000000"}},
			{Label: "delete-invite", Method: "POST", Path: "/api/v1/connection/delete-invite", Auth: "owner", Body: map[string]any{}},
			// Owner and guest are Connect()ed in the seed; deleting the connection is
			// a real state change pinned by the closing read. DeleteConnectionRequest's
			// wire field is "id" (the connected user's id), not "userId".
			{Label: "delete-connection", Method: "POST", Path: "/api/v1/connection/delete-connection", Auth: "owner",
				Body: map[string]any{"id": GuestID}},
			{Label: "get-connection-list-after-delete", Method: "GET", Path: "/api/v1/connection/get-connection-list", Auth: "owner"},
		}
	}})
}
