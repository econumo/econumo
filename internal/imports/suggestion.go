package imports

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/ai"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/prompts"
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
	if s.facts != nil {
		facts, err := s.facts.OwnerFacts(ctx, userID)
		if err != nil {
			return nil, err
		}
		if facts.AIOff {
			s.dropSuggestions(ctx, userID, open)
			return out, nil
		}
		// hidden, not dropped: restoring access or the switch brings them back
		if facts.Readonly || !facts.SuggestRules {
			return out, nil
		}
	}
	vocab, err := s.activeEntities(ctx, userID)
	if err != nil {
		return nil, err
	}
	look := &suggestionLookup{codes: map[vo.Id]string{}, owned: map[vo.Id]bool{}}
	for i := range open {
		sg := &open[i]
		if sg.CategoryID != nil && !vocab.categories[*sg.CategoryID] {
			sg.CategoryID = nil
		}
		if sg.PayeeID != nil && !vocab.payees[*sg.PayeeID] {
			sg.PayeeID = nil
		}
		var rows []model.ImportSuggestionTransactionResult
		if sg.CategoryID != nil || sg.PayeeID != nil {
			rows, err = s.pendingSuggestionRows(ctx, userID, sg, vocab.income, look)
			if err != nil {
				return nil, err
			}
		}
		if len(rows) == 0 {
			// Best effort: the suggestion is hidden either way, and a later read
			// retries. Only the links read here are released, so rows a job joined
			// meanwhile keep the suggestion for the next read.
			err := s.tx.WithTx(ctx, func(ctx context.Context) error {
				_, err := s.repo.ReleaseSuggestionLinks(ctx, sg.ID, sg.LinkIDs)
				return err
			})
			if err != nil {
				slog.Warn("import suggestion cleanup failed", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
			}
			continue
		}
		out.Items = append(out.Items, model.ImportSuggestionResult{
			Id: sg.ID.String(), MatchField: sg.MatchField, MatchText: sg.MatchText, Type: sg.Type.Alias(),
			CategoryId: idString(sg.CategoryID), PayeeId: idString(sg.PayeeID), Transactions: rows,
		})
	}
	return out, nil
}

// dropSuggestions deletes the open suggestions of a user who opted out of AI,
// both kinds, best effort: they are hidden either way and the next read retries.
func (s *Service) dropSuggestions(ctx context.Context, userID vo.Id, open []model.ImportSuggestion) {
	if err := s.deleteSuggestions(ctx, userID, open); err != nil {
		slog.Warn("import suggestion cleanup failed", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
	}
}

// ClearSuggestions deletes the user's open suggestions of both kinds: they
// turned AI off.
func (s *Service) ClearSuggestions(ctx context.Context, userID vo.Id) error {
	open, err := s.repo.ListOpenSuggestionsByUser(ctx, userID)
	if err != nil {
		return err
	}
	return s.deleteSuggestions(ctx, userID, open)
}

// deleteSuggestions keeps filled transaction suggestions: they are what the
// rule-suggestion list reads to tell a Fill in from a hand edit.
func (s *Service) deleteSuggestions(ctx context.Context, userID vo.Id, open []model.ImportSuggestion) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, sg := range open {
			if err := s.repo.DeleteSuggestion(ctx, sg.ID); err != nil {
				return err
			}
		}
		return s.repo.DeleteOpenTransactionSuggestionsByUser(ctx, userID)
	})
}

// activeIDs is the caller's non-archived categories and payees, as
// a suggestion may name them, plus which categories are income ones.
type activeIDs struct {
	categories, payees, income map[vo.Id]bool
}

func (s *Service) activeEntities(ctx context.Context, userID vo.Id) (activeIDs, error) {
	categories, err := s.entities.CategoriesByOwner(ctx, userID)
	if err != nil {
		return activeIDs{}, err
	}
	payees, err := s.entities.PayeesByOwner(ctx, userID)
	if err != nil {
		return activeIDs{}, err
	}
	v := activeIDs{categories: map[vo.Id]bool{}, income: map[vo.Id]bool{}}
	for _, c := range categories {
		id, perr := vo.ParseId(c.ID)
		if perr != nil {
			continue
		}
		if !c.Archived {
			v.categories[id] = true
		}
		if c.Income {
			v.income[id] = true
		}
	}
	if v.payees, err = idSet(activeNamed(payees), nil); err != nil {
		return activeIDs{}, err
	}
	return v, nil
}

// suggestionLookup caches per-account answers within one list read: many
// suggestions share a handful of accounts.
type suggestionLookup struct {
	codes map[vo.Id]string
	owned map[vo.Id]bool
}

// pendingSuggestionRows keeps a transaction while it is live, on an account
// the caller owns, still of the suggestion's type (an edit can change it) and
// still empty in a field the suggestion would set. A row filled through Fill
// in also stays while it carries every value the suggestion sets: taking the
// AI's values confirms the guess, so the rule is still worth offering. A
// filled row that carries a different value than the suggestion sets counts
// against it and goes, even with another field still empty.
func (s *Service) pendingSuggestionRows(ctx context.Context, userID vo.Id, sg *model.ImportSuggestion, income map[vo.Id]bool, look *suggestionLookup) ([]model.ImportSuggestionTransactionResult, error) {
	type pending struct {
		live *model.Transaction
		link *model.ImportTransactionLink
	}
	// agreeing rows stay only when filled; disputed ones only when not filled
	var kept, agreeing, disputed []pending
	for _, linkID := range sg.LinkIDs {
		l, live, err := s.suggestionTargetCached(ctx, userID, linkID, look.owned)
		if err != nil {
			return nil, err
		}
		if live == nil || live.Type != sg.Type {
			continue
		}
		p := pending{live: live, link: l}
		wantsCategory := sg.CategoryID != nil && live.CategoryID == nil && categoryFits(*sg.CategoryID, income, live.Type)
		wantsPayee := sg.PayeeID != nil && live.PayeeID == nil
		switch {
		case (wantsCategory || wantsPayee) && carriedAgrees(sg.CategoryID, live.CategoryID) && carriedAgrees(sg.PayeeID, live.PayeeID):
			kept = append(kept, p)
		case wantsCategory || wantsPayee:
			disputed = append(disputed, p)
		case agrees(sg.CategoryID, live.CategoryID) && agrees(sg.PayeeID, live.PayeeID):
			agreeing = append(agreeing, p)
		}
	}
	if len(agreeing)+len(disputed) > 0 {
		ids := make([]vo.Id, 0, len(agreeing)+len(disputed))
		for _, p := range append(append([]pending{}, agreeing...), disputed...) {
			ids = append(ids, p.link.ID)
		}
		filled, err := s.repo.FilledTransactionSuggestionLinks(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, p := range agreeing {
			if filled[p.link.ID] {
				kept = append(kept, p)
			}
		}
		for _, p := range disputed {
			if !filled[p.link.ID] {
				kept = append(kept, p)
			}
		}
	}
	if len(kept) == 0 {
		return nil, nil
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
		code, ok := look.codes[p.live.AccountID]
		if !ok {
			var err error
			if code, err = s.accounts.AccountCurrencyCode(ctx, p.live.AccountID); err != nil {
				return nil, err
			}
			look.codes[p.live.AccountID] = code
		}
		rows = append(rows, model.ImportSuggestionTransactionResult{
			TransactionId: p.live.ID.String(), AccountId: p.live.AccountID.String(), Amount: vo.NewDecimal(p.live.Amount).String(),
			Currency: code, Date: p.live.SpentAt.Format(datetime.Layout), ExternalPayee: p.link.ExternalPayee,
		})
	}
	return rows, nil
}

// agrees: a field the suggestion does not set agrees with any value.
func agrees(want, have *vo.Id) bool {
	return want == nil || (have != nil && *have == *want)
}

// carriedAgrees: an empty field does not disagree.
func carriedAgrees(want, have *vo.Id) bool {
	return have == nil || agrees(want, have)
}

// suggestionTarget resolves a suggestion's link to its live transaction, or
// nil when the row no longer qualifies (gone, unlinked, a transfer, or on an
// account the caller does not own).
func (s *Service) suggestionTarget(ctx context.Context, userID, linkID vo.Id) (*model.ImportTransactionLink, *model.Transaction, error) {
	return s.suggestionTargetCached(ctx, userID, linkID, nil)
}

func (s *Service) suggestionTargetCached(ctx context.Context, userID, linkID vo.Id, owned map[vo.Id]bool) (*model.ImportTransactionLink, *model.Transaction, error) {
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
	isOwner, cached := owned[live.AccountID]
	if !cached {
		if isOwner, err = s.ownsAccount(ctx, userID, live.AccountID); err != nil {
			return nil, nil, err
		}
		if owned != nil {
			owned[live.AccountID] = isOwner
		}
	}
	if !isOwner {
		return nil, nil, nil
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
	// One transaction for every row, the snapshots and the release: the first
	// row's update pins the Idempotency-Key, so a failure on a later row must
	// roll the earlier ones back too, or the retry would be a replay that can
	// never finish the rest.
	var out *model.AcceptImportSuggestionResult
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		out = &model.AcceptImportSuggestionResult{}
		// Under the row lock, read the suggestion again: a dismiss or a cleanup
		// that committed since the read above wins, and links it released are
		// not filled. Links a job joined since are left for the suggestion too
		// (the user accepted the rows they saw), so only links in both reads count.
		locked, err := s.repo.LockSuggestion(ctx, sg.ID)
		if isNotFoundErr(err) || (err == nil && (locked.UserID != userID || locked.Status != model.ImportSuggestionStatusOpen)) {
			return errs.NewNotFound(suggestionNotFoundMessage)
		}
		if err != nil {
			return err
		}
		still := make(map[vo.Id]bool, len(locked.LinkIDs))
		for _, id := range locked.LinkIDs {
			still[id] = true
		}
		var walked []vo.Id
		for _, id := range sg.LinkIDs {
			if still[id] {
				walked = append(walked, id)
			}
		}
		for _, linkID := range walked {
			l, live, err := s.suggestionTarget(ctx, userID, linkID)
			if err != nil {
				return err
			}
			if live == nil || live.Type != sg.Type || (rule.SourceID != nil && *rule.SourceID != l.SourceID) || !matchRule(*rule, linkEvent(l)) {
				continue
			}
			next := model.ImportClassification{CategoryID: live.CategoryID, PayeeID: live.PayeeID, TagID: live.TagID, LabelIDs: live.LabelIDs}
			fillCategory := live.CategoryID == nil && rule.TargetCategoryID != nil && categoryFits(*rule.TargetCategoryID, own.incomeCategories, live.Type)
			fillPayee := live.PayeeID == nil && rule.TargetPayeeID != nil
			if !fillCategory && !fillPayee {
				continue
			}
			if fillCategory {
				next.CategoryID = rule.TargetCategoryID
			}
			if fillPayee {
				next.PayeeID = rule.TargetPayeeID
			}
			next.RuleID = &rule.ID
			if _, err := s.txns.UpdateTransaction(ctx, userID, updateRequest(live, next)); err != nil {
				return err
			}
			// The snapshot moves only for the fields written here: one the user set
			// by hand must still read as edited, or a later apply-rule would
			// overwrite it.
			if fillCategory {
				l.AppliedCategoryID = next.CategoryID
			}
			if fillPayee {
				l.AppliedPayeeID = next.PayeeID
			}
			l.AppliedRuleID = next.RuleID
			if err := s.repo.UpdateLink(ctx, l); err != nil {
				return err
			}
			out.Updated++
		}
		// Only the links this accept walked: rows a job joined meanwhile keep
		// the suggestion open for them.
		if _, err := s.repo.ReleaseSuggestionLinks(ctx, sg.ID, walked); err != nil {
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
	// Only the links read here: rows a job joined meanwhile keep the
	// suggestion open for them, as accept does.
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		_, err := s.repo.ReleaseSuggestionLinks(ctx, sg.ID, sg.LinkIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	// The dismiss has committed: a failing list read must not answer an error
	// the client would take for a refusal.
	list, err := s.GetSuggestionList(ctx, userID)
	if err != nil {
		slog.Warn("import suggestion list after dismiss failed", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
		return &model.GetImportSuggestionListResult{Items: []model.ImportSuggestionResult{}}, nil
	}
	return list, nil
}

const suggestMatchTextTimeout = 10 * time.Second

// SuggestMatchText answers empty, never an error, when the AI is off or
// fails: the dialog then simply falls back to the raw payee. A text the row
// does not contain is dropped by matchTextIn, so a model cannot invent a
// rule that would never match.
func (s *Service) SuggestMatchText(ctx context.Context, userID vo.Id, req model.SuggestImportMatchTextRequest) (*model.SuggestImportMatchTextResult, error) {
	linkID, err := vo.ParseId(strings.TrimSpace(req.LinkId))
	if err != nil {
		return nil, errs.NewNotFound("Import link not found")
	}
	l, live, err := s.suggestionTarget(ctx, userID, linkID)
	if err != nil {
		return nil, err
	}
	if live == nil {
		return nil, errs.NewNotFound("Import link not found")
	}
	empty := &model.SuggestImportMatchTextResult{}
	// only the payee text ever goes to the AI; a row without one gets no help
	if s.completer == nil || s.facts == nil || aiPayee(*l) == "" {
		return empty, nil
	}
	facts, err := s.facts.OwnerFacts(ctx, userID)
	if err != nil {
		slog.Warn("import match text failed", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
		return empty, nil
	}
	if facts.AIOff || facts.Readonly {
		return empty, nil
	}
	if err := s.countAttempt(ctx, RateScopePreviewRule, userID); err != nil {
		return nil, err
	}
	field, text, ok := s.askMatchText(ctx, userID.String(), *l, facts.Language, facts.Timezone)
	if !ok {
		return empty, nil
	}
	return &model.SuggestImportMatchTextResult{MatchField: field, MatchText: text}, nil
}

// askMatchText is SuggestMatchText's AI call: the payee text, language and
// timezone go out, a match text the payee contains comes back. Any failure is
// logged and reads as no text.
func (s *Service) askMatchText(ctx context.Context, userID string, l model.ImportTransactionLink, language, timezone string) (field, text string, ok bool) {
	if language == "" {
		language = "en"
	}
	payload, err := json.Marshal(map[string]string{
		"language": language, "timezone": timezone, "payee": aiPayee(l),
	})
	if err != nil {
		return "", "", false
	}
	ctx, cancel := context.WithTimeout(ctx, suggestMatchTextTimeout)
	defer cancel()
	reply, err := s.completer.Complete(ctx, userID, prompts.MatchText, string(payload), matchTextSchema)
	var parsed struct {
		MatchText string `json:"matchText"`
	}
	if err == nil {
		if err = json.Unmarshal(replyJSON(reply), &parsed); err != nil {
			err = replyError{}
		}
	}
	if err != nil {
		slog.Warn("import match text failed", append([]any{"user_id", userID}, ai.LogAttrs(err)...)...)
		return "", "", false
	}
	return matchTextIn(l, parsed.MatchText)
}
