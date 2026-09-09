package simplefin

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/imports"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Parser reads the rows the Client stored (imports.EncodePullEvent) back into
// events; the posted day is taken in the caller's timezone.
type Parser struct{}

func (Parser) ParseEvent(ctx context.Context, ev *model.ImportEvent) (model.IngestEvent, error) {
	return Parse([]byte(ev.Payload), reqctx.Location(ctx))
}

var signedDecimalRe = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

func Parse(payload []byte, loc *time.Location) (model.IngestEvent, error) {
	var env imports.PullEvent
	if err := json.Unmarshal(payload, &env); err != nil {
		return model.IngestEvent{}, errors.New("invalid JSON")
	}
	account := imports.NormalizeExternalAccountID(env.ExternalAccountID)
	if account == "" {
		return model.IngestEvent{}, errors.New("externalAccountId is required")
	}
	var tx transaction
	if err := json.Unmarshal(env.Transaction, &tx); err != nil {
		return model.IngestEvent{}, errors.New("invalid transaction")
	}
	id := strings.TrimSpace(tx.ID)
	if id == "" {
		return model.IngestEvent{}, errors.New("id is required")
	}
	if tx.Posted <= 0 {
		return model.IngestEvent{}, errors.New("posted is required")
	}
	amount := strings.TrimSpace(tx.Amount)
	if !signedDecimalRe.MatchString(amount) {
		return model.IngestEvent{}, errors.New("amount must be a signed decimal")
	}
	d := vo.NewDecimal(amount)
	if d.IsZero() {
		return model.IngestEvent{}, errors.New("amount must not be zero")
	}
	typ := model.TransactionTypeIncome
	if d.IsNegative() {
		typ = model.TransactionTypeExpense
	}
	payee := strings.TrimSpace(tx.Payee)
	description := strings.TrimSpace(tx.Description)
	if payee == "" {
		payee = description
	}
	if runes := []rune(payee); len(runes) > 255 {
		payee = string(runes[:255])
	}
	return model.IngestEvent{
		ExternalAccountID: account, ExternalTransactionID: id, Type: typ, Amount: d.Abs().String(),
		PostedAt: time.Unix(tx.Posted, 0).In(loc), Payee: payee, Description: description,
	}, nil
}
