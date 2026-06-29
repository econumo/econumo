// Service wiring for the category module: the use-case orchestrator, its
// dependency seams, and the shared private helpers (entity->DTO conversion and
// the value-object constructors).
package category

import (
	"context"
	"time"

	domcategory "github.com/econumo/econumo/internal/domain/category"
	domconnection "github.com/econumo/econumo/internal/domain/connection"
	"github.com/econumo/econumo/internal/domain/shared/datetime"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

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

// OperationGuard provides the row-based idempotency for create-category. Claim
// attempts to record the request id; it reports
// already=true when the id was previously claimed (a duplicate request) so the
// caller can reject it. See claimOperation for the semantics.
type OperationGuard interface {
	// Claim inserts the id into operation_requests_ids. Returns already=true if a
	// row for the id already existed (duplicate). Runs inside the caller's tx.
	Claim(ctx context.Context, id vo.Id, now time.Time) (already bool, err error)
	// MarkHandled flips is_handled to true after the operation succeeds.
	MarkHandled(ctx context.Context, id vo.Id, now time.Time) error
}

// AccountAccess resolves shared-account ownership/role for the
// create-for-account path: which user owns an account, and what role a connected
// user holds on it. Backed by the connection module's AccountAccess repo. A
// missing grant is reported as ok=false (nil error).
type AccountAccess interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	GrantRole(ctx context.Context, accountID, userID vo.Id) (role domconnection.Role, ok bool, err error)
}

// Service is the category write-side use-case orchestrator. It owns the tx
// boundary and builds the response-shaped *Result structs directly.
type Service struct {
	repo   domcategory.Repository
	tx     TxRunner
	ops    OperationGuard
	clock  Clock
	read   ReadModel
	access AccountAccess
}

// NewService wires the category service. read is the own+shared category view
// (the same ReadModel get-category-list uses); order-category-list returns that
// full available list (own + shared, NOT owner-only). access resolves
// shared-account ownership for create-category-for-account.
func NewService(repo domcategory.Repository, tx TxRunner, ops OperationGuard, clock Clock, read ReadModel, access AccountAccess) *Service {
	return &Service{repo: repo, tx: tx, ops: ops, clock: clock, read: read, access: access}
}

// resolveAccountOwner returns the user a category created in the context of
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
	role, ok, err := s.access.GrantRole(ctx, accountID, userID)
	if err != nil {
		return vo.Id{}, err
	}
	if ok && role == domconnection.RoleAdmin {
		return owner, nil
	}
	return vo.Id{}, errs.NewAccessDenied("Access is not allowed")
}

// mutate loads the category, checks ownership, applies fn inside a transaction,
// and saves. It returns the mutated (in-memory) aggregate so the caller can
// build its result without a second read. Ownership failure -> AccessDenied
// (403).
func (s *Service) mutate(ctx context.Context, id, userID vo.Id, fn func(c *domcategory.Category, now time.Time)) (*domcategory.Category, error) {
	var loaded *domcategory.Category
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if !c.UserId().Equal(userID) {
			// The 403 envelope message is intentionally EMPTY here (frozen wire
			// behaviour for the ownership-denied path).
			return errs.NewAccessDenied("")
		}
		fn(c, s.clock.Now())
		if err := s.repo.Save(ctx, c); err != nil {
			return err
		}
		loaded = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return loaded, nil
}

// toResult is the single entity->DTO conversion in the module. It formats the
// timestamps in the "2006-01-02 15:04:05" wire form and maps the bool/type to
// the wire shapes (isArchived int 0/1, type alias string). See CLAUDE.md.
func toResult(c *domcategory.Category) CategoryResult {
	archived := 0
	if c.IsArchived() {
		archived = 1
	}
	return CategoryResult{
		Id:          c.Id().String(),
		OwnerUserId: c.UserId().String(),
		Name:        c.Name(),
		Position:    int(c.Position()),
		Type:        c.Type().Alias(),
		Icon:        c.Icon(),
		IsArchived:  archived,
		CreatedAt:   c.CreatedAt().Format(datetime.Layout),
		UpdatedAt:   c.UpdatedAt().Format(datetime.Layout),
	}
}

// listResults returns the user's AVAILABLE categories (own + shared via account
// access), ordered by position, in the wire shape — used by order-category-list.
// It reads through the same own+shared view as get-category-list so the order
// response carries the full list (own + shared, not owner-only).
func (s *Service) listResults(ctx context.Context, userID vo.Id) ([]CategoryResult, error) {
	rows, err := s.read.CategoryListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]CategoryResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	return items, nil
}

// newCategoryName enforces the category name invariant: rune length 3..64. The
// error message is EXACTLY "Category name must be 3-64 characters" (wire-compat
// with existing API clients) and the field key is "name". See CLAUDE.md.
func newCategoryName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Category name must be 3-64 characters",
			errs.FieldError{Key: "name", Message: "Category name must be 3-64 characters"})
	}
	return v, nil
}

// newIcon enforces the icon invariant: must not be empty. The field key is "icon".
func newIcon(v string) (string, error) {
	if v == "" {
		return "", errs.NewValidation("Icon value must not be empty",
			errs.FieldError{Key: "icon", Message: "Icon value must not be empty"})
	}
	return v, nil
}

// newCategoryType parses a type alias via the domain parser, accepting only
// "expense"/"income". The field key is "type".
func newCategoryType(alias string) (domcategory.Type, error) {
	typ, ok := domcategory.TypeFromAlias(alias)
	if !ok {
		return 0, errs.NewValidation("CategoryType not exists",
			errs.FieldError{Key: "type", Message: "CategoryType not exists"})
	}
	return typ, nil
}
