package connection

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// connectionCodeLength is the fixed length of a connection invite code.
const connectionCodeLength = 5

// inviteLifetime is how long a generated code stays valid.
const inviteLifetime = 5 * time.Minute

// ConnectionCode is the short invite code a user shares to connect. It is a
// 5-character string of hex characters with randomized per-character case. The
// value is validated to be exactly 5 characters on construction.
type ConnectionCode struct {
	value string
}

// NewConnectionCode validates and wraps an incoming code (the accept-invite
// path), rejecting anything not exactly connectionCodeLength chars.
func NewConnectionCode(value string) (ConnectionCode, error) {
	if len([]rune(value)) != connectionCodeLength {
		return ConnectionCode{}, errs.NewValidation("ConnectionCode is incorrect")
	}
	return ConnectionCode{value: value}, nil
}

// GenerateConnectionCode mints a fresh code: take the first 5 hex chars of a
// random source, then randomize each character's case. Only the length and
// uniqueness matter; the exact bytes are never compared.
func GenerateConnectionCode() ConnectionCode {
	var b [8]byte
	_, _ = rand.Read(b[:])
	hexStr := hex.EncodeToString(b[:]) // 16 lowercase hex chars
	code := []byte(hexStr[:connectionCodeLength])
	// Randomize per-character case for letters.
	var caseBits [1]byte
	_, _ = rand.Read(caseBits[:])
	for i := range code {
		c := code[i]
		if c >= 'a' && c <= 'f' && (caseBits[0]>>(uint(i)%8))&1 == 1 {
			code[i] = c - 'a' + 'A'
		}
	}
	return ConnectionCode{value: string(code)}
}

// Value returns the raw code string.
func (c ConnectionCode) Value() string { return c.value }

// IsZero reports whether the code is unset (cleared).
func (c ConnectionCode) IsZero() bool { return c.value == "" }

// ConnectionInvite is a user's current outstanding invite: a code + expiry, keyed
// by the inviting user's id (one row per user). A nil/empty code means no active
// invite.
type ConnectionInvite struct {
	userID    vo.Id
	code      ConnectionCode
	expiredAt *time.Time
}

// NewConnectionInvite creates an empty invite for a user (no code yet) — the
// state before GenerateNewCode is called.
func NewConnectionInvite(userID vo.Id) *ConnectionInvite {
	return &ConnectionInvite{userID: userID}
}

// InviteFromState rebuilds an invite from storage. code may be "" and
// expiredAt may be nil (a cleared invite row).
func InviteFromState(userID vo.Id, code string, expiredAt *time.Time) *ConnectionInvite {
	inv := &ConnectionInvite{userID: userID, expiredAt: expiredAt}
	if code != "" {
		inv.code = ConnectionCode{value: code}
	}
	return inv
}

// GenerateNewCode assigns a fresh code and sets the expiry to now+lifetime.
func (i *ConnectionInvite) GenerateNewCode(now time.Time) {
	i.code = GenerateConnectionCode()
	exp := now.Add(inviteLifetime)
	i.expiredAt = &exp
}

// ClearCode removes the code + expiry (after the invite is consumed or deleted).
func (i *ConnectionInvite) ClearCode() {
	i.code = ConnectionCode{}
	i.expiredAt = nil
}

// IsExpired reports whether the invite's expiry is before now. A cleared invite
// (no expiry) is treated as expired.
func (i *ConnectionInvite) IsExpired(now time.Time) bool {
	if i.expiredAt == nil {
		return true
	}
	return i.expiredAt.Before(now)
}

func (i *ConnectionInvite) UserId() vo.Id         { return i.userID }
func (i *ConnectionInvite) Code() ConnectionCode  { return i.code }
func (i *ConnectionInvite) ExpiredAt() *time.Time { return i.expiredAt }
