package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type ReadService struct {
	read ReadModel
}

func NewReadService(read ReadModel) *ReadService {
	return &ReadService{read: read}
}

// GetLabelList returns all the user's available labels (own + shared via
// account access, archived and not) in list order, in the wire shape.
func (s *ReadService) GetLabelList(ctx context.Context, userID vo.Id) (*model.GetLabelListResult, error) {
	rows, err := s.read.LabelListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]model.LabelResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	assignPositions(items)
	return &model.GetLabelListResult{Items: items}, nil
}

// toViewResult converts a read-side row to the wire shape (int 0/1 for
// isArchived).
func toViewResult(r model.LabelViewRow) model.LabelResult {
	archived := 0
	if r.IsArchived {
		archived = 1
	}
	return model.LabelResult{
		Id:          r.ID,
		OwnerUserId: r.UserID,
		Name:        r.Name,
		Icon:        r.Icon,
		IsArchived:  archived,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
