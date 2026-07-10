// The User entity: the User aggregate root, its UserOption members, the
// Header read projection, and the option-name constants. The repository
// interface, use-case services, and their request/result DTOs stay in
// internal/user; crypto (password hashing, email encryption, identifier
// hashing) lives in the infra auth layer and is invoked by the use-cases, not
// here.
package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// UserOption name constants and their defaults. CurrencyID is a synthetic option
// surfaced only in the API result (it is never persisted as a row); the rest are
// real rows.
const (
	OptionCurrency     = "currency"
	OptionCurrencyID   = "currency_id"
	OptionReportPeriod = "report_period"
	OptionBudget       = "budget"
	OptionOnboarding   = "onboarding"

	DefaultCurrency     = "USD"
	DefaultReportPeriod = "monthly"

	OnboardingStarted   = "started"
	OnboardingCompleted = "completed"
)

// Password-hash algorithm markers stored in users.algorithm. sha512 is the
// legacy scheme (see CLAUDE.md); argon2id is written by every new hash.
const (
	AlgorithmSHA512   = "sha512"
	AlgorithmArgon2id = "argon2id"
)

// persistedOptions are the option names that exist as users_options rows, in
// the order they are seeded at registration. CurrencyID is intentionally absent
// (it is computed in the result, never stored).
var persistedOptions = []string{
	OptionCurrency,
	OptionReportPeriod,
	OptionBudget,
	OptionOnboarding,
}

// UserOption is a single name/value setting belonging to a user. Value is a
// pointer because the column is nullable (e.g. budget defaults to NULL). Fields
// are exported for direct read access; all writes after construction go through
// setValue.
type UserOption struct {
	ID        vo.Id
	Name      string
	Value     *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewUserOption builds a fresh option (used when seeding registration defaults).
func NewUserOption(id vo.Id, name string, value *string, now time.Time) UserOption {
	return UserOption{ID: id, Name: name, Value: value, CreatedAt: now, UpdatedAt: now}
}

func ReconstituteUserOption(id vo.Id, name string, value *string, createdAt, updatedAt time.Time) UserOption {
	return UserOption{ID: id, Name: name, Value: value, CreatedAt: createdAt, UpdatedAt: updatedAt}
}

// setValue mutates the option in place, bumping UpdatedAt only on a real change.
func (o *UserOption) setValue(value *string, now time.Time) {
	if equalStrPtr(o.Value, value) {
		return
	}
	o.Value = value
	o.UpdatedAt = now
}

// Header is a lightweight read projection of a user's public display fields
// (id, name, avatar) — no options, no credentials. Owner/author embeds use it so
// they need only a single user-row query rather than hydrating the full aggregate.
type Header struct {
	ID        string
	Name      string
	AvatarURL string
}

// User is the user aggregate root. Strings that are encrypted at rest (Email)
// or hashed (Password, Identifier) are stored opaquely here; the service layer
// applies/reverses the crypto. The aggregate owns its Options. Fields are
// exported for direct read access; all writes after construction go through the
// mutators.
type User struct {
	ID         vo.Id
	Identifier string // md5(lower(email)+salt) — the auth lookup key
	Email      string // AES-encrypted ciphertext (opaque here)
	Name       string
	AvatarURL  string
	Password   string // sha512, 500 iterations, base64-encoded (see CLAUDE.md)
	Salt       string // sha1(random) hex, 40 chars (unused by argon2id hashes)
	Algorithm  string // which scheme hashed Password: AlgorithmSHA512 | AlgorithmArgon2id
	IsActive   bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Options    []UserOption
}

// NewUser constructs a freshly-registered user. The caller (the service) has
// already computed identifier, encrypted email, avatar URL, password hash and
// salt. Options are seeded separately via SeedDefaultOptions.
func NewUser(id vo.Id, identifier, encryptedEmail, name, avatarURL, passwordHash, salt string, now time.Time) *User {
	return &User{
		ID:         id,
		Identifier: identifier,
		Email:      encryptedEmail,
		Name:       name,
		AvatarURL:  avatarURL,
		Password:   passwordHash,
		Salt:       salt,
		Algorithm:  AlgorithmSHA512,
		IsActive:   true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// Option returns the option with the given name, or nil if absent.
func (u *User) Option(name string) *UserOption {
	for i := range u.Options {
		if u.Options[i].Name == name {
			return &u.Options[i]
		}
	}
	return nil
}

// CurrencyCode returns the user's currency option value, falling back to the
// default.
func (u *User) CurrencyCode() string {
	if o := u.Option(OptionCurrency); o != nil && o.Value != nil && *o.Value != "" {
		return *o.Value
	}
	return DefaultCurrency
}

// ReportPeriod returns the user's report_period option value or the default.
func (u *User) ReportPeriod() string {
	if o := u.Option(OptionReportPeriod); o != nil && o.Value != nil && *o.Value != "" {
		return *o.Value
	}
	return DefaultReportPeriod
}

func (u *User) UpdateName(name string, now time.Time) {
	u.Name = name
	u.UpdatedAt = now
}

// Activate marks the account active, bumping UpdatedAt only when the state
// actually changes so a no-op activate leaves the row untouched.
func (u *User) Activate(now time.Time) {
	if u.IsActive {
		return
	}
	u.IsActive = true
	u.UpdatedAt = now
}

// Deactivate marks the account inactive, bumping UpdatedAt only on a real state
// change.
func (u *User) Deactivate(now time.Time) {
	if !u.IsActive {
		return
	}
	u.IsActive = false
	u.UpdatedAt = now
}

// UpdatePassword replaces the stored password hash and records which algorithm
// produced it. The caller hashes the plaintext first.
func (u *User) UpdatePassword(passwordHash, algorithm string, now time.Time) {
	u.Password = passwordHash
	u.Algorithm = algorithm
	u.UpdatedAt = now
}

// UpdateEmail replaces the encrypted email, identifier and avatar URL together,
// all derived by the service.
func (u *User) UpdateEmail(identifier, encryptedEmail, avatarURL string, now time.Time) {
	u.Identifier = identifier
	u.Email = encryptedEmail
	u.AvatarURL = avatarURL
	u.UpdatedAt = now
}

// UpdateCurrency sets the currency option value.
func (u *User) UpdateCurrency(code string, now time.Time) {
	if o := u.Option(OptionCurrency); o != nil {
		v := code
		o.setValue(&v, now)
	}
}

func (u *User) UpdateReportPeriod(period string, now time.Time) {
	if o := u.Option(OptionReportPeriod); o != nil {
		v := period
		o.setValue(&v, now)
	}
}

// UpdateBudget sets the user's active budget option.
func (u *User) UpdateBudget(budgetID string, now time.Time) {
	if o := u.Option(OptionBudget); o != nil {
		v := budgetID
		o.setValue(&v, now)
	}
}

// CompleteOnboarding sets the onboarding option to completed.
func (u *User) CompleteOnboarding(now time.Time) {
	if o := u.Option(OptionOnboarding); o != nil {
		v := OnboardingCompleted
		o.setValue(&v, now)
	}
}

// SeedDefaultOptions populates the four persisted default options for a new
// user, in the registration seed order and with their default values. nextID
// supplies a fresh Id per option (the repo's NextIdentity).
func (u *User) SeedDefaultOptions(nextID func() vo.Id, now time.Time) {
	defaults := map[string]*string{
		OptionCurrency:     strPtr(DefaultCurrency),
		OptionReportPeriod: strPtr(DefaultReportPeriod),
		OptionBudget:       nil,
		OptionOnboarding:   strPtr(OnboardingStarted),
	}
	u.Options = u.Options[:0]
	for _, name := range persistedOptions {
		u.Options = append(u.Options, NewUserOption(nextID(), name, defaults[name], now))
	}
}

func strPtr(s string) *string { return &s }

func equalStrPtr(a, b *string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}
