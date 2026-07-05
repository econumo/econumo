package tag

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// ReadModel is the read-side data source, implemented by the infra tag ReadRepo.
type ReadModel interface {
	// TagListView returns all of the user's tags ordered by position.
	TagListView(ctx context.Context, userID string) ([]model.TagViewRow, error)
}

type ReadService struct {
	read ReadModel
}

func NewReadService(read ReadModel) *ReadService {
	return &ReadService{read: read}
}

// GetTagList returns all the user's tags (archived and not) ordered by position,
// in the wire shape.
func (s *ReadService) GetTagList(ctx context.Context, userID vo.Id) (*model.GetTagListResult, error) {
	rows, err := s.read.TagListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]model.TagResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	return &model.GetTagListResult{Items: items}, nil
}

// toViewResult converts a read-side row to the wire shape (int 0/1 for
// isArchived).
func toViewResult(r model.TagViewRow) model.TagResult {
	archived := 0
	if r.IsArchived {
		archived = 1
	}
	return model.TagResult{
		Id:          r.ID,
		OwnerUserId: r.UserID,
		Name:        r.Name,
		Position:    int(r.Position),
		IsArchived:  archived,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
