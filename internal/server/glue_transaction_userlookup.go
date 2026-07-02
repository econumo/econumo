// TransactionUserLookup satisfies the transaction service's UserLookup port
// (author embed) by delegating to the user repository. It lives here, not in
// internal/infra/repo/transaction, because it needs the user feature's Header
// type and an infra package must not import a feature (see archtest).
package server

import (
	"context"

	apptransaction "github.com/econumo/econumo/internal/app/transaction"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

// transactionUserByID is the minimal user-repo surface for the author embed.
type transactionUserByID interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (user.Header, error)
}

// TransactionUserLookup adapts the user repository to app/transaction.UserLookup.
type TransactionUserLookup struct{ users transactionUserByID }

var _ apptransaction.UserLookup = (*TransactionUserLookup)(nil)

// NewTransactionUserLookup wraps a user repository.
func NewTransactionUserLookup(users transactionUserByID) *TransactionUserLookup {
	return &TransactionUserLookup{users: users}
}

func (l *TransactionUserLookup) GetOwner(ctx context.Context, userID string) (apptransaction.AuthorView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return apptransaction.AuthorView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return apptransaction.AuthorView{}, err
	}
	return apptransaction.AuthorView{ID: h.ID, Name: h.Name, Avatar: h.AvatarURL}, nil
}
