// Classification-names glue: both classification kinds (budgeting "tags" and
// reporting "labels") are one concept to the user and share a single name
// namespace, so each feature's uniqueness check must see the other's names.
// Neither feature may import the other, so the crossing happens here.
package server

import (
	"context"

	applabel "github.com/econumo/econumo/internal/label"
	"github.com/econumo/econumo/internal/shared/vo"
	apptag "github.com/econumo/econumo/internal/tag"
)

type labelNamesAdapter struct{ svc *applabel.Service }

var _ apptag.LabelNames = labelNamesAdapter{}

func (a labelNamesAdapter) NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error) {
	return a.svc.NamesByOwner(ctx, ownerID)
}

type tagNamesAdapter struct{ svc *apptag.Service }

var _ applabel.TagNames = tagNamesAdapter{}

func (a tagNamesAdapter) NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error) {
	return a.svc.NamesByOwner(ctx, ownerID)
}

// WireClassificationNames cross-connects the two services so each sees the
// other's names. They are mutually dependent, so the peer cannot be a
// constructor argument on either side; BuildAPI calls this once.
func WireClassificationNames(tagSvc *apptag.Service, labelSvc *applabel.Service) {
	tagSvc.SetLabelNames(labelNamesAdapter{svc: labelSvc})
	labelSvc.SetTagNames(tagNamesAdapter{svc: tagSvc})
}
