package apiparity

// Income-element scenario: seeding, the income envelope lifecycle, both coded
// side backstops, planned income via set-limit — and the frozen-contract proof
// that get-budget shows none of it. The fixture's CatSalary (income) is seeded
// as a type=3 element by create-budget.

func init() {
	register(Scenario{Name: "budget_income_elements", Calls: func() []Call {
		const (
			incomeBudget   = "b0000000-0000-0000-0000-0000000000ee"
			expenseFolder  = "bf000000-0000-0000-0000-0000000000ee"
			incomeEnvelope = "be000000-0000-0000-0000-0000000000ee"
		)
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": incomeBudget, "name": "Income Plan", "currencyId": USD, "startDate": "2024-04-01"}},
			{Label: "get-budget-baseline", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + incomeBudget + "&date=2024-04-15", Auth: "owner"},
			{Label: "create-folder", Method: "POST", Path: "/api/v1/budget/create-folder", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": expenseFolder, "name": "Living"}},
			// CatFood makes the folder expense-sided.
			{Label: "seed-expense-folder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": CatFood, "folderId": expenseFolder, "afterId": nil}},
			// The behavior change on record: an income child in a default (expense)
			// envelope is now rejected instead of silently stored.
			{Label: "err:expense-envelope-income-child", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeEnvelope, "name": "Wrong Side", "icon": "cart",
					"currencyId": USD, "folderId": nil, "categories": []string{CatSalary}}},
			{Label: "create-income-envelope", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeEnvelope, "name": "Salaries", "icon": "payments",
					"currencyId": USD, "folderId": nil, "side": "income", "categories": []string{CatSalary}}},
			{Label: "err:income-envelope-expense-child", Method: "POST", Path: "/api/v1/budget/update-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeEnvelope, "name": "Salaries", "icon": "payments",
					"currencyId": USD, "isArchived": 0, "categories": []string{CatFood}}},
			{Label: "err:income-into-expense-folder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeEnvelope, "folderId": expenseFolder, "afterId": nil}},
			// Planned income is a plain limit on the income envelope.
			{Label: "set-income-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "elementId": incomeEnvelope, "period": "2024-05-01", "amount": "1500"}},
			// Frozen-contract proof: none of the income structure or its limit is
			// visible in the budget view.
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + incomeBudget + "&date=2024-05-15", Auth: "owner"},
		}
	}})
}
