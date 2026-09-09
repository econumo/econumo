package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (r *Repo) InsertRule(ctx context.Context, rule *model.ImportRule) error {
	err := r.q.InsertImportRule(ctx, r.db(ctx), insertRuleParams{
		ID: rule.ID.String(), UserID: rule.UserID.String(), SourceID: optionalID(rule.SourceID),
		Action: rule.Action, MatchField: rule.MatchField, MatchType: rule.MatchType, MatchValue: rule.MatchValue,
		IsCaseSensitive: rule.IsCaseSensitive, TargetCategoryID: optionalID(rule.TargetCategoryID),
		TargetPayeeID: optionalID(rule.TargetPayeeID), TargetTagID: optionalID(rule.TargetTagID),
		Priority: int64(rule.Priority), CreatedAt: rule.CreatedAt, UpdatedAt: rule.UpdatedAt,
	})
	if err != nil {
		return err
	}
	return r.replaceRuleLabels(ctx, rule.ID, rule.LabelIDs)
}

func (r *Repo) UpdateRule(ctx context.Context, rule *model.ImportRule) error {
	err := r.q.UpdateImportRule(ctx, r.db(ctx), updateRuleParams{
		SourceID: optionalID(rule.SourceID), Action: rule.Action, MatchField: rule.MatchField, MatchType: rule.MatchType,
		MatchValue: rule.MatchValue, IsCaseSensitive: rule.IsCaseSensitive, TargetCategoryID: optionalID(rule.TargetCategoryID),
		TargetPayeeID: optionalID(rule.TargetPayeeID), TargetTagID: optionalID(rule.TargetTagID),
		Priority: int64(rule.Priority), UpdatedAt: rule.UpdatedAt, ID: rule.ID.String(),
	})
	if err != nil {
		return err
	}
	return r.replaceRuleLabels(ctx, rule.ID, rule.LabelIDs)
}

func (r *Repo) replaceRuleLabels(ctx context.Context, ruleID vo.Id, labelIDs []vo.Id) error {
	if err := r.q.DeleteImportRuleLabels(ctx, r.db(ctx), ruleID.String()); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if err := r.q.InsertImportRuleLabel(ctx, r.db(ctx), insertRuleLabelParams{RuleID: ruleID.String(), LabelID: id.String()}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) DeleteRule(ctx context.Context, id vo.Id) error {
	return r.q.DeleteImportRule(ctx, r.db(ctx), id.String())
}

func (r *Repo) GetRule(ctx context.Context, id vo.Id) (*model.ImportRule, error) {
	row, err := r.q.GetImportRuleByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import rule not found")
		}
		return nil, err
	}
	rule, err := ruleFromRow(row)
	if err != nil {
		return nil, err
	}
	labels, err := r.q.ListImportRuleLabels(ctx, r.db(ctx), id.String())
	if err != nil {
		return nil, err
	}
	for _, l := range labels {
		lid, err := vo.ParseId(l.LabelID)
		if err != nil {
			return nil, err
		}
		rule.LabelIDs = append(rule.LabelIDs, lid)
	}
	return rule, nil
}

func (r *Repo) ListRulesByUser(ctx context.Context, userID vo.Id) ([]model.ImportRule, error) {
	rows, err := r.q.ListImportRulesByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	labelRows, err := r.q.ListImportRuleLabelsByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	labels := map[string][]vo.Id{}
	for _, l := range labelRows {
		lid, err := vo.ParseId(l.LabelID)
		if err != nil {
			return nil, err
		}
		labels[l.RuleID] = append(labels[l.RuleID], lid)
	}
	out := make([]model.ImportRule, 0, len(rows))
	for _, row := range rows {
		rule, err := ruleFromRow(row)
		if err != nil {
			return nil, err
		}
		rule.LabelIDs = labels[row.ID]
		out = append(out, *rule)
	}
	return out, nil
}

func ruleFromRow(row ruleRow) (*model.ImportRule, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	rule := &model.ImportRule{
		ID: id, UserID: userID, Action: row.Action, MatchField: row.MatchField, MatchType: row.MatchType,
		MatchValue: row.MatchValue, IsCaseSensitive: row.IsCaseSensitive, Priority: int(row.Priority),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	for _, f := range []struct {
		dst **vo.Id
		src *string
	}{{&rule.SourceID, row.SourceID}, {&rule.TargetCategoryID, row.TargetCategoryID}, {&rule.TargetPayeeID, row.TargetPayeeID}, {&rule.TargetTagID, row.TargetTagID}} {
		v, err := parseOptionalID(f.src)
		if err != nil {
			return nil, err
		}
		*f.dst = v
	}
	return rule, nil
}

func (r *Repo) ReplaceLinkAppliedLabels(ctx context.Context, linkID vo.Id, labelIDs []vo.Id) error {
	if err := r.q.DeleteImportLinkAppliedLabels(ctx, r.db(ctx), linkID.String()); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if err := r.q.InsertImportLinkAppliedLabel(ctx, r.db(ctx), insertLinkAppliedParams{LinkID: linkID.String(), LabelID: id.String()}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) ListLinkAppliedLabels(ctx context.Context, linkID vo.Id) ([]vo.Id, error) {
	rows, err := r.q.ListImportLinkAppliedLabels(ctx, r.db(ctx), linkID.String())
	if err != nil {
		return nil, err
	}
	out := make([]vo.Id, 0, len(rows))
	for _, row := range rows {
		id, err := vo.ParseId(row.LabelID)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (r *Repo) ListLinksByUser(ctx context.Context, userID vo.Id) ([]model.ImportTransactionLink, error) {
	rows, err := r.q.ListImportTransactionLinksByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportTransactionLink, 0, len(rows))
	for _, row := range rows {
		l, err := linkFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}
