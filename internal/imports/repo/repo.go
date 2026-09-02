package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	sourceRow           = sqlitegen.ImportSource
	eventRow            = sqlitegen.ImportEvent
	runRow              = sqlitegen.ImportRun
	linkRow             = sqlitegen.ImportTransactionLink
	insertSourceParams  = sqlitegen.InsertImportSourceParams
	insertEventParams   = sqlitegen.InsertImportEventParams
	updateEventParams   = sqlitegen.UpdateImportEventStatusParams
	insertRunParams     = sqlitegen.InsertImportRunParams
	updateRunParams     = sqlitegen.UpdateImportRunParams
	insertLinkParams    = sqlitegen.InsertImportTransactionLinkParams
	linkByExternalKeyPs = sqlitegen.GetImportTransactionLinkByExternalKeyParams
)

// querier method signatures mirror the sqlc-generated ones exactly, including
// *string for ListImportTransactionLinksByTransaction's parameter: sqlc infers
// a pointer there because transaction_id is a nullable column, even though the
// query only ever binds a non-NULL id.
type querier interface {
	InsertImportSource(ctx context.Context, db backend.DBTX, p insertSourceParams) error
	GetImportSourceByID(ctx context.Context, db backend.DBTX, id string) (sourceRow, error)
	InsertImportEvent(ctx context.Context, db backend.DBTX, p insertEventParams) (int64, error)
	GetImportEventByID(ctx context.Context, db backend.DBTX, id string) (eventRow, error)
	UpdateImportEventStatus(ctx context.Context, db backend.DBTX, p updateEventParams) error
	InsertImportRun(ctx context.Context, db backend.DBTX, p insertRunParams) error
	GetImportRunByID(ctx context.Context, db backend.DBTX, id string) (runRow, error)
	UpdateImportRun(ctx context.Context, db backend.DBTX, p updateRunParams) error
	InsertImportTransactionLink(ctx context.Context, db backend.DBTX, p insertLinkParams) error
	GetImportTransactionLinkByExternalKey(ctx context.Context, db backend.DBTX, p linkByExternalKeyPs) (linkRow, error)
	ListImportTransactionLinksByTransaction(ctx context.Context, db backend.DBTX, transactionID *string) ([]linkRow, error)
}

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ imports.Repository = (*Repo)(nil)

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Repo{tx: tx, q: pgsqlQuerier{}}
	default:
		panic("importsrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) InsertSource(ctx context.Context, s *model.ImportSource) error {
	return r.q.InsertImportSource(ctx, r.db(ctx), insertSourceParams{
		ID: s.ID.String(), UserID: s.UserID.String(), Provider: s.Provider, Name: s.Name,
		CredentialCiphertext: s.CredentialCiphertext, Status: s.Status, LastSyncedAt: s.LastSyncedAt,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	})
}

func (r *Repo) GetSource(ctx context.Context, id vo.Id) (*model.ImportSource, error) {
	row, err := r.q.GetImportSourceByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import source not found")
		}
		return nil, err
	}
	return sourceFromRow(row)
}

func sourceFromRow(row sourceRow) (*model.ImportSource, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.ImportSource{
		ID: id, UserID: userID, Provider: row.Provider, Name: row.Name,
		CredentialCiphertext: row.CredentialCiphertext, Status: row.Status, LastSyncedAt: row.LastSyncedAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *Repo) InsertEvent(ctx context.Context, e *model.ImportEvent) (bool, error) {
	n, err := r.q.InsertImportEvent(ctx, r.db(ctx), insertEventParams{
		ID: e.ID.String(), SourceID: e.SourceID.String(), RunID: optionalID(e.RunID),
		Payload: e.Payload, PayloadHash: e.PayloadHash, Status: e.Status, ParseError: e.ParseError,
		ReceivedAt: e.ReceivedAt,
	})
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (r *Repo) GetEvent(ctx context.Context, id vo.Id) (*model.ImportEvent, error) {
	row, err := r.q.GetImportEventByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import event not found")
		}
		return nil, err
	}
	return eventFromRow(row)
}

func (r *Repo) UpdateEventStatus(ctx context.Context, id vo.Id, status string, parseError *string) error {
	// run_id is left as-is by passing the stored value back: read first so a
	// status update never detaches an event from its run.
	cur, err := r.q.GetImportEventByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewNotFound("Import event not found")
		}
		return err
	}
	return r.q.UpdateImportEventStatus(ctx, r.db(ctx), updateEventParams{
		Status: status, ParseError: parseError, RunID: cur.RunID, ID: id.String(),
	})
}

func eventFromRow(row eventRow) (*model.ImportEvent, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	sourceID, err := vo.ParseId(row.SourceID)
	if err != nil {
		return nil, err
	}
	runID, err := parseOptionalID(row.RunID)
	if err != nil {
		return nil, err
	}
	return &model.ImportEvent{
		ID: id, SourceID: sourceID, RunID: runID, Payload: row.Payload, PayloadHash: row.PayloadHash,
		Status: row.Status, ParseError: row.ParseError, ReceivedAt: row.ReceivedAt,
	}, nil
}

func optionalID(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func parseOptionalID(s *string) (*vo.Id, error) {
	if s == nil {
		return nil, nil
	}
	id, err := vo.ParseId(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// The following are implemented by Task 5, which deletes every stub below.

func (r *Repo) InsertRun(ctx context.Context, run *model.ImportRun) error {
	return errors.New("importsrepo: not implemented")
}

func (r *Repo) GetRun(ctx context.Context, id vo.Id) (*model.ImportRun, error) {
	return nil, errors.New("importsrepo: not implemented")
}

func (r *Repo) UpdateRun(ctx context.Context, run *model.ImportRun) error {
	return errors.New("importsrepo: not implemented")
}

func (r *Repo) InsertLink(ctx context.Context, l *model.ImportTransactionLink) error {
	return errors.New("importsrepo: not implemented")
}

func (r *Repo) GetLinkByExternalKey(ctx context.Context, sourceID vo.Id, externalAccountID, externalTransactionID string) (*model.ImportTransactionLink, error) {
	return nil, errors.New("importsrepo: not implemented")
}

func (r *Repo) ListLinksByTransaction(ctx context.Context, transactionID vo.Id) ([]model.ImportTransactionLink, error) {
	return nil, errors.New("importsrepo: not implemented")
}
