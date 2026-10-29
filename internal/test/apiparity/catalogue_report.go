package apiparity

// report scenario: get-flow-report and get-net-worth-report over a fixed 2024
// window of explicitly-dated data on OwnerAccount. The shared fixture's own
// Txn1 (Food, 12.50) and Txn2 (Salary, 1000.00) land in April 2024 through the
// fixture clock, exactly as in the budget_plan scenario.
//
//	tx-apr-food    2024-04-10  expense  40.00  CatFood  PayeeShop  TagWork
//	tx-may-food    2024-05-02  expense  60.00  CatFood  PayeeShop
//	tx-may-nocat   2024-05-21  expense   5.00  (none)
//	tx-apr-salary  2024-04-05  income 1000.00  CatSalary
//	tx-jun-income  2024-06-06  income   50.00  (none)

func init() {
	register(Scenario{Name: "report", Calls: func() []Call {
		const (
			reportBudget = "b0000000-0000-0000-0000-0000000000f1"
			unknownBudg  = "b0000000-0000-0000-0000-0000000000f2"
			txAprFood    = "d0000000-0000-0000-0000-0000000000f1"
			txMayFood    = "d0000000-0000-0000-0000-0000000000f2"
			txMayNoCat   = "d0000000-0000-0000-0000-0000000000f3"
			txAprSalary  = "d0000000-0000-0000-0000-0000000000f4"
			txJunIncome  = "d0000000-0000-0000-0000-0000000000f5"
			win          = "from=2024-04-01&to=2024-06-01"
		)
		get := func(label, path string) Call {
			return Call{Label: label, Method: "GET", Path: path, Auth: "owner"}
		}
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": reportBudget, "name": "Report", "currencyId": USD, "startDate": "2024-04-01", "accountIds": []string{OwnerAccount}}},
			{Label: "set-expense-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": reportBudget, "elementId": CatFood, "period": "2024-05-01", "amount": "300"}},
			{Label: "tx-apr-food", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txAprFood, "accountId": OwnerAccount, "type": "expense", "amount": "40.00",
					"categoryId": CatFood, "payeeId": PayeeShop, "tagId": TagWork, "date": "2024-04-10 10:00:00"}},
			{Label: "tx-may-food", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txMayFood, "accountId": OwnerAccount, "type": "expense", "amount": "60.00",
					"categoryId": CatFood, "payeeId": PayeeShop, "date": "2024-05-02 09:00:00"}},
			{Label: "tx-may-nocat", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txMayNoCat, "accountId": OwnerAccount, "type": "expense", "amount": "5.00", "date": "2024-05-21 12:00:00"}},
			{Label: "tx-apr-salary", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txAprSalary, "accountId": OwnerAccount, "type": "income", "amount": "1000.00",
					"categoryId": CatSalary, "date": "2024-04-05 08:00:00"}},
			{Label: "tx-jun-income", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": txJunIncome, "accountId": OwnerAccount, "type": "income", "amount": "50.00", "date": "2024-06-06 08:00:00"}},

			get("flow-my-category", "/api/v1/report/get-flow-report?"+win),
			get("flow-my-payee", "/api/v1/report/get-flow-report?"+win+"&groupBy=payee"),
			get("flow-my-tag", "/api/v1/report/get-flow-report?"+win+"&groupBy=tag"),
			get("flow-budget", "/api/v1/report/get-flow-report?budgetId="+reportBudget+"&"+win),
			get("net-worth-my", "/api/v1/report/get-net-worth-report?"+win),
			get("net-worth-budget", "/api/v1/report/get-net-worth-report?budgetId="+reportBudget+"&"+win),

			get("err:window-too-long", "/api/v1/report/get-flow-report?from=2000-01-01&to=2024-06-01"),
			get("err:bad-group-by", "/api/v1/report/get-flow-report?"+win+"&groupBy=label"),
			get("err:payee-on-budget-scope", "/api/v1/report/get-flow-report?budgetId="+reportBudget+"&"+win+"&groupBy=payee"),
			get("err:flow-unknown-budget", "/api/v1/report/get-flow-report?budgetId="+unknownBudg+"&"+win),
			get("err:net-worth-unknown-budget", "/api/v1/report/get-net-worth-report?budgetId="+unknownBudg+"&"+win),
			get("err:net-worth-window-too-long", "/api/v1/report/get-net-worth-report?from=2000-01-01&to=2024-06-01"),
		}
	}})
}
