// Service wiring: the use-case orchestrator, its dependency seams, the
// constructor, and the shared private helpers. The individual use cases live in
// sibling files (login.go, register.go, profile.go, password.go, onboarding.go,
// userdata.go).
package user

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domuser "github.com/econumo/econumo/internal/domain/user"
	"github.com/econumo/econumo/internal/infra/auth"
)

// Clock supplies the current time. A seam (rather than calling time.Now
// directly) so tests can pin timestamps for byte-stable golden output.
type Clock interface {
	Now() time.Time
}

// CurrencyLookup resolves a currency code to its currency-id (the synthetic
// currency_id option in CurrentUserResult). DefaultCode returns the fallback
// used when the user's code can't be resolved. The currency module isn't built
// yet, so this is a minimal seam; userrepo-style sql impl lives in
// infra/repo/currency (see notes — a stub is acceptable until that module lands,
// but the interface must exist because CurrentUserResult depends on it).
type CurrencyLookup interface {
	// GetIDByCode returns the currency uuid for the given code, or an error if
	// not found. The service falls back to DefaultCode on error.
	GetIDByCode(ctx context.Context, code string) (string, error)
	// DefaultCode returns the fallback currency code (USD).
	DefaultCode() string
}

// BudgetExistence is the minimal budget lookup the update-budget use case needs:
// confirm a budget id exists before setting it as the user's default. PHP's
// BudgetService.updateBudget does an existence-only get (no ownership/access
// check) and maps a miss to the "Plan not found" validation error. The full
// budget module owns the table; this is the read-only port the user service
// depends on (mirrors CurrencyLookup).
type BudgetExistence interface {
	// Exists reports whether a budget with the given id exists.
	Exists(ctx context.Context, budgetID string) (bool, error)
}

// TxRunner is the transaction boundary the service owns. backend.TxManager
// satisfies it; defining it here keeps the app layer from importing the
// storage package directly.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// PasswordHasher and Encoder narrow the infra/auth surface the service uses.
type passwordHasher interface {
	Hash(plainPassword, salt string) string
	Verify(hashedPassword, plainPassword, salt string) bool
}

type encoder interface {
	Hash(value string) string
	Encode(value string) (string, error)
	Decode(value string) (string, error)
}

// jwtIssuer issues login tokens.
type jwtIssuer interface {
	Issue(userID, email string, now time.Time) (string, error)
}

// PasswordRequests persists password-reset codes (users_password_requests) for
// the remind/reset flow. The infra passwordrequestrepo implements it.
type PasswordRequests interface {
	// DeleteByUser removes all of a user's pending reset codes.
	DeleteByUser(ctx context.Context, userID vo.Id) error
	// Save inserts a new reset request.
	Save(ctx context.Context, pr *domuser.PasswordRequest) error
	// GetByUserAndCode loads a user's request matching code (NotFound if absent).
	GetByUserAndCode(ctx context.Context, userID vo.Id, code string) (*domuser.PasswordRequest, error)
	// Delete removes a request by id.
	Delete(ctx context.Context, id vo.Id) error
}

// ResetMailer sends the password-reset confirmation-code email. The infra mailer
// implements it; a no-op/unconfigured mailer simply sends nothing.
type ResetMailer interface {
	SendResetPasswordCode(ctx context.Context, toEmail, name, code string) error
}

// Service is the user use-case orchestrator. It owns the tx boundary and builds
// the response-shaped *Result structs directly.
type Service struct {
	repo              domuser.Repository
	tx                TxRunner
	encode            encoder
	hasher            passwordHasher
	jwt               jwtIssuer
	currency          CurrencyLookup
	budgets           BudgetExistence
	passwordRequests  PasswordRequests
	mailer            ResetMailer
	clock             Clock
	allowRegistration bool
	connectUsers      bool
}

// NewService wires the user service.
func NewService(
	repo domuser.Repository,
	tx TxRunner,
	encode *auth.EncodeService,
	hasher *auth.PasswordHasher,
	jwt *auth.JWT,
	currency CurrencyLookup,
	budgets BudgetExistence,
	passwordRequests PasswordRequests,
	mailer ResetMailer,
	clock Clock,
	allowRegistration bool,
	connectUsers bool,
) *Service {
	return &Service{
		repo:              repo,
		tx:                tx,
		encode:            encode,
		hasher:            hasher,
		jwt:               jwt,
		currency:          currency,
		budgets:           budgets,
		passwordRequests:  passwordRequests,
		mailer:            mailer,
		clock:             clock,
		allowRegistration: allowRegistration,
		connectUsers:      connectUsers,
	}
}

// Logout is stateless (JWT); nothing to invalidate server-side.
func (s *Service) Logout(ctx context.Context) (*LogoutResult, error) {
	_ = ctx
	// PHP's assembler hard-codes result = 'test'; replicate it verbatim.
	return &LogoutResult{Result: "test"}, nil
}

// ---------------------------------------------------------------------------
// shared private helpers used across the use cases
// ---------------------------------------------------------------------------

// mutate loads the user, applies fn inside a transaction, and saves. It returns
// the mutated (in-memory) aggregate so the caller can build its result without
// a second read — the saved state and the in-memory state are identical.
func (s *Service) mutate(ctx context.Context, userID vo.Id, fn func(u *domuser.User, now time.Time) error) (*domuser.User, error) {
	var loaded *domuser.User
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		u, err := s.repo.GetByID(ctx, userID)
		if err != nil {
			return err
		}
		if err := fn(u, s.clock.Now()); err != nil {
			return err
		}
		if err := s.repo.Save(ctx, u); err != nil {
			return err
		}
		loaded = u
		return nil
	})
	if err != nil {
		return nil, err
	}
	return loaded, nil
}

// toCurrentUser builds the CurrentUserResult, decoding the email itself.
func (s *Service) toCurrentUser(ctx context.Context, u *domuser.User) (CurrentUserResult, error) {
	email, err := s.encode.Decode(u.Email())
	if err != nil {
		return CurrentUserResult{}, err
	}
	return s.toCurrentUserWithEmail(ctx, u, email)
}

// toCurrentUserWithEmail builds the result given an already-decoded email. This
// is the one entity->DTO conversion in the module; the only non-trivial part is
// resolving the synthetic currency_id, which is a real lookup and therefore
// lives here in the service rather than in a mapping helper.
func (s *Service) toCurrentUserWithEmail(ctx context.Context, u *domuser.User, email string) (CurrentUserResult, error) {
	options := make([]OptionResult, 0, len(u.Options())+1)
	for _, o := range u.Options() {
		options = append(options, OptionResult{Name: o.Name(), Value: o.Value()})
	}

	// Resolve currency_id from the currency code, falling back to USD when the
	// code can't be resolved.
	code := u.CurrencyCode()
	currencyID, err := s.currency.GetIDByCode(ctx, code)
	if err != nil {
		code = s.currency.DefaultCode()
		currencyID, err = s.currency.GetIDByCode(ctx, code)
		if err != nil {
			return CurrentUserResult{}, err
		}
	}
	cid := currencyID
	options = append(options, OptionResult{Name: domuser.OptionCurrencyID, Value: &cid})

	return CurrentUserResult{
		Id:           u.Id().String(),
		Name:         u.Name(),
		Email:        email,
		Avatar:       u.AvatarURL(),
		Options:      options,
		Currency:     code,
		ReportPeriod: u.ReportPeriod(),
	}, nil
}

// ---------------------------------------------------------------------------
// tier-2 value-object constructors (user-module invariants)
// ---------------------------------------------------------------------------

// newCurrencyCode enforces the currency-code invariant: trim, uppercase, must be
// exactly 3 chars. Returns a *ValidationError on failure (HTTP 400).
func newCurrencyCode(v string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(v))
	if len([]rune(c)) != 3 {
		return "", errs.NewValidation("CurrencyCode is incorrect",
			errs.FieldError{Key: "currency", Message: "CurrencyCode is incorrect"})
	}
	return c, nil
}

// newReportPeriod enforces the report-period invariant: only "monthly" is valid.
func newReportPeriod(v string) (string, error) {
	if v != domuser.DefaultReportPeriod {
		return "", errs.NewValidation("ReportPeriod is incorrect",
			errs.FieldError{Key: "value", Message: "ReportPeriod is incorrect"})
	}
	return v, nil
}

// newSalt generates a salt as sha1(10 random bytes) -> 40 hex chars. See
// COMPATIBILITY.md.
func newSalt() (string, error) {
	b := make([]byte, 10)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:]), nil
}

// md5Hex returns hex(md5(v)) — the gravatar hash: the plain md5 of the
// lowercased email. See COMPATIBILITY.md.
func md5Hex(v string) string {
	sum := md5.Sum([]byte(v))
	return hex.EncodeToString(sum[:])
}
