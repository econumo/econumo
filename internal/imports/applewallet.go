package imports

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// appleWalletPayload is what the "Econumo Wallet" shortcut POSTs. amount is
// untyped because Shortcuts serializes numbers as numbers and text as text
// depending on how the automation was built.
type appleWalletPayload struct {
	Account    string `json:"account"`
	Payee      string `json:"payee"`
	Amount     any    `json:"amount"`
	Currency   string `json:"currency"`
	OccurredAt string `json:"occurredAt"`
	Type       string `json:"type"`
	EventID    string `json:"eventId"`
}

var (
	currencyCodeRe = regexp.MustCompile(`^[A-Z]{3}$`)
	plainDecimalRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
)

func ParseAppleWalletEvent(payload []byte, receivedAt time.Time) (model.IngestEvent, error) {
	var p appleWalletPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return model.IngestEvent{}, errors.New("invalid JSON")
	}
	account := normalizeExternalAccountID(p.Account)
	if account == "" {
		return model.IngestEvent{}, errors.New("account is required")
	}
	amount, err := normalizeAmount(amountText(p.Amount))
	if err != nil {
		return model.IngestEvent{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(p.Currency))
	if !currencyCodeRe.MatchString(currency) {
		return model.IngestEvent{}, errors.New("currency must be a 3-letter code")
	}
	var typ model.TransactionType
	switch strings.ToLower(strings.TrimSpace(p.Type)) {
	case "", "expense":
		typ = model.TransactionTypeExpense
	case "income":
		typ = model.TransactionTypeIncome
	default:
		return model.IngestEvent{}, errors.New("type must be expense or income")
	}
	// The shortcut sends RFC3339 with the device offset; anything else (or
	// nothing) is replaced by the server's receipt time — a tap is delivered
	// within seconds, so that is close enough to be useful.
	postedAt := receivedAt
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(p.OccurredAt)); err == nil {
		postedAt = t
	}
	payee := strings.TrimSpace(p.Payee)
	externalID := strings.TrimSpace(p.EventID)
	if externalID == "" {
		// Built from the PARSED values so formatting noise between two
		// deliveries of the same tap cannot fork the id. The account name is
		// lowercased here too: card identity is case-insensitive, and an
		// id-less tap re-delivered with different card-name casing must hash
		// to the same id.
		sum := sha256.Sum256([]byte(strings.ToLower(account) + "|" + postedAt.UTC().Format(time.RFC3339) + "|" + amount + "|" + payee))
		externalID = hex.EncodeToString(sum[:])
	}
	return model.IngestEvent{
		ExternalAccountID: account, ExternalTransactionID: externalID, Type: typ,
		Amount: amount, Currency: currency, PostedAt: postedAt, Payee: payee,
	}, nil
}

func amountText(v any) string {
	switch a := v.(type) {
	case string:
		return a
	case float64:
		return strconv.FormatFloat(a, 'f', -1, 64)
	default:
		return ""
	}
}

// normalizeAmount accepts what a phone produces — currency symbols, spaces,
// either separator, a sign — and returns the absolute value as plain decimal
// text. The LAST separator is the decimal point ("1.234,56" -> 1234.56).
func normalizeAmount(raw string) (string, error) {
	var b strings.Builder
	for _, r := range raw {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if last := strings.LastIndexAny(s, ".,"); last >= 0 {
		intPart := strings.NewReplacer(".", "", ",", "").Replace(s[:last])
		frac := s[last+1:]
		if frac == "" {
			s = intPart
		} else {
			s = intPart + "." + frac
		}
	}
	if strings.HasPrefix(s, ".") {
		s = "0" + s
	}
	if !plainDecimalRe.MatchString(s) {
		return "", errors.New("amount must be a positive number")
	}
	d := vo.NewDecimal(s)
	if d.IsZero() {
		return "", errors.New("amount must be a positive number")
	}
	return d.String(), nil
}

func normalizeExternalAccountID(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
