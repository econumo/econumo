package connection

import (
	"context"

	domconnection "github.com/econumo/econumo/internal/domain/connection"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// GetConnectionList returns the requesting user's connections, each with the
// accounts shared between that connected user and the requester. Mirrors PHP
// GetConnectionListV1ResultAssembler: for every connected user, it gathers the
// received-access grants (accounts shared TO me) and issued-access grants
// (grants on accounts I own), keeping those whose account is owned by either the
// connected user or me, deduplicated by account id (last write wins), in
// account-id discovery order.
func (s *Service) GetConnectionList(ctx context.Context, userID vo.Id) (*GetConnectionListResult, error) {
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
	for _, a := range append(append([]*domconnection.AccountAccess{}, received...), issued...) {
		if _, oerr := resolveOwner(a.AccountId()); oerr != nil {
			return nil, oerr
		}
	}

	result := &GetConnectionListResult{Items: []ConnectionResult{}}
	for _, cu := range connected {
		owner, oerr := s.users.GetOwner(ctx, cu.String())
		if oerr != nil {
			return nil, oerr
		}
		conn := ConnectionResult{
			User:           UserResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name},
			SharedAccounts: []AccountAccessResult{},
		}

		// Dedup by account id, preserving discovery order (received first, then
		// issued) -- the PHP map keeps last-write but array_values keeps insertion
		// order of the FIRST occurrence of each key, so we track order separately.
		order := []string{}
		byID := map[string]AccountAccessResult{}
		add := func(a *domconnection.AccountAccess) {
			accOwner := owners[a.AccountId().String()]
			if !accOwner.Equal(cu) && !accOwner.Equal(userID) {
				return
			}
			key := a.AccountId().String()
			if _, seen := byID[key]; !seen {
				order = append(order, key)
			}
			byID[key] = AccountAccessResult{Id: key, OwnerUserId: accOwner.String(), Role: a.Role().Alias()}
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
