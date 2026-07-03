package user

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the user aggregate's persistence port; the application service
// depends only on this interface. Methods that look up a missing user return an
// *errs.NotFoundError so the HTTP layer maps it consistently. Save upserts the
// user row and all of its option rows in one call.
type Repository interface {
	// NextIdentity allocates a fresh aggregate id (no DB round-trip).
	NextIdentity() vo.Id

	// GetByID loads a user (with options) by id. Missing -> *errs.NotFoundError.
	GetByID(ctx context.Context, id vo.Id) (*model.User, error)

	// GetByIdentifier loads a user (with options) by the md5 identifier used for
	// authentication. Missing -> *errs.NotFoundError.
	GetByIdentifier(ctx context.Context, identifier string) (*model.User, error)

	// ExistsByIdentifier reports whether a user with the identifier exists. Used
	// by registration to detect a duplicate without loading the row.
	ExistsByIdentifier(ctx context.Context, identifier string) (bool, error)

	// Save upserts the user row and its options.
	Save(ctx context.Context, u *model.User) error

	// ListIDs returns all user ids (for the optional connect-users flow on
	// registration).
	ListIDs(ctx context.Context) ([]vo.Id, error)

	// GetOptions loads just the option rows for a user (used by get-option-list,
	// which does not need the full aggregate).
	GetOptions(ctx context.Context, userID vo.Id) ([]model.UserOption, error)
}

// PasswordRequests persists password-reset codes (users_password_requests) for
// the remind/reset flow. The infra passwordrequestrepo implements it.
type PasswordRequests interface {
	// DeleteByUser removes all of a user's pending reset codes.
	DeleteByUser(ctx context.Context, userID vo.Id) error
	// Save inserts a new reset request.
	Save(ctx context.Context, pr *model.PasswordRequest) error
	// GetByUserAndCode loads a user's request matching code (NotFound if absent).
	GetByUserAndCode(ctx context.Context, userID vo.Id, code string) (*model.PasswordRequest, error)
	// Delete removes a request by id.
	Delete(ctx context.Context, id vo.Id) error
}
