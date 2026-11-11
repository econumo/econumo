// Update use case: change a category's name and icon.
package category

import (
	"time"

	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateCategory loads the category, checks ownership (403 otherwise), updates
// the name + icon, and returns the refreshed item.
func (s *Service) UpdateCategory(ctx context.Context, userID vo.Id, req model.UpdateCategoryRequest) (*model.UpdateCategoryResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	name, err := newCategoryName(req.Name)
	if err != nil {
		return nil, err
	}
	icon, err := newIcon(req.Icon)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(c *model.Category, now time.Time) {
		c.UpdateName(name, now)
		c.UpdateIcon(icon, now)
	}); err != nil {
		return nil, err
	}
	// Empty body ({"data":{}}); the entity is not echoed (frozen wire shape).
	return &model.UpdateCategoryResult{}, nil
}

// ReplaceIcon sets the icon of a category the caller owns, but only while it
// still shows from: one conditional write, so a change the user made in the
// meantime wins and a failed compare writes nothing. Reports whether the icon
// changed.
func (s *Service) ReplaceIcon(ctx context.Context, userID, id vo.Id, from, to string) (bool, error) {
	icon, err := newIcon(to)
	if err != nil {
		return false, err
	}
	if icon == from {
		return false, nil
	}
	return s.repo.ReplaceIcon(ctx, id, userID, from, icon, s.clock.Now())
}
