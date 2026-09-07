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
	sourceRow               = sqlitegen.ImportSource
	eventRow                = sqlitegen.ImportEvent
	runRow                  = sqlitegen.ImportRun
	linkRow                 = sqlitegen.ImportTransactionLink
	insertSourceParams      = sqlitegen.InsertImportSourceParams
	insertEventParams       = sqlitegen.InsertImportEventParams
	updateEventParams       = sqlitegen.UpdateImportEventStatusParams
	insertRunParams         = sqlitegen.InsertImportRunParams
	updateRunParams         = sqlitegen.UpdateImportRunParams
	insertLinkParams        = sqlitegen.InsertImportTransactionLinkParams
	linkByExternalKeyPs     = sqlitegen.GetImportTransactionLinkByExternalKeyParams
	accountLinkRow          = sqlitegen.ImportAccountLink
	insertAccountLinkParams = sqlitegen.InsertImportAccountLinkParams
	updateAccountLinkParams = sqlitegen.UpdateImportAccountLinkParams
	sourceByUserProviderPs  = sqlitegen.GetImportSourceByUserProviderParams
	eventsBySourceStatusPs  = sqlitegen.ListImportEventsBySourceStatusParams
	updateLinkParams        = sqlitegen.UpdateImportTransactionLinkParams
	purgeQueuedLinksParams  = sqlitegen.DeleteQueuedImportTransactionLinksByExternalAccountParams
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

	GetImportSourceByUserProvider(ctx context.Context, db backend.DBTX, p sourceByUserProviderPs) (sourceRow, error)
	ListImportSourcesByUser(ctx context.Context, db backend.DBTX, userID string) ([]sourceRow, error)
	DeleteImportSource(ctx context.Context, db backend.DBTX, id string) error
	InsertImportAccountLink(ctx context.Context, db backend.DBTX, p insertAccountLinkParams) error
	UpdateImportAccountLink(ctx context.Context, db backend.DBTX, p updateAccountLinkParams) error
	DeleteImportAccountLink(ctx context.Context, db backend.DBTX, id string) error
	GetImportAccountLinkByID(ctx context.Context, db backend.DBTX, id string) (accountLinkRow, error)
	ListImportAccountLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]accountLinkRow, error)
	DeleteImportEvent(ctx context.Context, db backend.DBTX, id string) error
	ListImportEventsBySourceStatus(ctx context.Context, db backend.DBTX, p eventsBySourceStatusPs) ([]eventRow, error)
	GetImportTransactionLinkByID(ctx context.Context, db backend.DBTX, id string) (linkRow, error)
	UpdateImportTransactionLink(ctx context.Context, db backend.DBTX, p updateLinkParams) error
	ListImportTransactionLinksBySource(ctx context.Context, db backend.DBTX, sourceID string) ([]linkRow, error)
	DeleteQueuedImportTransactionLinksByExternalAccount(ctx context.Context, db backend.DBTX, p purgeQueuedLinksParams) error
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

func (r *Repo) InsertRun(ctx context.Context, run *model.ImportRun) error {
	return r.q.InsertImportRun(ctx, r.db(ctx), insertRunParams{
		ID: run.ID.String(), UserID: run.UserID.String(), SourceID: run.SourceID.String(), Provider: run.Provider,
		Params: run.Params, Status: run.Status,
		ImportedCount: int64(run.ImportedCount), MatchedCount: int64(run.MatchedCount),
		SkippedCount: int64(run.SkippedCount), FailedCount: int64(run.FailedCount),
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	})
}

func (r *Repo) GetRun(ctx context.Context, id vo.Id) (*model.ImportRun, error) {
	row, err := r.q.GetImportRunByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import run not found")
		}
		return nil, err
	}
	return runFromRow(row)
}

func (r *Repo) UpdateRun(ctx context.Context, run *model.ImportRun) error {
	return r.q.UpdateImportRun(ctx, r.db(ctx), updateRunParams{
		Status:        run.Status,
		ImportedCount: int64(run.ImportedCount), MatchedCount: int64(run.MatchedCount),
		SkippedCount: int64(run.SkippedCount), FailedCount: int64(run.FailedCount),
		FinishedAt: run.FinishedAt, ID: run.ID.String(),
	})
}

func runFromRow(row runRow) (*model.ImportRun, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	sourceID, err := vo.ParseId(row.SourceID)
	if err != nil {
		return nil, err
	}
	return &model.ImportRun{
		ID: id, UserID: userID, SourceID: sourceID, Provider: row.Provider, Params: row.Params, Status: row.Status,
		ImportedCount: int(row.ImportedCount), MatchedCount: int(row.MatchedCount),
		SkippedCount: int(row.SkippedCount), FailedCount: int(row.FailedCount),
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
	}, nil
}

func (r *Repo) InsertLink(ctx context.Context, l *model.ImportTransactionLink) error {
	return r.q.InsertImportTransactionLink(ctx, r.db(ctx), insertLinkParams{
		ID: l.ID.String(), SourceID: l.SourceID.String(), RunID: optionalID(l.RunID), EventID: optionalID(l.EventID),
		ExternalAccountID: l.ExternalAccountID, ExternalTransactionID: l.ExternalTransactionID,
		TransactionID: optionalID(l.TransactionID), Status: l.Status,
		ExternalPayee: l.ExternalPayee, ExternalDescription: l.ExternalDescription,
		ExternalAmount: l.ExternalAmount, ExternalCurrency: l.ExternalCurrency, ExternalPostedAt: l.ExternalPostedAt,
		AppliedCategoryID: optionalID(l.AppliedCategoryID), AppliedPayeeID: optionalID(l.AppliedPayeeID),
		AppliedTagID: optionalID(l.AppliedTagID), AppliedRuleID: optionalID(l.AppliedRuleID),
		ImportedAt: l.ImportedAt,
	})
}

func (r *Repo) GetLinkByExternalKey(ctx context.Context, sourceID vo.Id, externalAccountID, externalTransactionID string) (*model.ImportTransactionLink, error) {
	row, err := r.q.GetImportTransactionLinkByExternalKey(ctx, r.db(ctx), linkByExternalKeyPs{
		SourceID: sourceID.String(), ExternalAccountID: externalAccountID, ExternalTransactionID: externalTransactionID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import link not found")
		}
		return nil, err
	}
	return linkFromRow(row)
}

func (r *Repo) ListLinksByTransaction(ctx context.Context, transactionID vo.Id) ([]model.ImportTransactionLink, error) {
	id := transactionID.String()
	rows, err := r.q.ListImportTransactionLinksByTransaction(ctx, r.db(ctx), &id)
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportTransactionLink, 0, len(rows))
	for _, row := range rows {
		l, err := linkFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func linkFromRow(row linkRow) (*model.ImportTransactionLink, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	sourceID, err := vo.ParseId(row.SourceID)
	if err != nil {
		return nil, err
	}
	l := &model.ImportTransactionLink{
		ID: id, SourceID: sourceID,
		ExternalAccountID: row.ExternalAccountID, ExternalTransactionID: row.ExternalTransactionID, Status: row.Status,
		ExternalPayee: row.ExternalPayee, ExternalDescription: row.ExternalDescription,
		ExternalAmount: vo.NewDecimal(row.ExternalAmount).String(), ExternalCurrency: row.ExternalCurrency,
		ExternalPostedAt: row.ExternalPostedAt, ImportedAt: row.ImportedAt,
	}
	for _, f := range []struct {
		src *string
		dst **vo.Id
	}{
		{row.RunID, &l.RunID}, {row.EventID, &l.EventID}, {row.TransactionID, &l.TransactionID},
		{row.AppliedCategoryID, &l.AppliedCategoryID}, {row.AppliedPayeeID, &l.AppliedPayeeID},
		{row.AppliedTagID, &l.AppliedTagID}, {row.AppliedRuleID, &l.AppliedRuleID},
	} {
		v, err := parseOptionalID(f.src)
		if err != nil {
			return nil, err
		}
		*f.dst = v
	}
	return l, nil
}

func (r *Repo) GetSourceByUserProvider(ctx context.Context, userID vo.Id, provider string) (*model.ImportSource, error) {
	row, err := r.q.GetImportSourceByUserProvider(ctx, r.db(ctx), sourceByUserProviderPs{UserID: userID.String(), Provider: provider})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import source not found")
		}
		return nil, err
	}
	return sourceFromRow(row)
}

func (r *Repo) ListSourcesByUser(ctx context.Context, userID vo.Id) ([]model.ImportSource, error) {
	rows, err := r.q.ListImportSourcesByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportSource, 0, len(rows))
	for _, row := range rows {
		s, err := sourceFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, nil
}

func (r *Repo) DeleteSource(ctx context.Context, id vo.Id) error {
	return r.q.DeleteImportSource(ctx, r.db(ctx), id.String())
}

func (r *Repo) InsertAccountLink(ctx context.Context, l *model.ImportAccountLink) error {
	return r.q.InsertImportAccountLink(ctx, r.db(ctx), insertAccountLinkParams{
		ID: l.ID.String(), SourceID: l.SourceID.String(), ExternalAccountID: l.ExternalAccountID, ExternalName: l.ExternalName,
		ExternalCurrency: l.ExternalCurrency, AccountID: optionalID(l.AccountID), Mode: l.Mode, CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	})
}

func (r *Repo) UpdateAccountLink(ctx context.Context, l *model.ImportAccountLink) error {
	return r.q.UpdateImportAccountLink(ctx, r.db(ctx), updateAccountLinkParams{
		ExternalCurrency: l.ExternalCurrency, AccountID: optionalID(l.AccountID), Mode: l.Mode, UpdatedAt: l.UpdatedAt, ID: l.ID.String(),
	})
}

func (r *Repo) DeleteAccountLink(ctx context.Context, id vo.Id) error {
	return r.q.DeleteImportAccountLink(ctx, r.db(ctx), id.String())
}

func (r *Repo) GetAccountLink(ctx context.Context, id vo.Id) (*model.ImportAccountLink, error) {
	row, err := r.q.GetImportAccountLinkByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import account link not found")
		}
		return nil, err
	}
	return accountLinkFromRow(row)
}

func (r *Repo) ListAccountLinksBySource(ctx context.Context, sourceID vo.Id) ([]model.ImportAccountLink, error) {
	rows, err := r.q.ListImportAccountLinksBySource(ctx, r.db(ctx), sourceID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportAccountLink, 0, len(rows))
	for _, row := range rows {
		l, err := accountLinkFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func accountLinkFromRow(row accountLinkRow) (*model.ImportAccountLink, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	sourceID, err := vo.ParseId(row.SourceID)
	if err != nil {
		return nil, err
	}
	accountID, err := parseOptionalID(row.AccountID)
	if err != nil {
		return nil, err
	}
	return &model.ImportAccountLink{
		ID: id, SourceID: sourceID, ExternalAccountID: row.ExternalAccountID, ExternalName: row.ExternalName,
		ExternalCurrency: row.ExternalCurrency, AccountID: accountID, Mode: row.Mode, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func (r *Repo) DeleteEvent(ctx context.Context, id vo.Id) error {
	return r.q.DeleteImportEvent(ctx, r.db(ctx), id.String())
}

func (r *Repo) ListEventsBySourceStatus(ctx context.Context, sourceID vo.Id, status string) ([]model.ImportEvent, error) {
	rows, err := r.q.ListImportEventsBySourceStatus(ctx, r.db(ctx), eventsBySourceStatusPs{SourceID: sourceID.String(), Status: status})
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportEvent, 0, len(rows))
	for _, row := range rows {
		e, err := eventFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, nil
}

func (r *Repo) GetLink(ctx context.Context, id vo.Id) (*model.ImportTransactionLink, error) {
	row, err := r.q.GetImportTransactionLinkByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Import link not found")
		}
		return nil, err
	}
	return linkFromRow(row)
}

func (r *Repo) UpdateLink(ctx context.Context, l *model.ImportTransactionLink) error {
	return r.q.UpdateImportTransactionLink(ctx, r.db(ctx), updateLinkParams{
		RunID: optionalID(l.RunID), TransactionID: optionalID(l.TransactionID), Status: l.Status,
		ExternalAmount: l.ExternalAmount, ExternalCurrency: l.ExternalCurrency,
		AppliedCategoryID: optionalID(l.AppliedCategoryID), AppliedPayeeID: optionalID(l.AppliedPayeeID),
		AppliedTagID: optionalID(l.AppliedTagID), AppliedRuleID: optionalID(l.AppliedRuleID), ID: l.ID.String(),
	})
}

func (r *Repo) ListLinksBySource(ctx context.Context, sourceID vo.Id) ([]model.ImportTransactionLink, error) {
	rows, err := r.q.ListImportTransactionLinksBySource(ctx, r.db(ctx), sourceID.String())
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportTransactionLink, 0, len(rows))
	for _, row := range rows {
		l, err := linkFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func (r *Repo) DeleteQueuedLinksByExternalAccount(ctx context.Context, sourceID vo.Id, externalAccountID string) error {
	return r.q.DeleteQueuedImportTransactionLinksByExternalAccount(ctx, r.db(ctx), purgeQueuedLinksParams{SourceID: sourceID.String(), ExternalAccountID: externalAccountID})
}
