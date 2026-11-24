package user

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/infra/billing"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// BillingAccessFetcher asks the billing portal for the subscription view of a
// user and of the connections Econumo names (infra/billing.AccessClient).
type BillingAccessFetcher interface {
	GetAccess(ctx context.Context, uid string, connectionIDs []string) (model.BillingAccessUser, []model.BillingAccessConnection, error)
}

// ConnectedUsers lists the users connected to a user (the connection feature).
type ConnectedUsers interface {
	ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error)
}

// AccessHeaders reads a user's stored access columns.
type AccessHeaders interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (model.Header, error)
}

const (
	billingAccessTTL        = 10 * time.Minute
	billingAccessFailureTTL = time.Minute
	billingAccessMaxEntries = 10_000
)

type billingAccessEntry struct {
	key     string
	result  model.GetBillingAccessResult
	expires time.Time
}

// BillingAccessService serves billing's subscription view to the SPA. It is
// never on the user-load path: only get-billing-access calls billing.
type BillingAccessService struct {
	fetcher BillingAccessFetcher
	conns   ConnectedUsers
	users   AccessHeaders
	clock   port.Clock

	mu    sync.Mutex
	cache map[string]billingAccessEntry
}

// NewBillingAccessService takes a nil fetcher when billing is not configured;
// the service then answers unavailable without any call.
func NewBillingAccessService(fetcher BillingAccessFetcher, conns ConnectedUsers, users AccessHeaders, clk port.Clock) *BillingAccessService {
	return &BillingAccessService{fetcher: fetcher, conns: conns, users: users, clock: clk, cache: map[string]billingAccessEntry{}}
}

func unavailableBillingAccess() model.GetBillingAccessResult {
	return model.GetBillingAccessResult{Connections: []model.BillingAccessConnection{}}
}

// GetBillingAccess answers from a per-user cache keyed by the access columns
// of the user and their connections: billing changes access through
// set-access, so a purchase moves the key and the next read asks again. A
// billing failure of any kind is "unavailable", never an error.
func (s *BillingAccessService) GetBillingAccess(ctx context.Context, userID vo.Id) (*model.GetBillingAccessResult, error) {
	if s.fetcher == nil {
		res := unavailableBillingAccess()
		return &res, nil
	}
	self, err := s.users.GetHeaderByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids, err := s.conns.ConnectedUserIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	connIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		connIDs = append(connIDs, id.String())
	}
	var key strings.Builder
	writeAccessKey(&key, self)
	for _, id := range ids {
		// A connection's access is part of the key: a partner's purchase
		// changes what billing says about them.
		h, herr := s.users.GetHeaderByID(ctx, id)
		if herr != nil {
			return nil, herr
		}
		writeAccessKey(&key, h)
	}
	cacheKey := key.String()

	now := s.clock.Now()
	if res, ok := s.cached(userID.String(), cacheKey, now); ok {
		reqctx.AddLogAttr(ctx, "billing_access", "cached")
		return &res, nil
	}

	res := unavailableBillingAccess()
	ttl := billingAccessFailureTTL
	u, conns, err := s.fetcher.GetAccess(ctx, userID.String(), connIDs)
	if err != nil {
		reqctx.AddLogAttr(ctx, "billing_access", "unavailable")
		reqctx.AddLogAttr(ctx, "billing_access_err_type", errType(err))
	} else {
		reqctx.AddLogAttr(ctx, "billing_access", "fetched")
		res = model.GetBillingAccessResult{Available: true, User: &u, Connections: knownConnections(conns, connIDs)}
		ttl = billingAccessTTL
	}
	s.store(userID.String(), billingAccessEntry{key: cacheKey, result: res, expires: now.Add(ttl)}, now)
	return &res, nil
}

func writeAccessKey(b *strings.Builder, h model.Header) {
	fmt.Fprintf(b, "%s|%s|%s;", h.ID, h.AccessLevel, datetime.FormatOrEmpty(h.AccessUntil))
}

// knownConnections keeps only the users Econumo asked about, once each, in
// the order asked: billing never decides who the caller's connections are.
func knownConnections(got []model.BillingAccessConnection, asked []string) []model.BillingAccessConnection {
	byID := make(map[string]model.BillingAccessConnection, len(got))
	for _, c := range got {
		if _, seen := byID[c.Id]; !seen {
			byID[c.Id] = c
		}
	}
	out := make([]model.BillingAccessConnection, 0, len(asked))
	for _, id := range asked {
		if c, ok := byID[id]; ok {
			out = append(out, c)
		}
	}
	return out
}

// errType names the failure without its message: a transport error's text
// carries the billing URL, query included.
func errType(err error) string {
	var t interface{ Timeout() bool }
	switch {
	case errors.As(err, &t) && t.Timeout():
		return "timeout"
	case errors.Is(err, billing.ErrBadResponse):
		return "bad_response"
	default:
		return "transport"
	}
}

func (s *BillingAccessService) cached(userID, key string, now time.Time) (model.GetBillingAccessResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[userID]
	if !ok || e.key != key || !now.Before(e.expires) {
		return model.GetBillingAccessResult{}, false
	}
	return e.result, true
}

func (s *BillingAccessService) store(userID string, e billingAccessEntry, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cache[userID]; !ok && len(s.cache) >= billingAccessMaxEntries {
		for id, old := range s.cache {
			if !now.Before(old.expires) {
				delete(s.cache, id)
			}
		}
		// Still full of live entries: drop an arbitrary one, a cache miss
		// costs only one billing call.
		for id := range s.cache {
			if len(s.cache) < billingAccessMaxEntries {
				break
			}
			delete(s.cache, id)
		}
	}
	s.cache[userID] = e
}
