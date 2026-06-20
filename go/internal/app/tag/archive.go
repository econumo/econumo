// Archive / unarchive use cases: toggle the is_archived flag.
package tag

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtag "github.com/econumo/econumo/internal/domain/tag"
)

// ArchiveTag loads the tag, checks ownership (403 otherwise), marks it archived,
// and returns the refreshed item.
//
// Simplification: a full archive also touches budget-element archival; until
// the budget module is ported this just toggles is_archived, matching the
// entity's archive() semantics.
func (s *Service) ArchiveTag(ctx context.Context, userID vo.Id, req ArchiveTagRequest) (*ArchiveTagResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	t, err := s.mutate(ctx, id, userID, func(t *domtag.Tag, now time.Time) {
		t.Archive(now)
	})
	if err != nil {
		return nil, err
	}
	return &ArchiveTagResult{Item: toResult(t)}, nil
}

// UnarchiveTag loads the tag, checks ownership (403 otherwise), clears the
// archived flag, and returns the refreshed item.
func (s *Service) UnarchiveTag(ctx context.Context, userID vo.Id, req UnarchiveTagRequest) (*UnarchiveTagResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	t, err := s.mutate(ctx, id, userID, func(t *domtag.Tag, now time.Time) {
		t.Unarchive(now)
	})
	if err != nil {
		return nil, err
	}
	return &UnarchiveTagResult{Item: toResult(t)}, nil
}
