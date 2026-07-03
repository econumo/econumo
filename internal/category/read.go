// CQRS read side of the category module. ReadService answers get-category-list
// by issuing a purpose-built read query and building the response DTO directly,
// without hydrating a domain aggregate.
//
// Writes still go through the aggregate-based Service (service.go); only the
// pure list read takes this shortcut.
package category

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// ReadModel is the read-side data source. The infra category ReadRepo implements
// it. Returning lightweight view rows (not domain entities) keeps the read path
// free of aggregate hydration.
type ReadModel interface {
	// CategoryListView returns all of the user's categories ordered by position.
	CategoryListView(ctx context.Context, userID string) ([]CategoryViewRow, error)
}

// CategoryViewRow is the read-side row shape the ReadModel returns. It is
// declared here, rather than in infra, so the app layer does not import the
// infra package (dependency points inward). Type is the raw SMALLINT; IsArchived
// the raw bool — the conversion to the wire shapes happens in toViewResult.
type CategoryViewRow struct {
	ID         string
	UserID     string
	Name       string
	Position   int16
	Type       int16
	Icon       string
	IsArchived bool
	CreatedAt  string // already formatted "2006-01-02 15:04:05" by the repo
	UpdatedAt  string
}

// ReadService serves the category read endpoint.
type ReadService struct {
	read ReadModel
}

// NewReadService wires the read service.
func NewReadService(read ReadModel) *ReadService {
	return &ReadService{read: read}
}

// GetCategoryList returns all the user's categories (archived and not) ordered
// by position, in the wire shape.
func (s *ReadService) GetCategoryList(ctx context.Context, userID vo.Id) (*GetCategoryListResult, error) {
	rows, err := s.read.CategoryListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]CategoryResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	return &GetCategoryListResult{Items: items}, nil
}

// toViewResult converts a read-side row to the wire shape (alias string for
// type, int 0/1 for isArchived). The timestamps arrive pre-formatted from the
// repo.
func toViewResult(r CategoryViewRow) CategoryResult {
	archived := 0
	if r.IsArchived {
		archived = 1
	}
	typ := Type(r.Type).Alias()
	return CategoryResult{
		Id:          r.ID,
		OwnerUserId: r.UserID,
		Name:        r.Name,
		Position:    int(r.Position),
		Type:        typ,
		Icon:        r.Icon,
		IsArchived:  archived,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
