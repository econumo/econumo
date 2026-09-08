package model

import (
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
)

const (
	ImportIngestStatusCreated   = "created"
	ImportIngestStatusMatched   = "matched"
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

const importCredentialFieldMax = 4096

// The card name is echoed back to the UI and stored on the mapping row, so it
// is bounded like every other user-visible name.
const importExternalNameMax = 255

// The client-side format of an encrypted credential (see importCrypto.ts).
// Rejecting anything else here is what stops a client bug from persisting the
// plaintext access URL the whole design exists to keep off the server.
const importCredentialPrefix = "v1:"

func tooLongField(key string) errs.FieldError {
	return errs.FieldError{Key: key, Message: "This value is too long.", Code: errs.CodeTooLong}
}

func checkExternalName(name string) error {
	if len(name) > importExternalNameMax {
		return errs.NewValidation("Validation failed", tooLongField("externalName"))
	}
	return nil
}

type CreateImportSourceRequest struct {
	Provider             string `json:"provider"`
	Name                 string `json:"name"`
	CredentialCiphertext string `json:"credentialCiphertext"`
}

func (r CreateImportSourceRequest) Validate() error {
	if err := requireNonBlank("provider", r.Provider, "name", r.Name); err != nil {
		return err
	}
	switch r.Provider {
	case ImportProviderAppleWallet:
		return nil
	case ImportProviderSimpleFIN:
		// a pull source is unusable without the encrypted access URL
		if err := requireNonBlank("credentialCiphertext", r.CredentialCiphertext); err != nil {
			return err
		}
		switch {
		case len(r.CredentialCiphertext) > importCredentialFieldMax:
			return errs.NewValidation("Validation failed", tooLongField("credentialCiphertext"))
		case !strings.HasPrefix(r.CredentialCiphertext, importCredentialPrefix):
			return errs.NewValidation("Validation failed", errs.FieldError{Key: "credentialCiphertext", Message: "This value is not valid.", Code: errs.CodeInvalidFormat})
		}
		return nil
	default:
		return errs.NewValidation("Validation failed", errs.FieldError{Key: "provider", Message: "This import provider is not supported.", Code: errs.CodeImportProviderUnsupported})
	}
}

type ClaimSetupTokenRequest struct {
	SetupToken string `json:"setupToken"`
}

func (r ClaimSetupTokenRequest) Validate() error { return requireNonBlank("setupToken", r.SetupToken) }

type ClaimSetupTokenResult struct {
	AccessUrl string `json:"accessUrl"`
}

type GetImportCredentialKeyResult struct {
	WrappedDataKey string `json:"wrappedDataKey"`
	Kdf            string `json:"kdf"`
	UpdatedAt      string `json:"updatedAt"`
}

type SetImportCredentialKeyRequest struct {
	WrappedDataKey string `json:"wrappedDataKey"`
	Kdf            string `json:"kdf"`
}

func (r SetImportCredentialKeyRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{{"wrappedDataKey", r.WrappedDataKey}, {"kdf", r.Kdf}} {
		switch {
		case strings.TrimSpace(f.val) == "":
			fields = append(fields, blankField(f.key))
		case len(f.val) > importCredentialFieldMax:
			fields = append(fields, tooLongField(f.key))
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type ListExternalAccountsRequest struct {
	SourceId  string `json:"sourceId"`
	AccessUrl string `json:"accessUrl"`
}

func (r ListExternalAccountsRequest) Validate() error {
	return requireNonBlank("sourceId", r.SourceId, "accessUrl", r.AccessUrl)
}

type ExternalAccountResult struct {
	ExternalAccountId string `json:"externalAccountId"`
	ExternalName      string `json:"externalName"`
	ExternalCurrency  string `json:"externalCurrency"`
	Balance           string `json:"balance"`
	OrgName           string `json:"orgName"`
	State             string `json:"state"`
	AccountId         string `json:"accountId"`
}

type ListExternalAccountsResult struct {
	Items []ExternalAccountResult `json:"items"`
}

const ImportDateLayout = "2006-01-02"

type SyncImportSourceRequest struct {
	SourceId  string `json:"sourceId"`
	AccessUrl string `json:"accessUrl"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

func (r SyncImportSourceRequest) Validate() error {
	var fields []errs.FieldError
	for _, f := range []struct{ key, val string }{{"sourceId", r.SourceId}, {"accessUrl", r.AccessUrl}, {"startDate", r.StartDate}} {
		if strings.TrimSpace(f.val) == "" {
			fields = append(fields, blankField(f.key))
		}
	}
	for _, f := range []struct{ key, val string }{{"startDate", r.StartDate}, {"endDate", r.EndDate}} {
		if strings.TrimSpace(f.val) == "" {
			continue
		}
		if _, err := time.Parse(ImportDateLayout, f.val); err != nil {
			fields = append(fields, errs.FieldError{Key: f.key, Message: "This value is not a valid datetime.", Code: errs.CodeInvalidDatetime})
		}
	}
	if len(fields) > 0 {
		return errs.NewValidation("Validation failed", fields...)
	}
	return nil
}

type SyncImportSourceResult struct {
	Run      ImportRunResult         `json:"run"`
	Accounts []ExternalAccountResult `json:"accounts"`
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
	Id                   string             `json:"id"`
	Provider             string             `json:"provider"`
	Name                 string             `json:"name"`
	Status               string             `json:"status"`
	CreatedAt            string             `json:"createdAt"`
	LastSyncedAt         string             `json:"lastSyncedAt"`
	CredentialCiphertext string             `json:"credentialCiphertext"`
	Cards                []ImportCardResult `json:"cards"`
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
	ExternalName      string `json:"externalName"`
}

func (r LinkImportAccountRequest) Validate() error {
	if err := requireNonBlank("sourceId", r.SourceId, "externalAccountId", r.ExternalAccountId, "accountId", r.AccountId); err != nil {
		return err
	}
	return checkExternalName(r.ExternalName)
}

type ImportAccountActionRequest struct {
	SourceId          string `json:"sourceId"`
	ExternalAccountId string `json:"externalAccountId"`
	ExternalName      string `json:"externalName"`
}

func (r ImportAccountActionRequest) Validate() error {
	if err := requireNonBlank("sourceId", r.SourceId, "externalAccountId", r.ExternalAccountId); err != nil {
		return err
	}
	return checkExternalName(r.ExternalName)
}

type ImportRunResult struct {
	Id                  string           `json:"id"`
	SourceId            string           `json:"sourceId"`
	Provider            string           `json:"provider"`
	Status              string           `json:"status"`
	Trigger             string           `json:"trigger"`
	ImportedCount       int              `json:"importedCount"`
	MatchedCount        int              `json:"matchedCount"`
	AmountsUpdatedCount int              `json:"amountsUpdatedCount"`
	QueuedCount         int              `json:"queuedCount"`
	SkippedCount        int              `json:"skippedCount"`
	FailedCount         int              `json:"failedCount"`
	Errors              []ImportRunError `json:"errors"`
	StartedAt           string           `json:"startedAt"`
	FinishedAt          string           `json:"finishedAt"`
}

type GetImportRunListResult struct {
	Items []ImportRunResult `json:"items"`
}

type ImportRunLinkResult struct {
	Id                    string `json:"id"`
	ExternalAccountId     string `json:"externalAccountId"`
	ExternalTransactionId string `json:"externalTransactionId"`
	TransactionId         string `json:"transactionId"`
	Status                string `json:"status"`
	ExternalPayee         string `json:"externalPayee"`
	ExternalAmount        string `json:"externalAmount"`
	ExternalCurrency      string `json:"externalCurrency"`
	ExternalPostedAt      string `json:"externalPostedAt"`
}

type GetImportRunResult struct {
	Item  ImportRunResult       `json:"item"`
	Links []ImportRunLinkResult `json:"links"`
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
