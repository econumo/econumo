package user_test

import (
	"fmt"
	"testing"

	"github.com/econumo/econumo/internal/shared/errs"
	appuser "github.com/econumo/econumo/internal/user"
)

type fakePgError struct{}

func (fakePgError) Error() string    { return "deadlock detected near user_secret@example.test" }
func (fakePgError) SQLState() string { return "40P01" }

type fakeSQLiteError struct{}

func (fakeSQLiteError) Error() string { return "database is locked: user_secret@example.test" }
func (fakeSQLiteError) Code() int     { return 5 }

func TestFailureReason(t *testing.T) {
	sentinel := errs.NewSentinel("server.ErrWipeSetChanged", "wipe: the set of budgets changed while locking")
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"wrapped sentinel":        {fmt.Errorf("erase: %w", fmt.Errorf("lock: %w", sentinel)), "server.ErrWipeSetChanged"},
		"wrapped PostgreSQL code": {fmt.Errorf("erase: %w", fakePgError{}), "sqlstate 40P01"},
		"wrapped SQLite code":     {fmt.Errorf("erase: %w", fakeSQLiteError{}), "sqlite 5"},
		"plain wrapped error":     {fmt.Errorf("erase: %w", fmt.Errorf("step: %w", wipeRefused{})), "user_test.wipeRefused"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := appuser.FailureReasonForTest(tc.err); got != tc.want {
				t.Errorf("reason = %q, want %q", got, tc.want)
			}
		})
	}
}
