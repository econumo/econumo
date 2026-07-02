package budget

import (
	"context"
	"sort"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateFolder adds a budget folder (canUpdate) at the end, renumbering positions.
func (s *Service) CreateFolder(ctx context.Context, userID vo.Id, req CreateBudgetFolderRequest) (*CreateBudgetFolderResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	folderID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	if err := dombudget.ValidateName("Folder", req.Name); err != nil {
		return nil, err
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	var created *dombudget.BudgetFolder
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// Insert the new folder at position 0 and renumber the existing folders
		// 1,2,3,... in their current position-ASC order, so the new folder lands at
		// the FRONT, not appended at the end.
		created = dombudget.NewBudgetFolder(folderID, budgetID, req.Name, 0, now)
		if serr := s.repo.SaveFolder(txCtx, created); serr != nil {
			return serr
		}
		existing := append([]*dombudget.BudgetFolder(nil), b.folders...)
		sort.SliceStable(existing, func(i, j int) bool { return existing[i].Position() < existing[j].Position() })
		pos := int16(0)
		for _, f := range existing {
			pos++
			if f.Position() == pos {
				continue
			}
			f.UpdatePosition(pos, now)
			if serr := s.repo.SaveFolder(txCtx, f); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &CreateBudgetFolderResult{Item: FolderResult{Id: created.Id().String(), Name: created.Name(), Position: int(created.Position())}}, nil
}

// UpdateFolder renames a budget folder (canUpdate).
func (s *Service) UpdateFolder(ctx context.Context, userID vo.Id, req UpdateBudgetFolderRequest) (*UpdateBudgetFolderResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	folderID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	if err := dombudget.ValidateName("Folder", req.Name); err != nil {
		return nil, err
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	var updated *dombudget.BudgetFolder
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		f, gerr := s.repo.GetFolder(txCtx, folderID)
		if gerr != nil {
			return gerr
		}
		f.UpdateName(req.Name, now)
		updated = f
		return s.repo.SaveFolder(txCtx, f)
	})
	if err != nil {
		return nil, err
	}
	return &UpdateBudgetFolderResult{Item: FolderResult{Id: updated.Id().String(), Name: updated.Name(), Position: int(updated.Position())}}, nil
}

// DeleteFolder removes a budget folder (canUpdate) and renumbers the rest.
func (s *Service) DeleteFolder(ctx context.Context, userID vo.Id, req DeleteFolderRequest) (*DeleteFolderResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	folderID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if derr := s.repo.DeleteFolder(txCtx, folderID); derr != nil {
			return derr
		}
		// Renumber remaining folders 0..n by their current position order.
		remaining := make([]*dombudget.BudgetFolder, 0, len(b.folders))
		for _, f := range b.folders {
			if !f.Id().Equal(folderID) {
				remaining = append(remaining, f)
			}
		}
		sort.SliceStable(remaining, func(i, j int) bool { return remaining[i].Position() < remaining[j].Position() })
		for i, f := range remaining {
			f.UpdatePosition(int16(i), now)
			if serr := s.repo.SaveFolder(txCtx, f); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &DeleteFolderResult{}, nil
}

// OrderFolderList applies new positions to budget folders (canUpdate).
func (s *Service) OrderFolderList(ctx context.Context, userID vo.Id, req OrderBudgetFolderListRequest) (*OrderBudgetFolderListResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, validateBlank(map[string]string{"budgetId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	byID := map[string]*dombudget.BudgetFolder{}
	for _, f := range b.folders {
		byID[f.Id().String()] = f
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		for _, item := range req.Items {
			f := byID[item.Id]
			if f == nil {
				continue
			}
			f.UpdatePosition(int16(item.Position), now)
			if serr := s.repo.SaveFolder(txCtx, f); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &OrderBudgetFolderListResult{}, nil
}
