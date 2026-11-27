package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
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

// PendingDeletion is a marked user as user:list-pending-deletion shows it.
// Email is decoded; WipeAt is nil when automatic wiping is off.
type PendingDeletion struct {
	ID       string
	Email    string
	Reason   string
	MarkedAt time.Time
	WipeAt   *time.Time
}

type InactiveCandidate struct {
	ID               vo.Id
	Email            string
	Name             string
	Language         string
	LastActiveAt     *time.Time
	CreatedAt        time.Time
	InactivityOption *string // the stored inactivity_months value, nil when absent
}

func (c InactiveCandidate) InactivityMonths(serverMonths int) int {
	return inactivityMonths(c.InactivityOption, serverMonths)
}

// Stamp is the moment the inactivity period is measured from.
func (c InactiveCandidate) Stamp() time.Time {
	if c.LastActiveAt != nil {
		return *c.LastActiveAt
	}
	return c.CreatedAt
}

// DaysPerInactivityMonth: periods are counted in 30-day months, never calendar
// months, so a period is the same length whenever it starts.
const DaysPerInactivityMonth = 30

// inactivityMonths owns the per-user period rule. A stored value that is not
// among today's choices stays valid (the 36 months seeded for lifetime
// accounts, or a choice made under an older server setting); an unparsable or
// non-positive one reads as absent.
func inactivityMonths(stored *string, serverMonths int) int {
	if stored != nil {
		if n, err := strconv.Atoi(*stored); err == nil && n > 0 {
			return n
		}
	}
	return serverMonths
}

// CurrentInactivity is what the current user carries: the effective period and
// today's choices, 0 and an empty list while the server deletes nobody.
func CurrentInactivity(stored *string, serverMonths int) (months int, choices []int) {
	if serverMonths <= 0 {
		return 0, []int{}
	}
	return inactivityMonths(stored, serverMonths), InactivityChoices(serverMonths)
}

// InactivityChoices are the periods a user may choose under the server setting:
// half of it (left out when that is 0), the setting itself and its double.
func InactivityChoices(serverMonths int) []int {
	out := []int{}
	if serverMonths <= 0 {
		return out
	}
	if half := serverMonths / 2; half > 0 {
		out = append(out, half)
	}
	return append(out, serverMonths, serverMonths*2)
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
