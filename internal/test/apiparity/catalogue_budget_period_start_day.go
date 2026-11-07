package apiparity

// Budget period start day scenario: create with a day, edit it, refuse an
// out-of-range value, and clone a shifted budget. It builds its own budgets so
// the heavily-asserted Budget scenarios keep their goldens.

func init() {
	register(Scenario{Name: "budget_period_start_day", Calls: func() []Call {
		const (
			shiftedBudget = "b0000000-0000-0000-0000-0000000000d1"
			cloneBudget   = "b0000000-0000-0000-0000-0000000000d2"
		)
		return []Call{
			{Label: "create-budget-day-28", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": shiftedBudget, "name": "Shifted", "currencyId": USD, "startDate": "2024-04-01", "periodStartDay": 28, "accountIds": []string{OwnerAccount}}},
			{Label: "err:create-budget-day-29", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": "b0000000-0000-0000-0000-0000000000d3", "name": "Too late", "currencyId": USD, "periodStartDay": 29, "accountIds": []string{OwnerAccount}}},
			{Label: "clone-budget-day-28", Method: "POST", Path: "/api/v1/budget/clone-budget", Auth: "owner",
				Body: map[string]any{"id": shiftedBudget, "newId": cloneBudget, "name": "Shifted copy", "withLimits": false}},
			{Label: "update-budget-day-5", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": shiftedBudget, "name": "Shifted", "currencyId": USD, "periodStartDay": 5}},
			{Label: "err:update-budget-day-29", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": shiftedBudget, "name": "Shifted", "currencyId": USD, "periodStartDay": 29}},
			{Label: "get-budget-clone", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + cloneBudget + "&date=2024-04-15", Auth: "owner"},
			{Label: "get-budget-updated", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + shiftedBudget + "&date=2024-04-15", Auth: "owner"},
		}
	}})
}
