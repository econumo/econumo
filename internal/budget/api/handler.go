package api

import (
	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/ui/apidoc"
)

var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appbudget.Service
	dev bool
}

func NewHandlers(svc *appbudget.Service, dev bool) *Handlers {
	return &Handlers{svc: svc, dev: dev}
}
