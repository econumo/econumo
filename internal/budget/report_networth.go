package budget

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetNetWorthReport returns every account's month-end balance. Without a
// budgetId it covers the accounts the caller owns; with one, every member
// account of the budget. Deleted accounts are left out entirely, their past
// balances too, and do not move the start of an all-time window.
func (s *Service) GetNetWorthReport(ctx context.Context, userID vo.Id, req model.GetNetWorthReportRequest) (*model.GetNetWorthReportResult, error) {
	var (
		accountIDs []vo.Id
		currency   vo.Id
		savings    = map[string]bool{}
		window     reportWindow
		endCap     *time.Time
		err        error
	)
	if req.BudgetId != "" {
		budgetID, perr := vo.ParseId(req.BudgetId)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
		}
		b, rerr := s.requireBudget(ctx, userID, budgetID)
		if rerr != nil {
			return nil, rerr
		}
		for _, m := range b.accounts {
			accountIDs = append(accountIDs, m.AccountID)
			savings[m.AccountID.String()] = m.IsSavings
		}
		currency = b.budget.CurrencyID
		endCap = b.endMonth()
	} else {
		if accountIDs, err = s.accounts.OwnedAccountIDs(ctx, userID); err != nil {
			return nil, err
		}
		if currency, err = s.personalReportCurrency(ctx, userID); err != nil {
			return nil, err
		}
	}
	views, accountIDs, err := s.liveAccounts(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	if window, err = s.resolveReportWindow(ctx, req.From, req.To, accountIDs, endCap); err != nil {
		return nil, err
	}
	// Balances are month-end, so an all-time window opens at the close of the
	// month before the first transaction (zero), and its change covers the
	// first month's activity, like a preset that asks for the month before.
	if req.From == "" && window.months < reportMonthsMax {
		window.from = window.from.AddDate(0, -1, 0)
		window.months++
	}
	item, err := s.buildNetWorth(ctx, userID, views, accountIDs, window, currency)
	if err != nil {
		return nil, err
	}
	for i := range item.Accounts {
		if savings[item.Accounts[i].Id] {
			item.Accounts[i].IsSavings = 1
		}
	}
	return &model.GetNetWorthReportResult{Item: item}, nil
}

func (s *Service) liveAccounts(ctx context.Context, ids []vo.Id) ([]model.AccountView, []vo.Id, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	all, err := s.accounts.AccountsByIDs(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	views := make([]model.AccountView, 0, len(all))
	live := make([]vo.Id, 0, len(all))
	for _, v := range all {
		if v.IsDeleted {
			continue
		}
		id, err := vo.ParseId(v.ID)
		if err != nil {
			return nil, nil, err
		}
		views = append(views, v)
		live = append(live, id)
	}
	return views, live, nil
}

func (s *Service) buildNetWorth(ctx context.Context, userID vo.Id, views []model.AccountView, accountIDs []vo.Id, w reportWindow, currency vo.Id) (model.NetWorthReportResult, error) {
	months := w.monthList()
	monthStrs := make([]string, len(months))
	monthIdx := map[string]int{}
	for i, m := range months {
		monthStrs[i] = m.Format(datetime.DateLayout)
		monthIdx[monthStrs[i]] = i
	}
	result := model.NetWorthReportResult{CurrencyId: currency.String(), Months: monthStrs, Accounts: []model.NetWorthAccountRow{}}
	if len(accountIDs) == 0 {
		return result, nil
	}

	opening, err := s.read.AccountsBalancesBeforeDate(ctx, accountIDs, w.from)
	if err != nil {
		return result, err
	}
	netsTo := w.end()
	if tomorrow := localTomorrow(ctx, s.clock.Now()); tomorrow.Before(netsTo) {
		netsTo = tomorrow
	}
	nets, err := s.read.AccountsNetByMonth(ctx, accountIDs, w.from, netsTo)
	if err != nil {
		return result, err
	}

	running := map[string]vo.DecimalNumber{}
	for _, r := range opening {
		running[r.AccountID] = vo.NewDecimal(r.Balance)
	}
	delta := map[string][]vo.DecimalNumber{}
	for _, r := range nets {
		i, ok := monthIdx[r.Month]
		if !ok {
			continue
		}
		if delta[r.AccountID] == nil {
			d := make([]vo.DecimalNumber, len(months))
			for k := range d {
				d[k] = vo.NewDecimal("0")
			}
			delta[r.AccountID] = d
		}
		delta[r.AccountID][i] = delta[r.AccountID][i].Add(vo.NewDecimal(r.Amount))
	}

	toConvert := map[string][]model.ConvertItem{}
	native := map[string][]vo.DecimalNumber{}
	for _, v := range views {
		cur, err := vo.ParseId(v.CurrencyID)
		if err != nil {
			return result, err
		}
		bal, ok := running[v.ID]
		if !ok {
			bal = vo.NewDecimal("0")
		}
		series := make([]vo.DecimalNumber, len(months))
		for i := range months {
			if d := delta[v.ID]; d != nil {
				bal = bal.Add(d[i])
			}
			series[i] = bal
			toConvert[fmt.Sprintf("%s|%d", v.ID, i)] = []model.ConvertItem{{
				PeriodStart: months[i], PeriodEnd: months[i].AddDate(0, 1, 0),
				From: cur, To: currency, Amount: bal,
			}}
		}
		native[v.ID] = series
	}

	// BulkConvert's top-level period doubles as month 0's rate range.
	converted, err := s.convertor.BulkConvert(ctx, months[0], months[0].AddDate(0, 1, 0), toConvert)
	if err != nil {
		return result, err
	}
	get := convertedGetter(converted)

	me := userID.String()
	for _, v := range views {
		balances := make([]string, len(months))
		nonZero := false
		for i := range months {
			x := get(fmt.Sprintf("%s|%d", v.ID, i))
			balances[i] = x.String()
			if !x.IsZero() {
				nonZero = true
			}
		}
		if !nonZero {
			continue
		}
		row := model.NetWorthAccountRow{
			Id: v.ID, Name: v.Name, Icon: v.Icon, CurrencyId: v.CurrencyID,
			OwnerUserId: v.OwnerID, Balances: balances,
		}
		result.Accounts = append(result.Accounts, row)
	}
	sort.SliceStable(result.Accounts, func(a, b int) bool {
		x, y := result.Accounts[a], result.Accounts[b]
		if (x.OwnerUserId == me) != (y.OwnerUserId == me) {
			return x.OwnerUserId == me
		}
		if x.OwnerUserId != y.OwnerUserId {
			return x.OwnerUserId < y.OwnerUserId
		}
		if x.Name != y.Name {
			return x.Name < y.Name
		}
		return x.Id < y.Id
	})
	return result, nil
}
