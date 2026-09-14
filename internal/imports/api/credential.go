package api

import (
	"net/http"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/web/apidoc"
	"github.com/econumo/econumo/internal/web/endpoint"
)

var _ = apidoc.JsonResponseError{}
var _ = model.ClaimSetupTokenResult{}

// ClaimSetupToken handles POST /api/v1/import/claim-setup-token (auth).
//
// @Summary     Claim a SimpleFIN setup token
// @Description Exchanges a one-time SimpleFIN setup token for the bridge access URL. The URL is returned to the client and never stored server-side; the client encrypts it with its credential key and sends the ciphertext to create-source. 403 when the bridge rejects the token (already claimed or expired).
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ClaimSetupTokenRequest true "Setup token"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ClaimSetupTokenResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     403     {object} apidoc.JsonResponseError
// @Failure     429     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/claim-setup-token [post]
func (h *Handlers) ClaimSetupToken(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ClaimSetupToken)
}

// GetCredentialKey handles GET /api/v1/import/get-credential-key (auth).
//
// @Summary     Get the wrapped import credential key
// @Description Returns the caller's passphrase-wrapped data key and KDF parameters. The server cannot unwrap it. 400 (coded not-found) when no key has been set.
// @Tags        Import
// @Produce     json
// @Success     200 {object} apidoc.JsonResponseOk{data=model.GetImportCredentialKeyResult}
// @Failure     400 {object} apidoc.JsonResponseError
// @Failure     401 {object} apidoc.JsonResponseUnauthorized
// @Failure     500 {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/get-credential-key [get]
func (h *Handlers) GetCredentialKey(w http.ResponseWriter, r *http.Request) {
	endpoint.HandleNoBody(w, r, h.svc.GetCredentialKey)
}

// SetCredentialKey handles POST /api/v1/import/set-credential-key (auth).
//
// @Summary     Set the wrapped import credential key
// @Description Stores (or replaces) the caller's passphrase-wrapped data key and KDF parameters. Replacing the key does not touch stored credential ciphertexts; the client re-encrypts and re-submits them via create-source.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.SetImportCredentialKeyRequest true "Wrapped key"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.GetImportCredentialKeyResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/set-credential-key [post]
func (h *Handlers) SetCredentialKey(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.SetCredentialKey)
}

// ListExternalAccounts handles POST /api/v1/import/list-external-accounts (auth).
//
// @Summary     List the accounts a pull source exposes
// @Description Asks the provider (with the client-decrypted access URL in the body) for its accounts and joins each with the caller's mapping state. POST because the access URL must not travel in a query string.
// @Tags        Import
// @Accept      json
// @Produce     json
// @Param       request body     model.ListExternalAccountsRequest true "Source + access URL"
// @Success     200     {object} apidoc.JsonResponseOk{data=model.ListExternalAccountsResult}
// @Failure     400     {object} apidoc.JsonResponseError
// @Failure     401     {object} apidoc.JsonResponseUnauthorized
// @Failure     402     {object} apidoc.JsonResponseError
// @Failure     500     {object} apidoc.JsonResponseException
// @Security    Bearer
// @Router      /api/v1/import/list-external-accounts [post]
func (h *Handlers) ListExternalAccounts(w http.ResponseWriter, r *http.Request) {
	endpoint.Handle(w, r, h.svc.ListExternalAccounts)
}
