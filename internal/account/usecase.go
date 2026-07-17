// Service wiring for the account+folder module. The key shared helper is
// buildAccountResult, the embed builder that assembles the full AccountResult
// (owner + currency + folder + position + balance + sharedAccess); the
// transaction module reuses it via this service.
package account

import (
	"context"
	"sort"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// correctionComment is the description stamped on the balance-correction
// transaction that update-account writes. It is the resolved English string
// "Balance adjustment" (not a translation key), frozen as the stored/returned
// value.
const correctionComment = "Balance adjustment"

// Service is the account+folder write-side use-case orchestrator. It owns the tx
// boundary and builds the response-shaped *Result structs directly. The one
// Repository/FolderRepository constructor param each split into their role
// interfaces here so every use-case file references the narrowest surface it
// actually needs.
type Service struct {
	accounts    AccountStore
	positions   PositionStore
	balances    BalanceReader
	folders     FolderStore
	memberships FolderMembership
	currency    CurrencyLookup
	users       UserLookup
	shared      SharedAccessLookup
	revoker     AccessRevoker
	tx          port.TxRunner
	ops         port.OperationGuard
	clock       port.Clock
}

// NewService wires the account+folder service. shared/revoker may be nil (no
// connection module): then sharedAccess[] is always empty and a non-owner delete
// returns AccessDenied. ops backs create-account's request-id idempotency.
func NewService(
	repo Repository,
	folders FolderRepository,
	currency CurrencyLookup,
	users UserLookup,
	shared SharedAccessLookup,
	revoker AccessRevoker,
	tx port.TxRunner,
	ops port.OperationGuard,
	clock port.Clock,
) *Service {
	return &Service{
		accounts: repo, positions: repo, balances: repo,
		folders: folders, memberships: folders,
		currency: currency, users: users, shared: shared, revoker: revoker, tx: tx, ops: ops, clock: clock,
	}
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

// localNow is the caller's current wall-clock (server now rendered in the request
// timezone) as a UTC-typed time, so it stores as that bare wall-clock. A
// balance-correction transaction is dated with it so its spent_at sits within the
// caller's "today" and is counted by balanceBefore immediately — otherwise a
// server-UTC "now" can fall after the caller's day boundary (a behind-UTC caller,
// once UTC has rolled past midnight) and the opening balance reads 0.
func (s *Service) localNow(ctx context.Context) time.Time {
	return wallClockIn(s.clock.Now(), reqctx.Location(ctx))
}

// wallClockIn renders now's wall-clock in loc as a UTC-typed time (see tomorrowIn
// for why spent_at/cutoff wall-clocks must be UTC-typed).
func wallClockIn(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	y, m, d := local.Date()
	h, mi, sec := local.Clock()
	return time.Date(y, m, d, h, mi, sec, 0, time.UTC)
}

// buildAccountResult assembles the full AccountResult for one account as seen by
// userID: owner, currency (with Intl-resolved name), folderId (the first folder
// containing the account among the user's folders), per-user position, the
// supplied balance, and an empty sharedAccess (until the connection module
// lands). memberships maps folderID -> account ids (pass nil to load lazily).
func (s *Service) buildAccountResult(ctx context.Context, userID vo.Id, acct *model.Account, balance string, foldersSorted []*model.Folder, memberships map[string][]string, cache *accountEmbedCache) (model.AccountResult, error) {
	ownerRes, err := s.resolveOwner(ctx, cache, acct.UserID.String())
	if err != nil {
		return model.AccountResult{}, err
	}
	curRes, err := s.resolveCurrency(ctx, cache, acct.CurrencyID.String())
	if err != nil {
		return model.AccountResult{}, err
	}

	// folderId = first folder (by position) that contains the account.
	var folderID *string
	for _, f := range foldersSorted {
		ids := memberships[f.ID.String()]
		for _, aid := range ids {
			if aid == acct.ID.String() {
				v := f.ID.String()
				folderID = &v
				break
			}
		}
		if folderID != nil {
			break
		}
	}

	// position from accounts_options (0 if no row).
	pos, _, err := s.positions.GetPosition(ctx, acct.ID, userID)
	if err != nil {
		return model.AccountResult{}, err
	}

	shared, err := s.sharedAccessFor(ctx, acct.ID, cache)
	if err != nil {
		return model.AccountResult{}, err
	}

	return model.AccountResult{
		Id:           acct.ID.String(),
		Owner:        ownerRes,
		FolderId:     folderID,
		Name:         acct.Name,
		Position:     int(pos),
		Currency:     curRes,
		Balance:      vo.NewDecimal(balance).String(),
		Type:         int(acct.Type.Int16()),
		Icon:         acct.Icon,
		SharedAccess: shared,
	}, nil
}

// accountEmbedCache memoizes the owner and currency embeds across one account-list
// build. The owner (usually the user themselves) and currency (one or two) repeat
// across every account, so without this each account re-fetches them — the owner
// lookup alone is two queries (user row + options). A nil cache disables
// memoization (single-account callers like create/update pass nil).
type accountEmbedCache struct {
	owners     map[string]model.UserResult
	currencies map[string]model.CurrencyResult
}

func newAccountEmbedCache() *accountEmbedCache {
	return &accountEmbedCache{owners: map[string]model.UserResult{}, currencies: map[string]model.CurrencyResult{}}
}

// resolveOwner returns the owner embed for a user id, via the cache when present.
func (s *Service) resolveOwner(ctx context.Context, cache *accountEmbedCache, userID string) (model.UserResult, error) {
	if cache != nil {
		if o, ok := cache.owners[userID]; ok {
			return o, nil
		}
	}
	owner, err := s.users.GetOwner(ctx, userID)
	if err != nil {
		return model.UserResult{}, err
	}
	res := model.UserResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name}
	if cache != nil {
		cache.owners[userID] = res
	}
	return res, nil
}

// resolveCurrency returns the currency embed for a currency id, via the cache when present.
func (s *Service) resolveCurrency(ctx context.Context, cache *accountEmbedCache, currencyID string) (model.CurrencyResult, error) {
	if cache != nil {
		if c, ok := cache.currencies[currencyID]; ok {
			return c, nil
		}
	}
	cur, err := s.currency.GetByID(ctx, currencyID)
	if err != nil {
		return model.CurrencyResult{}, err
	}
	res := model.CurrencyResult{Id: cur.ID, Code: cur.Code, Name: cur.Name, Symbol: cur.Symbol, FractionDigits: cur.FractionDigits}
	if cache != nil {
		cache.currencies[currencyID] = res
	}
	return res, nil
}

// sharedAccessFor builds the account's sharedAccess[] embed: one entry per
// accounts_access grant, with the granted user resolved (id/avatar/name). Always
// returns a non-nil slice (empty when no connection module or no grants).
func (s *Service) sharedAccessFor(ctx context.Context, accountID vo.Id, cache *accountEmbedCache) ([]model.SharedAccess, error) {
	out := []model.SharedAccess{}
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
		out = append(out, model.SharedAccess{User: u, Role: g.Role})
	}
	return out, nil
}

// buildAccountList builds the AccountResult list for all the user's available
// accounts (bulk balances + memberships loaded once). When reversed is true the
// list is reversed (get-account-list reverses; order-account-list does not).
func (s *Service) buildAccountList(ctx context.Context, userID vo.Id, reversed bool) ([]model.AccountResult, error) {
	accts, err := s.accounts.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	balances, err := s.balances.Balances(ctx, userID, s.balanceBefore(ctx))
	if err != nil {
		return nil, err
	}
	folders, err := s.sortedFolders(ctx, userID)
	if err != nil {
		return nil, err
	}
	memberships, err := s.memberships.MembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	cache := newAccountEmbedCache()
	items := make([]model.AccountResult, 0, len(accts))
	for _, a := range accts {
		bal := balances[a.ID.String()]
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
func (s *Service) sortedFolders(ctx context.Context, userID vo.Id) ([]*model.Folder, error) {
	folders, err := s.folders.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(folders, func(i, j int) bool { return folders[i].Position < folders[j].Position })
	return folders, nil
}

// newAccountName enforces the account name invariant: rune length 3..64
// ("Account name must be 3-64 characters", field "name").
func newAccountName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Account name must be 3-64 characters",
			errs.FieldError{
				Key: "name", Message: "Account name must be 3-64 characters", Code: errs.CodeAccountNameLength,
				Params: map[string]any{"min": 3, "max": 64},
			})
	}
	return v, nil
}

// newFolderName enforces the folder name invariant: rune length 3..64
// ("Folder name must be 3-64 characters", field "name"). Shares its code with
// the budget feature's folder-name check (identical English text).
func newFolderName(v string) (string, error) {
	n := len([]rune(v))
	if n < 3 || n > 64 {
		return "", errs.NewValidation("Folder name must be 3-64 characters",
			errs.FieldError{
				Key: "name", Message: "Folder name must be 3-64 characters", Code: errs.CodeFolderNameLength,
				Params: map[string]any{"min": 3, "max": 64},
			})
	}
	return v, nil
}

// newIcon enforces the icon invariant: non-empty (field "icon").
func newIcon(v string) (string, error) {
	if v == "" {
		return "", errs.NewValidation("Icon value must not be empty",
			errs.FieldError{Key: "icon", Message: "Icon value must not be empty", Code: errs.CodeIconRequired})
	}
	return v, nil
}

// toFolderResult maps a Folder to its wire shape (isVisible int 0/1).
func toFolderResult(f *model.Folder) model.AccountFolderResult {
	vis := 0
	if f.IsVisible {
		vis = 1
	}
	return model.AccountFolderResult{
		Id:        f.ID.String(),
		Name:      f.Name,
		Position:  int(f.Position),
		IsVisible: vis,
	}
}
