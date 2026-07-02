package tag

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateTag enforces name uniqueness among the owner's tags (excluding itself),
// updates the name, and returns the refreshed item; ownership failure is a 403.
func (s *Service) UpdateTag(ctx context.Context, userID vo.Id, req UpdateTagRequest) (*UpdateTagResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	name, err := newTagName(req.Name)
	if err != nil {
		return nil, err
	}
	t, err := s.mutateWithUnique(ctx, id, userID, name)
	if err != nil {
		return nil, err
	}
	return &UpdateTagResult{Item: toResult(t)}, nil
}

// mutateWithUnique is the update variant of mutate: it additionally enforces
// name uniqueness (excluding the tag being updated) inside the same tx before
// applying the name change.
func (s *Service) mutateWithUnique(ctx context.Context, id, userID vo.Id, name string) (*Tag, error) {
	return s.mutateChecked(ctx, id, userID, func(txCtx context.Context, t *Tag, now time.Time) error {
		if uerr := s.ensureNameUnique(txCtx, userID, name, id); uerr != nil {
			return uerr
		}
		t.UpdateName(name, now)
		return nil
	})
}
