// Package api wires the system module's HTTP edge.
package api

import (
	appsystem "github.com/econumo/econumo/internal/system"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appsystem.Service
}

func NewHandlers(svc *appsystem.Service) *Handlers {
	return &Handlers{svc: svc}
}
