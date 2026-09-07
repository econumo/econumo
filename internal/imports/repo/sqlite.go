package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type sqliteQuerier struct{}

var _ querier = sqliteQuerier{}

func (sqliteQuerier) InsertImportSource(ctx context.Context, db backend.DBTX, p insertSourceParams) error {
	return sqlitegen.New(db).InsertImportSource(ctx, p)
}

func (sqliteQuerier) GetImportSourceByID(ctx context.Context, db backend.DBTX, id string) (sourceRow, error) {
	return sqlitegen.New(db).GetImportSourceByID(ctx, id)
}

func (sqliteQuerier) InsertImportEvent(ctx context.Context, db backend.DBTX, p insertEventParams) (int64, error) {
	return sqlitegen.New(db).InsertImportEvent(ctx, p)
}

func (sqliteQuerier) GetImportEventByID(ctx context.Context, db backend.DBTX, id string) (eventRow, error) {
	return sqlitegen.New(db).GetImportEventByID(ctx, id)
}

func (sqliteQuerier) UpdateImportEventStatus(ctx context.Context, db backend.DBTX, p updateEventParams) error {
	return sqlitegen.New(db).UpdateImportEventStatus(ctx, p)
}

func (sqliteQuerier) InsertImportRun(ctx context.Context, db backend.DBTX, p insertRunParams) error {
	return sqlitegen.New(db).InsertImportRun(ctx, p)
}

func (sqliteQuerier) GetImportRunByID(ctx context.Context, db backend.DBTX, id string) (runRow, error) {
	return sqlitegen.New(db).GetImportRunByID(ctx, id)
}

func (sqliteQuerier) UpdateImportRun(ctx context.Context, db backend.DBTX, p updateRunParams) error {
	return sqlitegen.New(db).UpdateImportRun(ctx, p)
}

func (sqliteQuerier) InsertImportTransactionLink(ctx context.Context, db backend.DBTX, p insertLinkParams) error {
	return sqlitegen.New(db).InsertImportTransactionLink(ctx, p)
}

func (sqliteQuerier) GetImportTransactionLinkByExternalKey(ctx context.Context, db backend.DBTX, p linkByExternalKeyPs) (linkRow, error) {
	return sqlitegen.New(db).GetImportTransactionLinkByExternalKey(ctx, p)
}

func (sqliteQuerier) ListImportTransactionLinksByTransaction(ctx context.Context, db backend.DBTX, transactionID *string) ([]linkRow, error) {
	return sqlitegen.New(db).ListImportTransactionLinksByTransaction(ctx, transactionID)
}

func (sqliteQuerier) GetImportSourceByUserProvider(ctx context.Context, db backend.DBTX, p sourceByUserProviderPs) (sourceRow, error) {
	return sqlitegen.New(db).GetImportSourceByUserProvider(ctx, p)
}

func (sqliteQuerier) ListImportSourcesByUser(ctx context.Context, db backend.DBTX, userID string) ([]sourceRow, error) {
	return sqlitegen.New(db).ListImportSourcesByUser(ctx, userID)
}

func (sqliteQuerier) DeleteImportSource(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteImportSource(ctx, id)
}

func (sqliteQuerier) InsertImportAccountLink(ctx context.Context, db backend.DBTX, p insertAccountLinkParams) error {
	return sqlitegen.New(db).InsertImportAccountLink(ctx, p)
}

func (sqliteQuerier) UpdateImportAccountLink(ctx context.Context, db backend.DBTX, p updateAccountLinkParams) error {
	return sqlitegen.New(db).UpdateImportAccountLink(ctx, p)
}

func (sqliteQuerier) DeleteImportAccountLink(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteImportAccountLink(ctx, id)
}

func (sqliteQuerier) GetImportAccountLinkByID(ctx context.Context, db backend.DBTX, id string) (accountLinkRow, error) {
	return sqlitegen.New(db).GetImportAccountLinkByID(ctx, id)
}

func (sqliteQuerier) ListImportAccountLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]accountLinkRow, error) {
	return sqlitegen.New(db).ListImportAccountLinksBySource(ctx, sourceID)
}

func (sqliteQuerier) DeleteImportEvent(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteImportEvent(ctx, id)
}

func (sqliteQuerier) ListImportEventsBySourceStatus(ctx context.Context, db backend.DBTX, p eventsBySourceStatusPs) ([]eventRow, error) {
	return sqlitegen.New(db).ListImportEventsBySourceStatus(ctx, p)
}

func (sqliteQuerier) GetImportTransactionLinkByID(ctx context.Context, db backend.DBTX, id string) (linkRow, error) {
	return sqlitegen.New(db).GetImportTransactionLinkByID(ctx, id)
}

func (sqliteQuerier) UpdateImportTransactionLink(ctx context.Context, db backend.DBTX, p updateLinkParams) error {
	return sqlitegen.New(db).UpdateImportTransactionLink(ctx, p)
}

func (sqliteQuerier) ListImportTransactionLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]linkRow, error) {
	return sqlitegen.New(db).ListImportTransactionLinksBySource(ctx, sourceID)
}

func (sqliteQuerier) DeleteQueuedImportTransactionLinksByExternalAccount(ctx context.Context, db backend.DBTX, p purgeQueuedLinksParams) error {
	return sqlitegen.New(db).DeleteQueuedImportTransactionLinksByExternalAccount(ctx, p)
}
