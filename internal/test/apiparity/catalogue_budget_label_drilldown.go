package apiparity

// Label drill-down: pins get-transaction-list?labelId=... (internal/budget/txlist.go),
// the click target for a structure.labels chip. One expense carries BOTH
// LabelWork and LabelPersonal (the many-to-many link, not a single tag_id
// column), a second carries only LabelWork - so drilling into LabelWork must
// return both transactions while LabelPersonal returns only the shared one,
// directly exercising the transactions_labels JOIN rather than a query that
// ignores the label id. A missing ORDER BY on this JOIN would return
// SQLite-vs-PostgreSQL insertion order rather than a deterministic order (see
// budget_labels_populated's history), so this scenario also runs under
// enginecompare.
//
// period is a fixed literal (see budget_uncategorized_element.go's rationale)
// so the committed golden does not bake in today's date.
func init() {
	const period = "2024-04-01"
	register(Scenario{Name: "budget_label_drilldown", Calls: func() []Call {
		return []Call{
			{Label: "create-double-label-expense", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "d0000000-0000-0000-0000-0000000000fd", "accountId": OwnerAccount, "type": "expense",
					"amount": "50.00", "date": period + " 09:00:00",
					"labelIds": []string{LabelWork, LabelPersonal},
				}},
			{Label: "create-work-only-expense", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "d0000000-0000-0000-0000-0000000000fc", "accountId": OwnerAccount, "type": "expense",
					"amount": "10.00", "date": period + " 10:00:00",
					"labelIds": []string{LabelWork},
				}},
			{Label: "drilldown-work", Method: "GET",
				Path: "/api/v1/budget/get-transaction-list?budgetId=" + Budget + "&periodStart=" + period + "&labelId=" + LabelWork,
				Auth: "owner"},
			{Label: "drilldown-personal", Method: "GET",
				Path: "/api/v1/budget/get-transaction-list?budgetId=" + Budget + "&periodStart=" + period + "&labelId=" + LabelPersonal,
				Auth: "owner"},
		}
	}})
}
