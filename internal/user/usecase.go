// Service wiring: the use-case orchestrator, its dependency seams, the
// constructor, and the shared private helpers. The individual use cases live in
// sibling files (login.go, register.go, profile.go, password.go, onboarding.go).
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

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/jwt"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// PasswordRequests persists password-reset codes (users_password_requests) for
// the remind/reset flow. The infra passwordrequestrepo implements it.
type PasswordRequests interface {
	// DeleteByUser removes all of a user's pending reset codes.
	DeleteByUser(ctx context.Context, userID vo.Id) error
	// Save inserts a new reset request.
	Save(ctx context.Context, pr *PasswordRequest) error
	// GetByUserAndCode loads a user's request matching code (NotFound if absent).
	GetByUserAndCode(ctx context.Context, userID vo.Id, code string) (*PasswordRequest, error)
	// Delete removes a request by id.
	Delete(ctx context.Context, id vo.Id) error
}

// Service is the user use-case orchestrator. It owns the tx boundary and builds
// the response-shaped *Result structs directly.
type Service struct {
	repo              Repository
	tx                port.TxRunner
	encode            *auth.EncodeService
	hasher            *auth.PasswordHasher
	jwt               *jwt.JWT
	currency          CurrencyLookup
	budgets           BudgetExistence
	passwordRequests  PasswordRequests
	mailer            *mailer.ResetSender
	clock             port.Clock
	allowRegistration bool
}

func NewService(
	repo Repository,
	tx port.TxRunner,
	encode *auth.EncodeService,
	hasher *auth.PasswordHasher,
	jwtSvc *jwt.JWT,
	currency CurrencyLookup,
	budgets BudgetExistence,
	passwordRequests PasswordRequests,
	mailer *mailer.ResetSender,
	clock port.Clock,
	allowRegistration bool,
) *Service {
	return &Service{
		repo:              repo,
		tx:                tx,
		encode:            encode,
		hasher:            hasher,
		jwt:               jwtSvc,
		currency:          currency,
		budgets:           budgets,
		passwordRequests:  passwordRequests,
		mailer:            mailer,
		clock:             clock,
		allowRegistration: allowRegistration,
	}
}

// Logout is stateless (JWT); nothing to invalidate server-side.
func (s *Service) Logout(ctx context.Context) (*LogoutResult, error) {
	_ = ctx
	// The "test" literal is a frozen wire constant clients depend on (see LogoutResult).
	return &LogoutResult{Result: "test"}, nil
}

// mutate loads the user, applies fn inside a transaction, and saves. It returns
// the mutated (in-memory) aggregate so the caller can build its result without
// a second read — the saved state and the in-memory state are identical.
func (s *Service) mutate(ctx context.Context, userID vo.Id, fn func(u *User, now time.Time) error) (*User, error) {
	var loaded *User
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

func (s *Service) toCurrentUser(ctx context.Context, u *User) (CurrentUserResult, error) {
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
func (s *Service) toCurrentUserWithEmail(ctx context.Context, u *User, email string) (CurrentUserResult, error) {
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
	options = append(options, OptionResult{Name: OptionCurrencyID, Value: &cid})

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
	if v != DefaultReportPeriod {
		return "", errs.NewValidation("ReportPeriod is incorrect",
			errs.FieldError{Key: "value", Message: "ReportPeriod is incorrect"})
	}
	return v, nil
}

// newSalt generates a salt as sha1(10 random bytes) -> 40 hex chars. See
// CLAUDE.md.
func newSalt() (string, error) {
	b := make([]byte, 10)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:]), nil
}

// md5Hex returns hex(md5(v)) — the gravatar hash: the plain md5 of the
// lowercased email. See CLAUDE.md.
func md5Hex(v string) string {
	sum := md5.Sum([]byte(v))
	return hex.EncodeToString(sum[:])
}
