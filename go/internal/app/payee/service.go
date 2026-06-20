// Service wiring: the use-case orchestrator, its dependency seams, the
// constructor, and the shared private helpers (entity->DTO conversion, the
// value-object constructor, and the duplicate-name check). The individual use
// cases live in sibling files (create.go, update.go, archive.go, delete.go,
// order.go); the pure read lives in read.go.
package payee

import (
	"context"
	"time"

	dompayee "github.com/econumo/econumo/internal/domain/payee"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// apiDatetimeLayout is the wire format for createdAt/updatedAt: "2006-01-02
// 15:04:05" (space separator, no timezone). See COMPATIBILITY.md.
const apiDatetimeLayout = "2006-01-02 15:04:05"

// Clock supplies the current time. A seam so tests can pin timestamps for
// byte-stable golden output.
type Clock interface {
	Now() time.Time
}

// TxRunner is the transaction boundary the service owns. backend.TxManager
// satisfies it; defining it here keeps the app layer from importing the storage
// package directly.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// OperationGuard provides the row-based idempotency for create-payee. Claim
// attempts to record the request id; it reports already=true when the id was
// previously claimed (a duplicate request) so the caller can reject it. The
// shared operation.Guard satisfies it.
type OperationGuard interface {
	// Claim inserts the id into operation_requests_ids. Returns already=true if a
	// row for the id already existed (duplicate). Runs inside the caller's tx.
	Claim(ctx context.Context, id vo.Id, now time.Time) (already bool, err error)
	// MarkHandled flips is_handled to true after the operation succeeds.
	MarkHandled(ctx context.Context, id vo.Id, now time.Time) error
}

// Service is the payee write-side use-case orchestrator. It owns the tx boundary
// and builds the response-shaped *Result structs directly.
type Service struct {
	repo  dompayee.Repository
	tx    TxRunner
	ops   OperationGuard
	clock Clock
}

// NewService wires the payee service.
func NewService(repo dompayee.Repository, tx TxRunner, ops OperationGuard, clock Clock) *Service {
	return &Service{repo: repo, tx: tx, ops: ops, clock: clock}
}

// ---------------------------------------------------------------------------
// shared private helpers used across the use cases
// ---------------------------------------------------------------------------

// mutate loads the payee, checks ownership, applies fn inside a transaction, and
// saves. It returns the mutated (in-memory) aggregate so the caller can build
// its result without a second read. Ownership failure -> AccessDenied (403).
func (s *Service) mutate(ctx context.Context, id, userID vo.Id, fn func(p *dompayee.Payee, now time.Time)) (*dompayee.Payee, error) {
	var loaded *dompayee.Payee
	err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		p, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if !p.UserId().Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		fn(p, s.clock.Now())
		if err := s.repo.Save(txCtx, p); err != nil {
			return err
		}
		loaded = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return loaded, nil
}

// mutateChecked is mutate's variant whose fn can fail (e.g. a uniqueness check
// run inside the tx before mutating). The whole closure runs in one tx so a
// failed check rolls back cleanly.
//
// fn receives the tx-scoped context (txCtx) so any repo reads it performs (e.g.
// the uniqueness check's ListByOwner) run on the active transaction's
// connection rather than reaching for the pool — critical under a single-
// connection pool, where a pool read while the tx holds the only connection
// would deadlock.
func (s *Service) mutateChecked(ctx context.Context, id, userID vo.Id, fn func(ctx context.Context, p *dompayee.Payee, now time.Time) error) (*dompayee.Payee, error) {
	var loaded *dompayee.Payee
	err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		p, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if !p.UserId().Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		if ferr := fn(txCtx, p, s.clock.Now()); ferr != nil {
			return ferr
		}
		if err := s.repo.Save(txCtx, p); err != nil {
			return err
		}
		loaded = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return loaded, nil
}

// toResult is the single entity->DTO conversion in the module. It formats the
// timestamps in the "2006-01-02 15:04:05" wire form and maps the archived bool
// to the wire shape (isArchived int 0/1). See COMPATIBILITY.md.
func toResult(p *dompayee.Payee) PayeeResult {
	archived := 0
	if p.IsArchived() {
		archived = 1
	}
	return PayeeResult{
		Id:          p.Id().String(),
		OwnerUserId: p.UserId().String(),
		Name:        p.Name(),
		Position:    int(p.Position()),
		IsArchived:  archived,
		CreatedAt:   p.CreatedAt().Format(apiDatetimeLayout),
		UpdatedAt:   p.UpdatedAt().Format(apiDatetimeLayout),
	}
}

// listResults loads the owner's payees (ordered by position) and converts them
// to the wire shape — used by order-payee-list.
func (s *Service) listResults(ctx context.Context, userID vo.Id) ([]PayeeResult, error) {
	payees, err := s.repo.ListByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]PayeeResult, 0, len(payees))
	for _, p := range payees {
		items = append(items, toResult(p))
	}
	return items, nil
}

// ensureNameUnique enforces the per-owner name-uniqueness rule (PHP
// PayeeService::createPayee / updatePayee throw PayeeAlreadyExistsException).
// exceptID, when non-empty, is excluded from the comparison (for updates of the
// payee itself). The duplicate error message is "Payee already exists." (mirrors
// the PHP ValidationException for the missing 'payee.payee.already_exists'
// translation key — reconcile vs the real Cest if the PHP suite is ever run as
// oracle).
func (s *Service) ensureNameUnique(ctx context.Context, userID vo.Id, name string, exceptID vo.Id) error {
	payees, err := s.repo.ListByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, p := range payees {
		if p.Name() == name && !p.Id().Equal(exceptID) {
			return errs.NewValidation("Payee already exists.")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// tier-2 value-object constructor (payee-module invariant)
// ---------------------------------------------------------------------------

// newPayeeName enforces the payee name invariant: rune length 3..64. The error
// message is EXACTLY "Payee name must be 3-64 characters" (wire-compat with
// existing API clients) and the field key is "name". This mirrors the PHP
// GenericName validator (label derived from the VO short name "PayeeName" ->
// "Payee name"). See COMPATIBILITY.md.
func newPayeeName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Payee name must be 3-64 characters",
			errs.FieldError{Key: "name", Message: "Payee name must be 3-64 characters"})
	}
	return v, nil
}
