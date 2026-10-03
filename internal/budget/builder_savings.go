package budget

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// savingsRow is one savings account's in-progress row, shared by the monthly
// and plan builders.
type savingsRow struct {
	account    model.AccountView
	currencyID vo.Id // element currency
	sortKey    sortkey.Key
}

// savingsRows resolves each savings member's element currency (the element
// row's, else the account's own) and sort key. Rows derive from the CURRENT
// savings accounts, not from the element table, because sync is lazy: a stale
// element row whose account is no longer savings must not render.
func savingsRows(f filters, options map[string]elementOption) ([]savingsRow, error) {
	out := make([]savingsRow, 0, len(f.savingsAccounts))
	for _, a := range f.savingsAccounts {
		opt := options[elementKey(a.ID, model.ElementSavings)]
		cur := opt.currencyID
		if cur == nil {
			cid, err := vo.ParseId(a.CurrencyID)
			if err != nil {
				return nil, err
			}
			cur = &cid
		}
		out = append(out, savingsRow{account: a, currencyID: *cur, sortKey: opt.sortKey})
	}
	// Keyless rows (not synced yet) trail in membership order — the stable sort
	// keeps it — because the first sync keys them in that same order, so they
	// don't move once it runs.
	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := out[i].sortKey, out[j].sortKey
		if ki == "" || kj == "" {
			return ki != "" && kj == ""
		}
		if ki != kj {
			return ki < kj
		}
		return out[i].account.ID < out[j].account.ID
	})
	return out, nil
}

func savingsSpentKey(accountID string) string { return "savings-spent_" + accountID }

func savingsAccountIDs(f filters) ([]vo.Id, error) {
	out := make([]vo.Id, 0, len(f.savingsAccounts))
	for _, a := range f.savingsAccounts {
		id, err := vo.ParseId(a.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// savingsConvertKey resolves one AccountsNetByMonth row to its toConvert bucket
// key and the [start,end) rate period that row's amount was earned in — the
// monthly builder always returns the same key/period (one period, the whole
// call), the plan builder returns a distinct key/period per window month, and
// signals "not this window" with ok=false (a month outside monthIdx).
type savingsConvertKey func(row model.SavingsMonthRow) (key string, start, end time.Time, ok bool)

// addSavings queues each savings row's actual amount(s) into the caller's
// single bulk conversion, account currency -> element currency, over
// [from,to). keyFor is what differs between the monthly and plan builders
// (see savingsConvertKey); everything else — resolving the rows, loading
// AccountsNetByMonth once, and building the ConvertItem — is shared.
func (s *Service) addSavings(ctx context.Context, f filters, options map[string]elementOption, from, to time.Time,
	keyFor savingsConvertKey, toConvert map[string][]model.ConvertItem) ([]savingsRow, map[string]bool, error) {
	hasActual := map[string]bool{}
	rows, err := savingsRows(f, options)
	if err != nil || len(rows) == 0 {
		return rows, hasActual, err
	}
	ids, err := savingsAccountIDs(f)
	if err != nil {
		return nil, nil, err
	}
	actual, err := s.read.AccountsNetByMonth(ctx, ids, from, to)
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]savingsRow{}
	for _, r := range rows {
		byID[r.account.ID] = r
	}
	for _, a := range actual {
		r, ok := byID[a.AccountID]
		if !ok {
			continue
		}
		key, start, end, ok := keyFor(a)
		if !ok {
			continue
		}
		accountCur, perr := vo.ParseId(r.account.CurrencyID)
		if perr != nil {
			return nil, nil, perr
		}
		toConvert[key] = append(toConvert[key], model.ConvertItem{
			PeriodStart: start, PeriodEnd: end, From: accountCur, To: r.currencyID, Amount: vo.NewDecimal(a.Amount),
		})
		hasActual[a.AccountID] = true
	}
	return rows, hasActual, nil
}

// monthlySavings is the monthly builder's savings state between queueing the
// bulk conversion and emitting the rows: pending holds, per account, the months
// whose unmet plan still adds to the closing balance.
type monthlySavings struct {
	rows    []savingsRow
	pending map[string][]pendingSavingsMonth
}

// pendingSavingsMonth is one month from the caller's current month through the
// selected one: its plan (element currency) and the key of its converted actual.
type pendingSavingsMonth struct {
	planned   vo.DecimalNumber
	actualKey string
}

// addMonthlySavings queues each savings row's actual for the period, its
// closing balance and — from the caller's current month on — every month's
// actual since then into the structure's single bulk conversion, account
// currency -> element currency.
func (s *Service) addMonthlySavings(ctx context.Context, b *budgetAggregate, f filters, options map[string]elementOption, toConvert map[string][]model.ConvertItem) (monthlySavings, error) {
	rows, _, err := s.addSavings(ctx, f, options, f.periodStart, f.periodEnd,
		func(a model.SavingsMonthRow) (string, time.Time, time.Time, bool) {
			return savingsSpentKey(a.AccountID), f.periodStart, f.periodEnd, true
		}, toConvert)
	out := monthlySavings{rows: rows, pending: map[string][]pendingSavingsMonth{}}
	if err != nil || len(rows) == 0 {
		return out, err
	}
	ids, err := savingsAccountIDs(f)
	if err != nil {
		return out, err
	}
	balances, err := s.read.AccountsBalancesBeforeDate(ctx, ids, f.periodEnd)
	if err != nil {
		return out, err
	}
	closing := map[string]vo.DecimalNumber{}
	for _, bal := range balances {
		closing[bal.AccountID] = vo.NewDecimal(bal.Balance)
	}
	for _, r := range rows {
		if err := queueSavingsBalance(toConvert, savingsClosingKey(r.account.ID), f.periodStart, f.periodEnd, r, closing[r.account.ID]); err != nil {
			return out, err
		}
	}
	return out, s.addSavingsPending(ctx, b, f, ids, &out, toConvert)
}

// addSavingsPending fills out.pending when the selected month is the caller's
// current month or a later one: the closing balance is then a projection, the
// booked balance plus each month's plan not yet met by its actual, from the
// current month through the selected one. A past month's closing balance is
// what is booked.
func (s *Service) addSavingsPending(ctx context.Context, b *budgetAggregate, f filters, ids []vo.Id, out *monthlySavings, toConvert map[string][]model.ConvertItem) error {
	cur := localMonth(s.clock.Now(), reqctx.Location(ctx))
	if f.periodStart.Before(cur) {
		return nil
	}
	limitRows, err := s.read.LimitsByMonth(ctx, b.budget.ID, cur, f.periodEnd)
	if err != nil {
		return err
	}
	planned := map[string]vo.DecimalNumber{}
	for _, l := range limitRows {
		if model.ElementType(l.Type) == model.ElementSavings {
			planned[l.ExternalID+"_"+l.Month] = vo.NewDecimal(l.Amount)
		}
	}
	actual, err := s.read.AccountsNetByMonth(ctx, ids, cur, f.periodEnd)
	if err != nil {
		return err
	}
	actualByKey := map[string]string{}
	for _, a := range actual {
		actualByKey[a.AccountID+"_"+a.Month] = a.Amount
	}
	for _, r := range out.rows {
		if r.account.IsDeleted {
			continue
		}
		accountCur, err := vo.ParseId(r.account.CurrencyID)
		if err != nil {
			return err
		}
		for m := cur; m.Before(f.periodEnd); m = m.AddDate(0, 1, 0) {
			month := m.Format(datetime.DateLayout)
			p, ok := planned[r.account.ID+"_"+month]
			if !ok {
				continue
			}
			key := "savings-pending_" + r.account.ID + "_" + month
			if amount, ok := actualByKey[r.account.ID+"_"+month]; ok {
				toConvert[key] = append(toConvert[key], model.ConvertItem{
					PeriodStart: m, PeriodEnd: m.AddDate(0, 1, 0), From: accountCur, To: r.currencyID, Amount: vo.NewDecimal(amount),
				})
			}
			out.pending[r.account.ID] = append(out.pending[r.account.ID], pendingSavingsMonth{planned: p, actualKey: key})
		}
	}
	return nil
}

func savingsClosingKey(accountID string) string { return "savings-closing_" + accountID }

// queueSavingsBalance converts a balance (account currency) to the row's
// element currency at the rate of its month. A zero or absent balance queues
// nothing: the getter reads a missing key as zero.
func queueSavingsBalance(toConvert map[string][]model.ConvertItem, key string, start, end time.Time, r savingsRow, amount vo.DecimalNumber) error {
	if amount.IsZero() {
		return nil
	}
	accountCur, err := vo.ParseId(r.account.CurrencyID)
	if err != nil {
		return err
	}
	toConvert[key] = append(toConvert[key], model.ConvertItem{PeriodStart: start, PeriodEnd: end, From: accountCur, To: r.currencyID, Amount: amount})
	return nil
}

func emitMonthlySavings(ms monthlySavings, limits map[string]budgetedAmount, get func(string) vo.DecimalNumber) []model.SavingsElementResult {
	zero := vo.NewDecimal("0")
	out := []model.SavingsElementResult{}
	for _, r := range ms.rows {
		budgeted := orZero(limits[elementKey(r.account.ID, model.ElementSavings)].budgeted, zero)
		spent := get(savingsSpentKey(r.account.ID))
		// A deleted account stays only while it still carries a plan or activity.
		if r.account.IsDeleted && budgeted.IsZero() && spent.IsZero() {
			continue
		}
		closing := get(savingsClosingKey(r.account.ID))
		// the gap is per month, so saving more than planned in one month does not
		// cover another month's shortfall
		for _, p := range ms.pending[r.account.ID] {
			if gap := p.planned.Sub(get(p.actualKey)); gap.IsGreaterThan(zero) {
				closing = closing.Add(gap)
			}
		}
		out = append(out, model.SavingsElementResult{
			Id: r.account.ID, Type: int(model.ElementSavings.Int16()), Name: r.account.Name, Icon: r.account.Icon,
			CurrencyId: r.currencyID.String(), OwnerUserId: r.account.OwnerID, IsArchived: boolToInt(r.account.IsDeleted),
			Position: len(out), Budgeted: budgeted.String(), Spent: spent.String(), Available: budgeted.Sub(spent).String(),
			ClosingBalance: closing.String(),
		})
	}
	return out
}

// addPlanSavings queues each savings row's per-month actual into the plan's
// single bulk conversion, account currency -> element currency, under the
// same planKey scheme as the elements. hasActual marks accounts with any
// activity in the window.
func (s *Service) addPlanSavings(ctx context.Context, f filters, options map[string]elementOption, monthsList []time.Time, monthIdx map[string]int, toConvert map[string][]model.ConvertItem) ([]savingsRow, map[string]bool, error) {
	windowEnd := monthsList[0].AddDate(0, len(monthsList), 0)
	rows, hasActual, err := s.addSavings(ctx, f, options, monthsList[0], windowEnd,
		func(a model.SavingsMonthRow) (string, time.Time, time.Time, bool) {
			i, ok := monthIdx[a.Month]
			if !ok {
				return "", time.Time{}, time.Time{}, false
			}
			return planKey(i, elementKey(a.AccountID, model.ElementSavings)), monthsList[i], monthsList[i].AddDate(0, 1, 0), true
		}, toConvert)
	if err != nil || len(rows) == 0 {
		return rows, hasActual, err
	}
	if err := s.addPlanSavingsClosings(ctx, f, rows, monthsList, monthIdx, toConvert); err != nil {
		return nil, nil, err
	}
	return rows, hasActual, nil
}

// addPlanSavingsClosings queues each savings row's booked balance at the end of
// every window month: the balance before the window plus the net change of the
// months through it. The client adds
// the unmet plans of the current and later months on top.
func (s *Service) addPlanSavingsClosings(ctx context.Context, f filters, rows []savingsRow, monthsList []time.Time, monthIdx map[string]int, toConvert map[string][]model.ConvertItem) error {
	ids, err := savingsAccountIDs(f)
	if err != nil {
		return err
	}
	windowEnd := monthsList[0].AddDate(0, len(monthsList), 0)
	balances, err := s.read.AccountsBalancesBeforeDate(ctx, ids, monthsList[0])
	if err != nil {
		return err
	}
	nets, err := s.read.AccountsNetByMonth(ctx, ids, monthsList[0], windowEnd)
	if err != nil {
		return err
	}
	opening := map[string]vo.DecimalNumber{}
	for _, b := range balances {
		opening[b.AccountID] = vo.NewDecimal(b.Balance)
	}
	netByMonth := map[string][]vo.DecimalNumber{}
	for _, n := range nets {
		i, ok := monthIdx[n.Month]
		if !ok {
			continue
		}
		if netByMonth[n.AccountID] == nil {
			netByMonth[n.AccountID] = make([]vo.DecimalNumber, len(monthsList))
		}
		netByMonth[n.AccountID][i] = vo.NewDecimal(n.Amount)
	}
	for _, r := range rows {
		running, ok := opening[r.account.ID]
		if !ok {
			running = vo.NewDecimal("0")
		}
		index := savingsClosingKey(r.account.ID)
		for i, m := range monthsList {
			if net := netByMonth[r.account.ID]; net != nil && !net[i].IsZero() {
				running = running.Add(net[i])
			}
			if err := queueSavingsBalance(toConvert, planKey(i, index), m, m.AddDate(0, 1, 0), r, running); err != nil {
				return err
			}
		}
	}
	return nil
}

// emitPlanSavings renders the plan's savings rows in savingsRows order. A
// deleted account stays only while it carries a plan or activity in the window.
func emitPlanSavings(rows []savingsRow, plannedFor func(string) []string, hasActual map[string]bool, get func(string) vo.DecimalNumber, nMonths int) []model.PlanSavingsElementResult {
	out := []model.PlanSavingsElementResult{}
	for _, r := range rows {
		index := elementKey(r.account.ID, model.ElementSavings)
		planned := plannedFor(index)
		hasPlan := false
		for _, p := range planned {
			if p != "" {
				hasPlan = true
			}
		}
		if r.account.IsDeleted && !hasPlan && !hasActual[r.account.ID] {
			continue
		}
		cells := make([]model.PlanSavingsCellResult, nMonths)
		for i := range cells {
			cells[i] = model.PlanSavingsCellResult{
				Actual: get(planKey(i, index)).String(), Planned: planned[i],
				ClosingBalance: get(planKey(i, savingsClosingKey(r.account.ID))).String(),
			}
		}
		out = append(out, model.PlanSavingsElementResult{
			Id: r.account.ID, Type: int(model.ElementSavings.Int16()), Name: r.account.Name, Icon: r.account.Icon,
			CurrencyId: r.currencyID.String(), OwnerUserId: r.account.OwnerID, IsArchived: boolToInt(r.account.IsDeleted),
			Position: len(out), Cells: cells,
		})
	}
	return out
}

// buildSavingsOpeningBalances is buildOpeningBalances over the savings
// accounts only (strictly before the window), per savings-account currency,
// budget currency first then discovery order.
func (s *Service) buildSavingsOpeningBalances(ctx context.Context, budgetCurrencyID vo.Id, f filters, from time.Time) ([]model.OpeningBalanceResult, error) {
	out := []model.OpeningBalanceResult{}
	ids, err := savingsAccountIDs(f)
	if err != nil || len(ids) == 0 {
		return out, err
	}
	rows, err := s.read.AccountsBalancesBeforeDate(ctx, ids, from)
	if err != nil {
		return nil, err
	}
	for _, cid := range savingsCurrencies(f, budgetCurrencyID) {
		out = append(out, model.OpeningBalanceResult{CurrencyId: cid, Amount: sumBalances(rows, cid).String()})
	}
	return out, nil
}

// savingsCurrencies lists the savings accounts' currencies once each: the
// budget currency first (only when a savings account holds it), then
// discovery order.
func savingsCurrencies(f filters, budgetCurrencyID vo.Id) []string {
	budgetCur := budgetCurrencyID.String()
	seen := map[string]bool{}
	var rest []string
	hasBudgetCur := false
	for _, a := range f.savingsAccounts {
		if seen[a.CurrencyID] {
			continue
		}
		seen[a.CurrencyID] = true
		if a.CurrencyID == budgetCur {
			hasBudgetCur = true
			continue
		}
		rest = append(rest, a.CurrencyID)
	}
	if hasBudgetCur {
		return append([]string{budgetCur}, rest...)
	}
	return rest
}

// buildSavingsFlows sums byMonth's per-account rows over the savings accounts
// per (month, account currency), ordered by month, then budget currency first,
// then currency id. Months without activity have no entry.
func (s *Service) buildSavingsFlows(ctx context.Context, budgetCurrencyID vo.Id, f filters, from, to time.Time,
	byMonth func(context.Context, []vo.Id, time.Time, time.Time) ([]model.SavingsMonthRow, error)) ([]model.PlanSavingsFlowResult, error) {
	out := []model.PlanSavingsFlowResult{}
	ids, err := savingsAccountIDs(f)
	if err != nil || len(ids) == 0 {
		return out, err
	}
	rows, err := byMonth(ctx, ids, from, to)
	if err != nil {
		return nil, err
	}
	currencyOf := map[string]string{}
	for _, a := range f.savingsAccounts {
		currencyOf[a.ID] = a.CurrencyID
	}
	sums := map[[2]string]vo.DecimalNumber{}
	for _, r := range rows {
		k := [2]string{r.Month, currencyOf[r.AccountID]}
		acc, ok := sums[k]
		if !ok {
			acc = vo.NewDecimal("0")
		}
		sums[k] = acc.Add(vo.NewDecimal(r.Amount))
	}
	budgetCur := budgetCurrencyID.String()
	for k, v := range sums {
		out = append(out, model.PlanSavingsFlowResult{Month: k[0], CurrencyId: k[1], Amount: v.String()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Month != out[j].Month {
			return out[i].Month < out[j].Month
		}
		if (out[i].CurrencyId == budgetCur) != (out[j].CurrencyId == budgetCur) {
			return out[i].CurrencyId == budgetCur
		}
		return out[i].CurrencyId < out[j].CurrencyId
	})
	return out, nil
}
