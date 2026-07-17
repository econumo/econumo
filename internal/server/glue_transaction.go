// Transaction glue: every adapter satisfying a port that the transaction
// feature declares (see internal/transaction/ports.go). Features must not
// import each other (archtest); the composition root bridges them here.
package server

import (
	"context"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// transactionCategoryByID is the minimal category-repo surface the export
// adapter's name lookup uses.
type transactionCategoryByID interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Category, error)
}

// TransactionCategoryNameLookup adapts the category repository to the
// transaction export adapter's categoryNameLookup port.
type TransactionCategoryNameLookup struct {
	categories transactionCategoryByID
}

// NewTransactionCategoryNameLookup wraps a category repository.
func NewTransactionCategoryNameLookup(categories transactionCategoryByID) *TransactionCategoryNameLookup {
	return &TransactionCategoryNameLookup{categories: categories}
}

// CategoryName resolves a category's name ("" if not found).
func (l *TransactionCategoryNameLookup) CategoryName(ctx context.Context, id vo.Id) (string, error) {
	c, err := l.categories.GetByID(ctx, id)
	if err != nil {
		return "", nil
	}
	return c.Name, nil
}

// transactionTagByID is the minimal tag-repo surface the export adapter's
// name lookup uses.
type transactionTagByID interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Tag, error)
}

// TransactionTagNameLookup adapts the tag repository to the transaction
// export adapter's tagNameLookup port.
type TransactionTagNameLookup struct {
	tags transactionTagByID
}

// NewTransactionTagNameLookup wraps a tag repository.
func NewTransactionTagNameLookup(tags transactionTagByID) *TransactionTagNameLookup {
	return &TransactionTagNameLookup{tags: tags}
}

// TagName resolves a tag's name ("" if not found).
func (l *TransactionTagNameLookup) TagName(ctx context.Context, id vo.Id) (string, error) {
	t, err := l.tags.GetByID(ctx, id)
	if err != nil {
		return "", nil
	}
	return t.Name, nil
}

// transactionPayeeByID is the minimal payee-repo surface the export adapter's
// name lookup uses.
type transactionPayeeByID interface {
	GetByID(ctx context.Context, id vo.Id) (*model.Payee, error)
}

// TransactionPayeeNameLookup adapts the payee repository to the transaction
// export adapter's payeeNameLookup port.
type TransactionPayeeNameLookup struct {
	payees transactionPayeeByID
}

// NewTransactionPayeeNameLookup wraps a payee repository.
func NewTransactionPayeeNameLookup(payees transactionPayeeByID) *TransactionPayeeNameLookup {
	return &TransactionPayeeNameLookup{payees: payees}
}

// PayeeName resolves a payee's name ("" if not found).
func (l *TransactionPayeeNameLookup) PayeeName(ctx context.Context, id vo.Id) (string, error) {
	p, err := l.payees.GetByID(ctx, id)
	if err != nil {
		return "", nil
	}
	return p.Name, nil
}

// transactionImportAccountService is the account-service surface the importer
// uses.
type transactionImportAccountService interface {
	CreateAccount(ctx context.Context, userID vo.Id, req model.CreateAccountRequest) (*model.CreateAccountResult, error)
	CreateFolder(ctx context.Context, userID vo.Id, req model.CreateFolderRequest) (*model.CreateFolderResult, error)
}

// transactionImportAccountRepo / transactionImportFolderRepo are the read
// surfaces over the account + folder repos.
type transactionImportAccountRepo interface {
	ListAvailable(ctx context.Context, userID vo.Id) ([]*model.Account, error)
	GetByID(ctx context.Context, id vo.Id) (*model.Account, error)
}
type transactionImportFolderRepo interface {
	ListByUser(ctx context.Context, userID vo.Id) ([]*model.Folder, error)
}

// transactionImportCurrencyByCode resolves the base-currency id from its code
// (for new accounts), preferring the importing user's own custom currency
// over the global one.
type transactionImportCurrencyByCode interface {
	GetIDByCodeForUser(ctx context.Context, userID, code string) (string, error)
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

func (a *TransactionImportAccounts) AvailableAccounts(ctx context.Context, userID vo.Id) ([]model.ImportAccount, error) {
	accts, err := a.accountRepo.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportAccount, len(accts))
	for i, acct := range accts {
		out[i] = model.ImportAccount{ID: acct.ID.String(), Name: acct.Name, OwnerID: acct.UserID.String()}
	}
	return out, nil
}

func (a *TransactionImportAccounts) AccountByID(ctx context.Context, userID vo.Id, id vo.Id) (*model.ImportAccount, error) {
	acct, err := a.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil // not found -> nil
	}
	// Only available (own) accounts qualify.
	if !acct.UserID.Equal(userID) {
		return nil, nil
	}
	return &model.ImportAccount{ID: acct.ID.String(), Name: acct.Name, OwnerID: acct.UserID.String()}, nil
}

func (a *TransactionImportAccounts) CreateAccount(ctx context.Context, userID vo.Id, name string) (model.ImportAccount, error) {
	// folder: first existing, else create "Imported Accounts".
	folders, err := a.folderRepo.ListByUser(ctx, userID)
	if err != nil {
		return model.ImportAccount{}, err
	}
	var folderID string
	if len(folders) > 0 {
		folderID = folders[0].ID.String()
	} else {
		fres, ferr := a.svc.CreateFolder(ctx, userID, model.CreateFolderRequest{
			Name: "Imported Accounts",
		})
		if ferr != nil {
			return model.ImportAccount{}, ferr
		}
		folderID = fres.Item.Id
	}

	currencyID, err := a.currency.GetIDByCodeForUser(ctx, userID.String(), a.baseCode)
	if err != nil {
		return model.ImportAccount{}, err
	}
	res, err := a.svc.CreateAccount(ctx, userID, model.CreateAccountRequest{
		Id:         vo.NewId().String(),
		Name:       name,
		CurrencyId: currencyID,
		FolderId:   folderID,
		Balance:    "0",
		Icon:       "wallet",
	})
	if err != nil {
		return model.ImportAccount{}, err
	}
	return model.ImportAccount{ID: res.Item.Id, Name: res.Item.Name, OwnerID: userID.String()}, nil
}

// transactionImportCategoryService is the category-service create surface the
// importer uses.
type transactionImportCategoryService interface {
	CreateCategory(ctx context.Context, userID vo.Id, req model.CreateCategoryRequest) (*model.CreateCategoryResult, error)
}

// transactionImportCategoryLister is the read surface over the category repo.
type transactionImportCategoryLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Category, error)
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

func (c *TransactionImportCategories) CategoriesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	list, err := c.list.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportNamed, len(list))
	for i, cat := range list {
		out[i] = model.ImportNamed{ID: cat.ID.String(), Name: cat.Name, OwnerID: cat.UserID.String()}
	}
	return out, nil
}

func (c *TransactionImportCategories) CreateCategory(ctx context.Context, ownerID vo.Id, name string, income bool) (model.ImportNamed, error) {
	typ := "expense"
	if income {
		typ = "income"
	}
	icon := "category"
	res, err := c.svc.CreateCategory(ctx, ownerID, model.CreateCategoryRequest{
		Id: vo.NewId().String(), Name: name, Type: typ, Icon: &icon,
	})
	if err != nil {
		return model.ImportNamed{}, err
	}
	return model.ImportNamed{ID: res.Item.Id, Name: res.Item.Name, OwnerID: ownerID.String()}, nil
}

// transactionImportTagService is the tag-service create surface the importer
// uses.
type transactionImportTagService interface {
	CreateTag(ctx context.Context, userID vo.Id, req model.CreateTagRequest) (*model.CreateTagResult, error)
}

// transactionImportTagLister is the read surface over the tag repo.
type transactionImportTagLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Tag, error)
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

func (t *TransactionImportTags) TagsByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	list, err := t.list.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportNamed, len(list))
	for i, tg := range list {
		out[i] = model.ImportNamed{ID: tg.ID.String(), Name: tg.Name, OwnerID: tg.UserID.String()}
	}
	return out, nil
}

func (t *TransactionImportTags) CreateTag(ctx context.Context, ownerID vo.Id, name string) (model.ImportNamed, error) {
	res, err := t.svc.CreateTag(ctx, ownerID, model.CreateTagRequest{
		Id: vo.NewId().String(), Name: name,
	})
	if err != nil {
		return model.ImportNamed{}, err
	}
	return model.ImportNamed{ID: res.Item.Id, Name: res.Item.Name, OwnerID: ownerID.String()}, nil
}

// transactionImportPayeeService is the payee-service create surface the
// importer uses.
type transactionImportPayeeService interface {
	CreatePayee(ctx context.Context, userID vo.Id, req model.CreatePayeeRequest) (*model.CreatePayeeResult, error)
}

// transactionImportPayeeLister is the read surface over the payee repo.
type transactionImportPayeeLister interface {
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Payee, error)
}

// TransactionImportPayees adapts the payee service/repo to the transaction
// import adapter's payee port.
type TransactionImportPayees struct {
	svc  transactionImportPayeeService
	list transactionImportPayeeLister
}

// NewTransactionImportPayees wires the adapter.
func NewTransactionImportPayees(svc transactionImportPayeeService, list transactionImportPayeeLister) *TransactionImportPayees {
	return &TransactionImportPayees{svc: svc, list: list}
}

func (p *TransactionImportPayees) PayeesByOwner(ctx context.Context, ownerID vo.Id) ([]model.ImportNamed, error) {
	list, err := p.list.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ImportNamed, len(list))
	for i, py := range list {
		out[i] = model.ImportNamed{ID: py.ID.String(), Name: py.Name, OwnerID: py.UserID.String()}
	}
	return out, nil
}

func (p *TransactionImportPayees) CreatePayee(ctx context.Context, ownerID vo.Id, name string) (model.ImportNamed, error) {
	res, err := p.svc.CreatePayee(ctx, ownerID, model.CreatePayeeRequest{
		Id: vo.NewId().String(), Name: name,
	})
	if err != nil {
		return model.ImportNamed{}, err
	}
	return model.ImportNamed{ID: res.Item.Id, Name: res.Item.Name, OwnerID: ownerID.String()}, nil
}
