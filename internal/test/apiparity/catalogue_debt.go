package apiparity

import (
	"fmt"
	"strings"
	"time"
)

// Debts run on a weekly schedule whose last payment is a fixed number of days
// after the (wall-clock) ClockTime, so the number of payments left, and with
// it every amount in the golden, does not depend on the day the suite runs.
func init() {
	day := func(offset int) string { return ClockTime.AddDate(0, 0, offset).Format("2006-01-02") }
	month := func(offset int) string {
		return time.Date(ClockTime.Year(), ClockTime.Month()+time.Month(offset), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	rule := func(categoryID any, extra map[string]any) map[string]any {
		r := map[string]any{"categoryId": categoryID}
		for k, v := range extra {
			r[k] = v
		}
		return r
	}
	// never nil: an empty list must reach the server as [], not null
	rules := func(rs ...map[string]any) []map[string]any { return append([]map[string]any{}, rs...) }
	debtBody := func(extra map[string]any) map[string]any {
		body := map[string]any{
			"budgetId": Budget, "name": "Car loan", "currencyId": USD, "rules": rules(rule(CatFood, nil)),
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
	// the loan's facts (the origin rewritten, the start a week earlier), state
	// a statement balance today (a new snapshot), rename without terms and
	// give it its own icon (the start date left out is kept), mark paid off (replacing today's
	// statement), delete. A second debt is
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
					"id": &debtID, "name": "Car loan 2", "rules": rules(rule(CatFood, map[string]any{"payeeId": PayeeShop})), "startDate": day(-7),
					"amountBorrowed": "200", "payment": "104", "schedule": "weekly", "lastPaymentDate": day(14),
				}},
			{Label: "get-after-update", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "update-balance-now", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan 2", "balance": "150"}},
			// no rules, amount borrowed, balance, payment, schedule or last
			// payment date: the rules and the terms stay; the debt takes an
			// icon of its own instead of its category's
			{Label: "rename-keeps-terms", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan 3", "icon": "directions_car"}},
			{Label: "mark-paid-off", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
				Body: map[string]any{"id": &debtID}},
			{Label: "delete", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID}},
			{Label: "list-after-delete", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
			{Label: "err:get-deleted", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},

			{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: withoutTerms(map[string]any{"name": "Student loan", "rules": rules(rule(CatFood, map[string]any{"labelId": LabelWork}))}), CaptureIDInto: &draftID},
			{Label: "get-not-set-up", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &draftID},
			{Label: "update-payment-only", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &draftID, "name": "Student loan", "payment": "110", "schedule": "weekly"}},
			{Label: "err:paid-off-not-set-up", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
				Body: map[string]any{"id": &draftID}},
			{Label: "finish-setup", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &draftID, "name": "Student loan",
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
				Body: debtBody(map[string]any{"rules": rules(rule(&categoryID, nil))}), CaptureIDInto: &debtID},
			{Label: "pay", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": opPayment, "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": &categoryID, "date": now, "description": "loan"}},
			{Label: "plan", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": &debtID, "period": month(0), "amount": "105"}},
			{Label: "change-currency", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan", "currencyId": &eurID, "startDate": day(0)}},
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
		eleven := make([]map[string]any, 11)
		for i := range eleven {
			eleven[i] = rule(CatFood, map[string]any{"labelId": fmt.Sprintf("30000000-0000-0000-0000-0000000001%02d", i)})
		}
		return []Call{
			create("blank-name", map[string]any{"name": " "}),
			create("bad-category-id", map[string]any{"rules": rules(rule("nope", nil))}),
			create("no-budget", map[string]any{"budgetId": ""}),
			create("unknown-budget", map[string]any{"budgetId": "b0000000-0000-0000-0000-0000000000ee"}),
			create("no-rules", map[string]any{"rules": rules()}),
			create("eleven-rules", map[string]any{"rules": eleven}),
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
			create("category-not-expense", map[string]any{"rules": rules(rule(CatSalary, nil))}),
			create("unknown-category", map[string]any{"rules": rules(rule("c0000000-0000-0000-0000-0000000000ee", nil))}),
			create("unknown-payee", map[string]any{"rules": rules(rule(CatFood, map[string]any{"payeeId": "20000000-0000-0000-0000-0000000000ee"}))}),
			create("unknown-label", map[string]any{"rules": rules(rule(CatFood, map[string]any{"labelId": "30000000-0000-0000-0000-0000000000ee"}))}),
			create("loan-facts-missing-last-date", map[string]any{"lastPaymentDate": ""}),
			create("loan-facts-missing-amount-borrowed", map[string]any{"amountBorrowed": ""}),
			create("loan-facts-missing-start-date", map[string]any{"startDate": nil}),
			create("tag-and-label-in-a-rule", map[string]any{"rules": rules(rule(CatFood, map[string]any{"tagId": TagWork, "labelId": LabelWork}))}),
			// two rules of one debt that could match the same payment
			create("overlapping-rules", map[string]any{"rules": rules(rule(CatFood, nil), rule(CatFood, map[string]any{"payeeId": PayeeShop}))}),

			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			create("start-date-taken", nil),
			create("key-conflict", map[string]any{"rules": rules(rule(CatFood, map[string]any{"payeeId": PayeeShop}))}),
			// a label tells two debts apart only when both name one
			create("label-key-conflict", map[string]any{"rules": rules(rule(CatFood, map[string]any{"labelId": LabelWork}))}),
			{Label: "err:update-payment-dont-cover", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{
					"id": &debtID, "name": "Car loan", "startDate": day(0), "amountBorrowed": "100", "payment": "50",
					"schedule": "weekly", "lastPaymentDate": day(7),
				}},
			{Label: "err:update-unknown", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{
					"id": unknownDebt, "name": "x", "startDate": day(0), "amountBorrowed": "100", "payment": "105",
					"schedule": "weekly", "lastPaymentDate": day(7),
				}},
			{Label: "err:update-blank-id", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": "", "name": "x", "startDate": day(0), "amountBorrowed": "100", "payment": "105",
					"schedule": "weekly", "lastPaymentDate": day(7)}},
			{Label: "err:update-no-rules", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan", "rules": rules()}},
			{Label: "err:update-icon-too-long", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan", "icon": strings.Repeat("i", 65)}},
			{Label: "err:get-unknown", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=" + unknownDebt},
			{Label: "err:get-bad-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=nope"},
			{Label: "err:get-missing-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt"},
			{Label: "err:paid-off-unknown", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "owner",
				Body: map[string]any{"id": unknownDebt}},
			{Label: "err:delete-unknown", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": unknownDebt}},

			// A debt narrowed by a payee keeps its budget data when deleted, so
			// deleting it with plans needs a confirmation.
			{Label: "delete-whole-debt", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID}},
			{Label: "create-keyed-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(map[string]any{"rules": rules(rule(CatFood, map[string]any{"payeeId": PayeeShop}))}), CaptureIDInto: &keyedID},
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

	// debt_classification_cleanup: deleting or merging a category, payee,
	// tag or label a debt's rule names never refuses. A deleted category
	// takes its rule with it, a deleted payee, tag or label widens the rule it
	// narrowed, and when that leaves two debts' rules able to match one
	// payment, the newer debt loses its rule; a merge moves a rule to the
	// target, and one that makes two rules identical does the same.
	register(Scenario{Name: "debt_classification_cleanup", Calls: func() []Call {
		op := func(n string) string { return "d1000000-0000-0000-0000-0000000000" + n }
		var catA, catB, catC, catD, catE, catF, catG, srcCat, dstCat, payeeA, payeeB, payeeC, srcPayee, dstPayee, srcLabel, dstLabel string
		var tagA, srcTag, dstTag, labelA string
		var debtA, debtB, debtC, debtD, debtE, debtF, debtG, debtH, debtI, debtJ, debtK, debtL, debtM string
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
		createDebt := func(label string, into *string, r map[string]any) Call {
			c := post(label, "/api/v1/debt/create-debt", debtBody(map[string]any{"rules": rules(r)}))
			c.CaptureIDInto = into
			return c
		}
		getDebt := func(label string, id *string) Call {
			return Call{Label: label, Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: id}
		}
		return []Call{
			createCat("create-category-a", op("a1"), "Loan a", &catA),
			createCat("create-category-b", op("a2"), "Loan b", &catB),
			createCat("create-category-c", op("a3"), "Loan c", &catC),
			createCat("create-category-d", op("a4"), "Loan d", &catD),
			createCat("create-source-category", op("a5"), "Loan source", &srcCat),
			createCat("create-target-category", op("a6"), "Loan target", &dstCat),
			createNamed("create-payee-a", "/api/v1/payee/create-payee", op("b1"), "Bank a", &payeeA),
			createNamed("create-payee-b", "/api/v1/payee/create-payee", op("b2"), "Bank b", &payeeB),
			createNamed("create-payee-c", "/api/v1/payee/create-payee", op("b3"), "Bank c", &payeeC),
			createNamed("create-source-payee", "/api/v1/payee/create-payee", op("b4"), "Bank one", &srcPayee),
			createNamed("create-target-payee", "/api/v1/payee/create-payee", op("b5"), "Bank two", &dstPayee),
			createNamed("create-source-label", "/api/v1/label/create-label", op("e1"), "Debt car", &srcLabel),
			createNamed("create-target-label", "/api/v1/label/create-label", op("e2"), "Debt student", &dstLabel),

			createDebt("create-debt-on-category-a", &debtA, rule(&catA, nil)),
			post("delete-category-a", "/api/v1/category/delete-category", map[string]any{"id": &catA, "mode": "delete"}),
			getDebt("get-debt-without-its-rule", &debtA),

			createDebt("create-debt-narrowed-by-payee-a", &debtB, rule(&catB, map[string]any{"payeeId": &payeeA})),
			post("delete-payee-a", "/api/v1/payee/delete-payee", map[string]any{"id": &payeeA}),
			getDebt("get-debt-with-the-rule-widened", &debtB),

			createDebt("create-older-debt-on-payee-b", &debtC, rule(&catC, map[string]any{"payeeId": &payeeB})),
			createDebt("create-newer-debt-on-payee-c", &debtD, rule(&catC, map[string]any{"payeeId": &payeeC})),
			post("delete-payee-b", "/api/v1/payee/delete-payee", map[string]any{"id": &payeeB}),
			getDebt("get-older-debt-widened", &debtC),
			getDebt("get-newer-debt-without-its-rule", &debtD),

			createDebt("create-debt-whole-source", &debtE, rule(&srcCat, nil)),
			createDebt("create-debt-narrowed-target", &debtF, rule(&dstCat, map[string]any{"payeeId": PayeeShop})),
			post("merge-category", "/api/v1/category/merge-category", map[string]any{"sourceId": &srcCat, "targetId": &dstCat}),
			getDebt("get-debt-after-category-merge", &debtE),
			getDebt("get-narrowed-debt-after-category-merge", &debtF),

			createDebt("create-debt-payee-source", &debtG, rule(&catD, map[string]any{"payeeId": &srcPayee})),
			createDebt("create-debt-payee-target", &debtH, rule(&catD, map[string]any{"payeeId": &dstPayee})),
			post("merge-payee", "/api/v1/payee/merge-payee", map[string]any{"sourceId": &srcPayee, "targetId": &dstPayee}),
			getDebt("get-older-debt-after-payee-merge", &debtG),
			getDebt("get-newer-debt-after-payee-merge", &debtH),

			createDebt("create-debt-label-source", &debtI, rule(CatFood, map[string]any{"labelId": &srcLabel})),
			createDebt("create-debt-label-target", &debtJ, rule(CatFood, map[string]any{"labelId": &dstLabel})),
			post("merge-label", "/api/v1/label/merge-label", map[string]any{"sourceId": &srcLabel, "targetId": &dstLabel}),
			getDebt("get-older-debt-after-label-merge", &debtI),
			getDebt("get-newer-debt-after-label-merge", &debtJ),

			createCat("create-category-e", op("a7"), "Loan e", &catE),
			createNamed("create-tag-a", "/api/v1/tag/create-tag", op("f1"), "Debt tag a", &tagA),
			createDebt("create-debt-narrowed-by-tag-a", &debtK, rule(&catE, map[string]any{"tagId": &tagA})),
			post("delete-tag-a", "/api/v1/tag/delete-tag", map[string]any{"id": &tagA}),
			getDebt("get-debt-with-the-tag-rule-widened", &debtK),

			createCat("create-category-f", op("a8"), "Loan f", &catF),
			createNamed("create-source-tag", "/api/v1/tag/create-tag", op("f2"), "Debt tag one", &srcTag),
			createNamed("create-target-tag", "/api/v1/tag/create-tag", op("f3"), "Debt tag two", &dstTag),
			createDebt("create-debt-tag-source", &debtL, rule(&catF, map[string]any{"tagId": &srcTag})),
			post("merge-tag", "/api/v1/tag/merge-tag", map[string]any{"sourceId": &srcTag, "targetId": &dstTag}),
			getDebt("get-debt-after-tag-merge", &debtL),

			createCat("create-category-g", op("a9"), "Loan g", &catG),
			createNamed("create-label-a", "/api/v1/label/create-label", op("e3"), "Debt bike", &labelA),
			createDebt("create-debt-narrowed-by-label-a", &debtM, rule(&catG, map[string]any{"labelId": &labelA})),
			post("delete-label-a", "/api/v1/label/delete-label", map[string]any{"id": &labelA}),
			getDebt("get-debt-with-the-label-rule-widened", &debtM),
		}
	}})

	// debt_budget_access: a debt belongs to its budget. A pending invite
	// reads nothing; once accepted, a participant who may plan reads and edits
	// the debt; turned into a guest, they read it but may not change it.
	register(Scenario{Name: "debt_budget_access", Calls: func() []Call {
		var debtID string
		update := func(label string) Call {
			return Call{Label: label, Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "guest",
				Body: map[string]any{"id": &debtID, "name": "Shared loan"}}
		}
		return []Call{
			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			{Label: "err:invited-get", Method: "GET", Auth: "guest", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "invited-list-empty", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "guest"},
			{Label: "accept-access", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"budgetId": Budget}},
			{Label: "participant-get", Method: "GET", Auth: "guest", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "participant-list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "guest"},
			update("participant-update"),
			{Label: "make-guest", Method: "POST", Path: "/api/v1/budget/grant-access", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "userId": GuestID, "role": "guest"}},
			{Label: "guest-get", Method: "GET", Auth: "guest", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			update("err:guest-update"),
			{Label: "err:guest-paid-off", Method: "POST", Path: "/api/v1/debt/mark-debt-paid-off", Auth: "guest",
				Body: map[string]any{"id": &debtID}},
			{Label: "err:guest-delete", Method: "POST", Path: "/api/v1/debt/delete-debt", Auth: "guest",
				Body: map[string]any{"id": &debtID}},
			{Label: "err:guest-create", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "guest",
				Body: debtBody(map[string]any{"name": "Guest loan"})},
			{Label: "err:stranger-get", Method: "GET", Auth: "readonly", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "owner-list", Method: "GET", Path: "/api/v1/debt/get-debt-list", Auth: "owner"},
		}
	}})

	// debt_rules: a debt matches through up to ten rules, each on any
	// participant's expense category. add-debt-rule adds one; a rule another
	// rule of the debt covers, or one that could match another debt's
	// payments, is refused; update-debt replaces the set.
	register(Scenario{Name: "debt_rules", Calls: func() []Call {
		var partnerCat, loanCat, otherCat, debtID, otherID string
		addRule := func(label string, r map[string]any) Call {
			return Call{Label: label, Method: "POST", Path: "/api/v1/debt/add-debt-rule", Auth: "owner",
				Body: map[string]any{"id": &debtID, "rule": r}}
		}
		return []Call{
			{Label: "partner-accepts", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"budgetId": Budget}},
			{Label: "partner-creates-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "guest",
				Body:          map[string]any{"id": "d6000000-0000-0000-0000-0000000000a1", "name": "Car loan", "type": "expense", "icon": "car"},
				CaptureIDInto: &partnerCat},
			{Label: "owner-creates-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": "d6000000-0000-0000-0000-0000000000a2", "name": "Loan", "type": "expense", "icon": "bank"},
				CaptureIDInto: &loanCat},
			{Label: "owner-creates-other-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": "d6000000-0000-0000-0000-0000000000a3", "name": "Other loan", "type": "expense", "icon": "bank"},
				CaptureIDInto: &otherCat},
			{Label: "create-two-rule-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(map[string]any{"rules": rules(rule(CatFood, nil), rule(&partnerCat, nil))}), CaptureIDInto: &debtID},
			addRule("add-debt-rule", rule(&loanCat, nil)),
			addRule("err:add-covered-rule", rule(CatFood, map[string]any{"payeeId": PayeeShop})),
			{Label: "create-other-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body:          debtBody(map[string]any{"name": "Other", "rules": rules(rule(&otherCat, map[string]any{"payeeId": PayeeShop}))}),
				CaptureIDInto: &otherID},
			addRule("err:add-conflicting-rule", rule(&otherCat, nil)),
			{Label: "err:add-rule-unknown-debt", Method: "POST", Path: "/api/v1/debt/add-debt-rule", Auth: "owner",
				Body: map[string]any{"id": "d0000000-0000-0000-0000-0000000000ff", "rule": rule(&loanCat, nil)}},
			{Label: "err:add-rule-blank", Method: "POST", Path: "/api/v1/debt/add-debt-rule", Auth: "owner",
				Body: map[string]any{"id": &debtID, "rule": map[string]any{}}},
			{Label: "update-replaces-rules", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan", "rules": rules(rule(CatFood, nil), rule(&loanCat, nil))}},
			{Label: "err:update-empty-rules", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner",
				Body: map[string]any{"id": &debtID, "name": "Car loan", "rules": rules()}},
			{Label: "get", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "partner-get-other", Method: "GET", Auth: "guest", Path: "/api/v1/debt/get-debt?id=", PathID: &otherID},
		}
	}})

	// debt_rule_options lists what a rule can name, per participant. Only a
	// participant who may plan sees it: a guest is refused, a stranger finds
	// no budget.
	register(Scenario{Name: "debt_rule_options", Calls: func() []Call {
		options := func(label, auth, query string) Call {
			return Call{Label: label, Method: "GET", Auth: auth, Path: "/api/v1/debt/get-debt-rule-options" + query}
		}
		return []Call{
			options("err:invited", "guest", "?budgetId="+Budget),
			{Label: "partner-accepts", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"budgetId": Budget}},
			options("owner", "owner", "?budgetId="+Budget),
			options("partner", "guest", "?budgetId="+Budget),
			{Label: "make-guest", Method: "POST", Path: "/api/v1/budget/grant-access", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "userId": GuestID, "role": "guest"}},
			options("owner-without-the-guest", "owner", "?budgetId="+Budget),
			options("err:guest", "guest", "?budgetId="+Budget),
			options("err:stranger", "readonly", "?budgetId="+Budget),
			options("err:missing-budget-id", "owner", ""),
			options("err:bad-budget-id", "owner", "?budgetId=nope"),
		}
	}})

	// debt_forecast lists a debt's payments still to come. The weekly debt has
	// one due left, so the golden does not depend on the day the suite runs;
	// its budget's row plans nothing, so the regular payments stay and no
	// budget is named. Planned extras are pinned by the debt service's tests,
	// whose clock is fixed. A pending invite to the budget reads nothing.
	register(Scenario{Name: "debt_forecast", Calls: func() []Call {
		const unknownDebt = "d0000000-0000-0000-0000-0000000000ff"
		var debtID, draftID, categoryID string
		forecast := func(label, auth string, id *string) Call {
			return Call{Label: label, Method: "GET", Auth: auth, Path: "/api/v1/debt/get-debt-forecast?id=", PathID: id}
		}
		return []Call{
			{Label: "create-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debtBody(nil), CaptureIDInto: &debtID},
			forecast("forecast", "owner", &debtID),
			{Label: "get-budget", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
			forecast("forecast-after-budget-read", "owner", &debtID),
			{Label: "create-loan-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": "d4000000-0000-0000-0000-0000000000a1", "name": "Student loan", "type": "expense", "icon": "school"},
				CaptureIDInto: &categoryID},
			{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: withoutTerms(map[string]any{"name": "Student loan", "rules": rules(rule(&categoryID, nil))}), CaptureIDInto: &draftID},
			forecast("forecast-not-set-up", "owner", &draftID),
			forecast("err:invited-forecast", "guest", &debtID),
			{Label: "err:forecast-unknown", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-forecast?id=" + unknownDebt},
			{Label: "err:forecast-bad-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-forecast?id=nope"},
			{Label: "err:forecast-missing-id", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt-forecast"},
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
				Body: map[string]any{"id": &debtID, "name": "Car loan", "startDate": day(-2),
					"amountBorrowed": "200", "lastPaymentDate": day(7)}},
			{Label: "delete-reconciliation", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "owner",
				Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
			{Label: "err:invited-delete-reconciliation", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "guest",
				Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
			{Label: "err:readonly-delete-reconciliation", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "readonly",
				Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
			{Label: "err:delete-reconciliation-again", Method: "POST", Path: "/api/v1/debt/delete-debt-reconciliation", Auth: "owner",
				Body: map[string]any{"id": &debtID, "reconciliationId": &statementID}},
			{Label: "get-after-delete", Method: "GET", Auth: "owner", Path: "/api/v1/debt/get-debt?id=", PathID: &debtID},
			{Label: "err:invited-reconcile", Method: "POST", Path: "/api/v1/debt/reconcile-debt", Auth: "guest",
				Body: map[string]any{"id": &debtID, "balance": "1"}},
			{Label: "create-loan-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": "d5000000-0000-0000-0000-0000000000a1", "name": "Student loan", "type": "expense", "icon": "school"},
				CaptureIDInto: &categoryID},
			{Label: "create-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: withoutTerms(map[string]any{"name": "Student loan", "rules": rules(rule(&categoryID, nil))}), CaptureIDInto: &draftID},
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
