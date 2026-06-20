package budget

import (
	"context"

	dombudget "github.com/econumo/econumo/internal/domain/budget"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// MoveElementList repositions budget elements and moves them between folders
// (canUpdate). Each item identifies an element by its external id + type. Mirrors
// BudgetService.moveElements / BudgetElementsActionsService.moveElements.
func (s *Service) MoveElementList(ctx context.Context, userID vo.Id, req MoveElementListRequest) (*MoveElementListResult, error) {
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

	// Index elements by "<externalId>-<typeAlias>".
	byKey := map[string]*dombudget.BudgetElement{}
	for _, e := range b.elements {
		byKey[elementKey(e.ExternalId().String(), e.Type())] = e
	}

	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		for _, item := range req.Items {
			typ, terr := dombudget.ElementTypeFromAlias(item.Type)
			if terr != nil {
				return terr
			}
			el := byKey[elementKey(item.Id, typ)]
			if el == nil {
				continue
			}
			var folderID *vo.Id
			if item.FolderId != nil && *item.FolderId != "" {
				fid, ferr := vo.ParseId(*item.FolderId)
				if ferr != nil {
					return validateBlank(map[string]string{"folderId": ""})
				}
				folderID = &fid
			}
			el.UpdateFolder(folderID, now)
			el.UpdatePosition(int16(item.Position), now)
			if serr := s.repo.SaveElement(txCtx, el); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &MoveElementListResult{}, nil
}
