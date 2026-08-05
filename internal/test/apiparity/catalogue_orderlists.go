package apiparity

// order_lists exercises the move-{category,tag,payee,label} routes: one relative
// move per module, plus a closing read that must reflect the new order (catching
// an engine difference in the ORDER BY or the key write that a write-only
// assertion would miss). The scenario name is kept so its golden file stays put.
func init() {
	register(Scenario{Name: "order_lists", Calls: func() []Call {
		return []Call{
			// Anchored on a real sibling, not nil: a null anchor exercises the
			// "move to front" path only, and would not catch an afterId that
			// fails to resolve (which silently appends).
			{Label: "move-category", Method: "POST", Path: "/api/v1/category/move-category", Auth: "owner",
				Body: map[string]any{"id": CatFood, "afterId": CatSalary}},
			{Label: "get-category-list-after", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "move-tag", Method: "POST", Path: "/api/v1/tag/move-tag", Auth: "owner",
				Body: map[string]any{"id": TagWork, "afterId": nil}},
			{Label: "move-payee", Method: "POST", Path: "/api/v1/payee/move-payee", Auth: "owner",
				Body: map[string]any{"id": PayeeShop, "afterId": nil}},
			{Label: "move-label", Method: "POST", Path: "/api/v1/label/move-label", Auth: "owner",
				Body: map[string]any{"id": LabelWork, "afterId": nil}},
			// The bulk counterpart: a whole-list reorder, which no single
			// relative move can express (the A-Z action in settings).
			{Label: "sort-category-list", Method: "POST", Path: "/api/v1/category/sort-category-list", Auth: "owner",
				Body: map[string]any{"ids": []string{CatFood, CatSalary}}},
			{Label: "sort-tag-list", Method: "POST", Path: "/api/v1/tag/sort-tag-list", Auth: "owner",
				Body: map[string]any{"ids": []string{TagWork}}},
			{Label: "sort-payee-list", Method: "POST", Path: "/api/v1/payee/sort-payee-list", Auth: "owner",
				Body: map[string]any{"ids": []string{PayeeShop}}},
			{Label: "sort-label-list", Method: "POST", Path: "/api/v1/label/sort-label-list", Auth: "owner",
				Body: map[string]any{"ids": []string{LabelWork}}},
		}
	}})
}
