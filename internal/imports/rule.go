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

const ruleNotFoundMessage = "Import rule not found"

func (s *Service) GetRuleList(ctx context.Context, userID vo.Id) (*model.GetImportRuleListResult, error) {
	rules, err := s.repo.ListRulesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &model.GetImportRuleListResult{Items: make([]model.ImportRuleResult, 0, len(rules))}
	for i := range rules {
		out.Items = append(out.Items, ruleResult(&rules[i]))
	}
	return out, nil
}

func (s *Service) CreateRule(ctx context.Context, userID vo.Id, req model.CreateImportRuleRequest) (*model.ImportRuleResult, error) {
	id, err := vo.ParseId(strings.TrimSpace(req.Id))
	if err != nil {
		return nil, errs.NewValidation("Validation failed", errs.FieldError{Key: "id", Message: "This value is not valid.", Code: errs.CodeInvalidUUID})
	}
	r, err := s.ruleFromSpec(ctx, userID, req.ImportRuleSpec)
	if err != nil {
		return nil, err
	}
	now := s.clk.Now().UTC()
	r.ID, r.UserID, r.CreatedAt, r.UpdatedAt = id, userID, now, now
	reqctx.AddLogAttr(ctx, "rule_id", id.String())
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error { return s.repo.InsertRule(ctx, r) }); err != nil {
		return nil, err
	}
	out := ruleResult(r)
	return &out, nil
}

func (s *Service) UpdateRule(ctx context.Context, userID vo.Id, req model.UpdateImportRuleRequest) (*model.ImportRuleResult, error) {
	existing, err := s.ownedRule(ctx, userID, req.Id)
	if err != nil {
		return nil, err
	}
	r, err := s.ruleFromSpec(ctx, userID, req.ImportRuleSpec)
	if err != nil {
		return nil, err
	}
	r.ID, r.UserID, r.CreatedAt, r.UpdatedAt = existing.ID, userID, existing.CreatedAt, s.clk.Now().UTC()
	reqctx.AddLogAttr(ctx, "rule_id", r.ID.String())
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error { return s.repo.UpdateRule(ctx, r) }); err != nil {
		return nil, err
	}
	out := ruleResult(r)
	return &out, nil
}

func (s *Service) DeleteRule(ctx context.Context, userID vo.Id, req model.DeleteImportRuleRequest) error {
	r, err := s.ownedRule(ctx, userID, req.Id)
	if err != nil {
		return err
	}
	reqctx.AddLogAttr(ctx, "rule_id", r.ID.String())
	return s.repo.DeleteRule(ctx, r.ID)
}

func (s *Service) ownedRule(ctx context.Context, userID vo.Id, rawID string) (*model.ImportRule, error) {
	id, err := vo.ParseId(strings.TrimSpace(rawID))
	if err != nil {
		return nil, errs.NewNotFound(ruleNotFoundMessage)
	}
	r, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.UserID != userID {
		return nil, errs.NewNotFound(ruleNotFoundMessage)
	}
	return r, nil
}

// ruleFromSpec turns a validated spec into an entity, checking that every
// referenced id is the caller's: the source must be owned (not-found
// otherwise, like every other source lookup), and a category/payee/tag/label
// the caller does not own is a per-field validation error.
func (s *Service) ruleFromSpec(ctx context.Context, userID vo.Id, spec model.ImportRuleSpec) (*model.ImportRule, error) {
	r := &model.ImportRule{
		Action: spec.Action, MatchField: spec.MatchField, MatchType: spec.MatchType,
		MatchValue: strings.TrimSpace(spec.MatchValue), IsCaseSensitive: spec.IsCaseSensitive, Priority: spec.Priority,
	}
	if spec.SourceId != nil && strings.TrimSpace(*spec.SourceId) != "" {
		src, err := s.ownedSource(ctx, userID, *spec.SourceId)
		if err != nil {
			return nil, err
		}
		r.SourceID = &src.ID
	}
	if spec.Action == model.ImportRuleActionSkip {
		return r, nil
	}
	own, err := s.ownedEntities(ctx, userID)
	if err != nil {
		return nil, err
	}
	var fields []errs.FieldError
	pick := func(field string, raw *string, set map[vo.Id]bool) *vo.Id {
		if raw == nil || strings.TrimSpace(*raw) == "" {
			return nil
		}
		id, err := vo.ParseId(strings.TrimSpace(*raw))
		if err != nil || !set[id] {
			fields = append(fields, errs.FieldError{Key: field, Message: "This value is not valid.", Code: errs.CodeInvalidChoice})
			return nil
		}
		return &id
	}
	r.TargetCategoryID = pick("categoryId", spec.CategoryId, own.categories)
	r.TargetPayeeID = pick("payeeId", spec.PayeeId, own.payees)
	r.TargetTagID = pick("tagId", spec.TagId, own.tags)
	for _, raw := range spec.LabelIds {
		if id := pick("labelIds", &raw, own.labels); id != nil {
			r.LabelIDs = append(r.LabelIDs, *id)
		}
	}
	if len(fields) > 0 {
		return nil, errs.NewValidation("Validation failed", fields...)
	}
	return r, nil
}

func ruleResult(r *model.ImportRule) model.ImportRuleResult {
	return model.ImportRuleResult{
		Id: r.ID.String(), SourceId: idString(r.SourceID), Action: r.Action, MatchField: r.MatchField, MatchType: r.MatchType,
		MatchValue: r.MatchValue, IsCaseSensitive: r.IsCaseSensitive,
		CategoryId: idString(r.TargetCategoryID), PayeeId: idString(r.TargetPayeeID), TagId: idString(r.TargetTagID),
		LabelIds: idStrings(r.LabelIDs), Priority: r.Priority,
		CreatedAt: r.CreatedAt.Format(datetime.Layout), UpdatedAt: r.UpdatedAt.Format(datetime.Layout),
	}
}
