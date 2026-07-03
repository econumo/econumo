// Package api wires the currency module's HTTP edge.
package api

import (
	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/ui/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	read *appcurrency.ReadService
	dev  bool
}

func NewHandlers(read *appcurrency.ReadService, dev bool) *Handlers {
	return &Handlers{read: read, dev: dev}
}
