// TransactionImportAccounts adapts the account service/repos to the
// transaction import adapter's account port (internal/infra/repo/transaction's
// importAccountPort). It lives here, not in internal/infra/repo/transaction,
// because it needs the account feature's types and an infra package must not
// import a feature (see archtest).
package server

import (
	"context"

	account "github.com/econumo/econumo/internal/account"
	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	"github.com/econumo/econumo/internal/shared/vo"
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
