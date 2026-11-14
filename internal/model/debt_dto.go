// Request/result DTOs for the debt use cases, with their tier-1 Validate() methods.
package model

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateDebtRequest attaches a debt to an element of the budget (ElementId: a
// top-level expense category or an envelope), created not set up: its terms
// come with update-debt. A StartDate bounds which payments count.
type CreateDebtRequest struct {
	BudgetId  string        `json:"budgetId"`
	ElementId string        `json:"elementId"`
	Payment   vo.FlexString `json:"payment"`
	Schedule  string        `json:"schedule"`
	StartDate *string       `json:"startDate"`
}

func (r CreateDebtRequest) Validate() error {
	fields := debtIDField("budgetId", r.BudgetId, true)
	fields = append(fields, debtIDField("elementId", r.ElementId, true)...)
	fields = append(fields, debtFields(debtTermFields{startDate: r.StartDate, payment: r.Payment, schedule: r.Schedule}, false)...)
	return debtValidation(fields)
}

// UpdateDebtRequest changes a debt's terms; its element, budget and currency
// stay. AmountBorrowed and LastPaymentDate come as a pair; they, Payment,
// Schedule and (on a set-up debt) StartDate left out keep the stored ones;
// on a debt not set up a StartDate left out means none. A Balance alone gives
// a set-up debt a statement balance as of BalanceDate (default today).
type UpdateDebtRequest struct {
	Id              string        `json:"id"`
	StartDate       *string       `json:"startDate"`
	AmountBorrowed  vo.FlexString `json:"amountBorrowed"`
	Balance         vo.FlexString `json:"balance"`
	BalanceDate     string        `json:"balanceDate"`
	Payment         vo.FlexString `json:"payment"`
	Schedule        string        `json:"schedule"`
	LastPaymentDate string        `json:"lastPaymentDate"`
}

func (r UpdateDebtRequest) Validate() error {
	fields := debtFields(debtTermFields{r.StartDate,
		r.AmountBorrowed, r.Balance, r.BalanceDate, r.Payment, r.Schedule, r.LastPaymentDate}, true)
	fields = append(fields, debtIDField("id", r.Id, true)...)
	return debtValidation(fields)
}

// ReconcileDebtRequest states a set-up debt's balance from a statement as of
// BalanceDate (Y-m-d; empty = today), after its start date and no later than today.
type ReconcileDebtRequest struct {
	Id          string        `json:"id"`
	Balance     vo.FlexString `json:"balance"`
	BalanceDate string        `json:"balanceDate"`
}

func (r ReconcileDebtRequest) Validate() error {
	fields := debtIDField("id", r.Id, true)
	fields = append(fields, debtAmountField("balance", r.Balance.String(), true)...)
	fields = append(fields, debtDateField("balanceDate", r.BalanceDate)...)
	return debtValidation(fields)
}

// DeleteDebtReconciliationRequest removes one statement balance (a snapshot
// after the origin) from a debt.
type DeleteDebtReconciliationRequest struct {
	Id               string `json:"id"`
	ReconciliationId string `json:"reconciliationId"`
}

func (r DeleteDebtReconciliationRequest) Validate() error {
	return debtValidation(append(debtIDField("id", r.Id, true), debtIDField("reconciliationId", r.ReconciliationId, true)...))
}

// AddDebtRateChangeRequest records a rate change from Date (Y-m-d, on or
// after the start date): AnnualRate a yearly percent from 0 to 100, Payment
// the regular payment from then on. A change on a date that has one replaces it.
type AddDebtRateChangeRequest struct {
	Id         string        `json:"id"`
	Date       string        `json:"date"`
	AnnualRate vo.FlexString `json:"annualRate"`
	Payment    vo.FlexString `json:"payment"`
}

func (r AddDebtRateChangeRequest) Validate() error {
	fields := debtIDField("id", r.Id, true)
	if !filled(r.Date) {
		fields = append(fields, blankField("date"))
	} else {
		fields = append(fields, debtDateField("date", r.Date)...)
	}
	fields = append(fields, debtRateField("annualRate", r.AnnualRate.String())...)
	fields = append(fields, debtAmountField("payment", r.Payment.String(), false)...)
	return debtValidation(fields)
}

type DeleteDebtRateChangeRequest struct {
	Id           string `json:"id"`
	RateChangeId string `json:"rateChangeId"`
}

func (r DeleteDebtRateChangeRequest) Validate() error {
	return debtValidation(append(debtIDField("id", r.Id, true), debtIDField("rateChangeId", r.RateChangeId, true)...))
}

type DeleteDebtRequest struct {
	Id string `json:"id"`
}

func (r DeleteDebtRequest) Validate() error {
	return debtValidation(debtIDField("id", r.Id, true))
}

type DeleteDebtResult struct{}

// MarkDebtPaidOffRequest ends a debt as of Date (Y-m-d; empty = today).
type MarkDebtPaidOffRequest struct {
	Id   string `json:"id"`
	Date string `json:"date"`
}

func (r MarkDebtPaidOffRequest) Validate() error {
	return debtValidation(append(debtIDField("id", r.Id, true), debtDateField("date", r.Date)...))
}

// ReopenDebtRequest takes a paid-off debt back to active.
type ReopenDebtRequest struct {
	Id string `json:"id"`
}

func (r ReopenDebtRequest) Validate() error {
	return debtValidation(debtIDField("id", r.Id, true))
}

type GetDebtRequest struct {
	Id string `json:"id"`
}

func (r GetDebtRequest) Validate() error {
	return debtValidation(debtIDField("id", r.Id, true))
}

// GetDebtForecastRequest asks for a debt's projected payments, with the
// extras its budget plans.
type GetDebtForecastRequest struct {
	Id string `json:"id"`
}

func (r GetDebtForecastRequest) Validate() error {
	return debtValidation(debtIDField("id", r.Id, true))
}

// DebtForecastItem is one projected payment: amounts in the debt's currency,
// balance what is owed after it.
type DebtForecastItem struct {
	Date      string `json:"date"`
	Payment   string `json:"payment"`
	Interest  string `json:"interest"`
	Principal string `json:"principal"`
	Balance   string `json:"balance"`
}

// DebtForecastResult lists the payments still to come, soonest first, to the
// payoff. budgetId/budgetName name the debt's budget when its planned extras
// are included, "" when it follows the regular payments alone. paymentsLeft,
// payoffDate and overpaymentLeft are DebtResult's figures with those plans
// (equal to them when there are none), extraPlanned the planned extras the
// run applies ("0" with none); 0 and "" while the debt is not set up.
type DebtForecastResult struct {
	Items           []DebtForecastItem `json:"items"`
	BudgetId        string             `json:"budgetId"`
	BudgetName      string             `json:"budgetName"`
	PaymentsLeft    int                `json:"paymentsLeft"`
	PayoffDate      string             `json:"payoffDate"`
	OverpaymentLeft string             `json:"overpaymentLeft"`
	ExtraPlanned    string             `json:"extraPlanned"`
}

// DebtResult is one debt in the API. name, icon and currencyId are its
// element's (elementId, elementType category|envelope). status is active or
// paid_off, endedAt the last day payments count ("" while active).
// amountBorrowed is the loan's amount as of startDate; the terms (balance ..
// lastPaymentDate) are the latest snapshot's, the payment the origin's;
// amounts are decimal strings in the debt's currency, dates "2006-01-02" (""
// when none), isPaidOff and canEdit ints 0/1 and periodRate the rate in force
// today per schedule period with 10 decimals. extraPaid is what the payments
// dated up to today went above the regular payment, window by window (a
// payment on the start date in full). While isSetUp is 0 the terms and
// figures are "" and the counts 0. canEdit: the caller can plan in the debt's
// budget and it is not archived, whatever the debt's status.
type DebtResult struct {
	Id              string `json:"id"`
	BudgetId        string `json:"budgetId"`
	BudgetName      string `json:"budgetName"`
	ElementId       string `json:"elementId"`
	ElementType     string `json:"elementType"`
	Status          string `json:"status"`
	EndedAt         string `json:"endedAt"`
	CanEdit         int    `json:"canEdit"`
	Name            string `json:"name"`
	Icon            string `json:"icon"`
	CurrencyId      string `json:"currencyId"`
	StartDate       string `json:"startDate"`
	AmountBorrowed  string `json:"amountBorrowed"`
	Balance         string `json:"balance"`
	Payment         string `json:"payment"`
	Schedule        string `json:"schedule"`
	LastPaymentDate string `json:"lastPaymentDate"`
	CurrentBalance  string `json:"currentBalance"`
	PaymentsLeft    int    `json:"paymentsLeft"`
	PeriodsPaid     int    `json:"periodsPaid"`
	MoneyLeft       string `json:"moneyLeft"`
	OverpaymentLeft string `json:"overpaymentLeft"`
	OverpaidSoFar   string `json:"overpaidSoFar"`
	ExtraPaid       string `json:"extraPaid"`
	PayoffDate      string `json:"payoffDate"`
	NextDueDate     string `json:"nextDueDate"`
	PeriodRate      string `json:"periodRate"`
	// AnnualRate is the yearly percent in force today, ImpliedAnnualRate the
	// one the loan's terms imply; at most 8 decimals, "" while not set up.
	AnnualRate        string `json:"annualRate"`
	ImpliedAnnualRate string `json:"impliedAnnualRate"`
	IsPaidOff         int    `json:"isPaidOff"`
	IsSetUp           int    `json:"isSetUp"`
}

type DebtListResult struct {
	Items []DebtResult `json:"items"`
}

// DebtSnapshotResult is one entry of a debt's history: the origin (isOrigin
// 1, the amount borrowed as of the start date) and the statement balances
// after it. computedBalance is what Econumo had worked out for that date
// from the entries before it and the recorded payments ("" for the origin).
type DebtSnapshotResult struct {
	Id              string `json:"id"`
	IsOrigin        int    `json:"isOrigin"`
	ComputedBalance string `json:"computedBalance"`
	AsOf            string `json:"asOf"`
	Balance         string `json:"balance"`
	Payment         string `json:"payment"`
	Schedule        string `json:"schedule"`
	LastPaymentDate string `json:"lastPaymentDate"`
}

type DebtPaymentResult struct {
	TransactionId string `json:"transactionId"`
	Date          string `json:"date"`
	Amount        string `json:"amount"`
	Interest      string `json:"interest"`
	Principal     string `json:"principal"`
	// Balance is what was owed after the payment, "" while the debt is not set up.
	Balance string `json:"balance"`
}

// DebtRateChangeResult is one rate change: annualRate a yearly percent,
// payment the regular payment from date on.
type DebtRateChangeResult struct {
	Id         string `json:"id"`
	Date       string `json:"date"`
	AnnualRate string `json:"annualRate"`
	Payment    string `json:"payment"`
}

// DebtDetailsResult: rateChanges oldest first.
type DebtDetailsResult struct {
	Item        DebtResult             `json:"item"`
	Snapshots   []DebtSnapshotResult   `json:"snapshots"`
	Payments    []DebtPaymentResult    `json:"payments"`
	RateChanges []DebtRateChangeResult `json:"rateChanges"`
}

// DebtTimelineRequest asks for the period-by-period projection of the debts
// of Scope's budget over its periods (PeriodWindow on PeriodStartDay; 0 or 1
// = calendar months), counting the payments on Scope's accounts. Plans maps
// debt id -> period key "YYYY-MM-01" -> planned amount; Months are period keys.
type DebtTimelineRequest struct {
	Scope          DebtScope
	DebtIDs        []string
	Months         []time.Time
	Plans          map[string]map[string]string
	Today          time.Time
	PeriodStartDay int
}

type DebtTimelineMonth struct {
	ClosingBalance string
	PaymentsLeft   int
	ScheduledTotal string
	StillDue       string
}

type DebtTimeline struct {
	Months              map[string]DebtTimelineMonth
	PayoffDate          string
	BasePayoffDate      string
	OverpaymentLeft     string
	BaseOverpaymentLeft string
}

func debtValidation(fields []errs.FieldError) error {
	if len(fields) == 0 {
		return nil
	}
	return errs.NewValidation("Validation failed", fields...)
}

func debtInvalidField(key string) errs.FieldError {
	return errs.FieldError{Key: key, Message: "This value is not valid.", Code: errs.CodeInvalidFormat}
}

func debtIDField(key, v string, required bool) []errs.FieldError {
	if strings.TrimSpace(v) == "" {
		if required {
			return []errs.FieldError{blankField(key)}
		}
		return nil
	}
	if _, err := vo.ParseId(v); err != nil {
		return []errs.FieldError{{Key: key, Message: "This value is not a valid UUID.", Code: errs.CodeInvalidUUID}}
	}
	return nil
}

type debtTermFields struct {
	startDate                 *string
	amountBorrowed, balance   vo.FlexString
	balanceDate               string
	payment                   vo.FlexString
	schedule, lastPaymentDate string
}

func debtDateField(key, v string) []errs.FieldError {
	if !filled(v) {
		return nil
	}
	if _, err := time.Parse(datetime.DateLayout, v); err != nil {
		return []errs.FieldError{debtInvalidField(key)}
	}
	return nil
}

func filled(v string) bool { return strings.TrimSpace(v) != "" }

// debtFields: with keepable (an update) the payment and the schedule may be
// left out, and so may the loan's facts around a balance now.
func debtFields(f debtTermFields, keepable bool) []errs.FieldError {
	var fields []errs.FieldError
	hasStart := f.startDate != nil && filled(*f.startDate)
	if hasStart {
		if _, err := time.Parse(datetime.DateLayout, *f.startDate); err != nil {
			fields = append(fields, debtInvalidField("startDate"))
		}
	}
	// The loan's facts come together: amount borrowed and last payment date
	// both or neither, and on create the start date with them. A balance now
	// on create needs them too; on an update it may stand alone (a set-up debt).
	hasBorrowed, hasLast, hasBalance := filled(f.amountBorrowed.String()), filled(f.lastPaymentDate), filled(f.balance.String())
	setup := hasBorrowed || hasLast || (hasBalance && !keepable)
	if setup {
		fields = append(fields, debtAmountField("amountBorrowed", f.amountBorrowed.String(), false)...)
	}
	switch {
	case hasBalance:
		fields = append(fields, debtAmountField("balance", f.balance.String(), true)...)
		fields = append(fields, debtDateField("balanceDate", f.balanceDate)...)
	case filled(f.balanceDate):
		fields = append(fields, blankField("balance"))
	}
	// an update may leave the start out: a set-up debt keeps its stored one,
	// and the service refuses one that is not set up (it has none to keep)
	if setup && !hasStart && !keepable {
		fields = append(fields, blankField("startDate"))
	}
	if !keepable || filled(f.payment.String()) {
		fields = append(fields, debtAmountField("payment", f.payment.String(), false)...)
	}
	if !filled(f.schedule) {
		if !keepable {
			fields = append(fields, blankField("schedule"))
		}
	} else if _, ok := ParseRecurringSchedule(f.schedule); !ok {
		fields = append(fields, errs.FieldError{Key: "schedule", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	if !hasLast {
		if setup {
			fields = append(fields, blankField("lastPaymentDate"))
		}
	} else if _, err := time.Parse(datetime.DateLayout, f.lastPaymentDate); err != nil {
		fields = append(fields, debtInvalidField("lastPaymentDate"))
	}
	return fields
}

// plainDecimal is the form the stored decimal reads; ParseFloat also takes
// "1_000", hex floats and exponents, which would be stored as 0.
var plainDecimal = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)$`)

// debtRateField: a yearly percent from 0 to 100.
func debtRateField(key, v string) []errs.FieldError {
	v = strings.TrimSpace(v)
	if v == "" {
		return []errs.FieldError{blankField(key)}
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || !plainDecimal.MatchString(v) {
		return []errs.FieldError{debtInvalidField(key)}
	}
	if f < 0 || f > 100 {
		return []errs.FieldError{{Key: key, Message: "This value is out of range.", Code: errs.CodeOutOfRange}}
	}
	return nil
}

// debtAmountField: a balance may be zero, a payment or an amount borrowed
// must be positive.
func debtAmountField(key, v string, allowZero bool) []errs.FieldError {
	if strings.TrimSpace(v) == "" {
		return []errs.FieldError{blankField(key)}
	}
	v = strings.TrimSpace(v)
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || !plainDecimal.MatchString(v) {
		return []errs.FieldError{debtInvalidField(key)}
	}
	if f < 0 || (f == 0 && !allowZero) {
		return []errs.FieldError{{Key: key, Message: "This value is out of range.", Code: errs.CodeOutOfRange}}
	}
	return nil
}
