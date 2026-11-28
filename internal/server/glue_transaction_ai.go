package server

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/transaction"
)

type transactionAIFactsRepo interface {
	GetLanguage(ctx context.Context, id vo.Id) (string, error)
	GetByID(ctx context.Context, id vo.Id) (*model.User, error)
}

// TransactionAIFacts reads what the CSV column guesser needs about the caller
// straight from the user repository.
type TransactionAIFacts struct {
	users transactionAIFactsRepo
	clk   port.Clock
}

func NewTransactionAIFacts(users transactionAIFactsRepo, clk port.Clock) *TransactionAIFacts {
	return &TransactionAIFacts{users: users, clk: clk}
}

func (f *TransactionAIFacts) AIFacts(ctx context.Context, userID vo.Id) (transaction.AIFactsInfo, error) {
	u, err := f.users.GetByID(ctx, userID)
	if err != nil {
		return transaction.AIFactsInfo{}, err
	}
	language, err := f.users.GetLanguage(ctx, userID)
	if err != nil {
		return transaction.AIFactsInfo{}, err
	}
	return transaction.AIFactsInfo{
		Language: language,
		Readonly: u.EffectiveAccessLevel(f.clk.Now()) == model.AccessLevelReadonly,
		AIOff:    !u.AIEnabled(),
	}, nil
}

var _ transaction.AIFacts = (*TransactionAIFacts)(nil)
