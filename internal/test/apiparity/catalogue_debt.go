package apiparity

import "time"

// Debts run on a weekly schedule whose last payment is a fixed number of days
// after the (wall-clock) ClockTime, so the number of payments left, and with
// it every amount in the golden, does not depend on the day the suite runs.
func init() {
	day := func(offset int) string { return ClockTime.AddDate(0, 0, offset).Format("2006-01-02") }
	month := func(offset int) string {
		return time.Date(ClockTime.Year(), ClockTime.Month()+time.Month(offset), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
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
		var debtID, keyedID string
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

			// A debt keyed on a payee keeps its budget data when deleted, so
			// deleting it with plans needs a confirmation.
			{Label: "delete-whole-debt", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID}},
			{Label: "create-keyed-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(map[string]any{"payeeId": PayeeShop}), CaptureIDInto: &keyedID},
			{Label: "set-debt-limit", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": &keyedID, "period": month(0), "amount": "10"}},
			{Label: "err:set-debt-limit-out-of-range", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": &keyedID, "period": month(3), "amount": "10"}},
			{Label: "err:change-debt-currency", Method: "POST", Path: "/api/v1/budget/change-element-currency", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": &keyedID, "currencyId": USD}},
			{Label: "err:move-debt-into-folder", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": &keyedID, "folderId": BudgetFolder1}},
			{Label: "err:delete-removal-unconfirmed", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": &keyedID}},
			{Label: "delete-confirmed", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": &keyedID, "confirmDataRemoval": true}},
		}
	}})

	// debt_classification_guard: a category, payee or tag a debt uses cannot be
	// deleted, and a merge moves the debt to the target unless that would leave
	// two debts able to match the same payment.
	register(Scenario{Name: "debt_classification_guard", Calls: func() []Call {
		op := func(n string) string { return "d1000000-0000-0000-0000-0000000000" + n }
		var srcCat, dstCat, catA, catB, srcPayee, dstPayee, srcTag, dstTag string
		var debtA, debtB, debtC, debtD, debtE, debtF string
		post := func(label, path string, body map[string]any) Call {
			return Call{Label: label, Method: "POST", Path: path, Auth: "owner", Body: body}
		}
		createCat := func(label, id, name string, into *string) Call {
			c := post(label, "/api/v1/category/create-category", map[string]any{"id": id, "name": name, "type": "expense", "icon": "food"})
			c.CaptureIDInto = into
			return c
		}
		createNamed := func(label, path, id, name string, into *string) Call {
			c := post(label, path, map[string]any{"id": id, "name": name})
			c.CaptureIDInto = into
			return c
		}
		createDebt := func(label string, into *string, extra map[string]any) Call {
			c := post(label, "/api/v1/debt/create-debt", debtBody(extra))
			c.CaptureIDInto = into
			return c
		}
		getDebt := func(label string, id *string) Call {
			return Call{Label: label, Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: id}
		}
		return []Call{
			createCat("create-source-category", op("a1"), "Loan source", &srcCat),
			createCat("create-target-category", op("a2"), "Loan target", &dstCat),
			createCat("create-category-a", op("a3"), "Loan a", &catA),
			createCat("create-category-b", op("a4"), "Loan b", &catB),
			createNamed("create-source-payee", "/api/v1/payee/create-payee", op("b1"), "Bank one", &srcPayee),
			createNamed("create-target-payee", "/api/v1/payee/create-payee", op("b2"), "Bank two", &dstPayee),
			createNamed("create-source-tag", "/api/v1/tag/create-tag", op("c1"), "Loan one", &srcTag),
			createNamed("create-target-tag", "/api/v1/tag/create-tag", op("c2"), "Loan two", &dstTag),

			createDebt("create-debt-whole-source", &debtA, map[string]any{"categoryId": &srcCat}),
			{Label: "err:delete-used-category", Method: "POST", Path: "/api/v1/category/delete-category", Auth: "owner",
				Body: map[string]any{"id": &srcCat, "mode": "delete"}},
			createDebt("create-debt-keyed-target", &debtB, map[string]any{"categoryId": &dstCat, "payeeId": &srcPayee}),
			{Label: "err:merge-category-key-conflict", Method: "POST", Path: "/api/v1/category/merge-category", Auth: "owner",
				Body: map[string]any{"sourceId": &srcCat, "targetId": &dstCat}},
			post("delete-debt-keyed-target", "/api/v1/debt/delete-debt", map[string]any{"id": &debtB}),
			post("merge-category", "/api/v1/category/merge-category", map[string]any{"sourceId": &srcCat, "targetId": &dstCat}),
			getDebt("get-debt-after-category-merge", &debtA),

			createDebt("create-debt-payee-source", &debtC, map[string]any{"categoryId": &catA, "payeeId": &srcPayee}),
			{Label: "err:delete-used-payee", Method: "POST", Path: "/api/v1/payee/delete-payee", Auth: "owner",
				Body: map[string]any{"id": &srcPayee}},
			createDebt("create-debt-payee-target", &debtD, map[string]any{"categoryId": &catA, "payeeId": &dstPayee}),
			{Label: "err:merge-payee-key-conflict", Method: "POST", Path: "/api/v1/payee/merge-payee", Auth: "owner",
				Body: map[string]any{"sourceId": &srcPayee, "targetId": &dstPayee}},
			post("delete-debt-payee-target", "/api/v1/debt/delete-debt", map[string]any{"id": &debtD}),
			post("merge-payee", "/api/v1/payee/merge-payee", map[string]any{"sourceId": &srcPayee, "targetId": &dstPayee}),
			getDebt("get-debt-after-payee-merge", &debtC),

			createDebt("create-debt-tag-source", &debtE, map[string]any{"categoryId": &catB, "tagId": &srcTag}),
			{Label: "err:delete-used-tag", Method: "POST", Path: "/api/v1/tag/delete-tag", Auth: "owner",
				Body: map[string]any{"id": &srcTag}},
			createDebt("create-debt-tag-target", &debtF, map[string]any{"categoryId": &catB, "tagId": &dstTag}),
			{Label: "err:merge-tag-key-conflict", Method: "POST", Path: "/api/v1/tag/merge-tag", Auth: "owner",
				Body: map[string]any{"sourceId": &srcTag, "targetId": &dstTag}},
			post("delete-debt-tag-target", "/api/v1/debt/delete-debt", map[string]any{"id": &debtF}),
			post("merge-tag", "/api/v1/tag/merge-tag", map[string]any{"sourceId": &srcTag, "targetId": &dstTag}),
			getDebt("get-debt-after-tag-merge", &debtE),
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
