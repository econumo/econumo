// Sort use case: server-side ordering of the user's categories by name or by
// transaction usage over a sliding window, delegating the position writes to
// the order use case so own/shared semantics stay identical.
package category

import (
	"context"
	"sort"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) SortCategoryList(ctx context.Context, userID vo.Id, req model.SortCategoryListRequest) (*model.SortCategoryListResult, error) {
	// Owner-only: the same set order-category-list can move (shared categories
	// keep their positions), and the same set the settings page displays.
	cats, err := s.repo.ListByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	var counts map[string]int
	if req.By == "usage" {
		since := s.clock.Now().AddDate(0, -req.PeriodMonths, 0)
		counts, err = s.repo.UsageCounts(ctx, userID, since)
		if err != nil {
			return nil, err
		}
	}
	asc := req.Direction == "asc"
	sort.SliceStable(cats, func(i, j int) bool {
		a, b := cats[i], cats[j]
		if req.By == "usage" {
			ca, cb := counts[a.ID.String()], counts[b.ID.String()]
			if ca != cb {
				if asc {
					return ca < cb
				}
				return ca > cb
			}
			// usage ties break by name asc then id asc, regardless of direction
			na, nb := strings.ToLower(a.Name), strings.ToLower(b.Name)
			if na != nb {
				return na < nb
			}
			return a.ID.String() < b.ID.String()
		}
		na, nb := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if na != nb {
			if asc {
				return na < nb
			}
			return na > nb
		}
		return a.ID.String() < b.ID.String()
	})
	changes := make([]model.PositionChange, len(cats))
	for i, c := range cats {
		changes[i] = model.PositionChange{Id: c.ID.String(), Position: i}
	}
	ordered, err := s.OrderCategoryList(ctx, userID, model.OrderCategoryListRequest{Changes: changes})
	if err != nil {
		return nil, err
	}
	return &model.SortCategoryListResult{Items: ordered.Items}, nil
}
