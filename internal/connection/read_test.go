package connection

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type connectedUsersRepo struct {
	recordingAccountAccessRepo
	connected []vo.Id
}

func (r *connectedUsersRepo) ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	return r.connected, nil
}

type stubUserLookup struct {
	marked map[string]time.Time
	// gone answer NotFound from the named lookup ("owner" or "marked").
	gone map[string]string
}

func (l stubUserLookup) GetOwner(ctx context.Context, userID string) (model.OwnerView, error) {
	if l.gone[userID] == "owner" {
		return model.OwnerView{}, errs.NewNotFound("User not found")
	}
	return model.OwnerView{ID: userID, Name: "User " + userID}, nil
}

func (l stubUserLookup) DeletionMarkedAt(ctx context.Context, userID string) (*time.Time, error) {
	if l.gone[userID] == "marked" {
		return nil, errs.NewNotFound("User not found")
	}
	at, ok := l.marked[userID]
	if !ok {
		return nil, nil
	}
	return &at, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestGetConnectionList_DeletionMarkedAt(t *testing.T) {
	caller := vo.NewId()
	marked := vo.NewId()
	unmarked := vo.NewId()
	markedAt := time.Date(2026, 11, 20, 9, 30, 15, 0, time.UTC)

	repo := &connectedUsersRepo{connected: []vo.Id{marked, unmarked}}
	users := stubUserLookup{marked: map[string]time.Time{marked.String(): markedAt}}
	s := NewService(repo, nil, users, nil, nil, nil, stubTx{}, fixedClock{now: markedAt})

	res, err := s.GetConnectionList(context.Background(), caller)
	if err != nil {
		t.Fatalf("GetConnectionList: %v", err)
	}
	got := map[string]string{}
	for _, item := range res.Items {
		got[item.User.Id] = item.DeletionMarkedAt
	}
	if got[marked.String()] != "2026-11-20 09:30:15" {
		t.Errorf("marked user: deletionMarkedAt = %q, want %q", got[marked.String()], "2026-11-20 09:30:15")
	}
	if v, ok := got[unmarked.String()]; !ok || v != "" {
		t.Errorf("unmarked user: deletionMarkedAt = %q (present %v), want \"\"", v, ok)
	}
}

// A connected user wiped between the listing and the per-user lookups drops
// out of the list instead of failing it.
func TestGetConnectionList_SkipsAVanishedUser(t *testing.T) {
	for _, lookup := range []string{"owner", "marked"} {
		t.Run(lookup, func(t *testing.T) {
			caller, gone, kept := vo.NewId(), vo.NewId(), vo.NewId()
			repo := &connectedUsersRepo{connected: []vo.Id{gone, kept}}
			users := stubUserLookup{gone: map[string]string{gone.String(): lookup}}
			s := NewService(repo, nil, users, nil, nil, nil, stubTx{}, fixedClock{now: time.Now()})

			res, err := s.GetConnectionList(context.Background(), caller)
			if err != nil {
				t.Fatalf("GetConnectionList: %v", err)
			}
			if len(res.Items) != 1 || res.Items[0].User.Id != kept.String() {
				t.Fatalf("items = %+v, want only %s", res.Items, kept)
			}
		})
	}
}
