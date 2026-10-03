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

// redirectErr marks a bad request whose redirect target is verified, so the
// error goes back to the client; any other validation error must be shown to
// the user and never redirected.
type redirectErr struct{ code, desc string }

func clientNotFound() error {
	return &errs.ValidationError{Msg: "This app is not registered. Start the connection again from the app.", MsgCode: errs.CodeAuthServerClientNotFound}
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
		return c, &redirectErr{"unsupported_response_type", "response_type must be code"}, nil
	case req.CodeChallenge == "" || req.CodeChallengeMethod != "S256":
		return c, &redirectErr{"invalid_request", "PKCE with S256 is required"}, nil
	case req.Scope != "" && req.Scope != Scope:
		return c, &redirectErr{"invalid_scope", "the only scope is mcp"}, nil
	case !s.resourceOK(req.Resource):
		return c, &redirectErr{"invalid_target", "unknown resource"}, nil
	}
	return c, nil, nil
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

func (s *Service) errorRedirect(req model.AuthorizationRequest, code, desc string) string {
	p := url.Values{"error": {code}}
	if desc != "" {
		p.Set("error_description", desc)
	}
	if req.State != "" {
		p.Set("state", req.State)
	}
	return s.redirectWith(req.RedirectURI, p)
}

func (s *Service) DescribeAuthorization(ctx context.Context, userID vo.Id, req model.AuthorizationRequest) (model.AuthorizationRequestResult, error) {
	c, re, err := s.validate(ctx, req)
	if err != nil {
		return model.AuthorizationRequestResult{}, err
	}
	if re != nil {
		return model.AuthorizationRequestResult{ErrorRedirectURL: s.errorRedirect(req, re.code, re.desc)}, nil
	}
	host, loopback := RedirectHost(req.RedirectURI)
	return model.AuthorizationRequestResult{ClientName: c.Name, RedirectHost: host, IsLoopback: loopback}, nil
}

func (s *Service) ApproveAuthorization(ctx context.Context, userID vo.Id, req model.AuthorizationRequest) (model.AuthorizationDecisionResult, error) {
	c, re, err := s.validate(ctx, req)
	if err != nil {
		return model.AuthorizationDecisionResult{}, err
	}
	if re != nil {
		return model.AuthorizationDecisionResult{RedirectURL: s.errorRedirect(req, re.code, re.desc)}, nil
	}
	gen, err := s.creds.CredentialsGeneration(ctx, userID)
	if err != nil {
		return model.AuthorizationDecisionResult{}, err
	}
	raw, hash, err := newSecret()
	if err != nil {
		return model.AuthorizationDecisionResult{}, err
	}
	now := s.clock.Now().UTC()
	code := &model.OAuthAuthorizationCode{
		CodeHash: hash, ClientID: c.ID, UserID: userID,
		RedirectURI: req.RedirectURI, CodeChallenge: req.CodeChallenge, Resource: s.ResourceURL(),
		CredentialsGeneration: gen, CreatedAt: now, ExpiresAt: now.Add(CodeTTL),
	}
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
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
		return model.AuthorizationDecisionResult{RedirectURL: s.errorRedirect(req, re.code, re.desc)}, nil
	}
	return model.AuthorizationDecisionResult{RedirectURL: s.errorRedirect(req, "access_denied", "")}, nil
}
