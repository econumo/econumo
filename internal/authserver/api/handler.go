// Package api wires the authorization server's authenticated HTTP edge: the
// consent page's endpoints and the connected-apps list.
package api

import (
	"github.com/econumo/econumo/internal/authserver"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// _ keeps the apidoc import alias visible to swag's annotation parser.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *authserver.Service
}

func NewHandlers(svc *authserver.Service) *Handlers {
	return &Handlers{svc: svc}
}
