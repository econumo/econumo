package apiparity

// Income-element scenario: seeding, the create-envelope side=income rejection
// (income envelope creation is switched off for now), both coded side
// backstops driven through the seeded income category, planned income via
// set-limit, an empty income folder (the side is stored) — and the
// frozen-contract proof that get-budget shows none of it.
// The fixture's CatSalary (income) is seeded as a type=3 element by
// create-budget.

func init() {
	register(Scenario{Name: "budget_income_elements", Calls: func() []Call {
		const (
			incomeBudget   = "b0000000-0000-0000-0000-0000000000ee"
			expenseFolder  = "bf000000-0000-0000-0000-0000000000ee"
			incomeEnvelope = "be000000-0000-0000-0000-0000000000ee"
			incomeFolder   = "bf000000-0000-0000-0000-0000000000ef"
		)
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": incomeBudget, "name": "Income Plan", "currencyId": USD, "startDate": "2024-04-01", "accountIds": []string{OwnerAccount}}},
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
			// Income envelope creation is switched off: side=income is an invalid
			// choice on the side field, and nothing is written.
			{Label: "err:income-envelope-not-allowed", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeEnvelope, "name": "Salaries", "icon": "payments",
					"currencyId": USD, "folderId": nil, "side": "income", "categories": []string{CatSalary}}},
			{Label: "err:income-into-expense-folder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": CatSalary, "folderId": expenseFolder, "afterId": nil}},
			// Planned income is a plain limit on the income category.
			{Label: "set-income-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "elementId": CatSalary, "period": "2024-05-01", "amount": "1500"}},
			// A folder's side is stored, so an empty income folder stays one.
			{Label: "create-income-folder", Method: "POST", Path: "/api/v1/budget/create-folder", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": incomeFolder, "name": "Earnings", "side": "income"}},
			{Label: "err:invalid-folder-side", Method: "POST", Path: "/api/v1/budget/create-folder", Auth: "owner",
				Body: map[string]any{"budgetId": incomeBudget, "id": "bf000000-0000-0000-0000-0000000000f0", "name": "Nowhere", "side": "savings"}},
			// Frozen-contract proof: none of the income structure or its limit is
			// visible in the budget view -- the empty income folder included.
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + incomeBudget + "&date=2024-05-15", Auth: "owner"},
			// The plan view lists both folders, each with its side.
			{Label: "get-budget-plan-after", Method: "GET", Path: "/api/v1/budget/get-budget-plan?id=" + incomeBudget + "&from=2024-04-01&months=2", Auth: "owner"},
		}
	}})
}
