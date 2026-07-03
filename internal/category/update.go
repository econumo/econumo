// Update use case: change a category's name and icon.
package category

import (
	"time"

	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateCategory loads the category, checks ownership (403 otherwise), updates
// the name + icon, and returns the refreshed item.
func (s *Service) UpdateCategory(ctx context.Context, userID vo.Id, req UpdateCategoryRequest) (*UpdateCategoryResult, error) {
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
	if _, err := s.mutate(ctx, id, userID, func(c *Category, now time.Time) {
		c.UpdateName(name, now)
		c.UpdateIcon(icon, now)
	}); err != nil {
		return nil, err
	}
	// Empty body ({"data":{}}); the entity is not echoed (frozen wire shape).
	return &UpdateCategoryResult{}, nil
}
