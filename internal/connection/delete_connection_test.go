package connection

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// stubTx runs fn directly on the given context (no real transaction).
type stubTx struct{}

func (stubTx) WithTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

// recordingAccountAccessRepo is a minimal AccountAccessRepository: it only
// records DeleteConnection calls, since that is all DeleteConnection's tx body
// still reaches on the repo directly (the account-access unwind itself moved
// behind the AccountAccessRevoker port).
type recordingAccountAccessRepo struct {
	deleteConnectionCalls [][2]vo.Id
	deleteConnectionErr   error
}

func (r *recordingAccountAccessRepo) ListReceived(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	return nil, nil
}
func (r *recordingAccountAccessRepo) ListIssued(ctx context.Context, userID vo.Id) ([]*model.AccountAccess, error) {
	return nil, nil
}
func (r *recordingAccountAccessRepo) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return vo.Id{}, nil
}
func (r *recordingAccountAccessRepo) ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	return nil, nil
}
func (r *recordingAccountAccessRepo) DeleteConnection(ctx context.Context, a, b vo.Id) error {
	r.deleteConnectionCalls = append(r.deleteConnectionCalls, [2]vo.Id{a, b})
	return r.deleteConnectionErr
}
func (r *recordingAccountAccessRepo) ConnectUsers(ctx context.Context, a, b vo.Id) error { return nil }

// recordingAccountAccessRevoker stands in for the real account-service adapter
// (server.ConnectionAccountAccessRevoker): it just records the pair it was
// asked to unwind. The REAL end-to-end unwind — including a PENDING grant
// between the pair, in both directions — is covered by
// internal/server/glue_connection_test.go (over the real account.Service) and
// the delete-connection apiparity scenario.
type recordingAccountAccessRevoker struct {
	calls [][2]vo.Id
	err   error
}

func (r *recordingAccountAccessRevoker) RevokeAccessBetween(ctx context.Context, a, b vo.Id) error {
	r.calls = append(r.calls, [2]vo.Id{a, b})
	return r.err
}

type recordingBudgetAccessRevoker struct {
	calls [][2]vo.Id
	err   error
}

func (r *recordingBudgetAccessRevoker) RevokeBetween(ctx context.Context, a, b vo.Id) error {
	r.calls = append(r.calls, [2]vo.Id{a, b})
	return r.err
}

func TestDeleteConnection_RevokesAccountAndBudgetAccessThenLink(t *testing.T) {
	userID := vo.NewId()
	connectedID := vo.NewId()
	accountRevoker := &recordingAccountAccessRevoker{}
	budgetRevoker := &recordingBudgetAccessRevoker{}
	accessRepo := &recordingAccountAccessRepo{}

	s := NewService(accessRepo, nil, nil, accountRevoker, budgetRevoker, nil, stubTx{}, nil)

	res, err := s.DeleteConnection(context.Background(), userID, model.DeleteConnectionRequest{Id: connectedID.String()})
	if err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}
	if res == nil {
		t.Fatalf("want non-nil result")
	}

	if len(accountRevoker.calls) != 1 || accountRevoker.calls[0] != [2]vo.Id{userID, connectedID} {
		t.Fatalf("accountAccess.RevokeAccessBetween calls = %+v, want one call with (userID, connectedID)", accountRevoker.calls)
	}
	if len(budgetRevoker.calls) != 1 || budgetRevoker.calls[0] != [2]vo.Id{userID, connectedID} {
		t.Fatalf("budgetAccess.RevokeBetween calls = %+v, want one call with (userID, connectedID)", budgetRevoker.calls)
	}
	if len(accessRepo.deleteConnectionCalls) != 1 || accessRepo.deleteConnectionCalls[0] != [2]vo.Id{userID, connectedID} {
		t.Fatalf("access.DeleteConnection calls = %+v, want one call with (userID, connectedID)", accessRepo.deleteConnectionCalls)
	}
}

func TestDeleteConnection_NilRevokers_SkipsUnwindStillDeletesLink(t *testing.T) {
	userID := vo.NewId()
	connectedID := vo.NewId()
	accessRepo := &recordingAccountAccessRepo{}

	// Both revokers nil: delete-connection must not panic, and must still
	// remove the connection link.
	s := NewService(accessRepo, nil, nil, nil, nil, nil, stubTx{}, nil)

	if _, err := s.DeleteConnection(context.Background(), userID, model.DeleteConnectionRequest{Id: connectedID.String()}); err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}
	if len(accessRepo.deleteConnectionCalls) != 1 {
		t.Fatalf("access.DeleteConnection calls = %d, want 1", len(accessRepo.deleteConnectionCalls))
	}
}

func TestDeleteConnection_AccountRevokerError_PropagatesAndSkipsLinkDelete(t *testing.T) {
	userID := vo.NewId()
	connectedID := vo.NewId()
	sentinel := errors.New("boom")
	accountRevoker := &recordingAccountAccessRevoker{err: sentinel}
	accessRepo := &recordingAccountAccessRepo{}

	s := NewService(accessRepo, nil, nil, accountRevoker, nil, nil, stubTx{}, nil)

	_, err := s.DeleteConnection(context.Background(), userID, model.DeleteConnectionRequest{Id: connectedID.String()})
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error propagated, got %v", err)
	}
	if len(accessRepo.deleteConnectionCalls) != 0 {
		t.Fatalf("access.DeleteConnection must not run when the account unwind fails; calls=%d", len(accessRepo.deleteConnectionCalls))
	}
}

func TestDeleteConnection_Self_ValidationError(t *testing.T) {
	userID := vo.NewId()
	s := NewService(&recordingAccountAccessRepo{}, nil, nil, nil, nil, nil, stubTx{}, nil)
	if _, err := s.DeleteConnection(context.Background(), userID, model.DeleteConnectionRequest{Id: userID.String()}); err == nil {
		t.Fatalf("want validation error deleting oneself")
	}
}
