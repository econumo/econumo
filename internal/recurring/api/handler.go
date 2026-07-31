// Package api wires the recurring module's HTTP edge.
package api

import (
	apprecurring "github.com/econumo/econumo/internal/recurring"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *apprecurring.Service
}

func NewHandlers(svc *apprecurring.Service) *Handlers {
	return &Handlers{svc: svc}
}
