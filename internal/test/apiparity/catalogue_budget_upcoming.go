package apiparity

// budget_upcoming_recurring: get-budget's and get-budget-plan's upcoming
// fields over recurring templates of both participants of a shared budget. Every template is placed
// so the projection never depends on the day of the month: every one is
// monthly and due at ClockTime itself, so exactly one occurrence falls in the
// current month. A fixed date would not do: an overdue monthly template also
// projects this month's own occurrence while that day is still ahead. The
// month must stay out of the request path, which the golden records verbatim,
// so the selected month is the default (current) one plus a fixed past month,
// and the plan window starts at the default (current) month;
// future-month carry-over and overdue collapsing are covered by
// internal/budget/api/upcoming_test.go and the model's projection tests.
func init() {
	register(Scenario{Name: "budget_upcoming_recurring", Calls: func() []Call {
		now := ClockTime.Format("2006-01-02 15:04:05")
		return []Call{
			{Label: "guest-accepts", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"budgetId": Budget}},
			{Label: "guest-adds-account", Method: "POST", Path: "/api/v1/budget/add-account", Auth: "guest",
				Body: map[string]any{"id": Budget, "accountId": SharedAccount}},
			{Label: "owner-rent", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "e0000000-0000-0000-0000-0000000000e1", "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "monthly", "nextPaymentAt": now, "description": "rent",
				}},
			// Tagged, with no real spend and no limit: the tag shows through its
			// projection alone.
			{Label: "owner-tagged-gym", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": "e0000000-0000-0000-0000-0000000000e2", "type": "expense", "amount": "20.00",
					"accountId": OwnerAccount, "categoryId": CatFood, "tagId": TagWork,
					"schedule": "monthly", "nextPaymentAt": now, "description": "gym",
				}},
			{Label: "partner-bill", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "guest",
				Body: map[string]any{
					"id": "e0000000-0000-0000-0000-0000000000e3", "type": "expense", "amount": "30.00",
					"accountId": SharedAccount,
					"schedule":  "monthly", "nextPaymentAt": now, "description": "internet",
				}},
			{Label: "get-budget-current-month", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
			{Label: "get-budget-past-month", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget + "&date=2024-04-01", Auth: "owner"},
			{Label: "get-budget-plan-current-window", Method: "GET", Path: "/api/v1/budget/get-budget-plan?id=" + Budget + "&months=2", Auth: "owner"},
		}
	}})
}
