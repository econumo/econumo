// ConnectionFolderPort, ConnectionSharedAccessLookup and ConnectionAccessRevoker
// bridge the connection and account features. They live here, not in
// internal/connection/repo, because they need the account feature's
// Folder type and account's own ports, and a feature package must not import
// another feature (see archtest). The AccountAccessResolver and OptionPort
// counterparts stay in internal/connection/repo: they don't reference
// the account feature's types.
package server

import (
	"context"

	account "github.com/econumo/econumo/internal/account"
	domconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/shared/vo"
)

// connectionFolderRepo is the slice of the account FolderRepository the
// connection side effects need.
type connectionFolderRepo interface {
	ListByUser(ctx context.Context, userID vo.Id) ([]*account.Folder, error)
	MembershipsByUser(ctx context.Context, userID vo.Id) (map[string][]string, error)
	AddAccount(ctx context.Context, folderID, accountID vo.Id) error
	RemoveAccount(ctx context.Context, folderID, accountID vo.Id) error
}

// ConnectionFolderPort adapts the account FolderRepository to
// connection.FolderPort.
type ConnectionFolderPort struct{ folders connectionFolderRepo }

var _ domconnection.FolderPort = (*ConnectionFolderPort)(nil)

// NewConnectionFolderPort wraps an account FolderRepository.
func NewConnectionFolderPort(folders connectionFolderRepo) *ConnectionFolderPort {
	return &ConnectionFolderPort{folders: folders}
}

// LastFolderID returns the user's highest-position folder.
func (p *ConnectionFolderPort) LastFolderID(ctx context.Context, userID vo.Id) (vo.Id, bool, error) {
	fs, err := p.folders.ListByUser(ctx, userID)
	if err != nil {
		return vo.Id{}, false, err
	}
	var last *account.Folder
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
func (p *ConnectionFolderPort) FoldersContaining(ctx context.Context, userID, accountID vo.Id) ([]vo.Id, error) {
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
func (p *ConnectionFolderPort) AddAccount(ctx context.Context, folderID, accountID vo.Id) error {
	return p.folders.AddAccount(ctx, folderID, accountID)
}

// RemoveAccount removes the account from the folder.
func (p *ConnectionFolderPort) RemoveAccount(ctx context.Context, folderID, accountID vo.Id) error {
	return p.folders.RemoveAccount(ctx, folderID, accountID)
}

// connectionAccountAccessLister is the slice of the connection repo the
// account module's sharedAccess[] embed needs.
type connectionAccountAccessLister interface {
	ListByAccount(ctx context.Context, accountID vo.Id) ([]*domconnection.AccountAccess, error)
}

// ConnectionSharedAccessLookup adapts the connection repo to
// account.SharedAccessLookup so an account's sharedAccess[] embed lists its
// accounts_access grants.
type ConnectionSharedAccessLookup struct{ access connectionAccountAccessLister }

var _ account.SharedAccessLookup = (*ConnectionSharedAccessLookup)(nil)

// NewConnectionSharedAccessLookup wraps the connection AccountAccess repo.
func NewConnectionSharedAccessLookup(access connectionAccountAccessLister) *ConnectionSharedAccessLookup {
	return &ConnectionSharedAccessLookup{access: access}
}

// ListByAccount returns the grants on an account as {userID, role alias}.
func (l *ConnectionSharedAccessLookup) ListByAccount(ctx context.Context, accountID vo.Id) ([]account.SharedAccessView, error) {
	grants, err := l.access.ListByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]account.SharedAccessView, len(grants))
	for i, g := range grants {
		out[i] = account.SharedAccessView{UserID: g.UserID.String(), Role: g.Role.Alias()}
	}
	return out, nil
}

// connectionAccessRevokerDeps is the slice of the connection repo the account
// module's delete-account non-owner branch needs.
type connectionAccessRevokerDeps interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	Get(ctx context.Context, accountID, userID vo.Id) (*domconnection.AccountAccess, error)
}

// connectionOwnAccessRevoker is the connection-service method that drops the
// caller's own grant (RevokeOwnAccess).
type connectionOwnAccessRevoker interface {
	RevokeOwnAccess(ctx context.Context, userID, accountID vo.Id) error
}

// ConnectionAccessRevoker adapts the connection repo + service to
// account.AccessRevoker (delete-account non-owner branch).
type ConnectionAccessRevoker struct {
	repo connectionAccessRevokerDeps
	svc  connectionOwnAccessRevoker
}

var _ account.AccessRevoker = (*ConnectionAccessRevoker)(nil)

// NewConnectionAccessRevoker wires the access revoker over the connection
// repo + service.
func NewConnectionAccessRevoker(repo connectionAccessRevokerDeps, svc connectionOwnAccessRevoker) *ConnectionAccessRevoker {
	return &ConnectionAccessRevoker{repo: repo, svc: svc}
}

// HasAccess reports whether the user owns the account or holds a grant on it
// (the delete-account precondition).
func (a *ConnectionAccessRevoker) HasAccess(ctx context.Context, userID, accountID vo.Id) (bool, error) {
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
func (a *ConnectionAccessRevoker) RevokeOwnAccess(ctx context.Context, userID, accountID vo.Id) error {
	return a.svc.RevokeOwnAccess(ctx, userID, accountID)
}
