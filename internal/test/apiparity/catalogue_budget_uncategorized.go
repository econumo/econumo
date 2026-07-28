package apiparity

// Uncategorized-element scenario: pins the get-budget wire shape of the
// presentation-only "uncategorized" element (internal/budget/builder_structure_build.go)
// and the drill-down into it (internal/budget/txlist.go). A dedicated scenario
// rather than an addition to the shared Seed fixture: Seed backs nearly every
// scenario in this catalogue, so a NULL-category transaction added there would
// churn goldens across the board and obscure review. This scenario runs on its
// own fresh DB (like every other scenario - see smoke_test.go/NewHarness), so
// it cannot affect any other golden.
//
// Covers both routing shapes from internal/budget/builder_structure.go's
// CountSpending walk:
//   - an expense with no category and no tag -> the top-level "uncategorized" row
//   - an expense with a tag but no category -> an "uncategorized" child under
//     the tag (the shape delete-category's hard-delete mode leaves behind,
//     internal/category/delete.go, which motivated this feature)
//
// ...and both drill-down shapes: get-transaction-list with uncategorized=1
// (the top-level row's click target) and with tagId+uncategorized=1 (the tag
// child's click target).
//
// All dates are fixed literals (period = "2024-04-01", matching the
// convention in catalogue_budget_access.go's budget_account_writes scenario)
// rather than derived from the wall-clock ClockTime var: get-transaction-list
// takes periodStart as a literal query-string value, and the harness writes
// the request Path into the golden VERBATIM (smoke_test.go normalizes only
// the response body) - a ClockTime-derived periodStart would bake today's
// date into the committed golden and start failing every month purely from
// clock drift, unrelated to any real regression.

func init() {
	const period = "2024-04-01"
	register(Scenario{Name: "budget_uncategorized_element", Calls: func() []Call {
		return []Call{
			{Label: "create-untagged", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "d0000000-0000-0000-0000-0000000000f1", "accountId": OwnerAccount, "type": "expense",
					"amount": "42.00", "date": period + " 05:00:00",
				}},
			{Label: "create-tagged", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "d0000000-0000-0000-0000-0000000000f2", "accountId": OwnerAccount, "type": "expense",
					"amount": "23.75", "tagId": TagWork, "date": period + " 06:00:00",
				}},
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget + "&date=" + period, Auth: "owner"},
			{Label: "drilldown-top-level-uncategorized", Method: "GET",
				Path: "/api/v1/budget/get-transaction-list?budgetId=" + Budget + "&periodStart=" + period + "&uncategorized=1",
				Auth: "owner"},
			{Label: "drilldown-tag-uncategorized-child", Method: "GET",
				Path: "/api/v1/budget/get-transaction-list?budgetId=" + Budget + "&periodStart=" + period + "&tagId=" + TagWork + "&uncategorized=1",
				Auth: "owner"},
		}
	}})
}
