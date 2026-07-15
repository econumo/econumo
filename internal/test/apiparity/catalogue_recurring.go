package apiparity

func init() {
	register(Scenario{Name: "recurring_crud", Calls: func() []Call {
		const opCreate = "e0000000-0000-0000-0000-0000000000a1"
		var rtID string
		return []Call{
			{Label: "create-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": opCreate, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "monthly", "nextPaymentAt": "2026-08-31 00:00:00",
					"description": "rent",
				}, CaptureIDInto: &rtID},
			{Label: "list-after-create", Method: "GET", Path: "/api/v1/recurring/get-recurring-transaction-list", Auth: "owner"},
			{Label: "update-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/update-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": &rtID, "type": "expense", "amount": "60.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "weekly", "nextPaymentAt": "2026-09-05 00:00:00",
					"description": "rent updated",
				}},
			{Label: "skip-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/skip-recurring-transaction", Auth: "owner",
				Body: map[string]any{"id": &rtID}},
			{Label: "delete-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/delete-recurring-transaction", Auth: "owner",
				Body: map[string]any{"id": &rtID}},
			{Label: "list-after-delete", Method: "GET", Path: "/api/v1/recurring/get-recurring-transaction-list", Auth: "owner"},
		}
	}})

	register(Scenario{Name: "recurring_post", Calls: func() []Call {
		const opCreate = "e0000000-0000-0000-0000-0000000000b1"
		const opTx = "e0000000-0000-0000-0000-0000000000b2"
		var rtID string
		return []Call{
			{Label: "create-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": opCreate, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "monthly", "nextPaymentAt": "2026-08-31 00:00:00",
					"description": "rent",
				}, CaptureIDInto: &rtID},
			{Label: "post-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/post-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"recurringId": &rtID, "id": opTx, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"date": "2026-08-31 00:00:00", "description": "rent",
				}},
			{Label: "recurring-list-after-post", Method: "GET", Path: "/api/v1/recurring/get-recurring-transaction-list", Auth: "owner"},
			{Label: "transaction-list-after-post", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner"},
		}
	}})
}
