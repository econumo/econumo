package fixture

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/econumo/econumo/internal/model"
)

// ImportSource seeds one import_sources row; Provider defaults to
// "apple-wallet", Status to "active". CredentialCiphertext "" -> NULL.
type ImportSource struct {
	ID                   string
	UserID               string
	Provider             string
	Name                 string
	CredentialCiphertext string
	Status               string
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
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		id, s.UserID, provider, s.Name, nullable(s.CredentialCiphertext), status, RawTime{now}, RawTime{now})
	return id
}

// ImportTransactionLink seeds one ledger row. TransactionID "" -> NULL (a
// tombstone when Status is "linked"); Status defaults to "linked".
type ImportTransactionLink struct {
	ID                    string
	SourceID              string
	RunID                 string // "" -> NULL
	EventID               string // "" -> NULL
	ExternalAccountID     string
	ExternalTransactionID string
	TransactionID         string
	Status                string
	ExternalPayee         string
	ExternalDescription   string
	ExternalAmount        string // decimal text, e.g. "12.50000000"
	ExternalCurrency      string // "" -> NULL
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, ?)`,
		id, l.SourceID, nullable(l.RunID), nullable(l.EventID), l.ExternalAccountID, l.ExternalTransactionID, nullable(l.TransactionID), status,
		l.ExternalPayee, l.ExternalDescription, l.ExternalAmount, nullable(l.ExternalCurrency), RawTime{posted}, RawTime{b.now()})
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
		id, l.SourceID, l.ExternalAccountID, name, nullable(l.ExternalCurrency), nullable(l.AccountID), mode, RawTime{now}, RawTime{now})
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
		id, e.SourceID, e.Payload, hash, status, nullable(e.ParseError), RawTime{received})
	return id
}

// ImportRun seeds one import_runs row. Params defaults to "{}", Status to
// "completed", Errors to "[]"; Trigger is always seeded as "manual" (the
// only value stage 1/2 ever wrote).
type ImportRun struct {
	ID, UserID, SourceID, Provider, Params, Status         string
	ImportedCount, MatchedCount, SkippedCount, FailedCount int
	QueuedCount, AmountsUpdatedCount                       int
	Errors                                                 string // JSON, default "[]"
	StartedAt                                              time.Time
	FinishedAt                                             *time.Time
}

func (b *Builder) ImportRun(r ImportRun) string {
	b.t.Helper()
	id := b.orNewID(r.ID)
	if r.Params == "" {
		r.Params = "{}"
	}
	if r.Status == "" {
		r.Status = model.ImportRunStatusCompleted
	}
	if r.Errors == "" {
		r.Errors = "[]"
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = b.now()
	}
	var finishedAt any
	if r.FinishedAt != nil {
		finishedAt = RawTime{*r.FinishedAt}
	}
	b.insert(`INSERT INTO import_runs (id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, r.UserID, r.SourceID, r.Provider, r.Params, r.Status, r.ImportedCount, r.MatchedCount, r.SkippedCount, r.FailedCount,
		r.QueuedCount, r.AmountsUpdatedCount, model.ImportRunTriggerManual, r.Errors, RawTime{r.StartedAt}, finishedAt)
	return id
}

// ImportCredentialKey seeds one import_credential_keys row (unique per user).
// KDF defaults to a representative PBKDF2 config JSON.
type ImportCredentialKey struct {
	UserID, WrappedDataKey, KDF string
}

func (b *Builder) ImportCredentialKey(k ImportCredentialKey) {
	b.t.Helper()
	if k.KDF == "" {
		k.KDF = `{"alg":"PBKDF2-SHA256","salt":"c2FsdA==","iterations":600000}`
	}
	now := b.now()
	b.insert(`INSERT INTO import_credential_keys (user_id, wrapped_data_key, kdf, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		k.UserID, k.WrappedDataKey, k.KDF, RawTime{now}, RawTime{now})
}
