// Adapters that satisfy the connection service's FolderPort, OptionPort, and
// UserLookup ports by delegating to the account module's folder/account repos
// and the user repo. They live here (infra) so app/connection depends only on
// its own small interfaces.
package connectionrepo

import (
	"context"
	"errors"
	"time"

	appaccount "github.com/econumo/econumo/internal/app/account"
	appconnection "github.com/econumo/econumo/internal/app/connection"
	domaccount "github.com/econumo/econumo/internal/domain/account"
	domconnection "github.com/econumo/econumo/internal/domain/connection"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domuser "github.com/econumo/econumo/internal/domain/user"
)

// --- AccountAccessResolver over the AccountAccess repo ---

// accountAccessFull is the subset of the connection AccountAccess repo used to
// resolve an account's owner and a connected user's grant role for the
// create-for-account path (category/tag create with an accountId).
type accountAccessFull interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	Get(ctx context.Context, accountID, userID vo.Id) (*domconnection.AccountAccess, error)
}

// AccountAccessResolver answers "who owns this account" and "what role does this
// user hold on it" — the two questions the category and tag create-for-account
// paths need to mirror PHP AccountAccessService.checkAddCategory/checkAddTag
// (== isAdmin) and createCategoryForAccount/createTagForAccount (ownership goes
// to the account owner). It structurally satisfies the AccountAccess port that
// the category and tag app services declare.
type AccountAccessResolver struct{ access accountAccessFull }

// NewAccountAccessResolver wraps the connection AccountAccess repo.
func NewAccountAccessResolver(access accountAccessFull) *AccountAccessResolver {
	return &AccountAccessResolver{access: access}
}

// AccountOwner returns the owner user id of an account.
func (r *AccountAccessResolver) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return r.access.AccountOwner(ctx, accountID)
}

// GrantRole returns the user's granted role on the account. A missing grant
// (the repo's *errs.NotFoundError) is reported as ok=false, nil error; other
// errors propagate.
func (r *AccountAccessResolver) GrantRole(ctx context.Context, accountID, userID vo.Id) (domconnection.Role, bool, error) {
	grant, err := r.access.Get(ctx, accountID, userID)
	if err != nil {
		var nf *errs.NotFoundError
		if errors.As(err, &nf) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return grant.Role(), true, nil
}

// --- FolderPort over the account FolderRepository ---

// folderRepo is the slice of the account FolderRepository the connection side
// effects need.
type folderRepo interface {
	ListByUser(ctx context.Context, userID vo.Id) ([]*domaccount.Folder, error)
	MembershipsByUser(ctx context.Context, userID vo.Id) (map[string][]string, error)
	AddAccount(ctx context.Context, folderID, accountID vo.Id) error
	RemoveAccount(ctx context.Context, folderID, accountID vo.Id) error
}

// FolderPort adapts the account FolderRepository to app/connection.FolderPort.
type FolderPort struct{ folders folderRepo }

var _ appconnection.FolderPort = (*FolderPort)(nil)

// NewFolderPort wraps an account FolderRepository.
func NewFolderPort(folders folderRepo) *FolderPort { return &FolderPort{folders: folders} }

// LastFolderID returns the user's highest-position folder.
func (p *FolderPort) LastFolderID(ctx context.Context, userID vo.Id) (vo.Id, bool, error) {
	fs, err := p.folders.ListByUser(ctx, userID)
	if err != nil {
		return vo.Id{}, false, err
	}
	var last *domaccount.Folder
	for _, f := range fs {
		if last == nil || f.Position() > last.Position() {
			last = f
		}
	}
	if last == nil {
		return vo.Id{}, false, nil
	}
	return last.Id(), true, nil
}

// FoldersContaining returns the user's folder ids that contain the account.
func (p *FolderPort) FoldersContaining(ctx context.Context, userID, accountID vo.Id) ([]vo.Id, error) {
	memberships, err := p.folders.MembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	var out []vo.Id
	for folderID, accountIDs := range memberships {
		for _, aid := range accountIDs {
			if aid == accountID.String() {
				fid, perr := vo.ParseId(folderID)
				if perr != nil {
					return nil, perr
				}
				out = append(out, fid)
				break
			}
		}
	}
	return out, nil
}

// AddAccount adds the account to the folder.
func (p *FolderPort) AddAccount(ctx context.Context, folderID, accountID vo.Id) error {
	return p.folders.AddAccount(ctx, folderID, accountID)
}

// RemoveAccount removes the account from the folder.
func (p *FolderPort) RemoveAccount(ctx context.Context, folderID, accountID vo.Id) error {
	return p.folders.RemoveAccount(ctx, folderID, accountID)
}

// --- OptionPort over the account Repository ---

// optionRepo is the slice of the account Repository the connection side effects
// need (accounts_options position).
type optionRepo interface {
	MaxPosition(ctx context.Context, userID vo.Id) (int16, error)
	SavePosition(ctx context.Context, accountID, userID vo.Id, position int16, now time.Time) error
}

// OptionPort adapts the account Repository to app/connection.OptionPort.
type OptionPort struct{ accounts optionRepo }

var _ appconnection.OptionPort = (*OptionPort)(nil)

// NewOptionPort wraps an account Repository (anything exposing the position ops).
func NewOptionPort(accounts optionRepo) *OptionPort { return &OptionPort{accounts: accounts} }

// MaxPosition returns the user's highest accounts_options.position.
func (p *OptionPort) MaxPosition(ctx context.Context, userID vo.Id) (int16, error) {
	return p.accounts.MaxPosition(ctx, userID)
}

// SavePosition upserts the user's accounts_options row.
func (p *OptionPort) SavePosition(ctx context.Context, accountID, userID vo.Id, position int16, now time.Time) error {
	return p.accounts.SavePosition(ctx, accountID, userID, position, now)
}

// --- UserLookup over the user repository ---

type userByID interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (domuser.Header, error)
}

// UserLookup adapts the user repository to app/connection.UserLookup.
type UserLookup struct{ users userByID }

var _ appconnection.UserLookup = (*UserLookup)(nil)

// NewUserLookup wraps a user repository.
func NewUserLookup(users userByID) *UserLookup { return &UserLookup{users: users} }

// GetOwner resolves the connected-user embed (id, name, avatar).
func (l *UserLookup) GetOwner(ctx context.Context, userID string) (appconnection.OwnerView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return appconnection.OwnerView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return appconnection.OwnerView{}, err
	}
	return appconnection.OwnerView{ID: h.ID, Name: h.Name, Avatar: h.AvatarURL}, nil
}

// --- SharedAccessLookup over the connection AccountAccess repo ---

// accountAccessLister is the slice of the connection repo the account module's
// sharedAccess[] embed needs.
type accountAccessLister interface {
	ListByAccount(ctx context.Context, accountID vo.Id) ([]*domconnection.AccountAccess, error)
}

// SharedAccessLookup adapts the connection repo to app/account.SharedAccessLookup
// so an account's sharedAccess[] embed lists its accounts_access grants.
type SharedAccessLookup struct{ access accountAccessLister }

var _ appaccount.SharedAccessLookup = (*SharedAccessLookup)(nil)

// NewSharedAccessLookup wraps the connection AccountAccess repo.
func NewSharedAccessLookup(access accountAccessLister) *SharedAccessLookup {
	return &SharedAccessLookup{access: access}
}

// ListByAccount returns the grants on an account as {userID, role alias}.
func (l *SharedAccessLookup) ListByAccount(ctx context.Context, accountID vo.Id) ([]appaccount.SharedAccessView, error) {
	grants, err := l.access.ListByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]appaccount.SharedAccessView, len(grants))
	for i, g := range grants {
		out[i] = appaccount.SharedAccessView{UserID: g.UserId().String(), Role: g.Role().Alias()}
	}
	return out, nil
}

// --- AccessRevoker over the connection repo + service ---

// accessRevokerDeps is the slice of the connection repo + service the account
// module's delete-account non-owner branch needs.
type accessRevokerDeps interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	Get(ctx context.Context, accountID, userID vo.Id) (*domconnection.AccountAccess, error)
}

// ownAccessRevoker is the connection-service method that drops the caller's own
// grant (RevokeOwnAccess).
type ownAccessRevoker interface {
	RevokeOwnAccess(ctx context.Context, userID, accountID vo.Id) error
}

// AccessRevoker adapts the connection repo + service to
// app/account.AccessRevoker (delete-account non-owner branch).
type AccessRevoker struct {
	repo accessRevokerDeps
	svc  ownAccessRevoker
}

var _ appaccount.AccessRevoker = (*AccessRevoker)(nil)

// NewAccessRevoker wires the access revoker over the connection repo + service.
func NewAccessRevoker(repo accessRevokerDeps, svc ownAccessRevoker) *AccessRevoker {
	return &AccessRevoker{repo: repo, svc: svc}
}

// HasAccess reports whether the user owns the account or holds a grant on it
// (PHP canDeleteAccount = hasAccess).
func (a *AccessRevoker) HasAccess(ctx context.Context, userID, accountID vo.Id) (bool, error) {
	owner, err := a.repo.AccountOwner(ctx, accountID)
	if err == nil && owner.Equal(userID) {
		return true, nil
	}
	if _, gerr := a.repo.Get(ctx, accountID, userID); gerr == nil {
		return true, nil
	}
	return false, nil
}

// RevokeOwnAccess delegates to the connection service.
func (a *AccessRevoker) RevokeOwnAccess(ctx context.Context, userID, accountID vo.Id) error {
	return a.svc.RevokeOwnAccess(ctx, userID, accountID)
}
