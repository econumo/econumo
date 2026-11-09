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
	class      Classifications
	rates      Rates
	currencies Currencies
	owners     Owners
	budget     BudgetElements
	plans      BudgetPlans
}

func NewService(repo Repository, tx port.TxRunner, clock port.Clock, class Classifications, rates Rates, currencies Currencies, owners Owners) *Service {
	return &Service{repo: repo, tx: tx, clock: clock, class: class, rates: rates, currencies: currencies, owners: owners}
}

// SetBudgetElements installs the budget peer; the budget service is built
// after this one, so it cannot be a constructor argument.
func (s *Service) SetBudgetElements(b BudgetElements) { s.budget = b }

// SetBudgetPlans installs the source of the forecast's planned extras.
func (s *Service) SetBudgetPlans(p BudgetPlans) { s.plans = p }

// moveToDebt hands the category row's plans and comments to a whole-category
// debt's row, only for the months the debt can be planned in: the rest stays
// on the category, whose Available keeps counting it. A month both rows plan
// adds up, so nothing is lost.
func (s *Service) moveToDebt(ctx context.Context, d *model.Debt) error {
	if s.budget == nil {
		return nil
	}
	st, err := s.stateNow(ctx, d)
	if err != nil {
		return err
	}
	first, last := planningBounds(st, st.project(s.today(ctx), nil, nil))
	_, err = s.budget.TransferData(ctx, model.ElementDataTransfer{
		FromID: d.CategoryID, FromType: model.ElementCategory, ToID: d.ID, ToType: model.ElementDebt,
		CurrencyID: d.CurrencyID, SumOnClash: true, FirstDay: first, LastDay: last,
	})
	return err
}

// moveTailToCategory hands the plans and comments after a debt's end back to
// category, the whole category the debt covered before setup: it took them
// with no end, and its row hides every month past the end. currency is the
// one the debt's rows are in.
func (s *Service) moveTailToCategory(ctx context.Context, d *model.Debt, category, currency vo.Id) error {
	if s.budget == nil {
		return nil
	}
	st, err := s.stateNow(ctx, d)
	if err != nil {
		return err
	}
	_, last := planningBounds(st, st.project(s.today(ctx), nil, nil))
	if last.IsZero() {
		return nil
	}
	if err := s.lockRows(ctx, d.ID); err != nil {
		return err
	}
	_, err = s.budget.TransferData(ctx, model.ElementDataTransfer{
		FromID: d.ID, FromType: model.ElementDebt, ToID: category, ToType: model.ElementCategory,
		CurrencyID: currency, SumOnClash: true, AfterDay: last,
	})
	return err
}

// firstPlanningDay is the first day of the debt's planning bounds as stored.
func (s *Service) firstPlanningDay(ctx context.Context, d *model.Debt) (time.Time, error) {
	st, err := s.stateNow(ctx, d)
	if err != nil {
		return time.Time{}, err
	}
	first, _ := planningBounds(st, st.project(s.today(ctx), nil, nil))
	return first, nil
}

// moveHeadToCategory hands the plans and comments before a debt's new first
// planning day back to category, the whole category the debt covered: it
// took them from its old first day on, and its row hides every month before
// its start. A start that did not move later moves nothing. currency is the
// one the debt's rows are in.
func (s *Service) moveHeadToCategory(ctx context.Context, d *model.Debt, category vo.Id, wasFirst time.Time, currency vo.Id) error {
	if s.budget == nil {
		return nil
	}
	first, err := s.firstPlanningDay(ctx, d)
	if err != nil {
		return err
	}
	if !first.After(wasFirst) {
		return nil
	}
	if err := s.lockRows(ctx, d.ID); err != nil {
		return err
	}
	_, err = s.budget.TransferData(ctx, model.ElementDataTransfer{
		FromID: d.ID, FromType: model.ElementDebt, ToID: category, ToType: model.ElementCategory,
		CurrencyID: currency, SumOnClash: true, BeforeDay: first,
	})
	return err
}

// moveToCategory hands a deleted whole-category debt's row back to its
// category; a month the category already plans keeps the category's plan.
func (s *Service) moveToCategory(ctx context.Context, d *model.Debt) (leftBehind bool, err error) {
	if s.budget == nil {
		return false, nil
	}
	return s.budget.TransferData(ctx, model.ElementDataTransfer{
		FromID: d.ID, FromType: model.ElementDebt, ToID: d.CategoryID, ToType: model.ElementCategory, CurrencyID: d.CurrencyID,
	})
}

func (s *Service) removeRows(ctx context.Context, id vo.Id) error {
	if s.budget == nil {
		return nil
	}
	return s.budget.RemoveRows(ctx, id, model.ElementDebt)
}

func (s *Service) setRowsCurrency(ctx context.Context, id, currencyID vo.Id) error {
	if s.budget == nil {
		return nil
	}
	return s.budget.SetRowsCurrency(ctx, id, model.ElementDebt, currencyID)
}

func (s *Service) lockRows(ctx context.Context, id vo.Id) error {
	if s.budget == nil {
		return nil
	}
	return s.budget.LockRows(ctx, id, model.ElementDebt)
}

func (s *Service) hasData(ctx context.Context, id vo.Id) (bool, error) {
	if s.budget == nil {
		return false, nil
	}
	return s.budget.HasData(ctx, id, model.ElementDebt)
}

// ownerLocation is the owner's stored timezone, UTC when none (or an unknown
// one) is stored: the creation day is taken in it, so a debt's planning start
// does not depend on who views it.
func (s *Service) ownerLocation(ctx context.Context, ownerID vo.Id) (*time.Location, error) {
	tz, err := s.owners.GetTimezone(ctx, ownerID)
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

// today is the caller's calendar day as a UTC midnight, the form every debt
// date is stored and compared in.
func (s *Service) today(ctx context.Context) time.Time {
	y, m, d := s.clock.Now().In(reqctx.Location(ctx)).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ownedDebt masks a debt of another user as not found.
func (s *Service) ownedDebt(ctx context.Context, userID, id vo.Id) (*model.Debt, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !d.OwnerID.Equal(userID) {
		return nil, errs.NewNotFound("Debt not found")
	}
	return d, nil
}

func isWholeCategory(d *model.Debt) bool {
	return d.PayeeID == nil && d.TagID == nil && d.LabelID == nil && d.StartDate == nil
}

func sameIdPtr(a, b *vo.Id) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// idsOverlap: two optional key parts can match one transaction when either is
// unset or both are equal.
func idsOverlap(a, b *vo.Id) bool {
	return a == nil || b == nil || a.Equal(*b)
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

// checkKeys refuses a debt whose key can match a transaction another debt of
// the owner in the same category also matches. Identical keys are allowed
// only with distinct start dates, which partition the transactions. Two
// debts naming different labels may overlap otherwise: a payment carrying
// both labels goes to the older debt.
func checkKeys(d *model.Debt, others []*model.Debt) error {
	for _, o := range others {
		if o.ID.Equal(d.ID) || !o.CategoryID.Equal(d.CategoryID) {
			continue
		}
		if o.LabelID != nil && d.LabelID != nil && !o.LabelID.Equal(*d.LabelID) {
			continue
		}
		if sameIdPtr(o.PayeeID, d.PayeeID) && sameIdPtr(o.TagID, d.TagID) && sameIdPtr(o.LabelID, d.LabelID) {
			if sameDay(o.StartDate, d.StartDate) {
				return fieldError("startDate", "Another debt with the same category, payee, tag and reporting tag already starts on this date.", errs.CodeDebtStartDateTaken)
			}
			continue
		}
		if idsOverlap(o.PayeeID, d.PayeeID) && idsOverlap(o.TagID, d.TagID) {
			return fieldError("categoryId", "Another debt could match the same payments.", errs.CodeDebtKeyConflict)
		}
	}
	return nil
}

func termsFloats(t model.DebtTerms, digits int) model.MathSnapshot {
	return model.MathSnapshot{AsOf: t.AsOf, Balance: toFloat(t.Balance), Payment: toFloat(t.Payment),
		Schedule: t.Schedule, Last: t.LastPaymentDate, Digits: digits}
}

// validateTerms checks terms about to become a snapshot as of terms.AsOf, in
// a currency with the given fraction digits.
func validateTerms(t model.DebtTerms, digits int) error {
	if !t.LastPaymentDate.After(t.AsOf) {
		return fieldError("lastPaymentDate", "The last payment date must be after today.", errs.CodeDebtLastPaymentInPast)
	}
	if model.DebtTermTooLong(t.AsOf, t.LastPaymentDate) {
		return fieldError("lastPaymentDate", "This value is out of range.", errs.CodeOutOfRange)
	}
	if err := validatePayment(t.Payment); err != nil {
		return err
	}
	m := termsFloats(t, digits)
	n := len(model.DueDatesBetween(m.Schedule, m.Last, m.AsOf, m.Last))
	if _, err := model.ImpliedRate(m.Balance, m.Payment, n, digits); err != nil {
		return fieldError("payment", "The payments do not cover the balance.", errs.CodeDebtPaymentsDontCover)
	}
	return nil
}

func validatePayment(p vo.DecimalNumber) error {
	if !p.IsGreaterThan(vo.NewDecimal("0")) {
		return fieldError("payment", "This value is out of range.", errs.CodeOutOfRange)
	}
	return nil
}

func termsDiffer(a, b model.DebtTerms) bool {
	return !a.Balance.Equals(b.Balance) || !a.Payment.Equals(b.Payment) ||
		a.Schedule != b.Schedule || !a.LastPaymentDate.Equal(b.LastPaymentDate)
}

func (s *Service) snapshotOf(d *model.Debt, now time.Time) *model.DebtSnapshot {
	t := d.Current
	return &model.DebtSnapshot{ID: s.repo.NextIdentity(), DebtID: d.ID, AsOf: t.AsOf, Balance: t.Balance,
		Payment: t.Payment, Schedule: t.Schedule, LastPaymentDate: t.LastPaymentDate, CreatedAt: now}
}

// checkClassifications verifies the category is an expense category of the
// owner and the optional payee, tag and label are the owner's.
func (s *Service) checkClassifications(ctx context.Context, ownerID vo.Id, in debtInput) error {
	categoryID, payeeID, tagID := in.category, in.payee, in.tag
	_, _, isExpense, err := s.class.CategoryOf(ctx, ownerID, categoryID)
	if err != nil {
		return err
	}
	if !isExpense {
		return fieldError("categoryId", "A debt needs an expense category.", errs.CodeDebtCategoryNotExpense)
	}
	if payeeID != nil {
		ok, err := s.class.PayeeOwned(ctx, ownerID, *payeeID)
		if err != nil {
			return err
		}
		if !ok {
			return errs.NewNotFound("Payee not found")
		}
	}
	if tagID != nil {
		ok, err := s.class.TagOwned(ctx, ownerID, *tagID)
		if err != nil {
			return err
		}
		if !ok {
			return errs.NewNotFound("Tag not found")
		}
	}
	if in.label != nil {
		ok, err := s.class.LabelOwned(ctx, ownerID, *in.label)
		if err != nil {
			return err
		}
		if !ok {
			return errs.NewNotFound("Label not found")
		}
	}
	return nil
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
	return vo.NewDecimal(strconv.FormatFloat(roundTo(x, digits), 'f', digits, 64)).String()
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

// debtInput is a create/update body parsed into domain values; the balance and
// payment are rounded to the debt currency once its digits are known. Without
// hasTerms the body carries no balance and no last payment date; without
// hasPayment or hasSchedule an update keeps the stored one.
type debtInput struct {
	name                    string
	currency, category      vo.Id
	payee, tag, label       *vo.Id
	start                   *time.Time
	balance, payment        vo.DecimalNumber
	schedule                model.RecurringSchedule
	lastPayment             time.Time
	hasTerms                bool
	hasPayment, hasSchedule bool
}

// debtBody is the part of the create and update requests parseInput reads.
type debtBody struct {
	name, currencyID, categoryID   string
	payeeID, tagID, labelID, start *string
	balance, payment               vo.FlexString
	schedule, lastPayment          string
}

func parseOptionalID(v *string) (*vo.Id, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	id, err := vo.ParseId(*v)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func parseInput(b debtBody) (debtInput, error) {
	in := debtInput{name: b.name, balance: vo.NewDecimal(b.balance.String()), payment: vo.NewDecimal(b.payment.String())}
	var err error
	if b.currencyID != "" {
		if in.currency, err = vo.ParseId(b.currencyID); err != nil {
			return in, err
		}
	}
	if in.category, err = vo.ParseId(b.categoryID); err != nil {
		return in, err
	}
	if in.payee, err = parseOptionalID(b.payeeID); err != nil {
		return in, err
	}
	if in.tag, err = parseOptionalID(b.tagID); err != nil {
		return in, err
	}
	if in.label, err = parseOptionalID(b.labelID); err != nil {
		return in, err
	}
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
	// The terms are optional, but only as a pair.
	hasBalance, hasLast := strings.TrimSpace(b.balance.String()) != "", strings.TrimSpace(b.lastPayment) != ""
	if !hasBalance && !hasLast {
		return in, nil
	}
	if !hasBalance || !hasLast {
		return in, missingTerms(!hasBalance, !hasLast)
	}
	if err := in.requirePaymentAndSchedule(); err != nil {
		return in, err
	}
	if in.lastPayment, err = time.Parse(datetime.DateLayout, b.lastPayment); err != nil {
		return in, err
	}
	in.hasTerms = true
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

// changesTerms: a payment or schedule sent that differs from the stored terms.
func (in debtInput) changesTerms(t model.DebtTerms, digits int) bool {
	return (in.hasPayment && !in.payment.Round(digits).Equals(t.Payment)) || (in.hasSchedule && in.schedule != t.Schedule)
}

func missingTerms(balance, lastPayment bool) error {
	var missing []errs.FieldError
	if balance {
		missing = append(missing, errs.FieldError{Key: "balance", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if lastPayment {
		missing = append(missing, errs.FieldError{Key: "lastPaymentDate", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return errs.NewValidation("Validation failed", missing...)
}

// terms are the snapshot values of the input as of today.
func (in debtInput) terms(today time.Time, digits int) model.DebtTerms {
	return model.DebtTerms{AsOf: today, Balance: in.balance.Round(digits), Payment: in.payment.Round(digits),
		Schedule: in.schedule, LastPaymentDate: in.lastPayment}
}
