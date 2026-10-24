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
// ambiguous, and silently picking one would return the wrong set. transfers
// selects the transfers that crossed the budget boundary (the plan sheet's
// Transfers line) and composes with nothing: any other selector alongside it
// is rejected the same way. income turns exactly one of categoryId/envelopeId
// into an income row's list (income on the everyday accounts, tagged or not);
// accountId selects every transaction on one account of the budget (currently
// only its savings accounts: see savingsMemberID). Both compose with nothing
// else.
// Requires read access.
func (s *Service) GetTransactionList(ctx context.Context, userID vo.Id, req model.BudgetTransactionListRequest) (*model.GetBudgetTransactionListResult, error) {
	if req.Transfers && (req.Uncategorized || optIDSet(req.CategoryId) || optIDSet(req.TagId) || optIDSet(req.EnvelopeId) || optIDSet(req.LabelId)) {
		return nil, &errs.ValidationError{Msg: "Validation failed", MsgCode: errs.CodeBudgetTransactionFilterRequired}
	}
	if optIDSet(req.AccountId) && (req.Transfers || req.Income || req.Uncategorized || optIDSet(req.CategoryId) || optIDSet(req.TagId) || optIDSet(req.EnvelopeId) || optIDSet(req.LabelId)) {
		return nil, &errs.ValidationError{Msg: "Validation failed", MsgCode: errs.CodeBudgetTransactionFilterRequired}
	}
	if req.Income && (req.Transfers || req.Uncategorized || optIDSet(req.TagId) || optIDSet(req.LabelId) || optIDSet(req.CategoryId) == optIDSet(req.EnvelopeId)) {
		return nil, &errs.ValidationError{Msg: "Validation failed", MsgCode: errs.CodeBudgetTransactionFilterRequired}
	}
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
	if clamped := b.clampPeriod(periodStart); !clamped.Equal(periodStart) {
		periodStart = clamped
		periodEnd = periodStart.AddDate(0, 1, 0)
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

	query, err := s.txListQuery(ctx, f, req, cat, tag, env, lbl, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	rows, err := query(s.read)
	if err != nil {
		return nil, err
	}
	res, err := s.assembleTxList(ctx, f, rows)
	if err != nil {
		return nil, err
	}
	if !upcomingShown(ctx, periodEnd, s.clock.Now()) {
		return res, nil
	}
	pr, err := s.newProjectedRead(ctx, f, periodEnd)
	if err != nil || pr == nil {
		return res, err
	}
	upRows, err := query(pr)
	if err != nil {
		return nil, err
	}
	if res.Upcoming, err = s.assembleUpcoming(ctx, userID, f, pr, upRows); err != nil {
		return nil, err
	}
	return res, nil
}

// txListQuery is a selector bound to its parsed ids: the same list read runs
// against the stored transactions and against the projected occurrences.
type txListQuery func(r ReadModel) ([]model.BudgetTransactionRow, error)

func (s *Service) txListQuery(ctx context.Context, f filters, req model.BudgetTransactionListRequest, cat, tag, env, lbl string, periodStart, periodEnd time.Time) (txListQuery, error) {
	switch {
	case optIDSet(req.AccountId):
		accountID, perr := savingsMemberID(f, strings.TrimSpace(*req.AccountId))
		if perr != nil {
			return nil, perr
		}
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsOnAccount(ctx, accountID, periodStart, periodEnd)
		}, nil
	case req.Income:
		var catIDs []vo.Id
		if cat != "" {
			catID, perr := vo.ParseId(cat)
			if perr != nil {
				return nil, model.ValidateBlank(map[string]string{"categoryId": ""})
			}
			catIDs = []vo.Id{catID}
		} else {
			envID, perr := vo.ParseId(env)
			if perr != nil {
				return nil, model.ValidateBlank(map[string]string{"envelopeId": ""})
			}
			var err error
			if catIDs, err = s.envelopes.EnvelopeCategoryIDs(ctx, envID); err != nil {
				return nil, err
			}
		}
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsIncome(ctx, catIDs, f.everydayAccountIDs, periodStart, periodEnd)
		}, nil
	// transfers composes with nothing (guarded above), so it needs no
	// narrowing case of its own.
	case req.Transfers:
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsTransfers(ctx, f.includedAccountIDs, periodStart, periodEnd)
		}, nil
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
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsByLabelAndCategory(ctx, labelID, categoryID, f.everydayAccountIDs, periodStart, periodEnd)
		}, nil
	case lbl != "" && req.Uncategorized:
		labelID, perr := vo.ParseId(lbl)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"labelId": ""})
		}
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsByLabelUncategorized(ctx, labelID, f.everydayAccountIDs, periodStart, periodEnd)
		}, nil
	case lbl != "":
		labelID, perr := vo.ParseId(lbl)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"labelId": ""})
		}
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsByLabel(ctx, labelID, f.everydayAccountIDs, periodStart, periodEnd)
		}, nil
	case req.Uncategorized && tag == "" && env == "":
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsUncategorized(ctx, f.everydayAccountIDs, periodStart, periodEnd)
		}, nil
	case req.Uncategorized && tag != "" && env == "":
		tagID, perr := vo.ParseId(tag)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"tagId": ""})
		}
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsByTag(ctx, tagID, nil, true, f.everydayAccountIDs, periodStart, periodEnd)
		}, nil
	case cat != "" && tag == "" && env == "":
		catID, perr := vo.ParseId(cat)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"categoryId": ""})
		}
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsByCategories(ctx, []vo.Id{catID}, f.everydayAccountIDs, periodStart, periodEnd)
		}, nil
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
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsByTag(ctx, tagID, catFilter, false, f.everydayAccountIDs, periodStart, periodEnd)
		}, nil
	case env != "" && tag == "" && cat == "" && !req.Uncategorized:
		envID, perr := vo.ParseId(env)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"envelopeId": ""})
		}
		catIDs, cerr := s.envelopes.EnvelopeCategoryIDs(ctx, envID)
		if cerr != nil {
			return nil, cerr
		}
		return func(r ReadModel) ([]model.BudgetTransactionRow, error) {
			return r.BudgetTransactionsByCategories(ctx, catIDs, f.everydayAccountIDs, periodStart, periodEnd)
		}, nil
	default:
		return nil, &errs.ValidationError{Msg: "Validation failed", MsgCode: errs.CodeBudgetTransactionFilterRequired}
	}
}

// savingsMemberID parses raw and requires it to be one of the budget's savings
// accounts, so the list never reaches an account outside the budget. Widen this
// to unlock accountId for other members.
func savingsMemberID(f filters, raw string) (vo.Id, error) {
	for _, a := range f.savingsAccounts {
		if a.ID == raw {
			return vo.ParseId(raw)
		}
	}
	return vo.Id{}, model.ValidateBlank(map[string]string{"accountId": ""})
}

// txItems resolves author/category/payee/tag names into list items; one
// instance serves a whole response so the payee and author lookups run once.
type txItems struct {
	s       *Service
	f       filters
	payees  map[string]model.PayeeMeta
	authors map[string]model.UserResult
}

func (s *Service) newTxItems(ctx context.Context, f filters) (*txItems, error) {
	payees, err := s.metadata.PayeesByOwners(ctx, f.userIDs)
	if err != nil {
		return nil, err
	}
	byID := map[string]model.PayeeMeta{}
	for _, p := range payees {
		byID[p.ID] = p
	}
	return &txItems{s: s, f: f, payees: byID, authors: map[string]model.UserResult{}}, nil
}

func (b *txItems) item(ctx context.Context, row model.BudgetTransactionRow, labelIDs []string) (model.BudgetTransactionResult, error) {
	author, ok := b.authors[row.UserID]
	if !ok {
		o, err := b.s.users.GetOwner(ctx, row.UserID)
		if err != nil {
			return model.BudgetTransactionResult{}, err
		}
		author = model.UserResult{Id: o.ID, Avatar: o.Avatar, Name: o.Name}
		b.authors[row.UserID] = author
	}
	if labelIDs == nil {
		labelIDs = []string{}
	}
	item := model.BudgetTransactionResult{
		Id:          row.ID,
		Author:      author,
		CurrencyId:  row.CurrencyID,
		Amount:      vo.NewDecimal(row.Amount).String(),
		Description: row.Description,
		LabelIds:    labelIDs,
		SpentAt:     normalizeSpentAt(row.SpentAt),
		Direction:   row.Direction,
		Type:        row.Type,
	}
	if row.CategoryID != nil {
		c, ok := b.f.categories[*row.CategoryID]
		if !ok {
			c, ok = b.f.incomeCategories[*row.CategoryID]
		}
		if ok {
			item.Category = &model.TxCategoryResult{Id: c.ID, Name: c.Name, Icon: c.Icon}
		}
	}
	if row.PayeeID != nil {
		if p, ok := b.payees[*row.PayeeID]; ok {
			item.Payee = &model.TxPayeeResult{Id: p.ID, Name: p.Name}
		}
	}
	if row.TagID != nil {
		if t, ok := b.f.tags[*row.TagID]; ok {
			item.Tag = &model.TxTagResult{Id: t.ID, Name: t.Name}
		}
	}
	return item, nil
}

// assembleTxList builds the real rows' result; Upcoming starts empty.
func (s *Service) assembleTxList(ctx context.Context, f filters, rows []model.BudgetTransactionRow) (*model.GetBudgetTransactionListResult, error) {
	b, err := s.newTxItems(ctx, f)
	if err != nil {
		return nil, err
	}

	// One batch lookup for the whole page: labels are many-per-transaction, so
	// they cannot ride the row query without fanning each transaction into N
	// rows (the same reason the budget's label spend has its own query).
	labelsByTx := map[string][]string{}
	if s.txLabels != nil && len(rows) > 0 {
		txIDs := make([]vo.Id, 0, len(rows))
		for _, row := range rows {
			id, perr := vo.ParseId(row.ID)
			if perr != nil {
				continue
			}
			txIDs = append(txIDs, id)
		}
		found, lerr := s.txLabels.LabelsByTransactionIDs(ctx, txIDs)
		if lerr != nil {
			return nil, lerr
		}
		labelsByTx = found
	}

	items := make([]model.BudgetTransactionResult, 0, len(rows))
	for _, row := range rows {
		item, ierr := b.item(ctx, row, labelsByTx[row.ID])
		if ierr != nil {
			return nil, ierr
		}
		items = append(items, item)
	}
	return &model.GetBudgetTransactionListResult{Items: items, Upcoming: []model.BudgetUpcomingResult{}}, nil
}

// assembleUpcoming enriches the projected rows with their occurrence: the
// template, its account, whether it is the template's next payment, and
// whether the caller may act on it.
func (s *Service) assembleUpcoming(ctx context.Context, userID vo.Id, f filters, pr *projectedRead, rows []model.BudgetTransactionRow) ([]model.BudgetUpcomingResult, error) {
	out := make([]model.BudgetUpcomingResult, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	b, err := s.newTxItems(ctx, f)
	if err != nil {
		return nil, err
	}
	canEdit := map[string]bool{}
	members := idSet(f.includedAccountIDs)
	for _, row := range rows {
		o, ok := pr.occurrence(row.ID, row.SpentAt)
		if !ok {
			continue
		}
		item, ierr := b.item(ctx, row, append([]string{}, o.labelIDs...))
		if ierr != nil {
			return nil, ierr
		}
		edit, cached := canEdit[o.accountID]
		if !cached {
			accountID, perr := vo.ParseId(o.accountID)
			if perr != nil {
				return nil, perr
			}
			if edit, err = s.recurring.CanEdit(ctx, userID, accountID); err != nil {
				return nil, err
			}
			canEdit[o.accountID] = edit
		}
		labelNames := make([]string, 0, len(o.labelIDs))
		for _, l := range o.labelIDs {
			if meta, ok := f.labels[l]; ok {
				labelNames = append(labelNames, meta.Name)
			}
		}
		// A transfer into the budget may come from an account outside it, such
		// as a partner's private one, and one out of it may go to one: that
		// account's name is not the budget's to show.
		accountID, accountName := "", ""
		if members[o.accountID] {
			accountID, accountName = o.accountID, pr.nameOf[o.accountID]
		}
		recipientID, recipientName, amountRecipient, currencyRecipient := "", "", "", ""
		amountSource, currencySource := "", ""
		if o.recipientID != "" {
			amountRecipient, currencyRecipient = o.amountRecipient.String(), o.recipientCurrencyID
			amountSource, currencySource = o.amount.String(), o.currencyID
			if members[o.recipientID] {
				recipientID, recipientName = o.recipientID, pr.nameOf[o.recipientID]
			}
		}
		out = append(out, model.BudgetUpcomingResult{
			BudgetTransactionResult: item,
			TemplateId:              o.templateID,
			Schedule:                o.schedule,
			AccountId:               accountID,
			AccountName:             accountName,
			AccountRecipientId:      recipientID,
			AccountRecipientName:    recipientName,
			AmountRecipient:         amountRecipient,
			CurrencyRecipientId:     currencyRecipient,
			AmountSource:            amountSource,
			CurrencySourceId:        currencySource,
			LabelNames:              labelNames,
			IsNext:                  o.isNext,
			IsOverdue:               o.isOverdue,
			CanEdit:                 edit,
		})
	}
	return out, nil
}

// optID dereferences an optional id pointer to a string ("" if nil).
func optID(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// optIDSet reports whether an optional id carries anything but whitespace —
// the same "present" test the label exclusivity guard applies.
func optIDSet(p *string) bool {
	return strings.TrimSpace(optID(p)) != ""
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
