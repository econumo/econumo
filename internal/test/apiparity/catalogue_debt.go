package apiparity

// Debts run on a weekly schedule whose last payment is a fixed number of days
// after the (wall-clock) ClockTime, so the number of payments left, and with
// it every amount in the golden, does not depend on the day the suite runs.
func init() {
	day := func(offset int) string { return ClockTime.AddDate(0, 0, offset).Format("2006-01-02") }
	debtBody := func(extra map[string]any) map[string]any {
		body := map[string]any{
			"name": "Car loan", "currencyId": USD, "categoryId": CatFood,
			"balance": "100", "payment": "105", "schedule": "weekly", "lastPaymentDate": day(7),
		}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}

	// debt_crud walks one debt through its life: create, list, get, update the
	// terms (a new snapshot), mark paid off, delete.
	register(Scenario{Name: "debt_crud", Calls: func() []Call {
		var debtID string
		return []Call{
			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			{Label: "list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
			{Label: "get", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "update-terms", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{
					"id": &debtID, "name": "Car loan 2", "categoryId": CatFood, "payeeId": PayeeShop,
					"balance": "200", "payment": "104", "schedule": "weekly", "lastPaymentDate": day(14),
				}},
			{Label: "get-after-update", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "mark-paid-off", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
				Body: map[string]any{"id": &debtID}},
			{Label: "delete", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID}},
			{Label: "list-after-delete", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
			{Label: "err:get-deleted", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
		}
	}})

	// debt_validation pins every coded refusal of the write routes.
	register(Scenario{Name: "debt_validation", Calls: func() []Call {
		const unknownDebt = "d0000000-0000-0000-0000-0000000000ff"
		create := func(label string, extra map[string]any) Call {
			return Call{Label: "err:" + label, Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner", Body: debtBody(extra)}
		}
		var debtID string
		return []Call{
			create("blank-name", map[string]any{"name": " "}),
			create("bad-category-id", map[string]any{"categoryId": "nope"}),
			create("bad-balance", map[string]any{"balance": "-1"}),
			create("zero-payment", map[string]any{"payment": "0"}),
			create("bad-schedule", map[string]any{"schedule": "daily"}),
			create("bad-last-date", map[string]any{"lastPaymentDate": "soon"}),
			create("bad-start-date", map[string]any{"startDate": "1 Jan"}),
			create("last-payment-in-past", map[string]any{"lastPaymentDate": day(-1)}),
			create("payments-dont-cover", map[string]any{"payment": "50"}),
			create("category-not-expense", map[string]any{"categoryId": CatSalary}),
			create("unknown-category", map[string]any{"categoryId": "c0000000-0000-0000-0000-0000000000ee"}),
			create("unknown-payee", map[string]any{"payeeId": "20000000-0000-0000-0000-0000000000ee"}),

			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			create("start-date-taken", nil),
			create("key-conflict", map[string]any{"payeeId": PayeeShop}),
			{Label: "err:update-payment-dont-cover", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{
					"id": &debtID, "name": "Car loan", "categoryId": CatFood, "balance": "100", "payment": "50",
					"schedule": "weekly", "lastPaymentDate": day(7),
				}},
			{Label: "err:update-unknown", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{
					"id": unknownDebt, "name": "x", "categoryId": CatFood, "balance": "100", "payment": "105",
					"schedule": "weekly", "lastPaymentDate": day(7),
				}},
			{Label: "err:update-blank-id", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": "", "name": "x", "categoryId": CatFood, "balance": "100", "payment": "105",
					"schedule": "weekly", "lastPaymentDate": day(7)}},
			{Label: "err:get-unknown", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=" + unknownDebt},
			{Label: "err:get-bad-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=nope"},
			{Label: "err:get-missing-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt"},
			{Label: "err:paid-off-unknown", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
				Body: map[string]any{"id": unknownDebt}},
			{Label: "err:delete-unknown", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": unknownDebt}},
		}
	}})

	// debt_guest_access: a debt belongs to its owner alone; a connected guest
	// gets the same not-found as for an unknown id, on reads and writes.
	register(Scenario{Name: "debt_guest_access", Calls: func() []Call {
		var debtID string
		return []Call{
			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			{Label: "err:guest-get", Method: "GET", Auth: "guest", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "err:guest-update", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "guest",
				Body: map[string]any{
					"id": &debtID, "name": "Mine now", "categoryId": CatFood, "balance": "100", "payment": "105",
					"schedule": "weekly", "lastPaymentDate": day(7),
				}},
			{Label: "err:guest-paid-off", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "guest",
				Body: map[string]any{"id": &debtID}},
			{Label: "err:guest-delete", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "guest",
				Body: map[string]any{"id": &debtID}},
			{Label: "guest-list-empty", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "guest"},
			{Label: "owner-list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
		}
	}})

	// debt_readonly: a lapsed-trial user may read but not write.
	register(Scenario{Name: "debt_readonly", Calls: func() []Call {
		return []Call{
			{Label: "list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "readonly"},
			{Label: "err:create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "readonly", Body: debtBody(nil)},
		}
	}})
}
