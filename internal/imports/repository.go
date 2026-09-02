package imports

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Repository interface {
	InsertSource(ctx context.Context, s *model.ImportSource) error
	GetSource(ctx context.Context, id vo.Id) (*model.ImportSource, error)

	// InsertEvent is the inbox dedupe point: a payload already stored for the
	// source (same hash) is dropped and inserted=false is returned.
	InsertEvent(ctx context.Context, e *model.ImportEvent) (inserted bool, err error)
	GetEvent(ctx context.Context, id vo.Id) (*model.ImportEvent, error)
	UpdateEventStatus(ctx context.Context, id vo.Id, status string, parseError *string) error

	InsertRun(ctx context.Context, r *model.ImportRun) error
	GetRun(ctx context.Context, id vo.Id) (*model.ImportRun, error)
	UpdateRun(ctx context.Context, r *model.ImportRun) error

	InsertLink(ctx context.Context, l *model.ImportTransactionLink) error
	GetLinkByExternalKey(ctx context.Context, sourceID vo.Id, externalAccountID, externalTransactionID string) (*model.ImportTransactionLink, error)
	ListLinksByTransaction(ctx context.Context, transactionID vo.Id) ([]model.ImportTransactionLink, error)
}
