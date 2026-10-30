// The projected read model: the budget ReadModel answered from the projected
// occurrences of recurring templates instead of from stored transactions, so
// the existing builders can run a second time over "upcoming" money and file
// it exactly as they file real transactions. Every override mirrors the rules
// its SQL counterpart in internal/budget/repo/read.go documents; months are
// bucketed here in Go on FiledAt, so both engines agree by construction.
package budget

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// projectedTx is one occurrence flattened like a transaction row.
type projectedTx struct {
	templateID  string
	schedule    string
	userID      string
	typ         model.TransactionType
	accountID   string
	currencyID  string
	amount      vo.DecimalNumber
	categoryID  *string
	payeeID     *string
	tagID       *string
	labelIDs    []string
	description string
	// recipientID is "" unless the occurrence is a transfer; amountRecipient
	// is then amount converted into the recipient account's currency.
	recipientID         string
	recipientCurrencyID string
	amountRecipient     vo.DecimalNumber
	filedAt, dueAt      time.Time
	isNext, isOverdue   bool
}

// projectedRead serves the reports from occ; the embedded real ReadModel
// still answers the calls that are not about money moving (labels, limits,
// membership activity).
type projectedRead struct {
	ReadModel
	occ        []projectedTx
	currencyOf map[string]string
	nameOf     map[string]string
	byKey      map[occurrenceKey]*projectedTx
}

type occurrenceKey struct{ templateID, dueAt string }

var _ ReadModel = (*projectedRead)(nil)

// newProjectedRead projects every template touching the budget's member
// accounts up to until (exclusive, on FiledAt). It returns a nil read when
// nothing is projected, so callers skip the second builder pass and keep the
// zeros the real build set.
func (s *Service) newProjectedRead(ctx context.Context, f filters, until time.Time) (*projectedRead, error) {
	if s.recurring == nil || len(f.includedAccountIDs) == 0 {
		return nil, nil
	}
	templates, err := s.recurring.ListByAccountIDs(ctx, f.includedAccountIDs)
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, nil
	}
	p := &projectedRead{ReadModel: s.read, currencyOf: map[string]string{}, nameOf: map[string]string{}}

	var accountIDs []vo.Id
	add := func(id vo.Id) {
		if _, ok := p.currencyOf[id.String()]; !ok {
			p.currencyOf[id.String()] = ""
			accountIDs = append(accountIDs, id)
		}
	}
	for _, rt := range templates {
		add(rt.AccountID)
		if rt.Type.IsTransfer() && rt.AccountRecipID != nil {
			add(*rt.AccountRecipID)
		}
	}
	views, err := s.accounts.AccountsByIDs(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	deleted := map[string]bool{}
	for _, v := range views {
		p.currencyOf[v.ID] = v.CurrencyID
		p.nameOf[v.ID] = v.Name
		deleted[v.ID] = v.IsDeleted
	}

	today := localToday(s.clock.Now(), reqctx.Location(ctx))
	toConvert := map[string][]model.ConvertItem{}
	for _, rt := range templates {
		// A template on a deleted account can be neither seen, posted nor
		// removed, so projecting it would be upcoming money nothing can clear.
		if deleted[rt.AccountID.String()] || (rt.Type.IsTransfer() && rt.AccountRecipID != nil && deleted[rt.AccountRecipID.String()]) {
			continue
		}
		amount := vo.NewDecimal(rt.Amount)
		labels := make([]string, 0, len(rt.LabelIDs))
		for _, l := range rt.LabelIDs {
			labels = append(labels, l.String())
		}
		for _, o := range model.ProjectOccurrences(rt, today, until) {
			tx := projectedTx{
				templateID: rt.ID.String(), schedule: string(rt.Schedule), userID: rt.UserID.String(), typ: rt.Type,
				accountID: rt.AccountID.String(), currencyID: p.currencyOf[rt.AccountID.String()],
				amount: amount, categoryID: optString(rt.CategoryID), payeeID: optString(rt.PayeeID), tagID: optString(rt.TagID),
				labelIDs: labels, description: rt.Description,
				filedAt: o.FiledAt, dueAt: o.DueAt, isNext: o.IsNext, isOverdue: o.IsOverdue,
			}
			if rt.Type.IsTransfer() && rt.AccountRecipID != nil {
				tx.recipientID = rt.AccountRecipID.String()
				tx.recipientCurrencyID = p.currencyOf[tx.recipientID]
				tx.amountRecipient = amount
				if tx.recipientCurrencyID != tx.currencyID {
					from, perr := vo.ParseId(tx.currencyID)
					if perr != nil {
						return nil, perr
					}
					to, perr := vo.ParseId(tx.recipientCurrencyID)
					if perr != nil {
						return nil, perr
					}
					start, end := model.PeriodWindow(model.PeriodOf(o.FiledAt, f.startDay), f.startDay)
					toConvert[strconv.Itoa(len(p.occ))] = []model.ConvertItem{{
						PeriodStart: start, PeriodEnd: end, From: from, To: to, Amount: amount,
					}}
				}
			}
			p.occ = append(p.occ, tx)
		}
	}
	if len(toConvert) > 0 {
		start, end := model.PeriodWindow(model.PeriodOf(today, f.startDay), f.startDay)
		converted, cerr := s.convertor.BulkConvert(ctx, start, end, toConvert)
		if cerr != nil {
			return nil, cerr
		}
		for k, v := range converted {
			i, aerr := strconv.Atoi(k)
			if aerr != nil {
				return nil, aerr
			}
			p.occ[i].amountRecipient = v
		}
	}
	if len(p.occ) == 0 {
		return nil, nil
	}
	// Newest first, the order every BudgetTransactions* list returns.
	sort.SliceStable(p.occ, func(i, j int) bool {
		if !p.occ[i].dueAt.Equal(p.occ[j].dueAt) {
			return p.occ[i].dueAt.After(p.occ[j].dueAt)
		}
		return p.occ[i].templateID < p.occ[j].templateID
	})
	return p, nil
}

// occurrence finds the occurrence a list row was built from: rows carry the
// template id and the due date in SpentAt.
func (p *projectedRead) occurrence(templateID, spentAt string) (*projectedTx, bool) {
	if p.byKey == nil {
		p.byKey = make(map[occurrenceKey]*projectedTx, len(p.occ))
		for i := range p.occ {
			o := &p.occ[i]
			p.byKey[occurrenceKey{o.templateID, o.dueAt.Format(datetime.Layout)}] = o
		}
	}
	o, ok := p.byKey[occurrenceKey{templateID, spentAt}]
	return o, ok
}

// localToday is the caller's local midnight as a UTC-tagged wall clock, the
// form NextPaymentAt is stored in.
func localToday(now time.Time, loc *time.Location) time.Time {
	y, m, d := localNow(now, loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// localNow is the caller's local wall clock as a UTC-tagged time, comparable
// with period bounds and spent_at.
func localNow(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	w := now.In(loc)
	return time.Date(w.Year(), w.Month(), w.Day(), w.Hour(), w.Minute(), w.Second(), w.Nanosecond(), time.UTC)
}

// projectedMonth renders the "YYYY-MM-01" period key the by-month SQL groups on.
func projectedMonth(t time.Time, startDay int) string {
	return model.PeriodOf(t, startDay).Format(datetime.DateLayout)
}

func optString(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func idSet(ids []vo.Id) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id.String()] = true
	}
	return out
}

func (o *projectedTx) filedIn(start, end time.Time) bool {
	return !o.filedAt.Before(start) && o.filedAt.Before(end)
}

func (o *projectedTx) hasLabel(id string) bool {
	for _, l := range o.labelIDs {
		if l == id {
			return true
		}
	}
	return false
}

// signedOnSource is the occurrence's effect on its source account's balance.
func (o *projectedTx) signedOnSource() vo.DecimalNumber {
	if o.typ == model.TransactionTypeIncome {
		return o.amount
	}
	return vo.NewDecimal("0").Sub(o.amount)
}

func (o *projectedTx) row(amount vo.DecimalNumber, currencyID string) model.BudgetTransactionRow {
	return model.BudgetTransactionRow{
		ID: o.templateID, UserID: o.userID, CurrencyID: currencyID, Amount: amount.String(),
		Description: o.description, SpentAt: o.dueAt.Format(datetime.Layout),
		CategoryID: o.categoryID, PayeeID: o.payeeID, TagID: o.tagID,
	}
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ref is deref's inverse: ids are never empty, so "" stands for NULL.
func ref(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// sums accumulates decimal totals in first-seen key order.
type sums[K comparable] struct {
	keys []K
	vals map[K]vo.DecimalNumber
}

func newSums[K comparable]() *sums[K] { return &sums[K]{vals: map[K]vo.DecimalNumber{}} }

func (s *sums[K]) add(k K, v vo.DecimalNumber) {
	cur, ok := s.vals[k]
	if !ok {
		s.keys = append(s.keys, k)
		cur = vo.NewDecimal("0")
	}
	s.vals[k] = cur.Add(v)
}

// AccountsBalancesBeforeDate returns the projected balance change of every
// requested account a template touches, with balanceSQL's sign rules.
func (p *projectedRead) AccountsBalancesBeforeDate(_ context.Context, accountIDs []vo.Id, date time.Time) ([]model.AccountBalanceRow, error) {
	acc := idSet(accountIDs)
	totals := newSums[string]()
	for i := range p.occ {
		o := &p.occ[i]
		if !o.filedAt.Before(date) {
			continue
		}
		if acc[o.accountID] {
			totals.add(o.accountID, o.signedOnSource())
		}
		if o.recipientID != "" && acc[o.recipientID] {
			totals.add(o.recipientID, o.amountRecipient)
		}
	}
	var out []model.AccountBalanceRow
	for _, id := range accountIDs {
		cur, known := p.currencyOf[id.String()]
		if !known {
			continue
		}
		bal, ok := totals.vals[id.String()]
		if !ok {
			bal = vo.NewDecimal("0")
		}
		out = append(out, model.AccountBalanceRow{AccountID: id.String(), CurrencyID: cur, Balance: bal.String()})
	}
	return out, nil
}

func (p *projectedRead) AccountsReport(context.Context, []vo.Id, time.Time, time.Time) ([]model.AccountReportRow, error) {
	return nil, nil
}

func (p *projectedRead) HoldingsReport(context.Context, []vo.Id, time.Time, time.Time) ([]model.HoldingsRow, error) {
	return nil, nil
}

func (p *projectedRead) SummarizedLimits(context.Context, vo.Id, time.Time, time.Time) ([]model.SummarizedLimitRow, error) {
	return nil, nil
}

type spendingKey struct{ month, category, tag, currency string }

// spending is the CountSpending/SpendingByMonth filter: expenses on the
// accounts, in the category set or with no category at all.
func (p *projectedRead) spending(categoryIDs, accountIDs []vo.Id, start, end time.Time, byMonth bool, startDay int) *sums[spendingKey] {
	acc, cats := idSet(accountIDs), idSet(categoryIDs)
	out := newSums[spendingKey]()
	for i := range p.occ {
		o := &p.occ[i]
		if o.typ != model.TransactionTypeExpense || !acc[o.accountID] || !o.filedIn(start, end) {
			continue
		}
		if o.categoryID != nil && !cats[*o.categoryID] {
			continue
		}
		k := spendingKey{category: deref(o.categoryID), tag: deref(o.tagID), currency: o.currencyID}
		if byMonth {
			k.month = projectedMonth(o.filedAt, startDay)
		}
		out.add(k, o.amount)
	}
	return out
}

func (p *projectedRead) CountSpending(_ context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]model.SpendingRow, error) {
	s := p.spending(categoryIDs, accountIDs, start, end, false, 0)
	var out []model.SpendingRow
	for _, k := range s.keys {
		out = append(out, model.SpendingRow{CategoryID: ref(k.category), TagID: ref(k.tag), CurrencyID: k.currency, Amount: s.vals[k].String()})
	}
	return out, nil
}

func (p *projectedRead) SpendingByMonth(_ context.Context, categoryIDs, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.MonthlySpendingRow, error) {
	s := p.spending(categoryIDs, accountIDs, from, to, true, startDay)
	var out []model.MonthlySpendingRow
	for _, k := range s.keys {
		out = append(out, model.MonthlySpendingRow{Month: k.month, CategoryID: ref(k.category), TagID: ref(k.tag), CurrencyID: k.currency, Amount: s.vals[k].String()})
	}
	return out, nil
}

// CountSpendingByLabel fans each expense out to every label it carries, at
// its full amount.
func (p *projectedRead) CountSpendingByLabel(_ context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.LabelSpendingRow, error) {
	type key struct{ label, category, currency string }
	acc := idSet(accountIDs)
	s := newSums[key]()
	for i := range p.occ {
		o := &p.occ[i]
		if o.typ != model.TransactionTypeExpense || !acc[o.accountID] || !o.filedIn(start, end) {
			continue
		}
		for _, l := range o.labelIDs {
			s.add(key{label: l, category: deref(o.categoryID), currency: o.currencyID}, o.amount)
		}
	}
	sort.SliceStable(s.keys, func(i, j int) bool {
		a, b := s.keys[i], s.keys[j]
		if a.label != b.label {
			return a.label < b.label
		}
		if a.category != b.category {
			return a.category < b.category
		}
		return a.currency < b.currency
	})
	var out []model.LabelSpendingRow
	for _, k := range s.keys {
		out = append(out, model.LabelSpendingRow{LabelID: k.label, CategoryID: ref(k.category), CurrencyID: k.currency, Amount: s.vals[k].String()})
	}
	return out, nil
}

// IncomeByMonth has no category filter, like its SQL.
func (p *projectedRead) IncomeByMonth(_ context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.MonthlyIncomeRow, error) {
	type key struct{ month, category, currency string }
	acc := idSet(accountIDs)
	s := newSums[key]()
	for i := range p.occ {
		o := &p.occ[i]
		if o.typ != model.TransactionTypeIncome || !acc[o.accountID] || !o.filedIn(from, to) {
			continue
		}
		s.add(key{month: projectedMonth(o.filedAt, startDay), category: deref(o.categoryID), currency: o.currencyID}, o.amount)
	}
	var out []model.MonthlyIncomeRow
	for _, k := range s.keys {
		out = append(out, model.MonthlyIncomeRow{Month: k.month, CategoryID: ref(k.category), CurrencyID: k.currency, Amount: s.vals[k].String()})
	}
	return out, nil
}

// crossing reports which side of a transfer is the included one: out when
// only the source is in the set, in when only the recipient is.
func (o *projectedTx) crossing(acc map[string]bool) (out, in bool) {
	if !o.typ.IsTransfer() || o.recipientID == "" {
		return false, false
	}
	src, rcp := acc[o.accountID], acc[o.recipientID]
	return src && !rcp, rcp && !src
}

func (p *projectedRead) TransfersByMonth(_ context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.MonthlyTransferRow, error) {
	type key struct{ month, currency string }
	acc := idSet(accountIDs)
	ins, outs := newSums[key](), newSums[key]()
	for i := range p.occ {
		o := &p.occ[i]
		if !o.filedIn(from, to) {
			continue
		}
		isOut, isIn := o.crossing(acc)
		month := projectedMonth(o.filedAt, startDay)
		if isOut {
			outs.add(key{month, o.currencyID}, o.amount)
		}
		if isIn {
			ins.add(key{month, o.recipientCurrencyID}, o.amountRecipient)
		}
	}
	merged := map[key]*model.MonthlyTransferRow{}
	row := func(k key) *model.MonthlyTransferRow {
		r := merged[k]
		if r == nil {
			r = &model.MonthlyTransferRow{Month: k.month, CurrencyID: k.currency, In: "0", Out: "0"}
			merged[k] = r
		}
		return r
	}
	for _, k := range outs.keys {
		row(k).Out = outs.vals[k].String()
	}
	for _, k := range ins.keys {
		row(k).In = ins.vals[k].String()
	}
	out := make([]model.MonthlyTransferRow, 0, len(merged))
	for _, r := range merged {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Month != out[j].Month {
			return out[i].Month < out[j].Month
		}
		return out[i].CurrencyID < out[j].CurrencyID
	})
	return out, nil
}

// accountMonths sums per (month, account), sorted like sortedSavingsRows.
func (p *projectedRead) accountMonths(accountIDs []vo.Id, from, to time.Time, startDay int, transfers bool) []model.SavingsMonthRow {
	type key struct{ month, account string }
	acc := idSet(accountIDs)
	s := newSums[key]()
	for i := range p.occ {
		o := &p.occ[i]
		if !o.filedIn(from, to) || (!transfers && o.typ.IsTransfer()) {
			continue
		}
		month := projectedMonth(o.filedAt, startDay)
		if acc[o.accountID] {
			s.add(key{month, o.accountID}, o.signedOnSource())
		}
		if transfers && o.recipientID != "" && acc[o.recipientID] {
			s.add(key{month, o.recipientID}, o.amountRecipient)
		}
	}
	out := make([]model.SavingsMonthRow, 0, len(s.keys))
	for _, k := range s.keys {
		out = append(out, model.SavingsMonthRow{AccountID: k.account, Month: k.month, Amount: s.vals[k].String()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Month != out[j].Month {
			return out[i].Month < out[j].Month
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}

func (p *projectedRead) AccountsNetByMonth(_ context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.SavingsMonthRow, error) {
	return p.accountMonths(accountIDs, from, to, startDay, true), nil
}

func (p *projectedRead) AccountsIncomeExpenseByMonth(_ context.Context, accountIDs []vo.Id, from, to time.Time, startDay int) ([]model.SavingsMonthRow, error) {
	return p.accountMonths(accountIDs, from, to, startDay, false), nil
}

// expenses lists the expense occurrences on the accounts in [start, end)
// that keep returns true for, newest first.
func (p *projectedRead) expenses(accountIDs []vo.Id, start, end time.Time, keep func(o *projectedTx) bool) []model.BudgetTransactionRow {
	acc := idSet(accountIDs)
	var out []model.BudgetTransactionRow
	for i := range p.occ {
		o := &p.occ[i]
		if o.typ == model.TransactionTypeExpense && acc[o.accountID] && o.filedIn(start, end) && keep(o) {
			out = append(out, o.row(o.amount, o.currencyID))
		}
	}
	return out
}

func (p *projectedRead) BudgetTransactionsByCategories(_ context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	cats := idSet(categoryIDs)
	return p.expenses(accountIDs, start, end, func(o *projectedTx) bool {
		return o.tagID == nil && o.categoryID != nil && cats[*o.categoryID]
	}), nil
}

func (p *projectedRead) BudgetTransactionsByTag(_ context.Context, tagID vo.Id, categoryID *vo.Id, uncategorized bool, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	tag := tagID.String()
	return p.expenses(accountIDs, start, end, func(o *projectedTx) bool {
		if o.tagID == nil || *o.tagID != tag {
			return false
		}
		if categoryID != nil {
			return o.categoryID != nil && *o.categoryID == categoryID.String()
		}
		if uncategorized {
			return o.categoryID == nil
		}
		return true
	}), nil
}

func (p *projectedRead) BudgetTransactionsUncategorized(_ context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	return p.expenses(accountIDs, start, end, func(o *projectedTx) bool {
		return o.categoryID == nil && o.tagID == nil
	}), nil
}

func (p *projectedRead) BudgetTransactionsByLabel(_ context.Context, labelID vo.Id, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	label := labelID.String()
	return p.expenses(accountIDs, start, end, func(o *projectedTx) bool { return o.hasLabel(label) }), nil
}

func (p *projectedRead) BudgetTransactionsByLabelAndCategory(_ context.Context, labelID, categoryID vo.Id, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	label, cat := labelID.String(), categoryID.String()
	return p.expenses(accountIDs, start, end, func(o *projectedTx) bool {
		return o.hasLabel(label) && o.categoryID != nil && *o.categoryID == cat
	}), nil
}

func (p *projectedRead) BudgetTransactionsByLabelUncategorized(_ context.Context, labelID vo.Id, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	label := labelID.String()
	return p.expenses(accountIDs, start, end, func(o *projectedTx) bool {
		return o.hasLabel(label) && o.categoryID == nil
	}), nil
}

func (p *projectedRead) BudgetTransactionsTransfers(_ context.Context, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	acc := idSet(accountIDs)
	var out []model.BudgetTransactionRow
	for i := range p.occ {
		o := &p.occ[i]
		if !o.filedIn(start, end) {
			continue
		}
		isOut, isIn := o.crossing(acc)
		switch {
		case isOut:
			r := o.row(o.amount, o.currencyID)
			r.Direction = "out"
			out = append(out, r)
		case isIn:
			r := o.row(o.amountRecipient, o.recipientCurrencyID)
			r.Direction = "in"
			out = append(out, r)
		}
	}
	return out, nil
}

func (p *projectedRead) BudgetTransactionsIncome(_ context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	acc, cats := idSet(accountIDs), idSet(categoryIDs)
	var out []model.BudgetTransactionRow
	for i := range p.occ {
		o := &p.occ[i]
		if o.typ != model.TransactionTypeIncome || !acc[o.accountID] || !o.filedIn(start, end) || o.categoryID == nil || !cats[*o.categoryID] {
			continue
		}
		r := o.row(o.amount, o.currencyID)
		r.Direction, r.Type = "in", "income"
		out = append(out, r)
	}
	return out, nil
}

func (p *projectedRead) BudgetTransactionsIncomeUncategorized(_ context.Context, knownCategoryIDs, accountIDs []vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	acc, known := idSet(accountIDs), idSet(knownCategoryIDs)
	var out []model.BudgetTransactionRow
	for i := range p.occ {
		o := &p.occ[i]
		if o.typ != model.TransactionTypeIncome || !acc[o.accountID] || !o.filedIn(start, end) || (o.categoryID != nil && known[*o.categoryID]) {
			continue
		}
		r := o.row(o.amount, o.currencyID)
		r.Direction, r.Type = "in", "income"
		out = append(out, r)
	}
	return out, nil
}

func (p *projectedRead) BudgetTransactionsOnAccount(_ context.Context, accountID vo.Id, start, end time.Time) ([]model.BudgetTransactionRow, error) {
	id := accountID.String()
	var out []model.BudgetTransactionRow
	for i := range p.occ {
		o := &p.occ[i]
		if !o.filedIn(start, end) {
			continue
		}
		switch {
		case o.accountID == id:
			r := o.row(o.amount, o.currencyID)
			r.Direction, r.Type = "out", o.typ.Alias()
			if o.typ == model.TransactionTypeIncome {
				r.Direction = "in"
			}
			out = append(out, r)
		case o.typ.IsTransfer() && o.recipientID == id:
			r := o.row(o.amountRecipient, o.recipientCurrencyID)
			r.Direction, r.Type = "in", "transfer"
			out = append(out, r)
		}
	}
	return out, nil
}
