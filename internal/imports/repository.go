package imports

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Repository interface {
	InsertSource(ctx context.Context, s *model.ImportSource) error
	GetSource(ctx context.Context, id vo.Id) (*model.ImportSource, error)
	GetSourceByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.ImportSource, error)
	ListSourcesByUser(ctx context.Context, userID vo.Id) ([]model.ImportSource, error)
	UpdateSource(ctx context.Context, s *model.ImportSource) error
	DeleteSource(ctx context.Context, id vo.Id) error

	// UpsertCredentialKey/GetCredentialKey/DeleteCredentialKey manage the
	// single wrapped data-encryption key per user; GetCredentialKey returns
	// (nil, nil) when the user has none yet.
	UpsertCredentialKey(ctx context.Context, k *model.ImportCredentialKey) error
	GetCredentialKey(ctx context.Context, userID vo.Id) (*model.ImportCredentialKey, error)
	DeleteCredentialKey(ctx context.Context, userID vo.Id) error

	InsertAccountLink(ctx context.Context, l *model.ImportAccountLink) error
	UpdateAccountLink(ctx context.Context, l *model.ImportAccountLink) error
	DeleteAccountLink(ctx context.Context, id vo.Id) error
	GetAccountLink(ctx context.Context, id vo.Id) (*model.ImportAccountLink, error)
	ListAccountLinksBySource(ctx context.Context, sourceID vo.Id) ([]model.ImportAccountLink, error)

	// InsertEvent is the inbox dedupe point: a payload already stored for the
	// source (same hash) is dropped and inserted=false is returned.
	InsertEvent(ctx context.Context, e *model.ImportEvent) (inserted bool, err error)
	GetEvent(ctx context.Context, id vo.Id) (*model.ImportEvent, error)
	UpdateEventStatus(ctx context.Context, id vo.Id, status string, parseError *string) error
	DeleteEvent(ctx context.Context, id vo.Id) error
	ListEventsBySourceStatus(ctx context.Context, sourceID vo.Id, status string) ([]model.ImportEvent, error)

	InsertRun(ctx context.Context, r *model.ImportRun) error
	GetRun(ctx context.Context, id vo.Id) (*model.ImportRun, error)
	UpdateRun(ctx context.Context, r *model.ImportRun) error
	// ListRunsByUser returns newest first; sourceID nil lists every run for
	// the user, else only runs for that source.
	ListRunsByUser(ctx context.Context, userID vo.Id, sourceID *vo.Id, limit int) ([]model.ImportRun, error)

	InsertLink(ctx context.Context, l *model.ImportTransactionLink) error
	GetLink(ctx context.Context, id vo.Id) (*model.ImportTransactionLink, error)
	UpdateLink(ctx context.Context, l *model.ImportTransactionLink) error
	GetLinkByExternalKey(ctx context.Context, sourceID vo.Id, externalAccountID, externalTransactionID string) (*model.ImportTransactionLink, error)
	ListLinksByTransaction(ctx context.Context, transactionID vo.Id) ([]model.ImportTransactionLink, error)
	ListLinksBySource(ctx context.Context, sourceID vo.Id) ([]model.ImportTransactionLink, error)
	ListLinksByRun(ctx context.Context, runID vo.Id) ([]model.ImportTransactionLink, error)
	// DeleteQueuedLinksByExternalAccount purges only queued rows: seen rows
	// (linked/skipped/tombstone) are the dedupe memory and must survive.
	DeleteQueuedLinksByExternalAccount(ctx context.Context, sourceID vo.Id, externalAccountID string) error

	// rules
	InsertRule(ctx context.Context, r *model.ImportRule) error
	UpdateRule(ctx context.Context, r *model.ImportRule) error
	DeleteRule(ctx context.Context, id vo.Id) error
	GetRule(ctx context.Context, id vo.Id) (*model.ImportRule, error)
	ListRulesByUser(ctx context.Context, userID vo.Id) ([]model.ImportRule, error)
	// applied-label snapshot on ledger rows
	ReplaceLinkAppliedLabels(ctx context.Context, linkID vo.Id, labelIDs []vo.Id) error
	ListLinkAppliedLabels(ctx context.Context, linkID vo.Id) ([]vo.Id, error)
	// scope=all for preview/apply
	ListLinksByUser(ctx context.Context, userID vo.Id) ([]model.ImportTransactionLink, error)
}
