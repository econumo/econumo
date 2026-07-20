package admin

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

var now = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

type testClock struct{}

func (testClock) Now() time.Time { return now }

type stubUsers struct {
	byID  map[string]UserRecord
	saved map[string]model.AccessLevel
	until map[string]*time.Time
}

func (s *stubUsers) GetUser(_ context.Context, id vo.Id) (UserRecord, error) {
	u, ok := s.byID[id.String()]
	if !ok {
		return UserRecord{}, errs.NewNotFound("User not found")
	}
	return u, nil
}

func (s *stubUsers) SetAccess(_ context.Context, id vo.Id, level model.AccessLevel, until *time.Time) (UserRecord, error) {
	rec, ok := s.byID[id.String()]
	if !ok {
		return UserRecord{}, errs.NewNotFound("User not found")
	}
	s.saved[id.String()] = level
	s.until[id.String()] = until
	rec.AccessLevel, rec.AccessUntil = level, until
	s.byID[id.String()] = rec
	return rec, nil
}

type stubConns struct{ ids map[string][]vo.Id }

func (s *stubConns) ConnectedUserIDs(_ context.Context, id vo.Id) ([]vo.Id, error) {
	return s.ids[id.String()], nil
}

func newFixture() (*Service, *stubUsers, vo.Id, vo.Id) {
	self, partner := vo.NewId(), vo.NewId()
	users := &stubUsers{
		byID: map[string]UserRecord{
			self.String():    {ID: self.String(), Name: "Alex", Email: "alex@example.test", AccessLevel: model.AccessLevelFull},
			partner.String(): {ID: partner.String(), Name: "Sam", Email: "sam@example.test", AccessLevel: model.AccessLevelFull},
		},
		saved: map[string]model.AccessLevel{},
		until: map[string]*time.Time{},
	}
	conns := &stubConns{ids: map[string][]vo.Id{self.String(): {partner}}}
	return NewService(users, conns, testClock{}), users, self, partner
}

func TestSetAccessWritesLevelAndExpiry(t *testing.T) {
	svc, users, self, _ := newFixture()
	until := "2027-01-01 00:00:00"
	view, err := svc.SetAccess(context.Background(), model.AdminSetAccessRequest{
		UserId: self.String(), Level: "full", Until: &until,
	})
	if err != nil {
		t.Fatal(err)
	}
	if users.saved[self.String()] != model.AccessLevelFull {
		t.Fatalf("level = %q", users.saved[self.String()])
	}
	if view.AccessUntil != until {
		t.Fatalf("accessUntil = %q, want %q", view.AccessUntil, until)
	}
}

func TestSetAccessNilUntilClearsExpiry(t *testing.T) {
	svc, users, self, _ := newFixture()
	view, err := svc.SetAccess(context.Background(), model.AdminSetAccessRequest{
		UserId: self.String(), Level: "readonly", Until: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if users.until[self.String()] != nil {
		t.Fatal("until must be NULL")
	}
	if view.AccessUntil != "" {
		t.Fatalf("accessUntil = %q, want empty string for NULL", view.AccessUntil)
	}
}

// Stripe retries webhooks; applying the same call twice must be
// indistinguishable from applying it once.
func TestSetAccessIsIdempotent(t *testing.T) {
	svc, _, self, _ := newFixture()
	until := "2027-01-01 00:00:00"
	req := model.AdminSetAccessRequest{UserId: self.String(), Level: "full", Until: &until}
	first, err := svc.SetAccess(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.SetAccess(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if *first != *second {
		t.Fatalf("not idempotent: %+v vs %+v", *first, *second)
	}
}

func TestSetAccessRejectsUnknownLevel(t *testing.T) {
	svc, _, self, _ := newFixture()
	_, err := svc.SetAccess(context.Background(), model.AdminSetAccessRequest{
		UserId: self.String(), Level: "premium",
	})
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

func TestSetAccessRejectsBadUntil(t *testing.T) {
	svc, _, self, _ := newFixture()
	bad := "2027-01-01T00:00:00Z"
	_, err := svc.SetAccess(context.Background(), model.AdminSetAccessRequest{
		UserId: self.String(), Level: "full", Until: &bad,
	})
	if err == nil {
		t.Fatal("RFC3339 accepted; the wire format is the space-separated layout")
	}
}

func TestSetAccessUnknownUser(t *testing.T) {
	svc, _, _, _ := newFixture()
	_, err := svc.SetAccess(context.Background(), model.AdminSetAccessRequest{
		UserId: vo.NewId().String(), Level: "full",
	})
	var nf *errs.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestUserContextReturnsSelfAndConnections(t *testing.T) {
	svc, _, self, partner := newFixture()
	res, err := svc.UserContext(context.Background(), self)
	if err != nil {
		t.Fatal(err)
	}
	if res.User.Id != self.String() || res.User.Email != "alex@example.test" {
		t.Fatalf("user = %+v", res.User)
	}
	if len(res.Connections) != 1 || res.Connections[0].Id != partner.String() {
		t.Fatalf("connections = %+v", res.Connections)
	}
	if res.Connections[0].Email != "sam@example.test" {
		t.Fatalf("connection email = %q — the portal needs it to notify a beneficiary", res.Connections[0].Email)
	}
}

// The whole model turns on (level, until, now) collapsing to one value; the
// portal must not have to re-derive it.
func TestUserContextEffectiveDivergesAfterExpiry(t *testing.T) {
	svc, users, self, _ := newFixture()
	past := now.Add(-time.Hour)
	rec := users.byID[self.String()]
	rec.AccessUntil = &past
	users.byID[self.String()] = rec

	res, err := svc.UserContext(context.Background(), self)
	if err != nil {
		t.Fatal(err)
	}
	if res.User.AccessLevel != "full" {
		t.Fatalf("raw accessLevel = %q, want the stored value full", res.User.AccessLevel)
	}
	if res.User.EffectiveAccessLevel != "readonly" {
		t.Fatalf("effective = %q, want readonly", res.User.EffectiveAccessLevel)
	}
}

// A manually restricted user must stay distinguishable from a lapsed one: the
// portal offers a purchase to the second, not the first.
func TestUserContextManualRestrictionKeepsRawLevel(t *testing.T) {
	svc, users, self, _ := newFixture()
	rec := users.byID[self.String()]
	rec.AccessLevel = model.AccessLevelReadonly
	users.byID[self.String()] = rec

	res, err := svc.UserContext(context.Background(), self)
	if err != nil {
		t.Fatal(err)
	}
	if res.User.AccessLevel != "readonly" || res.User.AccessUntil != "" {
		t.Fatalf("want readonly with no expiry, got %q / %q", res.User.AccessLevel, res.User.AccessUntil)
	}
}

func TestUserContextUnknownUser(t *testing.T) {
	svc, _, _, _ := newFixture()
	if _, err := svc.UserContext(context.Background(), vo.NewId()); err == nil {
		t.Fatal("want NotFound for an unknown user")
	}
}

func TestUserContextNoConnections(t *testing.T) {
	svc, _, _, partner := newFixture()
	res, err := svc.UserContext(context.Background(), partner)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Connections) != 0 {
		t.Fatalf("connections = %+v, want empty", res.Connections)
	}
}

// A connection whose user row is gone (account deletion is the planned next
// iteration) must be skipped, not abort the whole context: the portal reads an
// error here as "no such user" for the TARGET user.
func TestUserContextSkipsDanglingConnection(t *testing.T) {
	svc, _, self, partner := newFixture()
	// A second connection pointing at a user that has no row.
	svcConns := &stubConns{ids: map[string][]vo.Id{self.String(): {partner, vo.NewId()}}}
	users := &stubUsers{byID: map[string]UserRecord{
		self.String():    {ID: self.String(), Name: "Alex", Email: "alex@example.test", AccessLevel: model.AccessLevelFull},
		partner.String(): {ID: partner.String(), Name: "Sam", Email: "sam@example.test", AccessLevel: model.AccessLevelFull},
	}, saved: map[string]model.AccessLevel{}, until: map[string]*time.Time{}}
	svc = NewService(users, svcConns, testClock{})

	res, err := svc.UserContext(context.Background(), self)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Connections) != 1 || res.Connections[0].Id != partner.String() {
		t.Fatalf("connections = %+v, want just the resolvable partner", res.Connections)
	}
}

// attrValue extracts a logged attr by key, or "" when absent.
func attrValue(attrs []slog.Attr, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value.String()
		}
	}
	return ""
}

// Every admin write must land on the operation log line with the target and
// the values written — the listener is driven by an unattended webhook, so the
// log IS the audit trail.
func TestSetAccessLogsTargetAndValues(t *testing.T) {
	svc, _, self, _ := newFixture()
	ctx := reqctx.WithLogAttrs(context.Background())
	until := "2027-01-01 00:00:00"
	if _, err := svc.SetAccess(ctx, model.AdminSetAccessRequest{
		UserId: self.String(), Level: "readonly", Until: &until,
	}); err != nil {
		t.Fatal(err)
	}
	attrs := reqctx.LogAttrs(ctx)
	if got := attrValue(attrs, "user_id"); got != self.String() {
		t.Fatalf("user_id attr = %q, want %q", got, self.String())
	}
	if got := attrValue(attrs, "access_level"); got != "readonly" {
		t.Fatalf("access_level attr = %q", got)
	}
	if got := attrValue(attrs, "access_until"); got != until {
		t.Fatalf("access_until attr = %q, want %q", got, until)
	}
}

// A rejected write must still record what was attempted: the WARN line with
// the attempted values is the evidence trail for a misbehaving portal.
func TestSetAccessLogsAttemptOnWriteFailure(t *testing.T) {
	svc, _, _, _ := newFixture()
	ctx := reqctx.WithLogAttrs(context.Background())
	unknown := vo.NewId()
	if _, err := svc.SetAccess(ctx, model.AdminSetAccessRequest{
		UserId: unknown.String(), Level: "full",
	}); err == nil {
		t.Fatal("want error for unknown user")
	}
	if got := attrValue(reqctx.LogAttrs(ctx), "user_id"); got != unknown.String() {
		t.Fatalf("user_id attr = %q — a failed write must still log the attempt", got)
	}
}

func TestUserContextLogsTargetAndDisclosure(t *testing.T) {
	svc, _, self, _ := newFixture()
	ctx := reqctx.WithLogAttrs(context.Background())
	if _, err := svc.UserContext(ctx, self); err != nil {
		t.Fatal(err)
	}
	attrs := reqctx.LogAttrs(ctx)
	if got := attrValue(attrs, "user_id"); got != self.String() {
		t.Fatalf("user_id attr = %q, want %q", got, self.String())
	}
	if got := attrValue(attrs, "connections"); got != "1" {
		t.Fatalf("connections attr = %q, want 1", got)
	}
}
