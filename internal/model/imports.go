package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	ImportProviderSimpleFIN   = "simplefin"
	ImportProviderCSV         = "csv"
	ImportProviderAppleWallet = "apple-wallet"

	ImportSourceStatusActive = "active"

	ImportEventStatusProcessed = "processed"
	ImportEventStatusFailed    = "failed"
	ImportEventStatusDiscarded = "discarded"

	ImportLinkStatusLinked  = "linked"
	ImportLinkStatusQueued  = "queued"
	ImportLinkStatusSkipped = "skipped"

	ImportRunStatusRunning   = "running"
	ImportRunStatusCompleted = "completed"
	ImportRunStatusFailed    = "failed"
	ImportRunStatusPartial   = "partial"

	ImportAccountLinkModeImport = "import"
	ImportAccountLinkModeIgnore = "ignore"
)

// Push providers deliver one event per tap and see the transaction before
// the bank posts it; pull providers see the posted record. The matcher
// treats the two differently (a push link is the "tip" a later bank record
// adopts).
func ImportProviderIsPush(provider string) bool {
	return provider == ImportProviderAppleWallet
}

type ImportSource struct {
	ID                   vo.Id
	UserID               vo.Id
	Provider             string
	Name                 string
	CredentialCiphertext *string
	Status               string
	LastSyncedAt         *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ImportAccountLink is the user's decision for one external account (a
// card name for Apple Wallet): import into AccountID, or ignore. AccountID
// is nil only in ignore mode (schema CHECK). ExternalCurrency is the
// currency the provider reported for the card, recorded when the link is
// created so a currency drift can be refused later.
type ImportAccountLink struct {
	ID                vo.Id
	SourceID          vo.Id
	ExternalAccountID string
	ExternalName      string
	ExternalCurrency  *string
	AccountID         *vo.Id
	Mode              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ImportEvent struct {
	ID          vo.Id
	SourceID    vo.Id
	RunID       *vo.Id
	Payload     string
	PayloadHash string
	Status      string
	ParseError  *string
	ReceivedAt  time.Time
}

type ImportRun struct {
	ID            vo.Id
	UserID        vo.Id
	SourceID      vo.Id
	Provider      string
	Params        string
	Status        string
	ImportedCount int
	MatchedCount  int
	SkippedCount  int
	FailedCount   int
	StartedAt     time.Time
	FinishedAt    *time.Time
}

// ImportTransactionLink is one ledger row: what an external transaction id
// became. A linked row whose TransactionID is nil is a tombstone — the user
// deleted the transaction, and the external id must never be re-imported.
type ImportTransactionLink struct {
	ID                    vo.Id
	SourceID              vo.Id
	RunID                 *vo.Id
	EventID               *vo.Id
	ExternalAccountID     string
	ExternalTransactionID string
	TransactionID         *vo.Id
	Status                string
	ExternalPayee         string
	ExternalDescription   string
	ExternalAmount        string
	ExternalCurrency      *string
	ExternalPostedAt      time.Time
	AppliedCategoryID     *vo.Id
	AppliedPayeeID        *vo.Id
	AppliedTagID          *vo.Id
	AppliedRuleID         *vo.Id
	ImportedAt            time.Time
}

// A queued row is not "seen": the user has not decided yet, so the same
// external id may be re-offered by a later sync.
func (l ImportTransactionLink) IsSeen() bool { return l.Status != ImportLinkStatusQueued }

func (l ImportTransactionLink) IsTombstone() bool {
	return l.Status == ImportLinkStatusLinked && l.TransactionID == nil
}

// IngestEvent is one normalized external transaction, the matcher's input.
// Amount is the absolute value as a decimal string; Type carries the sign.
type IngestEvent struct {
	ExternalAccountID     string
	ExternalTransactionID string
	Type                  TransactionType
	Amount                string
	Currency              string
	PostedAt              time.Time
	Payee                 string
	Description           string
}

// ImportCandidate is an existing transaction the matcher may adopt, with the
// ledger rows already pointing at it.
type ImportCandidate struct {
	TransactionID vo.Id
	Type          TransactionType
	Amount        string
	SpentAt       time.Time
	Links         []ImportCandidateLink
}

type ImportCandidateLink struct {
	SourceID       vo.Id
	Provider       string
	ExternalAmount string
	ExternalPayee  string
}
