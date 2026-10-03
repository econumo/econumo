package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type ClientRegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type ClientRegistrationResult struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientSecretExpiresAt   *int64   `json:"client_secret_expires_at,omitempty"`
}

// AuthorizationRequest is the consent page's view of /oauth/authorize's query
// (the SPA forwards the OAuth parameters under these camelCase names).
type AuthorizationRequest struct {
	ClientID            string `json:"clientId"`
	RedirectURI         string `json:"redirectUri"`
	ResponseType        string `json:"responseType"`
	CodeChallenge       string `json:"codeChallenge"`
	CodeChallengeMethod string `json:"codeChallengeMethod"`
	Resource            string `json:"resource"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
}

// Validate is a no-op: protocol validation lives in the service, where a bad
// request must be told apart as redirectable or not.
func (r AuthorizationRequest) Validate() error { return nil }

type AuthorizationRequestResult struct {
	ClientName       string `json:"clientName"`
	RedirectHost     string `json:"redirectHost"`
	IsLoopback       bool   `json:"isLoopback"`
	ErrorRedirectURL string `json:"errorRedirectUrl"` // non-empty: navigate there instead of showing consent
}

type AuthorizationDecisionResult struct {
	RedirectURL string `json:"redirectUrl"`
}

type TokenRequest struct {
	GrantType, Code, RedirectURI, CodeVerifier, RefreshToken, Resource, Scope, ClientID, ClientSecret string
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type ConnectedAppResult struct {
	ID           string `json:"id"`
	ClientName   string `json:"clientName"`
	RedirectHost string `json:"redirectHost"`
	IsLoopback   bool   `json:"isLoopback"`
	CreatedAt    string `json:"createdAt"`
	LastUsedAt   string `json:"lastUsedAt"`
}

type RevokeConnectedAppRequest struct {
	ID string `json:"id"`
}

func (r RevokeConnectedAppRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "id", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	if _, err := vo.ParseId(r.ID); err != nil {
		return errs.NewValidation("Validation failed",
			errs.FieldError{Key: "id", Message: "This value is not a valid UUID.", Code: errs.CodeInvalidUUID})
	}
	return nil
}

// RevokeConnectedAppResult is the revoke-connected-app response (empty object).
type RevokeConnectedAppResult struct{}
