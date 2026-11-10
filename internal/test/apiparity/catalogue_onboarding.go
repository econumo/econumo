package apiparity

func init() {
	// The owner already has an account and a budget, so save-categories creates
	// no budget and save-accounts adds to the default one. Each step answers a
	// replay of its id with the same state and writes nothing.
	register(Scenario{Name: "onboarding_steps", Calls: func() []Call {
		const (
			catOp   = "0f000000-0000-7000-8000-000000000001"
			accOp   = "0f000000-0000-7000-8000-000000000002"
			planOp  = "0f000000-0000-7000-8000-000000000003"
			otherOp = "0f000000-0000-7000-8000-000000000004"
		)
		categories := map[string]any{"id": catOp, "categories": []map[string]any{
			{"key": "row-coffee", "name": "Coffee", "type": "expense", "icon": "local_cafe"},
			{"key": "row-food", "name": "food", "type": "expense", "icon": "restaurant"},
			{"key": "row-bonus", "name": "Bonus", "type": "income", "icon": "payments"},
		}, "order": []string{"row-coffee", CatFood, "row-bonus", CatSalary}}
		accounts := map[string]any{"id": accOp, "accounts": []map[string]any{
			{"key": "row-cash", "name": "Cash box", "balance": "12.5", "currencyId": USD, "icon": "wallet"},
			{"key": "row-card", "name": "Card", "balance": "", "currencyId": USD, "icon": "credit_card"},
		}, "currencyIds": []string{USD}, "mainCurrencyId": USD}
		plan := map[string]any{"id": planOp, "budgetId": Budget, "amounts": []map[string]any{
			{"categoryId": CatFood, "amount": "300"},
			{"categoryId": CatSalary, "amount": "0"},
		}}
		return []Call{
			{Label: "err:save-categories-blank", Method: "POST", Path: "/api/v1/onboarding/save-categories", Auth: "owner",
				Body: map[string]any{"id": "", "categories": []any{}}},
			{Label: "err:save-categories-short-name", Method: "POST", Path: "/api/v1/onboarding/save-categories", Auth: "owner",
				Body: map[string]any{"id": otherOp, "categories": []map[string]any{{"key": "k", "name": "ab", "type": "expense", "icon": "home"}}}},
			{Label: "err:save-categories-readonly", Method: "POST", Path: "/api/v1/onboarding/save-categories", Auth: "readonly", Body: categories},
			{Label: "save-categories", Method: "POST", Path: "/api/v1/onboarding/save-categories", Auth: "owner", Body: categories},
			{Label: "save-categories-replay", Method: "POST", Path: "/api/v1/onboarding/save-categories", Auth: "owner", Body: categories},
			{Label: "get-category-list-after", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner"},
			{Label: "err:save-accounts-bad-balance", Method: "POST", Path: "/api/v1/onboarding/save-accounts", Auth: "owner",
				Body: map[string]any{"id": otherOp, "accounts": []map[string]any{{"key": "k", "name": "Cash box", "balance": "1,5", "currencyId": USD, "icon": "wallet"}}, "mainCurrencyId": USD}},
			{Label: "err:save-accounts-readonly", Method: "POST", Path: "/api/v1/onboarding/save-accounts", Auth: "readonly", Body: accounts},
			{Label: "save-accounts", Method: "POST", Path: "/api/v1/onboarding/save-accounts", Auth: "owner", Body: accounts},
			{Label: "save-accounts-replay", Method: "POST", Path: "/api/v1/onboarding/save-accounts", Auth: "owner", Body: accounts},
			{Label: "err:save-plan-negative", Method: "POST", Path: "/api/v1/onboarding/save-plan", Auth: "owner",
				Body: map[string]any{"id": otherOp, "amounts": []map[string]any{{"categoryId": CatFood, "amount": "-1"}}}},
			{Label: "err:save-plan-readonly", Method: "POST", Path: "/api/v1/onboarding/save-plan", Auth: "readonly", Body: plan},
			{Label: "save-plan", Method: "POST", Path: "/api/v1/onboarding/save-plan", Auth: "owner", Body: plan},
			{Label: "save-plan-replay", Method: "POST", Path: "/api/v1/onboarding/save-plan", Auth: "owner", Body: plan},
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
		}
	}})
}
