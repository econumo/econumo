// Package api wires the oauth module's HTTP edge.
package api

import appoauth "github.com/econumo/econumo/internal/oauth"

type Handlers struct {
	svc *appoauth.Service
}

func NewHandlers(svc *appoauth.Service) *Handlers { return &Handlers{svc: svc} }
