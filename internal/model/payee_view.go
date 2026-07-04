package model

// PayeeViewRow is the read-side row shape the payee ReadModel returns. It is
// declared here, rather than in infra, so the app layer does not import the
// infra package (dependency points inward).
type PayeeViewRow struct {
	ID         string
	UserID     string
	Name       string
	Position   int16
	IsArchived bool
	CreatedAt  string // already formatted "2006-01-02 15:04:05" by the repo
	UpdatedAt  string
}
