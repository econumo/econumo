package tag

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ArchiveTag marks the tag archived; ownership failure is a 403. This toggles
// only is_archived and does not touch budget-element archival.
func (s *Service) ArchiveTag(ctx context.Context, userID vo.Id, req model.ArchiveTagRequest) (*model.ArchiveTagResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(t *model.Tag, now time.Time) {
		t.Archive(now)
	}); err != nil {
		return nil, err
	}
	return &model.ArchiveTagResult{}, nil
}

// UnarchiveTag clears the archived flag; ownership failure is a 403.
func (s *Service) UnarchiveTag(ctx context.Context, userID vo.Id, req model.UnarchiveTagRequest) (*model.UnarchiveTagResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(t *model.Tag, now time.Time) {
		t.Unarchive(now)
	}); err != nil {
		return nil, err
	}
	return &model.UnarchiveTagResult{}, nil
}
