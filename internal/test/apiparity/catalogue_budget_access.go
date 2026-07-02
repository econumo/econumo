package apiparity

// Budget access + account scenarios: invite accept/decline/grant/revoke, plus
// account include/exclude, reset, and the transaction-list read — the 8 routes
// carved out of missingFromCatalogue by this file.
//
// accept-access and decline-access each consume the ONE seeded pending invite
// (fixture.go's BudgetAccess(Budget, GuestID, ...)), so they run in separate
// scenarios — every scenario gets a fresh DB.

func init() {
	register(Scenario{Name: "budget_access_accept", Calls: func() []Call {
		return []Call{
			// Guest accepts the seeded pending invite to Budget…
			{Label: "accept-access", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"budgetId": Budget}},
			// …then owner revokes it again.
			{Label: "revoke-access", Method: "POST", Path: "/api/v1/budget/revoke-access", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "userId": GuestID}},
			// Owner grants guest access to the invite-free second budget.
			// role enum: "admin" | "user" | "guest" (RoleFromAlias rejects "owner").
			{Label: "grant-access", Method: "POST", Path: "/api/v1/budget/grant-access", Auth: "owner",
				Body: map[string]any{"budgetId": Budget2, "userId": GuestID, "role": "user"}},
		}
	}})

	register(Scenario{Name: "budget_access_decline", Calls: func() []Call {
		return []Call{
			{Label: "decline-access", Method: "POST", Path: "/api/v1/budget/decline-access", Auth: "guest",
				Body: map[string]any{"budgetId": Budget}},
		}
	}})

	register(Scenario{Name: "budget_account_writes", Calls: func() []Call {
		// The fixture builder's clock starts at 2024-04-01T12:00:00 UTC and steps by
		// 1s per row (see fixture.go's Builder.now()), so Txn1's seeded spent_at
		// falls in April 2024 — NOT anywhere near the real wall-clock ClockTime used
		// for token issuance. periodStart must target that seed month or
		// get-transaction-list returns an empty (still-2xx, but wrong) list.
		const period = "2024-04-01"
		return []Call{
			// Field-name quirk (frozen): include/exclude carry the budget id as "id".
			{Label: "exclude-account", Method: "POST", Path: "/api/v1/budget/exclude-account", Auth: "owner",
				Body: map[string]any{"id": Budget, "accountId": OwnerAccount}},
			{Label: "include-account", Method: "POST", Path: "/api/v1/budget/include-account", Auth: "owner",
				Body: map[string]any{"id": Budget, "accountId": OwnerAccount}},
			// ResetBudgetRequest.StartedAt is parsed with datetime.Layout (full
			// "2006-01-02 15:04:05", not date-only Y-m-d) — a bare date fails Validate.
			{Label: "reset-budget", Method: "POST", Path: "/api/v1/budget/reset-budget", Auth: "owner",
				Body: map[string]any{"id": Budget, "startedAt": period + " 00:00:00"}},
			// Exactly-one-selector rule: categoryId alone is a valid mode. Must return
			// Txn1, the seeded Food expense (type=0, which is what the underlying
			// BudgetTransactionsByCategories query hard-filters on).
			{Label: "get-transaction-list", Method: "GET",
				Path: "/api/v1/budget/get-transaction-list?budgetId=" + Budget + "&periodStart=" + period + "&categoryId=" + CatFood,
				Auth: "owner"},
		}
	}})
}
