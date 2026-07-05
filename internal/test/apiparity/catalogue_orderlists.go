package apiparity

// order_lists exercises the order-{category,tag,payee}-list routes: a
// position-swap write per module, plus a closing read that must reflect the
// new order (catching an engine difference in the ORDER BY/position update
// that a write-only assertion would miss).
func init() {
	register(Scenario{Name: "order_lists", Calls: func() []Call {
		return []Call{
			{Label: "order-category-list", Method: "POST", Path: "/api/v1/category/order-category-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{{"id": CatFood, "position": 1}, {"id": CatSalary, "position": 0}}}},
			{Label: "get-category-list-after", Method: "GET", Path: "/api/v1/category/get-category-list", Auth: "owner", Body: map[string]any{}},
			{Label: "order-tag-list", Method: "POST", Path: "/api/v1/tag/order-tag-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{{"id": TagWork, "position": 0}}}},
			{Label: "order-payee-list", Method: "POST", Path: "/api/v1/payee/order-payee-list", Auth: "owner",
				Body: map[string]any{"changes": []map[string]any{{"id": PayeeShop, "position": 0}}}},
		}
	}})
}
