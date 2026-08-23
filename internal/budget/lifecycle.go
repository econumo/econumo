// Lifecycle use cases: archive-budget / unarchive-budget. Archived = hidden +
// read-only: requireNotArchived blocks every budget write except the allowlist
// (unarchive, delete, revoke, decline, accept — and archive itself, which is an
// idempotent no-op on an archived budget).
package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) requireNotArchived(b *budgetAggregate) error {
	if b.budget.IsArchived {
		return &errs.AccessDeniedError{Msg: "This budget is archived", Code: errs.CodeBudgetArchived}
	}
	return nil
}

func (s *Service) ArchiveBudget(ctx context.Context, userID vo.Id, req model.ArchiveBudgetRequest) (*model.ArchiveBudgetResult, error) {
	meta, err := s.setArchived(ctx, userID, req.Id, true)
	if err != nil {
		return nil, err
	}
	return &model.ArchiveBudgetResult{Item: meta}, nil
}

func (s *Service) UnarchiveBudget(ctx context.Context, userID vo.Id, req model.UnarchiveBudgetRequest) (*model.UnarchiveBudgetResult, error) {
	meta, err := s.setArchived(ctx, userID, req.Id, false)
	if err != nil {
		return nil, err
	}
	return &model.UnarchiveBudgetResult{Item: meta}, nil
}

func (s *Service) setArchived(ctx context.Context, userID vo.Id, rawID string, archived bool) (model.MetaResult, error) {
	budgetID, err := vo.ParseId(rawID)
	if err != nil {
		return model.MetaResult{}, model.ValidateBlank(map[string]string{"id": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return model.MetaResult{}, err
	}
	if !s.canDelete(b, userID) {
		return model.MetaResult{}, accessDenied()
	}
	now := s.clock.Now()
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if archived {
			b.budget.Archive(now)
		} else {
			b.budget.Unarchive(now)
		}
		return s.budgets.Save(txCtx, b.budget)
	}); err != nil {
		return model.MetaResult{}, err
	}
	return s.reloadMeta(ctx, budgetID)
}

// endMonth is the budget's last covered month (first-of-month), or nil when the
// budget is open-ended.
func (b *budgetAggregate) endMonth() *time.Time {
	if b.budget.EndedAt == nil {
		return nil
	}
	m := model.FirstOfMonth(*b.budget.EndedAt)
	return &m
}

// clampPeriod snaps a requested month down to the budget's end month; periods
// at or before it pass through unchanged.
func (b *budgetAggregate) clampPeriod(period time.Time) time.Time {
	if end := b.endMonth(); end != nil && period.After(*end) {
		return *end
	}
	return period
}
