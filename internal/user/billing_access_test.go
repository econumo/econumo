package user

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/infra/billing"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

var (
	baUser  = vo.MustParseId("11111111-1111-1111-1111-111111111111")
	baConnA = vo.MustParseId("aaaaaaaa-0000-0000-0000-000000000001")
	baConnB = vo.MustParseId("bbbbbbbb-0000-0000-0000-000000000001")
)

type baClock struct{ now time.Time }

func (c *baClock) Now() time.Time { return c.now }

type baConns struct{ ids []vo.Id }

func (c *baConns) ConnectedUserIDs(context.Context, vo.Id) ([]vo.Id, error) { return c.ids, nil }

type baHeaders struct{ byID map[string]model.Header }

func (h *baHeaders) GetHeaderByID(_ context.Context, id vo.Id) (model.Header, error) {
	return h.byID[id.String()], nil
}

type baFetcher struct {
	calls []([]string)
	user  model.BillingAccessUser
	conns []model.BillingAccessConnection
	err   error
}

func (f *baFetcher) GetAccess(_ context.Context, uid string, ids []string) (model.BillingAccessUser, []model.BillingAccessConnection, error) {
	f.calls = append(f.calls, append([]string{uid}, ids...))
	return f.user, f.conns, f.err
}

type baFixture struct {
	clock   *baClock
	headers *baHeaders
	fetcher *baFetcher
	svc     *BillingAccessService
}

func newBAFixture(withFetcher bool) *baFixture {
	clk := &baClock{now: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)}
	until := clk.now.Add(30 * 24 * time.Hour)
	headers := &baHeaders{byID: map[string]model.Header{
		baUser.String():  {ID: baUser.String(), AccessLevel: model.AccessLevelFull, AccessUntil: &until},
		baConnA.String(): {ID: baConnA.String(), AccessLevel: model.AccessLevelFull},
		baConnB.String(): {ID: baConnB.String(), AccessLevel: model.AccessLevelReadonly},
	}}
	f := &baFixture{clock: clk, headers: headers, fetcher: &baFetcher{
		user: model.BillingAccessUser{Paid: true, Renewing: true, RenewsAt: "2026-12-24 10:00:00"},
		conns: []model.BillingAccessConnection{
			{Id: baConnB.String(), Paid: false},
			{Id: "cccccccc-0000-0000-0000-000000000001", Paid: true, Renewing: true},
			{Id: baConnA.String(), Paid: true, Renewing: true},
		},
	}}
	var fetcher BillingAccessFetcher
	if withFetcher {
		fetcher = f.fetcher
	}
	// Connections come back unsorted: the request and the key must not depend on it.
	f.svc = NewBillingAccessService(fetcher, &baConns{ids: []vo.Id{baConnB, baConnA}}, headers, clk)
	return f
}

func (f *baFixture) get(t *testing.T) model.GetBillingAccessResult {
	t.Helper()
	res, err := f.svc.GetBillingAccess(context.Background(), baUser)
	if err != nil {
		t.Fatal(err)
	}
	return *res
}

func TestBillingAccessWithoutBillingMakesNoCall(t *testing.T) {
	f := newBAFixture(false)
	got := f.get(t)
	if got.Available || got.User != nil || got.Connections == nil || len(got.Connections) != 0 {
		t.Fatalf("got %+v, want unavailable", got)
	}
	if len(f.fetcher.calls) != 0 {
		t.Fatalf("billing called %d times", len(f.fetcher.calls))
	}
}

func TestBillingAccessAsksAboutTheConnectionsAndKeepsOnlyThose(t *testing.T) {
	f := newBAFixture(true)
	got := f.get(t)
	if want := [][]string{{baUser.String(), baConnA.String(), baConnB.String()}}; !reflect.DeepEqual(f.fetcher.calls, want) {
		t.Fatalf("calls = %v, want %v", f.fetcher.calls, want)
	}
	want := model.GetBillingAccessResult{
		Available: true,
		User:      &model.BillingAccessUser{Paid: true, Renewing: true, RenewsAt: "2026-12-24 10:00:00"},
		Connections: []model.BillingAccessConnection{
			{Id: baConnA.String(), Paid: true, Renewing: true},
			{Id: baConnB.String(), Paid: false},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBillingAccessIsCachedUntilTheAccessChanges(t *testing.T) {
	f := newBAFixture(true)
	f.get(t)
	f.clock.now = f.clock.now.Add(9 * time.Minute)
	f.get(t)
	if len(f.fetcher.calls) != 1 {
		t.Fatalf("calls = %d, want a cache hit", len(f.fetcher.calls))
	}

	// A purchase moves accessUntil through set-access: ask again at once.
	h := f.headers.byID[baUser.String()]
	later := h.AccessUntil.Add(30 * 24 * time.Hour)
	h.AccessUntil = &later
	f.headers.byID[baUser.String()] = h
	f.get(t)
	if len(f.fetcher.calls) != 2 {
		t.Fatalf("calls = %d, want a refetch after the user's access changed", len(f.fetcher.calls))
	}

	// So does a partner's purchase.
	c := f.headers.byID[baConnB.String()]
	c.AccessLevel = model.AccessLevelFull
	f.headers.byID[baConnB.String()] = c
	f.get(t)
	if len(f.fetcher.calls) != 3 {
		t.Fatalf("calls = %d, want a refetch after a connection's access changed", len(f.fetcher.calls))
	}

	f.clock.now = f.clock.now.Add(billingAccessTTL)
	f.get(t)
	if len(f.fetcher.calls) != 4 {
		t.Fatalf("calls = %d, want a refetch after the TTL", len(f.fetcher.calls))
	}
}

func TestBillingAccessFailureIsUnavailableAndRetriedSooner(t *testing.T) {
	f := newBAFixture(true)
	f.fetcher.err = billing.ErrBadResponse
	got := f.get(t)
	if got.Available || got.User != nil || len(got.Connections) != 0 {
		t.Fatalf("got %+v, want unavailable", got)
	}
	f.clock.now = f.clock.now.Add(50 * time.Second)
	f.get(t)
	if len(f.fetcher.calls) != 1 {
		t.Fatalf("calls = %d, want the failure cached", len(f.fetcher.calls))
	}
	f.fetcher.err = nil
	f.clock.now = f.clock.now.Add(10 * time.Second)
	if got := f.get(t); !got.Available || len(f.fetcher.calls) != 2 {
		t.Fatalf("got %+v after %d calls, want a fresh answer a minute after the failure", got, len(f.fetcher.calls))
	}
}

func TestBillingAccessErrTypeNamesNoMessage(t *testing.T) {
	if got := errType(errors.Join(billing.ErrBadResponse)); got != "bad_response" {
		t.Fatalf("errType = %q", got)
	}
	if got := errType(errors.New("dial tcp: https://billing.example/?secret=1")); got != "transport" {
		t.Fatalf("errType = %q", got)
	}
}

func TestBillingAccessCacheStaysBounded(t *testing.T) {
	f := newBAFixture(true)
	now := f.clock.now
	for i := 0; i < billingAccessMaxEntries+10; i++ {
		f.svc.store(vo.NewId().String(), billingAccessEntry{expires: now.Add(time.Hour)}, now)
	}
	if n := len(f.svc.cache); n > billingAccessMaxEntries {
		t.Fatalf("cache holds %d entries, max %d", n, billingAccessMaxEntries)
	}
}

func TestBillingAccessDoesNotCacheARequestTheCallerAbandoned(t *testing.T) {
	f := newBAFixture(true)
	f.fetcher.err = context.Canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if res, err := f.svc.GetBillingAccess(ctx, baUser); err != nil || res.Available {
		t.Fatalf("got %+v, %v; want unavailable", res, err)
	}
	f.fetcher.err = nil
	if got := f.get(t); !got.Available || len(f.fetcher.calls) != 2 {
		t.Fatalf("got %+v after %d calls, want a fresh ask", got, len(f.fetcher.calls))
	}
}
