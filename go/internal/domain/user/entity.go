// Package user is the user aggregate's domain layer: the User entity, its
// UserOption members, the repository interface, and the option-name constants.
// It is pure — no framework, persistence, JSON, or crypto imports. Crypto
// (password hashing, email encryption, identifier hashing) lives in infra/auth
// and is invoked by the application service, not here.
package user

import (
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
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
// pointer because the column is nullable (e.g. budget defaults to NULL).
type UserOption struct {
	id        vo.Id
	name      string
	value     *string
	createdAt time.Time
	updatedAt time.Time
}

// NewUserOption builds a fresh option (used when seeding registration defaults).
func NewUserOption(id vo.Id, name string, value *string, now time.Time) UserOption {
	return UserOption{id: id, name: name, value: value, createdAt: now, updatedAt: now}
}

// ReconstituteUserOption rebuilds an option from persisted state (repo use only).
func ReconstituteUserOption(id vo.Id, name string, value *string, createdAt, updatedAt time.Time) UserOption {
	return UserOption{id: id, name: name, value: value, createdAt: createdAt, updatedAt: updatedAt}
}

// Id returns the option's identifier.
func (o UserOption) Id() vo.Id { return o.id }

// Name returns the option name (one of the Option* constants).
func (o UserOption) Name() string { return o.name }

// Value returns the option value, which may be nil (SQL NULL).
func (o UserOption) Value() *string { return o.value }

// CreatedAt returns when the option was first created.
func (o UserOption) CreatedAt() time.Time { return o.createdAt }

// UpdatedAt returns when the option was last changed.
func (o UserOption) UpdatedAt() time.Time { return o.updatedAt }

// setValue mutates the option in place, bumping updatedAt only on a real change.
// now is supplied by the service's clock.
func (o *UserOption) setValue(value *string, now time.Time) {
	if equalStrPtr(o.value, value) {
		return
	}
	o.value = value
	o.updatedAt = now
}

// User is the user aggregate root. Strings that are encrypted at rest (email)
// or hashed (password, identifier) are stored opaquely here; the service layer
// applies/reverses the crypto via infra/auth. The aggregate owns its options
// and exposes intent-revealing mutators rather than raw setters.
type User struct {
	id         vo.Id
	identifier string // md5(lower(email)+salt) — the auth lookup key
	email      string // AES-encrypted ciphertext (opaque here)
	name       string
	avatarURL  string
	password   string // sha512, 500 iterations, base64-encoded (see COMPATIBILITY.md)
	salt       string // sha1(random) hex, 40 chars
	isActive   bool
	createdAt  time.Time
	updatedAt  time.Time
	options    []UserOption
}

// NewUser constructs a freshly-registered user. The caller (the service) has
// already computed identifier, encrypted email, avatar URL, password hash and
// salt via infra/auth. Options are seeded separately via SeedDefaultOptions.
func NewUser(id vo.Id, identifier, encryptedEmail, name, avatarURL, passwordHash, salt string, now time.Time) *User {
	return &User{
		id:         id,
		identifier: identifier,
		email:      encryptedEmail,
		name:       name,
		avatarURL:  avatarURL,
		password:   passwordHash,
		salt:       salt,
		isActive:   true,
		createdAt:  now,
		updatedAt:  now,
	}
}

// FromState rebuilds a User from persisted row data (repo reconstruction). The
// repo loads options separately and passes them in.
func FromState(id vo.Id, identifier, encryptedEmail, name, avatarURL, passwordHash, salt string, isActive bool, createdAt, updatedAt time.Time, options []UserOption) *User {
	return &User{
		id:         id,
		identifier: identifier,
		email:      encryptedEmail,
		name:       name,
		avatarURL:  avatarURL,
		password:   passwordHash,
		salt:       salt,
		isActive:   isActive,
		createdAt:  createdAt,
		updatedAt:  updatedAt,
		options:    options,
	}
}

// Id returns the user id.
func (u *User) Id() vo.Id { return u.id }

// Identifier returns the md5 lookup identifier.
func (u *User) Identifier() string { return u.identifier }

// Email returns the encrypted-at-rest email ciphertext.
func (u *User) Email() string { return u.email }

// Name returns the display name.
func (u *User) Name() string { return u.name }

// AvatarURL returns the gravatar URL.
func (u *User) AvatarURL() string { return u.avatarURL }

// Password returns the stored password hash.
func (u *User) Password() string { return u.password }

// Salt returns the per-user salt used by the password hasher.
func (u *User) Salt() string { return u.salt }

// IsActive reports whether the account is active.
func (u *User) IsActive() bool { return u.isActive }

// CreatedAt returns the creation time.
func (u *User) CreatedAt() time.Time { return u.createdAt }

// UpdatedAt returns the last-modification time.
func (u *User) UpdatedAt() time.Time { return u.updatedAt }

// Options returns the user's options (currency_id is NOT among these — it is
// computed in the result by the service).
func (u *User) Options() []UserOption { return u.options }

// Option returns the option with the given name, or nil if absent.
func (u *User) Option(name string) *UserOption {
	for i := range u.options {
		if u.options[i].name == name {
			return &u.options[i]
		}
	}
	return nil
}

// CurrencyCode returns the user's currency option value, falling back to the
// default.
func (u *User) CurrencyCode() string {
	if o := u.Option(OptionCurrency); o != nil && o.value != nil && *o.value != "" {
		return *o.value
	}
	return DefaultCurrency
}

// ReportPeriod returns the user's report_period option value or the default.
func (u *User) ReportPeriod() string {
	if o := u.Option(OptionReportPeriod); o != nil && o.value != nil && *o.value != "" {
		return *o.value
	}
	return DefaultReportPeriod
}

// UpdateName changes the display name.
func (u *User) UpdateName(name string, now time.Time) {
	u.name = name
	u.updatedAt = now
}

// UpdatePassword replaces the stored password hash. The caller hashes the
// plaintext via infra/auth using this user's salt first.
func (u *User) UpdatePassword(passwordHash string, now time.Time) {
	u.password = passwordHash
	u.updatedAt = now
}

// UpdateEmail replaces the encrypted email, identifier and avatar URL together,
// all derived by the service via infra/auth.
func (u *User) UpdateEmail(identifier, encryptedEmail, avatarURL string, now time.Time) {
	u.identifier = identifier
	u.email = encryptedEmail
	u.avatarURL = avatarURL
	u.updatedAt = now
}

// UpdateCurrency sets the currency option value.
func (u *User) UpdateCurrency(code string, now time.Time) {
	if o := u.Option(OptionCurrency); o != nil {
		v := code
		o.setValue(&v, now)
	}
}

// UpdateReportPeriod sets the report_period option value. NOTE: this
// deliberately writes to the REPORT_PERIOD option. The legacy implementation had
// a long-standing bug that wrote the value onto the CURRENCY option instead; we
// do not replicate it, because the wire result reads report_period back from its
// own option and replicating the bug would corrupt the currency option. See
// integration notes.
func (u *User) UpdateReportPeriod(period string, now time.Time) {
	if o := u.Option(OptionReportPeriod); o != nil {
		v := period
		o.setValue(&v, now)
	}
}

// UpdateBudget sets the user's active budget option (PHP userService.updateBudget).
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
	u.options = u.options[:0]
	for _, name := range persistedOptions {
		u.options = append(u.options, NewUserOption(nextID(), name, defaults[name], now))
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
