package user

import (
	"context"
	"net/url"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

type BillingLinkSigner interface {
	Sign(uid string, now time.Time) (string, error)
}

type BillingService struct {
	baseURL string
	signer  BillingLinkSigner
	clock   port.Clock
}

func NewBillingService(baseURL string, signer BillingLinkSigner, clk port.Clock) *BillingService {
	return &BillingService{baseURL: baseURL, signer: signer, clock: clk}
}

// CreateBillingLink mints a fresh assertion per click rather than carrying one
// on the user payload: the token lives 10 minutes and get-user-data is cached
// by the SPA, so a link built at login would often be stale by the time it is
// followed.
func (s *BillingService) CreateBillingLink(ctx context.Context, userID vo.Id, req model.CreateBillingLinkRequest) (*model.CreateBillingLinkResult, error) {
	if s.baseURL == "" {
		return nil, errs.NewValidation("Billing is not configured")
	}
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, err
	}

	token, err := s.signer.Sign(userID.String(), s.clock.Now())
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("t", token)
	if req.For != "" {
		// for is concatenated into a URL, so a malformed value would inject
		// parameters into the portal link. Authorization is the portal's job.
		forID, perr := vo.ParseId(req.For)
		if perr != nil {
			return nil, errs.NewValidation("Form validation error",
				errs.FieldError{Key: "for", Message: "Invalid user id"})
		}
		q.Set("for", forID.String())
	}
	// The SPA sends Accept-Language on every request, so this is the language
	// the caller is reading right now — fresher than users.language, which is
	// written only at login.
	q.Set("lang", reqctx.Language(ctx))
	u.RawQuery = q.Encode()

	return &model.CreateBillingLinkResult{URL: u.String()}, nil
}
