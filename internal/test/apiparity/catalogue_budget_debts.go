package apiparity

import "time"

// budget_debts: the debt rows of get-budget and get-budget-plan, and the debt
// row's drill-down, in a budget shared with a partner who owns a debt too.
//
// The figures must not depend on the day the suite runs, so each debt is 100
// paid back by one monthly payment of 105 on today's day of the next month
// (clamped to its last day): a single due after today, whatever the date.
// The current month then has 105 scheduled, nothing still due and a balance
// of 100 with one payment left; the next month has the 105 due and closes at
// 0. The owner's 50 payment today counts on the debt row, not on the
// category's. The owner's plan of 105 for the current month is the scheduled
// amount, so it projects no extra: the debt still closes the current month at
// 100 and owes 105 next month. The plan window runs from the previous month, before the debts
// start, to the month after the last payment, so its first and last cells are
// not plannable. Months stay out of the recorded paths (see
// budget_upcoming_recurring).
//
// Last, the owner adds a debt with no terms (payment and schedule only) on a
// third category and pays 30 into it today: get-budget and get-budget-plan
// then show it not set up, with its Paid, no balance, nothing scheduled and
// plannable from the current month with no end.
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
			return map[string]any{
				"name": "Loan", "currencyId": USD, "categoryId": categoryID,
				"amountBorrowed": "100", "startDate": ClockTime.Format("2006-01-02"), "payment": "105", "schedule": "monthly",
				"lastPaymentDate": lastPayment,
			}
		}
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
			{Label: "owner-pays", Method: "POST", Path: "/api/v1/transaction/create-transaction", Auth: "owner",
				Body: map[string]any{"id": "d2000000-0000-0000-0000-0000000000b1", "type": "expense", "amount": "50.00",
					"accountId": OwnerAccount, "categoryId": &loanCat, "date": now, "description": "loan"}},
			{Label: "owner-plans-current-month", Method: "POST", Path: "/api/v1/budget/set-limit", Auth: "owner",
				Body: map[string]any{"budgetId": Budget, "elementId": &ownerDebt, "period": month, "amount": "105"}},
			{Label: "partner-creates-loan-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "guest",
				Body:          map[string]any{"id": "d2000000-0000-0000-0000-0000000000a2", "name": "Bike loan", "type": "expense", "icon": "bike"},
				CaptureIDInto: &bikeCat},
			{Label: "partner-creates-debt", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "guest",
				Body: debt(&bikeCat), CaptureIDInto: &partnerDebt},

			{Label: "owner-get-budget", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "owner"},
			{Label: "partner-get-budget", Method: "GET", Path: "/api/v1/budget/get-budget?id=" + Budget, Auth: "guest"},
			{Label: "get-budget-plan", Method: "GET",
				Path:       "/api/v1/budget/get-budget-plan?id=" + Budget + "&from=" + previous + "&months=4",
				GoldenPath: "/api/v1/budget/get-budget-plan?id=" + Budget + "&from=<previous-month>&months=4", Auth: "owner"},

			{Label: "debt-transaction-list", Method: "GET", Auth: "owner",
				Path: txList + month + "&debtId=", GoldenPath: txList + "<current-month>&debtId=", PathID: &ownerDebt},
			{Label: "category-transaction-list-without-the-payment", Method: "GET", Auth: "owner",
				Path: txList + month + "&categoryId=", GoldenPath: txList + "<current-month>&categoryId=", PathID: &loanCat},
			{Label: "partner-debt-transaction-list", Method: "GET", Auth: "guest",
				Path: txList + month + "&debtId=", GoldenPath: txList + "<current-month>&debtId=", PathID: &partnerDebt},
			{Label: "err:debt-transaction-list-unknown-debt", Method: "GET", Auth: "owner",
				Path:       txList + month + "&debtId=d2000000-0000-0000-0000-0000000000ff",
				GoldenPath: txList + "<current-month>&debtId=d2000000-0000-0000-0000-0000000000ff"},
			{Label: "err:debt-transaction-list-with-category", Method: "GET", Auth: "owner",
				Path:       txList + month + "&categoryId=" + CatFood + "&debtId=",
				GoldenPath: txList + "<current-month>&categoryId=" + CatFood + "&debtId=", PathID: &ownerDebt},

			{Label: "owner-creates-study-category", Method: "POST", Path: "/api/v1/category/create-category", Auth: "owner",
				Body:          map[string]any{"id": "d2000000-0000-0000-0000-0000000000a3", "name": "Study loan", "type": "expense", "icon": "school"},
				CaptureIDInto: &studyCat},
			{Label: "owner-creates-debt-without-terms", Method: "POST", Path: "/api/v1/debt/create-debt", Auth: "owner",
				Body: map[string]any{"name": "Study", "currencyId": USD, "categoryId": &studyCat, "payment": "200", "schedule": "monthly"}},
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
