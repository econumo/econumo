package payee

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// ReadModel is the read-side data source, implemented by the infra payee
// ReadRepo.
type ReadModel interface {
	// PayeeListView returns all of the user's payees ordered by position.
	PayeeListView(ctx context.Context, userID string) ([]PayeeViewRow, error)
}

// PayeeViewRow is the read-side row shape the ReadModel returns. It is declared
// here, rather than in infra, so the app layer does not import the infra package
// (dependency points inward).
type PayeeViewRow struct {
	ID         string
	UserID     string
	Name       string
	Position   int16
	IsArchived bool
	CreatedAt  string // already formatted "2006-01-02 15:04:05" by the repo
	UpdatedAt  string
}

type ReadService struct {
	read ReadModel
}

func NewReadService(read ReadModel) *ReadService {
	return &ReadService{read: read}
}

// GetPayeeList returns all the user's payees (archived and not) ordered by
// position, in the wire shape.
func (s *ReadService) GetPayeeList(ctx context.Context, userID vo.Id) (*GetPayeeListResult, error) {
	rows, err := s.read.PayeeListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]PayeeResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	return &GetPayeeListResult{Items: items}, nil
}

// toViewResult converts a read-side row to the wire shape (int 0/1 for
// isArchived).
func toViewResult(r PayeeViewRow) PayeeResult {
	archived := 0
	if r.IsArchived {
		archived = 1
	}
	return PayeeResult{
		Id:          r.ID,
		OwnerUserId: r.UserID,
		Name:        r.Name,
		Position:    int(r.Position),
		IsArchived:  archived,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
