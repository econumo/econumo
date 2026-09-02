package fixture

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// ImportSource seeds one import_sources row; Provider defaults to
// "apple-wallet", Status to "active".
type ImportSource struct {
	ID       string
	UserID   string
	Provider string
	Name     string
	Status   string
}

func (b *Builder) ImportSource(s ImportSource) string {
	b.t.Helper()
	id := b.orNewID(s.ID)
	provider := s.Provider
	if provider == "" {
		provider = "apple-wallet"
	}
	status := s.Status
	if status == "" {
		status = "active"
	}
	now := b.now()
	b.insert(`INSERT INTO import_sources (id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULL, ?, NULL, ?, ?)`,
		id, s.UserID, provider, s.Name, status, now, now)
	return id
}

// ImportTransactionLink seeds one ledger row. TransactionID "" -> NULL (a
// tombstone when Status is "linked"); Status defaults to "linked".
type ImportTransactionLink struct {
	ID                    string
	SourceID              string
	ExternalAccountID     string
	ExternalTransactionID string
	TransactionID         string
	Status                string
	ExternalPayee         string
	ExternalDescription   string
	ExternalAmount        string // decimal text, e.g. "12.50000000"
	ExternalPostedAt      time.Time
}

func (b *Builder) ImportTransactionLink(l ImportTransactionLink) string {
	b.t.Helper()
	id := b.orNewID(l.ID)
	status := l.Status
	if status == "" {
		status = "linked"
	}
	if l.ExternalAmount == "" {
		l.ExternalAmount = "0.00000000"
	}
	posted := l.ExternalPostedAt
	if posted.IsZero() {
		posted = b.now()
	}
	b.insert(`INSERT INTO import_transaction_links (id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at)
		VALUES (?, ?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, NULL, ?, NULL, NULL, NULL, NULL, ?)`,
		id, l.SourceID, l.ExternalAccountID, l.ExternalTransactionID, nullable(l.TransactionID), status,
		l.ExternalPayee, l.ExternalDescription, l.ExternalAmount, posted, b.now())
	return id
}

// ImportAccountLink seeds one import_account_links row. Mode defaults to
// "import"; ExternalCurrency/AccountID "" -> NULL (AccountID must be set
// unless Mode is "ignore" — schema CHECK).
type ImportAccountLink struct {
	ID                string
	SourceID          string
	ExternalAccountID string
	ExternalName      string
	ExternalCurrency  string
	AccountID         string
	Mode              string
}

func (b *Builder) ImportAccountLink(l ImportAccountLink) string {
	b.t.Helper()
	id := b.orNewID(l.ID)
	mode := l.Mode
	if mode == "" {
		mode = "import"
	}
	name := l.ExternalName
	if name == "" {
		name = l.ExternalAccountID
	}
	now := b.now()
	b.insert(`INSERT INTO import_account_links (id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, l.SourceID, l.ExternalAccountID, name, nullable(l.ExternalCurrency), nullable(l.AccountID), mode, now, now)
	return id
}

// ImportEvent seeds one inbox row. Status defaults to "processed";
// PayloadHash "" -> sha256(Payload); ParseError "" -> NULL; ReceivedAt zero -> now.
type ImportEvent struct {
	ID          string
	SourceID    string
	Payload     string
	PayloadHash string
	Status      string
	ParseError  string
	ReceivedAt  time.Time
}

func (b *Builder) ImportEvent(e ImportEvent) string {
	b.t.Helper()
	id := b.orNewID(e.ID)
	status := e.Status
	if status == "" {
		status = "processed"
	}
	hash := e.PayloadHash
	if hash == "" {
		sum := sha256.Sum256([]byte(e.Payload))
		hash = hex.EncodeToString(sum[:])
	}
	received := e.ReceivedAt
	if received.IsZero() {
		received = b.now()
	}
	b.insert(`INSERT INTO import_events (id, source_id, run_id, payload, payload_hash, status, parse_error, received_at)
		VALUES (?, ?, NULL, ?, ?, ?, ?, ?)`,
		id, e.SourceID, e.Payload, hash, status, nullable(e.ParseError), received)
	return id
}
