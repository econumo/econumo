package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetTransactionList returns the budget's transactions for a category, tag, or
// envelope in a period. Exactly one of categoryId / tagId / envelopeId selects
// the mode. Requires read access.
func (s *Service) GetTransactionList(ctx context.Context, userID vo.Id, req model.BudgetTransactionListRequest) (*model.GetBudgetTransactionListResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	periodStart, err := time.Parse(datetime.DateLayout, req.PeriodStart)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"periodStart": ""})
	}
	periodStart = model.FirstOfMonth(periodStart)
	periodEnd := periodStart.AddDate(0, 1, 0)

	b, err := s.requireBudget(ctx, userID, budgetID)
	if err != nil {
		return nil, err
	}
	f, err := s.buildFilters(ctx, userID, b, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}

	cat := optID(req.CategoryId)
	tag := optID(req.TagId)
	env := optID(req.EnvelopeId)

	var rows []model.BudgetTransactionRow
	switch {
	case cat != "" && tag == "" && env == "":
		catID, perr := vo.ParseId(cat)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"categoryId": ""})
		}
		rows, err = s.read.BudgetTransactionsByCategories(ctx, []vo.Id{catID}, f.includedAccountIDs, periodStart, periodEnd)
	case tag != "" && env == "":
		tagID, perr := vo.ParseId(tag)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"tagId": ""})
		}
		var catFilter *vo.Id
		if cat != "" {
			c, cerr := vo.ParseId(cat)
			if cerr != nil {
				return nil, model.ValidateBlank(map[string]string{"categoryId": ""})
			}
			catFilter = &c
		}
		rows, err = s.read.BudgetTransactionsByTag(ctx, tagID, catFilter, f.includedAccountIDs, periodStart, periodEnd)
	case env != "" && tag == "" && cat == "":
		envID, perr := vo.ParseId(env)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"envelopeId": ""})
		}
		catIDs, cerr := s.envelopes.EnvelopeCategoryIDs(ctx, envID)
		if cerr != nil {
			return nil, cerr
		}
		rows, err = s.read.BudgetTransactionsByCategories(ctx, catIDs, f.includedAccountIDs, periodStart, periodEnd)
	default:
		return nil, &errs.ValidationError{Msg: "Validation failed", MsgCode: errs.CodeBudgetTransactionFilterRequired}
	}
	if err != nil {
		return nil, err
	}

	return s.assembleTxList(ctx, f, rows)
}

// assembleTxList resolves author/category/payee/tag names and builds the result.
func (s *Service) assembleTxList(ctx context.Context, f filters, rows []model.BudgetTransactionRow) (*model.GetBudgetTransactionListResult, error) {
	// category + tag name maps come from the filter set; payees need a lookup.
	payees, err := s.metadata.PayeesByOwners(ctx, f.userIDs)
	if err != nil {
		return nil, err
	}
	payeeByID := map[string]model.PayeeMeta{}
	for _, p := range payees {
		payeeByID[p.ID] = p
	}
	// tags map may be needed beyond the expense-only category filter; reuse f.tags.
	authorCache := map[string]model.UserResult{}

	items := make([]model.BudgetTransactionResult, 0, len(rows))
	for _, row := range rows {
		author, ok := authorCache[row.UserID]
		if !ok {
			o, oerr := s.users.GetOwner(ctx, row.UserID)
			if oerr != nil {
				return nil, oerr
			}
			author = model.UserResult{Id: o.ID, Avatar: o.Avatar, Name: o.Name}
			authorCache[row.UserID] = author
		}
		item := model.BudgetTransactionResult{
			Id:          row.ID,
			Author:      author,
			CurrencyId:  row.CurrencyID,
			Amount:      vo.NewDecimal(row.Amount).String(),
			Description: row.Description,
			SpentAt:     normalizeSpentAt(row.SpentAt),
		}
		if row.CategoryID != nil {
			if c, ok := f.categories[*row.CategoryID]; ok {
				item.Category = &model.TxCategoryResult{Id: c.ID, Name: c.Name, Icon: c.Icon}
			}
		}
		if row.PayeeID != nil {
			if p, ok := payeeByID[*row.PayeeID]; ok {
				item.Payee = &model.TxPayeeResult{Id: p.ID, Name: p.Name}
			}
		}
		if row.TagID != nil {
			if t, ok := f.tags[*row.TagID]; ok {
				item.Tag = &model.TxTagResult{Id: t.ID, Name: t.Name}
			}
		}
		items = append(items, item)
	}
	return &model.GetBudgetTransactionListResult{Items: items}, nil
}

// optID dereferences an optional id pointer to a string ("" if nil).
func optID(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// normalizeSpentAt renders the stored DATETIME as "2006-01-02 15:04:05".
func normalizeSpentAt(raw string) string {
	for _, layout := range []string{datetime.Layout, time.RFC3339, "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format(datetime.Layout)
		}
	}
	return raw
}
