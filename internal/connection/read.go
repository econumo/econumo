package connection

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetConnectionList returns the requesting user's connections, each with the
// accounts shared between that connected user and the requester. For every
// connected user, it gathers the received-access grants (accounts shared TO me)
// and issued-access grants (grants on accounts I own), keeping those whose
// account is owned by either the connected user or me, deduplicated by account id
// (last write wins), in account-id discovery order.
func (s *Service) GetConnectionList(ctx context.Context, userID vo.Id) (*model.GetConnectionListResult, error) {
	received, err := s.access.ListReceived(ctx, userID)
	if err != nil {
		return nil, err
	}
	issued, err := s.access.ListIssued(ctx, userID)
	if err != nil {
		return nil, err
	}
	connected, err := s.access.ConnectedUserIDs(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Resolve the owner of every account referenced by a grant once.
	owners := map[string]vo.Id{}
	resolveOwner := func(accountID vo.Id) (vo.Id, error) {
		key := accountID.String()
		if o, ok := owners[key]; ok {
			return o, nil
		}
		o, oerr := s.access.AccountOwner(ctx, accountID)
		if oerr != nil {
			return vo.Id{}, oerr
		}
		owners[key] = o
		return o, nil
	}
	for _, a := range append(append([]*model.AccountAccess{}, received...), issued...) {
		if _, oerr := resolveOwner(a.AccountID); oerr != nil {
			return nil, oerr
		}
	}

	result := &model.GetConnectionListResult{Items: []model.ConnectionResult{}}
	for _, cu := range connected {
		owner, oerr := s.users.GetOwner(ctx, cu.String())
		if oerr != nil {
			return nil, oerr
		}
		conn := model.ConnectionResult{
			User:           model.UserResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name},
			SharedAccounts: []model.AccountAccessResult{},
			AccessLevel:    string(model.EffectiveAccessLevel(owner.AccessLevel, owner.AccessUntil, s.clock.Now())),
			AccessUntil:    datetime.FormatOrEmpty(owner.AccessUntil),
		}

		// Dedup by account id, preserving discovery order (received first, then
		// issued): last write wins on the value, but the order follows the FIRST
		// occurrence of each key, so we track order separately.
		order := []string{}
		byID := map[string]model.AccountAccessResult{}
		add := func(a *model.AccountAccess) {
			accOwner := owners[a.AccountID.String()]
			if !accOwner.Equal(cu) && !accOwner.Equal(userID) {
				return
			}
			key := a.AccountID.String()
			if _, seen := byID[key]; !seen {
				order = append(order, key)
			}
			byID[key] = model.AccountAccessResult{Id: key, OwnerUserId: accOwner.String(), Role: a.Role.Alias()}
		}
		for _, a := range received {
			add(a)
		}
		for _, a := range issued {
			add(a)
		}
		for _, key := range order {
			conn.SharedAccounts = append(conn.SharedAccounts, byID[key])
		}
		result.Items = append(result.Items, conn)
	}
	return result, nil
}
