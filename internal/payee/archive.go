package payee

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// ArchivePayee marks the payee archived; ownership failure is a 403.
func (s *Service) ArchivePayee(ctx context.Context, userID vo.Id, req ArchivePayeeRequest) (*ArchivePayeeResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(p *Payee, now time.Time) {
		p.Archive(now)
	}); err != nil {
		return nil, err
	}
	return &ArchivePayeeResult{}, nil
}

// UnarchivePayee clears the archived flag; ownership failure is a 403.
func (s *Service) UnarchivePayee(ctx context.Context, userID vo.Id, req UnarchivePayeeRequest) (*UnarchivePayeeResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutate(ctx, id, userID, func(p *Payee, now time.Time) {
		p.Unarchive(now)
	}); err != nil {
		return nil, err
	}
	return &UnarchivePayeeResult{}, nil
}
