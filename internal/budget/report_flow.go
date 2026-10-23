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
		if groupBy != flowGroupCategory {
			return nil, invalidChoice("groupBy")
		}
		return s.budgetFlowReport(ctx, userID, req)
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
	item, err := s.buildPersonalFlow(ctx, accountIDs, window, groupBy, currency)
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

func (s *Service) flowGroupNames(ctx context.Context, groupBy string, flows []model.MonthlyFlowRow) (map[string]flowGroupMeta, error) {
	var ids []vo.Id
	seen := map[string]bool{}
	for _, r := range flows {
		if r.GroupID == nil || seen[*r.GroupID] {
			continue
		}
		seen[*r.GroupID] = true
		id, err := vo.ParseId(*r.GroupID)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	out := map[string]flowGroupMeta{}
	if len(ids) == 0 {
		return out, nil
	}
	switch groupBy {
	case flowGroupCategory:
		cats, err := s.metadata.CategoriesByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, c := range cats {
			out[c.ID] = flowGroupMeta{name: c.Name, icon: c.Icon, isArchived: c.IsArchived}
		}
	case flowGroupTag:
		tags, err := s.metadata.TagsByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, tg := range tags {
			out[tg.ID] = flowGroupMeta{name: tg.Name, isArchived: tg.IsArchived}
		}
	default:
		payees, err := s.metadata.PayeesByIDs(ctx, ids)
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

func (s *Service) buildPersonalFlow(ctx context.Context, accountIDs []vo.Id, w reportWindow, groupBy string, currency vo.Id) (model.FlowReportResult, error) {
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
	names, err := s.flowGroupNames(ctx, groupBy, flows)
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

	var expense, income []builtReportRow
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
			income = append(income, builtReportRow{row, total})
		} else {
			expense = append(expense, builtReportRow{row, total})
		}
	}
	in := make([]string, len(months))
	out := make([]string, len(months))
	for i := range months {
		in[i] = get(fmt.Sprintf("ti|%d", i)).String()
		out[i] = get(fmt.Sprintf("to|%d", i)).String()
	}
	return model.FlowReportResult{
		CurrencyId: currency.String(), Months: monthStrs,
		Expense: sortReportRows(expense), Income: sortReportRows(income),
		TransfersIn: in, TransfersOut: out, Savings: []model.ReportRow{},
	}, nil
}

type builtReportRow struct {
	row   model.ReportRow
	total vo.DecimalNumber
}

// sortReportRows orders rows by their window total, largest first, then name.
func sortReportRows(list []builtReportRow) []model.ReportRow {
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

var reportElementKinds = map[int]string{
	int(model.ElementEnvelope):       "envelope",
	int(model.ElementCategory):       "category",
	int(model.ElementTag):            "tag",
	int(model.ElementIncomeCategory): "income_category",
	int(model.ElementIncomeEnvelope): "income_envelope",
}

// budgetFlowReport reads the budget's own rows through the plan builder, so
// every figure equals get-budget-plan's for the same months. Unlike the plan
// it does not clamp to the budget start: earlier months are wanted.
func (s *Service) budgetFlowReport(ctx context.Context, userID vo.Id, req model.GetFlowReportRequest) (*model.GetFlowReportResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	b, err := s.requireBudget(ctx, userID, budgetID)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]vo.Id, 0, len(b.accounts))
	for _, m := range b.accounts {
		accountIDs = append(accountIDs, m.AccountID)
	}
	window, err := s.resolveReportWindow(ctx, req.From, req.To, accountIDs, b.endMonth())
	if err != nil {
		return nil, err
	}
	plan, err := s.BuildBudgetPlan(ctx, userID, b, window.from, window.months)
	if err != nil {
		return nil, err
	}
	item, err := s.flowFromPlan(ctx, b.budget.CurrencyID, window.monthList(), plan)
	if err != nil {
		return nil, err
	}
	return &model.GetFlowReportResult{Item: item}, nil
}

// flowFromPlan converts the plan's element-currency cells (actuals and plans
// alike) to the budget currency month by month and drops rows with neither
// activity nor a plan in the window.
func (s *Service) flowFromPlan(ctx context.Context, currency vo.Id, months []time.Time, plan model.BudgetPlanResult) (model.FlowReportResult, error) {
	toConvert := map[string][]model.ConvertItem{}
	queue := func(key string, i int, from, amount string) error {
		if amount == "" {
			return nil
		}
		cid, err := vo.ParseId(from)
		if err != nil {
			return err
		}
		toConvert[key] = append(toConvert[key], model.ConvertItem{
			PeriodStart: months[i], PeriodEnd: months[i].AddDate(0, 1, 0),
			From: cid, To: currency, Amount: vo.NewDecimal(amount),
		})
		return nil
	}
	actualKey := func(row string, i int) string { return fmt.Sprintf("%s|a|%d", row, i) }
	plannedKey := func(row string, i int) string { return fmt.Sprintf("%s|p|%d", row, i) }
	childKey := func(row string, ci, i int) string { return fmt.Sprintf("%s|c%d|%d", row, ci, i) }

	for e, el := range plan.Structure.Elements {
		row := fmt.Sprintf("e%d", e)
		for i, c := range el.Cells {
			if err := queue(actualKey(row, i), i, el.CurrencyId, c.Actual); err != nil {
				return model.FlowReportResult{}, err
			}
			if err := queue(plannedKey(row, i), i, el.CurrencyId, c.Planned); err != nil {
				return model.FlowReportResult{}, err
			}
		}
		for ci, ch := range el.Children {
			for i, c := range ch.Cells {
				if err := queue(childKey(row, ci, i), i, el.CurrencyId, c.Actual); err != nil {
					return model.FlowReportResult{}, err
				}
			}
		}
	}
	for k, sv := range plan.Structure.Savings {
		row := fmt.Sprintf("s%d", k)
		for i, c := range sv.Cells {
			if err := queue(actualKey(row, i), i, sv.CurrencyId, c.Actual); err != nil {
				return model.FlowReportResult{}, err
			}
			if err := queue(plannedKey(row, i), i, sv.CurrencyId, c.Planned); err != nil {
				return model.FlowReportResult{}, err
			}
		}
	}
	for i, mt := range plan.Transfers {
		for _, it := range mt.Items {
			if err := queue(fmt.Sprintf("ti|%d", i), i, it.CurrencyId, it.In); err != nil {
				return model.FlowReportResult{}, err
			}
			if err := queue(fmt.Sprintf("to|%d", i), i, it.CurrencyId, it.Out); err != nil {
				return model.FlowReportResult{}, err
			}
		}
	}

	converted, err := s.convertor.BulkConvert(ctx, months[0], months[0].AddDate(0, 1, 0), toConvert)
	if err != nil {
		return model.FlowReportResult{}, err
	}
	get := convertedGetter(converted)

	// cells renders one row's amounts and plans; active reports whether the
	// row has any non-zero amount or any plan.
	cells := func(row string, plannedRaw func(i int) string) (amounts, planned []string, total vo.DecimalNumber, active bool) {
		amounts = make([]string, len(months))
		planned = make([]string, len(months))
		total = vo.NewDecimal("0")
		for i := range months {
			v := get(actualKey(row, i))
			amounts[i] = v.String()
			total = total.Add(v)
			if !v.IsZero() {
				active = true
			}
			if plannedRaw(i) != "" {
				planned[i] = get(plannedKey(row, i)).String()
				active = true
			}
		}
		return amounts, planned, total, active
	}

	var expense, income []builtReportRow
	for e, el := range plan.Structure.Elements {
		row := fmt.Sprintf("e%d", e)
		amounts, planned, total, active := cells(row, func(i int) string { return el.Cells[i].Planned })
		if !active {
			continue
		}
		children := []model.ReportChildRow{}
		for ci, ch := range el.Children {
			chAmounts := make([]string, len(months))
			chActive := false
			for i := range months {
				v := get(childKey(row, ci, i))
				chAmounts[i] = v.String()
				if !v.IsZero() {
					chActive = true
				}
			}
			if !chActive {
				continue
			}
			children = append(children, model.ReportChildRow{
				Id: ch.Id, Kind: reportElementKinds[ch.Type], Name: ch.Name, Icon: ch.Icon,
				IsArchived: ch.IsArchived, Amounts: chAmounts,
			})
		}
		r := model.ReportRow{
			Id: el.Id, Kind: reportElementKinds[el.Type], Name: el.Name, Icon: el.Icon,
			IsArchived: el.IsArchived, OwnerUserId: el.OwnerUserId,
			Amounts: amounts, Planned: planned, Children: children,
		}
		if el.Type == int(model.ElementIncomeCategory) || el.Type == int(model.ElementIncomeEnvelope) {
			income = append(income, builtReportRow{r, total})
		} else {
			expense = append(expense, builtReportRow{r, total})
		}
	}

	savings := []model.ReportRow{}
	for k, sv := range plan.Structure.Savings {
		amounts, planned, _, active := cells(fmt.Sprintf("s%d", k), func(i int) string { return sv.Cells[i].Planned })
		if !active {
			continue
		}
		var owner *string
		if sv.OwnerUserId != "" {
			owner = &sv.OwnerUserId
		}
		savings = append(savings, model.ReportRow{
			Id: sv.Id, Kind: "savings", Name: sv.Name, Icon: sv.Icon,
			IsArchived: sv.IsArchived, OwnerUserId: owner,
			Amounts: amounts, Planned: planned, Children: []model.ReportChildRow{},
		})
	}

	monthStrs := make([]string, len(months))
	in := make([]string, len(months))
	out := make([]string, len(months))
	for i, m := range months {
		monthStrs[i] = m.Format(datetime.DateLayout)
		in[i] = get(fmt.Sprintf("ti|%d", i)).String()
		out[i] = get(fmt.Sprintf("to|%d", i)).String()
	}
	return model.FlowReportResult{
		CurrencyId: currency.String(), Months: monthStrs,
		Expense: sortReportRows(expense), Income: sortReportRows(income),
		TransfersIn: in, TransfersOut: out, Savings: savings,
	}, nil
}
