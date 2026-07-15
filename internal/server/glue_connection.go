// Connection glue: every adapter satisfying a port that the connection
// feature declares (see internal/connection/ports.go). Features must not
// import each other (archtest); the composition root bridges them here.
package server

import (
	"context"
	appconnection "github.com/econumo/econumo/internal/connection"
	domconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// connectionFolderRepo is the slice of the account FolderRepository the
// connection side effects need.
type connectionFolderRepo interface {
	ListByUser(ctx context.Context, userID vo.Id) ([]*model.Folder, error)
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
	var last *model.Folder
	for _, f := range fs {
		if last == nil || f.Position > last.Position {
			last = f
		}
	}
	if last == nil {
		return vo.Id{}, false, nil
	}
	return last.ID, true, nil
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

// connectionBudgetRepoPort is the slice of the budget repository the revoker
// needs. The budget *Repo satisfies it; declaring it here (consumer side)
// avoids importing the whole budget repo type and keeps the dependency
// one-directional.
type connectionBudgetRepoPort interface {
	ListForUser(ctx context.Context, userID vo.Id) ([]*model.Budget, error)
	ListAccess(ctx context.Context, budgetID vo.Id) ([]*model.BudgetAccess, error)
	DeleteAccess(ctx context.Context, budgetID, userID vo.Id) error
}

// ConnectionBudgetRevoker drops budget-sharing between two users in both
// directions. It implements connection.BudgetAccessRevoker, used by
// delete-connection to tear down the budget-sharing side effects of a
// connection.
type ConnectionBudgetRevoker struct {
	budgets connectionBudgetRepoPort
}

var _ appconnection.BudgetAccessRevoker = (*ConnectionBudgetRevoker)(nil)

// NewConnectionBudgetRevoker wires the revoker over the budget repository.
func NewConnectionBudgetRevoker(budgets connectionBudgetRepoPort) *ConnectionBudgetRevoker {
	return &ConnectionBudgetRevoker{budgets: budgets}
}

// RevokeBetween removes any budget-access grants shared between users a and b:
// for each budget A owns where B has access, revoke B; and symmetrically for
// budgets B owns where A has access.
func (r *ConnectionBudgetRevoker) RevokeBetween(ctx context.Context, a, b vo.Id) error {
	if err := r.revokeDirection(ctx, a, b); err != nil {
		return err
	}
	return r.revokeDirection(ctx, b, a)
}

// revokeDirection: for each budget in `owner`'s list, if `other` holds access,
// delete it. ListForUser returns owned + accessible budgets; the DeleteAccess is
// a no-op when `other` has no row, so this is safe over the full list.
func (r *ConnectionBudgetRevoker) revokeDirection(ctx context.Context, owner, other vo.Id) error {
	budgets, err := r.budgets.ListForUser(ctx, owner)
	if err != nil {
		return err
	}
	for _, bud := range budgets {
		access, aerr := r.budgets.ListAccess(ctx, bud.ID)
		if aerr != nil {
			return aerr
		}
		for _, acc := range access {
			if acc.UserID.Equal(other) {
				if derr := r.budgets.DeleteAccess(ctx, bud.ID, other); derr != nil {
					return derr
				}
				break
			}
		}
	}
	return nil
}
