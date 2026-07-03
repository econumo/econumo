package api

import (
	appcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/web/apidoc"
)

// Forces the apidoc import so swag annotations can resolve its envelope schemas.
var _ = apidoc.JsonResponseOk{}

type Handlers struct {
	svc  *appcategory.Service
	read *appcategory.ReadService
	dev  bool
}

func NewHandlers(svc *appcategory.Service, read *appcategory.ReadService, dev bool) *Handlers {
	return &Handlers{svc: svc, read: read, dev: dev}
}
