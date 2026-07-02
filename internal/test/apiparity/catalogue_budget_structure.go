package apiparity

// Budget-structure scenario: folder + envelope CRUD, folder ordering, element
// move/currency-change, then a closing get-budget read pinning the resulting
// structure — the 10 routes carved out of missingFromCatalogue by this file
// (9 structure writes + the GET /api/v1/budget/get-budget read, which had no
// prior coverage anywhere in the catalogue).

func init() {
	register(Scenario{Name: "budget_structure_writes", Calls: func() []Call {
		const (
			newFolder   = "bf000000-0000-0000-0000-0000000000aa"
			newEnvelope = "be000000-0000-0000-0000-0000000000aa"
		)
		return []Call{
			{Label: "create-folder", Method: "POST", Path: "/api/v1/budget/create-folder", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": newFolder, "name": "Bills"}},
			{Label: "update-folder", Method: "POST", Path: "/api/v1/budget/update-folder", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": newFolder, "name": "Bills 2"}},
			{Label: "order-folder-list", Method: "POST", Path: "/api/v1/budget/order-folder-list", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "items": []map[string]any{{"id": BudgetFolder1, "position": 0}, {"id": newFolder, "position": 1}}}},
			// Element identification (verified against internal/app/budget/move.go and
			// accounts.go's ChangeElementCurrency before writing this scenario):
			// move-element-list and change-element-currency both key elements by their
			// EXTERNAL id (category/tag/envelope id), not the budgets_elements row id —
			// so CatFood works directly for both calls below.
			{Label: "create-envelope", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": newEnvelope, "name": "Groceries", "icon": "cart",
					"currencyId": USD, "folderId": newFolder, "categories": []string{}}},
			{Label: "update-envelope", Method: "POST", Path: "/api/v1/budget/update-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": Envelope1, "name": "Envelope 2", "icon": "cart",
					"currencyId": USD, "isArchived": 0, "categories": []string{CatSalary}}},
			{Label: "move-element-list", Method: "POST", Path: "/api/v1/budget/move-element-list", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "items": []map[string]any{{"id": CatFood, "folderId": BudgetFolder1, "position": 0}}}},
			{Label: "change-element-currency", Method: "POST", Path: "/api/v1/budget/change-element-currency", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": CatFood, "currencyId": USD}},
			{Label: "delete-envelope", Method: "POST", Path: "/api/v1/budget/delete-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": Envelope1}},
			{Label: "delete-folder", Method: "POST", Path: "/api/v1/budget/delete-folder", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": newFolder}},
			{Label: "get-budget-after", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
		}
	}})
}
