package apiparity

func init() {
	// The harness has no AI provider, so every valid request answers icon "".
	// A read: a readonly user is answered too.
	register(Scenario{Name: "onboarding_suggest_icon", Calls: func() []Call {
		const path = "/api/v1/onboarding/suggest-icon"
		return []Call{
			{Label: "suggest-icon-category", Method: "GET", Path: path + "?kind=category&name=Mortgage&type=expense", Auth: "owner"},
			{Label: "suggest-icon-account", Method: "GET", Path: path + "?kind=account&name=Revolut&currency=EUR", Auth: "owner"},
			{Label: "suggest-icon-readonly", Method: "GET", Path: path + "?kind=account&name=Cash", Auth: "readonly"},
			{Label: "err:suggest-icon-no-kind", Method: "GET", Path: path + "?name=Mortgage", Auth: "owner"},
			{Label: "err:suggest-icon-bad-kind", Method: "GET", Path: path + "?kind=payee&name=Mortgage", Auth: "owner"},
			{Label: "err:suggest-icon-blank-name", Method: "GET", Path: path + "?kind=category&name=%20%20", Auth: "owner"},
			{Label: "err:suggest-icon-bad-type", Method: "GET", Path: path + "?kind=category&name=Mortgage&type=transfer", Auth: "owner"},
		}
	}})

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

	// The import path: finish-import reuses the owner's budget and marks only the
	// import step; the categories and accounts steps then review what exists
	// (archive/restore, hide/unhide); the plan step reads the suggestions.
	register(Scenario{Name: "onboarding_import_review", Calls: func() []Call {
		const (
			importOp = "0f000000-0000-7000-8000-000000000011"
			catOp    = "0f000000-0000-7000-8000-000000000012"
			accOp    = "0f000000-0000-7000-8000-000000000013"
			otherOp  = "0f000000-0000-7000-8000-000000000014"
			curOp    = "0f000000-0000-7000-8000-000000000015"
		)
		var eurID string
		finish := map[string]any{"id": importOp}
		categories := map[string]any{"id": catOp, "categories": []any{}, "archive": []string{CatFood}}
		accounts := map[string]any{"id": accOp, "accounts": []any{}, "currencyIds": []string{USD}, "mainCurrencyId": USD, "hide": []string{OwnerAccount},
			"currencies": []any{map[string]any{"accountId": OwnerAccount, "currencyId": &eurID}},
			"balances":   []any{map[string]any{"accountId": OwnerAccount, "balance": "1000"}}}
		return []Call{
			{Label: "err:finish-import-blank", Method: "POST", Path: "/api/v1/onboarding/finish-import", Auth: "owner", Body: map[string]any{"id": ""}},
			{Label: "err:finish-import-readonly", Method: "POST", Path: "/api/v1/onboarding/finish-import", Auth: "readonly", Body: finish},
			{Label: "finish-import", Method: "POST", Path: "/api/v1/onboarding/finish-import", Auth: "owner", Body: finish},
			{Label: "finish-import-replay", Method: "POST", Path: "/api/v1/onboarding/finish-import", Auth: "owner", Body: finish},
			{Label: "get-plan-suggestions", Method: "GET", Path: "/api/v1/onboarding/get-plan-suggestions", Auth: "owner"},
			{Label: "get-plan-suggestions-named", Method: "GET", Path: "/api/v1/onboarding/get-plan-suggestions?budgetId=" + Budget, Auth: "owner"},
			{Label: "err:get-plan-suggestions-bad-id", Method: "GET", Path: "/api/v1/onboarding/get-plan-suggestions?budgetId=nope", Auth: "owner"},
			{Label: "err:get-plan-suggestions-guest", Method: "GET", Path: "/api/v1/onboarding/get-plan-suggestions?budgetId=" + Budget, Auth: "guest"},
			{Label: "err:save-categories-archive-foreign", Method: "POST", Path: "/api/v1/onboarding/save-categories", Auth: "guest",
				Body: map[string]any{"id": otherOp, "categories": []any{}, "archive": []string{CatFood}}},
			{Label: "err:save-categories-archive-and-restore", Method: "POST", Path: "/api/v1/onboarding/save-categories", Auth: "owner",
				Body: map[string]any{"id": otherOp, "archive": []string{CatFood}, "restore": []string{CatFood}}},
			{Label: "save-categories-archive", Method: "POST", Path: "/api/v1/onboarding/save-categories", Auth: "owner", Body: categories},
			{Label: "get-category-list-after", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner"},
			{Label: "err:save-accounts-hide-shared", Method: "POST", Path: "/api/v1/onboarding/save-accounts", Auth: "owner",
				Body: map[string]any{"id": otherOp, "mainCurrencyId": USD, "hide": []string{SharedAccount}}},
			{Label: "create-eur-currency", Method: "POST", Path: "/api/v1/currency/create-currency", Auth: "owner",
				Body:          map[string]any{"id": curOp, "code": "EUR", "name": "Euro", "symbol": "€", "fractionDigits": 2, "rate": "2"},
				CaptureIDInto: &eurID},
			{Label: "err:save-accounts-currency-shared", Method: "POST", Path: "/api/v1/onboarding/save-accounts", Auth: "owner",
				Body: map[string]any{"id": otherOp, "mainCurrencyId": USD, "currencies": []any{map[string]any{"accountId": SharedAccount, "currencyId": USD}}}},
			{Label: "err:save-accounts-balance-format", Method: "POST", Path: "/api/v1/onboarding/save-accounts", Auth: "owner",
				Body: map[string]any{"id": otherOp, "mainCurrencyId": USD, "balances": []any{map[string]any{"accountId": OwnerAccount, "balance": "1,5"}}}},
			{Label: "save-accounts-hide", Method: "POST", Path: "/api/v1/onboarding/save-accounts", Auth: "owner", Body: accounts},
			{Label: "get-folder-list-after", Method: "GET", Path: "/api/v1/account/get-folder-list", Auth: "owner"},
			{Label: "get-account-list-after", Method: "GET", Path: "/api/v1/account/get-account-list", Auth: "owner"},
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
		}
	}})
}
