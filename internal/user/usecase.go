// Service wiring: the use-case orchestrator, its dependency seams, the
// constructor, and the shared private helpers. The individual use cases live in
// sibling files (login.go, register.go, profile.go, password.go, onboarding.go).
package user

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/auth"
	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/infra/mailer"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Service is the user use-case orchestrator. It owns the tx boundary and builds
// the response-shaped *Result structs directly.
type Service struct {
	repo              Repository
	tx                port.TxRunner
	encode            *auth.EncodeService
	hasher            *auth.PasswordHasher
	tokens            AccessTokens
	currency          CurrencyLookup
	budgets           BudgetAccess
	passwordRequests  PasswordRequests
	mailer            *mailer.ResetSender
	avatars           AvatarPicker
	clock             port.Clock
	limiter           AttemptLimiter
	allowRegistration bool
	trial             string
}

func NewService(
	repo Repository,
	tx port.TxRunner,
	encode *auth.EncodeService,
	hasher *auth.PasswordHasher,
	tokens AccessTokens,
	currency CurrencyLookup,
	budgets BudgetAccess,
	passwordRequests PasswordRequests,
	mailer *mailer.ResetSender,
	avatars AvatarPicker,
	clock port.Clock,
	limiter AttemptLimiter,
	allowRegistration bool,
	trial string,
) *Service {
	return &Service{
		repo:              repo,
		tx:                tx,
		encode:            encode,
		hasher:            hasher,
		tokens:            tokens,
		currency:          currency,
		budgets:           budgets,
		passwordRequests:  passwordRequests,
		mailer:            mailer,
		avatars:           avatars,
		clock:             clock,
		limiter:           limiter,
		allowRegistration: allowRegistration,
		trial:             trial,
	}
}

// Logout revokes the presenting session. The "test" literal is a frozen wire
// constant clients depend on (see LogoutResult).
func (s *Service) Logout(ctx context.Context, tokenID vo.Id) (*model.LogoutResult, error) {
	t, err := s.tokens.GetByID(ctx, tokenID)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			// Already gone: logout is idempotent.
			return &model.LogoutResult{Result: "test"}, nil
		}
		return nil, err
	}
	t.Revoke(s.clock.Now())
	if err := s.tokens.Update(ctx, t); err != nil {
		return nil, err
	}
	return &model.LogoutResult{Result: "test"}, nil
}

// mutate loads the user, applies fn inside a transaction, and saves. It returns
// the mutated (in-memory) aggregate so the caller can build its result without
// a second read — the saved state and the in-memory state are identical.
func (s *Service) mutate(ctx context.Context, userID vo.Id, fn func(u *model.User, now time.Time) error) (*model.User, error) {
	var loaded *model.User
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

func (s *Service) toCurrentUser(ctx context.Context, u *model.User) (model.CurrentUserResult, error) {
	email, err := s.encode.Decode(u.Email)
	if err != nil {
		return model.CurrentUserResult{}, err
	}
	return s.toCurrentUserWithEmail(ctx, u, email)
}

// toCurrentUserWithEmail builds the result given an already-decoded email. This
// is the one entity->DTO conversion in the module; the only non-trivial part is
// resolving the synthetic currency_id, which is a real lookup and therefore
// lives here in the service rather than in a mapping helper.
func (s *Service) toCurrentUserWithEmail(ctx context.Context, u *model.User, email string) (model.CurrentUserResult, error) {
	options := make([]model.OptionResult, 0, len(u.Options)+1)
	for _, o := range u.Options {
		options = append(options, model.OptionResult{Name: o.Name, Value: o.Value})
	}

	// Resolve currency_id from the currency code, falling back to USD when the
	// code can't be resolved.
	code := u.CurrencyCode()
	currencyID, err := s.currency.GetIDByCode(ctx, code)
	if err != nil {
		code = s.currency.DefaultCode()
		currencyID, err = s.currency.GetIDByCode(ctx, code)
		if err != nil {
			return model.CurrentUserResult{}, err
		}
	}
	cid := currencyID
	options = append(options, model.OptionResult{Name: model.OptionCurrencyID, Value: &cid})

	return model.CurrentUserResult{
		Id:           u.ID.String(),
		Name:         u.Name,
		Email:        email,
		Avatar:       u.Avatar,
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
			errs.FieldError{Key: "currency", Message: "CurrencyCode is incorrect", Code: errs.CodeInvalidCurrencyCode})
	}
	return c, nil
}

// newReportPeriod enforces the report-period invariant: only "monthly" is valid.
func newReportPeriod(v string) (string, error) {
	if v != model.DefaultReportPeriod {
		return "", errs.NewValidation("ReportPeriod is incorrect",
			errs.FieldError{Key: "value", Message: "ReportPeriod is incorrect", Code: errs.CodeUserReportPeriodInvalid})
	}
	return v, nil
}

// newLanguage enforces the language invariant: must be a member of
// i18n.Supported.
func newLanguage(v string) (string, error) {
	for _, lang := range i18n.Supported {
		if v == lang {
			return v, nil
		}
	}
	return "", errs.NewValidation("Language is incorrect",
		errs.FieldError{Key: "language", Message: "Language is incorrect", Code: errs.CodeUserLanguageInvalid})
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

// allowAttempt / failAttempt / clearAttempt guard the optional limiter (nil in
// the CLI and most tests), mirroring the nil-mailer pattern in RemindPassword.
func (s *Service) allowAttempt(scope, key string) error {
	if s.limiter == nil {
		return nil
	}
	return s.limiter.Allow(scope, key)
}

func (s *Service) failAttempt(scope, key string) {
	if s.limiter != nil {
		s.limiter.Fail(scope, key)
	}
}

func (s *Service) clearAttempt(scope, key string) {
	if s.limiter != nil {
		s.limiter.Clear(scope, key)
	}
}
