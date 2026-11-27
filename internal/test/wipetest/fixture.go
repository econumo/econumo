// Package wipetest proves the hard delete of a user end to end, on the real
// service graph, against the design's fixture.
package wipetest

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/config"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

// Today is the fixed clock of every graph built here: debts are read as of it.
var Today = time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return Today }

// Build wires the production graph on db with the fixed clock.
func Build(t testing.TB, db *dbtest.DB) *server.Built {
	t.Helper()
	// Inactivity on: with it off the wipe releases every inactive mark instead.
	cfg := config.Config{DatabaseDriver: db.Engine, CurrencyBase: "USD", RateLimitWindow: 15 * time.Minute, InactiveUserDays: 365}
	b, err := server.BuildWithServices(cfg, db.Raw, server.Seams{Clock: fixedClock{}, Avatars: appuser.FixedAvatarPicker(appuser.DefaultAvatar)})
	if err != nil {
		t.Fatalf("BuildWithServices: %v", err)
	}
	return b
}

// Fixture is the design's wipe fixture: U is wiped, P is a partner sharing
// both ways, X an ex-partner, T a third user sharing a budget with U and P.
type Fixture struct {
	U, P, X, T string
	UEmail     string

	UCash, UCard                string
	PCash, PCard, PSave, PSpare string
	XAcc, TAcc                  string
	UCurrency                   string
	UBudget, PBudget, TBudget   string
	UCat1, UCat2, PCat1, PCat2  string
	PCat3                       string
	Envelope                    string
	PCat1Element                string
	Comment                     string
	UToPTransfer, PToUTransfer  string
	LegacyTransfer              string
	UWroteInP, UWroteInX        string
	PRowWithUCategory           string
	D3Payment                   string
	D1, D2, D3                  string
	D3Baseline                  string
	App, TGrant                 string
	IdempotencyKey, OAuthState  string
	OtherAccounts               []string
	MovedAccounts               []string
}

func day(m time.Month, d int) time.Time { return time.Date(2026, m, d, 12, 0, 0, 0, time.UTC) }

// Seed builds the fixture. Debts go through the debt service of b so they
// are set up exactly as the API sets them up; D3's payment split is then
// materialized by the debt sync on a read.
func Seed(t *testing.T, db *dbtest.DB, b *server.Built) Fixture {
	t.Helper()
	ctx := context.Background()
	f := fixture.New(t, db)
	var fx Fixture
	fx.UEmail = "wiped@wipe.test"
	fx.U = f.User(fixture.User{Email: fx.UEmail, Name: "U"})
	fx.P = f.User(fixture.User{Email: "partner@wipe.test", Name: "P"})
	fx.X = f.User(fixture.User{Email: "ex@wipe.test", Name: "X"})
	fx.T = f.User(fixture.User{Email: "third@wipe.test", Name: "T"})
	f.Connect(fx.U, fx.P)
	f.Connect(fx.U, fx.T)
	f.Connect(fx.P, fx.T)

	fx.UCurrency = f.Currency(fixture.Currency{Code: "UCC", Symbol: "U", UserID: fx.U, Rate: "1.00000000"})
	f.Rate(fixture.Rate{CurrencyID: fx.UCurrency, Rate: "1.00000000", PublishedAt: "2026-01-01"})

	fx.UCash = f.Account(fixture.Account{UserID: fx.U, Name: "U cash"})
	fx.UCard = f.Account(fixture.Account{UserID: fx.U, Name: "U card"})
	fx.PCash = f.Account(fixture.Account{UserID: fx.P, Name: "P cash"})
	fx.PCard = f.Account(fixture.Account{UserID: fx.P, Name: "P card"})
	fx.PSave = f.Account(fixture.Account{UserID: fx.P, Name: "P savings"})
	fx.PSpare = f.Account(fixture.Account{UserID: fx.P, Name: "P spare"})
	fx.XAcc = f.Account(fixture.Account{UserID: fx.X, Name: "X cash"})
	fx.TAcc = f.Account(fixture.Account{UserID: fx.T, Name: "T cash"})
	f.AccountAccess(fx.UCash, fx.P, 1)
	f.AccountAccess(fx.PCash, fx.U, 1)

	fx.UCat1 = f.Category(fixture.Category{UserID: fx.U, Name: "U loan"})
	fx.UCat2 = f.Category(fixture.Category{UserID: fx.U, Name: "U car", Position: 1})
	fx.PCat1 = f.Category(fixture.Category{UserID: fx.P, Name: "P food"})
	fx.PCat2 = f.Category(fixture.Category{UserID: fx.P, Name: "P car", Position: 1})
	fx.PCat3 = f.Category(fixture.Category{UserID: fx.P, Name: "P fuel", Position: 2})
	f.Category(fixture.Category{UserID: fx.T, Name: "T misc"})

	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fx.UBudget = f.Budget(fixture.Budget{UserID: fx.U, Name: "U budget", CurrencyID: fx.UCurrency, StartedAt: started})
	f.BudgetAccess(fx.UBudget, fx.P, 1, true)
	f.BudgetAccount(fx.UBudget, fx.UCash)
	f.BudgetAccount(fx.UBudget, fx.PCash)
	f.Option(fx.P, "budget", &fx.UBudget)

	fx.PBudget = f.Budget(fixture.Budget{UserID: fx.P, Name: "P budget", CurrencyID: fx.UCurrency, StartedAt: started})
	f.BudgetAccess(fx.PBudget, fx.U, 1, true)
	f.BudgetAccount(fx.PBudget, fx.PCash)
	f.BudgetAccount(fx.PBudget, fx.UCash)
	fx.PCat1Element = f.BudgetElement(fixture.BudgetElement{BudgetID: fx.PBudget, ExternalID: fx.PCat1, Type: 1})
	f.BudgetElement(fixture.BudgetElement{BudgetID: fx.PBudget, ExternalID: fx.PCat2, Type: 1, CurrencyID: fx.UCurrency, Position: 1})
	f.BudgetElement(fixture.BudgetElement{BudgetID: fx.PBudget, ExternalID: fx.UCat1, Type: 1, Position: 2})
	fx.Envelope = f.BudgetEnvelope(fixture.BudgetEnvelope{BudgetID: fx.PBudget, Name: "Shared car"})
	f.EnvelopeCategory(fx.Envelope, fx.UCat2)
	f.EnvelopeCategory(fx.Envelope, fx.PCat3)
	f.BudgetElement(fixture.BudgetElement{BudgetID: fx.PBudget, ExternalID: fx.Envelope, Type: 0, Position: 3})
	fx.Comment = fixture.NewID()
	db.Exec(t, db.Rebind(`INSERT INTO budgets_elements_comments (id, element_id, period, user_id, comment, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		fx.Comment, fx.PCat1Element, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), fx.U, "from U", Today, Today)

	fx.TBudget = f.Budget(fixture.Budget{UserID: fx.T, Name: "T budget", StartedAt: started})
	f.BudgetAccess(fx.TBudget, fx.U, 1, true)
	f.BudgetAccess(fx.TBudget, fx.P, 1, true)
	f.BudgetAccount(fx.TBudget, fx.TAcc)

	f.Transaction(fixture.Transaction{UserID: fx.P, AccountID: fx.PCash, Type: 1, Amount: "1000", SpentAt: day(7, 1)})
	f.Transaction(fixture.Transaction{UserID: fx.P, AccountID: fx.PSpare, Type: 1, Amount: "5", SpentAt: day(7, 1)})
	f.Transaction(fixture.Transaction{UserID: fx.P, AccountID: fx.PCash, Type: 2, Amount: "3",
		AccountRecipientID: fx.PSpare, AmountRecipient: "3", SpentAt: day(7, 3)})
	f.Transaction(fixture.Transaction{UserID: fx.T, AccountID: fx.TAcc, Type: 0, Amount: "7", SpentAt: day(7, 2)})
	f.Transaction(fixture.Transaction{UserID: fx.U, AccountID: fx.UCash, Type: 1, Amount: "500", SpentAt: day(7, 1)})
	fx.UToPTransfer = f.Transaction(fixture.Transaction{UserID: fx.U, AccountID: fx.UCash, Type: 2, Amount: "100",
		AccountRecipientID: fx.PCash, AmountRecipient: "100", Description: "rent share", SpentAt: day(7, 3)})
	fx.PToUTransfer = f.Transaction(fixture.Transaction{UserID: fx.P, AccountID: fx.PCash, Type: 2, Amount: "30",
		AccountRecipientID: fx.UCash, AmountRecipient: "30", SpentAt: day(7, 3)})
	fx.LegacyTransfer = f.Transaction(fixture.Transaction{UserID: fx.U, AccountID: fx.UCard, Type: 2, Amount: "50",
		AccountRecipientID: fx.PCard, Description: "legacy", SpentAt: day(7, 4)})
	fx.UWroteInP = f.Transaction(fixture.Transaction{UserID: fx.U, AccountID: fx.PCash, Type: 0, Amount: "20",
		CategoryID: fx.PCat1, Description: "groceries by U", SpentAt: day(7, 3)})
	fx.UWroteInX = f.Transaction(fixture.Transaction{UserID: fx.U, AccountID: fx.XAcc, Type: 0, Amount: "15", SpentAt: day(7, 5)})
	fx.PRowWithUCategory = f.Transaction(fixture.Transaction{UserID: fx.P, AccountID: fx.PSave, Type: 0, Amount: "10",
		CategoryID: fx.UCat1, SpentAt: day(7, 5)})

	fx.App = f.OAuthClient(fixture.OAuthClient{Name: "U app", RedirectURIs: []string{"https://app.wipe.test/cb"}, OwnerUserID: fx.U})
	fx.TGrant = f.OAuthGrant(fixture.OAuthGrant{UserID: fx.T, ClientID: fx.App, RefreshHash: "t-refresh-hash"})
	fx.IdempotencyKey = fixture.NewID()
	db.Exec(t, db.Rebind(`INSERT INTO idempotency_keys (user_id, idempotency_key, resource_id, created_at) VALUES (?, ?, ?, ?)`),
		fx.U, fx.IdempotencyKey, fx.UToPTransfer, Today)
	fx.OAuthState = "state-hash-of-u"
	db.Exec(t, db.Rebind(`INSERT INTO oauth_states (state_hash, provider, nonce, client, intent, link_user_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		fx.OAuthState, "google", "n", "web", "link", fx.U, Today, Today.Add(10*time.Minute))

	pid := vo.MustParseId(fx.P)
	debt := func(elementID string) string {
		res, err := b.Debts.CreateDebt(ctx, pid, model.CreateDebtRequest{BudgetId: fx.PBudget, ElementId: elementID,
			AmountBorrowed: vo.NewFlexString("12000"), StartDate: ptr("2026-03-01"), Payment: vo.NewFlexString("600"),
			Schedule: "monthly", LastPaymentDate: "2028-02-01"})
		if err != nil {
			t.Fatalf("CreateDebt on %s: %v", elementID, err)
		}
		return res.Id
	}
	fx.D1 = debt(fx.UCat1)
	fx.D2 = debt(fx.PCat2)
	fx.D3 = debt(fx.Envelope)
	fx.D3Baseline = readDebt(t, b, fx.P, fx.D3).Item.CurrentBalance
	fx.D3Payment = f.Transaction(fixture.Transaction{UserID: fx.U, AccountID: fx.UCash, Type: 0, Amount: "600",
		CategoryID: fx.UCat2, SpentAt: day(4, 15)})

	fx.OtherAccounts = []string{fx.PCash, fx.PCard, fx.PSave, fx.PSpare, fx.XAcc, fx.TAcc}
	fx.MovedAccounts = []string{fx.PCash, fx.PCard, fx.PSave, fx.XAcc}
	return fx
}

func readDebt(t *testing.T, b *server.Built, userID, debtID string) *model.DebtDetailsResult {
	t.Helper()
	res, err := b.Debts.GetDebt(context.Background(), vo.MustParseId(userID), model.GetDebtRequest{Id: debtID})
	if err != nil {
		t.Fatalf("GetDebt %s: %v", debtID, err)
	}
	return res
}

func ptr[T any](v T) *T { return &v }
