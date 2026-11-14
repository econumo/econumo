package debt

import (
	"context"
	"math"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// payment is a matched expense in the debt's currency with its stored split.
type payment struct {
	txID     vo.Id
	date     time.Time
	amount   float64
	interest float64
}

type debtState struct {
	debt   *model.Debt
	view   model.DebtView
	snaps  []*model.DebtSnapshot
	rates  []*model.DebtRateChange // by effective date
	pays   []payment               // by date, then id
	digits int
}

func (st *debtState) mathSnaps() []model.MathSnapshot {
	out := make([]model.MathSnapshot, 0, len(st.snaps))
	for i, s := range st.snaps {
		// the first snapshot of a set-up debt is its origin (by position: see
		// Repository.Snapshots for why the ordering keeps it first)
		out = append(out, model.MathSnapshot{AsOf: s.AsOf, Balance: toFloat(s.Balance), Payment: toFloat(s.Payment),
			Schedule: s.Schedule, Last: s.LastPaymentDate, Digits: st.digits, Origin: i == 0})
	}
	if len(out) == 0 {
		return out
	}
	return model.WithRates(out, mathRates(st.rates, st.snaps[0].Schedule))
}

func (st *debtState) mathPays() ([]model.MathPayment, map[string]float64) {
	out := make([]model.MathPayment, 0, len(st.pays))
	interest := make(map[string]float64, len(st.pays))
	for _, p := range st.pays {
		id := p.txID.String()
		out = append(out, model.MathPayment{ID: id, Date: p.date, Amount: p.amount})
		interest[id] = p.interest
	}
	return out, interest
}

func (st *debtState) project(today time.Time, months []time.Time, plans map[string]float64) model.Projection {
	return st.projectPeriods(today, months, plans, model.MinPeriodStartDay)
}

// projectPeriods projects over budget periods starting on startDay.
func (st *debtState) projectPeriods(today time.Time, months []time.Time, plans map[string]float64, startDay int) model.Projection {
	pays, interest := st.mathPays()
	return model.ProjectDebt(model.ProjectionInput{Snaps: st.mathSnaps(), Pays: pays, Interest: interest,
		Today: today, Months: months, Plans: plans, Digits: st.digits, PeriodStartDay: startDay})
}

// Sync brings the stored payment splits of the budget's debts up to date.
func (s *Service) Sync(ctx context.Context, scope model.DebtScope) error {
	_, err := s.syncBudget(ctx, scope)
	return err
}

// syncBudget matches the expenses on the budget's accounts to its debts,
// drops splits that no longer belong and splits the new payments, then
// returns the debts' states in creation order. It runs in one transaction
// under the budget's row lock. Splits of other budgets' debts are never read
// or touched: a transaction can count for one debt in each budget.
func (s *Service) syncBudget(ctx context.Context, scope model.DebtScope) ([]*debtState, error) {
	if scope.BudgetID.IsZero() {
		return nil, nil
	}
	var states []*debtState
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockBudget(ctx, scope.BudgetID); err != nil {
			return err
		}
		debts, err := s.repo.ListByBudget(ctx, scope.BudgetID)
		if err != nil {
			return err
		}
		states, err = s.computeDebts(ctx, scope, debts, true)
		return err
	})
	if err != nil {
		return nil, err
	}
	return states, nil
}

// syncBudgetID is syncBudget for a budget whose accounts are not at hand.
func (s *Service) syncBudgetID(ctx context.Context, budgetID vo.Id) error {
	scope, err := s.scope(ctx, budgetID)
	if err != nil {
		return err
	}
	_, err = s.syncBudget(ctx, scope)
	return err
}

// readStates is syncBudget without a write or a lock: splits not stored yet
// are computed in memory, and stale stored ones are ignored.
func (s *Service) readStates(ctx context.Context, scope model.DebtScope) ([]*debtState, error) {
	if scope.BudgetID.IsZero() {
		return nil, nil
	}
	debts, err := s.repo.ListByBudget(ctx, scope.BudgetID)
	if err != nil {
		return nil, err
	}
	return s.computeDebts(ctx, scope, debts, false)
}

func ruleViews(rules []model.DebtRule) []model.DebtRuleView {
	out := make([]model.DebtRuleView, 0, len(rules))
	for _, r := range rules {
		out = append(out, model.DebtRuleView{CategoryID: r.CategoryID.String(), PayeeID: idString(r.PayeeID),
			TagID: idString(r.TagID), LabelID: idString(r.LabelID)})
	}
	return out
}

// matchViews assigns every debt its rules and range: a debt matches from its
// start up to the start of the next debt of the budget with an identical
// rule, on every one of its rules (model.DebtUntil).
func matchViews(debts []*model.Debt) map[vo.Id]model.DebtView {
	views := make(map[vo.Id]model.DebtView, len(debts))
	for _, d := range debts {
		v := model.DebtView{
			ID: d.ID.String(), BudgetID: d.BudgetID.String(), CurrencyID: d.CurrencyID.String(), Rules: ruleViews(d.Rules),
			Name: d.Name, CreatedAt: d.CreatedAt, SetUp: d.IsSetUp(), Until: model.DebtUntil(d, debts),
		}
		if d.StartDate != nil {
			from := *d.StartDate
			v.From = &from
		}
		views[d.ID] = v
	}
	return views
}

// computeDebts matches the expenses on the scope's accounts in the debts'
// rule categories to the budget's debts and builds their states. With persist
// it also drops stale splits and stores the new ones.
func (s *Service) computeDebts(ctx context.Context, scope model.DebtScope, debts []*model.Debt, persist bool) ([]*debtState, error) {
	if len(debts) == 0 {
		return nil, nil
	}
	ids := make([]vo.Id, 0, len(debts))
	var categories []vo.Id
	for _, d := range debts {
		ids = append(ids, d.ID)
		for _, c := range ruleCategories(d.Rules) {
			if !slices.ContainsFunc(categories, c.Equal) {
				categories = append(categories, c)
			}
		}
	}
	snaps, err := s.repo.Snapshots(ctx, ids)
	if err != nil {
		return nil, err
	}
	rates, err := s.repo.RateChanges(ctx, ids)
	if err != nil {
		return nil, err
	}
	splits, err := s.repo.Splits(ctx, ids)
	if err != nil {
		return nil, err
	}
	cands, err := s.repo.CandidateTransactions(ctx, categories, scope.AccountIDs)
	if err != nil {
		return nil, err
	}

	views := matchViews(debts)
	ordered := make([]model.DebtView, 0, len(debts))
	byView := make(map[string]vo.Id, len(debts))
	for _, d := range debts {
		ordered = append(ordered, views[d.ID])
		byView[d.ID.String()] = d.ID
	}
	assigned := map[vo.Id][]CandidateTx{}
	matchedTo := map[vo.Id]vo.Id{}
	for _, c := range cands {
		labels := make([]string, 0, len(c.LabelIDs))
		for _, l := range c.LabelIDs {
			labels = append(labels, l.String())
		}
		cat := c.CategoryID.String()
		if v, ok := model.AssignDebt(ordered, &cat, idString(c.PayeeID), idString(c.TagID), labels, c.SpentAt); ok {
			id := byView[v.ID]
			assigned[id] = append(assigned[id], c)
			matchedTo[c.ID] = id
		}
	}

	// A split of a payment dated after today is provisional: the terms may
	// still be replaced before its date, so it is dropped and recomputed on
	// every sync until the day arrives.
	today := s.today(ctx)
	future := map[vo.Id]bool{}
	for _, c := range cands {
		if day := time.Date(c.SpentAt.Year(), c.SpentAt.Month(), c.SpentAt.Day(), 0, 0, 0, 0, time.UTC); day.After(today) {
			future[c.ID] = true
		}
	}
	// stored splits are keyed by transaction per debt: within one budget a
	// payment counts on one debt, so a split is stored when it is the one the
	// payment is assigned to
	stored := map[vo.Id]vo.DecimalNumber{}
	stale := map[vo.Id][]vo.Id{}
	for _, sp := range splits {
		if to, ok := matchedTo[sp.TransactionID]; ok && to == sp.DebtID && !future[sp.TransactionID] {
			stored[sp.TransactionID] = sp.Interest
			continue
		}
		stale[sp.DebtID] = append(stale[sp.DebtID], sp.TransactionID)
	}
	for _, d := range debts {
		if !persist {
			break
		}
		if err := s.repo.DeleteSplits(ctx, d.ID, stale[d.ID]); err != nil {
			return nil, err
		}
	}

	snapsByDebt := map[vo.Id][]*model.DebtSnapshot{}
	for _, sn := range snaps {
		snapsByDebt[sn.DebtID] = append(snapsByDebt[sn.DebtID], sn)
	}
	ratesByDebt := map[vo.Id][]*model.DebtRateChange{}
	for _, c := range rates {
		ratesByDebt[c.DebtID] = append(ratesByDebt[c.DebtID], c)
	}
	digitsByCurrency := map[vo.Id]int{}
	states := make([]*debtState, 0, len(debts))
	for _, d := range debts {
		digits, ok := digitsByCurrency[d.CurrencyID]
		if !ok {
			if digits, err = s.rates.FractionDigits(ctx, d.CurrencyID); err != nil {
				return nil, err
			}
			digitsByCurrency[d.CurrencyID] = digits
		}
		st := &debtState{debt: d, view: views[d.ID], snaps: snapsByDebt[d.ID], rates: ratesByDebt[d.ID], digits: digits}
		if err := s.fillPayments(ctx, st, assigned[d.ID], stored, persist); err != nil {
			return nil, err
		}
		states = append(states, st)
	}
	return states, nil
}

// fillPayments converts the debt's matched expenses and splits those without
// a stored split, storing it when persist is set. Stored interests are fed to
// the split as they are, so payments split earlier never change.
func (s *Service) fillPayments(ctx context.Context, st *debtState, cands []CandidateTx, stored map[vo.Id]vo.DecimalNumber, persist bool) error {
	d := st.debt
	sort.SliceStable(cands, func(i, j int) bool {
		if !cands[i].SpentAt.Equal(cands[j].SpentAt) {
			return cands[i].SpentAt.Before(cands[j].SpentAt)
		}
		return cands[i].ID.String() < cands[j].ID.String()
	})
	pays := make([]payment, 0, len(cands))
	var missing bool
	for _, c := range cands {
		day := time.Date(c.SpentAt.Year(), c.SpentAt.Month(), c.SpentAt.Day(), 0, 0, 0, 0, time.UTC)
		amount := c.Amount
		if !c.CurrencyID.Equal(d.CurrencyID) {
			converted, err := s.rates.ConvertOnDay(ctx, c.CurrencyID, d.CurrencyID, c.Amount, day)
			if err != nil {
				return err
			}
			amount = converted
		}
		p := payment{txID: c.ID, date: day, amount: roundTo(toFloat(amount), st.digits)}
		if interest, ok := stored[c.ID]; ok {
			p.interest = math.Max(0, math.Min(toFloat(interest), p.amount))
		} else {
			missing = true
		}
		pays = append(pays, p)
	}
	st.pays = pays
	if !missing {
		return nil
	}
	// A debt not set up has no rate yet: its payments are stored with a
	// provisional zero interest, replaced when setup is finished.
	if len(st.snaps) == 0 {
		return s.storeZeroSplits(ctx, st, stored, persist)
	}

	mathPays := make([]model.MathPayment, 0, len(pays))
	for _, p := range pays {
		mp := model.MathPayment{ID: p.txID.String(), Date: p.date, Amount: p.amount}
		if _, ok := stored[p.txID]; ok {
			interest := p.interest
			mp.Interest = &interest
		}
		mathPays = append(mathPays, mp)
	}
	split := model.SplitPayments(st.mathSnaps(), mathPays)
	for i := range st.pays {
		p := &st.pays[i]
		if _, ok := stored[p.txID]; ok {
			continue
		}
		p.interest = roundTo(split[p.txID.String()], st.digits)
		if !persist {
			continue
		}
		err := s.repo.InsertSplit(ctx, model.DebtSplit{DebtID: d.ID, TransactionID: p.txID,
			Interest: vo.NewDecimal(strconv.FormatFloat(p.interest, 'f', st.digits, 64))})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) storeZeroSplits(ctx context.Context, st *debtState, stored map[vo.Id]vo.DecimalNumber, persist bool) error {
	if !persist {
		return nil
	}
	for _, p := range st.pays {
		if _, ok := stored[p.txID]; ok {
			continue
		}
		if err := s.repo.InsertSplit(ctx, model.DebtSplit{DebtID: st.debt.ID, TransactionID: p.txID, Interest: vo.NewDecimal("0")}); err != nil {
			return err
		}
	}
	return nil
}
