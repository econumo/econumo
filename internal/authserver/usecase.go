package authserver

import (
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/shared/port"
)

const (
	Scope                = "mcp"
	CodeTTL              = 2 * time.Minute
	AccessTokenTTL       = time.Hour
	GrantIdleTTL         = 90 * 24 * time.Hour
	RefreshGrace         = 60 * time.Second
	UnusedClientTTL      = 30 * 24 * time.Hour
	DeadRetention        = 30 * 24 * time.Hour
	HousekeepingInterval = 10 * time.Minute
)

type Service struct {
	repo    Repository
	creds   Credentials
	tx      port.TxRunner
	clock   port.Clock
	limiter Limiter
	issuer  string

	hkMu   sync.Mutex
	hkLast time.Time
}

// A nil limiter disables rate limiting. The server is enabled only for an
// https issuer (plain http only on a loopback host), because refresh tokens,
// client secrets and bearer tokens would otherwise cross the network in the
// clear, and only for an origin with no path; any other appURL leaves it
// disabled.
func NewService(repo Repository, creds Credentials, tx port.TxRunner, clock port.Clock, limiter Limiter, appURL string) *Service {
	return &Service{repo: repo, creds: creds, tx: tx, clock: clock, limiter: limiter, issuer: secureIssuer(appURL)}
}

func secureIssuer(appURL string) string {
	u, err := url.Parse(strings.TrimSpace(appURL))
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return ""
	}
	// Discovery, /mcp and every OAuth route are served from the root, so an
	// issuer with a path could not publish its own metadata (RFC 8414).
	if p := u.EscapedPath(); p != "" && p != "/" {
		return ""
	}
	if strings.HasSuffix(u.Host, ":") {
		return ""
	}
	scheme, host := strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	if scheme != "https" && !(scheme == "http" && isLoopbackHost(strings.ToLower(u.Hostname()))) {
		return ""
	}
	return scheme + "://" + host
}

func (s *Service) Enabled() bool       { return s.issuer != "" }
func (s *Service) Issuer() string      { return s.issuer }
func (s *Service) ResourceURL() string { return s.issuer + "/mcp" }
