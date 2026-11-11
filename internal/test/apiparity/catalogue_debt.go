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
			"amountBorrowed": "100", "startDate": day(0), "payment": "105", "schedule": "weekly", "lastPaymentDate": day(7),
		}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	// withoutTerms leaves the loan's facts (amount borrowed, start date, last
	// payment date) out: the debt is created not set up.
	withoutTerms := func(extra map[string]any) map[string]any {
		body := debtBody(extra)
		delete(body, "amountBorrowed")
		delete(body, "startDate")
		delete(body, "lastPaymentDate")
		return body
	}

	// debt_crud walks one debt through its life: create, list, get, correct
	// the loan's facts (the origin rewritten), state a balance now (a new
	// snapshot), rename without terms, mark paid off, delete. A second debt is
	// created without terms, matching by a label, and then set up.
	register(Scenario{Name: "debt_crud", Calls: func() []Call {
		var debtID, draftID string
		return []Call{
			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			{Label: "list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
			{Label: "get", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "update-terms", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{
					"id": &debtID, "name": "Car loan 2", "categoryId": CatFood, "payeeId": PayeeShop, "startDate": day(0),
					"amountBorrowed": "200", "payment": "104", "schedule": "weekly", "lastPaymentDate": day(14),
				}},
			{Label: "get-after-update", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "update-balance-now", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan 2", "categoryId": CatFood, "payeeId": PayeeShop,
					"startDate": day(0), "balance": "150"}},
			// no amount borrowed, balance, payment, schedule or last payment
			// date: the terms stay
			{Label: "rename-keeps-terms", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan 3", "categoryId": CatFood, "payeeId": PayeeShop, "startDate": day(0)}},
			{Label: "mark-paid-off", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
				Body: map[string]any{"id": &debtID}},
			{Label: "delete", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID}},
			{Label: "list-after-delete", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
			{Label: "err:get-deleted", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},

			{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: withoutTerms(map[string]any{"name": "Student loan", "labelId": LabelWork}), CaptureIDInto: &draftID},
			{Label: "get-not-set-up", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &draftID},
			{Label: "update-payment-only", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &draftID, "name": "Student loan", "categoryId": CatFood, "labelId": LabelWork,
					"payment": "110", "schedule": "weekly"}},
			{Label: "err:paid-off-not-set-up", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
				Body: map[string]any{"id": &draftID}},
			{Label: "finish-setup", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &draftID, "name": "Student loan", "categoryId": CatFood, "labelId": LabelWork,
					"amountBorrowed": "100", "startDate": day(0), "lastPaymentDate": day(7)}},
			{Label: "get-after-setup", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &draftID},
		}
	}})

	// debt_currency_change moves a set-up debt with a payment and a plan from
	// USD to a custom currency at 2 per USD, without its terms: the debt keeps
	// its numbers, its 50 USD payment reads 100 and is split again (made on
	// the start day, it is all principal and pays the 100 off), and the budget
	// row turns to the new currency with its plan of 105 kept.
	register(Scenario{Name: "debt_currency_change", Calls: func() []Call {
		const (
			opCurrency = "cc000000-0000-0000-0000-0000000000d1"
			opCategory = "d3000000-0000-0000-0000-0000000000a1"
			opPayment  = "d3000000-0000-0000-0000-0000000000b1"
		)
		now := ClockTime.Format("2006-01-02 15:04:05")
		var eurID, categoryID, debtID string
		return []Call{
			{Label: "create-eur-currency", Method: "POST", Path: "/api/v1/currency/create-currency", Auth: "owner",
				Body:          map[string]any{"id": opCurrency, "code": "EUR", "name": "Euro", "symbol": "€", "fractionDigits": 2, "rate": "2"},
				CaptureIDInto: &eurID},
			{Label: "create-loan-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": opCategory, "name": "Car loan", "type": "expense", "icon": "car"},
				CaptureIDInto: &categoryID},
			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(map[string]any{"categoryId": &categoryID}), CaptureIDInto: &debtID},
			{Label: "pay", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": opPayment, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": &categoryID, "date": now, "description": "loan"}},
			{Label: "plan", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": &debtID, "period": month(0), "amount": "105"}},
			{Label: "change-currency", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan", "currencyId": &eurID, "categoryId": &categoryID, "startDate": day(0)}},
			{Label: "get-after-change", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "get-budget-after-change", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
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
			create("zero-amount-borrowed", map[string]any{"amountBorrowed": "0"}),
			create("zero-payment", map[string]any{"payment": "0"}),
			create("bad-schedule", map[string]any{"schedule": "daily"}),
			create("bad-last-date", map[string]any{"lastPaymentDate": "soon"}),
			create("bad-start-date", map[string]any{"startDate": "1 Jan"}),
			create("last-payment-before-start", map[string]any{"lastPaymentDate": day(-1)}),
			create("start-in-future", map[string]any{"startDate": day(1)}),
			create("balance-now-after-last-payment", map[string]any{"startDate": day(-14), "lastPaymentDate": day(-7), "balance": "10"}),
			create("payments-dont-cover", map[string]any{"payment": "50"}),
			create("category-not-expense", map[string]any{"categoryId": CatSalary}),
			create("unknown-category", map[string]any{"categoryId": "c0000000-0000-0000-0000-0000000000ee"}),
			create("unknown-payee", map[string]any{"payeeId": "20000000-0000-0000-0000-0000000000ee"}),
			create("unknown-label", map[string]any{"labelId": "30000000-0000-0000-0000-0000000000ee"}),
			create("loan-facts-missing-last-date", map[string]any{"lastPaymentDate": ""}),
			create("loan-facts-missing-amount-borrowed", map[string]any{"amountBorrowed": ""}),
			create("loan-facts-missing-start-date", map[string]any{"startDate": nil}),
			create("tag-and-label", map[string]any{"tagId": TagWork, "labelId": LabelWork}),

			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			create("start-date-taken", nil),
			create("key-conflict", map[string]any{"payeeId": PayeeShop}),
			// a label tells two debts apart only when both name one
			create("label-key-conflict", map[string]any{"labelId": LabelWork}),
			{Label: "err:update-payment-dont-cover", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{
					"id": &debtID, "name": "Car loan", "categoryId": CatFood, "startDate": day(0), "amountBorrowed": "100", "payment": "50",
					"schedule": "weekly", "lastPaymentDate": day(7),
				}},
			{Label: "err:update-unknown", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{
					"id": unknownDebt, "name": "x", "categoryId": CatFood, "startDate": day(0), "amountBorrowed": "100", "payment": "105",
					"schedule": "weekly", "lastPaymentDate": day(7),
				}},
			{Label: "err:update-blank-id", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": "", "name": "x", "categoryId": CatFood, "startDate": day(0), "amountBorrowed": "100", "payment": "105",
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

	// debt_classification_guard: a category, payee, tag or label a debt uses
	// cannot be deleted, and a merge moves the debt to the target unless that
	// would leave two debts able to match the same payment.
	register(Scenario{Name: "debt_classification_guard", Calls: func() []Call {
		op := func(n string) string { return "d1000000-0000-0000-0000-0000000000" + n }
		var srcCat, dstCat, catA, catB, catC, srcPayee, dstPayee, srcTag, dstTag, srcLabel, dstLabel string
		var debtA, debtB, debtC, debtD, debtE, debtF, debtG, debtH string
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
			createCat("create-category-c", op("a5"), "Loan c", &catC),
			createNamed("create-source-payee", "/api/v1/payee/create-payee", op("b1"), "Bank one", &srcPayee),
			createNamed("create-target-payee", "/api/v1/payee/create-payee", op("b2"), "Bank two", &dstPayee),
			createNamed("create-source-tag", "/api/v1/tag/create-tag", op("c1"), "Loan one", &srcTag),
			createNamed("create-target-tag", "/api/v1/tag/create-tag", op("c2"), "Loan two", &dstTag),
			createNamed("create-source-label", "/api/v1/label/create-label", op("e1"), "Debt car", &srcLabel),
			createNamed("create-target-label", "/api/v1/label/create-label", op("e2"), "Debt student", &dstLabel),

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

			createDebt("create-debt-label-source", &debtG, map[string]any{"categoryId": &catC, "labelId": &srcLabel}),
			{Label: "err:delete-used-label", Method: "POST", Path: "/api/v1/label/delete-label", Auth: "owner",
				Body: map[string]any{"id": &srcLabel}},
			createDebt("create-debt-label-target", &debtH, map[string]any{"categoryId": &catC, "labelId": &dstLabel}),
			{Label: "err:merge-label-key-conflict", Method: "POST", Path: "/api/v1/label/merge-label", Auth: "owner",
				Body: map[string]any{"sourceId": &srcLabel, "targetId": &dstLabel}},
			post("delete-debt-label-target", "/api/v1/debt/delete-debt", map[string]any{"id": &debtH}),
			post("merge-label", "/api/v1/label/merge-label", map[string]any{"sourceId": &srcLabel, "targetId": &dstLabel}),
			getDebt("get-debt-after-label-merge", &debtG),
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
					"id": &debtID, "name": "Mine now", "categoryId": CatFood, "startDate": day(0), "amountBorrowed": "100", "payment": "105",
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

	// debt_forecast lists a debt's payments still to come. The weekly debt has
	// one due left, so the golden does not depend on the day the suite runs; a
	// budget whose row of the debt plans nothing, or one the caller cannot
	// name, leaves the regular payments and names no budget. Planned extras
	// are pinned by the debt service's tests, whose clock is fixed.
	register(Scenario{Name: "debt_forecast", Calls: func() []Call {
		const unknownDebt = "d0000000-0000-0000-0000-0000000000ff"
		const unknownBudget = "b0000000-0000-0000-0000-0000000000ff"
		var debtID, draftID, categoryID string
		forecast := func(label, auth string, query string, id *string) Call {
			return Call{Label: label, Method: "GET", Auth: auth, Path: "/api/v1/debt/get-debt-forecast?" + query + "id=", PathID: id}
		}
		return []Call{
			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			forecast("forecast", "owner", "", &debtID),
			{Label: "get-budget", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
			forecast("forecast-from-budget", "owner", "budgetId="+Budget+"&", &debtID),
			forecast("forecast-unknown-budget", "owner", "budgetId="+unknownBudget+"&", &debtID),
			{Label: "create-loan-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": "d4000000-0000-0000-0000-0000000000a1", "name": "Student loan", "type": "expense", "icon": "school"},
				CaptureIDInto: &categoryID},
			{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: withoutTerms(map[string]any{"name": "Student loan", "categoryId": &categoryID}), CaptureIDInto: &draftID},
			forecast("forecast-not-set-up", "owner", "", &draftID),
			forecast("err:guest-forecast", "guest", "", &debtID),
			{Label: "err:forecast-unknown", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-forecast?id=" + unknownDebt},
			{Label: "err:forecast-bad-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-forecast?id=nope"},
			{Label: "err:forecast-missing-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-forecast"},
			forecast("err:forecast-bad-budget-id", "owner", "budgetId=nope&", &debtID),
		}
	}})

	// debt_reconcile: a loan of 300 taken two weeks ago (three weekly dues of
	// 105 left from then) is reconciled with statements: today's sets what is
	// owed, one three days ago lands between, a second one on that date
	// replaces it, and deleting it splits the payments again from the origin.
	// The dates and the payment fall on the fixed weekly offsets, so the
	// figures do not depend on the day the suite runs.
	register(Scenario{Name: "debt_reconcile", Calls: func() []Call {
		const unknownDebt = "d0000000-0000-0000-0000-0000000000ff"
		var debtID, draftID, statementID, categoryID string
		reconcile := func(label string, body map[string]any) Call {
			return Call{Label: label, Method: "POST", Path: "/api/v1/debt/reconcile-debt", Auth: "owner", Body: body}
		}
		return []Call{
			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(map[string]any{"amountBorrowed": "300", "startDate": day(-14)}), CaptureIDInto: &debtID},
			reconcile("reconcile-today", map[string]any{"id": &debtID, "balance": "100"}),
			reconcile("reconcile-earlier", map[string]any{"id": &debtID, "balance": "200", "balanceDate": day(-3)}),
			reconcile("reconcile-same-date-replaces", map[string]any{"id": &debtID, "balance": "190", "balanceDate": day(-3)}),
			{Label: "get", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID,
				CaptureIDInto: &statementID, CapturePath: "data.snapshots.1.id"},
			reconcile("err:before-start", map[string]any{"id": &debtID, "balance": "100", "balanceDate": day(-15)}),
			reconcile("err:after-today", map[string]any{"id": &debtID, "balance": "100", "balanceDate": day(1)}),
			reconcile("err:bad-balance", map[string]any{"id": &debtID, "balance": "-1"}),
			reconcile("err:unknown", map[string]any{"id": unknownDebt, "balance": "1"}),
			{Label: "err:correction-conflicts", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan", "categoryId": CatFood, "startDate": day(-2),
					"amountBorrowed": "200", "lastPaymentDate": day(7)}},
			{Label: "delete-reconciliation", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "owner",
				Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
			{Label: "err:delete-reconciliation-again", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "owner",
				Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
			{Label: "get-after-delete", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "err:guest-reconcile", Method: "POST", Path: "/api/v1/debt/reconcile-debt", Auth: "guest",
				Body: map[string]any{"id": &debtID, "balance": "1"}},
			{Label: "create-loan-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": "d5000000-0000-0000-0000-0000000000a1", "name": "Student loan", "type": "expense", "icon": "school"},
				CaptureIDInto: &categoryID},
			{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: withoutTerms(map[string]any{"name": "Student loan", "categoryId": &categoryID}), CaptureIDInto: &draftID},
			reconcile("err:not-set-up", map[string]any{"id": &draftID, "balance": "1"}),
			{Label: "err:readonly-reconcile", Method: "POST", Path: "/api/v1/debt/reconcile-debt", Auth: "readonly",
				Body: map[string]any{"id": unknownDebt, "balance": "1"}},
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
