// Package api wires the currency module's HTTP edge.
package api

import (
	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	read *appcurrency.ReadService
}

func NewHandlers(read *appcurrency.ReadService) *Handlers {
	return &Handlers{read: read}
}
