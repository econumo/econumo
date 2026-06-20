// Order use case: apply position changes to the user's tags.
package tag

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// OrderTagList applies each {id, position} change to the matching tag owned by
// the user, saving only the ones that actually changed, then returns the full
// ordered list.
//
// Changes referencing an id the user does not own are ignored (only tags found
// among the owner's are mutated).
func (s *Service) OrderTagList(ctx context.Context, userID vo.Id, req OrderTagListRequest) (*OrderTagListResult, error) {
	// Build an id -> position map from the changes.
	positions := make(map[string]int16, len(req.Changes))
	for _, ch := range req.Changes {
		id, err := vo.ParseId(ch.Id)
		if err != nil {
			return nil, err
		}
		positions[id.String()] = int16(ch.Position)
	}

	var items []TagResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		tags, err := s.repo.ListByOwner(ctx, userID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, t := range tags {
			pos, ok := positions[t.Id().String()]
			if !ok {
				continue
			}
			before := t.Position()
			t.UpdatePosition(pos, now)
			if t.Position() != before {
				if serr := s.repo.Save(ctx, t); serr != nil {
					return serr
				}
			}
		}
		built, berr := s.listResults(ctx, userID)
		if berr != nil {
			return berr
		}
		items = built
		return nil
	}); err != nil {
		return nil, err
	}

	return &OrderTagListResult{Items: items}, nil
}
