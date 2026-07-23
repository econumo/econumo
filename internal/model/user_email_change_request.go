package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// emailChangeTTL is how long a pending change-email code stays valid.
const emailChangeTTL = 10 * time.Minute

// EmailChangeRequest is a pending email change for a user: the proposed new
// email plus the emailed code (hashed by the app layer). One outstanding row
// per user; the request is replaced, never updated.
type EmailChangeRequest struct {
	ID        vo.Id
	UserID    vo.Id
	NewEmail  string
	Code      string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiredAt time.Time
}

// NewEmailChangeRequest builds a fresh pending change expiring 10 minutes from now.
func NewEmailChangeRequest(id, userID vo.Id, newEmail, code string, now time.Time) *EmailChangeRequest {
	return &EmailChangeRequest{
		ID:        id,
		UserID:    userID,
		NewEmail:  newEmail,
		Code:      code,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiredAt: now.Add(emailChangeTTL),
	}
}

// IsExpired reports whether the code is no longer valid at the given time.
func (r *EmailChangeRequest) IsExpired(now time.Time) bool { return now.After(r.ExpiredAt) }

// RetryAfter reports how long until another code may be sent (see
// EmailVerification.RetryAfter — same semantics, same resend gap).
func (r *EmailChangeRequest) RetryAfter(now time.Time) time.Duration {
	remaining := r.CreatedAt.Add(EmailVerificationResendGap).Sub(now)
	if remaining <= 0 {
		return 0
	}
	if remaining > EmailVerificationResendGap {
		return EmailVerificationResendGap
	}
	if rem := remaining % time.Second; rem > 0 {
		remaining += time.Second - rem
	}
	return remaining
}
