package payee

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Service is the payee write-side use-case orchestrator; it owns the tx boundary.
type Service struct {
	repo   Repository
	tx     port.TxRunner
	ops    port.OperationGuard
	clock  port.Clock
	read   ReadModel
	access AccountAccess
}

// NewService wires the payee service. read is the own+shared payee view (the
// same ReadModel get-payee-list uses); order-payee-list returns that full
// available list. access resolves shared-account ownership for
// create-payee-for-account. ops backs create-payee's request-id idempotency.
func NewService(repo Repository, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, read ReadModel, access AccountAccess) *Service {
	return &Service{repo: repo, tx: tx, ops: ops, clock: clock, read: read, access: access}
}

// resolveAccountOwner returns the user a payee created in the context of
// accountID must be owned by — the account owner. The caller must own the
// account or hold an admin grant on it; otherwise AccessDenied.
func (s *Service) resolveAccountOwner(ctx context.Context, userID, accountID vo.Id) (vo.Id, error) {
	owner, err := s.access.AccountOwner(ctx, accountID)
	if err != nil {
		return vo.Id{}, err
	}
	if owner.Equal(userID) {
		return owner, nil
	}
	ok, err := s.access.HasAdminGrant(ctx, accountID, userID)
	if err != nil {
		return vo.Id{}, err
	}
	if ok {
		return owner, nil
	}
	return vo.Id{}, errs.NewAccessDenied("Access is not allowed")
}

// mutate loads the payee, checks ownership, applies fn inside a transaction, and
// saves. It returns the mutated (in-memory) aggregate so the caller can build
// its result without a second read. Ownership failure -> AccessDenied (403).
func (s *Service) mutate(ctx context.Context, id, userID vo.Id, fn func(p *model.Payee, now time.Time)) (*model.Payee, error) {
	var loaded *model.Payee
	err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		p, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if !p.UserID.Equal(userID) {
			// Mask a foreign-owned payee as not-found (matching the repo above), so
			// the response can't probe which payee ids exist.
			return errs.NewNotFound("Payee not found")
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
func (s *Service) mutateChecked(ctx context.Context, id, userID vo.Id, fn func(ctx context.Context, p *model.Payee, now time.Time) error) (*model.Payee, error) {
	var loaded *model.Payee
	err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		p, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if !p.UserID.Equal(userID) {
			// Mask a foreign-owned payee as not-found (matching the repo above), so
			// the response can't probe which payee ids exist.
			return errs.NewNotFound("Payee not found")
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

// toResult formats the timestamps in the "2006-01-02 15:04:05" wire form and
// maps the archived bool to the wire shape (isArchived int 0/1). See CLAUDE.md.
func toResult(p *model.Payee) model.PayeeResult {
	archived := 0
	if p.IsArchived {
		archived = 1
	}
	return model.PayeeResult{
		Id:          p.ID.String(),
		OwnerUserId: p.UserID.String(),
		Name:        p.Name,
		Position:    int(p.Position),
		IsArchived:  archived,
		CreatedAt:   p.CreatedAt.Format(datetime.Layout),
		UpdatedAt:   p.UpdatedAt.Format(datetime.Layout),
	}
}

// listResults returns the user's AVAILABLE payees (own + shared via account
// access), ordered by position, in the wire shape — used by order-payee-list.
// It reads through the same own+shared view as get-payee-list, not owner-only.
func (s *Service) listResults(ctx context.Context, userID vo.Id) ([]model.PayeeResult, error) {
	rows, err := s.read.PayeeListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]model.PayeeResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	return items, nil
}

// ensureNameUnique enforces the per-owner name-uniqueness rule. exceptID, when
// non-empty, is excluded from the comparison (for updates of the payee itself).
// The duplicate error message is exactly "Payee already exists." (wire-compat).
func (s *Service) ensureNameUnique(ctx context.Context, userID vo.Id, name string, exceptID vo.Id) error {
	payees, err := s.repo.ListByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, p := range payees {
		if p.Name == name && !p.ID.Equal(exceptID) {
			return &errs.ValidationError{Msg: "Payee already exists.", MsgCode: errs.CodePayeeAlreadyExists}
		}
	}
	return nil
}

// newPayeeName enforces the payee name invariant: rune length 3..64. The error
// message is EXACTLY "Payee name must be 3-64 characters" (wire-compat with
// existing API clients) and the field key is "name". See CLAUDE.md.
func newPayeeName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Payee name must be 3-64 characters",
			errs.FieldError{
				Key: "name", Message: "Payee name must be 3-64 characters", Code: errs.CodePayeeNameLength,
				Params: map[string]any{"min": 3, "max": 64},
			})
	}
	return v, nil
}
