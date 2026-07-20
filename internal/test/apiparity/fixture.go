package apiparity

// Identity + entity ids used across the catalogue. UUIDs are literal so both
// engines store the same keys; the comparison is over the API RESPONSES.

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

const (
	OwnerID    = "11111111-1111-1111-1111-111111111111"
	OwnerEmail = "owner@example.test"
	GuestID    = "22222222-2222-2222-2222-222222222222"
	GuestEmail = "guest@example.test"
	// A third user whose trial has lapsed: access_level "full" but access_until
	// in the past, which EffectiveAccessLevel collapses to read-only. Stored as
	// full-with-expiry rather than a bare "readonly" so the access_until column
	// itself round-trips through the API on both engines (SQLite DATETIME vs
	// PostgreSQL TIMESTAMP) — the parity property the 402 scenario exists to pin.
	//
	// NOT 3333…: enginecompare's connection-invite test seeds its own third user
	// at that id on top of this fixture, and the two would collide on users.id.
	ReadonlyID    = "88888888-8888-8888-8888-888888888888"
	ReadonlyEmail = "readonly@example.test"

	USD = "dffc2a06-6f29-4704-8575-31709adee926" // seeded by the baseline migration

	OwnerFolder   = "f0000000-0000-0000-0000-000000000001"
	GuestFolder   = "f0000000-0000-0000-0000-000000000002"
	OwnerAccount  = "a0000000-0000-0000-0000-000000000001"
	SharedAccount = "a0000000-0000-0000-0000-000000000002" // owned by guest, shared to owner

	CatFood   = "c0000000-0000-0000-0000-000000000001"
	CatSalary = "c0000000-0000-0000-0000-000000000002"
	TagWork   = "10000000-0000-0000-0000-000000000001"
	PayeeShop = "20000000-0000-0000-0000-000000000001"

	Txn1 = "d0000000-0000-0000-0000-000000000001"
	Txn2 = "d0000000-0000-0000-0000-000000000002"

	Budget = "b0000000-0000-0000-0000-000000000001"

	OwnerFolder2  = "f0000000-0000-0000-0000-000000000003"
	Budget2       = "b0000000-0000-0000-0000-000000000002"
	BudgetFolder1 = "bf000000-0000-0000-0000-000000000001"
	Envelope1     = "be000000-0000-0000-0000-000000000001"
	ElementFood   = "e0000000-0000-0000-0000-000000000001"

	SeedPassword = "secret-pw"

	// Raw seeded bearer tokens (43-char payloads: "owner-seed-token-" is 17
	// chars + 26 zeros — deliberately NOT random so both engines seed identical
	// rows). Their sha256 goes into access_tokens.
	OwnerToken    = "eco_ses_owner-seed-token-00000000000000000000000000"
	GuestToken    = "eco_ses_guest-seed-token-00000000000000000000000000"
	ReadonlyToken = "eco_ses_readonly-seed-token-00000000000000000000000"

	// Seeded session row ids (fixed, non-v7 so they survive normalization —
	// scenarios reference them, e.g. err:revoke-session-foreign).
	OwnerSessionID = "55555555-5555-5555-5555-555555555555"
	GuestSessionID = "66666666-6666-6666-6666-666666666666"

	ReadonlySessionID = "77777777-7777-7777-7777-777777777777"
)

// ReadonlyAccessUntil is the lapsed trial expiry seeded on the read-only user.
// A literal past instant (not clock-derived) so both engines persist the same
// bytes and the value the API echoes back is stable across runs.
var ReadonlyAccessUntil = time.Date(2020, 1, 15, 10, 30, 0, 0, time.UTC)

// Seed seeds an identical, cross-module fixture into the given engine via the
// typed fixture builder. It seeds: two connected users (with hashed password +
// encrypted email so login works) plus an isolated lapsed-trial user, their
// default options, folders, an owned
// account and a guest-owned account shared to the owner, categories, a tag, a
// payee, two transactions, and a budget — so every read endpoint returns
// non-empty data on both engines. Fixed ids are used (the scenarios reference
// them in request bodies); the comparison is over the API RESPONSES.
func Seed(t testing.TB, db *dbtest.DB) {
	t.Helper()
	// Plaintext seed (empty salt) to match the salt-free API; cfg.DataSalt is set
	// but ignored, so seeding salted data would make login + lookups mismatch.
	f := fixture.New(t, db).WithCrypto("")

	f.User(fixture.User{ID: OwnerID, Email: OwnerEmail, Name: "User " + OwnerID[:4], Password: SeedPassword})
	f.DefaultOptions(OwnerID)
	f.User(fixture.User{ID: GuestID, Email: GuestEmail, Name: "User " + GuestID[:4], Password: SeedPassword})
	f.DefaultOptions(GuestID)

	// Lapsed-trial user: full access that ran out in the past, so every request
	// it makes is evaluated as read-only. Deliberately NOT connected to the
	// owner/guest and given no entities, so it stays invisible to every other
	// scenario's responses.
	readonlyUntil := ReadonlyAccessUntil
	f.User(fixture.User{ID: ReadonlyID, Email: ReadonlyEmail, Name: "User " + ReadonlyID[:4], Password: SeedPassword,
		AccessLevel: string(model.AccessLevelFull), AccessUntil: &readonlyUntil})
	f.DefaultOptions(ReadonlyID)

	// Live sessions for each seeded user: the harness presents the matching
	// Owner/Guest/Readonly token as a bearer token, resolving to these rows.
	ownerExp := ClockTime.Add(appuser.SessionTTL)
	guestExp := ownerExp
	f.AccessToken(fixture.AccessToken{ID: OwnerSessionID, UserID: OwnerID, Kind: model.TokenKindSession,
		TokenHash: appuser.HashAccessToken(OwnerToken), UserAgent: "apiparity", ExpiresAt: &ownerExp})
	f.AccessToken(fixture.AccessToken{ID: GuestSessionID, UserID: GuestID, Kind: model.TokenKindSession,
		TokenHash: appuser.HashAccessToken(GuestToken), UserAgent: "apiparity", ExpiresAt: &guestExp})
	// The session itself is live; only the USER's access has lapsed, so the
	// scenario exercises the 402 path rather than a 401.
	f.AccessToken(fixture.AccessToken{ID: ReadonlySessionID, UserID: ReadonlyID, Kind: model.TokenKindSession,
		TokenHash: appuser.HashAccessToken(ReadonlyToken), UserAgent: "apiparity", ExpiresAt: &ownerExp})
	f.Connect(OwnerID, GuestID)

	// Folders.
	f.Folder(fixture.Folder{ID: OwnerFolder, UserID: OwnerID, Name: "Main"})
	f.Folder(fixture.Folder{ID: GuestFolder, UserID: GuestID, Name: "Main"})

	// Owner's own account.
	f.Account(fixture.Account{ID: OwnerAccount, UserID: OwnerID, CurrencyID: USD, Name: "Cash"})
	f.AccountInFolder(OwnerFolder, OwnerAccount)
	f.AccountOption(OwnerAccount, OwnerID, 0)

	// Guest-owned account, SHARED to the owner (accounts_access grant) so the
	// owner's get-account-list / sharedAccess[] / connection list are non-empty.
	f.Account(fixture.Account{ID: SharedAccount, UserID: GuestID, CurrencyID: USD, Name: "Shared"})
	f.AccountInFolder(GuestFolder, SharedAccount)
	f.AccountOption(SharedAccount, GuestID, 0)
	f.AccountAccess(SharedAccount, OwnerID, 1)

	// Categories (owner): one expense, one income.
	f.Category(fixture.Category{ID: CatFood, UserID: OwnerID, Name: "Food", Position: 0, Type: 0})
	f.Category(fixture.Category{ID: CatSalary, UserID: OwnerID, Name: "Salary", Position: 1, Type: 1})

	// Tag + payee (owner).
	f.Tag(fixture.Tag{ID: TagWork, UserID: OwnerID, Name: "Work"})
	f.Payee(fixture.Payee{ID: PayeeShop, UserID: OwnerID, Name: "Shop"})

	// Transactions on the owner's account (one expense, one income). The domain
	// enum is expense=0 / income=1 (internal/transaction/entity.go), so the
	// types match each row's category semantics: Txn1 is a Food EXPENSE, Txn2 a
	// Salary INCOME.
	f.Transaction(fixture.Transaction{ID: Txn1, UserID: OwnerID, AccountID: OwnerAccount, CategoryID: CatFood, PayeeID: PayeeShop, Type: 0, Amount: "12.50000000", Description: "lunch"})
	f.Transaction(fixture.Transaction{ID: Txn2, UserID: OwnerID, AccountID: OwnerAccount, CategoryID: CatSalary, Type: 1, Amount: "1000.00000000", Description: "pay"})

	// A budget owned by the owner.
	f.Budget(fixture.Budget{ID: Budget, UserID: OwnerID, CurrencyID: USD, Name: "Budget"})

	// Second account folder (replace-folder / order-folder-list targets).
	f.Folder(fixture.Folder{ID: OwnerFolder2, UserID: OwnerID, Name: "Spare"})

	// Second budget with no invites (grant-access target).
	f.Budget(fixture.Budget{ID: Budget2, UserID: OwnerID, CurrencyID: USD, Name: "Second"})

	// Budget structure: a folder, an envelope (with a category link), and a
	// category element row so move/change-currency/envelope scenarios have fixed
	// ids to reference.
	f.BudgetFolder(fixture.BudgetFolder{ID: BudgetFolder1, BudgetID: Budget, Name: "Bills"})
	f.BudgetEnvelope(fixture.BudgetEnvelope{ID: Envelope1, BudgetID: Budget, Name: "Envelope", Icon: "cart"})
	f.EnvelopeCategory(Envelope1, CatSalary)
	f.BudgetElement(fixture.BudgetElement{ID: ElementFood, BudgetID: Budget, ExternalID: CatFood, Type: 1, Position: 0}) // category element (envelope=0, category=1, tag=2)

	// Pending (not accepted) budget invite: guest invited to Budget — the
	// accept-access and decline-access scenarios each consume it on a fresh DB.
	// role=1 is budget.RoleUser (internal/budget/valueobject.go: admin=0,
	// user=1, guest=2).
	f.BudgetAccess(Budget, GuestID, 1, false)
}
