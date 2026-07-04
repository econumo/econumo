package category

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// stubAccountAccess is a minimal AccountAccess whose AccountOwner and
// HasAdminGrant results are controlled per test.
type stubAccountAccess struct {
	owner    vo.Id
	ownerErr error
	admin    bool
	adminErr error
}

func (s stubAccountAccess) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return s.owner, s.ownerErr
}

func (s stubAccountAccess) HasAdminGrant(ctx context.Context, accountID, userID vo.Id) (bool, error) {
	return s.admin, s.adminErr
}

var errSentinel = errors.New("boom")

func TestResolveAccountOwner_AccountOwnerError_PropagatesError(t *testing.T) {
	s := &Service{access: stubAccountAccess{ownerErr: errSentinel}}
	_, err := s.resolveAccountOwner(context.Background(), vo.NewId(), vo.NewId())
	if !errors.Is(err, errSentinel) {
		t.Fatalf("want sentinel error propagated, got %v", err)
	}
}

func TestResolveAccountOwner_HasAdminGrantError_PropagatesError(t *testing.T) {
	s := &Service{access: stubAccountAccess{owner: vo.NewId(), adminErr: errSentinel}}
	_, err := s.resolveAccountOwner(context.Background(), vo.NewId(), vo.NewId())
	if !errors.Is(err, errSentinel) {
		t.Fatalf("want sentinel error propagated, got %v", err)
	}
}

func TestResolveAccountOwner_NotOwnerNotAdmin_ReturnsAccessDenied(t *testing.T) {
	s := &Service{access: stubAccountAccess{owner: vo.NewId(), admin: false}}
	_, err := s.resolveAccountOwner(context.Background(), vo.NewId(), vo.NewId())
	ad, ok := errs.AsAccessDenied(err)
	if !ok {
		t.Fatalf("want AccessDeniedError, got %v (%T)", err, err)
	}
	if ad.Msg != "Access is not allowed" {
		t.Errorf("want frozen message, got %q", ad.Msg)
	}
}

func TestResolveAccountOwner_OwnerMatch_ReturnsOwner(t *testing.T) {
	userID := vo.NewId()
	s := &Service{access: stubAccountAccess{owner: userID}}
	owner, err := s.resolveAccountOwner(context.Background(), userID, vo.NewId())
	if err != nil {
		t.Fatalf("want nil error for owner match, got %v", err)
	}
	if !owner.Equal(userID) {
		t.Errorf("want owner == userID, got %v", owner)
	}
}

func TestResolveAccountOwner_AdminGrant_ReturnsOwner(t *testing.T) {
	owner := vo.NewId()
	s := &Service{access: stubAccountAccess{owner: owner, admin: true}}
	got, err := s.resolveAccountOwner(context.Background(), vo.NewId(), vo.NewId())
	if err != nil {
		t.Fatalf("want nil error for admin grant, got %v", err)
	}
	if !got.Equal(owner) {
		t.Errorf("want owner returned, got %v", got)
	}
}
