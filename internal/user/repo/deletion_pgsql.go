package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type deletionPgsqlQuerier struct{}

var _ deletionQuerier = deletionPgsqlQuerier{}

func (deletionPgsqlQuerier) GetUserDeletionState(ctx context.Context, db backend.DBTX, id string) (deletionStateRow, error) {
	row, err := pgsqlgen.New(db).GetUserDeletionState(ctx, id)
	return deletionStateRow(row), err
}

func (deletionPgsqlQuerier) MarkUserForDeletion(ctx context.Context, db backend.DBTX, p markForDeletionParams) (int64, error) {
	return pgsqlgen.New(db).MarkUserForDeletion(ctx, pgsqlgen.MarkUserForDeletionParams(p))
}

func (deletionPgsqlQuerier) UnmarkUserDeletion(ctx context.Context, db backend.DBTX, id string) (int64, error) {
	return pgsqlgen.New(db).UnmarkUserDeletion(ctx, id)
}

func (deletionPgsqlQuerier) TouchUserLastActive(ctx context.Context, db backend.DBTX, p touchLastActiveParams) error {
	return pgsqlgen.New(db).TouchUserLastActive(ctx, pgsqlgen.TouchUserLastActiveParams(p))
}

func (deletionPgsqlQuerier) ListMarkedUsers(ctx context.Context, db backend.DBTX) ([]markedUserRow, error) {
	rows, err := pgsqlgen.New(db).ListMarkedUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]markedUserRow, len(rows))
	for i, row := range rows {
		out[i] = markedUserRow(row)
	}
	return out, nil
}

func (deletionPgsqlQuerier) ListInactiveUserCandidates(ctx context.Context, db backend.DBTX, cutoff time.Time) ([]inactiveCandidateRow, error) {
	rows, err := pgsqlgen.New(db).ListInactiveUserCandidates(ctx, &cutoff)
	if err != nil {
		return nil, err
	}
	out := make([]inactiveCandidateRow, len(rows))
	for i, row := range rows {
		out[i] = inactiveCandidateRow(row)
	}
	return out, nil
}

func (deletionPgsqlQuerier) UserHasActiveConnection(ctx context.Context, db backend.DBTX, p activeConnectionParams) (bool, error) {
	return pgsqlgen.New(db).UserHasActiveConnection(ctx, pgsqlgen.UserHasActiveConnectionParams(p))
}

func (deletionPgsqlQuerier) ListWipeConnectedUserIDs(ctx context.Context, db backend.DBTX, userID string) ([]string, error) {
	return pgsqlgen.New(db).ListWipeConnectedUserIDs(ctx, userID)
}

func (deletionPgsqlQuerier) DeleteUserByID(ctx context.Context, db backend.DBTX, id string) (int64, error) {
	return pgsqlgen.New(db).DeleteUserByID(ctx, id)
}

// LockUserRowForWipe maps no-rows to success, like LockUserRow.
func (deletionPgsqlQuerier) LockUserRowForWipe(ctx context.Context, db backend.DBTX, id string) error {
	_, err := pgsqlgen.New(db).LockUserRowForWipe(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func (deletionPgsqlQuerier) LockDeletionWipes(ctx context.Context, db backend.DBTX) error {
	return pgsqlgen.New(db).LockDeletionWipes(ctx)
}

func (deletionPgsqlQuerier) InsertUserAudit(ctx context.Context, db backend.DBTX, p auditInsertParams) (int64, error) {
	return pgsqlgen.New(db).InsertUserAudit(ctx, pgsqlgen.InsertUserAuditParams(p))
}

func (deletionPgsqlQuerier) ListUserAuditByEmailHash(ctx context.Context, db backend.DBTX, hash string) ([]auditRow, error) {
	rows, err := pgsqlgen.New(db).ListUserAuditByEmailHash(ctx, &hash)
	return auditRows(rows), err
}

func (deletionPgsqlQuerier) ListUserAuditByEvent(ctx context.Context, db backend.DBTX, p auditByEventQueryParams) ([]auditRow, error) {
	rows, err := pgsqlgen.New(db).ListUserAuditByEvent(ctx, pgsqlgen.ListUserAuditByEventParams{
		Event:    p.Event,
		Since:    p.Since,
		AfterID:  p.AfterID,
		RowLimit: int32(p.RowLimit),
	})
	return auditRows(rows), err
}

func auditRows(rows []pgsqlgen.UsersAudit) []auditRow {
	out := make([]auditRow, len(rows))
	for i, row := range rows {
		out[i] = auditRow(row)
	}
	return out
}

func (deletionPgsqlQuerier) DeleteUserDeletionRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).DeleteUserDeletionRequestsByUser(ctx, userID)
}

func (deletionPgsqlQuerier) InsertUserDeletionRequestIfGeneration(ctx context.Context, db backend.DBTX, p deletionRequestInsert) (int64, error) {
	return pgsqlgen.New(db).InsertUserDeletionRequestIfGeneration(ctx, pgsqlgen.InsertUserDeletionRequestIfGenerationParams(p))
}

func (deletionPgsqlQuerier) ConsumeUserDeletionRequest(ctx context.Context, db backend.DBTX, p deletionRequestConsume) (int64, error) {
	return pgsqlgen.New(db).ConsumeUserDeletionRequest(ctx, pgsqlgen.ConsumeUserDeletionRequestParams(p))
}

func (deletionPgsqlQuerier) GetUserDeletionRequestByUser(ctx context.Context, db backend.DBTX, userID string) (deletionRequestRow, error) {
	row, err := pgsqlgen.New(db).GetUserDeletionRequestByUser(ctx, userID)
	return deletionRequestRow(row), err
}
