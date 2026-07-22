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
			{Label: "update-avatar", Method: "POST", Path: "/api/v1/user/update-avatar", Auth: "owner",
				Body: map[string]any{"icon": "pets", "color": "teal"}},
			// Pins the tier-1 blank envelope and the tier-2 format/choice envelope.
			{Label: "err:update-avatar-blank", Method: "POST", Path: "/api/v1/user/update-avatar", Auth: "owner",
				Body: map[string]any{"icon": "", "color": ""}},
			{Label: "err:update-avatar-bad-values", Method: "POST", Path: "/api/v1/user/update-avatar", Auth: "owner",
				Body: map[string]any{"icon": "Not-Valid", "color": "neon"}},
			// update-password spares the presenting session (only OTHER sessions are
			// revoked), so the owner token keeps working here.
			{Label: "update-password", Method: "POST", Path: "/api/v1/user/update-password", Auth: "owner",
				Body: map[string]any{"oldPassword": SeedPassword, "newPassword": "new-secret-pw"}},
			{Label: "get-user-data-after", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "owner"},
			// Logout revokes the presenting session (DB-backed tokens) — a subsequent
			// call with the same token must 401 with the frozen envelope. Also pins
			// the frozen {"result":"test"} quirk.
			// Billing link: pins the assembled portal URL (the signed assertion
			// itself is redacted). "for" preselects a beneficiary and is a hint
			// only — the portal authorizes it server-side.
			{Label: "create-billing-link", Method: "POST", Path: "/api/v1/user/create-billing-link", Auth: "owner",
				Body: map[string]any{}},
			{Label: "create-billing-link-for", Method: "POST", Path: "/api/v1/user/create-billing-link", Auth: "owner",
				Body: map[string]any{"for": GuestID}},
			{Label: "err:create-billing-link-bad-for", Method: "POST", Path: "/api/v1/user/create-billing-link", Auth: "owner",
				Body: map[string]any{"for": "not-a-uuid"}},
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
			// Default config runs flag-off, so the seeded owner is verified:
			// confirm with any code pins the generic invalid-code envelope, and
			// resend pins the silent-success (no email side effect) envelope.
			{Label: "err:confirm-email-bad-code", Method: "POST", Path: "/api/v1/user/confirm-email", Auth: "",
				Body: map[string]any{"username": OwnerEmail, "code": "000000000000"}},
			{Label: "resend-verification-code", Method: "POST", Path: "/api/v1/user/resend-verification-code", Auth: "",
				Body: map[string]any{"username": OwnerEmail}},
		}
	}})
}

func init() {
	register(Scenario{Name: "user_sessions", Calls: func() []Call {
		return []Call{
			// The seeded owner session is the only one -> a single isCurrent row.
			{Label: "get-session-list", Method: "GET", Path: "/api/v1/user/get-session-list", Auth: "owner"},
			// A second login mints a second session (its raw token stays inside the
			// login response; datetimes + UUIDv7 ids are redacted by the normalizer).
			{Label: "login-second-session", Method: "POST", Path: "/api/v1/user/login-user", Auth: "",
				Body: map[string]any{"username": OwnerEmail, "password": SeedPassword}},
			{Label: "get-session-list-two", Method: "GET", Path: "/api/v1/user/get-session-list", Auth: "owner"},
			{Label: "revoke-other-sessions", Method: "POST", Path: "/api/v1/user/revoke-other-sessions", Auth: "owner"},
			{Label: "get-session-list-after-revoke", Method: "GET", Path: "/api/v1/user/get-session-list", Auth: "owner"},
			// Foreign session id -> the domain-not-found envelope (400), and the
			// owner's session survives the attempt.
			{Label: "err:revoke-session-foreign", Method: "POST", Path: "/api/v1/user/revoke-session", Auth: "guest",
				Body: map[string]any{"id": OwnerSessionID}},
			{Label: "err:revoke-session-blank", Method: "POST", Path: "/api/v1/user/revoke-session", Auth: "owner",
				Body: map[string]any{"id": ""}},
			{Label: "revoke-session-current", Method: "POST", Path: "/api/v1/user/revoke-session", Auth: "guest",
				Body: map[string]any{"id": GuestSessionID}},
			{Label: "err:guest-after-self-revoke", Method: "GET", Path: "/api/v1/user/get-user-data", Auth: "guest"},
		}
	}})
}

func init() {
	register(Scenario{Name: "user_personal_tokens", Calls: func() []Call {
		return []Call{
			{Label: "create-personal-token", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "CI export", "expiresAt": ""}},
			{Label: "create-personal-token-expiring", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "Short lived", "expiresAt": "2030-01-01 00:00:00"}},
			{Label: "get-personal-token-list", Method: "GET", Path: "/api/v1/user/get-personal-token-list", Auth: "owner"},
			{Label: "err:create-personal-token-past", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "Expired", "expiresAt": "2020-01-01 00:00:00"}},
			{Label: "err:create-personal-token-blank-name", Method: "POST", Path: "/api/v1/user/create-personal-token", Auth: "owner",
				Body: map[string]any{"name": "", "expiresAt": ""}},
			{Label: "err:revoke-personal-token-unknown", Method: "POST", Path: "/api/v1/user/revoke-personal-token", Auth: "owner",
				Body: map[string]any{"id": "00000000-0000-0000-0000-000000000009"}},
		}
	}})
}
