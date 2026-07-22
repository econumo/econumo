package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// emailVerificationTTL is how long a login verification code stays valid.
const emailVerificationTTL = 10 * time.Minute

// EmailVerificationResendGap is the minimum wait between two code emails for
// the same user. It bounds mailbox spam (and the cost of sending) between the
// coarser per-window attempt cap, and is the value the API reports back so a
// client's countdown survives a reload or a second tab.
const EmailVerificationResendGap = 60 * time.Second

// EmailVerification is a pending email-verification code for a user, mirroring
// PasswordRequest: the code is generated (and hashed) by the application layer
// and passed in; the request is replaced, never updated.
type EmailVerification struct {
	ID        vo.Id
	UserID    vo.Id
	Code      string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiredAt time.Time
}

// NewEmailVerification builds a fresh verification expiring 10 minutes from now.
func NewEmailVerification(id, userID vo.Id, code string, now time.Time) *EmailVerification {
	return &EmailVerification{
		ID:        id,
		UserID:    userID,
		Code:      code,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiredAt: now.Add(emailVerificationTTL),
	}
}

// IsExpired reports whether the code is no longer valid at the given time.
func (v *EmailVerification) IsExpired(now time.Time) bool { return now.After(v.ExpiredAt) }

// RetryAfter reports how long the caller must wait before another code may be
// sent, rounded up to whole seconds so a client never retries a fraction early.
// Zero means a resend is allowed now. CreatedAt is the last send: a resend
// replaces the row rather than updating it.
func (v *EmailVerification) RetryAfter(now time.Time) time.Duration {
	remaining := v.CreatedAt.Add(EmailVerificationResendGap).Sub(now)
	if remaining <= 0 {
		return 0
	}
	// A row stamped in the future (clock skew between writer and reader, or a
	// clock stepped backwards) must never strand the user behind a countdown
	// longer than the gap they were promised.
	if remaining > EmailVerificationResendGap {
		return EmailVerificationResendGap
	}
	if rem := remaining % time.Second; rem > 0 {
		remaining += time.Second - rem
	}
	return remaining
}
