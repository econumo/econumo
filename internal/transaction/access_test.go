package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// stubAccountResolver is a minimal AccountResolver whose AccountOwner and
// AccountListForUser results are controlled per test.
type stubAccountResolver struct {
	owner    vo.Id
	ownerErr error
}

func (s stubAccountResolver) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return s.owner, s.ownerErr
}

func (s stubAccountResolver) AccountListForUser(ctx context.Context, userID vo.Id) ([]model.AccountResult, error) {
	return nil, nil
}

func (s stubAccountResolver) AccountCurrency(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return vo.Id{}, s.ownerErr
}

// stubAccountGrants is a minimal AccountGrants whose HasWriteGrant result is
// controlled per test.
type stubAccountGrants struct {
	ok  bool
	err error
}

func (s stubAccountGrants) HasWriteGrant(ctx context.Context, accountID, userID vo.Id) (bool, error) {
	return s.ok, s.err
}

// stubVisibleAccounts is a minimal VisibleAccounts whose VisibleAccountIDs
// result is controlled per test.
type stubVisibleAccounts struct {
	ids []vo.Id
	err error
}

func (s stubVisibleAccounts) VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	return s.ids, s.err
}

var errSentinel = errors.New("boom")

func TestCheckWriteAccess_AccountOwnerError_ReturnsNotAvailableValidation(t *testing.T) {
	s := &Service{
		accounts: stubAccountResolver{ownerErr: errSentinel},
	}
	err := s.checkWriteAccess(context.Background(), vo.NewId(), vo.NewId(), "account.account.not_available")
	ve, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("want ValidationError, got %v (%T)", err, err)
	}
	if ve.Msg != "account.account.not_available" {
		t.Errorf("want notAvailableMsg propagated, got %q", ve.Msg)
	}
}

func TestCheckWriteAccess_HasWriteGrantError_PropagatesError(t *testing.T) {
	userID := vo.NewId()
	accountID := vo.NewId()
	s := &Service{
		accounts: stubAccountResolver{owner: vo.NewId()}, // not the caller: owner mismatch
		grants:   stubAccountGrants{err: errSentinel},
	}
	err := s.checkWriteAccess(context.Background(), userID, accountID, "transaction.transaction.not_available")
	if !errors.Is(err, errSentinel) {
		t.Fatalf("want sentinel error propagated, got %v", err)
	}
}

func TestCheckWriteAccess_OwnerMatch_Allowed(t *testing.T) {
	userID := vo.NewId()
	s := &Service{accounts: stubAccountResolver{owner: userID}}
	if err := s.checkWriteAccess(context.Background(), userID, vo.NewId(), "msg"); err != nil {
		t.Fatalf("want nil for owner match, got %v", err)
	}
}

func TestCheckWriteAccess_WriteGrantDenied_ReturnsNotAvailableValidation(t *testing.T) {
	s := &Service{
		accounts: stubAccountResolver{owner: vo.NewId()},
		grants:   stubAccountGrants{ok: false},
	}
	err := s.checkWriteAccess(context.Background(), vo.NewId(), vo.NewId(), "account.account.not_available")
	ve, ok := errs.AsValidation(err)
	if !ok {
		t.Fatalf("want ValidationError, got %v (%T)", err, err)
	}
	if ve.Msg != "account.account.not_available" {
		t.Errorf("want notAvailableMsg, got %q", ve.Msg)
	}
}

func TestCheckWriteAccess_WriteGrantAllowed(t *testing.T) {
	s := &Service{
		accounts: stubAccountResolver{owner: vo.NewId()},
		grants:   stubAccountGrants{ok: true},
	}
	if err := s.checkWriteAccess(context.Background(), vo.NewId(), vo.NewId(), "msg"); err != nil {
		t.Fatalf("want nil when write grant held, got %v", err)
	}
}

func TestCheckViewAccess_VisibleAccountIDsError_PropagatesError(t *testing.T) {
	s := &Service{visible: stubVisibleAccounts{err: errSentinel}}
	err := s.checkViewAccess(context.Background(), vo.NewId(), vo.NewId())
	if !errors.Is(err, errSentinel) {
		t.Fatalf("want sentinel error propagated, got %v", err)
	}
}

func TestCheckViewAccess_NotVisible_ReturnsAccessDenied(t *testing.T) {
	s := &Service{visible: stubVisibleAccounts{ids: []vo.Id{vo.NewId()}}}
	err := s.checkViewAccess(context.Background(), vo.NewId(), vo.NewId())
	ad, ok := errs.AsAccessDenied(err)
	if !ok {
		t.Fatalf("want AccessDeniedError, got %v (%T)", err, err)
	}
	if ad.Msg != "Access is not allowed" {
		t.Errorf("want frozen message, got %q", ad.Msg)
	}
}

func TestCheckViewAccess_Visible_Allowed(t *testing.T) {
	accountID := vo.NewId()
	s := &Service{visible: stubVisibleAccounts{ids: []vo.Id{vo.NewId(), accountID}}}
	if err := s.checkViewAccess(context.Background(), vo.NewId(), accountID); err != nil {
		t.Fatalf("want nil when account is visible, got %v", err)
	}
}
