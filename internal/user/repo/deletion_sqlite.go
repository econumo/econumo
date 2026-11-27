package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type deletionSqliteQuerier struct{}

var _ deletionQuerier = deletionSqliteQuerier{}

func (deletionSqliteQuerier) GetUserDeletionState(ctx context.Context, db backend.DBTX, id string) (deletionStateRow, error) {
	return sqlitegen.New(db).GetUserDeletionState(ctx, id)
}

func (deletionSqliteQuerier) MarkUserForDeletion(ctx context.Context, db backend.DBTX, p markForDeletionParams) (int64, error) {
	return sqlitegen.New(db).MarkUserForDeletion(ctx, p)
}

func (deletionSqliteQuerier) UnmarkUserDeletion(ctx context.Context, db backend.DBTX, id string) (int64, error) {
	return sqlitegen.New(db).UnmarkUserDeletion(ctx, id)
}

func (deletionSqliteQuerier) TouchUserLastActive(ctx context.Context, db backend.DBTX, p touchLastActiveParams) error {
	return sqlitegen.New(db).TouchUserLastActive(ctx, p)
}

func (deletionSqliteQuerier) ListMarkedUsers(ctx context.Context, db backend.DBTX) ([]markedUserRow, error) {
	return sqlitegen.New(db).ListMarkedUsers(ctx)
}

func (deletionSqliteQuerier) ListInactiveUserCandidates(ctx context.Context, db backend.DBTX, cutoff time.Time) ([]inactiveCandidateRow, error) {
	return sqlitegen.New(db).ListInactiveUserCandidates(ctx, &cutoff)
}

func (deletionSqliteQuerier) UserHasActiveConnection(ctx context.Context, db backend.DBTX, p activeConnectionParams) (bool, error) {
	n, err := sqlitegen.New(db).UserHasActiveConnection(ctx, p)
	return n != 0, err
}

func (deletionSqliteQuerier) ListWipeConnectedUserIDs(ctx context.Context, db backend.DBTX, userID string) ([]string, error) {
	return sqlitegen.New(db).ListWipeConnectedUserIDs(ctx, userID)
}

func (deletionSqliteQuerier) DeleteUserByID(ctx context.Context, db backend.DBTX, id string) (int64, error) {
	return sqlitegen.New(db).DeleteUserByID(ctx, id)
}

func (deletionSqliteQuerier) LockUserRowForWipe(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).LockUserRowForWipe(ctx, id)
}

// LockDeletionWipes is a no-op: SQLite admits one writer at a time.
func (deletionSqliteQuerier) LockDeletionWipes(context.Context, backend.DBTX) error { return nil }

func (deletionSqliteQuerier) InsertUserAudit(ctx context.Context, db backend.DBTX, p auditInsertParams) (int64, error) {
	return sqlitegen.New(db).InsertUserAudit(ctx, p)
}

func (deletionSqliteQuerier) ListUserAuditByEmailHash(ctx context.Context, db backend.DBTX, hash string) ([]auditRow, error) {
	return sqlitegen.New(db).ListUserAuditByEmailHash(ctx, &hash)
}

func (deletionSqliteQuerier) ListUserAuditByEvent(ctx context.Context, db backend.DBTX, p auditByEventQueryParams) ([]auditRow, error) {
	return sqlitegen.New(db).ListUserAuditByEvent(ctx, p)
}

func (deletionSqliteQuerier) DeleteUserDeletionRequestsByUser(ctx context.Context, db backend.DBTX, userID string) error {
	return sqlitegen.New(db).DeleteUserDeletionRequestsByUser(ctx, userID)
}

func (deletionSqliteQuerier) InsertUserDeletionRequestIfGeneration(ctx context.Context, db backend.DBTX, p deletionRequestInsert) (int64, error) {
	return sqlitegen.New(db).InsertUserDeletionRequestIfGeneration(ctx, p)
}

func (deletionSqliteQuerier) ConsumeUserDeletionRequest(ctx context.Context, db backend.DBTX, p deletionRequestConsume) (int64, error) {
	return sqlitegen.New(db).ConsumeUserDeletionRequest(ctx, p)
}

func (deletionSqliteQuerier) GetUserDeletionRequestByUser(ctx context.Context, db backend.DBTX, userID string) (deletionRequestRow, error) {
	return sqlitegen.New(db).GetUserDeletionRequestByUser(ctx, userID)
}
