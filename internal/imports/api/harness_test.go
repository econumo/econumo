package api_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appimports "github.com/econumo/econumo/internal/imports"
	handlerimports "github.com/econumo/econumo/internal/imports/api"
	"github.com/econumo/econumo/internal/imports/applewallet"
	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/imports/simplefin"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/authstub"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	"github.com/econumo/econumo/internal/web/middleware"
)

const (
	usdID  = "dffc2a06-6f29-4704-8575-31709adee926"
	userA  = "0a000000-0000-0000-0000-000000000001"
	acct1  = "0a000000-0000-0000-0000-0000000000a1"
	source = "0c000000-0000-0000-0000-000000000001"
)

var now = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

type clock struct{ t time.Time }

func (c clock) Now() time.Time { return c.t }

type fakeAccounts struct{}

func (fakeAccounts) AccountOwner(_ context.Context, id vo.Id) (vo.Id, error) {
	if id.String() == acct1 {
		return vo.MustParseId(userA), nil
	}
	return vo.Id{}, errs.NewNotFound("Account not found")
}
func (fakeAccounts) AccountDeleted(context.Context, vo.Id) (bool, error)        { return false, nil }
func (fakeAccounts) AccountCurrencyCode(context.Context, vo.Id) (string, error) { return "USD", nil }

type fakeConverter struct{}

func (fakeConverter) Convert(_ context.Context, _ vo.Id, from, to, amount string, _ time.Time) (string, bool, error) {
	if from == to {
		return amount, true, nil
	}
	return "", false, nil
}

// fakeTxns stands in for the transaction feature: it still writes a real
// transactions row (through the SAME ctx-scoped tx the pipeline is running
// in, via tx.Querier) so the ledger row the pipeline links to it satisfies
// the schema's transaction_id foreign key. Writing through the fixture
// builder instead would reach for the pooled *sql.DB directly (MaxOpenConns=1
// in tests) while the pipeline's own transaction holds the only connection,
// deadlocking the test.
type fakeTxns struct {
	db       *dbtest.DB
	created  int
	updated  []model.UpdateTransactionRequest
	replaced []model.UpdateTransactionRequest
}

func (f *fakeTxns) CreateTransaction(ctx context.Context, _ vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error) {
	f.created++
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	q := f.db.TX.Querier(ctx)
	query := f.db.Rebind(
		`INSERT INTO transactions (id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, type, amount, amount_recipient, description, spent_at, created_at, updated_at)
			VALUES (?, ?, ?, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?, ?, ?)`)
	if _, err := q.ExecContext(ctx, query,
		req.Id, userA, req.AccountId, req.Amount.String(), desc, req.Date, req.Date, req.Date); err != nil {
		return nil, err
	}
	return &model.CreateTransactionResult{Item: model.TransactionResult{Id: req.Id, AccountId: req.AccountId, Amount: req.Amount.String()}}, nil
}
func (f *fakeTxns) UpdateTransaction(_ context.Context, _ vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) {
	f.updated = append(f.updated, req)
	return &model.UpdateTransactionResult{}, nil
}
func (f *fakeTxns) ListByAccount(context.Context, vo.Id, time.Time, time.Time) ([]*model.Transaction, error) {
	return nil, nil
}

func (f *fakeTxns) GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error) {
	q := f.db.TX.Querier(ctx)
	var (
		accountID, amount, description string
		typ                            int
		spentAt                        time.Time
	)
	row := q.QueryRowContext(ctx, f.db.Rebind(
		`SELECT type, account_id, amount, description, spent_at FROM transactions WHERE id = ?`), id.String())
	if err := row.Scan(&typ, &accountID, &amount, &description, &spentAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Transaction not found")
		}
		return nil, err
	}
	return &model.Transaction{
		ID: id, UserID: vo.MustParseId(userA), Type: model.TransactionType(typ),
		AccountID: vo.MustParseId(accountID), Amount: amount, Description: description, SpentAt: spentAt,
	}, nil
}

func (f *fakeTxns) UpdateTransactionReplacingLabels(ctx context.Context, _ vo.Id, req model.UpdateTransactionRequest) (*model.UpdateTransactionResult, error) {
	f.replaced = append(f.replaced, req)
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	q := f.db.TX.Querier(ctx)
	if _, err := q.ExecContext(ctx, f.db.Rebind(`UPDATE transactions SET description = ? WHERE id = ?`), description, req.Id); err != nil {
		return nil, err
	}
	return &model.UpdateTransactionResult{}, nil
}

// fakeEntities lists owned ids as plain slices; every id listed counts as
// user A's.
type fakeEntities struct{ categories, payees, tags, labels []string }

func named(ids []string) []model.ImportNamed {
	out := make([]model.ImportNamed, 0, len(ids))
	for _, id := range ids {
		out = append(out, model.ImportNamed{ID: id, Name: id, OwnerID: userA})
	}
	return out
}

func (f *fakeEntities) CategoriesByOwner(context.Context, vo.Id) ([]model.ImportNamed, error) {
	return named(f.categories), nil
}
func (f *fakeEntities) PayeesByOwner(context.Context, vo.Id) ([]model.ImportNamed, error) {
	return named(f.payees), nil
}
func (f *fakeEntities) TagsByOwner(context.Context, vo.Id) ([]model.ImportNamed, error) {
	return named(f.tags), nil
}
func (f *fakeEntities) LabelsByOwner(context.Context, vo.Id) ([]model.ImportNamed, error) {
	return named(f.labels), nil
}

type harness struct {
	srv      *httptest.Server
	txns     *fakeTxns
	entities *fakeEntities
	f        *fixture.Builder
	provider *stubProvider
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db).At(now)
	f.User(fixture.User{ID: userA, Email: "a@example.test", Name: "A"})
	f.Account(fixture.Account{ID: acct1, UserID: userA, CurrencyID: usdID, Name: "Card"})
	f.ImportSource(fixture.ImportSource{ID: source, UserID: userA, Name: "iPhone"})
	txns := &fakeTxns{db: db}
	entities := &fakeEntities{}
	svc := appimports.NewService(importsrepo.NewRepo(db.Engine, db.TX), fakeAccounts{}, fakeConverter{}, txns, txns, entities, nil, db.TX, clock{now}, appimports.DefaultMatcherConfig())
	svc.RegisterParser(model.ImportProviderAppleWallet, applewallet.Parser{})
	svc.RegisterParser(model.ImportProviderSimpleFIN, simplefin.Parser{})
	provider := &stubProvider{}
	svc.RegisterProvider(model.ImportProviderSimpleFIN, provider)

	mux := http.NewServeMux()
	handlerimports.RegisterAPI(handlerimports.NewHandlers(svc), authstub.Authenticator{})(mux)
	srv := httptest.NewServer(middleware.Chain(middleware.RequestID, middleware.AccessLog)(mux))
	t.Cleanup(srv.Close)
	return &harness{srv: srv, txns: txns, entities: entities, f: f, provider: provider}
}
