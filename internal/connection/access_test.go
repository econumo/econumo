package connection

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// stubAccountAccessRepo is a minimal AccountAccessRepository whose AccountOwner
// and Get results are controlled per test; the remaining methods are unused by
// requireOwnerAdmin and just return zero values.
type stubAccountAccessRepo struct {
	owner    vo.Id
	ownerErr error
	grant    *model.AccountAccess
	getErr   error
}

func (s stubAccountAccessRepo) Get(ctx context.Context, accountID, userID vo.Id) (*model.AccountAccess, error) {
	return s.grant, s.getErr
}
func (s stubAccountAccessRepo) Save(ctx context.Context, a *model.AccountAccess) error { return nil }
func (s stubAccountAccessRepo) Delete(ctx context.Context, accountID, userID vo.Id) error {
	return nil
}
func (s stubAccountAccessRepo) ListReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	return nil, nil
}
func (s stubAccountAccessRepo) ListIssued(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	return nil, nil
}
func (s stubAccountAccessRepo) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return s.owner, s.ownerErr
}
func (s stubAccountAccessRepo) ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	return nil, nil
}
func (s stubAccountAccessRepo) DeleteConnection(ctx context.Context, a, b vo.Id) error { return nil }
func (s stubAccountAccessRepo) ConnectUsers(ctx context.Context, a, b vo.Id) error     { return nil }
func (s stubAccountAccessRepo) DeleteOption(ctx context.Context, accountID, userID vo.Id) error {
	return nil
}

var errSentinel = errors.New("boom")

func TestRequireOwnerAdmin_AccountOwnerError_PropagatesError(t *testing.T) {
	s := &Service{access: stubAccountAccessRepo{ownerErr: errSentinel}}
	err := s.requireOwnerAdmin(context.Background(), vo.NewId(), vo.NewId())
	if !errors.Is(err, errSentinel) {
		t.Fatalf("want sentinel error propagated, got %v", err)
	}
}

func TestRequireOwnerAdmin_GetNotFound_ReturnsAccessDenied(t *testing.T) {
	s := &Service{access: stubAccountAccessRepo{
		owner:  vo.NewId(),
		getErr: errs.NewNotFound("not found"),
	}}
	err := s.requireOwnerAdmin(context.Background(), vo.NewId(), vo.NewId())
	ad, ok := errs.AsAccessDenied(err)
	if !ok {
		t.Fatalf("want AccessDeniedError, got %v (%T)", err, err)
	}
	if ad.Msg != "Access denied" {
		t.Errorf("want frozen message, got %q", ad.Msg)
	}
}

func TestRequireOwnerAdmin_GetOtherError_PropagatesError(t *testing.T) {
	s := &Service{access: stubAccountAccessRepo{
		owner:  vo.NewId(),
		getErr: errSentinel,
	}}
	err := s.requireOwnerAdmin(context.Background(), vo.NewId(), vo.NewId())
	if !errors.Is(err, errSentinel) {
		t.Fatalf("want sentinel error propagated (not coerced to AccessDenied), got %v", err)
	}
}

func TestRequireOwnerAdmin_GrantNotAdmin_ReturnsAccessDenied(t *testing.T) {
	s := &Service{access: stubAccountAccessRepo{
		owner: vo.NewId(),
		grant: &model.AccountAccess{Role: model.RoleUser},
	}}
	err := s.requireOwnerAdmin(context.Background(), vo.NewId(), vo.NewId())
	ad, ok := errs.AsAccessDenied(err)
	if !ok {
		t.Fatalf("want AccessDeniedError, got %v (%T)", err, err)
	}
	if ad.Msg != "Access denied" {
		t.Errorf("want frozen message, got %q", ad.Msg)
	}
}

func TestRequireOwnerAdmin_OwnerMatch_Allowed(t *testing.T) {
	userID := vo.NewId()
	s := &Service{access: stubAccountAccessRepo{owner: userID}}
	if err := s.requireOwnerAdmin(context.Background(), userID, vo.NewId()); err != nil {
		t.Fatalf("want nil for owner match, got %v", err)
	}
}

func TestRequireOwnerAdmin_GrantAdmin_Allowed(t *testing.T) {
	s := &Service{access: stubAccountAccessRepo{
		owner: vo.NewId(),
		grant: &model.AccountAccess{Role: model.RoleAdmin},
	}}
	if err := s.requireOwnerAdmin(context.Background(), vo.NewId(), vo.NewId()); err != nil {
		t.Fatalf("want nil for admin grant, got %v", err)
	}
}
