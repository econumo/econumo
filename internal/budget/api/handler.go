package api

import (
	appbudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/web/apidoc"
)

var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc *appbudget.Service
}

func NewHandlers(svc *appbudget.Service) *Handlers {
	return &Handlers{svc: svc}
}
