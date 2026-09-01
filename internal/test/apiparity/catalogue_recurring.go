package apiparity

func init() {
	// Every recurring occurrence here must stay ahead of the harness clock: a
	// posted transaction dated today lands inside the account's "balance as of
	// end of today" window and shifts the golden balances, so a hard-coded
	// calendar date silently rots the goldens the day it arrives.
	futureDate := ClockTime.AddDate(0, 1, 0).Format("2006-01-02 15:04:05")

	register(Scenario{Name: "recurring_crud", Calls: func() []Call {
		const opCreate = "e0000000-0000-0000-0000-0000000000a1"
		var rtID string
		return []Call{
			{Label: "create-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": opCreate, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "monthly", "nextPaymentAt": futureDate,
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

	// The create-from-transaction flow: the source transaction is linked to the
	// new template in the same write (its recurringId points at the template).
	register(Scenario{Name: "recurring_create_from_transaction", Calls: func() []Call {
		const opTx = "e0000000-0000-0000-0000-0000000000c1"
		const opCreate = "e0000000-0000-0000-0000-0000000000c2"
		var txID string
		return []Call{
			{Label: "create-source-transaction", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{
					"id": opTx, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"date": "2026-07-31 10:00:00", "description": "rent",
				}, CaptureIDInto: &txID},
			{Label: "create-recurring-from-transaction", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": opCreate, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "monthly", "nextPaymentAt": futureDate,
					"description": "rent", "sourceTransactionId": &txID,
				}},
			{Label: "transaction-list-after-link", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner"},
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
					"schedule": "monthly", "nextPaymentAt": futureDate,
					"description": "rent",
				}, CaptureIDInto: &rtID},
			{Label: "post-recurring-transaction", Method: "POST", Path: "/api/v1/recurring/post-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"recurringId": &rtID, "id": opTx, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"date": futureDate, "description": "rent",
				}},
			{Label: "recurring-list-after-post", Method: "GET", Path: "/api/v1/recurring/get-recurring-transaction-list", Auth: "owner"},
			{Label: "transaction-list-after-post", Method: "GET", Path: "/api/v1/transaction/get-transaction-list", Auth: "owner"},
		}
	}})

	// post-time labelIds has THREE cases, and the wire has to keep them apart:
	// absent inherits the template's labels, an explicit list replaces them,
	// and an explicit EMPTY list means none (not "inherit"). All three post
	// the same template, so the goldens also show the template's own labels
	// surviving every override.
	register(Scenario{Name: "recurring_post_labels", Calls: func() []Call {
		const opCreate = "e0000000-0000-0000-0000-0000000000d1"
		const opInherit = "e0000000-0000-0000-0000-0000000000d2"
		const opOverride = "e0000000-0000-0000-0000-0000000000d3"
		const opClear = "e0000000-0000-0000-0000-0000000000d4"
		var rtID string
		post := func(label, opID string, body map[string]any) Call {
			full := map[string]any{
				"recurringId": &rtID, "id": opID, "type": "expense", "amount": "50.00",
				"accountId": OwnerAccount, "categoryId": CatFood,
				"date": futureDate, "description": "rent",
			}
			for k, v := range body {
				full[k] = v
			}
			return Call{Label: label, Method: "POST", Path: "/api/v1/recurring/post-recurring-transaction", Auth: "owner", Body: full}
		}
		return []Call{
			{Label: "create-recurring-with-labels", Method: "POST", Path: "/api/v1/recurring/create-recurring-transaction", Auth: "owner",
				Body: map[string]any{
					"id": opCreate, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": CatFood,
					"schedule": "monthly", "nextPaymentAt": futureDate,
					"description": "rent", "labelIds": []string{LabelPersonal, LabelWork},
				}, CaptureIDInto: &rtID},
			post("post-inherits-template-labels", opInherit, nil),
			post("post-overrides-labels", opOverride, map[string]any{"labelIds": []string{LabelWork}}),
			post("post-clears-labels", opClear, map[string]any{"labelIds": []string{}}),
			{Label: "recurring-list-after-posts", Method: "GET", Path: "/api/v1/recurring/get-recurring-transaction-list", Auth: "owner"},
			{Label: "transaction-list-after-posts", Method: "GET", Path: "/api/v1/transaction/get-transaction-list?accountId=" + OwnerAccount, Auth: "owner"},
		}
	}})
}
