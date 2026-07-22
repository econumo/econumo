package model

import (
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// emailVerificationTTL is how long a login verification code stays valid.
const emailVerificationTTL = 10 * time.Minute

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
