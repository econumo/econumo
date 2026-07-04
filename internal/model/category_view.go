package model

// CategoryViewRow is the read-side row shape the category ReadModel returns.
// It is declared here, rather than in infra, so the app layer does not import
// the infra package (dependency points inward). Type is the raw SMALLINT;
// IsArchived the raw bool — the conversion to the wire shapes happens in
// toViewResult.
type CategoryViewRow struct {
	ID         string
	UserID     string
	Name       string
	Position   int16
	Type       int16
	Icon       string
	IsArchived bool
	CreatedAt  string // already formatted "2006-01-02 15:04:05" by the repo
	UpdatedAt  string
}
