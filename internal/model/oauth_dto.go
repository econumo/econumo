package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

type ProviderItem struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type StartOAuthRequest struct {
	Provider string `json:"provider"`
	Client   string `json:"client"`
}

func (r StartOAuthRequest) Validate() error {
	var fields []errs.FieldError
	if !IsOAuthProvider(r.Provider) {
		fields = append(fields, errs.FieldError{Key: "provider", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	if r.Client != OAuthClientWeb && r.Client != OAuthClientApp {
		fields = append(fields, errs.FieldError{Key: "client", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type StartOAuthResult struct {
	Url string `json:"url"`
}

type ExchangeHandoffRequest struct {
	Code string `json:"code"`
}

func (r ExchangeHandoffRequest) Validate() error {
	if strings.TrimSpace(r.Code) == "" {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "code", Message: "This value should not be blank.", Code: errs.CodeIsBlank})
	}
	return nil
}

type IdentityItem struct {
	Provider  string `json:"provider"`
	Email     string `json:"email"`
	CreatedAt string `json:"createdAt"`
}

type UnlinkIdentityRequest struct {
	Provider string `json:"provider"`
}

func (r UnlinkIdentityRequest) Validate() error {
	if !IsOAuthProvider(r.Provider) {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "provider", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	return nil
}

type UnlinkIdentityResult struct{}
