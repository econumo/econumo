// Service wiring for the account+folder module: the use-case orchestrator, its
// dependency seams, the constructor, the value-object constructors, and the
// shared helpers — most importantly buildAccountResult, the embed builder that
// assembles the full AccountResult (owner + currency + folder + position +
// balance + sharedAccess). The transaction module reuses buildAccountResult
// later via this service.
//
// Individual use cases live in sibling files (create.go, update.go, delete.go,
// order.go, folder.go); the pure reads live in read.go.
package account

import (
	"context"
	"sort"
	"time"

	domaccount "github.com/econumo/econumo/internal/domain/account"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// apiDatetimeLayout is the wire format for dates ("2006-01-02 15:04:05").
const apiDatetimeLayout = "2006-01-02 15:04:05"

// correctionComment is the description stamped on the balance-correction
// transaction that update-account writes. PHP uses
// trans('account.correction.message'); that key IS defined in
// translations/messages.en.yaml as "Balance adjustment" (default_locale=en), so
// the stored/returned description is the translated string, not the key.
const correctionComment = "Balance adjustment"

// Clock supplies the current time (seam for deterministic tests).
type Clock interface {
	Now() time.Time
}

// TxRunner is the transaction boundary the service owns.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// OperationGuard is the shared idempotency guard (create-account has an
// OperationId). The shared operation.Guard satisfies it.
type OperationGuard interface {
	Claim(ctx context.Context, id vo.Id, now time.Time) (already bool, err error)
	MarkHandled(ctx context.Context, id vo.Id, now time.Time) error
}

// CurrencyView is the embeddable currency shape the account result needs. The
// currencyrepo.Lookup.GetByID returns it (display name already resolved).
type CurrencyView struct {
	ID             string
	Code           string
	Name           string
	Symbol         string
	FractionDigits int
}

// CurrencyLookup resolves a currency by id for the account-result embed.
type CurrencyLookup interface {
	GetByID(ctx context.Context, id string) (CurrencyView, error)
}

// OwnerView is the minimal owner shape the account result embeds.
type OwnerView struct {
	ID     string
	Name   string
	Avatar string
}

// UserLookup resolves the owner (id, name, avatar) for the account-result embed.
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (OwnerView, error)
}

// SharedAccessView is one accounts_access grant on an account: the granted
// user's id + the role alias. The owner embed (name/avatar) is resolved by the
// account service via UserLookup.
type SharedAccessView struct {
	UserID string
	Role   string
}

// SharedAccessLookup lists the accounts_access grants on an account (for the
// account result's sharedAccess[] embed). Satisfied by an adapter over the
// connection repo. A nil lookup means "no connection module" -> empty slice.
type SharedAccessLookup interface {
	ListByAccount(ctx context.Context, accountID vo.Id) ([]SharedAccessView, error)
}

// AccessRevoker drops the caller's own grant on a shared account (the
// delete-account non-owner branch). HasAccess reports whether the user owns or
// has any grant on the account (PHP canDeleteAccount = hasAccess). Satisfied by
// an adapter over the connection service. May be nil (no connection module) ->
// non-owner delete falls back to AccessDenied.
type AccessRevoker interface {
	HasAccess(ctx context.Context, userID, accountID vo.Id) (bool, error)
	RevokeOwnAccess(ctx context.Context, userID, accountID vo.Id) error
}

// Service is the account+folder write-side use-case orchestrator. It owns the tx
// boundary and builds the response-shaped *Result structs directly.
type Service struct {
	repo     domaccount.Repository
	folders  domaccount.FolderRepository
	currency CurrencyLookup
	users    UserLookup
	shared   SharedAccessLookup
	revoker  AccessRevoker
	tx       TxRunner
	ops      OperationGuard
	clock    Clock
}

// NewService wires the account+folder service. shared/revoker may be nil (no
// connection module): then sharedAccess[] is always empty and a non-owner delete
// returns AccessDenied.
func NewService(
	repo domaccount.Repository,
	folders domaccount.FolderRepository,
	currency CurrencyLookup,
	users UserLookup,
	shared SharedAccessLookup,
	revoker AccessRevoker,
	tx TxRunner,
	ops OperationGuard,
	clock Clock,
) *Service {
	return &Service{repo: repo, folders: folders, currency: currency, users: users, shared: shared, revoker: revoker, tx: tx, ops: ops, clock: clock}
}

// ---------------------------------------------------------------------------
// balance time
// ---------------------------------------------------------------------------

// balanceBefore is the exclusive upper bound for the balance SUM: the start of
// tomorrow (today's date + 1 day at 00:00:00), so the balance includes
// everything spent through today. Mirrors PHP datetimeService->getNextDay().
func (s *Service) balanceBefore() time.Time {
	now := s.clock.Now()
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
}

// ---------------------------------------------------------------------------
// the embed builder (reused by the transaction module later)
// ---------------------------------------------------------------------------

// buildAccountResult assembles the full AccountResult for one account as seen by
// userID: owner, currency (with Intl-resolved name), folderId (the first folder
// containing the account among the user's folders), per-user position, the
// supplied balance, and an empty sharedAccess (until the connection module
// lands). memberships maps folderID -> account ids (pass nil to load lazily).
func (s *Service) buildAccountResult(ctx context.Context, userID vo.Id, acct *domaccount.Account, balance string, foldersSorted []*domaccount.Folder, memberships map[string][]string) (AccountResult, error) {
	owner, err := s.users.GetOwner(ctx, acct.UserId().String())
	if err != nil {
		return AccountResult{}, err
	}
	cur, err := s.currency.GetByID(ctx, acct.CurrencyId().String())
	if err != nil {
		return AccountResult{}, err
	}

	// folderId = first folder (by position) that contains the account.
	var folderID *string
	for _, f := range foldersSorted {
		ids := memberships[f.Id().String()]
		for _, aid := range ids {
			if aid == acct.Id().String() {
				v := f.Id().String()
				folderID = &v
				break
			}
		}
		if folderID != nil {
			break
		}
	}

	// position from accounts_options (0 if no row).
	pos, _, err := s.repo.GetPosition(ctx, acct.Id(), userID)
	if err != nil {
		return AccountResult{}, err
	}

	shared, err := s.sharedAccessFor(ctx, acct.Id())
	if err != nil {
		return AccountResult{}, err
	}

	return AccountResult{
		Id:       acct.Id().String(),
		Owner:    OwnerResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name},
		FolderId: folderID,
		Name:     acct.Name(),
		Position: int(pos),
		Currency: CurrencyResult{
			Id:             cur.ID,
			Code:           cur.Code,
			Name:           cur.Name,
			Symbol:         cur.Symbol,
			FractionDigits: cur.FractionDigits,
		},
		Balance:      vo.NewDecimal(balance).String(),
		Type:         int(acct.Type().Int16()),
		Icon:         acct.Icon(),
		SharedAccess: shared,
	}, nil
}

// sharedAccessFor builds the account's sharedAccess[] embed: one entry per
// accounts_access grant, with the granted user resolved (id/avatar/name). Always
// returns a non-nil slice (empty when no connection module or no grants).
func (s *Service) sharedAccessFor(ctx context.Context, accountID vo.Id) ([]SharedAccess, error) {
	out := []SharedAccess{}
	if s.shared == nil {
		return out, nil
	}
	grants, err := s.shared.ListByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		u, uerr := s.users.GetOwner(ctx, g.UserID)
		if uerr != nil {
			return nil, uerr
		}
		out = append(out, SharedAccess{
			User: OwnerResult{Id: u.ID, Avatar: u.Avatar, Name: u.Name},
			Role: g.Role,
		})
	}
	return out, nil
}

// buildAccountList builds the AccountResult list for all the user's available
// accounts (bulk balances + memberships loaded once). When reversed is true the
// list is reversed (get-account-list reverses; order-account-list does not).
func (s *Service) buildAccountList(ctx context.Context, userID vo.Id, reversed bool) ([]AccountResult, error) {
	accts, err := s.repo.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	balances, err := s.repo.Balances(ctx, userID, s.balanceBefore())
	if err != nil {
		return nil, err
	}
	folders, err := s.sortedFolders(ctx, userID)
	if err != nil {
		return nil, err
	}
	memberships, err := s.folders.MembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	items := make([]AccountResult, 0, len(accts))
	for _, a := range accts {
		bal := balances[a.Id().String()]
		if bal == "" {
			bal = "0"
		}
		item, berr := s.buildAccountResult(ctx, userID, a, bal, folders, memberships)
		if berr != nil {
			return nil, berr
		}
		items = append(items, item)
	}
	if reversed {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
	return items, nil
}

// sortedFolders returns the user's folders ordered by position (ascending).
func (s *Service) sortedFolders(ctx context.Context, userID vo.Id) ([]*domaccount.Folder, error) {
	folders, err := s.folders.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(folders, func(i, j int) bool { return folders[i].Position() < folders[j].Position() })
	return folders, nil
}

// ---------------------------------------------------------------------------
// tier-2 value-object constructors
// ---------------------------------------------------------------------------

// newAccountName enforces the account name invariant: rune length 3..64
// ("Account name must be 3-64 characters", field "name").
func newAccountName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Account name must be 3-64 characters",
			errs.FieldError{Key: "name", Message: "Account name must be 3-64 characters"})
	}
	return v, nil
}

// newFolderName enforces the folder name invariant: rune length 3..64
// ("Folder name must be 3-64 characters", field "name").
func newFolderName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Folder name must be 3-64 characters",
			errs.FieldError{Key: "name", Message: "Folder name must be 3-64 characters"})
	}
	return v, nil
}

// newIcon enforces the icon invariant: non-empty (field "icon").
func newIcon(v string) (string, error) {
	if v == "" {
		return "", errs.NewValidation("Icon value must not be empty",
			errs.FieldError{Key: "icon", Message: "Icon value must not be empty"})
	}
	return v, nil
}

// toFolderResult maps a Folder to its wire shape (isVisible int 0/1).
func toFolderResult(f *domaccount.Folder) FolderResult {
	vis := 0
	if f.IsVisible() {
		vis = 1
	}
	return FolderResult{
		Id:        f.Id().String(),
		Name:      f.Name(),
		Position:  int(f.Position()),
		IsVisible: vis,
	}
}
