// Request/result DTOs for the debt use cases, with their tier-1 Validate() methods.
package model

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

const debtNameMaxLength = 64

type CreateDebtRequest struct {
	Name            string        `json:"name"`
	CurrencyId      string        `json:"currencyId"`
	CategoryId      string        `json:"categoryId"`
	PayeeId         *string       `json:"payeeId"`
	TagId           *string       `json:"tagId"`
	LabelId         *string       `json:"labelId"`
	StartDate       *string       `json:"startDate"`
	Balance         vo.FlexString `json:"balance"`
	Payment         vo.FlexString `json:"payment"`
	Schedule        string        `json:"schedule"`
	LastPaymentDate string        `json:"lastPaymentDate"`
}

func (r CreateDebtRequest) Validate() error {
	fields := debtFields(r.Name, r.CategoryId, r.PayeeId, r.TagId, r.LabelId, r.StartDate, r.Balance, r.Payment, r.Schedule, r.LastPaymentDate)
	fields = append(fields, debtIDField("currencyId", r.CurrencyId, true)...)
	return debtValidation(fields)
}

// UpdateDebtRequest carries the same fields as create. The currency is never
// changed: CurrencyId is accepted and ignored.
type UpdateDebtRequest struct {
	Id              string        `json:"id"`
	Name            string        `json:"name"`
	CurrencyId      string        `json:"currencyId"`
	CategoryId      string        `json:"categoryId"`
	PayeeId         *string       `json:"payeeId"`
	TagId           *string       `json:"tagId"`
	LabelId         *string       `json:"labelId"`
	StartDate       *string       `json:"startDate"`
	Balance         vo.FlexString `json:"balance"`
	Payment         vo.FlexString `json:"payment"`
	Schedule        string        `json:"schedule"`
	LastPaymentDate string        `json:"lastPaymentDate"`
}

func (r UpdateDebtRequest) Validate() error {
	fields := debtFields(r.Name, r.CategoryId, r.PayeeId, r.TagId, r.LabelId, r.StartDate, r.Balance, r.Payment, r.Schedule, r.LastPaymentDate)
	fields = append(fields, debtIDField("id", r.Id, true)...)
	return debtValidation(fields)
}

type DeleteDebtRequest struct {
	Id                 string `json:"id"`
	ConfirmDataRemoval bool   `json:"confirmDataRemoval"`
}

func (r DeleteDebtRequest) Validate() error {
	return debtValidation(debtIDField("id", r.Id, true))
}

type DeleteDebtResult struct{}

type MarkDebtPaidOffRequest struct {
	Id string `json:"id"`
}

func (r MarkDebtPaidOffRequest) Validate() error {
	return debtValidation(debtIDField("id", r.Id, true))
}

type GetDebtRequest struct {
	Id string `json:"id"`
}

func (r GetDebtRequest) Validate() error {
	return debtValidation(debtIDField("id", r.Id, true))
}

// DebtResult is one debt in the API. The terms (balance .. lastPaymentDate)
// are the latest snapshot's; amounts are decimal strings in the debt's
// currency, dates "2006-01-02" ("" when none), isPaidOff an int 0/1 and
// periodRate the implied rate per schedule period with 10 decimals. While
// isSetUp is 0 the terms and figures are "" and the counts 0.
type DebtResult struct {
	Id              string  `json:"id"`
	Name            string  `json:"name"`
	Icon            string  `json:"icon"`
	CurrencyId      string  `json:"currencyId"`
	CategoryId      string  `json:"categoryId"`
	PayeeId         *string `json:"payeeId"`
	TagId           *string `json:"tagId"`
	LabelId         *string `json:"labelId"`
	StartDate       string  `json:"startDate"`
	Balance         string  `json:"balance"`
	Payment         string  `json:"payment"`
	Schedule        string  `json:"schedule"`
	LastPaymentDate string  `json:"lastPaymentDate"`
	CurrentBalance  string  `json:"currentBalance"`
	PaymentsLeft    int     `json:"paymentsLeft"`
	PeriodsPaid     int     `json:"periodsPaid"`
	MoneyLeft       string  `json:"moneyLeft"`
	OverpaymentLeft string  `json:"overpaymentLeft"`
	OverpaidSoFar   string  `json:"overpaidSoFar"`
	PayoffDate      string  `json:"payoffDate"`
	NextDueDate     string  `json:"nextDueDate"`
	PeriodRate      string  `json:"periodRate"`
	IsPaidOff       int     `json:"isPaidOff"`
	IsSetUp         int     `json:"isSetUp"`
}

type DebtListResult struct {
	Items []DebtResult `json:"items"`
}

type DebtSnapshotResult struct {
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
}

type DebtDetailsResult struct {
	Item      DebtResult           `json:"item"`
	Snapshots []DebtSnapshotResult `json:"snapshots"`
	Payments  []DebtPaymentResult  `json:"payments"`
}

// DebtTimelineRequest asks for the period-by-period projection of debts over
// a budget's periods (PeriodWindow on PeriodStartDay; 0 or 1 = calendar
// months). Plans maps debt id -> period key "YYYY-MM-01" -> planned amount;
// Months are period keys.
type DebtTimelineRequest struct {
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

func debtOptionalIDField(key string, v *string) []errs.FieldError {
	if v == nil {
		return nil
	}
	return debtIDField(key, *v, false)
}

func debtFields(name, categoryID string, payeeID, tagID, labelID, startDate *string, balance, payment vo.FlexString, schedule, lastPaymentDate string) []errs.FieldError {
	var fields []errs.FieldError
	switch n := strings.TrimSpace(name); {
	case n == "":
		fields = append(fields, blankField("name"))
	case utf8.RuneCountInString(n) > debtNameMaxLength:
		fields = append(fields, errs.FieldError{Key: "name", Message: "This value is too long.", Code: errs.CodeTooLong})
	}
	fields = append(fields, debtIDField("categoryId", categoryID, true)...)
	fields = append(fields, debtOptionalIDField("payeeId", payeeID)...)
	fields = append(fields, debtOptionalIDField("tagId", tagID)...)
	fields = append(fields, debtOptionalIDField("labelId", labelID)...)
	if startDate != nil && strings.TrimSpace(*startDate) != "" {
		if _, err := time.Parse(datetime.DateLayout, *startDate); err != nil {
			fields = append(fields, debtInvalidField("startDate"))
		}
	}
	// Balance and last payment date are the optional terms: both or neither.
	hasBalance, hasLast := strings.TrimSpace(balance.String()) != "", strings.TrimSpace(lastPaymentDate) != ""
	if hasBalance || hasLast {
		fields = append(fields, debtAmountField("balance", balance.String(), true)...)
	}
	fields = append(fields, debtAmountField("payment", payment.String(), false)...)
	if strings.TrimSpace(schedule) == "" {
		fields = append(fields, blankField("schedule"))
	} else if _, ok := ParseRecurringSchedule(schedule); !ok {
		fields = append(fields, errs.FieldError{Key: "schedule", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	if !hasLast {
		if hasBalance {
			fields = append(fields, blankField("lastPaymentDate"))
		}
	} else if _, err := time.Parse(datetime.DateLayout, lastPaymentDate); err != nil {
		fields = append(fields, debtInvalidField("lastPaymentDate"))
	}
	return fields
}

// plainDecimal is the form the stored decimal reads; ParseFloat also takes
// "1_000", hex floats and exponents, which would be stored as 0.
var plainDecimal = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)$`)

// debtAmountField: a balance may be zero, a payment must be positive.
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
