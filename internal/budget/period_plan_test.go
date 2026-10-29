package budget_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (h *periodHarness) plan(from string, months int) model.BudgetPlanResult {
	h.t.Helper()
	res, err := h.svc.GetBudgetPlan(context.Background(), h.owner, model.GetBudgetPlanRequest{
		Id: h.budgetID, From: from, Months: fmt.Sprint(months),
	})
	if err != nil {
		h.t.Fatalf("get-budget-plan %s/%d: %v", from, months, err)
	}
	return res.Item
}

func planRow(p model.BudgetPlanResult, id string, typ model.ElementType) (model.PlanElementResult, bool) {
	for _, e := range p.Structure.Elements {
		if e.Id == id && e.Type == int(typ) {
			return e, true
		}
	}
	return model.PlanElementResult{}, false
}

func (h *periodHarness) planCategory(p model.BudgetPlanResult) model.PlanElementResult {
	h.t.Helper()
	row, ok := planRow(p, h.category, model.ElementCategory)
	if !ok {
		h.t.Fatalf("plan category row missing: %+v", p.Structure.Elements)
	}
	return row
}

func (h *periodHarness) elementID(externalID string, typ model.ElementType) string {
	h.t.Helper()
	var id string
	if err := h.tdb.Raw.QueryRow(h.tdb.Rebind(`SELECT id FROM budgets_elements WHERE budget_id = ? AND external_id = ? AND type = ?`),
		h.budgetID, externalID, int(typ)).Scan(&id); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func sameMonths(t *testing.T, p model.BudgetPlanResult, want ...string) {
	t.Helper()
	if fmt.Sprint(p.Months) != fmt.Sprint(want) {
		t.Fatalf("months = %v, want %v", p.Months, want)
	}
}

func TestGetBudgetPlanShifted(t *testing.T) {
	h := newPeriodHarness(t, 28, periodNow)
	h.f.Transaction(fixture.Transaction{UserID: h.owner.String(), AccountID: h.account, Type: 1, Amount: "1000", SpentAt: "2026-09-27 23:59:59"})
	h.f.Transaction(fixture.Transaction{UserID: h.owner.String(), AccountID: h.account, Type: 1, Amount: "500", SpentAt: "2026-09-28 00:00:00"})
	h.spend("100", "2026-10-27 23:59:59")
	h.spend("50", "2026-10-28 00:00:00")
	h.spend("20", "2026-11-27 10:00:00")
	h.spend("10", "2026-11-28 00:00:00")
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: h.elementID(h.category, model.ElementCategory), Period: "2026-11-01 00:00:00", Amount: "40"})

	p := h.plan("2026-10-01", 2)
	sameMonths(t, p, "2026-10-01", "2026-11-01")
	row := h.planCategory(p)
	sameAmount(t, "october actual", row.Cells[0].Actual, "100")
	sameAmount(t, "november actual", row.Cells[1].Actual, "70")
	if row.Cells[0].Planned != "" {
		t.Errorf("october planned = %q, want empty", row.Cells[0].Planned)
	}
	sameAmount(t, "november planned", row.Cells[1].Planned, "40")
	if len(p.OpeningBalances) != 1 {
		t.Fatalf("opening balances = %+v", p.OpeningBalances)
	}
	sameAmount(t, "opening balance (before 2026-09-28)", p.OpeningBalances[0].Amount, "1000")
	income, ok := planRow(p, model.UncategorizedID, model.ElementIncomeCategory)
	if !ok {
		t.Fatalf("income uncategorized row missing: %+v", p.Structure.Elements)
	}
	sameAmount(t, "october income", income.Cells[0].Actual, "500")
	sameAmount(t, "november income", income.Cells[1].Actual, "0")
}

func TestGetBudgetPlanShiftedCurrency(t *testing.T) {
	h := newPeriodHarness(t, 28, periodNow)
	eur := h.f.Currency(fixture.Currency{Code: "EUR", Symbol: "€"})
	h.f.Rate(fixture.Rate{CurrencyID: eur, Rate: "0.50000000", PublishedAt: "2026-10-05"})
	h.f.Rate(fixture.Rate{CurrencyID: eur, Rate: "0.25000000", PublishedAt: "2026-11-05"})
	euro := h.f.Account(fixture.Account{UserID: h.owner.String(), CurrencyID: eur, Name: "Euro"})
	h.f.BudgetAccount(h.budgetID, euro)
	h.f.Transaction(fixture.Transaction{UserID: h.owner.String(), AccountID: euro, CategoryID: h.category, Amount: "10", SpentAt: "2026-10-10 12:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: h.owner.String(), AccountID: euro, CategoryID: h.category, Amount: "10", SpentAt: "2026-10-29 12:00:00"})

	p := h.plan("2026-10-01", 2)
	row := h.planCategory(p)
	// [2026-09-28, 2026-10-28) averages October's rate (0.5), [2026-10-28,
	// 2026-11-28) November's (0.25).
	sameAmount(t, "october actual", row.Cells[0].Actual, "20")
	sameAmount(t, "november actual", row.Cells[1].Actual, "40")
}

func TestGetBudgetPlanShiftedSavings(t *testing.T) {
	h := newPeriodHarness(t, 28, periodNow)
	h.f.BudgetAccount(h.budgetID, h.savings)
	h.exec(`UPDATE budgets_accounts SET is_savings = ? WHERE budget_id = ? AND account_id = ?`, true, h.budgetID, h.savings)
	h.f.Transaction(fixture.Transaction{
		UserID: h.owner.String(), AccountID: h.account, AccountRecipientID: h.savings, Type: 2,
		Amount: "30", AmountRecipient: "30", SpentAt: "2026-10-29 09:00:00",
	})

	p := h.plan("2026-10-01", 2)
	if len(p.Structure.Savings) != 1 {
		t.Fatalf("savings rows = %+v", p.Structure.Savings)
	}
	cells := p.Structure.Savings[0].Cells
	sameAmount(t, "october saved", cells[0].Actual, "0")
	sameAmount(t, "october closing", cells[0].ClosingBalance, "0")
	sameAmount(t, "november saved", cells[1].Actual, "30")
	sameAmount(t, "november closing", cells[1].ClosingBalance, "30")
	if len(p.SavingsFlows) != 1 || p.SavingsFlows[0].Month != "2026-11-01" {
		t.Fatalf("savings flows = %+v, want one entry for 2026-11-01", p.SavingsFlows)
	}
	sameAmount(t, "november flow", p.SavingsFlows[0].Amount, "30")
}

func TestGetBudgetPlanShiftedTransfers(t *testing.T) {
	h := newPeriodHarness(t, 28, periodNow)
	other := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Other"})
	for _, tr := range []struct{ amount, at string }{{"15", "2026-11-27 10:00:00"}, {"25", "2026-11-28 00:00:00"}} {
		h.f.Transaction(fixture.Transaction{
			UserID: h.owner.String(), AccountID: h.account, AccountRecipientID: other, Type: 2,
			Amount: tr.amount, AmountRecipient: tr.amount, SpentAt: tr.at,
		})
	}

	p := h.plan("2026-11-01", 2)
	sameMonths(t, p, "2026-11-01", "2026-12-01")
	for i, want := range []string{"15", "25"} {
		items := p.Transfers[i].Items
		if p.Transfers[i].Period != p.Months[i] || len(items) != 1 {
			t.Fatalf("transfers[%d] = %+v", i, p.Transfers[i])
		}
		sameAmount(t, p.Months[i]+" out", items[0].Out, want)
	}
}

func TestGetBudgetPlanShiftedUpcoming(t *testing.T) {
	h := newPeriodHarness(t, 28, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	h.monthly("40", time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC))
	h.spend("9", "2026-11-27 10:00:00")

	p := h.plan("2026-11-01", 2)
	row := h.planCategory(p)
	// the future-dated 9 is upcoming in its own period, the 2026-11-29 payment
	// in the next one
	sameAmount(t, "2026-11 upcoming", row.Cells[0].Upcoming, "9")
	sameAmount(t, "2026-12 upcoming", row.Cells[1].Upcoming, "40")
	sameAmount(t, "2026-11 actual", row.Cells[0].Actual, "0")
}

// oracleRow is one generated transaction. kind: expense, income, out (to a
// non-member account), in (from it), internal (between the two members).
type oracleRow struct {
	kind     string
	category string // "" = uncategorized
	account  string
	amount   vo.DecimalNumber
	at       time.Time
}

type oracleSums map[string]map[string]vo.DecimalNumber // key -> bucket -> sum

func (o oracleSums) add(key, bucket string, v vo.DecimalNumber) {
	if o[key] == nil {
		o[key] = map[string]vo.DecimalNumber{}
	}
	cur, ok := o[key][bucket]
	if !ok {
		cur = vo.NewDecimal("0")
	}
	o[key][bucket] = cur.Add(v)
}

func (o oracleSums) get(key, bucket string) vo.DecimalNumber {
	if v, ok := o[key][bucket]; ok {
		return v
	}
	return vo.NewDecimal("0")
}

func randomAmount(r *rand.Rand, maxCents int) vo.DecimalNumber {
	c := 1 + r.IntN(maxCents)
	return vo.NewDecimal(fmt.Sprintf("%d.%02d", c/100, c%100))
}

func generateOracleRows(r *rand.Rand, d int, expenseCats, incomeCats, members []string) []oracleRow {
	pick := func(list []string) string {
		i := r.IntN(len(list) + 1)
		if i == len(list) {
			return ""
		}
		return list[i]
	}
	row := func(at time.Time, amount vo.DecimalNumber) oracleRow {
		o := oracleRow{account: members[r.IntN(len(members))], amount: amount, at: at}
		switch n := r.IntN(10); {
		case n < 6:
			o.kind, o.category = "expense", pick(expenseCats)
		case n < 8:
			o.kind, o.category = "income", pick(incomeCats)
		default:
			o.kind = []string{"out", "in", "internal"}[r.IntN(3)]
		}
		return o
	}
	var rows []oracleRow
	first := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	span := int(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC).Sub(first) / time.Second)
	for range 200 {
		rows = append(rows, row(first.Add(time.Duration(r.IntN(span))*time.Second), randomAmount(r, 50000)))
	}
	for m := first; m.Before(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)); m = m.AddDate(0, 1, 0) {
		boundary := m.AddDate(0, 0, d-1)
		rows = append(rows, row(boundary, randomAmount(r, 50000)), row(boundary.Add(-time.Second), randomAmount(r, 50000)))
	}
	dense := time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC)
	for i := range 30 {
		at := dense
		if i%5 != 0 {
			at = dense.Add(time.Duration(r.IntN(86400)) * time.Second)
		}
		rows = append(rows, oracleRow{kind: "expense", category: pick(expenseCats), account: members[r.IntN(len(members))], amount: randomAmount(r, 999), at: at})
	}
	return rows
}

// oracleKey finds the period key whose PeriodWindow holds t by scanning the
// candidate keys, independently of PeriodOf.
func oracleKey(t *testing.T, at time.Time, d int) string {
	t.Helper()
	found := ""
	for k := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC); k.Before(time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC)); k = k.AddDate(0, 1, 0) {
		start, end := model.PeriodWindow(k, d)
		if !at.Before(start) && at.Before(end) {
			if found != "" {
				t.Fatalf("%s in two windows (d=%d)", at, d)
			}
			found = k.Format(datetime.DateLayout)
		}
	}
	if found == "" {
		t.Fatalf("%s in no window (d=%d)", at, d)
	}
	return found
}

func TestPlanMatchesBudgetOracle(t *testing.T) {
	for _, d := range []int{1, 5, 15, 16, 28} {
		t.Run(fmt.Sprint("d=", d), func(t *testing.T) { runPlanOracle(t, d) })
	}
}

func runPlanOracle(t *testing.T, d int) {
	h := newPeriodHarness(t, d, time.Date(2027, 3, 15, 12, 0, 0, 0, time.UTC))
	h.exec(`UPDATE budgets SET started_at = ? WHERE id = ?`, "2026-07-01 00:00:00", h.budgetID)
	second := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Card"})
	h.f.BudgetAccount(h.budgetID, second)
	outside := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Outside"})
	expenseCats := []string{h.category}
	for _, name := range []string{"Rent", "Fun"} {
		c := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: name})
		h.f.BudgetElement(fixture.BudgetElement{BudgetID: h.budgetID, ExternalID: c, Type: int(model.ElementCategory)})
		expenseCats = append(expenseCats, c)
	}
	var incomeCats []string
	for _, name := range []string{"Salary", "Bonus"} {
		c := h.f.Category(fixture.Category{UserID: h.owner.String(), Name: name, Type: 1})
		h.f.BudgetElement(fixture.BudgetElement{BudgetID: h.budgetID, ExternalID: c, Type: int(model.ElementIncomeCategory)})
		incomeCats = append(incomeCats, c)
	}
	members := []string{h.account, second}

	rows := generateOracleRows(rand.New(rand.NewPCG(20261028, uint64(d))), d, expenseCats, incomeCats, members)
	spent, income, totals := oracleSums{}, oracleSums{}, oracleSums{}
	keySet := map[string]bool{}
	for _, o := range rows {
		tx := fixture.Transaction{UserID: h.owner.String(), AccountID: o.account, CategoryID: o.category,
			Amount: o.amount.String(), SpentAt: o.at.Format(datetime.Layout)}
		key := oracleKey(t, o.at, d)
		keySet[key] = true
		switch o.kind {
		case "expense":
			spent.add(key, o.category, o.amount)
			totals.add(key, "expenses", o.amount)
		case "income":
			tx.Type = 1
			income.add(key, o.category, o.amount)
			totals.add(key, "income", o.amount)
		case "out":
			tx.Type, tx.AccountRecipientID, tx.AmountRecipient, tx.CategoryID = 2, outside, o.amount.String(), ""
			totals.add(key, "out", o.amount)
		case "in":
			tx.Type, tx.AccountID, tx.AccountRecipientID, tx.AmountRecipient, tx.CategoryID = 2, outside, o.account, o.amount.String(), ""
			totals.add(key, "in", o.amount)
		case "internal":
			other := members[0]
			if o.account == other {
				other = members[1]
			}
			tx.Type, tx.AccountRecipientID, tx.AmountRecipient, tx.CategoryID = 2, other, o.amount.String(), ""
		}
		h.f.Transaction(tx)
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	first, _ := time.Parse(datetime.DateLayout, keys[0])
	last, _ := time.Parse(datetime.DateLayout, keys[len(keys)-1])
	var months []string
	for k := first; !k.After(last); k = k.AddDate(0, 1, 0) {
		months = append(months, k.Format(datetime.DateLayout))
	}

	// running balance strictly before each window start
	balanceBefore := func(key string) vo.DecimalNumber {
		k, _ := time.Parse(datetime.DateLayout, key)
		start, _ := model.PeriodWindow(k, d)
		bal := vo.NewDecimal("0")
		for _, o := range rows {
			if !o.at.Before(start) {
				continue
			}
			switch o.kind {
			case "expense", "out":
				bal = bal.Sub(o.amount)
			case "income", "in":
				bal = bal.Add(o.amount)
			}
		}
		return bal
	}

	expenseBuckets := append(append([]string(nil), expenseCats...), "")
	incomeBuckets := append(append([]string(nil), incomeCats...), "")
	idOf := func(c string) string {
		if c == "" {
			return model.UncategorizedID
		}
		return c
	}

	p := h.plan(months[0], len(months))
	sameMonths(t, p, months...)
	sameAmount(t, "plan opening balance", p.OpeningBalances[0].Amount, balanceBefore(months[0]).String())
	planCell := func(id string, typ model.ElementType, i int) string {
		row, ok := planRow(p, id, typ)
		if !ok {
			return "0"
		}
		return row.Cells[i].Actual
	}

	for i, key := range months {
		res := h.get(context.Background(), h.owner, key)
		for _, c := range expenseBuckets {
			want := spent.get(key, c).String()
			budgetSpent := "0"
			for _, e := range res.Structure.Elements {
				if e.Id == idOf(c) && e.Type == int(model.ElementCategory) {
					budgetSpent = e.Spent
				}
			}
			cell := planCell(idOf(c), model.ElementCategory, i)
			sameAmount(t, fmt.Sprintf("%s get-budget spent %q", key, c), budgetSpent, want)
			sameAmount(t, fmt.Sprintf("%s plan actual %q", key, c), cell, want)
			sameAmount(t, fmt.Sprintf("%s plan vs get-budget %q", key, c), cell, budgetSpent)
		}
		for _, c := range incomeBuckets {
			sameAmount(t, fmt.Sprintf("%s plan income %q", key, c), planCell(idOf(c), model.ElementIncomeCategory, i), income.get(key, c).String())
		}

		bal := res.Balances[0]
		// The balances block's SQLite SUM is a float (report_sql.go), so it can
		// read 1412.58999999 for 1412.59 on any start day; compare it in cents.
		sameAmount(t, key+" get-budget income", vo.NewDecimal(*bal.Income).Round(2).String(), totals.get(key, "income").String())
		sameAmount(t, key+" get-budget expenses", vo.NewDecimal(*bal.Expenses).Round(2).String(), totals.get(key, "expenses").String())
		sameAmount(t, key+" get-budget start balance", *bal.StartBalance, balanceBefore(key).String())
		net := totals.get(key, "in").Sub(totals.get(key, "out"))
		sameAmount(t, key+" get-budget holdings", *bal.Holdings, net.String())

		planIn, planOut := "0", "0"
		if items := p.Transfers[i].Items; len(items) > 0 {
			planIn, planOut = items[0].In, items[0].Out
		}
		sameAmount(t, key+" plan transfers in", planIn, totals.get(key, "in").String())
		sameAmount(t, key+" plan transfers out", planOut, totals.get(key, "out").String())
		sameAmount(t, key+" plan vs get-budget holdings", vo.NewDecimal(planIn).Sub(vo.NewDecimal(planOut)).String(), *bal.Holdings)
	}
}
