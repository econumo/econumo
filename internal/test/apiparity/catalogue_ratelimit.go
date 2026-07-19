package apiparity

import "fmt"

// Auth brute-force protection: the harness wires the production-default limits
// (5 login / 5 reset / 3 remind / 5 register per 15m window), so this scenario
// freezes the over-limit 429 envelope for each protected endpoint. Each
// scenario runs on a fresh DB + fresh in-memory limiter, keeping counts
// deterministic; total calls (22) stay under the global 60/min backstop.
func init() {
	register(Scenario{Name: "auth_rate_limit", Calls: func() []Call {
		var calls []Call
		for i := 1; i <= 5; i++ {
			calls = append(calls, Call{Label: fmt.Sprintf("err:login-bad-%d", i), Method: "POST", Path: "/api/v1/user/login-user", Auth: "",
				Body: map[string]any{"username": OwnerEmail, "password": "wrong"}})
		}
		// Correct password, but over the failure limit: lockout is by attempt
		// count, not credential.
		calls = append(calls, Call{Label: "err:login-limited", Method: "POST", Path: "/api/v1/user/login-user", Auth: "",
			Body: map[string]any{"username": OwnerEmail, "password": SeedPassword}})

		for i := 1; i <= 5; i++ {
			calls = append(calls, Call{Label: fmt.Sprintf("err:reset-bad-%d", i), Method: "POST", Path: "/api/v1/user/reset-password", Auth: "",
				Body: map[string]any{"username": GuestEmail, "code": "00000000-0000-0000-0000-000000000000", "password": "irrelevant-pw"}})
		}
		calls = append(calls, Call{Label: "err:reset-limited", Method: "POST", Path: "/api/v1/user/reset-password", Auth: "",
			Body: map[string]any{"username": GuestEmail, "code": "00000000-0000-0000-0000-000000000000", "password": "irrelevant-pw"}})

		// Remind counts every request (each sends an email via the console
		// transport), so three 200s then a 429.
		for i := 1; i <= 3; i++ {
			calls = append(calls, Call{Label: fmt.Sprintf("remind-%d", i), Method: "POST", Path: "/api/v1/user/remind-password", Auth: "",
				Body: map[string]any{"username": GuestEmail}})
		}
		calls = append(calls, Call{Label: "err:remind-limited", Method: "POST", Path: "/api/v1/user/remind-password", Auth: "",
			Body: map[string]any{"username": GuestEmail}})

		// Register counts every attempt: one success, four duplicate-email 400s,
		// then the cap.
		calls = append(calls, Call{Label: "register-1", Method: "POST", Path: "/api/v1/user/register-user", Auth: "",
			Body: map[string]any{"email": "ratelimit@example.test", "password": SeedPassword, "name": "RLU"}})
		for i := 2; i <= 5; i++ {
			calls = append(calls, Call{Label: fmt.Sprintf("err:register-dup-%d", i), Method: "POST", Path: "/api/v1/user/register-user", Auth: "",
				Body: map[string]any{"email": "ratelimit@example.test", "password": SeedPassword, "name": "RLU"}})
		}
		calls = append(calls, Call{Label: "err:register-limited", Method: "POST", Path: "/api/v1/user/register-user", Auth: "",
			Body: map[string]any{"email": "ratelimit@example.test", "password": SeedPassword, "name": "RLU"}})
		return calls
	}})
}
