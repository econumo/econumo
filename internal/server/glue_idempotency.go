package server

import "github.com/econumo/econumo/internal/web/middleware"

// IdempotencyStore implements middleware.IdempotencyProvider on the wired
// authenticator decorator.
func (a *timezoneTrackingAuthenticator) IdempotencyStore() middleware.IdempotencyStore {
	return a.idempotency
}
