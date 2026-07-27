package tag

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Service is the tag write-side use-case orchestrator; it owns the tx boundary.
type Service struct {
	repo   Repository
	tx     port.TxRunner
	ops    port.OperationGuard
	clock  port.Clock
	read   ReadModel
	access AccountAccess
}

// NewService wires the tag service. read is the own+shared tag view (the same
// ReadModel get-tag-list uses); order-tag-list returns that full available list.
// access resolves shared-account ownership for create-tag-for-account. ops
// backs create-tag's request-id idempotency.
func NewService(repo Repository, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, read ReadModel, access AccountAccess) *Service {
	return &Service{repo: repo, tx: tx, ops: ops, clock: clock, read: read, access: access}
}

// resolveAccountOwner returns the user a tag created in the context of accountID
// must be owned by — the account owner. The caller must own the account or hold
// an admin grant on it; otherwise AccessDenied.
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

// mutate loads the tag, checks ownership, applies fn inside a transaction, and
// saves. It returns the mutated (in-memory) aggregate so the caller can build
// its result without a second read. Ownership failure -> AccessDenied (403).
func (s *Service) mutate(ctx context.Context, id, userID vo.Id, fn func(t *model.Tag, now time.Time)) (*model.Tag, error) {
	var loaded *model.Tag
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		t, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if !t.UserID.Equal(userID) {
			// Mask a foreign-owned tag as not-found (matching the repo above), so
			// the response can't probe which tag ids exist.
			return errs.NewNotFound("Tag not found")
		}
		fn(t, s.clock.Now())
		if err := s.repo.Save(ctx, t); err != nil {
			return err
		}
		loaded = t
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
func (s *Service) mutateChecked(ctx context.Context, id, userID vo.Id, fn func(ctx context.Context, t *model.Tag, now time.Time) error) (*model.Tag, error) {
	var loaded *model.Tag
	err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		t, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if !t.UserID.Equal(userID) {
			// Mask a foreign-owned tag as not-found (matching the repo above), so
			// the response can't probe which tag ids exist.
			return errs.NewNotFound("Tag not found")
		}
		if ferr := fn(txCtx, t, s.clock.Now()); ferr != nil {
			return ferr
		}
		if err := s.repo.Save(txCtx, t); err != nil {
			return err
		}
		loaded = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return loaded, nil
}

// toResult formats the timestamps in the "2006-01-02 15:04:05" wire form and
// maps the archived bool to the wire shape (isArchived int 0/1). See CLAUDE.md.
func toResult(t *model.Tag) model.TagResult {
	archived := 0
	if t.IsArchived {
		archived = 1
	}
	return model.TagResult{
		Id:          t.ID.String(),
		OwnerUserId: t.UserID.String(),
		Name:        t.Name,
		Position:    int(t.Position),
		IsArchived:  archived,
		CreatedAt:   t.CreatedAt.Format(datetime.Layout),
		UpdatedAt:   t.UpdatedAt.Format(datetime.Layout),
	}
}

// listResults returns the user's AVAILABLE tags (own + shared via account
// access), ordered by position, in the wire shape — used by order-tag-list. It
// reads through the same own+shared view as get-tag-list, not owner-only.
func (s *Service) listResults(ctx context.Context, userID vo.Id) ([]model.TagResult, error) {
	rows, err := s.read.TagListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]model.TagResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	return items, nil
}

// ensureNameUnique enforces the per-owner name-uniqueness rule. exceptID, when
// non-empty, is excluded from the comparison (for updates of the tag itself).
// The duplicate error message is exactly "Tag already exists." (wire-compat).
func (s *Service) ensureNameUnique(ctx context.Context, userID vo.Id, name string, exceptID vo.Id) error {
	tags, err := s.repo.ListByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, t := range tags {
		if t.Name == name && !t.ID.Equal(exceptID) {
			return &errs.ValidationError{Msg: "Tag already exists.", MsgCode: errs.CodeTagAlreadyExists}
		}
	}
	return nil
}

// newTagName enforces the tag name invariant: rune length 3..64. The error
// message is EXACTLY "Tag name must be 3-64 characters" (wire-compat with
// existing API clients) and the field key is "name". See CLAUDE.md.
func newTagName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Tag name must be 3-64 characters",
			errs.FieldError{
				Key: "name", Message: "Tag name must be 3-64 characters", Code: errs.CodeTagNameLength,
				Params: map[string]any{"min": 3, "max": 64},
			})
	}
	return v, nil
}
