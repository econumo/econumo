// TransactionUserLookup satisfies the transaction service's UserLookup port
// (author embed) by delegating to the user repository. It lives here, not in
// internal/transaction/repo, because it needs the model package's Header
// type and an infra package must not import a feature (see archtest).
package server

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

// transactionUserByID is the minimal user-repo surface for the author embed.
type transactionUserByID interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (model.Header, error)
}

// TransactionUserLookup adapts the user repository to transaction.UserLookup.
type TransactionUserLookup struct{ users transactionUserByID }

var _ apptransaction.UserLookup = (*TransactionUserLookup)(nil)

// NewTransactionUserLookup wraps a user repository.
func NewTransactionUserLookup(users transactionUserByID) *TransactionUserLookup {
	return &TransactionUserLookup{users: users}
}

func (l *TransactionUserLookup) GetOwner(ctx context.Context, userID string) (model.AuthorView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return model.AuthorView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return model.AuthorView{}, err
	}
	return model.AuthorView{ID: h.ID, Name: h.Name, Avatar: h.AvatarURL}, nil
}
