// Package api wires the payee module's HTTP edge.
package api

import (
	apppayee "github.com/econumo/econumo/internal/payee"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc  *apppayee.Service
	read *apppayee.ReadService
	dev  bool
}

func NewHandlers(svc *apppayee.Service, read *apppayee.ReadService, dev bool) *Handlers {
	return &Handlers{svc: svc, read: read, dev: dev}
}
