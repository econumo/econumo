// Package api wires the transaction module's HTTP edge.
package api

import (
	apptransaction "github.com/econumo/econumo/internal/transaction"
	"github.com/econumo/econumo/internal/web/apidoc"
)

var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *apptransaction.Service
	dev bool
}

func NewHandlers(svc *apptransaction.Service, dev bool) *Handlers {
	return &Handlers{svc: svc, dev: dev}
}
