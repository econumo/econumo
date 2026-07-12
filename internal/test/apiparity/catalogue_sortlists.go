package apiparity

// sort_lists exercises the three sort-*-list routes: a name sort per module
// with a follow-up read, a usage sort (the seeded transactions fall inside the
// 6-month window relative to the frozen clock), and the validation errors that
// freeze the messages.
func init() {
	register(Scenario{Name: "sort_lists", Calls: func() []Call {
		return []Call{
			{Label: "sort-category-name-asc", Method: "POST", Path: "/api/v1/category/sort-category-list", Auth: "owner",
				Body: map[string]any{"by": "name", "direction": "asc"}},
			{Label: "get-category-list-after-sort", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "sort-category-usage-desc", Method: "POST", Path: "/api/v1/category/sort-category-list", Auth: "owner",
				Body: map[string]any{"by": "usage", "direction": "desc", "periodMonths": 6}},
			{Label: "sort-payee-name-asc", Method: "POST", Path: "/api/v1/payee/sort-payee-list", Auth: "owner",
				Body: map[string]any{"by": "name", "direction": "asc"}},
			{Label: "sort-tag-name-desc", Method: "POST", Path: "/api/v1/tag/sort-tag-list", Auth: "owner",
				Body: map[string]any{"by": "name", "direction": "desc"}},
			{Label: "err:sort-bad-by", Method: "POST", Path: "/api/v1/category/sort-category-list", Auth: "owner",
				Body: map[string]any{"by": "color", "direction": "asc"}},
			{Label: "err:sort-usage-no-period", Method: "POST", Path: "/api/v1/category/sort-category-list", Auth: "owner",
				Body: map[string]any{"by": "usage", "direction": "asc"}},
		}
	}})
}
