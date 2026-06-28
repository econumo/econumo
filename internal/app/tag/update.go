// Update use case: change a tag's name.
package tag

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtag "github.com/econumo/econumo/internal/domain/tag"
)

// UpdateTag loads the tag, checks ownership (403 otherwise), enforces name
// uniqueness among the owner's tags (excluding itself), updates the name, and
// returns the refreshed item.
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
func (s *Service) mutateWithUnique(ctx context.Context, id, userID vo.Id, name string) (*domtag.Tag, error) {
	return s.mutateChecked(ctx, id, userID, func(txCtx context.Context, t *domtag.Tag, now time.Time) error {
		if uerr := s.ensureNameUnique(txCtx, userID, name, id); uerr != nil {
			return uerr
		}
		t.UpdateName(name, now)
		return nil
	})
}
