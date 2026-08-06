package label

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ArchiveLabel marks the label archived; ownership failure is a masked
// not-found (400), so a caller cannot probe which label ids exist. This
// toggles only is_archived and does not touch budget-element archival (labels
// have no budget role at all).
func (s *Service) ArchiveLabel(ctx context.Context, userID vo.Id, req model.ArchiveLabelRequest) (*model.ArchiveLabelResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(l *model.Label, now time.Time) {
		l.Archive(now)
	}); err != nil {
		return nil, err
	}
	return &model.ArchiveLabelResult{}, nil
}

// UnarchiveLabel clears the archived flag; ownership failure is a masked
// not-found (400), so a caller cannot probe which label ids exist.
func (s *Service) UnarchiveLabel(ctx context.Context, userID vo.Id, req model.UnarchiveLabelRequest) (*model.UnarchiveLabelResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(l *model.Label, now time.Time) {
		l.Unarchive(now)
	}); err != nil {
		return nil, err
	}
	return &model.UnarchiveLabelResult{}, nil
}
