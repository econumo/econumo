package imports

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const transactionSuggestionNotFoundMessage = "Import transaction suggestion not found"

// fillable is what Fill in would write: a field the transaction already
// carries is never overwritten, and an archived or foreign category or payee,
// or a category of the other kind, counts as unset.
func (v activeIDs) fillable(sg *model.ImportTransactionSuggestion, live *model.Transaction) (category, payee *vo.Id) {
	if sg.CategoryID != nil && live.CategoryID == nil && v.categories[*sg.CategoryID] && categoryFits(*sg.CategoryID, v.income, live.Type) {
		category = sg.CategoryID
	}
	if sg.PayeeID != nil && live.PayeeID == nil && v.payees[*sg.PayeeID] {
		payee = sg.PayeeID
	}
	return category, payee
}

// GetTransactionSuggestionList is computed at read time, like the rule list:
// an edit, a rule, a deletion or an archived category clears a suggestion with
// no write path of its own, and one left with nothing to fill is deleted here.
func (s *Service) GetTransactionSuggestionList(ctx context.Context, userID vo.Id) (*model.GetImportTransactionSuggestionListResult, error) {
	out := &model.GetImportTransactionSuggestionListResult{Items: []model.ImportTransactionSuggestionResult{}}
	open, err := s.repo.ListOpenTransactionSuggestionsByUser(ctx, userID)
	if err != nil || len(open) == 0 {
		return out, err
	}
	if s.facts != nil {
		facts, err := s.facts.OwnerFacts(ctx, userID)
		if err != nil {
			return nil, err
		}
		if facts.AIOff {
			if err := s.repo.DeleteOpenTransactionSuggestionsByUser(ctx, userID); err != nil {
				slog.Warn("import transaction suggestion cleanup failed", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
			}
			return out, nil
		}
		// hidden, not dropped: restoring access or the switch brings them back
		if facts.Readonly || !facts.SuggestFill {
			return out, nil
		}
	}
	vocab, err := s.activeEntities(ctx, userID)
	if err != nil {
		return nil, err
	}
	type listed struct {
		item model.ImportTransactionSuggestionResult
		live *model.Transaction
	}
	var kept []listed
	var stale []vo.Id
	look := &suggestionLookup{codes: map[vo.Id]string{}, owned: map[vo.Id]bool{}}
	for i := range open {
		sg := &open[i]
		l, live, err := s.suggestionTargetCached(ctx, userID, sg.LinkID, look.owned)
		if err != nil {
			return nil, err
		}
		if live == nil || live.Type != sg.Type {
			stale = append(stale, sg.LinkID)
			continue
		}
		category, payee := vocab.fillable(sg, live)
		if category == nil && payee == nil {
			stale = append(stale, sg.LinkID)
			continue
		}
		code, ok := look.codes[live.AccountID]
		if !ok {
			if code, err = s.accounts.AccountCurrencyCode(ctx, live.AccountID); err != nil {
				return nil, err
			}
			look.codes[live.AccountID] = code
		}
		kept = append(kept, listed{live: live, item: model.ImportTransactionSuggestionResult{
			Id: sg.LinkID.String(), TransactionId: live.ID.String(), AccountId: live.AccountID.String(), Type: live.Type.Alias(),
			Amount: vo.NewDecimal(live.Amount).String(), Currency: code, Date: live.SpentAt.Format(datetime.Layout),
			Description: live.Description, ExternalPayee: l.ExternalPayee, CategoryId: idString2(category), PayeeId: idString2(payee),
			TransactionCategoryId: idString2(live.CategoryID), TransactionPayeeId: idString2(live.PayeeID),
		}})
	}
	if len(stale) > 0 {
		// Best effort: the rows are hidden either way and a later read retries.
		// Open rows only, so a Fill in that landed since this read keeps its row.
		if err := s.repo.DeleteTransactionSuggestionsByLinks(ctx, stale); err != nil {
			slog.Warn("import transaction suggestion cleanup failed", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i].live, kept[j].live
		if !a.SpentAt.Equal(b.SpentAt) {
			return a.SpentAt.After(b.SpentAt)
		}
		return a.ID.String() > b.ID.String()
	})
	for _, k := range kept {
		out.Items = append(out.Items, k.item)
	}
	return out, nil
}

// lockOwnedOpenTransactionSuggestion: another user's id, a filled one and an
// unknown one all read the same, so ids cannot be probed.
func (s *Service) lockOwnedOpenTransactionSuggestion(ctx context.Context, userID, linkID vo.Id) (*model.ImportTransactionSuggestion, error) {
	sg, err := s.repo.LockTransactionSuggestion(ctx, linkID)
	if isNotFoundErr(err) {
		return nil, errs.NewNotFound(transactionSuggestionNotFoundMessage)
	}
	if err != nil {
		return nil, err
	}
	if sg.UserID != userID || sg.Status != model.ImportTransactionSuggestionStatusOpen {
		return nil, errs.NewNotFound(transactionSuggestionNotFoundMessage)
	}
	return sg, nil
}

// FillTransactionSuggestion writes the suggestion's category and payee into
// the transaction's empty fields. Unlike AcceptSuggestion it leaves the
// link's applied snapshot alone: values taken with Fill in are the user's
// choice, so apply-rule treats them as a hand edit.
func (s *Service) FillTransactionSuggestion(ctx context.Context, userID vo.Id, req model.FillImportTransactionSuggestionRequest) (*model.FillImportTransactionSuggestionResult, error) {
	linkID, err := vo.ParseId(strings.TrimSpace(req.Id))
	if err != nil {
		return nil, errs.NewNotFound(transactionSuggestionNotFoundMessage)
	}
	reqctx.AddLogAttr(ctx, "link_id", linkID.String())
	var out *model.FillImportTransactionSuggestionResult
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		out = &model.FillImportTransactionSuggestionResult{}
		sg, err := s.lockOwnedOpenTransactionSuggestion(ctx, userID, linkID)
		if err != nil {
			return err
		}
		// The write below sends the whole transaction back: locked before the
		// read, an edit committed meanwhile is read rather than reverted.
		if err := s.lockSuggestionTransaction(ctx, sg.LinkID); err != nil {
			return err
		}
		_, live, err := s.suggestionTarget(ctx, userID, sg.LinkID)
		if err != nil {
			return err
		}
		// read after the locks, so a category or payee archived meanwhile is not written
		vocab, err := s.activeEntities(ctx, userID)
		if err != nil {
			return err
		}
		var category, payee *vo.Id
		if live != nil && live.Type == sg.Type {
			category, payee = vocab.fillable(sg, live)
		}
		if category == nil && payee == nil {
			if err := s.repo.DeleteTransactionSuggestion(ctx, sg.LinkID); err != nil {
				return err
			}
			reqctx.PinIdempotency(ctx)
			return nil
		}
		next := model.ImportClassification{CategoryID: live.CategoryID, PayeeID: live.PayeeID, TagID: live.TagID, LabelIDs: live.LabelIDs}
		if category != nil {
			next.CategoryID = category
		}
		if payee != nil {
			next.PayeeID = payee
		}
		if _, err := s.txns.UpdateTransaction(ctx, userID, updateRequest(live, next)); err != nil {
			return err
		}
		if err := s.repo.MarkTransactionSuggestionFilled(ctx, sg.LinkID, s.clk.Now()); err != nil {
			return err
		}
		out.Updated = 1
		reqctx.PinIdempotency(ctx)
		return nil
	})
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "updated", out.Updated)
	return out, nil
}

func (s *Service) lockSuggestionTransaction(ctx context.Context, linkID vo.Id) error {
	l, err := s.repo.GetLink(ctx, linkID)
	if isNotFoundErr(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if l.TransactionID == nil {
		return nil
	}
	return s.repo.LockTransactionRow(ctx, *l.TransactionID)
}

// DismissTransactionSuggestion deletes the suggestion: its link qualified
// only at its live import, so it is never suggested again.
func (s *Service) DismissTransactionSuggestion(ctx context.Context, userID vo.Id, req model.DismissImportTransactionSuggestionRequest) (*model.GetImportTransactionSuggestionListResult, error) {
	linkID, err := vo.ParseId(strings.TrimSpace(req.Id))
	if err != nil {
		return nil, errs.NewNotFound(transactionSuggestionNotFoundMessage)
	}
	reqctx.AddLogAttr(ctx, "link_id", linkID.String())
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		sg, err := s.lockOwnedOpenTransactionSuggestion(ctx, userID, linkID)
		if err != nil {
			return err
		}
		return s.repo.DeleteTransactionSuggestion(ctx, sg.LinkID)
	})
	if err != nil {
		return nil, err
	}
	// The dismiss has committed: a failing list read must not answer an error
	// the client would take for a refusal.
	list, err := s.GetTransactionSuggestionList(ctx, userID)
	if err != nil {
		slog.Warn("import transaction suggestion list after dismiss failed", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
		return &model.GetImportTransactionSuggestionListResult{Items: []model.ImportTransactionSuggestionResult{}}, nil
	}
	return list, nil
}
