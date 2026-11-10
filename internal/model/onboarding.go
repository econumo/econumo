// Onboarding: the account facts the background savings classification reads.
package model

// OnboardingAccount is one account as the onboarding classifier sees it: its
// owner and deleted flag gate it, and only the name and the account's own
// currency code are sent to the AI provider.
type OnboardingAccount struct {
	ID           string
	OwnerID      string
	Name         string
	CurrencyCode string
	IsDeleted    bool
}
