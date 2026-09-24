package apiparity

// budget_savings exercises the savings budget-element surface end to end
// (account type=3, the ElementSavings=5 element, structure.savings, and the
// folder/reorder rules) against the real production handler.
//
// Three fresh accounts: an everyday account (default type), a savings account
// created directly with "type":3 (USD, same currency as the budget), and a
// second account created normal then FLIPPED to type 3 via update-account —
// in a custom currency (EUR-like, rate 2 per base USD) so its element
// currency is later changed to USD via change-element-currency, forcing a
// real BulkConvert (not a same-currency pass-through) that both engines must
// compute identically.
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
// set-limit on S1 self-heals BOTH savings element rows via syncElements (see
// getElementSelfHeal), so by the time move-element is exercised both rows
// already exist and can be found by external id.

func init() {
	register(Scenario{Name: "budget_savings", Calls: func() []Call {
		const (
			savingsBudget   = "b0000000-0000-0000-0000-00000000005a"
			opCurrency      = "cc000000-0000-0000-0000-00000000005a"
			opEveryday      = "a0000000-0000-0000-0000-00000000005a"
			opSavingsA      = "a0000000-0000-0000-0000-00000000005b"
			opSavingsB      = "a0000000-0000-0000-0000-00000000005c"
			opBadType       = "a0000000-0000-0000-0000-00000000005d"
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
			{Label: "create-savings-account", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body:          map[string]any{"id": opSavingsA, "name": "Rainy day", "icon": "savings", "currencyId": USD, "folderId": OwnerFolder, "type": 3},
				CaptureIDInto: &savingsAID},
			{Label: "create-future-savings-account", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body:          map[string]any{"id": opSavingsB, "name": "Euro pot", "icon": "euro", "currencyId": &eurID, "folderId": OwnerFolder},
				CaptureIDInto: &savingsBID},
			{Label: "flip-account-to-savings", Method: "POST", Path: "/api/v1/account/update-account", Auth: "owner",
				Body: map[string]any{"id": &savingsBID, "name": "Euro pot", "icon": "euro", "updatedAt": "2024-05-01 00:00:00", "type": 3}},
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": savingsBudget, "name": "Savings Budget", "currencyId": USD, "startDate": "2024-04-01",
					"accountIds": []any{&everydayID, &savingsAID, &savingsBID}}},
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
			// Both savings rows already exist (self-healed by set-limit-savings-a
			// above), so this hits the savings-specific 400 rather than a silent
			// no-op reorder.
			{Label: "err:move-savings-into-folder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": savingsBudget, "id": &savingsAID, "folderId": savingsFolderID, "afterId": nil}},
			{Label: "move-savings-reorder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": savingsBudget, "id": &savingsBID, "folderId": nil, "afterId": nil}},
			{Label: "get-budget-after-move", Method: "GET",
				Path: "/api/v1/budget/get-budget?id=" + savingsBudget + "&date=2024-05-15", Auth: "owner"},
			{Label: "err:create-account-invalid-type", Method: "POST", Path: "/api/v1/account/create-account", Auth: "owner",
				Body: map[string]any{"id": opBadType, "name": "Bad", "icon": "wallet", "currencyId": USD, "folderId": OwnerFolder, "type": 9}},
		}
	}})
}
