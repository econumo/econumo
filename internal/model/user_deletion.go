package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

type DeletionReason string

const (
	DeletionReasonSelf     DeletionReason = "self_initiated"
	DeletionReasonInactive DeletionReason = "inactive"
	DeletionReasonCLI      DeletionReason = "cli"
)

const (
	AuditDeletionMarked    = "deletion_marked"
	AuditDeletionCancelled = "deletion_cancelled"
	AuditDeleted           = "deleted"
	AuditLifetimeRegranted = "lifetime_regranted"
)

const (
	CancelCauseSignIn           = "sign_in"
	CancelCauseActiveConnection = "active_connection"
	CancelCauseInactivityOff    = "inactivity_off"
)

type DeletionState struct {
	MarkedAt     *time.Time
	Reason       DeletionReason
	LastActiveAt *time.Time
	CreatedAt    time.Time
}

type UserAuditEvent struct {
	ID        vo.Id
	UserID    vo.Id
	Event     string
	EmailHash *string
	// RefID is set on lifetime_regranted only: the deleted event it consumes,
	// unique so one entitlement is regranted at most once.
	RefID     *vo.Id
	Details   string    // JSON object
	CreatedAt time.Time // truncated to the second on write
}

type MarkedUser struct {
	ID       vo.Id
	Email    string
	Reason   DeletionReason
	MarkedAt time.Time
}

type InactiveCandidate struct {
	ID       vo.Id
	Email    string
	Name     string
	Language string
}

// DeletionRequest is a pending self-service deletion code. Email is users.email
// as stored when the code was sent.
type DeletionRequest struct {
	ID        vo.Id
	UserID    vo.Id
	Email     string
	Code      string
	CreatedAt time.Time
	ExpiredAt time.Time
}

const DeletionRequestTTL = 10 * time.Minute

func NewDeletionRequest(id, userID vo.Id, storedEmail, codeHash string, now time.Time) *DeletionRequest {
	return &DeletionRequest{
		ID:        id,
		UserID:    userID,
		Email:     storedEmail,
		Code:      codeHash,
		CreatedAt: now,
		ExpiredAt: now.Add(DeletionRequestTTL),
	}
}

func (r *DeletionRequest) IsExpired(now time.Time) bool { return now.After(r.ExpiredAt) }

// EmailHash is lowercase hex SHA-256 of NormalizeEmail(address).
func EmailHash(address string) string {
	sum := sha256.Sum256([]byte(NormalizeEmail(address)))
	return hex.EncodeToString(sum[:])
}
