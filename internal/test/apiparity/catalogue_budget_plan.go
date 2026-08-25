package apiparity

// get-budget-plan scenario: a fixed 2024 window over explicitly-dated data —
// the seeded income category (CatSalary, type=3) with planned income, an
// expense limit, transactions in distinct months on both sides (including
// category-less rows for the two Uncategorized cells) — plus the months-bounds
// validation error and a closing get-budget re-proof that none of it leaks
// into the budget view. Income ENVELOPE creation is switched off for now
// (side=income is rejected — budget_income_elements carries that rejection),
// so planned income here lives on the standalone income category.
//
// The fixture's dirty Envelope1<-CatSalary link (an expense envelope holding
// an income-category child, re-seeded AFTER migrations run — a state
// production can no longer reach) is deliberately KEPT: the plan read must
// stay robust to residual dirty rows, and this golden pins the correct
// rendering — CatSalary surfaces as a standalone income row of the seeded
// Budget's plan, never as an expense-envelope child.
//
// Amount table (all on OwnerAccount):
//
//	tx-apr-food          2024-04-10 10:00:00  expense  40.00  CatFood
//	tx-may-food          2024-05-02 09:00:00  expense  60.00  CatFood
//	tx-may-nocat         2024-05-21 12:00:00  expense   5.00  (none)
//	tx-apr-salary        2024-04-05 08:00:00  income  1000.00  CatSalary
//	tx-jun-nocat-income  2024-06-06 08:00:00  income    50.00  (none)
//
// The shared apiparity fixture's own Txn1 (Food, 12.50) / Txn2 (Salary,
// 1000.00) are NOT wall-clock-dated: fixture.Builder's default clock starts
// at 2024-04-01 12:00:00 and steps by 1s per seeded row (internal/test/fixture
// New()), and Seed() never overrides it before creating them — so both land
// within this scenario's April bucket too. The golden's April cells are
// therefore the table above PLUS that fixture pair: Food 40+12.5=52.5,
// Salary/CatSalary 1000+1000=2000. This is existing, precedented behavior
// (see budget_income_elements' get-budget-baseline golden, which already
// reflects the same two rows at date=2024-04-15), not something this
// scenario introduces.
//
// Transfers across the budget boundary (a Savings account excluded from the
// plan budget) feed the plan's transfers block and its drill-down list:
//
//	tx-apr-to-savings    2024-04-12 10:00:00  transfer 100.00  OwnerAccount -> Savings (out)
//	tx-may-from-savings  2024-05-08 10:00:00  transfer  30.00  Savings -> OwnerAccount (in)
//
// Savings is explicitly added as a member of the seeded fixture Budget (the
// add-savings-to-fixture-budget call), so its plan (the last call) sees both
// transfers as internal moves and reports no transfer rows.

func init() {
	register(Scenario{Name: "budget_plan", Calls: func() []Call {
		const (
			planBudget    = "b0000000-0000-0000-0000-0000000000ef"
			txAprFood     = "d0000000-0000-0000-0000-0000000000e1"
			txMayFood     = "d0000000-0000-0000-0000-0000000000e2"
			txMayNoCat    = "d0000000-0000-0000-0000-0000000000e3"
			txAprSalary   = "d0000000-0000-0000-0000-0000000000e4"
			txJunNoCatInc = "d0000000-0000-0000-0000-0000000000e5"
			opSavings     = "a0000000-0000-0000-0000-0000000000e6"
			txAprToSav    = "d0000000-0000-0000-0000-0000000000e7"
			txMayFromSav  = "d0000000-0000-0000-0000-0000000000e8"
		)
		var savingsID string
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": planBudget, "name": "Plan", "currencyId": USD, "startDate": "2024-04-01", "accountIds": []string{OwnerAccount}}},
			{Label: "set-expense-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": planBudget, "elementId": CatFood, "period": "2024-05-01", "amount": "300"}},
			{Label: "set-income-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": planBudget, "elementId": CatSalary, "period": "2024-05-01", "amount": "1500"}},
			// Explicitly-dated transactions — the seeded fixture rows sit at the
			// wall-clock ClockTime, outside this fixed window. Body shape copied
			// from catalogue_transaction.go's create-with-labels call (the "id"
			// is an operation id; the entity gets a server-minted UUIDv7, which
			// the golden normalizer redacts — nothing downstream needs it).
			{Label: "tx-apr-food", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txAprFood, "accountId": OwnerAccount, "type": "expense",
					"amount": "40.00", "categoryId": CatFood, "date": "2024-04-10 10:00:00"}},
			{Label: "tx-may-food", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txMayFood, "accountId": OwnerAccount, "type": "expense",
					"amount": "60.00", "categoryId": CatFood, "date": "2024-05-02 09:00:00"}},
			{Label: "tx-may-nocat", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txMayNoCat, "accountId": OwnerAccount, "type": "expense",
					"amount": "5.00", "date": "2024-05-21 12:00:00"}},
			{Label: "tx-apr-salary", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txAprSalary, "accountId": OwnerAccount, "type": "income",
					"amount": "1000.00", "categoryId": CatSalary, "date": "2024-04-05 08:00:00"}},
			{Label: "tx-jun-nocat-income", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txJunNoCatInc, "accountId": OwnerAccount, "type": "income",
					"amount": "50.00", "date": "2024-06-06 08:00:00"}},
			// The boundary: an owner account excluded from the plan budget, one
			// transfer out to it in April, one back in May.
			{Label: "create-savings-account", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body: map[string]any{"id": opSavings, "name": "Savings", "icon": "bank", "currencyId": USD, "folderId": OwnerFolder}, CaptureIDInto: &savingsID},
			{Label: "remove-savings-account", Method: "POST", Path: "/api/v1/budget/remove-account", Auth: "owner",
				Body: map[string]any{"id": planBudget, "accountId": &savingsID}},
			// Explicit membership pin: Savings is a NEW account (created above),
			// never a member of planBudget or the fixture Budget by default. It
			// must never join planBudget (that boundary is the whole point of this
			// scenario), but the fixture Budget's re-proof below still expects it
			// INCLUDED there (see the file header) — under the old implicit-member
			// model a new owner account joined every owner budget automatically;
			// under explicit membership it has to be added.
			{Label: "add-savings-to-fixture-budget", Method: "POST", Path: "/api/v1/budget/add-account", Auth: "owner",
				Body: map[string]any{"id": Budget, "accountId": &savingsID}},
			{Label: "tx-apr-to-savings", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txAprToSav, "accountId": OwnerAccount, "accountRecipientId": &savingsID, "type": "transfer",
					"amount": "100.00", "amountRecipient": "100.00", "date": "2024-04-12 10:00:00"}},
			{Label: "tx-may-from-savings", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txMayFromSav, "accountId": &savingsID, "accountRecipientId": OwnerAccount, "type": "transfer",
					"amount": "30.00", "amountRecipient": "30.00", "date": "2024-05-08 10:00:00"}},
			{Label: "get-budget-plan", Method: "GET",
				Path: "/api/v1/budget/get-budget-plan?id=" + planBudget + "&from=2024-04-01&months=3", Auth: "owner"},
			{Label: "get-transaction-list-transfers", Method: "GET",
				Path: "/api/v1/budget/get-transaction-list?budgetId=" + planBudget + "&periodStart=2024-04-01&transfers=1", Auth: "owner"},
			{Label: "err:transfers-with-uncategorized", Method: "GET",
				Path: "/api/v1/budget/get-transaction-list?budgetId=" + planBudget + "&periodStart=2024-04-01&transfers=1&uncategorized=1", Auth: "owner"},
			{Label: "err:months-out-of-range", Method: "GET",
				Path: "/api/v1/budget/get-budget-plan?id=" + planBudget + "&from=2024-04-01&months=40", Auth: "owner"},
			// Frozen-contract re-proof: the budget view shows none of the income
			// structure, planned income, or plan-only rows.
			{Label: "get-budget-after", Method: "GET",
				Path: "/api/v1/budget/get-budget?id=" + planBudget + "&date=2024-05-15", Auth: "owner"},
			// The seeded fixture Budget's plan — THIS is the dirty-link pin: its
			// Envelope1 (expense) holds the CatSalary (income) child link.
			{Label: "get-budget-plan-fixture-budget", Method: "GET",
				Path: "/api/v1/budget/get-budget-plan?id=" + Budget + "&from=2024-04-01&months=2", Auth: "owner"},
		}
	}})
}
