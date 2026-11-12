package imports

import (
	"context"
	"sort"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const suggestionNotFoundMessage = "Import suggestion not found"

// GetSuggestionList is computed at read time: an edit, a rule, a deletion or
// an unmapped account clears a suggestion with no write path of its own, and
// a suggestion left with nothing to offer is deleted here.
func (s *Service) GetSuggestionList(ctx context.Context, userID vo.Id) (*model.GetImportSuggestionListResult, error) {
	out := &model.GetImportSuggestionListResult{Items: []model.ImportSuggestionResult{}}
	open, err := s.repo.ListOpenSuggestionsByUser(ctx, userID)
	if err != nil || len(open) == 0 {
		return out, err
	}
	categories, err := s.entities.CategoriesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	payees, err := s.entities.PayeesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	activeCategories, income := map[vo.Id]bool{}, map[vo.Id]bool{}
	for _, c := range categories {
		id, perr := vo.ParseId(c.ID)
		if perr != nil {
			continue
		}
		if !c.Archived {
			activeCategories[id] = true
		}
		if c.Income {
			income[id] = true
		}
	}
	ownPayees, err := idSet(payees, nil)
	if err != nil {
		return nil, err
	}
	codes := map[vo.Id]string{}
	for i := range open {
		sg := &open[i]
		if sg.CategoryID != nil && !activeCategories[*sg.CategoryID] {
			sg.CategoryID = nil
		}
		if sg.PayeeID != nil && !ownPayees[*sg.PayeeID] {
			sg.PayeeID = nil
		}
		var rows []model.ImportSuggestionTransactionResult
		if sg.CategoryID != nil || sg.PayeeID != nil {
			rows, err = s.pendingSuggestionRows(ctx, userID, sg, income, codes)
			if err != nil {
				return nil, err
			}
		}
		if len(rows) == 0 {
			if err := s.repo.DeleteSuggestion(ctx, sg.ID); err != nil {
				return nil, err
			}
			continue
		}
		out.Items = append(out.Items, model.ImportSuggestionResult{
			Id: sg.ID.String(), MatchField: sg.MatchField, MatchText: sg.MatchText,
			CategoryId: idString(sg.CategoryID), PayeeId: idString(sg.PayeeID), Transactions: rows,
		})
	}
	return out, nil
}

// pendingSuggestionRows keeps a transaction while it is live, on an account
// the caller owns, and still empty in a field the suggestion would set.
func (s *Service) pendingSuggestionRows(ctx context.Context, userID vo.Id, sg *model.ImportSuggestion, income map[vo.Id]bool, codes map[vo.Id]string) ([]model.ImportSuggestionTransactionResult, error) {
	type pending struct {
		live *model.Transaction
		link *model.ImportTransactionLink
	}
	var kept []pending
	for _, linkID := range sg.LinkIDs {
		l, live, err := s.suggestionTarget(ctx, userID, linkID)
		if err != nil {
			return nil, err
		}
		if live == nil {
			continue
		}
		wantsCategory := sg.CategoryID != nil && live.CategoryID == nil && categoryFits(*sg.CategoryID, income, live.Type)
		wantsPayee := sg.PayeeID != nil && live.PayeeID == nil
		if wantsCategory || wantsPayee {
			kept = append(kept, pending{live: live, link: l})
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i].live, kept[j].live
		if !a.SpentAt.Equal(b.SpentAt) {
			return a.SpentAt.After(b.SpentAt)
		}
		return a.ID.String() > b.ID.String()
	})
	rows := make([]model.ImportSuggestionTransactionResult, 0, len(kept))
	for _, p := range kept {
		code, ok := codes[p.live.AccountID]
		if !ok {
			var err error
			if code, err = s.accounts.AccountCurrencyCode(ctx, p.live.AccountID); err != nil {
				return nil, err
			}
			codes[p.live.AccountID] = code
		}
		rows = append(rows, model.ImportSuggestionTransactionResult{
			TransactionId: p.live.ID.String(), AccountId: p.live.AccountID.String(), Amount: p.live.Amount,
			Currency: code, Date: p.live.SpentAt.Format(datetime.Layout), ExternalPayee: p.link.ExternalPayee,
		})
	}
	return rows, nil
}

// suggestionTarget resolves a suggestion's link to its live transaction, or
// nil when the row no longer qualifies (gone, unlinked, a transfer, or on an
// account the caller does not own).
func (s *Service) suggestionTarget(ctx context.Context, userID, linkID vo.Id) (*model.ImportTransactionLink, *model.Transaction, error) {
	l, err := s.repo.GetLink(ctx, linkID)
	if isNotFoundErr(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if l.Status != model.ImportLinkStatusLinked || l.TransactionID == nil {
		return nil, nil, nil
	}
	live, err := s.lister.GetByID(ctx, *l.TransactionID)
	if isNotFoundErr(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if live.Type == model.TransactionTypeTransfer {
		return nil, nil, nil
	}
	owned, err := s.ownsAccount(ctx, userID, live.AccountID)
	if err != nil || !owned {
		return nil, nil, err
	}
	return l, live, nil
}

// ownedOpenSuggestion: another user's id, a settled one and an unknown one
// all read the same, so ids cannot be probed.
func (s *Service) ownedOpenSuggestion(ctx context.Context, userID vo.Id, rawID string) (*model.ImportSuggestion, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, errs.NewNotFound(suggestionNotFoundMessage)
	}
	sg, err := s.repo.GetSuggestion(ctx, id)
	if isNotFoundErr(err) {
		return nil, errs.NewNotFound(suggestionNotFoundMessage)
	}
	if err != nil {
		return nil, err
	}
	if sg.UserID != userID || sg.Status != model.ImportSuggestionStatusOpen {
		return nil, errs.NewNotFound(suggestionNotFoundMessage)
	}
	return sg, nil
}

// AcceptSuggestion runs after the client created a rule from the suggestion.
// It is not apply-rule: that skips any row the user touched, while here a
// row whose payee was set by hand still gets its empty category. A field the
// user set is never overwritten, and a row the saved rule no longer matches
// is left alone.
func (s *Service) AcceptSuggestion(ctx context.Context, userID vo.Id, req model.AcceptImportSuggestionRequest) (*model.AcceptImportSuggestionResult, error) {
	sg, err := s.ownedOpenSuggestion(ctx, userID, req.Id)
	if err != nil {
		return nil, err
	}
	rule, err := s.ownedRule(ctx, userID, req.RuleId)
	if err != nil {
		return nil, err
	}
	if rule.Action == model.ImportRuleActionSkip {
		return nil, &errs.ValidationError{Msg: "A skip rule cannot be applied to existing transactions", MsgCode: errs.CodeImportRuleSkipNotApplicable}
	}
	reqctx.AddLogAttr(ctx, "suggestion_id", sg.ID.String())
	reqctx.AddLogAttr(ctx, "rule_id", rule.ID.String())
	own, err := s.ownedEntities(ctx, userID)
	if err != nil {
		return nil, err
	}
	filterRuleTargets(rule, own)
	out := &model.AcceptImportSuggestionResult{}
	for _, linkID := range sg.LinkIDs {
		l, live, err := s.suggestionTarget(ctx, userID, linkID)
		if err != nil {
			return nil, err
		}
		if live == nil || (rule.SourceID != nil && *rule.SourceID != l.SourceID) || !matchRule(*rule, linkEvent(l)) {
			continue
		}
		next := model.ImportClassification{CategoryID: live.CategoryID, PayeeID: live.PayeeID, TagID: live.TagID, LabelIDs: live.LabelIDs}
		filled := false
		if live.CategoryID == nil && rule.TargetCategoryID != nil && categoryFits(*rule.TargetCategoryID, own.incomeCategories, live.Type) {
			next.CategoryID, filled = rule.TargetCategoryID, true
		}
		if live.PayeeID == nil && rule.TargetPayeeID != nil {
			next.PayeeID, filled = rule.TargetPayeeID, true
		}
		if !filled {
			continue
		}
		next.RuleID = &rule.ID
		err = s.tx.WithTx(ctx, func(ctx context.Context) error {
			if _, err := s.txns.UpdateTransaction(ctx, userID, updateRequest(live, next)); err != nil {
				return err
			}
			setApplied(l, next)
			if err := s.repo.UpdateLink(ctx, l); err != nil {
				return err
			}
			return s.repo.ReplaceLinkAppliedLabels(ctx, l.ID, next.LabelIDs)
		})
		if err != nil {
			return nil, err
		}
		out.Updated++
	}
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.SetSuggestionStatus(ctx, sg.ID, model.ImportSuggestionStatusAccepted, s.clk.Now().UTC()); err != nil {
			return err
		}
		reqctx.PinIdempotency(ctx)
		return nil
	})
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "updated", out.Updated)
	return out, nil
}

// DismissSuggestion is not remembered: a later tap from the same merchant
// may bring a fresh suggestion.
func (s *Service) DismissSuggestion(ctx context.Context, userID vo.Id, req model.DismissImportSuggestionRequest) (*model.GetImportSuggestionListResult, error) {
	sg, err := s.ownedOpenSuggestion(ctx, userID, req.Id)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "suggestion_id", sg.ID.String())
	if err := s.repo.DeleteSuggestion(ctx, sg.ID); err != nil {
		return nil, err
	}
	return s.GetSuggestionList(ctx, userID)
}
