package api

import (
	"context"
	"net/http"

	"github.com/econumo/econumo/internal/model"
	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/web/endpoint"
	"github.com/econumo/econumo/internal/web/httpx"
)

// GetProviderList handles GET /api/v1/oauth/get-provider-list (public).
//
// @Summary     List sign-in providers
// @Description Enabled OAuth/OIDC providers in display order (google, apple, oidc).
// @Tags        OAuth
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=[]model.ProviderItem}
// @Failure     500 {object} apidoc.JsonResponseException
// @Router      /api/v1/oauth/get-provider-list [get]
func (h *Handlers) GetProviderList(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, h.svc.ListProviders())
}

// StartLogin handles POST /api/v1/oauth/start-login (public).
//
// @Summary     Start a provider sign-in
// @Description Creates the authorization request and returns the provider URL to navigate to.
// @Tags        OAuth
// @Accept      json
// @Produce     json
// @Param       request body     model.StartOAuthRequest true "Provider and client"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.StartOAuthResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/oauth/start-login [post]
func (h *Handlers) StartLogin(w http.ResponseWriter, r *http.Request) {
	endpoint.HandlePublic(w, r, h.svc.StartLogin)
}

// StartLink handles POST /api/v1/oauth/start-link (auth).
//
// @Summary     Start linking a provider to the current account
// @Tags        OAuth
// @Accept      json
// @Produce     json
// @Param       request body     model.StartOAuthRequest true "Provider and client"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.StartOAuthResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/oauth/start-link [post]
func (h *Handlers) StartLink(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.StartOAuthRequest) (*model.StartOAuthResult, error) {
		return h.svc.StartLink(ctx, userID, req)
	})
}

// CallbackGoogle handles GET /api/v1/oauth/callback-google (public): the
// provider's redirect. Always answers with a 302 to the SPA or the app scheme.
//
// @Summary     Google callback
// @Tags        OAuth
// @Param       code  query string false "Authorization code"
// @Param       state query string true  "State"
// @Param       error query string false "Provider error"
// @Success     302
// @Router      /api/v1/oauth/callback-google [get]
func (h *Handlers) CallbackGoogle(w http.ResponseWriter, r *http.Request) {
	h.callbackQuery(w, r, model.OAuthProviderGoogle)
}

// CallbackOIDC handles GET /api/v1/oauth/callback-oidc (public).
//
// @Summary     Custom OIDC callback
// @Tags        OAuth
// @Param       code  query string false "Authorization code"
// @Param       state query string true  "State"
// @Param       error query string false "Provider error"
// @Success     302
// @Router      /api/v1/oauth/callback-oidc [get]
func (h *Handlers) CallbackOIDC(w http.ResponseWriter, r *http.Request) {
	h.callbackQuery(w, r, model.OAuthProviderOIDC)
}

// CallbackApple handles POST /api/v1/oauth/callback-apple (public): Apple's
// response_mode=form_post lands here as form fields.
//
// @Summary     Apple callback
// @Tags        OAuth
// @Accept      x-www-form-urlencoded
// @Param       code  formData string false "Authorization code"
// @Param       state formData string true  "State"
// @Param       user  formData string false "Apple's one-time user JSON"
// @Param       error formData string false "Provider error"
// @Success     302
// @Router      /api/v1/oauth/callback-apple [post]
func (h *Handlers) CallbackApple(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, h.svc.Callback(r.Context(), model.OAuthProviderApple, appoauth.CallbackInput{}), http.StatusFound)
		return
	}
	in := appoauth.CallbackInput{Code: r.PostForm.Get("code"), State: r.PostForm.Get("state"),
		Error: r.PostForm.Get("error"), AppleUser: r.PostForm.Get("user")}
	reqctx.AddLogAttr(r.Context(), "provider", model.OAuthProviderApple)
	http.Redirect(w, r, h.svc.Callback(r.Context(), model.OAuthProviderApple, in), http.StatusFound)
}

func (h *Handlers) callbackQuery(w http.ResponseWriter, r *http.Request, provider string) {
	q := r.URL.Query()
	in := appoauth.CallbackInput{Code: q.Get("code"), State: q.Get("state"), Error: q.Get("error")}
	reqctx.AddLogAttr(r.Context(), "provider", provider)
	http.Redirect(w, r, h.svc.Callback(r.Context(), provider, in), http.StatusFound)
}

// ExchangeHandoff handles POST /api/v1/oauth/exchange-handoff (public). Like
// login-user it answers with the raw {token,user} body, not the envelope.
//
// @Summary     Exchange a sign-in handoff for a session
// @Tags        OAuth
// @Accept      json
// @Produce     json
// @Param       request body     model.ExchangeHandoffRequest true "Handoff code"
// @Success     200     {object} model.LoginResult "Raw {token,user} body — NOT wrapped in the standard envelope (same shape as login-user)."
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Router      /api/v1/oauth/exchange-handoff [post]
func (h *Handlers) ExchangeHandoff(w http.ResponseWriter, r *http.Request) {
	var req model.ExchangeHandoffRequest
	if err := httpx.DecodeValidate(r, &req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.ExchangeHandoff(r.Context(), req, r.Header.Get("User-Agent"))
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.Raw(w, res)
}

// GetIdentityList handles GET /api/v1/oauth/get-identity-list (auth).
//
// @Summary     List linked accounts
// @Tags        OAuth
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=[]model.IdentityItem}
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/oauth/get-identity-list [get]
func (h *Handlers) GetIdentityList(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.ListIdentities)
}

// UnlinkIdentity handles POST /api/v1/oauth/unlink-identity (auth).
//
// @Summary     Unlink a provider from the current account
// @Tags        OAuth
// @Accept      json
// @Produce     json
// @Param       request body     model.UnlinkIdentityRequest true "Provider"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.UnlinkIdentityResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/oauth/unlink-identity [post]
func (h *Handlers) UnlinkIdentity(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, func(ctx context.Context, userID vo.Id, req model.UnlinkIdentityRequest) (*model.UnlinkIdentityResult, error) {
		return h.svc.UnlinkIdentity(ctx, userID, req)
	})
}
