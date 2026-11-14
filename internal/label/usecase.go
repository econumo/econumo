package label

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Service is the label write-side use-case orchestrator; it owns the tx boundary.
type Service struct {
	repo     Repository
	tx       port.TxRunner
	ops      port.OperationGuard
	clock    port.Clock
	read     ReadModel
	access   AccountAccess
	tagNames TagNames
}

// SetTagNames installs the shared-namespace peer. The label and tag services
// depend on each other, so neither constructor can take the other; this breaks
// the cycle and is called once at composition time (server.BuildAPI).
func (s *Service) SetTagNames(n TagNames) { s.tagNames = n }

// NamesByOwner satisfies the tag feature's LabelNames port (wired by server
// glue) so the other kind can enforce the shared name namespace.
func (s *Service) NamesByOwner(ctx context.Context, ownerID vo.Id) ([]string, error) {
	labels, err := s.repo.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(labels))
	for _, l := range labels {
		names = append(names, l.Name)
	}
	return names, nil
}

// NewService wires the label service. read is the own+shared label view (the
// same ReadModel get-label-list uses); move-label and sort-label-list return
// that full available list. access resolves shared-account ownership for
// create-label-for-account. ops backs create-label's request-id idempotency.
func NewService(repo Repository, tx port.TxRunner, ops port.OperationGuard, clock port.Clock, read ReadModel, access AccountAccess) *Service {
	return &Service{repo: repo, tx: tx, ops: ops, clock: clock, read: read, access: access}
}

// resolveAccountOwner returns the user a label created in the context of
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

// mutate loads the label, checks ownership, applies fn inside a transaction, and
// saves. It returns the mutated (in-memory) aggregate so the caller can build
// its result without a second read. Ownership failure is masked as a not-found
// (400), matching the repo lookup above, so a caller cannot probe which label
// ids exist by distinguishing "not yours" from "doesn't exist".
func (s *Service) mutate(ctx context.Context, id, userID vo.Id, fn func(l *model.Label, now time.Time)) (*model.Label, error) {
	var loaded *model.Label
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		l, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if !l.UserID.Equal(userID) {
			// Mask a foreign-owned label as not-found (matching the repo above), so
			// the response can't probe which label ids exist.
			return errs.NewNotFound("Label not found")
		}
		fn(l, s.clock.Now())
		if err := s.repo.Save(ctx, l); err != nil {
			return err
		}
		loaded = l
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
func (s *Service) mutateChecked(ctx context.Context, id, userID vo.Id, fn func(ctx context.Context, l *model.Label, now time.Time) error) (*model.Label, error) {
	var loaded *model.Label
	err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		l, err := s.repo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if !l.UserID.Equal(userID) {
			// Mask a foreign-owned label as not-found (matching the repo above), so
			// the response can't probe which label ids exist.
			return errs.NewNotFound("Label not found")
		}
		if ferr := fn(txCtx, l, s.clock.Now()); ferr != nil {
			return ferr
		}
		if err := s.repo.Save(txCtx, l); err != nil {
			return err
		}
		loaded = l
		return nil
	})
	if err != nil {
		return nil, err
	}
	return loaded, nil
}

// toResult formats the timestamps in the "2006-01-02 15:04:05" wire form and
// maps the archived bool to the wire shape (isArchived int 0/1). See CLAUDE.md.
func toResult(l *model.Label) model.LabelResult {
	archived := 0
	if l.IsArchived {
		archived = 1
	}
	return model.LabelResult{
		Id:          l.ID.String(),
		OwnerUserId: l.UserID.String(),
		Name:        l.Name,
		Icon:        l.Icon,
		IsArchived:  archived,
		CreatedAt:   l.CreatedAt.Format(datetime.Layout),
		UpdatedAt:   l.UpdatedAt.Format(datetime.Layout),
	}
}

// listResults returns the user's AVAILABLE labels (own + shared via account
// access), in list order, in the wire shape — used by move-label and
// sort-label-list. It reads through the same own+shared view as get-label-list,
// not owner-only.
func (s *Service) listResults(ctx context.Context, userID vo.Id) ([]model.LabelResult, error) {
	rows, err := s.read.LabelListView(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	items := make([]model.LabelResult, 0, len(rows))
	for _, r := range rows {
		items = append(items, toViewResult(r))
	}
	assignPositions(items)
	return items, nil
}

// assignPositions stamps the dense 0-based index the wire contract calls
// "position". The stored sort key never leaves the server, so this index is what
// clients order by; it is derived from the already-sorted list rather than read
// from a column.
func assignPositions(items []model.LabelResult) {
	for i := range items {
		items[i].Position = i
	}
}

// itemResult builds a single-item write response, stamping the dense index the
// item occupies in the caller's available list. Single-item responses must derive
// "position" exactly like list responses do, because the entity no longer carries
// one -- the stored sort key never leaves the server.
func (s *Service) itemResult(ctx context.Context, userID vo.Id, l *model.Label) (model.LabelResult, error) {
	res := toResult(l)
	items, err := s.listResults(ctx, userID)
	if err != nil {
		return model.LabelResult{}, err
	}
	id := l.ID.String()
	for i, it := range items {
		if it.Id == id {
			res.Position = i
			break
		}
	}
	return res, nil
}

// ensureNameUnique enforces the per-owner name-uniqueness rule. exceptID, when
// non-empty, is excluded from the comparison (for updates of the label itself).
// Both kinds share one name namespace, so the duplicate error is the tag
// code/message ("Tag already exists.") regardless of which kind holds the name.
func (s *Service) ensureNameUnique(ctx context.Context, userID vo.Id, name string, exceptID vo.Id) error {
	labels, err := s.repo.ListByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, l := range labels {
		// Case-insensitive: one name means one label regardless of case, matching
		// how CSV import already resolves names.
		if strings.EqualFold(l.Name, name) && !l.ID.Equal(exceptID) {
			return &errs.ValidationError{Msg: "Tag already exists.", MsgCode: errs.CodeTagAlreadyExists}
		}
	}
	// The other classification kind shares this namespace. exceptID needs no
	// counterpart here: ids are unique across both tables, so a rename can never
	// collide with itself.
	if s.tagNames == nil {
		return nil
	}
	names, err := s.tagNames.NamesByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return &errs.ValidationError{Msg: "Tag already exists.", MsgCode: errs.CodeTagAlreadyExists}
		}
	}
	return nil
}

// newLabelName enforces the label name invariant: rune length 3..64. The
// message is exactly "Label name must be 3-64 characters" and the field key is
// "name".
func newLabelName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Label name must be 3-64 characters",
			errs.FieldError{
				Key: "name", Message: "Label name must be 3-64 characters", Code: errs.CodeLabelNameLength,
				Params: map[string]any{"min": 3, "max": 64},
			})
	}
	return v, nil
}
