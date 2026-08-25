// CloneBudget deep-copies a budget the caller owns: structure, sharing,
// membership and (optionally) plans, under fresh ids.
package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) CloneBudget(ctx context.Context, userID vo.Id, req model.CloneBudgetRequest) (*model.CloneBudgetResult, error) {
	sourceID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	newID, err := vo.ParseId(req.NewId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"newId": ""})
	}
	if err := model.ValidateName("Budget", req.Name); err != nil {
		return nil, err
	}

	src, err := s.loadAggregate(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	// Owner only: a copy carries the source's whole sharing set, so only the
	// owner may spawn one. An archived or ended source is still cloneable —
	// continuing from a completed budget is the point of the feature.
	role, err := s.budgetRole(src, userID)
	if err != nil {
		return nil, err
	}
	if role != model.BudgetRoleOwner {
		return nil, accessDenied()
	}

	now := s.clock.Now()
	startDate := model.FirstOfMonth(src.budget.StartedAt)
	if req.StartDate != "" {
		d, perr := time.Parse(datetime.DateLayout, req.StartDate)
		if perr != nil {
			return nil, model.ValidateBlank(map[string]string{"startDate": ""})
		}
		startDate = model.FirstOfMonth(d)
		// The copy may start anywhere between the source's start and the caller's
		// current month: earlier would invent history, later would skip months.
		if startDate.Before(model.FirstOfMonth(src.budget.StartedAt)) ||
			startDate.After(localMonth(now, reqctx.Location(ctx))) {
			return nil, model.ValidateBlank(map[string]string{"startDate": ""})
		}
	}

	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		nb := model.NewBudget(newID, userID, req.Name, src.budget.CurrencyID, startDate, now)
		if serr := s.budgets.Save(txCtx, nb); serr != nil {
			return serr
		}
		for _, a := range src.access {
			na := model.NewBudgetAccess(s.access.NextIdentity(), newID, a.UserID, a.Role, now)
			na.IsAccepted = a.IsAccepted
			if serr := s.access.SaveAccess(txCtx, na); serr != nil {
				return serr
			}
		}
		for _, m := range src.accounts {
			if serr := s.budgets.AddAccount(txCtx, newID, m.AccountID, now); serr != nil {
				return serr
			}
		}
		folderMap := make(map[vo.Id]vo.Id, len(src.folders))
		for _, f := range src.folders {
			nf := model.NewBudgetFolder(vo.NewId(), newID, f.Name, now)
			nf.SetSortKey(f.SortKey)
			if serr := s.folders.SaveFolder(txCtx, nf); serr != nil {
				return serr
			}
			folderMap[f.ID] = nf.ID
		}
		envMap := make(map[vo.Id]vo.Id, len(src.envelopes))
		for _, e := range src.envelopes {
			ne := model.NewBudgetEnvelope(vo.NewId(), newID, e.Name, e.Icon, now)
			ne.SetArchived(e.IsArchived, now)
			if serr := s.envelopes.SaveEnvelope(txCtx, ne); serr != nil {
				return serr
			}
			envMap[e.ID] = ne.ID
			catIDs, cerr := s.envelopes.EnvelopeCategoryIDs(txCtx, e.ID)
			if cerr != nil {
				return cerr
			}
			for _, cid := range catIDs {
				if serr := s.envelopes.AddEnvelopeCategory(txCtx, ne.ID, cid); serr != nil {
					return serr
				}
			}
		}
		elementMap := make(map[vo.Id]vo.Id, len(src.elements))
		for _, el := range src.elements {
			var folderID *vo.Id
			if el.FolderID != nil {
				if mapped, ok := folderMap[*el.FolderID]; ok {
					f := mapped
					folderID = &f
				}
			}
			// A category/tag element's external_id names a global entity and is
			// kept as-is; an envelope element's names this budget's own envelope
			// row, which the clone re-created under a fresh id.
			externalID := el.ExternalID
			if el.Type == model.ElementEnvelope || el.Type == model.ElementIncomeEnvelope {
				if mapped, ok := envMap[el.ExternalID]; ok {
					externalID = mapped
				}
			}
			ne := model.NewBudgetElement(s.elements.NextIdentity(), newID, externalID, el.Type, el.CurrencyID, folderID, now)
			ne.SetSortKey(el.SortKey)
			if serr := s.elements.SaveElement(txCtx, ne); serr != nil {
				return serr
			}
			elementMap[el.ID] = ne.ID
		}
		if req.WithLimits {
			limits, lerr := s.limits.ListLimitsFrom(txCtx, sourceID, startDate)
			if lerr != nil {
				return lerr
			}
			for _, l := range limits {
				mapped, ok := elementMap[l.ElementID]
				if !ok {
					continue
				}
				nl := model.NewBudgetElementLimit(s.limits.NextIdentity(), mapped, l.Amount, l.Period, now)
				if serr := s.limits.SaveLimit(txCtx, nl); serr != nil {
					return serr
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	b, err := s.loadAggregate(ctx, newID)
	if err != nil {
		return nil, err
	}
	result, err := s.BuildBudget(ctx, userID, b, localMonth(now, reqctx.Location(ctx)), now)
	if err != nil {
		return nil, err
	}
	return &model.CloneBudgetResult{Item: result}, nil
}
