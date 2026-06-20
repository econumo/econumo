// Update use case: change a category's name and icon.
package category

import (
	"time"

	"context"

	domcategory "github.com/econumo/econumo/internal/domain/category"
	"github.com/econumo/econumo/internal/domain/shared/vo"
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
	c, err := s.mutate(ctx, id, userID, func(c *domcategory.Category, now time.Time) {
		c.UpdateName(name, now)
		c.UpdateIcon(icon, now)
	})
	if err != nil {
		return nil, err
	}
	return &UpdateCategoryResult{Item: toResult(c)}, nil
}
