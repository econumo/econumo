// Package api wires the debt module's HTTP edge.
package api

import (
	appdebt "github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appdebt.Service
}

func NewHandlers(svc *appdebt.Service) *Handlers {
	return &Handlers{svc: svc}
}
