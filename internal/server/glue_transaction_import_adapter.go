// TransactionImportAccounts adapts the account service/repos to the
// transaction import adapter's account port (internal/infra/repo/transaction's
// importAccountPort), TransactionImportCategories adapts the category
// service/repo to its category port (importCategoryPort), and
// TransactionImportTags adapts the tag service/repo to its tag port
// (importTagPort). All three live here, not in
// internal/infra/repo/transaction, because they need the
// account/category/tag features' types and an infra package must not import
// a feature (see archtest).
package server

import (
	"context"

	account "github.com/econumo/econumo/internal/account"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	category "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/shared/vo"
	tag "github.com/econumo/econumo/internal/tag"
)

// transactionImportAccountService is the account-service surface the importer
// uses.
type transactionImportAccountService interface {
	CreateAccount(ctx context.Context, userID vo.Id, req account.CreateAccountRequest) (*account.CreateAccountResult, error)
	CreateFolder(ctx context.Context, userID vo.Id, req account.CreateFolderRequest) (*account.CreateFolderResult, error)
}

// transactionImportAccountRepo / transactionImportFolderRepo are the read
// surfaces over the account + folder repos.
type transactionImportAccountRepo interface {
	ListAvailable(ctx context.Context, userID vo.Id) ([]*account.Account, error)
	GetByID(ctx context.Context, id vo.Id) (*account.Account, error)
}
type transactionImportFolderRepo interface {
	ListByUser(ctx context.Context, userID vo.Id) ([]*account.Folder, error)
}

// transactionImportCurrencyByCode resolves the base-currency id from its code
// (for new accounts).
type transactionImportCurrencyByCode interface {
	GetIDByCode(ctx context.Context, code string) (string, error)
}

// TransactionImportAccounts adapts the account service/repos + currency lookup
// to the transaction import adapter's account port.
type TransactionImportAccounts struct {
	svc         transactionImportAccountService
	accountRepo transactionImportAccountRepo
	folderRepo  transactionImportFolderRepo
	currency    transactionImportCurrencyByCode
	baseCode    string
}

// NewTransactionImportAccounts wires the adapter. baseCode is the configured
// base currency code used when creating accounts for unknown account names.
func NewTransactionImportAccounts(
	svc transactionImportAccountService,
	accountRepo transactionImportAccountRepo,
	folderRepo transactionImportFolderRepo,
	currency transactionImportCurrencyByCode,
	baseCode string,
) *TransactionImportAccounts {
	return &TransactionImportAccounts{svc: svc, accountRepo: accountRepo, folderRepo: folderRepo, currency: currency, baseCode: baseCode}
}

func (a *TransactionImportAccounts) AvailableAccounts(ctx context.Context, userID vo.Id) ([]apptransaction.ImportAccount, error) {
	accts, err := a.accountRepo.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apptransaction.ImportAccount, len(accts))
	for i, acct := range accts {
		out[i] = apptransaction.ImportAccount{ID: acct.Id().String(), Name: acct.Name(), OwnerID: acct.UserId().String()}
	}
	return out, nil
}

func (a *TransactionImportAccounts) AccountByID(ctx context.Context, userID vo.Id, id vo.Id) (*apptransaction.ImportAccount, error) {
	acct, err := a.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil // not found -> nil
	}
	// Only available (own) accounts qualify.
	if !acct.UserId().Equal(userID) {
		return nil, nil
	}
	return &apptransaction.ImportAccount{ID: acct.Id().String(), Name: acct.Name(), OwnerID: acct.UserId().String()}, nil
}

func (a *TransactionImportAccounts) CreateAccount(ctx context.Context, userID vo.Id, name string) (apptransaction.ImportAccount, error) {
	// folder: first existing, else create "Imported Accounts".
	folders, err := a.folderRepo.ListByUser(ctx, userID)
	if err != nil {
		return apptransaction.ImportAccount{}, err
	}
	var folderID string
	if len(folders) > 0 {
		folderID = folders[0].Id().String()
	} else {
		fres, ferr := a.svc.CreateFolder(ctx, userID, account.CreateFolderRequest{
			Name: "Imported Accounts",
		})
		if ferr != nil {
			return apptransaction.ImportAccount{}, ferr
		}
		folderID = fres.Item.Id
	}

	currencyID, err := a.currency.GetIDByCode(ctx, a.baseCode)
	if err != nil {
		return apptransaction.ImportAccount{}, err
	}
	res, err := a.svc.CreateAccount(ctx, userID, account.CreateAccountRequest{
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

// transactionImportCategoryService is the category-service create surface the
// importer uses.
type transactionImportCategoryService interface {
	CreateCategory(ctx context.Context, userID vo.Id, req category.CreateCategoryRequest) (*category.CreateCategoryResult, error)
}

// transactionImportCategoryLister is the read surface over the category repo.
type transactionImportCategoryLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*category.Category, error)
}

// TransactionImportCategories adapts the category service/repo to the
// transaction import adapter's category port.
type TransactionImportCategories struct {
	svc  transactionImportCategoryService
	list transactionImportCategoryLister
}

// NewTransactionImportCategories wires the adapter.
func NewTransactionImportCategories(svc transactionImportCategoryService, list transactionImportCategoryLister) *TransactionImportCategories {
	return &TransactionImportCategories{svc: svc, list: list}
}

func (c *TransactionImportCategories) CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error) {
	list, err := c.list.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]apptransaction.ImportNamed, len(list))
	for i, cat := range list {
		out[i] = apptransaction.ImportNamed{ID: cat.Id().String(), Name: cat.Name(), OwnerID: cat.UserId().String()}
	}
	return out, nil
}

func (c *TransactionImportCategories) CreateCategory(ctx context.Context, ownerID vo.Id, name string, income bool) (apptransaction.ImportNamed, error) {
	typ := "expense"
	if income {
		typ = "income"
	}
	icon := "category"
	res, err := c.svc.CreateCategory(ctx, ownerID, category.CreateCategoryRequest{
		Id: vo.NewId().String(), Name: name, Type: typ, Icon: &icon,
	})
	if err != nil {
		return apptransaction.ImportNamed{}, err
	}
	return apptransaction.ImportNamed{ID: res.Item.Id, Name: res.Item.Name, OwnerID: ownerID.String()}, nil
}

// transactionImportTagService is the tag-service create surface the importer
// uses.
type transactionImportTagService interface {
	CreateTag(ctx context.Context, userID vo.Id, req tag.CreateTagRequest) (*tag.CreateTagResult, error)
}

// transactionImportTagLister is the read surface over the tag repo.
type transactionImportTagLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*tag.Tag, error)
}

// TransactionImportTags adapts the tag service/repo to the transaction
// import adapter's tag port.
type TransactionImportTags struct {
	svc  transactionImportTagService
	list transactionImportTagLister
}

// NewTransactionImportTags wires the adapter.
func NewTransactionImportTags(svc transactionImportTagService, list transactionImportTagLister) *TransactionImportTags {
	return &TransactionImportTags{svc: svc, list: list}
}

func (t *TransactionImportTags) TagsByOwner(ctx context.Context, ownerID vo.Id) ([]apptransaction.ImportNamed, error) {
	list, err := t.list.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]apptransaction.ImportNamed, len(list))
	for i, tg := range list {
		out[i] = apptransaction.ImportNamed{ID: tg.Id().String(), Name: tg.Name(), OwnerID: tg.UserId().String()}
	}
	return out, nil
}

func (t *TransactionImportTags) CreateTag(ctx context.Context, ownerID vo.Id, name string) (apptransaction.ImportNamed, error) {
	res, err := t.svc.CreateTag(ctx, ownerID, tag.CreateTagRequest{
		Id: vo.NewId().String(), Name: name,
	})
	if err != nil {
		return apptransaction.ImportNamed{}, err
	}
	return apptransaction.ImportNamed{ID: res.Item.Id, Name: res.Item.Name, OwnerID: ownerID.String()}, nil
}
