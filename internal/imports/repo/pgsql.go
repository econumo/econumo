package repo

import (
	"context"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) InsertImportSource(ctx context.Context, db backend.DBTX, p insertSourceParams) error {
	return pgsqlgen.New(db).InsertImportSource(ctx, pgsqlgen.InsertImportSourceParams(p))
}

func (pgsqlQuerier) GetImportSourceByID(ctx context.Context, db backend.DBTX, id string) (sourceRow, error) {
	s, err := pgsqlgen.New(db).GetImportSourceByID(ctx, id)
	return sourceRow(s), err
}

func (pgsqlQuerier) InsertImportEvent(ctx context.Context, db backend.DBTX, p insertEventParams) (int64, error) {
	return pgsqlgen.New(db).InsertImportEvent(ctx, pgsqlgen.InsertImportEventParams(p))
}

func (pgsqlQuerier) GetImportEventByID(ctx context.Context, db backend.DBTX, id string) (eventRow, error) {
	e, err := pgsqlgen.New(db).GetImportEventByID(ctx, id)
	return eventRow(e), err
}

func (pgsqlQuerier) UpdateImportEventStatus(ctx context.Context, db backend.DBTX, p updateEventParams) error {
	return pgsqlgen.New(db).UpdateImportEventStatus(ctx, pgsqlgen.UpdateImportEventStatusParams(p))
}

func (pgsqlQuerier) InsertImportRun(ctx context.Context, db backend.DBTX, p insertRunParams) error {
	return pgsqlgen.New(db).InsertImportRun(ctx, pgsqlgen.InsertImportRunParams(p))
}

func (pgsqlQuerier) GetImportRunByID(ctx context.Context, db backend.DBTX, id string) (runRow, error) {
	r, err := pgsqlgen.New(db).GetImportRunByID(ctx, id)
	return runRow(r), err
}

func (pgsqlQuerier) UpdateImportRun(ctx context.Context, db backend.DBTX, p updateRunParams) error {
	return pgsqlgen.New(db).UpdateImportRun(ctx, pgsqlgen.UpdateImportRunParams(p))
}

func (pgsqlQuerier) InsertImportTransactionLink(ctx context.Context, db backend.DBTX, p insertLinkParams) error {
	return pgsqlgen.New(db).InsertImportTransactionLink(ctx, pgsqlgen.InsertImportTransactionLinkParams(p))
}

func (pgsqlQuerier) GetImportTransactionLinkByExternalKey(ctx context.Context, db backend.DBTX, p linkByExternalKeyPs) (linkRow, error) {
	l, err := pgsqlgen.New(db).GetImportTransactionLinkByExternalKey(ctx, pgsqlgen.GetImportTransactionLinkByExternalKeyParams(p))
	return linkRow(l), err
}

func (pgsqlQuerier) ListImportTransactionLinksByTransaction(ctx context.Context, db backend.DBTX, transactionID *string) ([]linkRow, error) {
	rows, err := pgsqlgen.New(db).ListImportTransactionLinksByTransaction(ctx, transactionID)
	if err != nil {
		return nil, err
	}
	out := make([]linkRow, len(rows))
	for i, row := range rows {
		out[i] = linkRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) GetImportSourceByUserProvider(ctx context.Context, db backend.DBTX, p sourceByUserProviderPs) (sourceRow, error) {
	s, err := pgsqlgen.New(db).GetImportSourceByUserProvider(ctx, pgsqlgen.GetImportSourceByUserProviderParams(p))
	return sourceRow(s), err
}

func (pgsqlQuerier) ListImportSourcesByUser(ctx context.Context, db backend.DBTX, userID string) ([]sourceRow, error) {
	rows, err := pgsqlgen.New(db).ListImportSourcesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]sourceRow, len(rows))
	for i, row := range rows {
		out[i] = sourceRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) DeleteImportSource(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteImportSource(ctx, id)
}

func (pgsqlQuerier) InsertImportAccountLink(ctx context.Context, db backend.DBTX, p insertAccountLinkParams) error {
	return pgsqlgen.New(db).InsertImportAccountLink(ctx, pgsqlgen.InsertImportAccountLinkParams(p))
}

func (pgsqlQuerier) UpdateImportAccountLink(ctx context.Context, db backend.DBTX, p updateAccountLinkParams) error {
	return pgsqlgen.New(db).UpdateImportAccountLink(ctx, pgsqlgen.UpdateImportAccountLinkParams(p))
}

func (pgsqlQuerier) DeleteImportAccountLink(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteImportAccountLink(ctx, id)
}

func (pgsqlQuerier) GetImportAccountLinkByID(ctx context.Context, db backend.DBTX, id string) (accountLinkRow, error) {
	a, err := pgsqlgen.New(db).GetImportAccountLinkByID(ctx, id)
	return accountLinkRow(a), err
}

func (pgsqlQuerier) ListImportAccountLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]accountLinkRow, error) {
	rows, err := pgsqlgen.New(db).ListImportAccountLinksBySource(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	out := make([]accountLinkRow, len(rows))
	for i, row := range rows {
		out[i] = accountLinkRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) DeleteImportEvent(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteImportEvent(ctx, id)
}

func (pgsqlQuerier) ListImportEventsBySourceStatus(ctx context.Context, db backend.DBTX, p eventsBySourceStatusPs) ([]eventRow, error) {
	rows, err := pgsqlgen.New(db).ListImportEventsBySourceStatus(ctx, pgsqlgen.ListImportEventsBySourceStatusParams(p))
	if err != nil {
		return nil, err
	}
	out := make([]eventRow, len(rows))
	for i, row := range rows {
		out[i] = eventRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) GetImportTransactionLinkByID(ctx context.Context, db backend.DBTX, id string) (linkRow, error) {
	l, err := pgsqlgen.New(db).GetImportTransactionLinkByID(ctx, id)
	return linkRow(l), err
}

func (pgsqlQuerier) UpdateImportTransactionLink(ctx context.Context, db backend.DBTX, p updateLinkParams) error {
	return pgsqlgen.New(db).UpdateImportTransactionLink(ctx, pgsqlgen.UpdateImportTransactionLinkParams(p))
}

func (pgsqlQuerier) ListImportTransactionLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]linkRow, error) {
	rows, err := pgsqlgen.New(db).ListImportTransactionLinksBySource(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	out := make([]linkRow, len(rows))
	for i, row := range rows {
		out[i] = linkRow(row)
	}
	return out, nil
}

func (pgsqlQuerier) DeleteQueuedImportTransactionLinksByExternalAccount(ctx context.Context, db backend.DBTX, p purgeQueuedLinksParams) error {
	return pgsqlgen.New(db).DeleteQueuedImportTransactionLinksByExternalAccount(ctx, pgsqlgen.DeleteQueuedImportTransactionLinksByExternalAccountParams(p))
}
