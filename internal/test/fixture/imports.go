package fixture

import "time"

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
