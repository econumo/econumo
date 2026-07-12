// Sort use case: server-side ordering of the user's payees by name or by
// transaction usage over a sliding window, delegating the position writes to
// the order use case so own/shared semantics stay identical.
package payee

import (
	"context"
	"sort"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) SortPayeeList(ctx context.Context, userID vo.Id, req model.SortPayeeListRequest) (*model.SortPayeeListResult, error) {
	// Owner-only: the same set order-payee-list can move (shared payees keep
	// their positions), and the same set the settings page displays.
	payees, err := s.repo.ListByOwner(ctx, userID)
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
	sort.SliceStable(payees, func(i, j int) bool {
		a, b := payees[i], payees[j]
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
	changes := make([]model.PositionChange, len(payees))
	for i, p := range payees {
		changes[i] = model.PositionChange{Id: p.ID.String(), Position: i}
	}
	ordered, err := s.OrderPayeeList(ctx, userID, model.OrderPayeeListRequest{Changes: changes})
	if err != nil {
		return nil, err
	}
	return &model.SortPayeeListResult{Items: ordered.Items}, nil
}
