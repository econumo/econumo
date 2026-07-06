// Package api wires the tag module's HTTP edge.
package api

import (
	apptag "github.com/econumo/econumo/internal/tag"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc  *apptag.Service
	read *apptag.ReadService
	dev  bool
}

func NewHandlers(svc *apptag.Service, read *apptag.ReadService, dev bool) *Handlers {
	return &Handlers{svc: svc, read: read, dev: dev}
}
