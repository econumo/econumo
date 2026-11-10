package apiparity

func init() {
	// The harness configures no AI provider, so both onboarding writes do only
	// their synchronous part: no account ever turns savings here.
	register(Scenario{Name: "onboarding_writes", Calls: func() []Call {
		const onbBudget = "b0000000-0000-0000-0000-0000000000d0"
		tooMany := make([]string, 51)
		for i := range tooMany {
			tooMany[i] = OwnerAccount
		}
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/onboarding/create-budget", Auth: "owner",
				Body: map[string]any{"id": onbBudget, "name": "My budget", "startDate": "", "currencyId": USD, "accountIds": []string{OwnerAccount}}},
			// The same operation id again: refused exactly like budget/create-budget.
			{Label: "err:create-budget-again", Method: "POST", Path: "/api/v1/onboarding/create-budget", Auth: "owner",
				Body: map[string]any{"id": onbBudget, "name": "My budget", "startDate": "", "currencyId": USD, "accountIds": []string{OwnerAccount}}},
			{Label: "err:create-budget-blank", Method: "POST", Path: "/api/v1/onboarding/create-budget", Auth: "owner",
				Body: map[string]any{"id": "", "name": ""}},
			{Label: "err:create-budget-too-many", Method: "POST", Path: "/api/v1/onboarding/create-budget", Auth: "owner",
				Body: map[string]any{"id": onbBudget, "name": "My budget", "accountIds": tooMany}},
			{Label: "add-budget-accounts", Method: "POST", Path: "/api/v1/onboarding/add-budget-accounts", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "accountIds": []string{OwnerAccount}}},
			{Label: "err:add-budget-accounts-foreign", Method: "POST", Path: "/api/v1/onboarding/add-budget-accounts", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "accountIds": []string{SharedAccount}}},
			{Label: "err:add-budget-accounts-blank", Method: "POST", Path: "/api/v1/onboarding/add-budget-accounts", Auth: "owner",
				Body: map[string]any{"budgetId": "", "accountIds": []string{}}},
			{Label: "err:add-budget-accounts-not-uuid", Method: "POST", Path: "/api/v1/onboarding/add-budget-accounts", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "accountIds": []string{"nope"}}},
			{Label: "err:add-budget-accounts-too-many", Method: "POST", Path: "/api/v1/onboarding/add-budget-accounts", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "accountIds": tooMany}},
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + onbBudget, Auth: "owner"},
		}
	}})
}
