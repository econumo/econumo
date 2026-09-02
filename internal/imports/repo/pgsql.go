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
