package api

import (
	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// Forces the apidoc import so swag annotations can resolve its envelope schemas.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appaccount.Service
}

func NewHandlers(svc *appaccount.Service) *Handlers {
	return &Handlers{svc: svc}
}
