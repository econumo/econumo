package apiparity

// Uncategorized-element scenario: pins the get-budget wire shape of the
// presentation-only "uncategorized" element (internal/budget/builder_structure_build.go).
// A dedicated scenario rather than an addition to the shared Seed fixture: Seed
// backs nearly every scenario in this catalogue, so a NULL-category transaction
// added there would churn goldens across the board and obscure review. This
// scenario runs on its own fresh DB (like every other scenario - see
// smoke_test.go/NewHarness), so it cannot affect any other golden.
//
// Covers both routing shapes from internal/budget/builder_structure.go's
// CountSpending walk:
//   - an expense with no category and no tag -> the top-level "uncategorized" row
//   - an expense with a tag but no category -> an "uncategorized" child under
//     the tag (the shape delete-category's hard-delete mode leaves behind,
//     internal/category/delete.go, which motivated this feature)

func init() {
	register(Scenario{Name: "budget_uncategorized_element", Calls: func() []Call {
		return []Call{
			{Label: "create-untagged", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "d0000000-0000-0000-0000-0000000000f1", "accountId": OwnerAccount, "type": "expense",
					"amount": "42.00", "date": ClockTime.Format("2006-01-02 15:04:05"),
				}},
			{Label: "create-tagged", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "d0000000-0000-0000-0000-0000000000f2", "accountId": OwnerAccount, "type": "expense",
					"amount": "23.75", "tagId": TagWork, "date": ClockTime.Format("2006-01-02 15:04:05"),
				}},
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
		}
	}})
}
