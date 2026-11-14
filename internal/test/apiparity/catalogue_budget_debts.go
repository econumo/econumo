package apiparity

import "time"

// budget_debts: debts on budget elements in get-budget and get-budget-plan,
// in a budget shared with a partner who tracks a debt on their own category
// too. A debt only adds figures to its element: payments, plans and totals
// stay the element's own.
//
// The figures must not depend on the day the suite runs, so each debt is 100
// paid back by one monthly payment of 105 on today's day of the next month
// (clamped to its last day): a single due after today, whatever the date.
// The current month then schedules nothing (today's day of the series is the
// start day, which pays no due), has nothing still due and a balance of 100
// with one payment left; the next month has the 105 due and closes at 0 (a
// one-payment loan accrues 5 whatever the length of its prorated first
// period). The owner's 50 payment today, the loan's start day, stays in the
// loan category's spent and comes fully off the 100; with nothing scheduled
// this month, 55 of the category's plan of 105 is extra, which pays off the
// 50 left: that debt closes the current month at 0 with nothing scheduled
// after. The plan window runs from the previous month, before the debts
// start, so its first cells carry no debt. Months stay out of the recorded
// paths (see budget_upcoming_recurring).
//
// An expense envelope carries a debt too (not set up). A category whose debt
// is active cannot go into an envelope, by move-element or create-envelope.
//
// Last, the owner adds a debt with no terms (payment and schedule only) on a
// third category and pays 30 into it today: get-budget and get-budget-plan
// then show it not set up, with no balance and nothing scheduled.
func init() {
	register(Scenario{Name: "budget_debts", Calls: func() []Call {
		now := ClockTime.Format("2006-01-02 15:04:05")
		current := time.Date(ClockTime.Year(), ClockTime.Month(), 1, 0, 0, 0, 0, time.UTC)
		previous := current.AddDate(0, -1, 0).Format("2006-01-02")
		day := ClockTime.Day()
		if last := current.AddDate(0, 2, -1).Day(); day > last {
			day = last
		}
		lastPayment := time.Date(current.Year(), current.Month()+1, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		month := current.Format("2006-01-02")
		debt := func(categoryID *string) map[string]any {
			return map[string]any{"budgetId": Budget, "elementId": categoryID, "payment": "105", "schedule": "monthly"}
		}
		setup := func(debtID *string) map[string]any {
			return map[string]any{"id": debtID, "amountBorrowed": "100", "startDate": ClockTime.Format("2006-01-02"), "lastPaymentDate": lastPayment}
		}
		const envelope = "d2000000-0000-0000-0000-0000000000e1"
		var loanCat, bikeCat, studyCat, ownerDebt, partnerDebt string
		txList := "/api/v1/budget/get-transaction-list?budgetId=" + Budget + "&periodStart="
		return []Call{
			{Label: "guest-accepts", Method: "POST", Path: "/api/v1/budget/accept-access", Auth: "guest",
				Body: map[string]any{"budgetId": Budget}},
			{Label: "owner-creates-loan-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": "d2000000-0000-0000-0000-0000000000a1", "name": "Car loan", "type": "expense", "icon": "car"},
				CaptureIDInto: &loanCat},
			{Label: "owner-creates-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: debt(&loanCat), CaptureIDInto: &ownerDebt},
			{Label: "owner-sets-up-debt", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "owner", Body: setup(&ownerDebt)},
			{Label: "owner-pays", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": "d2000000-0000-0000-0000-0000000000b1", "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": &loanCat, "date": now, "description": "loan"}},
			{Label: "owner-plans-current-month", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": &loanCat, "period": month, "amount": "105"}},
			{Label: "partner-creates-loan-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "guest",
				Body:          map[string]any{"id": "d2000000-0000-0000-0000-0000000000a2", "name": "Bike loan", "type": "expense", "icon": "bike"},
				CaptureIDInto: &bikeCat},
			{Label: "partner-creates-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "guest",
				Body: debt(&bikeCat), CaptureIDInto: &partnerDebt},
			{Label: "partner-sets-up-debt", Method: "POST", Path: "/api/v1/debt/update-debt", Auth: "guest", Body: setup(&partnerDebt)},
			{Label: "owner-creates-envelope", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": envelope, "name": "Housing", "icon": "home", "currencyId": USD, "categories": []any{}}},
			{Label: "owner-creates-envelope-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": envelope, "payment": "300", "schedule": "monthly"}},
			{Label: "err:move-category-with-debt-into-envelope", Method: "POST", Path: "/api/v1/budget/move-element", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": &loanCat, "envelopeId": envelope}},
			{Label: "err:create-envelope-with-category-with-debt", Method: "POST", Path: "/api/v1/budget/create-envelope", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "id": "d2000000-0000-0000-0000-0000000000e2", "name": "Loans", "icon": "bank",
					"currencyId": USD, "categories": []any{&loanCat}}},

			{Label: "owner-get-budget", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
			{Label: "partner-get-budget", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "guest"},
			{Label: "get-budget-plan", Method: "GET",
				Path:       "/api/v1/budget/get-budget-plan?id=" + Budget + "&from=" + previous + "&months=4",
				GoldenPath: "/api/v1/budget/get-budget-plan?id=" + Budget + "&from=<previous-month>&months=4", Auth: "owner"},

			{Label: "category-transaction-list-with-the-payment", Method: "GET", Auth: "owner",
				Path: txList + month + "&categoryId=", GoldenPath: txList + "<current-month>&categoryId=", PathID: &loanCat},

			{Label: "owner-creates-study-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": "d2000000-0000-0000-0000-0000000000a3", "name": "Study loan", "type": "expense", "icon": "school"},
				CaptureIDInto: &studyCat},
			{Label: "owner-creates-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": &studyCat, "payment": "200", "schedule": "monthly"}},
			{Label: "owner-pays-debt-without-terms", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": "d2000000-0000-0000-0000-0000000000b2", "type": "expense", "amount": "30.00",
					"accountId": OwnerAccount, "categoryId": &studyCat, "date": now, "description": "study"}},
			{Label: "owner-get-budget-with-debt-without-terms", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
			{Label: "get-budget-plan-with-debt-without-terms", Method: "GET",
				Path:       "/api/v1/budget/get-budget-plan?id=" + Budget + "&from=" + previous + "&months=4",
				GoldenPath: "/api/v1/budget/get-budget-plan?id=" + Budget + "&from=<previous-month>&months=4", Auth: "owner"},
		}
	}})
}
