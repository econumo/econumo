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

const elementDebt int16 = 6

type Service struct {
	repo       Repository
	tx         port.TxRunner
	clock      port.Clock
	class      Classifications
	rates      Rates
	currencies Currencies
	budget     BudgetElements
}

func NewService(repo Repository, tx port.TxRunner, clock port.Clock, class Classifications, rates Rates, currencies Currencies) *Service {
	return &Service{repo: repo, tx: tx, clock: clock, class: class, rates: rates, currencies: currencies}
}

// SetBudgetElements installs the budget peer; the budget service is built
// after this one, so it cannot be a constructor argument.
func (s *Service) SetBudgetElements(b BudgetElements) { s.budget = b }

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
		CurrencyID: d.CurrencyID, SumOnClash: true, FirstMonth: first, LastMonth: last,
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

func (s *Service) lockRows(ctx context.Context, id vo.Id) error {
	if s.budget == nil {
		return nil
	}
	return s.budget.LockRows(ctx, id, elementDebt)
}

func (s *Service) hasData(ctx context.Context, id vo.Id) (bool, error) {
	if s.budget == nil {
		return false, nil
	}
	return s.budget.HasData(ctx, id, elementDebt)
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
	return d.PayeeID == nil && d.TagID == nil && d.StartDate == nil
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
// only with distinct start dates, which partition the transactions.
func checkKeys(d *model.Debt, others []*model.Debt) error {
	for _, o := range others {
		if o.ID.Equal(d.ID) || !o.CategoryID.Equal(d.CategoryID) {
			continue
		}
		if sameIdPtr(o.PayeeID, d.PayeeID) && sameIdPtr(o.TagID, d.TagID) {
			if sameDay(o.StartDate, d.StartDate) {
				return fieldError("startDate", "Another debt with the same category, payee and tag already starts on this date.", errs.CodeDebtStartDateTaken)
			}
			continue
		}
		if idsOverlap(o.PayeeID, d.PayeeID) && idsOverlap(o.TagID, d.TagID) {
			return fieldError("categoryId", "Another debt could match the same payments.", errs.CodeDebtKeyConflict)
		}
	}
	return nil
}

func termsFloats(t model.DebtTerms) model.MathSnapshot {
	return model.MathSnapshot{AsOf: t.AsOf, Balance: toFloat(t.Balance), Payment: toFloat(t.Payment),
		Schedule: t.Schedule, Last: t.LastPaymentDate}
}

// validateTerms checks terms about to become a snapshot as of terms.AsOf.
func validateTerms(t model.DebtTerms) error {
	if !t.LastPaymentDate.After(t.AsOf) {
		return fieldError("lastPaymentDate", "The last payment date must be after today.", errs.CodeDebtLastPaymentInPast)
	}
	if !t.Payment.IsGreaterThan(vo.NewDecimal("0")) {
		return fieldError("payment", "This value is out of range.", errs.CodeOutOfRange)
	}
	m := termsFloats(t)
	n := len(model.DueDatesBetween(m.Schedule, m.Last, m.AsOf, m.Last))
	if _, err := model.ImpliedRate(m.Balance, m.Payment, n); err != nil {
		return fieldError("payment", "The payments do not cover the balance.", errs.CodeDebtPaymentsDontCover)
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
// owner and the optional payee and tag are the owner's.
func (s *Service) checkClassifications(ctx context.Context, ownerID vo.Id, categoryID vo.Id, payeeID, tagID *vo.Id) error {
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

func isNotFound(err error) bool {
	_, ok := errs.AsNotFound(err)
	return ok
}

// debtInput is a create/update body parsed into domain values; the balance and
// payment are rounded to the debt currency once its digits are known.
type debtInput struct {
	name               string
	currency, category vo.Id
	payee, tag         *vo.Id
	start              *time.Time
	balance, payment   vo.DecimalNumber
	schedule           model.RecurringSchedule
	lastPayment        time.Time
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

func parseInput(name, currencyID, categoryID string, payeeID, tagID, startDate *string, balance, payment vo.FlexString, schedule, lastPayment string) (debtInput, error) {
	in := debtInput{name: name, balance: vo.NewDecimal(balance.String()), payment: vo.NewDecimal(payment.String())}
	var err error
	if currencyID != "" {
		if in.currency, err = vo.ParseId(currencyID); err != nil {
			return in, err
		}
	}
	if in.category, err = vo.ParseId(categoryID); err != nil {
		return in, err
	}
	if in.payee, err = parseOptionalID(payeeID); err != nil {
		return in, err
	}
	if in.tag, err = parseOptionalID(tagID); err != nil {
		return in, err
	}
	if startDate != nil && strings.TrimSpace(*startDate) != "" {
		start, err := time.Parse(datetime.DateLayout, *startDate)
		if err != nil {
			return in, err
		}
		in.start = &start
	}
	sched, ok := model.ParseRecurringSchedule(schedule)
	if !ok {
		return in, errs.NewValidation("Validation failed", errs.FieldError{
			Key: "schedule", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	in.schedule = sched
	if in.lastPayment, err = time.Parse(datetime.DateLayout, lastPayment); err != nil {
		return in, err
	}
	return in, nil
}

// terms are the snapshot values of the input as of today.
func (in debtInput) terms(today time.Time, digits int) model.DebtTerms {
	return model.DebtTerms{AsOf: today, Balance: in.balance.Round(digits), Payment: in.payment.Round(digits),
		Schedule: in.schedule, LastPaymentDate: in.lastPayment}
}
