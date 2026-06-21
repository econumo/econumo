// Service wiring: the use-case orchestrator, its dependency seams, the
// constructor, and the shared private helpers (entity->DTO conversion, the
// value-object constructors, and the operation-id idempotency helper). The
// individual use cases live in sibling files (create.go, update.go, archive.go,
// delete.go, order.go); the pure read lives in read.go.
package category

import (
	"context"
	"strings"
	"time"

	domcategory "github.com/econumo/econumo/internal/domain/category"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// apiDatetimeLayout is the wire format for createdAt/updatedAt: "2006-01-02
// 15:04:05" (space separator, no timezone). See COMPATIBILITY.md.
const apiDatetimeLayout = "2006-01-02 15:04:05"

// defaultIcon is the create fallback: an empty icon becomes "local_offer".
const defaultIcon = "local_offer"

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

// Service is the category write-side use-case orchestrator. It owns the tx
// boundary and builds the response-shaped *Result structs directly.
type Service struct {
	repo  domcategory.Repository
	tx    TxRunner
	ops   OperationGuard
	clock Clock
	read  ReadModel
}

// NewService wires the category service. read is the own+shared category view
// (the same ReadModel get-category-list uses); order-category-list returns that
// full available list, mirroring PHP's OrderCategoryListV1ResultAssembler, which
// calls findAvailableForUserId (NOT owner-only).
func NewService(repo domcategory.Repository, tx TxRunner, ops OperationGuard, clock Clock, read ReadModel) *Service {
	return &Service{repo: repo, tx: tx, ops: ops, clock: clock, read: read}
}

// ---------------------------------------------------------------------------
// shared private helpers used across the use cases
// ---------------------------------------------------------------------------

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
			// PHP throws a bare AccessDeniedException() here (CategoryService
			// updateCategory/archive/etc.), so the 403 envelope message is EMPTY.
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
// the wire shapes (isArchived int 0/1, type alias string). See COMPATIBILITY.md.
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
		CreatedAt:   c.CreatedAt().Format(apiDatetimeLayout),
		UpdatedAt:   c.UpdatedAt().Format(apiDatetimeLayout),
	}
}

// listResults returns the user's AVAILABLE categories (own + shared via account
// access), ordered by position, in the wire shape — used by order-category-list.
// It reads through the same own+shared view as get-category-list so the order
// response carries the full list (PHP's assembler uses findAvailableForUserId,
// not owner-only).
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

// ---------------------------------------------------------------------------
// tier-2 value-object constructors (category-module invariants)
// ---------------------------------------------------------------------------

// newCategoryName enforces the category name invariant: rune length 3..64. The
// error message is EXACTLY "Category name must be 3-64 characters" (wire-compat
// with existing API clients) and the field key is "name". See COMPATIBILITY.md.
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

// newCategoryType parses a type alias: lowercase+trim, accept only
// "expense"/"income". The field key is "type".
func newCategoryType(alias string) (domcategory.Type, error) {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case aliasExpense:
		return domcategory.TypeExpense, nil
	case aliasIncome:
		return domcategory.TypeIncome, nil
	default:
		return 0, errs.NewValidation("CategoryType not exists",
			errs.FieldError{Key: "type", Message: "CategoryType not exists"})
	}
}

const (
	aliasExpense = "expense"
	aliasIncome  = "income"
)
