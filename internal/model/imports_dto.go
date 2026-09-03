package model

import (
	"strings"

	"github.com/econumo/econumo/internal/shared/errs"
)

const (
	ImportIngestStatusCreated   = "created"
	ImportIngestStatusQueued    = "queued"
	ImportIngestStatusSkipped   = "skipped"
	ImportIngestStatusDuplicate = "duplicate"
	ImportIngestStatusFailed    = "failed"

	ImportCardStateMapped   = "mapped"
	ImportCardStateIgnored  = "ignored"
	ImportCardStateUnmapped = "unmapped"

	ImportQueueReasonUnmapped       = "unmapped"
	ImportQueueReasonAccountDeleted = "account_deleted"
	ImportQueueReasonNoRate         = "no_rate"
)

func blankField(key string) errs.FieldError {
	return errs.FieldError{Key: key, Message: "This value should not be blank.", Code: errs.CodeIsBlank}
}

func requireNonBlank(pairs ...string) error {
	var fields []errs.FieldError
	for i := 0; i+1 < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) == "" {
			fields = append(fields, blankField(pairs[i]))
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type CreateImportSourceRequest struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

func (r CreateImportSourceRequest) Validate() error {
	if err := requireNonBlank("provider", r.Provider, "name", r.Name); err != nil {
		return err
	}
	if r.Provider != ImportProviderAppleWallet {
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "provider", Message: "This import provider is not supported.", Code: errs.CodeImportProviderUnsupported})
	}
	return nil
}

type ImportCardResult struct {
	ExternalAccountId string `json:"externalAccountId"`
	ExternalName      string `json:"externalName"`
	ExternalCurrency  string `json:"externalCurrency"`
	State             string `json:"state"`
	AccountId         string `json:"accountId"`
	QueuedCount       int    `json:"queuedCount"`
	TapCount          int    `json:"tapCount"`
	LastSeenAt        string `json:"lastSeenAt"`
}

type ImportSourceResult struct {
	Id        string             `json:"id"`
	Provider  string             `json:"provider"`
	Name      string             `json:"name"`
	Status    string             `json:"status"`
	CreatedAt string             `json:"createdAt"`
	Cards     []ImportCardResult `json:"cards"`
}

type CreateImportSourceResult struct {
	Item ImportSourceResult `json:"item"`
}

type GetImportSourceListResult struct {
	Items []ImportSourceResult `json:"items"`
}

type DeleteImportSourceRequest struct {
	Id string `json:"id"`
}

func (r DeleteImportSourceRequest) Validate() error { return requireNonBlank("id", r.Id) }

type LinkImportAccountRequest struct {
	SourceId          string `json:"sourceId"`
	ExternalAccountId string `json:"externalAccountId"`
	AccountId         string `json:"accountId"`
}

func (r LinkImportAccountRequest) Validate() error {
	return requireNonBlank("sourceId", r.SourceId, "externalAccountId", r.ExternalAccountId, "accountId", r.AccountId)
}

type ImportAccountActionRequest struct {
	SourceId          string `json:"sourceId"`
	ExternalAccountId string `json:"externalAccountId"`
}

func (r ImportAccountActionRequest) Validate() error {
	return requireNonBlank("sourceId", r.SourceId, "externalAccountId", r.ExternalAccountId)
}

type ImportRunResult struct {
	Id            string `json:"id"`
	Status        string `json:"status"`
	ImportedCount int    `json:"importedCount"`
	MatchedCount  int    `json:"matchedCount"`
	SkippedCount  int    `json:"skippedCount"`
	FailedCount   int    `json:"failedCount"`
}

// UpdateImportAccountResult is shared by link/ignore/unlink: the source with
// its refreshed card list, plus the conversion run link-account triggered
// (null for ignore/unlink).
type UpdateImportAccountResult struct {
	Item ImportSourceResult `json:"item"`
	Run  *ImportRunResult   `json:"run"`
}

type IngestEventResult struct {
	Status  string `json:"status"`
	EventId string `json:"eventId"`
}

type ImportQueuedEventResult struct {
	LinkId            string `json:"linkId"`
	SourceId          string `json:"sourceId"`
	ExternalAccountId string `json:"externalAccountId"`
	AccountId         string `json:"accountId"`
	Payee             string `json:"payee"`
	Amount            string `json:"amount"`
	Currency          string `json:"currency"`
	Type              string `json:"type"`
	PostedAt          string `json:"postedAt"`
	Reason            string `json:"reason"`
}

// Payload is the raw body as received (the queue page's "needs attention"
// section shows it so the user can see what the shortcut actually sent).
type ImportFailedEventResult struct {
	EventId    string `json:"eventId"`
	SourceId   string `json:"sourceId"`
	ReceivedAt string `json:"receivedAt"`
	Error      string `json:"error"`
	Payload    string `json:"payload"`
}

type GetImportQueueResult struct {
	Queued  []ImportQueuedEventResult `json:"queued"`
	Skipped []ImportQueuedEventResult `json:"skipped"`
	Failed  []ImportFailedEventResult `json:"failed"`
}

type ImportQueuedEventRequest struct {
	LinkId      string                   `json:"linkId"`
	Transaction CreateTransactionRequest `json:"transaction"`
}

func (r ImportQueuedEventRequest) Validate() error {
	if err := requireNonBlank("linkId", r.LinkId); err != nil {
		return err
	}
	return r.Transaction.Validate()
}

type ImportLinkActionRequest struct {
	LinkId string `json:"linkId"`
}

func (r ImportLinkActionRequest) Validate() error { return requireNonBlank("linkId", r.LinkId) }

type RetryImportEventRequest struct {
	EventId string `json:"eventId"`
}

func (r RetryImportEventRequest) Validate() error { return requireNonBlank("eventId", r.EventId) }

type DiscardImportEventRequest struct {
	EventId string `json:"eventId"`
}

func (r DiscardImportEventRequest) Validate() error { return requireNonBlank("eventId", r.EventId) }

type TransactionImportListRequest struct {
	TransactionId string `json:"transactionId"`
}

func (r TransactionImportListRequest) Validate() error {
	return requireNonBlank("transactionId", r.TransactionId)
}

type TransactionImportLinkResult struct {
	Id                    string `json:"id"`
	SourceId              string `json:"sourceId"`
	Provider              string `json:"provider"`
	SourceName            string `json:"sourceName"`
	ExternalAccountId     string `json:"externalAccountId"`
	ExternalTransactionId string `json:"externalTransactionId"`
	ExternalPayee         string `json:"externalPayee"`
	ExternalAmount        string `json:"externalAmount"`
	ExternalCurrency      string `json:"externalCurrency"`
	ExternalPostedAt      string `json:"externalPostedAt"`
	Status                string `json:"status"`
	ImportedAt            string `json:"importedAt"`
}

type GetTransactionImportListResult struct {
	Items []TransactionImportLinkResult `json:"items"`
}
