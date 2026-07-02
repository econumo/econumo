// Importer adapter: satisfies app/transaction.Importer via the
// account/category/tag/payee ports (internal/server.TransactionImportAccounts,
// internal/server.TransactionImportCategories, internal/server.
// TransactionImportTags, internal/server.TransactionImportPayees) for
// account/category/tag/payee reads/creates. findOrCreate caching lives in the
// app service; this adapter performs atomic lookups/creates within the
// import-wide transaction. The account-, category-, tag-, and payee-touching
// surfaces live in internal/server, not here, because this package is a leaf
// that must not import the account/category/tag/payee features (see
// archtest).
package transactionrepo

import (
	"context"

	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	domconnection "github.com/econumo/econumo/internal/domain/connection"
	domtransaction "github.com/econumo/econumo/internal/domain/transaction"
	"github.com/econumo/econumo/internal/shared/vo"
)

// importAccountPort is the account-touching surface the importer uses,
// expressed purely in apptransaction types so this file never imports the
// account feature directly.
type importAccountPort interface {
	AvailableAccounts(ctx context.Context, userID vo.Id) ([]apptransaction.ImportAccount, error)
	AccountByID(ctx context.Context, userID vo.Id, id vo.Id) (*apptransaction.ImportAccount, error)
	CreateAccount(ctx context.Context, userID vo.Id, name string) (apptransaction.ImportAccount, error)
}

// importAccountAccess resolves account ownership + a connected user's grant role,
// for the import write-access check (CanAddTransaction). Backed by the connection
// AccountAccess repo; a missing grant is reported as ok=false (nil error).
type importAccountAccess interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	GrantRole(ctx context.Context, accountID, userID vo.Id) (domconnection.Role, bool, error)
}

// importCategoryPort is the category-touching surface the importer uses,
// expressed purely in apptransaction types so this file never imports the
// category feature directly.
type importCategoryPort interface {
	CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error)
	CreateCategory(ctx context.Context, ownerID vo.Id, name string, income bool) (apptransaction.ImportNamed, error)
}

// importTagPort is the tag-touching surface the importer uses, expressed
// purely in apptransaction types so this file never imports the tag feature
// directly.
type importTagPort interface {
	TagsByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error)
	CreateTag(ctx context.Context, ownerID vo.Id, name string) (apptransaction.ImportNamed, error)
}

// importPayeePort is the payee-touching surface the importer uses, expressed
// purely in apptransaction types so this file never imports the payee
// feature directly.
type importPayeePort interface {
	PayeesByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error)
	CreatePayee(ctx context.Context, ownerID vo.Id, name string) (apptransaction.ImportNamed, error)
}

// ImportLookup adapts the collaborators to app/transaction.Importer.
type ImportLookup struct {
	accounts  importAccountPort
	access    importAccountAccess
	category  importCategoryPort
	payee     importPayeePort
	tag       importTagPort
	transRepo *Repo
}

var _ apptransaction.Importer = (*ImportLookup)(nil)

// NewImportLookup wires the import adapter. category is typically
// server.TransactionImportCategories, payee is typically
// server.TransactionImportPayees, and tag is typically
// server.TransactionImportTags.
func NewImportLookup(
	accounts importAccountPort,
	access importAccountAccess,
	category importCategoryPort,
	payee importPayeePort,
	tag importTagPort,
	transRepo *Repo,
) *ImportLookup {
	return &ImportLookup{
		accounts: accounts, access: access,
		category: category, payee: payee, tag: tag,
		transRepo: transRepo,
	}
}

func (l *ImportLookup) AvailableAccounts(ctx context.Context, userID vo.Id) ([]apptransaction.ImportAccount, error) {
	return l.accounts.AvailableAccounts(ctx, userID)
}

func (l *ImportLookup) AccountByID(ctx context.Context, userID vo.Id, id vo.Id) (*apptransaction.ImportAccount, error) {
	return l.accounts.AccountByID(ctx, userID, id)
}

// CanAddTransaction reports whether the user may add a transaction to the
// account: they own it, or hold an admin/user grant on it (a guest grant or no
// grant is denied). A missing account yields false (the importer then creates a
// new own account), preserving the find-or-create flow.
func (l *ImportLookup) CanAddTransaction(ctx context.Context, userID vo.Id, accountID vo.Id) (bool, error) {
	owner, err := l.access.AccountOwner(ctx, accountID)
	if err != nil {
		return false, nil
	}
	if owner.Equal(userID) {
		return true, nil
	}
	role, ok, err := l.access.GrantRole(ctx, accountID, userID)
	if err != nil {
		return false, err
	}
	return ok && (role == domconnection.RoleAdmin || role == domconnection.RoleUser), nil
}

func (l *ImportLookup) CreateAccount(ctx context.Context, userID vo.Id, name string) (apptransaction.ImportAccount, error) {
	return l.accounts.CreateAccount(ctx, userID, name)
}

func (l *ImportLookup) CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error) {
	return l.category.CategoriesByOwner(ctx, ownerID)
}

func (l *ImportLookup) PayeesByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error) {
	return l.payee.PayeesByOwner(ctx, ownerID)
}

func (l *ImportLookup) TagsByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error) {
	return l.tag.TagsByOwner(ctx, ownerID)
}

func (l *ImportLookup) CreateCategory(ctx context.Context, ownerID vo.Id, name string, income bool) (apptransaction.ImportNamed, error) {
	return l.category.CreateCategory(ctx, ownerID, name, income)
}

func (l *ImportLookup) CreatePayee(ctx context.Context, ownerID vo.Id, name string) (apptransaction.ImportNamed, error) {
	return l.payee.CreatePayee(ctx, ownerID, name)
}

func (l *ImportLookup) CreateTag(ctx context.Context, ownerID vo.Id, name string) (apptransaction.ImportNamed, error) {
	return l.tag.CreateTag(ctx, ownerID, name)
}

func (l *ImportLookup) SaveTransaction(ctx context.Context, t *domtransaction.Transaction) error {
	return l.transRepo.Save(ctx, t)
}
