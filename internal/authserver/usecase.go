package authserver

import (
	"net/url"
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
	UnusedClientTTL = 30 * 24 * time.Hour
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

// A nil limiter disables rate limiting. The server is enabled only for an
// https issuer (plain http only on a loopback host), because refresh tokens,
// client secrets and bearer tokens would otherwise cross the network in the
// clear; any other appURL leaves it disabled.
func NewService(repo Repository, creds Credentials, tx port.TxRunner, clock port.Clock, limiter Limiter, appURL string) *Service {
	return &Service{repo: repo, creds: creds, tx: tx, clock: clock, limiter: limiter, issuer: secureIssuer(appURL)}
}

func secureIssuer(appURL string) string {
	u, err := url.Parse(strings.TrimSpace(appURL))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(appURL), "/")
}

func (s *Service) Enabled() bool       { return s.issuer != "" }
func (s *Service) Issuer() string      { return s.issuer }
func (s *Service) ResourceURL() string { return s.issuer + "/mcp" }
