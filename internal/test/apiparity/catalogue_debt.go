package apiparity

import (
	"time"
)

// Debts run on a weekly schedule whose last payment is a fixed number of days
// after the (wall-clock) ClockTime, so the number of payments left, and with
// it every amount in the golden, does not depend on the day the suite runs.
// A debt is created on an element not set up; the loan's facts come with
// update-debt, so a set-up debt takes two calls.
func init() {
	day := func(offset int) string { return ClockTime.AddDate(0, 0, offset).Format("2006-01-02") }
	month := func(offset int) string {
		return time.Date(ClockTime.Year(), ClockTime.Month()+time.Month(offset), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	merge := func(body, extra map[string]any) map[string]any {
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	debtBody := func(extra map[string]any) map[string]any {
		return merge(map[string]any{"budgetId": Budget, "elementId": CatFood, "payment": "105", "schedule": "weekly"}, extra)
	}
	setupBody := func(id *string, extra map[string]any) map[string]any {
		return merge(map[string]any{"id": id, "amountBorrowed": "100", "startDate": day(0), "lastPaymentDate": day(7)}, extra)
	}
	// setUp creates a debt (captured into id) and sets it up with the loan's
	// facts.
	setUp := func(label, auth string, id *string, create, facts map[string]any) []Call {
		return []Call{
			{Label: label, Method: "POST", Path: "/api/v1/debt/create-debt", Auth: auth, Body: debtBody(create), CaptureIDInto: id},
			{Label: label + "-setup", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: auth, Body: setupBody(id, facts)},
		}
	}
	calls := func(parts ...[]Call) []Call {
		var out []Call
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	createCategory := func(label, auth, id, name string, into *string) Call {
		return Call{Label: label, Method: "POST", Path: "/api/v1/category/create-category", Auth: auth,
			Body: map[string]any{"id": id, "name": name, "type": "expense", "icon": "bank"}, CaptureIDInto: into}
	}
	getDebt := func(label, auth string, id *string) Call {
		return Call{Label: label, Method: "GET", Auth: auth, Path: "/api/v1/debt/get-debt?id=", PathID: id}
	}

	// debt_crud walks one debt through its life: create on a category, set
	// up, list, get, correct the loan's facts (the origin rewritten, the start
	// a week earlier), state a statement balance today (a new snapshot), save
	// with nothing to change, mark paid off as of today, delete. A second debt
	// on the freed category is created without terms and then set up.
	register(Scenario{Name: "debt_crud", Calls: func() []Call {
		var debtID, draftID string
		return calls(
			setUp("create-debt", "owner", &debtID, nil, nil),
			[]Call{
				{Label: "list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
				getDebt("get", "owner", &debtID),
				{Label: "update-terms", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
					Body: map[string]any{"id": &debtID, "startDate": day(-7), "amountBorrowed": "200", "payment": "104",
						"schedule": "weekly", "lastPaymentDate": day(14)}},
				getDebt("get-after-update", "owner", &debtID),
				{Label: "update-balance-now", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
					Body: map[string]any{"id": &debtID, "balance": "150"}},
				// no amount borrowed, balance, payment, schedule, last payment
				// date or start: the terms stay
				{Label: "update-keeps-terms", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
					Body: map[string]any{"id": &debtID}},
				{Label: "mark-paid-off", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
					Body: map[string]any{"id": &debtID}},
				{Label: "delete", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
					Body: map[string]any{"id": &debtID}},
				{Label: "list-after-delete", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
				getDebt("err:get-deleted", "owner", &debtID),

				{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
					Body: debtBody(nil), CaptureIDInto: &draftID},
				getDebt("get-not-set-up", "owner", &draftID),
				{Label: "update-payment-only", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
					Body: map[string]any{"id": &draftID, "payment": "110", "schedule": "weekly"}},
				{Label: "err:paid-off-not-set-up", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
					Body: map[string]any{"id": &draftID}},
				{Label: "finish-setup", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
					Body: setupBody(&draftID, nil)},
				getDebt("get-after-setup", "owner", &draftID),
			},
		)
	}})

	// debt_currency_change: a debt takes its element row's currency. The
	// loan category's row is moved to a custom currency first, so a debt
	// created on it is in that currency, and its 50 payment in USD reads 100
	// at 2 per USD. Moving the row back to USD moves the active debt with it,
	// its stored numbers kept.
	register(Scenario{Name: "debt_currency_change", Calls: func() []Call {
		const (
			opCurrency = "cc000000-0000-0000-0000-0000000000d1"
			opCategory = "d3000000-0000-0000-0000-0000000000a1"
			opPayment  = "d3000000-0000-0000-0000-0000000000b1"
		)
		now := ClockTime.Format("2006-01-02 15:04:05")
		var eurID, categoryID, debtID string
		return calls(
			[]Call{
				{Label: "create-eur-currency", Method: "POST", Path: "/api/v1/currency/create-currency", Auth: "owner",
					Body:          map[string]any{"id": opCurrency, "code": "EUR", "name": "Euro", "symbol": "€", "fractionDigits": 2, "rate": "2"},
					CaptureIDInto: &eurID},
				createCategory("create-loan-category", "owner", opCategory, "Car loan", &categoryID),
				{Label: "change-element-currency", Method: "POST", Path: "/api/v1/budget/change-element-currency", Auth: "owner",
					Body: map[string]any{"budgetId": Budget, "elementId": &categoryID, "currencyId": &eurID}},
			},
			setUp("create-debt", "owner", &debtID, map[string]any{"elementId": &categoryID}, nil),
			[]Call{
				{Label: "pay", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
					Body: map[string]any{"id": opPayment, "type": "expense", "amount": "50.00",
						"accountId": OwnerAccount, "categoryId": &categoryID, "date": now, "description": "loan"}},
				getDebt("get-after-payment", "owner", &debtID),
				{Label: "change-element-currency-back", Method: "POST", Path: "/api/v1/budget/change-element-currency", Auth: "owner",
					Body: map[string]any{"budgetId": Budget, "elementId": &categoryID, "currencyId": USD}},
				getDebt("get-after-currency-change", "owner", &debtID),
			},
		)
	}})

	// debt_validation pins every coded refusal of the write routes.
	register(Scenario{Name: "debt_validation", Calls: func() []Call {
		const unknownDebt = "d0000000-0000-0000-0000-0000000000ff"
		create := func(label string, extra map[string]any) Call {
			return Call{Label: "err:" + label, Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner", Body: debtBody(extra)}
		}
		var draftID, debtID string
		setup := func(label string, extra map[string]any) Call {
			return Call{Label: "err:" + label, Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner", Body: setupBody(&draftID, extra)}
		}
		return []Call{
			create("bad-element-id", map[string]any{"elementId": "nope"}),
			create("no-element", map[string]any{"elementId": ""}),
			create("no-budget", map[string]any{"budgetId": ""}),
			create("unknown-budget", map[string]any{"budgetId": "b0000000-0000-0000-0000-0000000000ee"}),
			create("zero-payment", map[string]any{"payment": "0"}),
			create("bad-schedule", map[string]any{"schedule": "daily"}),
			create("bad-start-date", map[string]any{"startDate": "1 Jan"}),
			create("income-category", map[string]any{"elementId": CatSalary}),
			create("unknown-element", map[string]any{"elementId": "c0000000-0000-0000-0000-0000000000ee"}),
			create("tag-element", map[string]any{"elementId": TagWork}),

			{Label: "create-draft", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &draftID},
			create("element-has-debt", nil),
			setup("bad-balance", map[string]any{"balance": "-1"}),
			setup("zero-amount-borrowed", map[string]any{"amountBorrowed": "0"}),
			setup("bad-last-date", map[string]any{"lastPaymentDate": "soon"}),
			setup("last-payment-before-start", map[string]any{"lastPaymentDate": day(-1)}),
			setup("start-in-future", map[string]any{"startDate": day(1)}),
			setup("balance-now-after-last-payment", map[string]any{"startDate": day(-14), "lastPaymentDate": day(-7), "balance": "10"}),
			setup("payments-dont-cover", map[string]any{"amountBorrowed": "300"}),
			setup("loan-facts-missing-last-date", map[string]any{"lastPaymentDate": ""}),
			setup("loan-facts-missing-amount-borrowed", map[string]any{"amountBorrowed": ""}),
			setup("loan-facts-missing-start-date", map[string]any{"startDate": nil}),
			{Label: "err:update-unknown", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": unknownDebt, "startDate": day(0), "amountBorrowed": "100", "payment": "105",
					"schedule": "weekly", "lastPaymentDate": day(7)}},
			{Label: "err:update-blank-id", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": "", "startDate": day(0), "amountBorrowed": "100", "payment": "105",
					"schedule": "weekly", "lastPaymentDate": day(7)}},
			{Label: "err:get-unknown", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=" + unknownDebt},
			{Label: "err:get-bad-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=nope"},
			{Label: "err:get-missing-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt"},
			{Label: "err:paid-off-unknown", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
				Body: map[string]any{"id": unknownDebt}},
			{Label: "err:paid-off-bad-date", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
				Body: map[string]any{"id": &draftID, "date": "soon"}},
			{Label: "err:reopen-unknown", Method: "POST", Path: "/api/v1/debt/reopen-debt", Auth: "owner",
				Body: map[string]any{"id": unknownDebt}},
			{Label: "err:reopen-bad-id", Method: "POST", Path: "/api/v1/debt/reopen-debt", Auth: "owner",
				Body: map[string]any{"id": "nope"}},
			{Label: "err:delete-unknown", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": unknownDebt}},
			{Label: "delete-draft", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": &draftID}},

			// a debt is no budget row: set-limit does not know its id
			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			{Label: "setup", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner", Body: setupBody(&debtID, nil)},
			{Label: "err:set-limit-on-a-debt-id", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": &debtID, "period": month(0), "amount": "10"}},
			{Label: "err:update-payment-dont-cover", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "payment": "50"}},
		}
	}})

	// debt_classification_cleanup: deleting or merging a category never
	// refuses because of a debt. A deleted category takes its debts with it;
	// a merge moves the source's debts to a target that carries none, and
	// deletes them when the target already carries one. A deleted payee, tag
	// or label touches no debt.
	register(Scenario{Name: "debt_classification_cleanup", Calls: func() []Call {
		op := func(n string) string { return "d1000000-0000-0000-0000-0000000000" + n }
		var catA, srcCat, dstCat, srcBusy, dstBusy, debtA, debtB, debtC, debtD string
		post := func(label, path string, body map[string]any) Call {
			return Call{Label: label, Method: "POST", Path: path, Auth: "owner", Body: body}
		}
		create := func(label string, into, element *string) Call {
			c := post(label, "/api/v1/debt/create-debt", debtBody(map[string]any{"elementId": element}))
			c.CaptureIDInto = into
			return c
		}
		return []Call{
			createCategory("create-category-a", "owner", op("a1"), "Loan a", &catA),
			createCategory("create-source-category", "owner", op("a2"), "Loan source", &srcCat),
			createCategory("create-target-category", "owner", op("a3"), "Loan target", &dstCat),
			createCategory("create-busy-source-category", "owner", op("a4"), "Loan busy source", &srcBusy),
			createCategory("create-busy-target-category", "owner", op("a5"), "Loan busy target", &dstBusy),

			create("create-debt-on-category-a", &debtA, &catA),
			post("delete-category-a", "/api/v1/category/delete-category", map[string]any{"id": &catA, "mode": "delete"}),
			getDebt("err:get-debt-of-the-deleted-category", "owner", &debtA),

			create("create-debt-on-source", &debtB, &srcCat),
			post("merge-into-a-free-target", "/api/v1/category/merge-category", map[string]any{"sourceId": &srcCat, "targetId": &dstCat}),
			getDebt("get-debt-moved-to-the-target", "owner", &debtB),

			create("create-debt-on-busy-source", &debtC, &srcBusy),
			create("create-debt-on-busy-target", &debtD, &dstBusy),
			post("merge-into-a-busy-target", "/api/v1/category/merge-category", map[string]any{"sourceId": &srcBusy, "targetId": &dstBusy}),
			getDebt("err:get-source-debt-after-the-merge", "owner", &debtC),
			getDebt("get-target-debt-after-the-merge", "owner", &debtD),

			post("delete-payee", "/api/v1/payee/delete-payee", map[string]any{"id": PayeeShop}),
			post("delete-tag", "/api/v1/tag/delete-tag", map[string]any{"id": TagWork}),
			post("delete-label", "/api/v1/label/delete-label", map[string]any{"id": LabelWork}),
			{Label: "list-after-classification-deletes", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
		}
	}})

	// debt_budget_access: a debt belongs to its budget. A pending invite
	// reads nothing; once accepted, a participant who may plan reads and edits
	// the debt; turned into a guest, they read it but may not change it.
	register(Scenario{Name: "debt_budget_access", Calls: func() []Call {
		var debtID string
		update := func(label string) Call {
			return Call{Label: label, Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "guest",
				Body: map[string]any{"id": &debtID, "payment": "104"}}
		}
		return calls(
			setUp("create-debt", "owner", &debtID, nil, nil),
			[]Call{
				getDebt("err:invited-get", "guest", &debtID),
				{Label: "invited-list-empty", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "guest"},
				{Label: "accept-access", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
					Body: map[string]any{"budgetId": Budget}},
				getDebt("participant-get", "guest", &debtID),
				{Label: "participant-list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "guest"},
				update("participant-update"),
				{Label: "make-guest", Method: "POST", Path: "/api/v1/budget/grant-access", Auth: "owner",
					Body: map[string]any{"budgetId": Budget, "userId": GuestID, "role": "guest"}},
				getDebt("guest-get", "guest", &debtID),
				update("err:guest-update"),
				{Label: "err:guest-paid-off", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "guest",
					Body: map[string]any{"id": &debtID}},
				{Label: "err:guest-reopen", Method: "POST", Path: "/api/v1/debt/reopen-debt", Auth: "guest",
					Body: map[string]any{"id": &debtID}},
				{Label: "err:guest-delete", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "guest",
					Body: map[string]any{"id": &debtID}},
				{Label: "err:guest-create", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "guest",
					Body: debtBody(map[string]any{"elementId": CatSalary})},
				getDebt("err:stranger-get", "readonly", &debtID),
				{Label: "owner-list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
			},
		)
	}})

	// debt_elements: a debt attaches to a top-level expense category of any
	// participant or to an expense envelope, and shows that element's name,
	// icon and currency. A category inside an envelope, or an income
	// category, cannot carry one.
	register(Scenario{Name: "debt_elements", Calls: func() []Call {
		const envelope = "d6000000-0000-0000-0000-0000000000e1"
		var partnerCat, childCat, partnerDebt, envelopeDebt string
		return []Call{
			{Label: "partner-accepts", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"budgetId": Budget}},
			createCategory("partner-creates-category", "guest", "d6000000-0000-0000-0000-0000000000a1", "Car loan", &partnerCat),
			createCategory("owner-creates-child-category", "owner", "d6000000-0000-0000-0000-0000000000a2", "Insurance", &childCat),
			{Label: "create-envelope", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": envelope, "name": "Housing", "icon": "home",
					"currencyId": USD, "folderId": BudgetFolder1, "categories": []any{&childCat}}},
			{Label: "partner-debt-on-own-category", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "guest",
				Body: debtBody(map[string]any{"elementId": &partnerCat}), CaptureIDInto: &partnerDebt},
			{Label: "owner-debt-on-envelope", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(map[string]any{"elementId": envelope}), CaptureIDInto: &envelopeDebt},
			{Label: "err:category-inside-an-envelope", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(map[string]any{"elementId": &childCat})},
			{Label: "err:income-category", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(map[string]any{"elementId": CatSalary})},
			getDebt("owner-gets-partner-debt", "owner", &partnerDebt),
			getDebt("partner-gets-envelope-debt", "guest", &envelopeDebt),
			{Label: "list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
		}
	}})

	// debt_paid_off: a loan taken two weeks ago is marked paid off a week
	// ago and becomes read-only; a new debt on the same category starts after
	// that end. The older debt cannot be reopened while the newer one exists;
	// once that is deleted, reopening brings the debt back to active without
	// its closing statement.
	register(Scenario{Name: "debt_paid_off", Calls: func() []Call {
		var oldID, newID, closingID string
		return calls(
			setUp("create-debt", "owner", &oldID, nil, map[string]any{"amountBorrowed": "300", "startDate": day(-14)}),
			[]Call{
				{Label: "err:paid-off-on-the-start", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
					Body: map[string]any{"id": &oldID, "date": day(-14)}},
				{Label: "err:paid-off-after-today", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
					Body: map[string]any{"id": &oldID, "date": day(1)}},
				{Label: "mark-paid-off", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
					Body: map[string]any{"id": &oldID, "date": day(-7)}},
				{Label: "get-paid-off", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &oldID,
					CaptureIDInto: &closingID, CapturePath: "data.snapshots.1.id"},
				{Label: "err:update-paid-off", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
					Body: map[string]any{"id": &oldID, "payment": "100"}},
				{Label: "err:reconcile-paid-off", Method: "POST", Path: "/api/v1/debt/reconcile-debt", Auth: "owner",
					Body: map[string]any{"id": &oldID, "balance": "100", "balanceDate": day(-10)}},
				{Label: "err:delete-closing-statement", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "owner",
					Body: map[string]any{"id": &oldID, "reconciliationId": &closingID}},
				{Label: "err:mark-paid-off-again", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
					Body: map[string]any{"id": &oldID}},
				{Label: "err:start-before-previous-end", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
					Body: debtBody(map[string]any{"startDate": day(-7)})},
				{Label: "create-next-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
					Body: debtBody(map[string]any{"startDate": day(-6)}), CaptureIDInto: &newID},
				{Label: "list-both", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
				{Label: "err:reopen-with-a-later-debt", Method: "POST", Path: "/api/v1/debt/reopen-debt", Auth: "owner",
					Body: map[string]any{"id": &oldID}},
				{Label: "delete-next-debt", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
					Body: map[string]any{"id": &newID}},
				{Label: "reopen", Method: "POST", Path: "/api/v1/debt/reopen-debt", Auth: "owner",
					Body: map[string]any{"id": &oldID}},
				getDebt("get-after-reopen", "owner", &oldID),
			},
		)
	}})

	// debt_forecast lists a debt's payments still to come. The weekly debt has
	// one due left, so the golden does not depend on the day the suite runs;
	// its element plans nothing, so the regular payments stay and no budget is
	// named. Planned extras are pinned by the debt service's tests, whose
	// clock is fixed. A pending invite to the budget reads nothing.
	register(Scenario{Name: "debt_forecast", Calls: func() []Call {
		const unknownDebt = "d0000000-0000-0000-0000-0000000000ff"
		var debtID, draftID, categoryID string
		forecast := func(label, auth string, id *string) Call {
			return Call{Label: label, Method: "GET", Auth: auth, Path: "/api/v1/debt/get-debt-forecast?id=", PathID: id}
		}
		return calls(
			setUp("create-debt", "owner", &debtID, nil, nil),
			[]Call{
				forecast("forecast", "owner", &debtID),
				{Label: "get-budget", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
				forecast("forecast-after-budget-read", "owner", &debtID),
				createCategory("create-loan-category", "owner", "d4000000-0000-0000-0000-0000000000a1", "Student loan", &categoryID),
				{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
					Body: debtBody(map[string]any{"elementId": &categoryID}), CaptureIDInto: &draftID},
				forecast("forecast-not-set-up", "owner", &draftID),
				forecast("err:invited-forecast", "guest", &debtID),
				{Label: "err:forecast-unknown", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-forecast?id=" + unknownDebt},
				{Label: "err:forecast-bad-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-forecast?id=nope"},
				{Label: "err:forecast-missing-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-forecast"},
			},
		)
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
		return calls(
			setUp("create-debt", "owner", &debtID, nil, map[string]any{"amountBorrowed": "300", "startDate": day(-14)}),
			[]Call{
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
					Body: map[string]any{"id": &debtID, "startDate": day(-2), "amountBorrowed": "200", "lastPaymentDate": day(7)}},
				{Label: "delete-reconciliation", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "owner",
					Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
				{Label: "err:invited-delete-reconciliation", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "guest",
					Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
				{Label: "err:readonly-delete-reconciliation", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "readonly",
					Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
				{Label: "err:delete-reconciliation-again", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "owner",
					Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
				getDebt("get-after-delete", "owner", &debtID),
				{Label: "err:invited-reconcile", Method: "POST", Path: "/api/v1/debt/reconcile-debt", Auth: "guest",
					Body: map[string]any{"id": &debtID, "balance": "1"}},
				createCategory("create-loan-category", "owner", "d5000000-0000-0000-0000-0000000000a1", "Student loan", &categoryID),
				{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
					Body: debtBody(map[string]any{"elementId": &categoryID}), CaptureIDInto: &draftID},
				reconcile("err:not-set-up", map[string]any{"id": &draftID, "balance": "1"}),
				{Label: "err:readonly-reconcile", Method: "POST", Path: "/api/v1/debt/reconcile-debt", Auth: "readonly",
					Body: map[string]any{"id": unknownDebt, "balance": "1"}},
			},
		)
	}})

	// debt_rate_change: a loan of 300 taken two weeks ago takes a rate change
	// a week ago (which stores the start-date row beside it), the same date
	// again replaces it. Deleting the added change leaves the start-date row;
	// deleting that one too falls back to the loan's implied rate. A guest may
	// not add one and a debt not set up refuses it.
	register(Scenario{Name: "debt_rate_change", Calls: func() []Call {
		const unknownDebt = "d0000000-0000-0000-0000-0000000000ff"
		var debtID, draftID, categoryID, startRowID, addedID string
		add := func(label, auth string, body map[string]any) Call {
			return Call{Label: label, Method: "POST", Path: "/api/v1/debt/add-debt-rate-change", Auth: auth, Body: body}
		}
		return calls(
			setUp("create-debt", "owner", &debtID, nil, map[string]any{"amountBorrowed": "300", "startDate": day(-14)}),
			[]Call{
				add("add-rate-change", "owner", map[string]any{"id": &debtID, "date": day(-7), "annualRate": "5", "payment": "105"}),
				add("add-same-date-replaces", "owner", map[string]any{"id": &debtID, "date": day(-7), "annualRate": "6", "payment": "110"}),
				{Label: "get", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID,
					CaptureIDInto: &startRowID, CapturePath: "data.rateChanges.0.id"},
				{Label: "get-added", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID,
					CaptureIDInto: &addedID, CapturePath: "data.rateChanges.1.id"},
				{Label: "list", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-list"},
				add("err:before-start", "owner", map[string]any{"id": &debtID, "date": day(-15), "annualRate": "5", "payment": "105"}),
				add("err:bad-rate", "owner", map[string]any{"id": &debtID, "date": day(-6), "annualRate": "1e1", "payment": "105"}),
				add("err:rate-out-of-range", "owner", map[string]any{"id": &debtID, "date": day(-6), "annualRate": "101", "payment": "105"}),
				add("err:unknown", "owner", map[string]any{"id": unknownDebt, "date": day(-6), "annualRate": "5", "payment": "105"}),
				add("err:invited-add", "guest", map[string]any{"id": &debtID, "date": day(-6), "annualRate": "5", "payment": "105"}),
				add("err:readonly-add", "readonly", map[string]any{"id": unknownDebt, "date": day(-6), "annualRate": "5", "payment": "105"}),
				{Label: "delete-added-change", Method: "POST", Path: "/api/v1/debt/delete-debt-rate-change", Auth: "owner",
					Body: map[string]any{"id": &debtID, "rateChangeId": &addedID}},
				{Label: "err:delete-added-again", Method: "POST", Path: "/api/v1/debt/delete-debt-rate-change", Auth: "owner",
					Body: map[string]any{"id": &debtID, "rateChangeId": &addedID}},
				{Label: "err:invited-delete", Method: "POST", Path: "/api/v1/debt/delete-debt-rate-change", Auth: "guest",
					Body: map[string]any{"id": &debtID, "rateChangeId": &startRowID}},
				getDebt("get-after-delete-added", "owner", &debtID),
				{Label: "delete-start-row", Method: "POST", Path: "/api/v1/debt/delete-debt-rate-change", Auth: "owner",
					Body: map[string]any{"id": &debtID, "rateChangeId": &startRowID}},
				getDebt("get-after-delete-start-row", "owner", &debtID),
				{Label: "accept-access", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
					Body: map[string]any{"budgetId": Budget}},
				{Label: "make-guest", Method: "POST", Path: "/api/v1/budget/grant-access", Auth: "owner",
					Body: map[string]any{"budgetId": Budget, "userId": GuestID, "role": "guest"}},
				add("err:guest-add", "guest", map[string]any{"id": &debtID, "date": day(-6), "annualRate": "5", "payment": "105"}),
				createCategory("create-loan-category", "owner", "d6000000-0000-0000-0000-0000000000a1", "Student loan", &categoryID),
				{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
					Body: debtBody(map[string]any{"elementId": &categoryID}), CaptureIDInto: &draftID},
				add("err:not-set-up", "owner", map[string]any{"id": &draftID, "date": day(0), "annualRate": "5", "payment": "105"}),
			},
		)
	}})

	// debt_readonly: a lapsed-trial user may read but not write.
	register(Scenario{Name: "debt_readonly", Calls: func() []Call {
		return []Call{
			{Label: "list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "readonly"},
			{Label: "err:create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "readonly", Body: debtBody(nil)},
			{Label: "err:reopen-debt", Method: "POST", Path: "/api/v1/debt/reopen-debt", Auth: "readonly",
				Body: map[string]any{"id": "d0000000-0000-0000-0000-0000000000ff"}},
		}
	}})
}
