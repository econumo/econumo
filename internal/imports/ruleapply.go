package imports

import (
	"context"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) PreviewRule(ctx context.Context, userID vo.Id, req model.PreviewImportRuleRequest) (*model.PreviewImportRuleResult, error) {
	// A preview walks every matching link and reads each live transaction, and
	// the editors fire it from a typing debounce — so it is capped per user
	// like the other unbounded import endpoints; every request counts.
	if s.limiter != nil {
		if err := s.limiter.Allow(RateScopePreviewRule, userID.String()); err != nil {
			return nil, err
		}
		s.limiter.Fail(RateScopePreviewRule, userID.String())
	}
	r, err := s.ruleFromSpec(ctx, userID, req.ImportRuleSpec)
	if err != nil {
		return nil, err
	}
	links, err := s.scopedLinks(ctx, userID, req.Scope, req.RunId, req.ScopeSourceId)
	if err != nil {
		return nil, err
	}
	out := &model.PreviewImportRuleResult{}
	for i := range links {
		l := &links[i]
		if !matchRule(*r, linkEvent(l)) {
			continue
		}
		live, err := s.lister.GetByID(ctx, *l.TransactionID)
		if isNotFoundErr(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out.Matched++
		applied, err := s.appliedOf(ctx, l)
		if err != nil {
			return nil, err
		}
		if edited(applied, live) {
			out.AlreadyEdited++
		}
	}
	return out, nil
}

func (s *Service) ApplyRule(ctx context.Context, userID vo.Id, req model.ApplyImportRuleRequest) (*model.ApplyImportRuleResult, error) {
	rule, err := s.ownedRule(ctx, userID, req.RuleId)
	if err != nil {
		return nil, err
	}
	if rule.Action == model.ImportRuleActionSkip {
		return nil, &errs.ValidationError{Msg: "A skip rule cannot be applied to existing transactions", MsgCode: errs.CodeImportRuleSkipNotApplicable}
	}
	reqctx.AddLogAttr(ctx, "rule_id", rule.ID.String())
	// Filter targets the same way the pipeline does, so a stale id never
	// reaches the transaction service.
	own, err := s.ownedEntities(ctx, userID)
	if err != nil {
		return nil, err
	}
	filterRuleTargets(rule, own)
	links, err := s.scopedLinks(ctx, userID, req.Scope, req.RunId, req.ScopeSourceId)
	if err != nil {
		return nil, err
	}
	out := &model.ApplyImportRuleResult{}
	for i := range links {
		l := &links[i]
		if !matchRule(*rule, linkEvent(l)) {
			continue
		}
		live, err := s.lister.GetByID(ctx, *l.TransactionID)
		if isNotFoundErr(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !req.IncludeEdited {
			applied, aerr := s.appliedOf(ctx, l)
			if aerr != nil {
				return nil, aerr
			}
			if edited(applied, live) {
				out.Skipped++
				continue
			}
		}
		next := applyTargets(rule, live)
		next.RuleID = &rule.ID
		err = s.tx.WithTx(ctx, func(ctx context.Context) error {
			if _, err := s.txns.UpdateTransactionReplacingLabels(ctx, userID, updateRequest(live, next)); err != nil {
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
	reqctx.AddLogAttr(ctx, "updated", out.Updated)
	reqctx.AddLogAttr(ctx, "skipped", out.Skipped)
	return out, nil
}

// scopedLinks narrows to rows that are linked to a live transaction: a
// queued or skipped row has nothing to rewrite and a preview must not count it.
func (s *Service) scopedLinks(ctx context.Context, userID vo.Id, scope, runID, sourceID string) ([]model.ImportTransactionLink, error) {
	var links []model.ImportTransactionLink
	var err error
	switch scope {
	case model.ImportRuleScopeRun:
		id, perr := vo.ParseId(strings.TrimSpace(runID))
		if perr != nil {
			return nil, errs.NewNotFound("Import run not found")
		}
		run, gerr := s.repo.GetRun(ctx, id)
		if gerr != nil {
			return nil, gerr
		}
		if run.UserID != userID {
			return nil, errs.NewNotFound("Import run not found")
		}
		links, err = s.repo.ListLinksByRun(ctx, run.ID)
	case model.ImportRuleScopeSource:
		src, serr := s.ownedSource(ctx, userID, sourceID)
		if serr != nil {
			return nil, serr
		}
		links, err = s.repo.ListLinksBySource(ctx, src.ID)
	default:
		links, err = s.repo.ListLinksByUser(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	linked := links[:0]
	for _, l := range links {
		if l.Status == model.ImportLinkStatusLinked && l.TransactionID != nil {
			linked = append(linked, l)
		}
	}
	return linked, nil
}

func linkEvent(l *model.ImportTransactionLink) model.IngestEvent {
	return model.IngestEvent{Payee: l.ExternalPayee, Description: l.ExternalDescription}
}

// A label read failure must not read as "no labels applied": that would make
// every row look edited, so apply would silently skip the whole batch and
// still answer 200.
func (s *Service) appliedOf(ctx context.Context, l *model.ImportTransactionLink) (model.ImportClassification, error) {
	labels, err := s.repo.ListLinkAppliedLabels(ctx, l.ID)
	if err != nil {
		return model.ImportClassification{}, err
	}
	return model.ImportClassification{CategoryID: l.AppliedCategoryID, PayeeID: l.AppliedPayeeID, TagID: l.AppliedTagID, LabelIDs: labels, RuleID: l.AppliedRuleID}, nil
}

// edited is the "already edited" test: any classification field the user
// changed since the import means a rule must not silently overwrite it.
func edited(applied model.ImportClassification, live *model.Transaction) bool {
	return !sameID(applied.CategoryID, live.CategoryID) || !sameID(applied.PayeeID, live.PayeeID) || !sameID(applied.TagID, live.TagID) || !sameIDs(applied.LabelIDs, live.LabelIDs)
}

// isNotFoundErr: a linked transaction the user has since deleted is simply
// not a candidate, not a failure of the whole preview/apply.
func isNotFoundErr(err error) bool {
	_, ok := errs.AsNotFound(err)
	return ok
}

func sameID(a, b *vo.Id) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameIDs(a, b []vo.Id) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[vo.Id]bool, len(a))
	for _, id := range a {
		seen[id] = true
	}
	for _, id := range b {
		if !seen[id] {
			return false
		}
	}
	return true
}

// applyTargets is what the transaction looks like after the rule: only the
// fields the rule sets change; labels union, capped.
func applyTargets(r *model.ImportRule, live *model.Transaction) model.ImportClassification {
	next := model.ImportClassification{CategoryID: live.CategoryID, PayeeID: live.PayeeID, TagID: live.TagID, LabelIDs: append([]vo.Id(nil), live.LabelIDs...)}
	if r.TargetCategoryID != nil {
		next.CategoryID = r.TargetCategoryID
	}
	if r.TargetPayeeID != nil {
		next.PayeeID = r.TargetPayeeID
	}
	if r.TargetTagID != nil {
		next.TagID = r.TargetTagID
	}
	seen := make(map[vo.Id]bool, len(next.LabelIDs))
	for _, id := range next.LabelIDs {
		seen[id] = true
	}
	for _, id := range r.LabelIDs {
		if !seen[id] && len(next.LabelIDs) < model.MaxImportRuleLabels {
			seen[id] = true
			next.LabelIDs = append(next.LabelIDs, id)
		}
	}
	return next
}

func updateRequest(live *model.Transaction, next model.ImportClassification) model.UpdateTransactionRequest {
	req := model.UpdateTransactionRequest{
		Id: live.ID.String(), Type: live.Type.Alias(), AccountId: live.AccountID.String(), Amount: vo.NewFlexString(live.Amount),
		Date: live.SpentAt.Format(datetime.Layout), Description: optionalString(live.Description),
		CategoryId: idString2(next.CategoryID), PayeeId: idString2(next.PayeeID), TagId: idString2(next.TagID), LabelIds: idStrings(next.LabelIDs),
		AccountRecipientId: idString2(live.AccountRecipID),
	}
	if live.AmountRecipient != nil {
		ar := vo.NewFlexString(*live.AmountRecipient)
		req.AmountRecipient = &ar
	}
	return req
}
