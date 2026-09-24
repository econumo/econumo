package apiparity

// Period-boundary scenario: a transaction dated exactly 00:00 on the 1st (what
// every date-only CSV import row is) counts once, in that month's flows, and a
// month's startBalance equals the previous month's endBalance. The start
// balance used spent_at <= start while the flows use spent_at >= start, so the
// midnight row was counted twice; the golden pins the corrected figures on both
// engines.

func init() {
	register(Scenario{Name: "budget_period_chain", Calls: func() []Call {
		const (
			chainBudget   = "b0000000-0000-0000-0000-0000000000c2"
			aprilIncome   = "d0000000-0000-0000-0000-0000000000c3"
			midnightFirst = "d0000000-0000-0000-0000-0000000000c4"
		)
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": chainBudget, "name": "Chain", "currencyId": USD, "startDate": "2024-04-01", "accountIds": []string{OwnerAccount}}},
			{Label: "create-april-income", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": aprilIncome, "accountId": OwnerAccount, "type": "income", "amount": "100", "date": "2024-04-10 10:00:00"}},
			{Label: "create-midnight-on-the-first", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": midnightFirst, "accountId": OwnerAccount, "type": "income", "amount": "40", "date": "2024-05-01 00:00:00"}},
			// April ends at 100; May starts at 100 and its income is the 40.
			{Label: "get-budget-april", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + chainBudget + "&date=2024-04-15", Auth: "owner"},
			{Label: "get-budget-may", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + chainBudget + "&date=2024-05-15", Auth: "owner"},
		}
	}})
}
