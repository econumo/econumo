package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the user aggregate's persistence port; the application service
// depends only on this interface. Methods that look up a missing user return an
// *errs.NotFoundError so the HTTP layer maps it consistently. Save upserts the
// user row and all of its option rows in one call.
type Repository interface {
	// NextIdentity allocates a fresh aggregate id (no DB round-trip).
	NextIdentity() vo.Id

	// GetByID loads a user (with options) by id. Missing -> *errs.NotFoundError.
	GetByID(ctx context.Context, id vo.Id) (*model.User, error)

	// GetByEmail loads a user (with options) by email, case-insensitively.
	// Missing -> *errs.NotFoundError.
	GetByEmail(ctx context.Context, email string) (*model.User, error)

	// ExistsByEmail reports whether a user with that email exists
	// (case-insensitive). Used by registration/change-email dup-checks.
	ExistsByEmail(ctx context.Context, email string) (bool, error)

	// Save upserts the user row and its options.
	Save(ctx context.Context, u *model.User) error

	// UpdateLanguage persists the user's last selected UI language. Kept out of
	// Save/UpsertUser so profile mutations cannot clobber it.
	UpdateLanguage(ctx context.Context, id vo.Id, language string) error

	// ListIDs returns all user ids (for the optional connect-users flow on
	// registration).
	ListIDs(ctx context.Context) ([]vo.Id, error)

	// GetOptions loads just the option rows for a user (used by get-option-list,
	// which does not need the full aggregate).
	GetOptions(ctx context.Context, userID vo.Id) ([]model.UserOption, error)

	// GetTimezone loads the caller-observed IANA timezone (empty string if never
	// set). Missing user -> *errs.NotFoundError.
	GetTimezone(ctx context.Context, id vo.Id) (string, error)

	// UpdateTimezone persists the caller-observed IANA timezone.
	UpdateTimezone(ctx context.Context, id vo.Id, tz string) error

	// GetLanguage loads the user's last selected UI language (empty string if
	// never set). Missing user -> *errs.NotFoundError.
	GetLanguage(ctx context.Context, id vo.Id) (string, error)
}

// AccessTokens persists opaque bearer credentials (sessions + PATs). Liveness
// is evaluated in the domain (AccessToken.IsLive), not in SQL. Lookups on a
// missing row return *errs.NotFoundError.
type AccessTokens interface {
	Insert(ctx context.Context, t *model.AccessToken) error

	// GetByHash resolves the sha256 hex of a presented bearer token — the hot
	// path behind every authenticated request — joining the owning user's
	// stored access level and expiry in the same round trip so Authenticate
	// can report the caller's effective access level without a second query;
	// unlike the is_active deactivation shortcut (see admin.go), an expired
	// trial must NOT revoke sessions, so this join cannot be skipped the way
	// the is_active one is.
	GetByHash(ctx context.Context, hash string) (*model.AccessToken, model.AccessLevel, *time.Time, error)

	// GetByID loads one row (logout / revoke-by-id paths).
	GetByID(ctx context.Context, id vo.Id) (*model.AccessToken, error)

	// Update persists the mutable lifecycle fields (last_used_at, expires_at,
	// revoked_at) of an existing row.
	Update(ctx context.Context, t *model.AccessToken) error

	// ListByUser returns ALL rows (live and dead) of one kind, ordered by
	// (created_at, id); callers filter with IsLive/IsDead.
	ListByUser(ctx context.Context, userID vo.Id, kind string) ([]model.AccessToken, error)

	Delete(ctx context.Context, id vo.Id) error

	// DeleteDead removes every row (all users, both kinds) whose expiry or
	// revocation happened before cutoff, returning the number deleted. Backed
	// by the revoked_at/expires_at indexes so it stays cheap on large tables.
	DeleteDead(ctx context.Context, cutoff time.Time) (int64, error)
}

// PasswordRequests persists password-reset codes (users_password_requests) for
// the remind/reset flow. The infra passwordrequestrepo implements it.
type PasswordRequests interface {
	// DeleteByUser removes all of a user's pending reset codes.
	DeleteByUser(ctx context.Context, userID vo.Id) error
	// Save inserts a new reset request.
	Save(ctx context.Context, pr *model.PasswordRequest) error
	// GetByUserAndCode loads a user's request matching code (NotFound if absent).
	GetByUserAndCode(ctx context.Context, userID vo.Id, code string) (*model.PasswordRequest, error)
	// Delete removes a request by id.
	Delete(ctx context.Context, id vo.Id) error
}

// EmailVerifications persists login email-verification codes
// (users_email_verifications) for the ECONUMO_EMAIL_VERIFICATION flow. One
// outstanding row per user; GetByUser on a missing row returns
// *errs.NotFoundError.
type EmailVerifications interface {
	GetByUser(ctx context.Context, userID vo.Id) (*model.EmailVerification, error)
	Save(ctx context.Context, v *model.EmailVerification) error
	DeleteByUser(ctx context.Context, userID vo.Id) error
}
