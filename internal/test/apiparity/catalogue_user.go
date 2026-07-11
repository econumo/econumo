package apiparity

func init() {
	register(Scenario{Name: "user_writes", Calls: func() []Call {
		return []Call{
			// Public registration; returns the created user WITHOUT a token (frozen contract).
			{Label: "register-user", Method: "POST", Path: "/api/v1/user/register-user", Auth: "",
				Body: map[string]any{"email": "newuser@example.test", "password": SeedPassword, "name": "Newbie"}},
			{Label: "complete-onboarding", Method: "POST", Path: "/api/v1/user/complete-onboarding", Auth: "owner"}, // no body
			{Label: "update-currency", Method: "POST", Path: "/api/v1/user/update-currency", Auth: "owner",
				Body: map[string]any{"currency": "USD"}},
			// Field name is "value" (a budget id) — frozen quirk.
			{Label: "update-budget", Method: "POST", Path: "/api/v1/user/update-budget", Auth: "owner",
				Body: map[string]any{"value": Budget}},
			// update-password spares the presenting session (only OTHER sessions are
			// revoked), so the owner token keeps working here.
			{Label: "update-password", Method: "POST", Path: "/api/v1/user/update-password", Auth: "owner",
				Body: map[string]any{"oldPassword": SeedPassword, "newPassword": "new-secret-pw"}},
			{Label: "get-user-data-after", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "owner"},
			// Logout revokes the presenting session (DB-backed tokens) — a subsequent
			// call with the same token must 401 with the frozen envelope. Also pins
			// the frozen {"result":"test"} quirk.
			{Label: "logout-user", Method: "POST", Path: "/api/v1/user/logout-user", Auth: "owner"},
			{Label: "err:get-user-data-after-logout", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "owner"},
		}
	}})

	register(Scenario{Name: "user_auth_flows", Calls: func() []Call {
		return []Call{
			// Public login: returns {token, user}; the token is redacted in goldens.
			// Uses the password BEFORE user_writes' update-password — scenarios run on fresh DBs.
			{Label: "login-user", Method: "POST", Path: "/api/v1/user/login-user", Auth: "",
				Body: map[string]any{"username": OwnerEmail, "password": SeedPassword}},
			{Label: "err:login-bad-credentials", Method: "POST", Path: "/api/v1/user/login-user", Auth: "",
				Body: map[string]any{"username": OwnerEmail, "password": "wrong"}}, // pins "Invalid credentials." 401
			// Remind sends the reset email via the console transport (stdout) — the
			// HTTP response is pinned; the code itself is not reachable over HTTP.
			// The request field is "username" (an email), not "email".
			{Label: "remind-password", Method: "POST", Path: "/api/v1/user/remind-password", Auth: "",
				Body: map[string]any{"username": OwnerEmail}},
			// The happy reset path needs the emailed code and stays covered by the
			// build-tagged reset_password flow test; here we pin the error envelope.
			// ResetPasswordRequest also requires "username" (NotBlank+Email) alongside code/password.
			{Label: "err:reset-password-bad-code", Method: "POST", Path: "/api/v1/user/reset-password", Auth: "",
				Body: map[string]any{"username": OwnerEmail, "code": "00000000-0000-0000-0000-000000000000", "password": "irrelevant-pw"}},
		}
	}})
}
