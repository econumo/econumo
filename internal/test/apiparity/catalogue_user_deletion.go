package apiparity

// The owner is never confirmed with the right code (it is known only from the
// email): a mark would revoke the owner's fixture session for every later call.
func init() {
	register(Scenario{Name: "user_deletion", Calls: func() []Call {
		return []Call{
			{Label: "err:request-user-deletion-wrong-password", Method: "POST", Path: "/api/v1/user/request-user-deletion", Auth: "owner",
				Body: map[string]any{"password": "wrong-password"}},
			{Label: "request-user-deletion", Method: "POST", Path: "/api/v1/user/request-user-deletion", Auth: "owner",
				Body: map[string]any{"password": SeedPassword}},
			{Label: "err:confirm-user-deletion-bad-code", Method: "POST", Path: "/api/v1/user/confirm-user-deletion", Auth: "owner",
				Body: map[string]any{"code": "000000"}},
			{Label: "err:confirm-user-deletion-blank-code", Method: "POST", Path: "/api/v1/user/confirm-user-deletion", Auth: "owner",
				Body: map[string]any{"code": " "}},
			// On the readonly allowlist: leaving is never a paid feature.
			{Label: "readonly:request-user-deletion", Method: "POST", Path: "/api/v1/user/request-user-deletion", Auth: "readonly",
				Body: map[string]any{"password": SeedPassword}},
		}
	}})
}
