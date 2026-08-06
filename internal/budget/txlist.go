package budget

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetTransactionList returns the budget's transactions for a category, tag,
// envelope, or the uncategorized bucket, in a period. categoryId alone
// selects that category's UNTAGGED transactions; tagId (with an optional
// categoryId) selects that tag's transactions, narrowed to the given category
// when both are set; envelopeId alone selects the envelope's categories'
// untagged transactions. uncategorized alone selects the top-level
// uncategorized-and-untagged bucket (category_id IS NULL AND tag_id IS NULL);
// uncategorized with tagId selects that tag's uncategorized child (tag_id =
// tagId AND category_id IS NULL) - the two compose. uncategorized and
// categoryId are mutually exclusive. tagId and envelopeId are mutually
// exclusive; so are uncategorized and envelopeId - envelopes have no
// uncategorized bucket of their own, so uncategorized+envelopeId falls
// through to the same CodeBudgetTransactionFilterRequired error as any other
// unsupported combination, exactly like tagId+envelopeId. labelId selects that
// label's transactions (a many-to-many join, unlike tagId's column compare),
// narrowed by categoryId or uncategorized when either is set - the reporting-tag
// folder's child rows drill down to exactly that pair. Pairing labelId with
// another SELECTOR (tagId or envelopeId) stays rejected up front with
// CodeBudgetTransactionFilterRequired: which of the two narrows would be
// ambiguous, and silently picking one would return the wrong set.
// Requires read access.
func (s *Service) GetTransactionList(ctx context.Context, userID vo.Id, req model.BudgetTransactionListRequest) (*model.GetBudgetTransactionListResult, error) {
	if req.Uncategorized && req.CategoryId != nil && strings.TrimSpace(*req.CategoryId) != "" {
		return nil, errs.NewValidation("Validation failed", errs.FieldError{
			Key: "categoryId", Message: "This value should not be provided when uncategorized is true.", Code: errs.CodeInvalidChoice,
		})
	}
	if req.LabelId != nil && strings.TrimSpace(*req.LabelId) != "" &&
		((req.TagId != nil && strings.TrimSpace(*req.TagId) != "") ||
			(req.EnvelopeId != nil && strings.TrimSpace(*req.EnvelopeId) != "")) {
		return nil, &errs.ValidationError{Msg: "Validation failed", MsgCode: errs.CodeBudgetTransactionFilterRequired}
	}
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
	// lbl is trimmed to agree with the mutual-exclusion guard above (which
	// compares strings.TrimSpace(*req.LabelId)): a whitespace-only labelId
	// must be treated as absent by BOTH checks, else it slips past the guard
	// untrimmed and falls into an earlier switch case (e.g. categoryId's),
	// silently discarding it instead of being rejected or ignored consistently.
	lbl := strings.TrimSpace(optID(req.LabelId))

	var rows []model.BudgetTransactionRow
	switch {
	// The label cases lead the switch, narrowest first: Go takes the FIRST
	// matching case, and both the uncategorized and the categoryId case below
	// would otherwise match a label request and silently drop the label.
	case lbl != "" && cat != "":
		labelID, perr := vo.ParseId(lbl)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"labelId": ""})
		}
		categoryID, perr := vo.ParseId(cat)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"categoryId": ""})
		}
		rows, err = s.read.BudgetTransactionsByLabelAndCategory(ctx, labelID, categoryID, f.includedAccountIDs, periodStart, periodEnd)
	case lbl != "" && req.Uncategorized:
		labelID, perr := vo.ParseId(lbl)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"labelId": ""})
		}
		rows, err = s.read.BudgetTransactionsByLabelUncategorized(ctx, labelID, f.includedAccountIDs, periodStart, periodEnd)
	case lbl != "":
		labelID, perr := vo.ParseId(lbl)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"labelId": ""})
		}
		rows, err = s.read.BudgetTransactionsByLabel(ctx, labelID, f.includedAccountIDs, periodStart, periodEnd)
	case req.Uncategorized && tag == "" && env == "":
		rows, err = s.read.BudgetTransactionsUncategorized(ctx, f.includedAccountIDs, periodStart, periodEnd)
	case req.Uncategorized && tag != "" && env == "":
		tagID, perr := vo.ParseId(tag)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"tagId": ""})
		}
		rows, err = s.read.BudgetTransactionsByTag(ctx, tagID, nil, true, f.includedAccountIDs, periodStart, periodEnd)
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
		rows, err = s.read.BudgetTransactionsByTag(ctx, tagID, catFilter, false, f.includedAccountIDs, periodStart, periodEnd)
	case env != "" && tag == "" && cat == "" && !req.Uncategorized:
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
