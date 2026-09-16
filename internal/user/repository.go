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

	// LockRow takes the user row's write lock for the rest of the caller's
	// transaction without changing anything. It is the primitive behind every
	// write to an existing user's row and every credential mint (sessions, PATs,
	// oauth identities): taken FIRST, it orders them against an account reclaim,
	// which holds the same lock while it bumps the generation and sweeps. It is
	// also how a read-then-write over a user's sign-in methods (the oauth
	// identity unlink) serializes: two concurrent unlinks would otherwise both
	// count two identities and both delete, stranding a passwordless account
	// with none.
	//
	// A MISSING user succeeds silently (the no-op UPDATE matches zero rows and
	// returns no error), and two error shapes depend on that: ConfirmEmail's
	// anti-enumeration generic invalid-code, raised by the GetByID that follows,
	// and the mints' fence, which then writes zero rows and yields a 401 rather
	// than a 500. Never "fix" it to error on a missing row.
	LockRow(ctx context.Context, userID vo.Id) error

	// BumpCredentialsGeneration invalidates every flow that read its evidence
	// before this call. Part of the reclaim, inside its transaction.
	BumpCredentialsGeneration(ctx context.Context, userID vo.Id) error

	// UpdatePasswordIfGeneration rewrites only the credential columns, and only
	// while the generation still matches (see login.go rehashLegacyPassword).
	UpdatePasswordIfGeneration(ctx context.Context, userID vo.Id, hash, salt, algorithm string, now time.Time, generation int64) (int64, error)

	// ReplaceEmailIfPasswordless writes the provider's new address onto the
	// primary email (marking it verified) only while the account is still
	// passwordless and still at the given generation, so a password reset
	// committing after the oauth callback's eligibility checks keeps the
	// recovered account's own address. Returns the rows affected.
	ReplaceEmailIfPasswordless(ctx context.Context, userID vo.Id, encryptedEmail string, now time.Time, generation int64) (int64, error)

	// ReplaceEmailIfGeneration writes the confirmed new address onto the
	// primary email (marking it verified) only while the account is still at
	// the given generation — read after LockRow, so the confirm path can never
	// save a stale aggregate over an account a reset has just reclaimed.
	// Returns the rows affected.
	ReplaceEmailIfGeneration(ctx context.Context, userID vo.Id, encryptedEmail string, now time.Time, generation int64) (int64, error)

	// UpsertOption writes a single option row without touching the user row or
	// any other option — the narrow write the analytics-preference backfill
	// needs (Save would rewrite the whole user aggregate per row, which does
	// not scale to a boot-time sweep over every user).
	UpsertOption(ctx context.Context, userID vo.Id, o model.UserOption) error

	// UpdateLanguage persists the user's last selected UI language. Kept out of
	// Save/UpsertUser so profile mutations cannot clobber it.
	UpdateLanguage(ctx context.Context, id vo.Id, language string) error

	// ListIDs returns all user ids (for the optional connect-users flow on
	// registration).
	ListIDs(ctx context.Context) ([]vo.Id, error)

	// ListUserIDsMissingOption returns every user id with no row for the given
	// option name; it exists for the analytics-preference backfill.
	ListUserIDsMissingOption(ctx context.Context, name string) ([]vo.Id, error)

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
	// InsertIfGeneration writes a session row only while the user's credentials
	// generation still matches the one the caller's evidence was read under,
	// reporting the rows written. Zero means an account reclaim landed in
	// between and this session must not exist: the check happens at write time,
	// inside the database, because a Go-side read would be exactly the race it
	// is meant to close.
	InsertIfGeneration(ctx context.Context, t *model.AccessToken, generation int64) (int64, error)

	// InsertIfPresenterLive writes a personal token only while presentingTokenID
	// — the request's authenticated credential — is still unrevoked, reporting
	// the rows written. Used for PATs: unlike a session (evidence is a password
	// check), a PAT is minted mid-session, so the fence is "is the credential
	// that got me here still good", checked at write time inside the database
	// for the same race-closing reason as InsertIfGeneration.
	InsertIfPresenterLive(ctx context.Context, t *model.AccessToken, presentingTokenID vo.Id) (int64, error)

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

	// Touch slides last_used_at/expires_at of a row that is still unrevoked,
	// reporting the rows written. It never writes revoked_at: a request that
	// read the row before a reclaim revoked it must not be able to put the
	// stale NULL back, so the guard lives in the statement, not in Go. Zero
	// rows means the credential was revoked or deleted between the read and
	// this write and the caller must fail closed.
	Touch(ctx context.Context, id vo.Id, lastUsedAt time.Time, expiresAt *time.Time) (int64, error)

	// Revoke stamps revoked_at on one still-unrevoked row, so an earlier
	// revocation keeps its original timestamp and a concurrent touch cannot
	// undo it.
	Revoke(ctx context.Context, id vo.Id, now time.Time) error

	// RevokeAll revokes every still-unrevoked row of one kind for one user
	// except exceptID (the presenting credential; pass the zero id to spare
	// nothing) in a single statement, so a sweep cannot lose rows to a
	// concurrent touch the way a read-then-write loop could.
	RevokeAll(ctx context.Context, userID vo.Id, kind string, exceptID vo.Id, now time.Time) error

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

	// Consume deletes one code by (id, user), reporting the rows deleted. The
	// reset reads that row as its evidence and consumes it under the user's
	// row lock, so zero rows means a replacement code (or a concurrent reset)
	// already took it and this reset must fail closed.
	Consume(ctx context.Context, id, userID vo.Id) (int64, error)
}

// EmailVerifications persists login email-verification codes
// (users_email_verifications) for the ECONUMO_EMAIL_VERIFICATION flow. One
// outstanding row per user; GetByUser on a missing row returns
// *errs.NotFoundError.
type EmailVerifications interface {
	GetByUser(ctx context.Context, userID vo.Id) (*model.EmailVerification, error)
	Save(ctx context.Context, v *model.EmailVerification) error
	DeleteByUser(ctx context.Context, userID vo.Id) error

	// Consume deletes one code by (id, user), reporting the rows deleted. The
	// confirmation reads that row as its evidence and consumes it under the
	// user's row lock, so zero rows means a resend replaced it (or a concurrent
	// confirmation already took it) and this confirmation must fail closed.
	Consume(ctx context.Context, id, userID vo.Id) (int64, error)
}

// EmailChangeRequests persists pending self-service email changes
// (users_email_change_requests). One outstanding row per user; GetByUser on a
// missing row returns *errs.NotFoundError.
type EmailChangeRequests interface {
	GetByUser(ctx context.Context, userID vo.Id) (*model.EmailChangeRequest, error)

	// Save inserts a pending change only while the user's credentials
	// generation still matches the one the password check was read under,
	// reporting the rows written. A pending change is a grant to rewrite the
	// login key, so zero rows means a reclaim landed in between and the grant
	// must not exist (same fence, and the same reason, as
	// AccessTokens.InsertIfGeneration).
	Save(ctx context.Context, r *model.EmailChangeRequest, generation int64) (int64, error)

	// Consume deletes one pending row by (id, user), reporting the rows
	// deleted. The confirm path reads that row as its evidence and consumes it
	// under the user's row lock, so zero rows means the reclaim (or a
	// concurrent confirm) already took the grant and this confirmation must
	// fail closed.
	Consume(ctx context.Context, id, userID vo.Id) (int64, error)

	DeleteByUser(ctx context.Context, userID vo.Id) error
}
