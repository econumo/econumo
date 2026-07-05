// Archive / unarchive use cases: toggle the is_archived flag.
package category

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ArchiveCategory loads the category, checks ownership (403 otherwise), marks it
// archived, and returns the refreshed item.
//
// Simplification: a full archive also touches budget-element archival and
// position effects; until those modules are ported this just toggles
// is_archived, matching the entity's archive() semantics.
func (s *Service) ArchiveCategory(ctx context.Context, userID vo.Id, req model.ArchiveCategoryRequest) (*model.ArchiveCategoryResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(c *model.Category, now time.Time) {
		c.Archive(now)
	}); err != nil {
		return nil, err
	}
	return &model.ArchiveCategoryResult{}, nil
}

// UnarchiveCategory loads the category, checks ownership (403 otherwise), clears
// the archived flag, and returns the refreshed item.
func (s *Service) UnarchiveCategory(ctx context.Context, userID vo.Id, req model.UnarchiveCategoryRequest) (*model.UnarchiveCategoryResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(c *model.Category, now time.Time) {
		c.Unarchive(now)
	}); err != nil {
		return nil, err
	}
	return &model.UnarchiveCategoryResult{}, nil
}
