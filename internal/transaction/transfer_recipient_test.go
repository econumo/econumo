package transaction

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// A transfer persisted without a recipient renders as "[Hidden account]" and
// credits nothing (issue #261), so the state builder must refuse it before the
// row can exist.
func TestBuildState_TransferWithoutRecipient_RejectedAsBlank(t *testing.T) {
	empty := ""
	for name, recipient := range map[string]*string{"nil": nil, "empty": &empty} {
		t.Run(name, func(t *testing.T) {
			_, err := buildState(vo.NewId(), vo.NewId(), model.TransactionTypeTransfer, vo.NewId(), "10",
				nil, recipient, nil, nil, nil, "", time.Now(), time.Now())
			ve, ok := errs.AsValidation(err)
			if !ok {
				t.Fatalf("want ValidationError, got %v (%T)", err, err)
			}
			if len(ve.Fields) != 1 || ve.Fields[0].Key != "accountRecipientId" || ve.Fields[0].Code != errs.CodeIsBlank {
				t.Fatalf("want a single accountRecipientId blank error, got %+v", ve.Fields)
			}
		})
	}
}

func TestBuildState_NonTransferWithoutRecipient_Accepted(t *testing.T) {
	_, err := buildState(vo.NewId(), vo.NewId(), model.TransactionTypeExpense, vo.NewId(), "10",
		nil, nil, nil, nil, nil, "", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("expense without recipient must build, got %v", err)
	}
}
