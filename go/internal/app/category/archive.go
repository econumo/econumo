// Archive / unarchive use cases: toggle the is_archived flag.
package category

import (
	"context"
	"time"

	domcategory "github.com/econumo/econumo/internal/domain/category"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// ArchiveCategory loads the category, checks ownership (403 otherwise), marks it
// archived, and returns the refreshed item.
//
// Simplification: a full archive also touches budget-element archival and
// position effects; until those modules are ported this just toggles
// is_archived, matching the entity's archive() semantics. See README.
func (s *Service) ArchiveCategory(ctx context.Context, userID vo.Id, req ArchiveCategoryRequest) (*ArchiveCategoryResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	c, err := s.mutate(ctx, id, userID, func(c *domcategory.Category, now time.Time) {
		c.Archive(now)
	})
	if err != nil {
		return nil, err
	}
	return &ArchiveCategoryResult{Item: toResult(c)}, nil
}

// UnarchiveCategory loads the category, checks ownership (403 otherwise), clears
// the archived flag, and returns the refreshed item.
func (s *Service) UnarchiveCategory(ctx context.Context, userID vo.Id, req UnarchiveCategoryRequest) (*UnarchiveCategoryResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	c, err := s.mutate(ctx, id, userID, func(c *domcategory.Category, now time.Time) {
		c.Unarchive(now)
	})
	if err != nil {
		return nil, err
	}
	return &UnarchiveCategoryResult{Item: toResult(c)}, nil
}
