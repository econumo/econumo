package tag

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// OrderTagList applies each {id, position} change to the matching tag in the
// user's AVAILABLE set (own + shared via account access), saving those that
// actually changed, then returns the full ordered list. A SHARED tag's position
// is updated too: the changes are restricted to the available id set (an id the
// user has no access to is ignored), then each is loaded via GetByID (not
// owner-scoped) and persisted.
func (s *Service) OrderTagList(ctx context.Context, userID vo.Id, req OrderTagListRequest) (*OrderTagListResult, error) {
	positions := make(map[string]int16, len(req.Changes))
	order := make([]string, 0, len(req.Changes))
	for _, ch := range req.Changes {
		id, err := vo.ParseId(ch.Id)
		if err != nil {
			return nil, err
		}
		if _, seen := positions[id.String()]; !seen {
			order = append(order, id.String())
		}
		positions[id.String()] = int16(ch.Position)
	}

	var items []TagResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		avail, err := s.read.TagListView(ctx, userID.String())
		if err != nil {
			return err
		}
		available := make(map[string]struct{}, len(avail))
		for _, r := range avail {
			available[r.ID] = struct{}{}
		}
		now := s.clock.Now()
		for _, idStr := range order {
			if _, ok := available[idStr]; !ok {
				continue // not accessible to this user — ignore
			}
			id, _ := vo.ParseId(idStr)
			t, gerr := s.repo.GetByID(ctx, id)
			if gerr != nil {
				return gerr
			}
			before := t.Position()
			t.UpdatePosition(positions[idStr], now)
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
