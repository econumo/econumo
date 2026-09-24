// Package api wires the imports module's HTTP edge.
package api

import (
	appimports "github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appimports.Service
}

func NewHandlers(svc *appimports.Service) *Handlers {
	return &Handlers{svc: svc}
}
