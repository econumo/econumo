// Order use case: apply position changes to the user's tags.
package tag

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// OrderTagList applies each {id, position} change to the matching tag in the
// user's AVAILABLE set (own + shared via account access), saving those that
// actually changed, then returns the full ordered list.
//
// PHP's TagService::orderTags iterates findAvailableForUserId and updates+saves
// each tag named in the changes — so a SHARED tag's position is updated too, not
// just owned tags. Mirror that: restrict to the available id set (so an id the
// user has no access to is ignored), then load each via GetByID (not owner-
// scoped) and persist.
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
		// The available set: the same own+shared view the list/order response uses.
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
				continue // not accessible to this user — ignore (PHP only touches available)
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
