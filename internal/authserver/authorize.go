package authserver

import (
	"context"
	"net/url"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const maxState = 1024

// redirectErr marks a bad request whose redirect target is verified, so the
// error goes back to the client; any other validation error must be shown to
// the user and never redirected. dropState keeps a rejected state value out of
// the redirect.
type redirectErr struct {
	code, desc string
	dropState  bool
}

func clientNotFound() error {
	return &errs.ValidationError{Msg: "This app's registration has expired or is unknown. Remove Econumo from the app and add it again.", MsgCode: errs.CodeAuthServerClientNotFound}
}

func (s *Service) validate(ctx context.Context, req model.AuthorizationRequest) (*model.OAuthClient, *redirectErr, error) {
	if !s.Enabled() {
		return nil, nil, &errs.ValidationError{Msg: "Connecting apps is not available on this server.", MsgCode: errs.CodeAuthServerDisabled}
	}
	id, err := vo.ParseId(req.ClientID)
	if err != nil {
		return nil, nil, clientNotFound()
	}
	c, err := s.repo.GetClient(ctx, id)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, nil, clientNotFound()
		}
		return nil, nil, err
	}
	if !MatchRedirectURI(c.RedirectURIs, req.RedirectURI) {
		return nil, nil, &errs.ValidationError{Msg: "This sign-in link is not valid for this app. Start the connection again from the app.", MsgCode: errs.CodeAuthServerRedirectMismatch}
	}
	switch {
	case req.ResponseType != "code":
		return c, &redirectErr{code: "unsupported_response_type", desc: "response_type must be code"}, nil
	case req.CodeChallengeMethod != "S256" || !validChallenge(req.CodeChallenge):
		return c, &redirectErr{code: "invalid_request", desc: "PKCE with S256 is required"}, nil
	case len(req.State) > maxState:
		return c, &redirectErr{code: "invalid_request", desc: "state is too long", dropState: true}, nil
	case !s.resourceOK(req.Resource):
		return c, &redirectErr{code: "invalid_target", desc: "unknown resource"}, nil
	}
	return c, nil, nil
}

// An S256 challenge is the unpadded base64url of a SHA-256 digest: 43 characters.
func validChallenge(c string) bool {
	if len(c) != 43 {
		return false
	}
	for i := 0; i < len(c); i++ {
		b := c[i]
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_') {
			return false
		}
	}
	return true
}

func (s *Service) resourceOK(r string) bool {
	return r == "" || strings.TrimRight(r, "/") == s.ResourceURL()
}

func (s *Service) redirectWith(base string, params url.Values) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for k, v := range params {
		q[k] = v
	}
	q.Set("iss", s.issuer)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Service) errorRedirect(req model.AuthorizationRequest, re redirectErr) string {
	p := url.Values{"error": {re.code}}
	if re.desc != "" {
		p.Set("error_description", re.desc)
	}
	if req.State != "" && !re.dropState {
		p.Set("state", req.State)
	}
	return s.redirectWith(req.RedirectURI, p)
}

func (s *Service) DescribeAuthorization(ctx context.Context, userID vo.Id, req model.AuthorizationRequest) (model.AuthorizationRequestResult, error) {
	c, re, err := s.validate(ctx, req)
	if err != nil {
		return model.AuthorizationRequestResult{}, err
	}
	host, loopback := RedirectHost(req.RedirectURI)
	res := model.AuthorizationRequestResult{ClientName: c.Name, RedirectHost: host, IsLoopback: loopback}
	if re != nil {
		res.ErrorRedirectURL = s.errorRedirect(req, *re)
	}
	return res, nil
}

// The generation is read under the user row lock, together with a check that
// the presenting session is still live: a reclaim that committed after the
// auth middleware accepted the session has revoked it, so no code is issued.
func (s *Service) ApproveAuthorization(ctx context.Context, userID, tokenID vo.Id, req model.AuthorizationRequest) (model.AuthorizationDecisionResult, error) {
	c, re, err := s.validate(ctx, req)
	if err != nil {
		return model.AuthorizationDecisionResult{}, err
	}
	if re != nil {
		return model.AuthorizationDecisionResult{RedirectURL: s.errorRedirect(req, *re)}, nil
	}
	raw, hash, err := newSecret()
	if err != nil {
		return model.AuthorizationDecisionResult{}, err
	}
	now := s.clock.Now().UTC()
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		gen, err := s.creds.LockForOAuth(ctx, userID)
		if err != nil {
			return err
		}
		live, err := s.creds.IsTokenLive(ctx, userID, tokenID)
		if err != nil {
			return err
		}
		if !live {
			return errs.NewUnauthorized("Invalid access token")
		}
		code := &model.OAuthAuthorizationCode{
			CodeHash: hash, ClientID: c.ID, UserID: userID,
			RedirectURI: req.RedirectURI, CodeChallenge: req.CodeChallenge, Resource: s.ResourceURL(),
			CredentialsGeneration: gen, CreatedAt: now, ExpiresAt: now.Add(CodeTTL),
		}
		if err := s.repo.InsertCode(ctx, code); err != nil {
			return err
		}
		return s.repo.MarkClientUsed(ctx, c.ID, now)
	})
	if err != nil {
		return model.AuthorizationDecisionResult{}, err
	}
	reqctx.AddLogAttr(ctx, "client_id", c.ID.String())
	p := url.Values{"code": {raw}}
	if req.State != "" {
		p.Set("state", req.State)
	}
	return model.AuthorizationDecisionResult{RedirectURL: s.redirectWith(req.RedirectURI, p)}, nil
}

func (s *Service) DeclineAuthorization(ctx context.Context, userID vo.Id, req model.AuthorizationRequest) (model.AuthorizationDecisionResult, error) {
	_, re, err := s.validate(ctx, req)
	if err != nil {
		return model.AuthorizationDecisionResult{}, err
	}
	if re != nil {
		return model.AuthorizationDecisionResult{RedirectURL: s.errorRedirect(req, *re)}, nil
	}
	return model.AuthorizationDecisionResult{RedirectURL: s.errorRedirect(req, redirectErr{code: "access_denied"})}, nil
}
