package api

import (
	appconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// Forces the apidoc import so swag annotations can resolve its envelope schemas.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appconnection.Service
}

func NewHandlers(svc *appconnection.Service) *Handlers {
	return &Handlers{svc: svc}
}
