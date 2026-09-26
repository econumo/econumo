package apiparity

// budget_savings exercises the savings budget-element surface end to end (the
// budgets_accounts.is_savings membership flag, the ElementSavings=5 element,
// structure.savings, the folder/reorder rules and the removal confirmation)
// against the real production handler.
//
// Three fresh accounts join the budget as ordinary members: an everyday
// account, a USD account and a second one in a custom currency (EUR-like,
// rate 2 per base USD). update-budget then flags the latter two as savings in
// this budget. The EUR one's element currency is changed to USD via
// change-element-currency, forcing a real BulkConvert (not a same-currency
// pass-through) that both engines must compute identically.
//
// Amounts (all dated May 2024, all on the everyday account's transfers out):
//
//	transfer-to-savings-a   150.00 USD -> S1 (USD, no conversion)
//	transfer-to-savings-b    50.00 USD -> S2 (100.00 EUR; element currency USD,
//	                                           100/2 = 50.00 after conversion)
//
// S1 carries a 200 limit for May (budgeted 200.00, spent 150.00, available
// 50.00); S2 carries no limit (budgeted 0.00, spent 50.00 [converted],
// available -50.00).
//
// Flagging syncs both savings element rows, so move-element finds them by
// external id. Turning S1 off is refused while its May plan exists and goes
// through once confirmed, taking the row (and the plan) with it.

func init() {
	register(Scenario{Name: "budget_savings", Calls: func() []Call {
		const (
			savingsBudget   = "b0000000-0000-0000-0000-00000000005a"
			opCurrency      = "cc000000-0000-0000-0000-00000000005a"
			opEveryday      = "a0000000-0000-0000-0000-00000000005a"
			opSavingsA      = "a0000000-0000-0000-0000-00000000005b"
			opSavingsB      = "a0000000-0000-0000-0000-00000000005c"
			opTransferA     = "d0000000-0000-0000-0000-00000000005a"
			opTransferB     = "d0000000-0000-0000-0000-00000000005b"
			savingsFolderID = "bf000000-0000-0000-0000-00000000005a"
		)
		var eurID, everydayID, savingsAID, savingsBID string
		return []Call{
			{Label: "create-eur-currency", Method: "POST", Path: "/api/v1/currency/create-currency", Auth: "owner",
				Body:          map[string]any{"id": opCurrency, "code": "EUR", "name": "Euro", "symbol": "€", "fractionDigits": 2, "rate": "2"},
				CaptureIDInto: &eurID},
			{Label: "create-everyday-account", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body:          map[string]any{"id": opEveryday, "name": "Checking", "icon": "wallet", "currencyId": USD, "folderId": OwnerFolder},
				CaptureIDInto: &everydayID},
			{Label: "create-savings-account-a", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body:          map[string]any{"id": opSavingsA, "name": "Rainy day", "icon": "savings", "currencyId": USD, "folderId": OwnerFolder},
				CaptureIDInto: &savingsAID},
			{Label: "create-savings-account-b", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body:          map[string]any{"id": opSavingsB, "name": "Euro pot", "icon": "euro", "currencyId": &eurID, "folderId": OwnerFolder},
				CaptureIDInto: &savingsBID},
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": savingsBudget, "name": "Savings Budget", "currencyId": USD, "startDate": "2024-04-01",
					"accountIds": []any{&everydayID, &savingsAID, &savingsBID}}},
			{Label: "flag-savings", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": savingsBudget, "name": "Savings Budget", "currencyId": USD,
					"savingsAccountIds": []any{&savingsAID, &savingsBID}}},
			{Label: "set-limit-savings-a", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": savingsBudget, "elementId": &savingsAID, "period": "2024-05-01", "amount": "200"}},
			{Label: "change-savings-b-currency", Method: "POST", Path: "/api/v1/budget/change-element-currency", Auth: "owner",
				Body: map[string]any{"budgetId": savingsBudget, "elementId": &savingsBID, "currencyId": USD}},
			{Label: "transfer-to-savings-a", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": opTransferA, "accountId": &everydayID, "accountRecipientId": &savingsAID, "type": "transfer",
					"amount": "150.00", "amountRecipient": "150.00", "date": "2024-05-10 09:00:00"}},
			{Label: "transfer-to-savings-b", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": opTransferB, "accountId": &everydayID, "accountRecipientId": &savingsBID, "type": "transfer",
					"amount": "50.00", "amountRecipient": "100.00", "date": "2024-05-12 09:00:00"}},
			{Label: "get-budget", Method: "GET",
				Path: "/api/v1/budget/get-budget?id=" + savingsBudget + "&date=2024-05-15", Auth: "owner"},
			{Label: "get-budget-plan", Method: "GET",
				Path: "/api/v1/budget/get-budget-plan?id=" + savingsBudget + "&from=2024-04-01&months=3", Auth: "owner"},
			{Label: "create-folder", Method: "POST", Path: "/api/v1/budget/create-folder", Auth: "owner",
				Body: map[string]any{"budgetId": savingsBudget, "id": savingsFolderID, "name": "Vault"}},
			// Both savings rows exist (synced by flag-savings), so this hits the
			// savings-specific 400 rather than a silent no-op reorder.
			{Label: "err:move-savings-into-folder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": savingsBudget, "id": &savingsAID, "folderId": savingsFolderID, "afterId": nil}},
			{Label: "move-savings-reorder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": savingsBudget, "id": &savingsBID, "folderId": nil, "afterId": nil}},
			{Label: "get-budget-after-move", Method: "GET",
				Path: "/api/v1/budget/get-budget?id=" + savingsBudget + "&date=2024-05-15", Auth: "owner"},
			{Label: "err:savings-off-unconfirmed", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": savingsBudget, "name": "Savings Budget", "currencyId": USD,
					"savingsAccountIds": []any{&savingsBID}}},
			{Label: "savings-off-confirmed", Method: "POST", Path: "/api/v1/budget/update-budget", Auth: "owner",
				Body: map[string]any{"id": savingsBudget, "name": "Savings Budget", "currencyId": USD,
					"savingsAccountIds": []any{&savingsBID}, "confirmSavingsRemoval": true}},
			{Label: "get-budget-after-savings-off", Method: "GET",
				Path: "/api/v1/budget/get-budget?id=" + savingsBudget + "&date=2024-05-15", Auth: "owner"},
		}
	}})
}
