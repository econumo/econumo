package apiparity

// budget_comments exercises the four comment routes end to end: the owner posts
// on a category cell, a guest posts on the same cell, the owner edits their own
// comment and moderates the guest's, and the list read returns the window. The
// err: calls pin the refusals: a guest editing someone else's comment, an
// unknown comment id, and a period before the budget's start month.
func init() {
	register(Scenario{Name: "budget_comments", Calls: func() []Call {
		const (
			commentBudget  = "b0000000-0000-0000-0000-0000000000c7"
			ownerComment   = "c0000000-0000-0000-0000-0000000000c1"
			guestComment   = "c0000000-0000-0000-0000-0000000000c2"
			unknownComment = "c0000000-0000-0000-0000-0000000000ff"
		)
		return []Call{
			{Label: "create-budget", Method: "POST", Path: "/api/v1/budget/create-budget", Auth: "owner",
				Body: map[string]any{"id": commentBudget, "name": "Comments", "currencyId": USD, "startDate": "2024-04-01", "accountIds": []string{OwnerAccount}}},
			{Label: "share-with-guest", Method: "POST", Path: "/api/v1/budget/grant-access", Auth: "owner",
				Body: map[string]any{"budgetId": commentBudget, "userId": GuestID, "role": "guest"}},
			{Label: "guest-accepts", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"budgetId": commentBudget}},

			{Label: "owner-posts", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "owner",
				Body: map[string]any{"id": ownerComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-05-01", "comment": "Trip to Lisbon"}},
			{Label: "owner-posts-retry", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "owner",
				Body: map[string]any{"id": ownerComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-05-01", "comment": "Trip to Lisbon"}},
			{Label: "guest-posts", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "guest",
				Body: map[string]any{"id": guestComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-05-01", "comment": "Book the flights"}},

			{Label: "owner-edits-own", Method: "POST", Path: "/api/v1/budget/update-comment", Auth: "owner",
				Body: map[string]any{"id": ownerComment, "comment": "Trip to Lisbon in May"}},
			{Label: "err:guest-edits-owners", Method: "POST", Path: "/api/v1/budget/update-comment", Auth: "guest",
				Body: map[string]any{"id": ownerComment, "comment": "not mine"}},
			{Label: "err:unknown-comment", Method: "POST", Path: "/api/v1/budget/update-comment", Auth: "owner",
				Body: map[string]any{"id": unknownComment, "comment": "nobody home"}},
			{Label: "err:period-before-start", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "owner",
				Body: map[string]any{"id": unknownComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-01-01", "comment": "too early"}},
			{Label: "err:blank-comment", Method: "POST", Path: "/api/v1/budget/create-comment", Auth: "owner",
				Body: map[string]any{"id": unknownComment, "budgetId": commentBudget, "elementId": CatFood, "period": "2024-05-01", "comment": "   "}},

			{Label: "list-window", Method: "GET", Auth: "owner",
				Path: "/api/v1/budget/get-comment-list?budgetId=" + commentBudget + "&from=2024-05-01&months=2"},
			{Label: "err:list-bad-months", Method: "GET", Auth: "owner",
				Path: "/api/v1/budget/get-comment-list?budgetId=" + commentBudget + "&from=2024-05-01&months=99"},

			{Label: "owner-moderates-guest", Method: "POST", Path: "/api/v1/budget/delete-comment", Auth: "owner",
				Body: map[string]any{"id": guestComment}},
			{Label: "list-after-delete", Method: "GET", Auth: "owner",
				Path: "/api/v1/budget/get-comment-list?budgetId=" + commentBudget + "&from=2024-05-01&months=2"},
		}
	}})
}
