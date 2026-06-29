package tag

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtag "github.com/econumo/econumo/internal/domain/tag"
)

// ArchiveTag marks the tag archived; ownership failure is a 403. This toggles
// only is_archived and does not touch budget-element archival.
func (s *Service) ArchiveTag(ctx context.Context, userID vo.Id, req ArchiveTagRequest) (*ArchiveTagResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(t *domtag.Tag, now time.Time) {
		t.Archive(now)
	}); err != nil {
		return nil, err
	}
	return &ArchiveTagResult{}, nil
}

// UnarchiveTag clears the archived flag; ownership failure is a 403.
func (s *Service) UnarchiveTag(ctx context.Context, userID vo.Id, req UnarchiveTagRequest) (*UnarchiveTagResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(t *domtag.Tag, now time.Time) {
		t.Unarchive(now)
	}); err != nil {
		return nil, err
	}
	return &UnarchiveTagResult{}, nil
}
