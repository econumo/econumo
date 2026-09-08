package imports

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// simpleFINEnvelope is the stored payload of a pull event: the provider's
// row verbatim plus the account it belongs to, which the row itself lacks.
type simpleFINEnvelope struct {
	ExternalAccountID string          `json:"externalAccountId"`
	Transaction       json.RawMessage `json:"transaction"`
}

type simpleFINTransaction struct {
	ID          string `json:"id"`
	Posted      int64  `json:"posted"`
	Amount      string `json:"amount"`
	Description string `json:"description"`
	Payee       string `json:"payee"`
}

var signedDecimalRe = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

func EncodeSimpleFINEvent(tx model.ExternalTransaction) []byte {
	b, _ := json.Marshal(simpleFINEnvelope{ExternalAccountID: tx.ExternalAccountID, Transaction: tx.Raw})
	return b
}

func ParseSimpleFINEvent(payload []byte, loc *time.Location) (model.IngestEvent, error) {
	var env simpleFINEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return model.IngestEvent{}, errors.New("invalid JSON")
	}
	account := normalizeExternalAccountID(env.ExternalAccountID)
	if account == "" {
		return model.IngestEvent{}, errors.New("externalAccountId is required")
	}
	var tx simpleFINTransaction
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
