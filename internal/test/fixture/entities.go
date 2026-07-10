package fixture

import (
	"strconv"

	"github.com/econumo/econumo/internal/shared/vo"
)

// NewID returns a fresh UUID string (UUIDv7, matching production id generation).
// Exposed so tests can mint ids they reference before seeding.
func NewID() string { return vo.NewId().Value() }

// USD is the US-dollar currency id seeded by the baseline migration; the most
// common currency tests attach accounts/budgets to.
const USD = "dffc2a06-6f29-4704-8575-31709adee926"

// User describes a user row. Zero fields take defaults: a fresh id, a derived
// email/name, active=true. When the Builder has WithCrypto, identifier + email
// are stored encrypted and the password is hashed (so login works); otherwise
// literal placeholder values are stored (fine for tests that never authenticate).
type User struct {
	ID       string
	Email    string // default "user-<id8>@example.test"
	Name     string // default "User <id4>"
	Avatar   string
	Password string // default "secret-pw" (only meaningful WithCrypto)
	Salt     string // default 40-char sha1-shaped salt
	Inactive bool   // default active
}

// defaultSalt is a fixed 40-char sha1-shaped salt for seeded users.
const defaultSalt = "0000000000000000000000000000000000000001"

func (b *Builder) User(u User) string {
	b.t.Helper()
	id := b.orNewID(u.ID)
	if u.Email == "" {
		u.Email = "user-" + id[:8] + "@example.test"
	}
	if u.Name == "" {
		u.Name = "User " + id[:4]
	}
	if u.Salt == "" {
		u.Salt = defaultSalt
	}
	if u.Password == "" {
		u.Password = "secret-pw"
	}

	identifier, email, password := u.Email, u.Email, u.Password
	if b.encode != nil {
		identifier = b.encode.Hash(lower(u.Email))
		enc, err := b.encode.Encode(u.Email)
		if err != nil {
			b.t.Fatalf("fixture: encode email: %v", err)
		}
		email = enc
		password = b.hasher.Hash(u.Password, u.Salt)
	} else {
		// Keep identifier unique without crypto (the column is UNIQUE).
		identifier = "ident-" + id[:8]
	}

	now := b.now()
	active := "TRUE"
	if u.Inactive {
		active = "FALSE"
	}
	b.insert(`INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, algorithm, created_at, updated_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'sha512', ?, ?, `+active+`)`,
		id, identifier, email, u.Name, u.Avatar, password, u.Salt, now, now)
	return id
}

// Option seeds a single users_options row. value is nil for a NULL value.
func (b *Builder) Option(userID, name string, value *string) {
	b.t.Helper()
	now := b.now()
	if value == nil {
		b.insert(`INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, ?, NULL, ?, ?)`,
			NewID(), userID, name, now, now)
		return
	}
	b.insert(`INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		NewID(), userID, name, *value, now, now)
}

// DefaultOptions seeds the four standard options a registered user has (currency,
// report_period, onboarding, budget=NULL) — the production registration set.
//
// All four are seeded with an IDENTICAL created_at, exactly as production does
// (one clock.Now() per registration stamps every option). This faithfully
// reproduces the case where the options query's created_at sort has no natural
// tiebreak — so the query's secondary "ORDER BY ..., id" is what makes the order
// deterministic and identical across engines. Seeding them at distinct times
// would mask that. Their ids are fixed + strictly increasing so the resulting
// order is stable.
func (b *Builder) DefaultOptions(userID string) {
	b.t.Helper()
	ts := b.now() // a single timestamp shared by all four rows
	s := func(v string) *string { return &v }
	// Per-user, strictly increasing option ids (derived from the user id so two
	// users' option rows never collide on the primary key).
	prefix := userID
	if len(prefix) >= 8 {
		prefix = prefix[:8]
	}
	for i, o := range []struct {
		name  string
		value *string
	}{
		{"currency", s("USD")},
		{"report_period", s("monthly")},
		{"onboarding", s("started")},
		{"budget", nil},
	} {
		id := prefix + "-0ec0-0000-0000-00000000000" + strconv.Itoa(i+1)
		if o.value == nil {
			b.insert(`INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, ?, NULL, ?, ?)`,
				id, userID, o.name, ts, ts)
		} else {
			b.insert(`INSERT INTO users_options (id, user_id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
				id, userID, o.name, *o.value, ts, ts)
		}
	}
}

// Connect seeds a bidirectional users_connections link between two users.
func (b *Builder) Connect(userA, userB string) {
	b.t.Helper()
	b.insert(`INSERT INTO users_connections (user_id, connected_user_id) VALUES (?, ?)`, userA, userB)
	b.insert(`INSERT INTO users_connections (user_id, connected_user_id) VALUES (?, ?)`, userB, userA)
}

// Currency describes a currencies row. Code/Symbol default to a USD-like entry;
// most tests reuse the baseline-seeded USD (the USD const) instead.
type Currency struct {
	ID             string
	Code           string // default "USD"
	Symbol         string // default "$"
	Name           string // nullable; empty -> NULL
	FractionDigits *int   // default 2; pointer so an explicit 0 (e.g. JPY/unknown) is honored
}

func (b *Builder) Currency(c Currency) string {
	b.t.Helper()
	id := b.orNewID(c.ID)
	if c.Code == "" {
		c.Code = "USD"
	}
	if c.Symbol == "" {
		c.Symbol = "$"
	}
	digits := 2
	if c.FractionDigits != nil {
		digits = *c.FractionDigits
	}
	now := b.now()
	if c.Name == "" {
		b.insert(`INSERT INTO currencies (id, code, symbol, name, fraction_digits, created_at) VALUES (?, ?, ?, NULL, ?, ?)`,
			id, c.Code, c.Symbol, digits, now)
	} else {
		b.insert(`INSERT INTO currencies (id, code, symbol, name, fraction_digits, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			id, c.Code, c.Symbol, c.Name, digits, now)
	}
	return id
}

// Rate describes a currencies_rates row.
type Rate struct {
	ID             string
	CurrencyID     string
	BaseCurrencyID string // default USD
	Rate           string // decimal string, e.g. "0.85000000"
	PublishedAt    string // "Y-m-d"; default the builder's current date
}

func (b *Builder) Rate(r Rate) string {
	b.t.Helper()
	id := b.orNewID(r.ID)
	if r.BaseCurrencyID == "" {
		r.BaseCurrencyID = USD
	}
	if r.Rate == "" {
		r.Rate = "1.00000000"
	}
	pub := r.PublishedAt
	if pub == "" {
		pub = b.now().Format("2006-01-02")
	}
	b.insert(`INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate) VALUES (?, ?, ?, ?, ?)`,
		id, r.CurrencyID, r.BaseCurrencyID, pub, r.Rate)
	return id
}

// Folder describes a folders row.
type Folder struct {
	ID       string
	UserID   string
	Name     string // default "Main"
	Position int
	Hidden   bool // default visible
}

func (b *Builder) Folder(f Folder) string {
	b.t.Helper()
	id := b.orNewID(f.ID)
	if f.Name == "" {
		f.Name = "Main"
	}
	visible := "TRUE"
	if f.Hidden {
		visible = "FALSE"
	}
	now := b.now()
	b.insert(`INSERT INTO folders (id, user_id, name, position, is_visible, created_at, updated_at) VALUES (?, ?, ?, ?, `+visible+`, ?, ?)`,
		id, f.UserID, f.Name, f.Position, now, now)
	return id
}

// Account describes an accounts row.
type Account struct {
	ID         string
	UserID     string
	CurrencyID string // default USD
	Name       string // default "Account"
	Type       int    // default 2 (regular)
	Icon       string // default "wallet"
	Deleted    bool
}

func (b *Builder) Account(a Account) string {
	b.t.Helper()
	id := b.orNewID(a.ID)
	if a.CurrencyID == "" {
		a.CurrencyID = USD
	}
	if a.Name == "" {
		a.Name = "Account"
	}
	if a.Type == 0 {
		a.Type = 2
	}
	if a.Icon == "" {
		a.Icon = "wallet"
	}
	deleted := "FALSE"
	if a.Deleted {
		deleted = "TRUE"
	}
	now := b.now()
	b.insert(`INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, `+deleted+`, ?, ?)`,
		id, a.CurrencyID, a.UserID, a.Name, a.Type, a.Icon, now, now)
	return id
}

// AccountInFolder links an account into a folder (accounts_folders).
func (b *Builder) AccountInFolder(folderID, accountID string) {
	b.t.Helper()
	b.insert(`INSERT INTO accounts_folders (folder_id, account_id) VALUES (?, ?)`, folderID, accountID)
}

// AccountOption seeds an accounts_options row (per-user account position).
func (b *Builder) AccountOption(accountID, userID string, position int) {
	b.t.Helper()
	now := b.now()
	b.insert(`INSERT INTO accounts_options (account_id, user_id, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, userID, position, now, now)
}

// AccountAccess grants a user access to an account (accounts_access). role is
// stored verbatim (admin=0, user=1, guest=2) — 0 is a real role, not "unset", so
// it is NOT coerced.
func (b *Builder) AccountAccess(accountID, userID string, role int) {
	b.t.Helper()
	now := b.now()
	b.insert(`INSERT INTO accounts_access (account_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		accountID, userID, role, now, now)
}

// Category describes a categories row.
type Category struct {
	ID       string
	UserID   string
	Name     string // default "Category"
	Position int
	Type     int    // 0 expense, 1 income
	Icon     string // default "i"
	Archived bool
}

func (b *Builder) Category(c Category) string {
	b.t.Helper()
	id := b.orNewID(c.ID)
	if c.Name == "" {
		c.Name = "Category"
	}
	if c.Icon == "" {
		c.Icon = "i"
	}
	arch := "FALSE"
	if c.Archived {
		arch = "TRUE"
	}
	now := b.now()
	b.insert(`INSERT INTO categories (id, user_id, name, position, type, icon, is_archived, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, `+arch+`, ?, ?)`,
		id, c.UserID, c.Name, c.Position, c.Type, c.Icon, now, now)
	return id
}

// Tag describes a tags row.
type Tag struct {
	ID       string
	UserID   string
	Name     string // default "Tag"
	Position int
	Archived bool
}

func (b *Builder) Tag(tg Tag) string {
	b.t.Helper()
	id := b.orNewID(tg.ID)
	if tg.Name == "" {
		tg.Name = "Tag"
	}
	arch := "FALSE"
	if tg.Archived {
		arch = "TRUE"
	}
	now := b.now()
	b.insert(`INSERT INTO tags (id, user_id, name, position, is_archived, created_at, updated_at) VALUES (?, ?, ?, ?, `+arch+`, ?, ?)`,
		id, tg.UserID, tg.Name, tg.Position, now, now)
	return id
}

// Payee describes a payees row.
type Payee struct {
	ID       string
	UserID   string
	Name     string // default "Payee"
	Position int
	Archived bool
}

func (b *Builder) Payee(p Payee) string {
	b.t.Helper()
	id := b.orNewID(p.ID)
	if p.Name == "" {
		p.Name = "Payee"
	}
	arch := "FALSE"
	if p.Archived {
		arch = "TRUE"
	}
	now := b.now()
	b.insert(`INSERT INTO payees (id, user_id, name, position, is_archived, created_at, updated_at) VALUES (?, ?, ?, ?, `+arch+`, ?, ?)`,
		id, p.UserID, p.Name, p.Position, now, now)
	return id
}

// Transaction describes a transactions row. CategoryID/PayeeID/TagID are
// optional (empty -> NULL). SpentAt defaults to the builder's current time.
type Transaction struct {
	ID          string
	UserID      string
	AccountID   string
	CategoryID  string // optional
	PayeeID     string // optional
	TagID       string // optional
	Type        int    // 0 expense, 1 income (domain transaction.TypeExpense/TypeIncome)
	Amount      string // decimal string; default "0"
	Description string
	SpentAt     interface{} // time.Time or "Y-m-d H:i:s"; default builder time
}

func (b *Builder) Transaction(tx Transaction) string {
	b.t.Helper()
	id := b.orNewID(tx.ID)
	if tx.Amount == "" {
		tx.Amount = "0.00000000"
	}
	now := b.now()
	var spent any = now
	if tx.SpentAt != nil {
		spent = tx.SpentAt
	}
	cat := nullable(tx.CategoryID)
	payee := nullable(tx.PayeeID)
	tag := nullable(tx.TagID)
	b.insert(`INSERT INTO transactions (id, user_id, account_id, category_id, payee_id, tag_id, type, amount, description, spent_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, tx.UserID, tx.AccountID, cat, payee, tag, tx.Type, tx.Amount, tx.Description, spent, now, now)
	return id
}

// Budget describes a budgets row. StartedAt defaults to the builder's time.
type Budget struct {
	ID         string
	UserID     string
	CurrencyID string // default USD
	Name       string // default "Budget"
	StartedAt  interface{}
}

func (b *Builder) Budget(bg Budget) string {
	b.t.Helper()
	id := b.orNewID(bg.ID)
	if bg.CurrencyID == "" {
		bg.CurrencyID = USD
	}
	if bg.Name == "" {
		bg.Name = "Budget"
	}
	now := b.now()
	var started any = now
	if bg.StartedAt != nil {
		started = bg.StartedAt
	}
	b.insert(`INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, bg.CurrencyID, bg.UserID, bg.Name, started, now, now)
	return id
}

// BudgetElement describes a budgets_elements row. ExternalID is the
// category/tag the element points at. Type matches the domain ElementType enum:
// envelope=0, category=1, tag=2.
type BudgetElement struct {
	ID         string
	BudgetID   string
	ExternalID string
	Type       int
	Position   int
}

func (b *Builder) BudgetElement(e BudgetElement) string {
	b.t.Helper()
	id := b.orNewID(e.ID)
	now := b.now()
	b.insert(`INSERT INTO budgets_elements (id, budget_id, external_id, type, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, e.BudgetID, e.ExternalID, e.Type, e.Position, now, now)
	return id
}

// BudgetLimit describes a budgets_elements_limits row. Period is a "Y-m-d"
// string (month start). Amount is a decimal string.
type BudgetLimit struct {
	ID        string
	ElementID string
	Period    string
	Amount    string
}

func (b *Builder) BudgetLimit(l BudgetLimit) string {
	b.t.Helper()
	id := b.orNewID(l.ID)
	if l.Amount == "" {
		l.Amount = "0.00000000"
	}
	now := b.now()
	b.insert(`INSERT INTO budgets_elements_limits (id, element_id, period, amount, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, l.ElementID, l.Period, l.Amount, now, now)
	return id
}

// BudgetFolder describes a budgets_folders row.
type BudgetFolder struct {
	ID       string
	BudgetID string
	Name     string // default "Folder"
	Position int
}

func (b *Builder) BudgetFolder(f BudgetFolder) string {
	b.t.Helper()
	id := b.orNewID(f.ID)
	if f.Name == "" {
		f.Name = "Folder"
	}
	now := b.now()
	b.insert(`INSERT INTO budgets_folders (id, budget_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, f.BudgetID, f.Name, f.Position, now, now)
	return id
}

// BudgetEnvelope describes a budgets_envelopes row. Name/Icon are nullable;
// empty -> NULL.
type BudgetEnvelope struct {
	ID       string
	BudgetID string
	Name     string
	Icon     string
	Archived bool
}

func (b *Builder) BudgetEnvelope(e BudgetEnvelope) string {
	b.t.Helper()
	id := b.orNewID(e.ID)
	now := b.now()
	b.insert(`INSERT INTO budgets_envelopes (id, budget_id, name, icon, is_archived, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, e.BudgetID, nullable(e.Name), nullable(e.Icon), e.Archived, now, now)
	return id
}

// EnvelopeCategory links a category into a budget envelope
// (budgets_envelopes_categories).
func (b *Builder) EnvelopeCategory(envelopeID, categoryID string) {
	b.t.Helper()
	b.insert(`INSERT INTO budgets_envelopes_categories (budget_envelope_id, category_id) VALUES (?, ?)`,
		envelopeID, categoryID)
}

// BudgetAccess grants userID access to budgetID. role is the stored SMALLINT
// (see internal/budget/valueobject.go: admin=0, user=1, guest=2);
// accepted=false models a pending invite.
func (b *Builder) BudgetAccess(budgetID, userID string, role int, accepted bool) {
	b.t.Helper()
	now := b.now()
	b.insert(`INSERT INTO budgets_access (budget_id, user_id, role, is_accepted, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		budgetID, userID, role, accepted, now, now)
}

// nullable returns nil for an empty string (-> SQL NULL), else the string.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// lower lowercases ASCII without importing strings (kept tiny + allocation-light).
func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
