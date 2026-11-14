package debt

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Service struct {
	repo       Repository
	tx         port.TxRunner
	clock      port.Clock
	categories Categories
	rates      Rates
	users      Users
	budgets    Budgets
	elements   Elements
}

func NewService(repo Repository, tx port.TxRunner, clock port.Clock, categories Categories, rates Rates, users Users) *Service {
	return &Service{repo: repo, tx: tx, clock: clock, categories: categories, rates: rates, users: users}
}

// SetBudgets installs the budget access peer; the budget service is built
// after this one, so it cannot be a constructor argument.
func (s *Service) SetBudgets(b Budgets) { s.budgets = b }

// SetElements installs the budget elements peer.
func (s *Service) SetElements(e Elements) { s.elements = e }

// LockBudget takes the budget's row lock in the caller's transaction, for a
// budget write that reads or changes the debts of its elements: the budget
// row is locked before any element row, everywhere.
func (s *Service) LockBudget(ctx context.Context, budgetID vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error { return s.repo.LockBudget(ctx, budgetID) })
}

// userLocation is the user's stored timezone, UTC when none (or an unknown
// one) is stored: the creation day is taken in the creator's, so a debt's
// planning start does not depend on who views it.
func (s *Service) userLocation(ctx context.Context, userID vo.Id) (*time.Location, error) {
	tz, err := s.users.GetTimezone(ctx, userID)
	if err != nil {
		return nil, err
	}
	if tz == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC, nil
	}
	return loc, nil
}

// inOneTx runs a debt write whole in one transaction: the sync before it, the
// write, and the sync that presents it, each nested WithTx a savepoint. The
// request's commit hook (the Idempotency-Key) then fires once, at the write's
// commit: a sync committing on its own would record the key before the write,
// so a retry could be told it landed while it had not, or never land at all.
// A write refused after its sync rolls the sync back with it.
func inOneTx[T any](ctx context.Context, s *Service, fn func(ctx context.Context) (T, error)) (T, error) {
	var out T
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = fn(ctx)
		return err
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return out, nil
}

// today is the caller's calendar day as a UTC midnight, the form every debt
// date is stored and compared in.
func (s *Service) today(ctx context.Context) time.Time {
	y, m, d := s.clock.Now().In(reqctx.Location(ctx)).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func fieldError(key, message, code string) error {
	return errs.NewValidation("Validation failed", errs.FieldError{Key: key, Message: message, Code: code})
}

// termsFloats: origin marks the loan's first snapshot.
func termsFloats(t model.DebtTerms, digits int, origin bool) model.MathSnapshot {
	return model.MathSnapshot{AsOf: t.AsOf, Balance: toFloat(t.Balance), Payment: toFloat(t.Payment),
		Schedule: t.Schedule, Last: t.LastPaymentDate, Digits: digits, Origin: origin}
}

// mathTerms are the origin and later terms as the debt math reads them, with
// the debt's rate changes (always through WithRates, which makes the payment
// in force the changes' or the origin's).
func mathTerms(changes []*model.DebtRateChange, digits int, origin model.DebtTerms, later ...model.DebtTerms) []model.MathSnapshot {
	out := []model.MathSnapshot{termsFloats(origin, digits, true)}
	for _, t := range later {
		out = append(out, termsFloats(t, digits, false))
	}
	return model.WithRates(out, mathRates(changes, origin.Schedule))
}

// mathRates are the rate changes per period of schedule.
func mathRates(changes []*model.DebtRateChange, schedule model.RecurringSchedule) []model.MathRate {
	out := make([]model.MathRate, 0, len(changes))
	for _, c := range changes {
		out = append(out, model.MathRate{From: c.EffectiveDate, PeriodRate: model.AnnualToPeriodRate(toFloat(c.AnnualRate), schedule),
			Payment: toFloat(c.Payment)})
	}
	return out
}

// validateTerms checks terms about to become a statement as of terms.AsOf
// after origin, in a currency with the given fraction digits. A balance of 0
// needs no payments left; any other must be repayable by the payment in
// force at the rate in force (the loan's, the origin's, until a rate change).
func validateTerms(t, origin model.DebtTerms, digits int, changes []*model.DebtRateChange) error {
	if err := validatePayment(t.Payment); err != nil {
		return err
	}
	if !t.Balance.IsGreaterThan(vo.NewDecimal("0")) {
		return nil
	}
	if !t.LastPaymentDate.After(t.AsOf) {
		return fieldError("lastPaymentDate", "The last payment date must be after today.", errs.CodeDebtLastPaymentInPast)
	}
	ms := mathTerms(changes, digits, origin, t)
	if err := model.StatementRepayable(ms[0], ms[1]); err != nil {
		return paymentsDontCover()
	}
	return nil
}

// validateOrigin checks the loan's facts: the origin as of the start date,
// which today may not precede, ending on the last payment date.
func validateOrigin(o model.DebtTerms, today time.Time, digits int) error {
	if err := validatePayment(o.Payment); err != nil {
		return err
	}
	if o.AsOf.After(today) {
		return fieldError("startDate", "The start date must be today or earlier.", errs.CodeDebtStartInFuture)
	}
	if !o.LastPaymentDate.After(o.AsOf) {
		return fieldError("lastPaymentDate", "The last payment date must be after the start date.", errs.CodeDebtLastPaymentBeforeStart)
	}
	return validateCover(o, digits, true)
}

// validateCover: the payments due after the origin's day must repay the
// amount borrowed, within the projection's horizon (its first period prorated).
func validateCover(t model.DebtTerms, digits int, origin bool) error {
	if model.DebtTermTooLong(t.AsOf, t.LastPaymentDate) {
		return fieldError("lastPaymentDate", "This value is out of range.", errs.CodeOutOfRange)
	}
	if _, err := model.SnapshotRate(model.WithRates([]model.MathSnapshot{termsFloats(t, digits, origin)}, nil)[0]); err != nil {
		return paymentsDontCover()
	}
	return nil
}

func paymentsDontCover() error {
	return fieldError("payment", "The payments do not cover the balance.", errs.CodeDebtPaymentsDontCover)
}

func validatePayment(p vo.DecimalNumber) error {
	if !p.IsGreaterThan(vo.NewDecimal("0")) {
		return fieldError("payment", "This value is out of range.", errs.CodeOutOfRange)
	}
	return nil
}

func termsDiffer(a, b model.DebtTerms) bool {
	return !a.AsOf.Equal(b.AsOf) || !a.Balance.Equals(b.Balance) || !a.Payment.Equals(b.Payment) ||
		a.Schedule != b.Schedule || !a.LastPaymentDate.Equal(b.LastPaymentDate)
}

func (s *Service) snapshotOf(debtID vo.Id, t model.DebtTerms, now time.Time) *model.DebtSnapshot {
	return &model.DebtSnapshot{ID: s.repo.NextIdentity(), DebtID: debtID, AsOf: t.AsOf, Balance: t.Balance,
		Payment: t.Payment, Schedule: t.Schedule, LastPaymentDate: t.LastPaymentDate, CreatedAt: now}
}

func snapshotTerms(sn *model.DebtSnapshot) model.DebtTerms {
	return model.DebtTerms{AsOf: sn.AsOf, Balance: sn.Balance, Payment: sn.Payment, Schedule: sn.Schedule, LastPaymentDate: sn.LastPaymentDate}
}

func toFloat(d vo.DecimalNumber) float64 {
	f, err := strconv.ParseFloat(d.String(), 64)
	if err != nil {
		return 0
	}
	return f
}

func roundTo(x float64, digits int) float64 {
	p := math.Pow10(digits)
	r := math.Round(x*p) / p
	if r == 0 {
		return 0
	}
	return r
}

func amountString(x float64, digits int) string {
	return decimalOf(x, digits).String()
}

func decimalOf(x float64, digits int) vo.DecimalNumber {
	return vo.NewDecimal(strconv.FormatFloat(roundTo(x, digits), 'f', digits, 64))
}

// rateString is a yearly percent as the wire and the store carry it: at most
// 8 decimals, trailing zeros trimmed.
func rateString(annual float64) string {
	return vo.NewDecimal(strconv.FormatFloat(annual, 'f', rateDigits, 64)).String()
}

func dateString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(datetime.DateLayout)
}

func monthOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func debtDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func isNotFound(err error) bool {
	_, ok := errs.AsNotFound(err)
	return ok
}

// debtInput is a create/update body parsed into domain values; the amounts
// are rounded to the debt currency once its digits are known. hasOrigin: the
// body carries the loan's facts (amount borrowed and last payment date, with
// a start date); hasOwed: a statement balance, as of owedOn (zero = today).
// Without hasPayment or hasSchedule an update keeps the stored one.
type debtInput struct {
	start                   *time.Time
	borrowed, owed, payment vo.DecimalNumber
	schedule                model.RecurringSchedule
	lastPayment, owedOn     time.Time
	hasOrigin, hasOwed      bool
	hasPayment, hasSchedule bool
}

// debtBody is the part of the create and update requests parseInput reads.
type debtBody struct {
	start                      *string
	borrowed, balance, payment vo.FlexString
	schedule, lastPayment      string
	balanceDate                string
}

func parseInput(b debtBody) (debtInput, error) {
	in := debtInput{borrowed: vo.NewDecimal(b.borrowed.String()), owed: vo.NewDecimal(b.balance.String()),
		payment: vo.NewDecimal(b.payment.String())}
	var err error
	if b.start != nil && strings.TrimSpace(*b.start) != "" {
		start, err := time.Parse(datetime.DateLayout, *b.start)
		if err != nil {
			return in, err
		}
		in.start = &start
	}
	in.hasPayment = strings.TrimSpace(b.payment.String()) != ""
	if in.hasSchedule = strings.TrimSpace(b.schedule) != ""; in.hasSchedule {
		sched, ok := model.ParseRecurringSchedule(b.schedule)
		if !ok {
			return in, errs.NewValidation("Validation failed", errs.FieldError{
				Key: "schedule", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
		}
		in.schedule = sched
	}
	in.hasOwed = strings.TrimSpace(b.balance.String()) != ""
	if strings.TrimSpace(b.balanceDate) != "" {
		if in.owedOn, err = time.Parse(datetime.DateLayout, b.balanceDate); err != nil {
			return in, err
		}
	}
	// The amount borrowed and the last payment date come together; the start
	// date with them is checked by the caller (a set-up debt keeps its own).
	hasBorrowed, hasLast := strings.TrimSpace(b.borrowed.String()) != "", strings.TrimSpace(b.lastPayment) != ""
	if !hasBorrowed && !hasLast {
		return in, nil
	}
	if !hasBorrowed || !hasLast {
		return in, missingFacts(!hasBorrowed, false, !hasLast)
	}
	if in.lastPayment, err = time.Parse(datetime.DateLayout, b.lastPayment); err != nil {
		return in, err
	}
	in.hasOrigin = true
	return in, nil
}

func (in debtInput) requirePaymentAndSchedule() error {
	var missing []errs.FieldError
	if !in.hasPayment {
		missing = append(missing, errs.FieldError{Key: "payment", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if !in.hasSchedule {
		missing = append(missing, errs.FieldError{Key: "schedule", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if len(missing) == 0 {
		return nil
	}
	return errs.NewValidation("Validation failed", missing...)
}

// missingFacts names the loan's facts a body left out.
func missingFacts(borrowed, start, last bool) error {
	var missing []errs.FieldError
	for _, f := range []struct {
		key  string
		miss bool
	}{{"amountBorrowed", borrowed}, {"startDate", start}, {"lastPaymentDate", last}} {
		if f.miss {
			missing = append(missing, errs.FieldError{Key: f.key, Message: "This value should not be blank.", Code: errs.CodeIsBlank})
		}
	}
	return errs.NewValidation("Validation failed", missing...)
}

// origin is the debt's first snapshot as the body states it, each fact left
// out taken from base (the stored origin, or the stored payment and schedule
// of a debt not set up), the amounts rounded to digits.
func (in debtInput) origin(base model.DebtTerms, digits int) model.DebtTerms {
	o := roundedTerms(base, digits)
	if in.start != nil {
		o.AsOf = *in.start
	}
	if in.hasOrigin {
		o.Balance, o.LastPaymentDate = in.borrowed.Round(digits), in.lastPayment
	}
	if in.hasPayment {
		o.Payment = in.payment.Round(digits)
	}
	if in.hasSchedule {
		o.Schedule = in.schedule
	}
	return o
}

// statement is the body's statement balance as a snapshot under terms t, as
// of its date (today when none).
func (in debtInput) statement(t model.DebtTerms, today time.Time, digits int) model.DebtTerms {
	t.AsOf, t.Balance = today, in.owed.Round(digits)
	if !in.owedOn.IsZero() {
		t.AsOf = in.owedOn
	}
	return t
}
