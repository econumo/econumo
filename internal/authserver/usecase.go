package authserver

import (
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/port"
)

const (
	Scope           = "mcp"
	CodeTTL         = 2 * time.Minute
	AccessTokenTTL  = time.Hour
	GrantIdleTTL    = 90 * 24 * time.Hour
	RefreshGrace    = 60 * time.Second
	UnusedClientTTL = 24 * time.Hour
	DeadRetention   = 30 * 24 * time.Hour
)

type Service struct {
	repo    Repository
	creds   Credentials
	tx      port.TxRunner
	clock   port.Clock
	limiter Limiter
	issuer  string
}

// A nil limiter disables rate limiting; an empty appURL disables the server.
func NewService(repo Repository, creds Credentials, tx port.TxRunner, clock port.Clock, limiter Limiter, appURL string) *Service {
	return &Service{repo: repo, creds: creds, tx: tx, clock: clock, limiter: limiter, issuer: strings.TrimRight(appURL, "/")}
}

func (s *Service) Enabled() bool       { return s.issuer != "" }
func (s *Service) Issuer() string      { return s.issuer }
func (s *Service) ResourceURL() string { return s.issuer + "/mcp" }
