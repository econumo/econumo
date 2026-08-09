package model

// LabelViewRow is the read-side row shape the label ReadModel returns. It is
// declared here so the app layer does not import infra.
type LabelViewRow struct {
	ID         string
	UserID     string
	Name       string
	Icon       string
	IsArchived bool
	CreatedAt  string // already formatted "2006-01-02 15:04:05" by the repo
	UpdatedAt  string
}
