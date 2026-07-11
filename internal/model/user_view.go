package model

// UserViewRow / OptionViewRow are the user read-side row shapes the
// user.ReadModel port returns. They are declared here, in model, rather than
// in infra, so the app layer does not import the infra package (dependency
// points inward).
type UserViewRow struct {
	ID     string
	Email  string
	Name   string
	Avatar string
}

type OptionViewRow struct {
	Name  string
	Value *string
}
