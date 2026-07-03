package api

import (
	appconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/ui/apidoc"
)

// Forces the apidoc import so swag annotations can resolve its envelope schemas.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appconnection.Service
	dev bool
}

func NewHandlers(svc *appconnection.Service, dev bool) *Handlers {
	return &Handlers{svc: svc, dev: dev}
}
