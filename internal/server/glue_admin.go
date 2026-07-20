// Adapter bridging the admin feature to the user feature. It lives here
// because features must not import each other; only the composition root may
// join them. The connection side needs no adapter: *connectionrepo.Repo already
// satisfies admin.ConnectionLookup (asserted below) and is wired directly.
package server

import (
	"context"
	"time"

	appadmin "github.com/econumo/econumo/internal/admin"
	connectionrepo "github.com/econumo/econumo/internal/connection/repo"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

var _ appadmin.ConnectionLookup = (*connectionrepo.Repo)(nil)

type AdminUserAccess struct{ users *appuser.Service }

var _ appadmin.UserLookup = (*AdminUserAccess)(nil)

func NewAdminUserAccess(users *appuser.Service) *AdminUserAccess {
	return &AdminUserAccess{users: users}
}

func (a *AdminUserAccess) GetUser(ctx context.Context, id vo.Id) (appadmin.UserRecord, error) {
	u, email, err := a.users.AdminUserByID(ctx, id)
	if err != nil {
		return appadmin.UserRecord{}, err
	}
	return record(u, email), nil
}

func (a *AdminUserAccess) SetAccess(ctx context.Context, id vo.Id, level model.AccessLevel, until *time.Time) (appadmin.UserRecord, error) {
	u, email, err := a.users.AdminSetAccessByID(ctx, id, level, until)
	if err != nil {
		return appadmin.UserRecord{}, err
	}
	return record(u, email), nil
}

func record(u *model.User, email string) appadmin.UserRecord {
	return appadmin.UserRecord{
		ID:          u.ID.String(),
		Name:        u.Name,
		Email:       email,
		AccessLevel: u.AccessLevel,
		AccessUntil: u.AccessUntil,
	}
}
