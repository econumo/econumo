// Importer adapter: satisfies app/transaction.Importer by reusing the existing
// payee/tag application services for creation and the account/category ports
// (internal/server.TransactionImportAccounts, internal/server.
// TransactionImportCategories) for account/category reads/creates.
// findOrCreate caching lives in the app service; this adapter performs atomic
// lookups/creates within the import-wide transaction. The account- and
// category-touching surfaces live in internal/server, not here, because this
// package is a leaf that must not import the account/category features (see
// archtest).
package transactionrepo

import (
	"context"

	apppayee "github.com/econumo/econumo/internal/app/payee"
	apptag "github.com/econumo/econumo/internal/app/tag"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	domconnection "github.com/econumo/econumo/internal/domain/connection"
	dompayee "github.com/econumo/econumo/internal/domain/payee"
	domtag "github.com/econumo/econumo/internal/domain/tag"
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

// importPayeeService / importTagService are the create surfaces over those
// services. The repos give the owner's existing entities.
type importPayeeService interface {
	CreatePayee(ctx context.Context, userID vo.Id, req apppayee.CreatePayeeRequest) (*apppayee.CreatePayeeResult, error)
}
type importTagService interface {
	CreateTag(ctx context.Context, userID vo.Id, req apptag.CreateTagRequest) (*apptag.CreateTagResult, error)
}

// tagEntityLister/payeeEntityLister are the per-aggregate list surfaces; their
// elements expose Id()/Name()/UserId().
type tagEntityLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*domtag.Tag, error)
}
type payeeEntityLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*dompayee.Payee, error)
}

// ImportLookup adapts the collaborators to app/transaction.Importer.
type ImportLookup struct {
	accounts  importAccountPort
	access    importAccountAccess
	category  importCategoryPort
	payeeSvc  importPayeeService
	tagSvc    importTagService
	tags      tagEntityLister
	payees    payeeEntityLister
	transRepo *Repo
}

var _ apptransaction.Importer = (*ImportLookup)(nil)

// NewImportLookup wires the import adapter. category is typically
// server.TransactionImportCategories.
func NewImportLookup(
	accounts importAccountPort,
	access importAccountAccess,
	category importCategoryPort,
	payeeSvc importPayeeService,
	tagSvc importTagService,
	tags tagEntityLister,
	payees payeeEntityLister,
	transRepo *Repo,
) *ImportLookup {
	return &ImportLookup{
		accounts: accounts, access: access,
		category: category, payeeSvc: payeeSvc, tagSvc: tagSvc,
		tags: tags, payees: payees,
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
	list, err := l.payees.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]apptransaction.ImportNamed, len(list))
	for i, p := range list {
		out[i] = apptransaction.ImportNamed{ID: p.Id().String(), Name: p.Name(), OwnerID: p.UserId().String()}
	}
	return out, nil
}

func (l *ImportLookup) TagsByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error) {
	list, err := l.tags.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]apptransaction.ImportNamed, len(list))
	for i, t := range list {
		out[i] = apptransaction.ImportNamed{ID: t.Id().String(), Name: t.Name(), OwnerID: t.UserId().String()}
	}
	return out, nil
}

func (l *ImportLookup) CreateCategory(ctx context.Context, ownerID vo.Id, name string, income bool) (apptransaction.ImportNamed, error) {
	return l.category.CreateCategory(ctx, ownerID, name, income)
}

func (l *ImportLookup) CreatePayee(ctx context.Context, ownerID vo.Id, name string) (apptransaction.ImportNamed, error) {
	res, err := l.payeeSvc.CreatePayee(ctx, ownerID, apppayee.CreatePayeeRequest{
		Id: vo.NewId().String(), Name: name,
	})
	if err != nil {
		return apptransaction.ImportNamed{}, err
	}
	return apptransaction.ImportNamed{ID: res.Item.Id, Name: res.Item.Name, OwnerID: ownerID.String()}, nil
}

func (l *ImportLookup) CreateTag(ctx context.Context, ownerID vo.Id, name string) (apptransaction.ImportNamed, error) {
	res, err := l.tagSvc.CreateTag(ctx, ownerID, apptag.CreateTagRequest{
		Id: vo.NewId().String(), Name: name,
	})
	if err != nil {
		return apptransaction.ImportNamed{}, err
	}
	return apptransaction.ImportNamed{ID: res.Item.Id, Name: res.Item.Name, OwnerID: ownerID.String()}, nil
}

func (l *ImportLookup) SaveTransaction(ctx context.Context, t *domtransaction.Transaction) error {
	return l.transRepo.Save(ctx, t)
}
