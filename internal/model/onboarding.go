// Onboarding: the account facts the background savings classification reads.
package model

import "time"

// OnboardingAccount is one account as the onboarding classifier sees it: its
// owner and deleted flag gate it, and only the name and the account's own
// currency code are sent to the AI provider. The step writes also read its
// currency id (visible currencies) and creation time (replay matching).
type OnboardingAccount struct {
	ID           string
	OwnerID      string
	Name         string
	CurrencyCode string
	CurrencyID   string
	IsDeleted    bool
	CreatedAt    time.Time
}
