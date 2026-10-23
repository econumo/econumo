package budget

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	flowExpense = 0
	flowIncome  = 1
)

// GetFlowReport returns the monthly income/expense/transfer flow. Without a
// budgetId it covers every account the caller owns, deleted ones included.
func (s *Service) GetFlowReport(ctx context.Context, userID vo.Id, req model.GetFlowReportRequest) (*model.GetFlowReportResult, error) {
	groupBy, err := parseFlowGroupBy(req.GroupBy)
	if err != nil {
		return nil, err
	}
	if req.BudgetId != "" {
		return nil, errors.New("flow report: budget scope not implemented")
	}
	accountIDs, err := s.accounts.OwnedAccountIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	window, err := s.resolveReportWindow(ctx, req.From, req.To, accountIDs, nil)
	if err != nil {
		return nil, err
	}
	currency, err := s.personalReportCurrency(ctx, userID)
	if err != nil {
		return nil, err
	}
	item, err := s.buildPersonalFlow(ctx, userID, accountIDs, window, groupBy, currency)
	if err != nil {
		return nil, err
	}
	return &model.GetFlowReportResult{Item: item}, nil
}

func (s *Service) personalReportCurrency(ctx context.Context, userID vo.Id) (vo.Id, error) {
	raw, err := s.users.DefaultCurrencyID(ctx, userID.String())
	if err != nil {
		return vo.Id{}, err
	}
	if raw == "" {
		return s.rates.BaseCurrencyID(ctx)
	}
	return vo.ParseId(raw)
}

type flowGroupMeta struct {
	name       string
	icon       string
	isArchived bool
}

func (s *Service) flowGroupNames(ctx context.Context, userID vo.Id, groupBy string) (map[string]flowGroupMeta, error) {
	owners := []vo.Id{userID}
	out := map[string]flowGroupMeta{}
	switch groupBy {
	case flowGroupCategory:
		cats, err := s.metadata.CategoriesByOwners(ctx, owners)
		if err != nil {
			return nil, err
		}
		for _, c := range cats {
			out[c.ID] = flowGroupMeta{name: c.Name, icon: c.Icon, isArchived: c.IsArchived}
		}
	case flowGroupTag:
		tags, err := s.metadata.TagsByOwners(ctx, owners)
		if err != nil {
			return nil, err
		}
		for _, tg := range tags {
			out[tg.ID] = flowGroupMeta{name: tg.Name, isArchived: tg.IsArchived}
		}
	default:
		payees, err := s.metadata.PayeesByOwners(ctx, owners)
		if err != nil {
			return nil, err
		}
		for _, p := range payees {
			out[p.ID] = flowGroupMeta{name: p.Name}
		}
	}
	return out, nil
}

func flowKind(groupBy string, typ int) string {
	if groupBy == flowGroupCategory && typ == flowIncome {
		return "income_category"
	}
	return groupBy
}

func flowKey(typ int, group string, monthIdx int) string {
	return fmt.Sprintf("f%d|%s|%d", typ, group, monthIdx)
}

func (s *Service) buildPersonalFlow(ctx context.Context, userID vo.Id, accountIDs []vo.Id, w reportWindow, groupBy string, currency vo.Id) (model.FlowReportResult, error) {
	months := w.monthList()
	monthStrs := make([]string, len(months))
	monthIdx := map[string]int{}
	for i, m := range months {
		monthStrs[i] = m.Format(datetime.DateLayout)
		monthIdx[monthStrs[i]] = i
	}
	end := w.end()

	flows, err := s.read.FlowByMonth(ctx, accountIDs, w.from, end, groupBy)
	if err != nil {
		return model.FlowReportResult{}, err
	}
	transfers, err := s.read.TransfersByMonth(ctx, accountIDs, w.from, end)
	if err != nil {
		return model.FlowReportResult{}, err
	}
	names, err := s.flowGroupNames(ctx, userID, groupBy)
	if err != nil {
		return model.FlowReportResult{}, err
	}

	toConvert := map[string][]model.ConvertItem{}
	add := func(key string, i int, cur string, amount string) error {
		from, err := vo.ParseId(cur)
		if err != nil {
			return err
		}
		toConvert[key] = append(toConvert[key], model.ConvertItem{
			PeriodStart: months[i], PeriodEnd: months[i].AddDate(0, 1, 0),
			From: from, To: currency, Amount: vo.NewDecimal(amount),
		})
		return nil
	}

	type groupRef struct {
		typ   int
		group string
	}
	seen := map[groupRef]bool{}
	var order []groupRef
	for _, r := range flows {
		i, ok := monthIdx[r.Month]
		if !ok {
			continue
		}
		group := model.UncategorizedID
		if r.GroupID != nil {
			if _, known := names[*r.GroupID]; known {
				group = *r.GroupID
			}
		}
		ref := groupRef{r.Type, group}
		if !seen[ref] {
			seen[ref] = true
			order = append(order, ref)
		}
		if err := add(flowKey(r.Type, group, i), i, r.CurrencyID, r.Amount); err != nil {
			return model.FlowReportResult{}, err
		}
	}
	for _, r := range transfers {
		i, ok := monthIdx[r.Month]
		if !ok {
			continue
		}
		if err := add(fmt.Sprintf("ti|%d", i), i, r.CurrencyID, r.In); err != nil {
			return model.FlowReportResult{}, err
		}
		if err := add(fmt.Sprintf("to|%d", i), i, r.CurrencyID, r.Out); err != nil {
			return model.FlowReportResult{}, err
		}
	}

	// BulkConvert's top-level period doubles as month 0's rate range, so it
	// must be month 0's own range (see buildPlanStructure).
	converted, err := s.convertor.BulkConvert(ctx, months[0], months[0].AddDate(0, 1, 0), toConvert)
	if err != nil {
		return model.FlowReportResult{}, err
	}
	get := convertedGetter(converted)

	type built struct {
		row   model.ReportRow
		total vo.DecimalNumber
	}
	var expense, income []built
	for _, ref := range order {
		amounts := make([]string, len(months))
		total := vo.NewDecimal("0")
		for i := range months {
			v := get(flowKey(ref.typ, ref.group, i))
			amounts[i] = v.String()
			total = total.Add(v)
		}
		row := model.ReportRow{
			Id: ref.group, Kind: flowKind(groupBy, ref.typ),
			Name: model.UncategorizedName, Amounts: amounts,
			Planned: []string{}, Children: []model.ReportChildRow{},
		}
		if ref.group != model.UncategorizedID {
			meta := names[ref.group]
			row.Name, row.Icon = meta.name, meta.icon
			if meta.isArchived {
				row.IsArchived = 1
			}
		}
		if ref.typ == flowIncome {
			income = append(income, built{row, total})
		} else {
			expense = append(expense, built{row, total})
		}
	}
	emit := func(list []built) []model.ReportRow {
		sort.SliceStable(list, func(a, b int) bool {
			if !list[a].total.Equals(list[b].total) {
				return list[a].total.IsGreaterThan(list[b].total)
			}
			return list[a].row.Name < list[b].row.Name
		})
		out := make([]model.ReportRow, len(list))
		for i, b := range list {
			out[i] = b.row
		}
		return out
	}

	in := make([]string, len(months))
	out := make([]string, len(months))
	for i := range months {
		in[i] = get(fmt.Sprintf("ti|%d", i)).String()
		out[i] = get(fmt.Sprintf("to|%d", i)).String()
	}
	return model.FlowReportResult{
		CurrencyId: currency.String(), Months: monthStrs,
		Expense: emit(expense), Income: emit(income),
		TransfersIn: in, TransfersOut: out, Savings: []model.ReportRow{},
	}, nil
}
