package budget

import (
	"context"
	"sort"

	"github.com/econumo/econumo/internal/model"
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
	// Keyless rows (not synced yet) trail, by account id.
	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := out[i].sortKey, out[j].sortKey
		if (ki == "") != (kj == "") {
			return kj == ""
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

// addMonthlySavings queues each savings row's actual for the period into the
// structure's single bulk conversion, account currency -> element currency.
func (s *Service) addMonthlySavings(ctx context.Context, f filters, options map[string]elementOption, toConvert map[string][]model.ConvertItem) ([]savingsRow, error) {
	rows, err := savingsRows(f, options)
	if err != nil || len(rows) == 0 {
		return rows, err
	}
	ids, err := savingsAccountIDs(f)
	if err != nil {
		return nil, err
	}
	actual, err := s.read.SavingsByMonth(ctx, ids, f.everydayAccountIDs, f.periodStart, f.periodEnd)
	if err != nil {
		return nil, err
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
		from, perr := vo.ParseId(r.account.CurrencyID)
		if perr != nil {
			return nil, perr
		}
		key := savingsSpentKey(a.AccountID)
		toConvert[key] = append(toConvert[key], model.ConvertItem{
			PeriodStart: f.periodStart, PeriodEnd: f.periodEnd, From: from, To: r.currencyID, Amount: vo.NewDecimal(a.Amount),
		})
	}
	return rows, nil
}

func emitMonthlySavings(rows []savingsRow, limits map[string]budgetedAmount, get func(string) vo.DecimalNumber) []model.SavingsElementResult {
	zero := vo.NewDecimal("0")
	out := []model.SavingsElementResult{}
	for _, r := range rows {
		budgeted := orZero(limits[elementKey(r.account.ID, model.ElementSavings)].budgeted, zero)
		spent := get(savingsSpentKey(r.account.ID))
		// A deleted account stays only while it still carries a plan or activity.
		if r.account.IsDeleted && budgeted.IsZero() && spent.IsZero() {
			continue
		}
		out = append(out, model.SavingsElementResult{
			Id: r.account.ID, Type: int(model.ElementSavings.Int16()), Name: r.account.Name, Icon: r.account.Icon,
			CurrencyId: r.currencyID.String(), OwnerUserId: r.account.OwnerID, IsArchived: boolToInt(r.account.IsDeleted),
			Position: len(out), Budgeted: budgeted.String(), Spent: spent.String(), Available: budgeted.Sub(spent).String(),
		})
	}
	return out
}
