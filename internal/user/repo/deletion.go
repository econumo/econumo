package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

type (
	deletionStateRow        = sqlitegen.GetUserDeletionStateRow
	markedUserRow           = sqlitegen.ListMarkedUsersRow
	inactiveCandidateRow    = sqlitegen.ListInactiveUserCandidatesRow
	auditRow                = sqlitegen.UsersAudit
	auditInsertParams       = sqlitegen.InsertUserAuditParams
	deletionRequestRow      = sqlitegen.UsersDeletionRequest
	deletionRequestInsert   = sqlitegen.InsertUserDeletionRequestIfGenerationParams
	deletionRequestConsume  = sqlitegen.ConsumeUserDeletionRequestParams
	markForDeletionParams   = sqlitegen.MarkUserForDeletionParams
	touchLastActiveParams   = sqlitegen.TouchUserLastActiveParams
	activeConnectionParams  = sqlitegen.UserHasActiveConnectionParams
	auditByEventQueryParams = sqlitegen.ListUserAuditByEventParams
)

type deletionQuerier interface {
	GetUserDeletionState(ctx context.Context, db backend.DBTX, id string) (deletionStateRow, error)
	MarkUserForDeletion(ctx context.Context, db backend.DBTX, p markForDeletionParams) (int64, error)
	UnmarkUserDeletion(ctx context.Context, db backend.DBTX, id string) (int64, error)
	TouchUserLastActive(ctx context.Context, db backend.DBTX, p touchLastActiveParams) error
	ListMarkedUsers(ctx context.Context, db backend.DBTX) ([]markedUserRow, error)
	ListInactiveUserCandidates(ctx context.Context, db backend.DBTX, cutoff time.Time) ([]inactiveCandidateRow, error)
	UserHasActiveConnection(ctx context.Context, db backend.DBTX, p activeConnectionParams) (bool, error)
	ListWipeConnectedUserIDs(ctx context.Context, db backend.DBTX, userID string) ([]string, error)
	DeleteUserByID(ctx context.Context, db backend.DBTX, id string) (int64, error)
	LockUserRowForWipe(ctx context.Context, db backend.DBTX, id string) error
	LockDeletionWipes(ctx context.Context, db backend.DBTX) error

	InsertUserAudit(ctx context.Context, db backend.DBTX, p auditInsertParams) (int64, error)
	ListUserAuditByEmailHash(ctx context.Context, db backend.DBTX, hash string) ([]auditRow, error)
	ListUserAuditByEvent(ctx context.Context, db backend.DBTX, p auditByEventQueryParams) ([]auditRow, error)

	DeleteUserDeletionRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error
	InsertUserDeletionRequestIfGeneration(ctx context.Context, db backend.DBTX, p deletionRequestInsert) (int64, error)
	ConsumeUserDeletionRequest(ctx context.Context, db backend.DBTX, p deletionRequestConsume) (int64, error)
	GetUserDeletionRequestByUser(ctx context.Context, db backend.DBTX, userID string) (deletionRequestRow, error)
}

func newDeletionQuerier(driver string) deletionQuerier {
	switch driver {
	case "sqlite":
		return deletionSqliteQuerier{}
	case "postgresql":
		return deletionPgsqlQuerier{}
	default:
		panic("deletionrepo: unknown database driver " + driver)
	}
}

// sec truncates to the whole second the columns hold. PostgreSQL's
// TIMESTAMP(0) would round instead, so 12:00:00.750 would read back as
// 12:00:01 there and 12:00:00 on SQLite.
func sec(t time.Time) time.Time { return t.UTC().Truncate(time.Second) }

type DeletionRepo struct {
	tx *backend.TxManager
	q  deletionQuerier
}

var _ user.DeletionStore = (*DeletionRepo)(nil)

func NewDeletionRepo(driver string, tx *backend.TxManager) *DeletionRepo {
	return &DeletionRepo{tx: tx, q: newDeletionQuerier(driver)}
}

func (r *DeletionRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *DeletionRepo) GetState(ctx context.Context, userID vo.Id) (model.DeletionState, error) {
	row, err := r.q.GetUserDeletionState(ctx, r.db(ctx), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.DeletionState{}, errs.NewNotFound("User not found")
		}
		return model.DeletionState{}, err
	}
	return model.DeletionState{
		MarkedAt:     utcPtr(row.DeletionMarkedAt),
		Reason:       model.DeletionReason(row.DeletionReason),
		LastActiveAt: utcPtr(row.LastActiveAt),
		CreatedAt:    row.CreatedAt.UTC(),
	}, nil
}

func (r *DeletionRepo) Mark(ctx context.Context, userID vo.Id, reason model.DeletionReason, now time.Time) (int64, error) {
	at := sec(now)
	return r.q.MarkUserForDeletion(ctx, r.db(ctx), markForDeletionParams{
		DeletionMarkedAt: &at,
		DeletionReason:   string(reason),
		ID:               userID.String(),
	})
}

func (r *DeletionRepo) Unmark(ctx context.Context, userID vo.Id) (int64, error) {
	return r.q.UnmarkUserDeletion(ctx, r.db(ctx), userID.String())
}

func (r *DeletionRepo) TouchLastActive(ctx context.Context, userID vo.Id, now time.Time) error {
	at := sec(now)
	before := at.Add(-24 * time.Hour)
	return r.q.TouchUserLastActive(ctx, r.db(ctx), touchLastActiveParams{
		LastActiveAt:   &at,
		ID:             userID.String(),
		LastActiveAt_2: &before,
	})
}

func (r *DeletionRepo) ListMarked(ctx context.Context) ([]model.MarkedUser, error) {
	rows, err := r.q.ListMarkedUsers(ctx, r.db(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]model.MarkedUser, 0, len(rows))
	for _, row := range rows {
		id, err := vo.ParseId(row.ID)
		if err != nil {
			return nil, err
		}
		u := model.MarkedUser{ID: id, Email: row.Email, Reason: model.DeletionReason(row.DeletionReason)}
		if row.DeletionMarkedAt != nil { // the query excludes NULL
			u.MarkedAt = row.DeletionMarkedAt.UTC()
		}
		out = append(out, u)
	}
	return out, nil
}

func (r *DeletionRepo) ListInactiveCandidates(ctx context.Context, cutoff time.Time) ([]model.InactiveCandidate, error) {
	rows, err := r.q.ListInactiveUserCandidates(ctx, r.db(ctx), sec(cutoff))
	if err != nil {
		return nil, err
	}
	out := make([]model.InactiveCandidate, 0, len(rows))
	for _, row := range rows {
		id, err := vo.ParseId(row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, model.InactiveCandidate{ID: id, Email: row.Email, Name: row.Name, Language: row.Language})
	}
	return out, nil
}

func (r *DeletionRepo) HasActiveConnection(ctx context.Context, userID vo.Id, cutoff time.Time) (bool, error) {
	at := sec(cutoff)
	return r.q.UserHasActiveConnection(ctx, r.db(ctx), activeConnectionParams{UserID: userID.String(), Cutoff: &at})
}

func (r *DeletionRepo) DeleteUser(ctx context.Context, userID vo.Id) (int64, error) {
	return r.q.DeleteUserByID(ctx, r.db(ctx), userID.String())
}

func (r *DeletionRepo) LockRowForWipe(ctx context.Context, userID vo.Id) error {
	return r.q.LockUserRowForWipe(ctx, r.db(ctx), userID.String())
}

func (r *DeletionRepo) LockWipes(ctx context.Context) error {
	return r.q.LockDeletionWipes(ctx, r.db(ctx))
}

func (r *DeletionRepo) ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	raw, err := r.q.ListWipeConnectedUserIDs(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]vo.Id, 0, len(raw))
	for _, s := range raw {
		id, err := vo.ParseId(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

type AuditRepo struct {
	tx *backend.TxManager
	q  deletionQuerier
}

var _ user.AuditLog = (*AuditRepo)(nil)

func NewAuditRepo(driver string, tx *backend.TxManager) *AuditRepo {
	return &AuditRepo{tx: tx, q: newDeletionQuerier(driver)}
}

func (r *AuditRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *AuditRepo) Insert(ctx context.Context, e model.UserAuditEvent) (bool, error) {
	var ref *string
	if e.RefID != nil {
		s := e.RefID.String()
		ref = &s
	}
	details := e.Details
	if details == "" {
		details = "{}"
	}
	n, err := r.q.InsertUserAudit(ctx, r.db(ctx), auditInsertParams{
		ID:        e.ID.String(),
		UserID:    e.UserID.String(),
		Event:     e.Event,
		EmailHash: e.EmailHash,
		RefID:     ref,
		Details:   details,
		CreatedAt: sec(e.CreatedAt),
	})
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (r *AuditRepo) ListByEmailHash(ctx context.Context, hash string) ([]model.UserAuditEvent, error) {
	rows, err := r.q.ListUserAuditByEmailHash(ctx, r.db(ctx), hash)
	if err != nil {
		return nil, err
	}
	return auditEvents(rows)
}

func (r *AuditRepo) ListByEvent(ctx context.Context, event string, since time.Time, afterID string, limit int) ([]model.UserAuditEvent, error) {
	if afterID == "" {
		afterID = nilUserID
	}
	rows, err := r.q.ListUserAuditByEvent(ctx, r.db(ctx), auditByEventQueryParams{
		Event:    event,
		Since:    sec(since),
		AfterID:  afterID,
		RowLimit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	return auditEvents(rows)
}

func auditEvents(rows []auditRow) ([]model.UserAuditEvent, error) {
	out := make([]model.UserAuditEvent, 0, len(rows))
	for _, row := range rows {
		id, err := vo.ParseId(row.ID)
		if err != nil {
			return nil, err
		}
		uid, err := vo.ParseId(row.UserID)
		if err != nil {
			return nil, err
		}
		e := model.UserAuditEvent{ID: id, UserID: uid, Event: row.Event, EmailHash: row.EmailHash,
			Details: row.Details, CreatedAt: row.CreatedAt.UTC()}
		if row.RefID != nil {
			ref, err := vo.ParseId(*row.RefID)
			if err != nil {
				return nil, err
			}
			e.RefID = &ref
		}
		out = append(out, e)
	}
	return out, nil
}

type DeletionRequestRepo struct {
	tx *backend.TxManager
	q  deletionQuerier
}

var _ user.DeletionRequests = (*DeletionRequestRepo)(nil)

func NewDeletionRequestRepo(driver string, tx *backend.TxManager) *DeletionRequestRepo {
	return &DeletionRequestRepo{tx: tx, q: newDeletionQuerier(driver)}
}

func (r *DeletionRequestRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *DeletionRequestRepo) GetByUser(ctx context.Context, userID vo.Id) (*model.DeletionRequest, error) {
	row, err := r.q.GetUserDeletionRequestByUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Deletion request not found")
		}
		return nil, err
	}
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	uid, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.DeletionRequest{ID: id, UserID: uid, Email: row.Email, Code: row.Code,
		CreatedAt: row.CreatedAt.UTC(), ExpiredAt: row.ExpiredAt.UTC()}, nil
}

func (r *DeletionRequestRepo) Save(ctx context.Context, dr *model.DeletionRequest, generation int64) (int64, error) {
	return r.q.InsertUserDeletionRequestIfGeneration(ctx, r.db(ctx), deletionRequestInsert{
		ID:                    dr.ID.String(),
		UserID:                dr.UserID.String(),
		Email:                 dr.Email,
		Code:                  dr.Code,
		CreatedAt:             sec(dr.CreatedAt),
		ExpiredAt:             sec(dr.ExpiredAt),
		ID_2:                  dr.UserID.String(),
		CredentialsGeneration: generation,
	})
}

func (r *DeletionRequestRepo) Consume(ctx context.Context, id, userID vo.Id) (int64, error) {
	return r.q.ConsumeUserDeletionRequest(ctx, r.db(ctx), deletionRequestConsume{ID: id.String(), UserID: userID.String()})
}

func (r *DeletionRequestRepo) DeleteByUser(ctx context.Context, userID vo.Id) error {
	return r.q.DeleteUserDeletionRequestsByUser(ctx, r.db(ctx), userID.String())
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
