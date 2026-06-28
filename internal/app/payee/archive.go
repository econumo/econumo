// Archive / unarchive use cases: toggle the is_archived flag.
package payee

import (
	"context"
	"time"

	dompayee "github.com/econumo/econumo/internal/domain/payee"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// ArchivePayee loads the payee, checks ownership (403 otherwise), marks it
// archived, and returns the refreshed item.
func (s *Service) ArchivePayee(ctx context.Context, userID vo.Id, req ArchivePayeeRequest) (*ArchivePayeeResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(p *dompayee.Payee, now time.Time) {
		p.Archive(now)
	}); err != nil {
		return nil, err
	}
	return &ArchivePayeeResult{}, nil
}

// UnarchivePayee loads the payee, checks ownership (403 otherwise), clears the
// archived flag, and returns the refreshed item.
func (s *Service) UnarchivePayee(ctx context.Context, userID vo.Id, req UnarchivePayeeRequest) (*UnarchivePayeeResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(p *dompayee.Payee, now time.Time) {
		p.Unarchive(now)
	}); err != nil {
		return nil, err
	}
	return &UnarchivePayeeResult{}, nil
}
