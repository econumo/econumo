package user_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestTimezonePersistAndGet(t *testing.T) {
	db := dbtest.NewSQLite(t)
	f := fixture.New(t, db)
	userID := f.User(fixture.User{})
	svc, _, _ := newUserSvc(t, db)

	uid, err := vo.ParseId(userID)
	if err != nil {
		t.Fatal(err)
	}
	// Fresh user: empty.
	tz, err := svc.GetTimezone(context.Background(), uid)
	if err != nil || tz != "" {
		t.Fatalf("GetTimezone = %q, %v; want empty, nil", tz, err)
	}
	// Persist a valid IANA name.
	if err := svc.PersistTimezone(context.Background(), uid, "Europe/Amsterdam"); err != nil {
		t.Fatal(err)
	}
	tz, _ = svc.GetTimezone(context.Background(), uid)
	if tz != "Europe/Amsterdam" {
		t.Fatalf("persisted timezone = %q", tz)
	}
	// Invalid names are silently ignored (never fail the request path).
	if err := svc.PersistTimezone(context.Background(), uid, "Not/AZone"); err != nil {
		t.Fatal(err)
	}
	tz, _ = svc.GetTimezone(context.Background(), uid)
	if tz != "Europe/Amsterdam" {
		t.Fatalf("invalid name overwrote timezone: %q", tz)
	}
}
