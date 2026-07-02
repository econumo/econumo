// Importer adapter: satisfies app/transaction.Importer by reusing the existing
// account/category/payee/tag application services for creation and the
// account/folder repos + currency lookup for reads. findOrCreate caching lives
// in the app service; this adapter performs atomic lookups/creates within the
// import-wide transaction.
package transactionrepo

import (
	"context"

	appaccount "github.com/econumo/econumo/internal/app/account"
	appcategory "github.com/econumo/econumo/internal/app/category"
	apppayee "github.com/econumo/econumo/internal/app/payee"
	apptag "github.com/econumo/econumo/internal/app/tag"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	domaccount "github.com/econumo/econumo/internal/domain/account"
	domcategory "github.com/econumo/econumo/internal/domain/category"
	domconnection "github.com/econumo/econumo/internal/domain/connection"
	dompayee "github.com/econumo/econumo/internal/domain/payee"
	domtag "github.com/econumo/econumo/internal/domain/tag"
	domtransaction "github.com/econumo/econumo/internal/domain/transaction"
	"github.com/econumo/econumo/internal/shared/vo"
)

// importAccountService is the account-service surface the importer uses.
type importAccountService interface {
	CreateAccount(ctx context.Context, userID vo.Id, req appaccount.CreateAccountRequest) (*appaccount.CreateAccountResult, error)
	CreateFolder(ctx context.Context, userID vo.Id, req appaccount.CreateFolderRequest) (*appaccount.CreateFolderResult, error)
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
}

// importAccountAccess resolves account ownership + a connected user's grant role,
// for the import write-access check (CanAddTransaction). Backed by the connection
// AccountAccess repo; a missing grant is reported as ok=false (nil error).
type importAccountAccess interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	GrantRole(ctx context.Context, accountID, userID vo.Id) (domconnection.Role, bool, error)
}

// importAccountRepo / importFolderRepo are the read surfaces over the account +
// folder repos.
type importAccountRepo interface {
	ListAvailable(ctx context.Context, userID vo.Id) ([]*domaccount.Account, error)
	GetByID(ctx context.Context, id vo.Id) (*domaccount.Account, error)
}
type importFolderRepo interface {
	ListByUser(ctx context.Context, userID vo.Id) ([]*domaccount.Folder, error)
}

// importCategoryService / importPayeeService / importTagService are the create
// surfaces over those services. The repos give the owner's existing entities.
type importCategoryService interface {
	CreateCategory(ctx context.Context, userID vo.Id, req appcategory.CreateCategoryRequest) (*appcategory.CreateCategoryResult, error)
}
type importPayeeService interface {
	CreatePayee(ctx context.Context, userID vo.Id, req apppayee.CreatePayeeRequest) (*apppayee.CreatePayeeResult, error)
}
type importTagService interface {
	CreateTag(ctx context.Context, userID vo.Id, req apptag.CreateTagRequest) (*apptag.CreateTagResult, error)
}

// currencyByCode resolves the base-currency id from its code (for new accounts).
type currencyByCode interface {
	GetIDByCode(ctx context.Context, code string) (string, error)
}

// categoryEntityLister/tagEntityLister/payeeEntityLister are the per-aggregate
// list surfaces; their elements expose Id()/Name()/UserId().
type categoryEntityLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*domcategory.Category, error)
}
type tagEntityLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*domtag.Tag, error)
}
type payeeEntityLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*dompayee.Payee, error)
}

// ImportLookup adapts the collaborators to app/transaction.Importer.
type ImportLookup struct {
	accountSvc  importAccountService
	access      importAccountAccess
	accountRepo importAccountRepo
	folderRepo  importFolderRepo
	categorySvc importCategoryService
	payeeSvc    importPayeeService
	tagSvc      importTagService
	categories  categoryEntityLister
	tags        tagEntityLister
	payees      payeeEntityLister
	currency    currencyByCode
	transRepo   *Repo
	baseCode    string
}

var _ apptransaction.Importer = (*ImportLookup)(nil)

// NewImportLookup wires the import adapter. baseCode is the configured base
// currency code used when creating accounts for unknown account names.
func NewImportLookup(
	accountSvc importAccountService,
	access importAccountAccess,
	accountRepo importAccountRepo,
	folderRepo importFolderRepo,
	categorySvc importCategoryService,
	payeeSvc importPayeeService,
	tagSvc importTagService,
	categories categoryEntityLister,
	tags tagEntityLister,
	payees payeeEntityLister,
	currency currencyByCode,
	transRepo *Repo,
	baseCode string,
) *ImportLookup {
	return &ImportLookup{
		accountSvc: accountSvc, access: access, accountRepo: accountRepo, folderRepo: folderRepo,
		categorySvc: categorySvc, payeeSvc: payeeSvc, tagSvc: tagSvc,
		categories: categories, tags: tags, payees: payees,
		currency: currency, transRepo: transRepo, baseCode: baseCode,
	}
}

func (l *ImportLookup) AvailableAccounts(ctx context.Context, userID vo.Id) ([]apptransaction.ImportAccount, error) {
	accts, err := l.accountRepo.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apptransaction.ImportAccount, len(accts))
	for i, a := range accts {
		out[i] = apptransaction.ImportAccount{ID: a.Id().String(), Name: a.Name(), OwnerID: a.UserId().String()}
	}
	return out, nil
}

func (l *ImportLookup) AccountByID(ctx context.Context, userID vo.Id, id vo.Id) (*apptransaction.ImportAccount, error) {
	a, err := l.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil // not found -> nil
	}
	// Only available (own) accounts qualify.
	if !a.UserId().Equal(userID) {
		return nil, nil
	}
	return &apptransaction.ImportAccount{ID: a.Id().String(), Name: a.Name(), OwnerID: a.UserId().String()}, nil
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
	// folder: first existing, else create "Imported Accounts".
	folders, err := l.folderRepo.ListByUser(ctx, userID)
	if err != nil {
		return apptransaction.ImportAccount{}, err
	}
	var folderID string
	if len(folders) > 0 {
		folderID = folders[0].Id().String()
	} else {
		fres, ferr := l.accountSvc.CreateFolder(ctx, userID, appaccount.CreateFolderRequest{
			Name: "Imported Accounts",
		})
		if ferr != nil {
			return apptransaction.ImportAccount{}, ferr
		}
		folderID = fres.Item.Id
	}

	currencyID, err := l.currency.GetIDByCode(ctx, l.baseCode)
	if err != nil {
		return apptransaction.ImportAccount{}, err
	}
	res, err := l.accountSvc.CreateAccount(ctx, userID, appaccount.CreateAccountRequest{
		Id:         vo.NewId().String(),
		Name:       name,
		CurrencyId: currencyID,
		FolderId:   folderID,
		Balance:    "0",
		Icon:       "wallet",
	})
	if err != nil {
		return apptransaction.ImportAccount{}, err
	}
	return apptransaction.ImportAccount{ID: res.Item.Id, Name: res.Item.Name, OwnerID: userID.String()}, nil
}

func (l *ImportLookup) CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error) {
	list, err := l.categories.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]apptransaction.ImportNamed, len(list))
	for i, c := range list {
		out[i] = apptransaction.ImportNamed{ID: c.Id().String(), Name: c.Name(), OwnerID: c.UserId().String()}
	}
	return out, nil
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
	typ := "expense"
	if income {
		typ = "income"
	}
	icon := "category"
	res, err := l.categorySvc.CreateCategory(ctx, ownerID, appcategory.CreateCategoryRequest{
		Id: vo.NewId().String(), Name: name, Type: typ, Icon: &icon,
	})
	if err != nil {
		return apptransaction.ImportNamed{}, err
	}
	return apptransaction.ImportNamed{ID: res.Item.Id, Name: res.Item.Name, OwnerID: ownerID.String()}, nil
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
