// Package debt holds the budget-debt use cases (loans and credit lines paid
// down through a category).
package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CandidateTx is an expense in a debt's category, in its account's currency.
type CandidateTx struct {
	ID         vo.Id
	SpentAt    time.Time
	Amount     vo.DecimalNumber
	CurrencyID vo.Id
	PayeeID    *vo.Id
	TagID      *vo.Id
	LabelIDs   []vo.Id
}

type Repository interface {
	NextIdentity() vo.Id
	// LockOwner takes the owner's user row lock for the rest of the
	// transaction (PostgreSQL); a no-op on SQLite.
	LockOwner(ctx context.Context, ownerID vo.Id) error
	GetByID(ctx context.Context, id vo.Id) (*model.Debt, error)
	ListByOwners(ctx context.Context, ownerIDs []vo.Id) ([]*model.Debt, error)
	ListByCategory(ctx context.Context, categoryID vo.Id) ([]*model.Debt, error)
	// ListByClassification: kind is "payee", "tag" or "label".
	ListByClassification(ctx context.Context, kind string, id vo.Id) ([]*model.Debt, error)
	Save(ctx context.Context, d *model.Debt) error
	Delete(ctx context.Context, id vo.Id) error
	// AddSnapshot is always paired with a Save of the same terms in one transaction.
	AddSnapshot(ctx context.Context, s *model.DebtSnapshot) error
	// UpdateSnapshot rewrites a snapshot's as-of, balance and terms: only the
	// origin's facts and, with them, a later snapshot's terms are corrected.
	UpdateSnapshot(ctx context.Context, s *model.DebtSnapshot) error
	// DeleteSnapshot removes a statement balance (never the origin).
	DeleteSnapshot(ctx context.Context, id vo.Id) error
	// Snapshots are ordered by debt, as_of, created_at, id. A debt's first is
	// its origin: the service tells it by position, with no flag stored. No
	// statement may precede the start (validateStatement, laterConflict), and
	// on a shared date the origin, written first, sorts first by created_at
	// (then by its UUIDv7 id, minted earlier).
	Snapshots(ctx context.Context, debtIDs []vo.Id) ([]*model.DebtSnapshot, error)
	Splits(ctx context.Context, debtIDs []vo.Id) ([]model.DebtSplit, error)
	// InsertSplit upserts on the transaction id: a payment has one split.
	InsertSplit(ctx context.Context, s model.DebtSplit) error
	DeleteSplits(ctx context.Context, debtID vo.Id, transactionIDs []vo.Id) error
	// CandidateTransactions returns expenses only (type 0).
	CandidateTransactions(ctx context.Context, categoryID vo.Id) ([]CandidateTx, error)
	UsesCategory(ctx context.Context, id vo.Id) (bool, error)
	UsesPayee(ctx context.Context, id vo.Id) (bool, error)
	UsesTag(ctx context.Context, id vo.Id) (bool, error)
	UsesLabel(ctx context.Context, id vo.Id) (bool, error)
	ReassignCategory(ctx context.Context, oldID, newID vo.Id) error
	ReassignPayee(ctx context.Context, oldID, newID vo.Id) error
	ReassignTag(ctx context.Context, oldID, newID vo.Id) error
	ReassignLabel(ctx context.Context, oldID, newID vo.Id) error
}
