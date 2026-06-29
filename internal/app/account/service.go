// Service wiring for the account+folder module. The key shared helper is
// buildAccountResult, the embed builder that assembles the full AccountResult
// (owner + currency + folder + position + balance + sharedAccess); the
// transaction module reuses it via this service.
package account

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/app/reqctx"
	domaccount "github.com/econumo/econumo/internal/domain/account"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// correctionComment is the description stamped on the balance-correction
// transaction that update-account writes. It is the resolved English string
// "Balance adjustment" (not a translation key), frozen as the stored/returned
// value.
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
// has any grant on the account (the gate for deleting it). Satisfied by an
// adapter over the connection service. May be nil (no connection module) ->
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

// balanceBefore is the exclusive upper bound for the balance SUM: the start of
// tomorrow, so the balance includes everything spent through end of today and
// excludes future-dated transactions. "Today" is the CALLER's day (from the
// request timezone), not the server's UTC day — see tomorrowIn.
func (s *Service) balanceBefore(ctx context.Context) time.Time {
	return tomorrowIn(s.clock.Now(), reqctx.Location(ctx))
}

// tomorrowIn returns the start of the day after now's calendar date in loc,
// expressed as a UTC-typed wall-clock time.
//
// spent_at is stored as a naive "Y-m-d H:i:s" wall-clock (the date the user
// picked, no zone), and the balance query compares spent_at < cutoff as such.
// We therefore take the user's LOCAL calendar date (now.In(loc)) but build the
// cutoff as a UTC-typed time, so the DB driver serializes it as that bare
// wall-clock — making "balance as of end of today" use the USER's day boundary.
//
// Computing the boundary in UTC instead lets a user behind UTC see their
// next-day, future transactions counted: once UTC has rolled past midnight, the
// server's "tomorrow" is two of the user's days away. Using the user's day is an
// intentional, documented choice.
func tomorrowIn(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
}

// buildAccountResult assembles the full AccountResult for one account as seen by
// userID: owner, currency (with Intl-resolved name), folderId (the first folder
// containing the account among the user's folders), per-user position, the
// supplied balance, and an empty sharedAccess (until the connection module
// lands). memberships maps folderID -> account ids (pass nil to load lazily).
func (s *Service) buildAccountResult(ctx context.Context, userID vo.Id, acct *domaccount.Account, balance string, foldersSorted []*domaccount.Folder, memberships map[string][]string, cache *accountEmbedCache) (AccountResult, error) {
	ownerRes, err := s.resolveOwner(ctx, cache, acct.UserId().String())
	if err != nil {
		return AccountResult{}, err
	}
	curRes, err := s.resolveCurrency(ctx, cache, acct.CurrencyId().String())
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

	shared, err := s.sharedAccessFor(ctx, acct.Id(), cache)
	if err != nil {
		return AccountResult{}, err
	}

	return AccountResult{
		Id:           acct.Id().String(),
		Owner:        ownerRes,
		FolderId:     folderID,
		Name:         acct.Name(),
		Position:     int(pos),
		Currency:     curRes,
		Balance:      vo.NewDecimal(balance).String(),
		Type:         int(acct.Type().Int16()),
		Icon:         acct.Icon(),
		SharedAccess: shared,
	}, nil
}

// accountEmbedCache memoizes the owner and currency embeds across one account-list
// build. The owner (usually the user themselves) and currency (one or two) repeat
// across every account, so without this each account re-fetches them — the owner
// lookup alone is two queries (user row + options). A nil cache disables
// memoization (single-account callers like create/update pass nil).
type accountEmbedCache struct {
	owners     map[string]OwnerResult
	currencies map[string]CurrencyResult
}

func newAccountEmbedCache() *accountEmbedCache {
	return &accountEmbedCache{owners: map[string]OwnerResult{}, currencies: map[string]CurrencyResult{}}
}

// resolveOwner returns the owner embed for a user id, via the cache when present.
func (s *Service) resolveOwner(ctx context.Context, cache *accountEmbedCache, userID string) (OwnerResult, error) {
	if cache != nil {
		if o, ok := cache.owners[userID]; ok {
			return o, nil
		}
	}
	owner, err := s.users.GetOwner(ctx, userID)
	if err != nil {
		return OwnerResult{}, err
	}
	res := OwnerResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name}
	if cache != nil {
		cache.owners[userID] = res
	}
	return res, nil
}

// resolveCurrency returns the currency embed for a currency id, via the cache when present.
func (s *Service) resolveCurrency(ctx context.Context, cache *accountEmbedCache, currencyID string) (CurrencyResult, error) {
	if cache != nil {
		if c, ok := cache.currencies[currencyID]; ok {
			return c, nil
		}
	}
	cur, err := s.currency.GetByID(ctx, currencyID)
	if err != nil {
		return CurrencyResult{}, err
	}
	res := CurrencyResult{Id: cur.ID, Code: cur.Code, Name: cur.Name, Symbol: cur.Symbol, FractionDigits: cur.FractionDigits}
	if cache != nil {
		cache.currencies[currencyID] = res
	}
	return res, nil
}

// sharedAccessFor builds the account's sharedAccess[] embed: one entry per
// accounts_access grant, with the granted user resolved (id/avatar/name). Always
// returns a non-nil slice (empty when no connection module or no grants).
func (s *Service) sharedAccessFor(ctx context.Context, accountID vo.Id, cache *accountEmbedCache) ([]SharedAccess, error) {
	out := []SharedAccess{}
	if s.shared == nil {
		return out, nil
	}
	grants, err := s.shared.ListByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		u, uerr := s.resolveOwner(ctx, cache, g.UserID)
		if uerr != nil {
			return nil, uerr
		}
		out = append(out, SharedAccess{User: u, Role: g.Role})
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
	balances, err := s.repo.Balances(ctx, userID, s.balanceBefore(ctx))
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

	cache := newAccountEmbedCache()
	items := make([]AccountResult, 0, len(accts))
	for _, a := range accts {
		bal := balances[a.Id().String()]
		if bal == "" {
			bal = "0"
		}
		item, berr := s.buildAccountResult(ctx, userID, a, bal, folders, memberships, cache)
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
