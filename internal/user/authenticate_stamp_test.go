package user_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	appuser "github.com/econumo/econumo/internal/user"
)

type stampOrderTokens struct {
	appuser.AccessTokens
	calls *[]string
}

func (r stampOrderTokens) Touch(ctx context.Context, id vo.Id, lastUsedAt time.Time, expiresAt *time.Time) (int64, error) {
	*r.calls = append(*r.calls, "touch")
	return r.AccessTokens.Touch(ctx, id, lastUsedAt, expiresAt)
}

type stampOrderDeletion struct {
	appuser.DeletionStore
	calls *[]string
	err   error
}

func (r stampOrderDeletion) TouchLastActive(context.Context, vo.Id, time.Time) error {
	*r.calls = append(*r.calls, "stamp")
	return r.err
}

func newStampEnv(t *testing.T, stampErr error) (*appuser.Service, *[]string) {
	t.Helper()
	var calls []string
	svc, tokens, clk, uid := newAuthEnvOnWrapped(t, dbtest.New(t), func(inner appuser.AccessTokens) appuser.AccessTokens {
		return stampOrderTokens{AccessTokens: inner, calls: &calls}
	})
	svc.SetDeletion(stampOrderDeletion{calls: &calls, err: stampErr}, nil, nil, appuser.DeletionConfig{})
	exp := authT0.Add(appuser.SessionTTL)
	seedToken(t, tokens, uid, model.TokenKindSession, "eco_ses_stamp", &exp)
	clk.now = authT0.Add(time.Hour)
	return svc, &calls
}

func TestAuthenticate_StampsActivityBeforeTouchingTheToken(t *testing.T) {
	svc, calls := newStampEnv(t, nil)

	if _, err := svc.Authenticate(context.Background(), "eco_ses_stamp"); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if want := []string{"stamp", "touch"}; !slices.Equal(*calls, want) {
		t.Errorf("calls = %v, want %v", *calls, want)
	}
}

func TestAuthenticate_StampFailureStillAuthenticates(t *testing.T) {
	svc, calls := newStampEnv(t, errors.New("stamp failed"))

	if _, err := svc.Authenticate(context.Background(), "eco_ses_stamp"); err != nil {
		t.Fatalf("Authenticate with a failing stamp: %v", err)
	}
	if !slices.Contains(*calls, "touch") {
		t.Errorf("calls = %v, want the token touched", *calls)
	}
}
