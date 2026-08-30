package apiparity

// Budget lifecycle scenario: end date, archive/unarchive, and clone. It runs
// against Budget2 so the heavily-asserted Budget scenarios keep their goldens.

func init() {
	register(Scenario{Name: "budget_lifecycle", Calls: func() []Call {
		// Budget2 starts at the fixture clock's month (April 2024), so June is a
		// valid end month and January 2024 is before the start.
		const cloneID = "b0000000-0000-0000-0000-0000000000c3"
		return []Call{
			{Label: "update-budget-end-date", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": Budget2, "name": "Second", "currencyId": USD, "endDate": "2024-06-15"}},
			{Label: "err:update-budget-end-before-start", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": Budget2, "name": "Second", "currencyId": USD, "endDate": "2024-01-01"}},
			{Label: "archive-budget", Method: "POST", Path: "/api/v1/budget/archive-budget", Auth: "owner",
				Body: map[string]any{"id": Budget2}},
			// A write on the archived budget is refused with the coded 403.
			{Label: "err:set-limit-archived", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": Budget2, "elementId": CatFood, "period": "2024-05-01", "amount": "10"}},
			{Label: "unarchive-budget", Method: "POST", Path: "/api/v1/budget/unarchive-budget", Auth: "owner",
				Body: map[string]any{"id": Budget2}},
			{Label: "clone-budget", Method: "POST", Path: "/api/v1/budget/clone-budget", Auth: "owner",
				Body: map[string]any{"id": Budget2, "newId": cloneID, "name": "Copy", "withLimits": true}},
			{Label: "get-budget-clone", Method: "GET",
				Path: "/api/v1/budget/get-budget?id=" + cloneID + "&date=2024-05-01", Auth: "owner"},
			// Only owner|admin may clone — the guest (no grant on Budget2) is refused.
			{Label: "err:clone-budget-not-owner", Method: "POST", Path: "/api/v1/budget/clone-budget", Auth: "guest",
				Body: map[string]any{"id": Budget2, "newId": "b0000000-0000-0000-0000-0000000000c4", "name": "Nope", "withLimits": false}},
			// An accepted admin ("full control") may clone; the copy is theirs and
			// the former owner joins its sharing set as an accepted admin.
			{Label: "grant-access-for-admin-clone", Method: "POST", Path: "/api/v1/budget/grant-access", Auth: "owner",
				Body: map[string]any{"budgetId": Budget2, "userId": GuestID, "role": "admin"}},
			{Label: "accept-access-for-admin-clone", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"budgetId": Budget2}},
			{Label: "clone-budget-as-admin", Method: "POST", Path: "/api/v1/budget/clone-budget", Auth: "guest",
				Body: map[string]any{"id": Budget2, "newId": "b0000000-0000-0000-0000-0000000000c5", "name": "Admin Copy", "withLimits": false}},
			{Label: "get-budget-clone-as-admin", Method: "GET",
				Path: "/api/v1/budget/get-budget?id=b0000000-0000-0000-0000-0000000000c5&date=2024-05-01", Auth: "owner"},
			// "" clears the end month again.
			{Label: "update-budget-clear-end-date", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": Budget2, "name": "Second", "currencyId": USD, "endDate": ""}},
		}
	}})
}
