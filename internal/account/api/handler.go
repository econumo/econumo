package api

import (
	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/ui/apidoc"
)

// Forces the apidoc import so swag annotations can resolve its envelope schemas.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appaccount.Service
	dev bool
}

func NewHandlers(svc *appaccount.Service, dev bool) *Handlers {
	return &Handlers{svc: svc, dev: dev}
}
