// Service wiring for the category module: the use-case orchestrator, its
// dependency seams, and the shared private helpers (entity->DTO conversion and
// the value-object constructors).
package category

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountAccess resolves shared-account ownership/admin-grant for the
// create-for-account path: which user owns an account, and whether a connected
// user holds an admin grant on it. Backed by the connection module's
// AccountAccess repo (the connection/domconnection.Role comparison lives on
// that side, so this port stays free of connection's types).
type AccountAccess interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	HasAdminGrant(ctx context.Context, accountID, userID vo.Id) (bool, error)
}

// Service is the category write-side use-case orchestrator. It owns the tx
// boundary and builds the response-shaped *Result structs directly.
type Service struct {
	repo   Repository
	tx     port.TxRunner
	ops    port.OperationGuard
	clock  port.Clock
	read   ReadModel
	access AccountAccess
}

// NewService wires the category service. read is the own+shared category view
// (the same ReadModel get-category-list uses); order-category-list returns that
// full available list (own + shared, NOT owner-only). access resolves
// shared-account ownership for create-category-for-account. ops backs
// create-category's request-id idempotency (see CreateCategory).
func NewService(repo Repository, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, read ReadModel, access AccountAccess) *Service {
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
	isAdmin, err := s.access.HasAdminGrant(ctx, accountID, userID)
	if err != nil {
		return vo.Id{}, err
	}
	if isAdmin {
		return owner, nil
	}
	return vo.Id{}, errs.NewAccessDenied("Access is not allowed")
}

// mutate loads the category, checks ownership, applies fn inside a transaction,
// and saves. It returns the mutated (in-memory) aggregate so the caller can
// build its result without a second read. Ownership failure -> AccessDenied
// (403).
func (s *Service) mutate(ctx context.Context, id, userID vo.Id, fn func(c *Category, now time.Time)) (*Category, error) {
	var loaded *Category
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
func toResult(c *Category) CategoryResult {
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
func newCategoryType(alias string) (Type, error) {
	typ, ok := TypeFromAlias(alias)
	if !ok {
		return 0, errs.NewValidation("CategoryType not exists",
			errs.FieldError{Key: "type", Message: "CategoryType not exists"})
	}
	return typ, nil
}
