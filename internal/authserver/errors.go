package authserver

// OAuthError is a protocol error for the public endpoints, rendered as an
// RFC 6749 JSON error rather than the API envelope.
type OAuthError struct {
	Status            int
	Code, Description string
}

func (e *OAuthError) Error() string { return e.Code + ": " + e.Description }
