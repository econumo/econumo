package label

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateLabel enforces name uniqueness among the owner's labels (excluding
// itself), updates the name, and returns the refreshed item; ownership failure
// is a 403.
func (s *Service) UpdateLabel(ctx context.Context, userID vo.Id, req model.UpdateLabelRequest) (*model.UpdateLabelResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	name, err := newLabelName(req.Name)
	if err != nil {
		return nil, err
	}
	l, err := s.mutateWithUnique(ctx, id, userID, name)
	if err != nil {
		return nil, err
	}
	return &model.UpdateLabelResult{Item: toResult(l)}, nil
}

// mutateWithUnique is the update variant of mutate: it additionally enforces
// name uniqueness (excluding the label being updated) inside the same tx before
// applying the name change.
func (s *Service) mutateWithUnique(ctx context.Context, id, userID vo.Id, name string) (*model.Label, error) {
	return s.mutateChecked(ctx, id, userID, func(txCtx context.Context, l *model.Label, now time.Time) error {
		if uerr := s.ensureNameUnique(txCtx, userID, name, id); uerr != nil {
			return uerr
		}
		l.UpdateName(name, now)
		return nil
	})
}
