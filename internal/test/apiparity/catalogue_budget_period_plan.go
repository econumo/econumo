package apiparity

// A day-28 budget read over shifted periods: key 2024-04 covers
// [2024-03-28, 2024-04-28), key 2024-05 [2024-04-28, 2024-05-28). Every pair
// below straddles a boundary by one second, so get-budget and get-budget-plan
// must split them the same way.
//
//	tx-food-before     2024-04-27 23:59:59  expense  30.00  CatFood   key 2024-04
//	tx-food-after      2024-04-28 00:00:00  expense  20.00  CatFood   key 2024-05
//	tx-salary-before   2024-05-27 23:59:59  income  500.00  CatSalary key 2024-05
//	tx-salary-after    2024-05-28 00:00:00  income  700.00  CatSalary key 2024-06
//	tx-out-before      2024-05-27 12:00:00  transfer 15.00  out       key 2024-05
//	tx-out-after       2024-05-28 00:00:00  transfer 25.00  out       key 2024-06
//
// The fixture's own Txn1/Txn2 sit at 2024-04-01, in key 2024-04.

func init() {
	register(Scenario{Name: "budget_period_plan", Calls: func() []Call {
		const (
			budget    = "b0000000-0000-0000-0000-0000000000f1"
			opOutside = "a0000000-0000-0000-0000-0000000000f2"
		)
		var outsideID string
		expense := func(label, id, amount, date string) Call {
			return Call{Label: label, Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": id, "accountId": OwnerAccount, "type": "expense", "amount": amount, "categoryId": CatFood, "date": date}}
		}
		income := func(label, id, amount, date string) Call {
			return Call{Label: label, Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": id, "accountId": OwnerAccount, "type": "income", "amount": amount, "categoryId": CatSalary, "date": date}}
		}
		out := func(label, id, amount, date string) Call {
			return Call{Label: label, Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": id, "accountId": OwnerAccount, "accountRecipientId": &outsideID, "type": "transfer",
					"amount": amount, "amountRecipient": amount, "date": date}}
		}
		return []Call{
			{Label: "create-budget-day-28", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": budget, "name": "Payday", "currencyId": USD, "startDate": "2024-04-01", "periodStartDay": 28, "accountIds": []string{OwnerAccount}}},
			{Label: "set-limit-may", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": budget, "elementId": CatFood, "period": "2024-05-01", "amount": "200"}},
			{Label: "create-outside-account", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body: map[string]any{"id": opOutside, "name": "Outside", "icon": "bank", "currencyId": USD, "folderId": OwnerFolder}, CaptureIDInto: &outsideID},
			{Label: "remove-outside-account", Method: "POST", Path: "/api/v1/budget/remove-account", Auth: "owner",
				Body: map[string]any{"id": budget, "accountId": &outsideID}},
			expense("tx-food-before", "d0000000-0000-0000-0000-0000000000f3", "30.00", "2024-04-27 23:59:59"),
			expense("tx-food-after", "d0000000-0000-0000-0000-0000000000f4", "20.00", "2024-04-28 00:00:00"),
			income("tx-salary-before", "d0000000-0000-0000-0000-0000000000f5", "500.00", "2024-05-27 23:59:59"),
			income("tx-salary-after", "d0000000-0000-0000-0000-0000000000f6", "700.00", "2024-05-28 00:00:00"),
			out("tx-out-before", "d0000000-0000-0000-0000-0000000000f7", "15.00", "2024-05-27 12:00:00"),
			out("tx-out-after", "d0000000-0000-0000-0000-0000000000f8", "25.00", "2024-05-28 00:00:00"),
			{Label: "get-budget-april", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + budget + "&date=2024-04-15", Auth: "owner"},
			{Label: "get-budget-may", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + budget + "&date=2024-05-01", Auth: "owner"},
			{Label: "get-budget-plan", Method: "GET", Path: "/api/v1/budget/get-budget-plan?id=" + budget + "&from=2024-04-01&months=3", Auth: "owner"},
		}
	}})
}
