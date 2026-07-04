package payee

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ReadModel is the read-side data source, implemented by the infra payee
// ReadRepo.
type ReadModel interface {
	// PayeeListView returns all of the user's payees ordered by position.
	PayeeListView(ctx context.Context, userID string) ([]model.PayeeViewRow, error)
}

type ReadService struct {
	read ReadModel
}

func NewReadService(read ReadModel) *ReadService {
	return &ReadService{read: read}
}

// GetPayeeList returns all the user's payees (archived and not) ordered by
// position, in the wire shape.
func (s *ReadService) GetPayeeList(ctx context.Context, userID vo.Id) (*model.GetPayeeListResult, error) {
	rows, err := s.read.PayeeListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]model.PayeeResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	return &model.GetPayeeListResult{Items: items}, nil
}

// toViewResult converts a read-side row to the wire shape (int 0/1 for
// isArchived).
func toViewResult(r model.PayeeViewRow) model.PayeeResult {
	archived := 0
	if r.IsArchived {
		archived = 1
	}
	return model.PayeeResult{
		Id:          r.ID,
		OwnerUserId: r.UserID,
		Name:        r.Name,
		Position:    int(r.Position),
		IsArchived:  archived,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
