// Package api wires the label module's HTTP edge.
package api

import (
	applabel "github.com/econumo/econumo/internal/label"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc  *applabel.Service
	read *applabel.ReadService
}

func NewHandlers(svc *applabel.Service, read *applabel.ReadService) *Handlers {
	return &Handlers{svc: svc, read: read}
}
