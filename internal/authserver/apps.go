package authserver

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

var (
	ErrAppNotOfficial     = errors.New("only an official app can be changed; a user app's redirect URIs belong to its user's client")
	ErrAppAlreadyOfficial = errors.New("the app is already official")
	ErrAppPublic          = errors.New("the app is public: it has no secret to rotate")
	ErrAppNameRequired    = errors.New("name is required")
	ErrAppBusy            = errors.New("the app keeps getting new connections; try again")
)

func appNotFound() error { return errs.NewNotFound("OAuth app not found") }

func plain(oe *OAuthError) error { return errors.New(oe.Description) }

func (s *Service) getApp(ctx context.Context, id vo.Id) (*model.OAuthClient, error) {
	c, err := s.repo.GetClient(ctx, id)
	if err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return nil, appNotFound()
		}
		return nil, err
	}
	return c, nil
}

func officialName(raw string) (string, error) {
	name, oe := cleanClientName(raw)
	if oe != nil {
		return "", plain(oe)
	}
	if name == "" {
		return "", ErrAppNameRequired
	}
	return name, nil
}

// RegisterOfficialApp returns the raw secret, "" for a public app.
func (s *Service) RegisterOfficialApp(ctx context.Context, name string, redirectURIs []string, public bool) (model.OAuthClient, string, error) {
	name, err := officialName(name)
	if err != nil {
		return model.OAuthClient{}, "", err
	}
	if oe := checkRedirectURIs(redirectURIs); oe != nil {
		return model.OAuthClient{}, "", plain(oe)
	}
	c := model.OAuthClient{ID: vo.NewId(), Name: name, RedirectURIs: slices.Clone(redirectURIs), CreatedAt: s.clock.Now().UTC(), IsGlobal: true}
	var raw string
	if !public {
		r, hash, err := newSecret()
		if err != nil {
			return model.OAuthClient{}, "", err
		}
		raw, c.SecretHash = r, &hash
	}
	if err := s.repo.InsertClient(ctx, &c); err != nil {
		return model.OAuthClient{}, "", err
	}
	return c, raw, nil
}

func (s *Service) ListApps(ctx context.Context) ([]model.OAuthApp, error) {
	clients, err := s.repo.ListClients(ctx)
	if err != nil {
		return nil, err
	}
	grants, err := s.repo.ListAllUnrevokedGrants(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	byClient := map[vo.Id][]model.OAuthGrant{}
	for _, g := range grants {
		if g.IsLive(now) {
			byClient[g.ClientID] = append(byClient[g.ClientID], g)
		}
	}
	out := make([]model.OAuthApp, 0, len(clients))
	for _, c := range clients {
		out = append(out, model.OAuthApp{Client: c, Grants: byClient[c.ID]})
	}
	return out, nil
}

// GetApp reads the client after its grants, so a client deleted in between is
// reported as not found rather than with an empty grant list.
func (s *Service) GetApp(ctx context.Context, id vo.Id) (model.OAuthApp, error) {
	grants, err := s.liveGrants(ctx, id)
	if err != nil {
		return model.OAuthApp{}, err
	}
	c, err := s.getApp(ctx, id)
	if err != nil {
		return model.OAuthApp{}, err
	}
	return model.OAuthApp{Client: *c, Grants: grants}, nil
}

func (s *Service) liveGrants(ctx context.Context, clientID vo.Id) ([]model.OAuthGrant, error) {
	all, err := s.repo.ListClientGrants(ctx, clientID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	return slices.DeleteFunc(all, func(g model.OAuthGrant) bool { return !g.IsLive(now) }), nil
}

// UpdateOfficialApp leaves the name unchanged when name is nil and the redirect
// URIs when redirectURIs is empty. The read and the write share the client row
// lock, so a concurrent change to the field left alone is not overwritten.
func (s *Service) UpdateOfficialApp(ctx context.Context, id vo.Id, name *string, redirectURIs []string) error {
	var newName string
	if name != nil {
		var err error
		if newName, err = officialName(*name); err != nil {
			return err
		}
	}
	if len(redirectURIs) > 0 {
		if oe := checkRedirectURIs(redirectURIs); oe != nil {
			return plain(oe)
		}
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockClient(ctx, id); err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				return appNotFound()
			}
			return err
		}
		c, err := s.getApp(ctx, id)
		if err != nil {
			return err
		}
		if !c.IsGlobal {
			return ErrAppNotOfficial
		}
		if name == nil {
			newName = c.Name
		}
		uris := c.RedirectURIs
		if len(redirectURIs) > 0 {
			uris = slices.Clone(redirectURIs)
		}
		n, err := s.repo.UpdateGlobalClient(ctx, id, newName, uris)
		if err != nil {
			return err
		}
		if n == 0 {
			return appNotFound()
		}
		return nil
	})
}

func (s *Service) MakeAppOfficial(ctx context.Context, id vo.Id) error {
	c, err := s.getApp(ctx, id)
	if err != nil {
		return err
	}
	if c.IsGlobal {
		return ErrAppAlreadyOfficial
	}
	n, err := s.repo.MakeClientGlobal(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		if _, err := s.getApp(ctx, id); err != nil {
			return err
		}
		return ErrAppAlreadyOfficial
	}
	return nil
}

// RotateAppSecret returns the new raw secret; the old one stops working at once.
func (s *Service) RotateAppSecret(ctx context.Context, id vo.Id) (string, error) {
	c, err := s.getApp(ctx, id)
	if err != nil {
		return "", err
	}
	if c.SecretHash == nil {
		return "", ErrAppPublic
	}
	raw, hash, err := newSecret()
	if err != nil {
		return "", err
	}
	n, err := s.repo.SetClientSecret(ctx, id, hash)
	if err != nil {
		return "", err
	}
	if n == 0 {
		if _, err := s.getApp(ctx, id); err != nil {
			return "", err
		}
		return "", ErrAppPublic
	}
	return raw, nil
}

// DisconnectApp revokes the app's grants and drops its outstanding
// authorization codes, only userID's when it is set, and returns how many live
// grants it revoked. It locks like DeleteApp: the grants' users, then the
// client. An exchange consumes its code under the client lock too, so a code
// it read before then is either dropped here or its grant commits first and is
// revoked here. A consent given after this commits is a new connection.
func (s *Service) DisconnectApp(ctx context.Context, id vo.Id, userID *vo.Id) (int, error) {
	if _, err := s.getApp(ctx, id); err != nil {
		return 0, err
	}
	var users []vo.Id
	if userID != nil {
		users = []vo.Id{*userID}
	} else {
		grants, err := s.repo.ListClientGrants(ctx, id)
		if err != nil {
			return 0, err
		}
		users = addGrantUsers(nil, grants)
	}
	for range deleteAppAttempts {
		revoked, more, err := s.disconnectApp(ctx, id, users, userID)
		if !errors.Is(err, errGrantUsersChanged) {
			return revoked, err
		}
		users = addGrantUsers(users, more)
	}
	return 0, ErrAppBusy
}

func (s *Service) disconnectApp(ctx context.Context, id vo.Id, users []vo.Id, userID *vo.Id) (int, []model.OAuthGrant, error) {
	revoked := 0
	var current []model.OAuthGrant
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		revoked, current = 0, nil
		if err := s.lockUsersAndClient(ctx, id, users); err != nil {
			return err
		}
		if _, err := s.repo.DeleteClientCodes(ctx, id, userID); err != nil {
			return err
		}
		now := s.clock.Now()
		var err error
		if current, err = s.repo.ListClientGrants(ctx, id); err != nil {
			return err
		}
		if userID == nil && hasUnlockedUser(current, users) {
			return errGrantUsersChanged
		}
		for _, g := range current {
			if userID != nil && !g.UserID.Equal(*userID) {
				continue
			}
			n, err := s.repo.RevokeGrant(ctx, g.ID, now)
			if err != nil {
				return err
			}
			if n == 0 {
				continue
			}
			if err := s.creds.RevokeOAuthGrantTokens(ctx, g.ID); err != nil {
				return err
			}
			if g.IsLive(now) {
				revoked++
			}
		}
		return nil
	})
	if err != nil {
		return 0, current, err
	}
	return revoked, current, nil
}

func (s *Service) lockUsersAndClient(ctx context.Context, id vo.Id, users []vo.Id) error {
	for _, u := range users {
		if _, err := s.creds.LockForOAuth(ctx, u); err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				return errs.NewNotFound("user not found")
			}
			return err
		}
	}
	if err := s.repo.LockClient(ctx, id); err != nil {
		if _, ok := errs.AsNotFound(err); ok {
			return appNotFound()
		}
		return err
	}
	return nil
}

func hasUnlockedUser(grants []model.OAuthGrant, users []vo.Id) bool {
	for _, g := range grants {
		if !slices.ContainsFunc(users, g.UserID.Equal) {
			return true
		}
	}
	return false
}

// deleteAppAttempts bounds how often DeleteApp and DisconnectApp start over
// because a grant for a user they had not locked appeared before they took the
// client lock.
const deleteAppAttempts = 5

var errGrantUsersChanged = errors.New("grants for unlocked users appeared")

// DeleteApp revokes every grant and its access tokens, then deletes the client.
// The client row lock closes the race with a concurrent code exchange: its
// grant INSERT either commits before the lock and is revoked here, or fails its
// foreign key once the client is gone.
//
// Revoking a grant's tokens needs its user's row lock, taken before the client
// lock (the order approve and exchange use). A grant committed for another user
// after the users were read cannot have its user locked once the client lock is
// held without inverting that order, so the attempt rolls back and starts over
// with that user locked too.
func (s *Service) DeleteApp(ctx context.Context, id vo.Id) (int, error) {
	grants, err := s.repo.ListClientGrants(ctx, id)
	if err != nil {
		return 0, err
	}
	users := addGrantUsers(nil, grants)
	for range deleteAppAttempts {
		revoked, more, err := s.deleteApp(ctx, id, users)
		if !errors.Is(err, errGrantUsersChanged) {
			return revoked, err
		}
		users = addGrantUsers(users, more)
	}
	return 0, ErrAppBusy
}

func addGrantUsers(users []vo.Id, grants []model.OAuthGrant) []vo.Id {
	for _, g := range grants {
		if !slices.ContainsFunc(users, g.UserID.Equal) {
			users = append(users, g.UserID)
		}
	}
	slices.SortFunc(users, func(a, b vo.Id) int { return strings.Compare(a.String(), b.String()) })
	return users
}

// deleteApp returns errGrantUsersChanged with the grants read under the client
// lock when one of them belongs to a user outside users.
func (s *Service) deleteApp(ctx context.Context, id vo.Id, users []vo.Id) (int, []model.OAuthGrant, error) {
	revoked := 0
	var current []model.OAuthGrant
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		revoked, current = 0, nil
		if err := s.lockUsersAndClient(ctx, id, users); err != nil {
			return err
		}
		now := s.clock.Now()
		var err error
		current, err = s.repo.ListClientGrants(ctx, id)
		if err != nil {
			return err
		}
		if hasUnlockedUser(current, users) {
			return errGrantUsersChanged
		}
		ids, err := s.repo.RevokeClientGrants(ctx, id, now)
		if err != nil {
			return err
		}
		for _, g := range ids {
			if err := s.creds.RevokeOAuthGrantTokens(ctx, g); err != nil {
				return err
			}
			// A grant missing from the list was committed after it was read, so it is new.
			i := slices.IndexFunc(current, func(c model.OAuthGrant) bool { return c.ID.Equal(g) })
			if i < 0 || current[i].IsLive(now) {
				revoked++
			}
		}
		if _, err := s.repo.DeleteClient(ctx, id); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, current, err
	}
	return revoked, current, nil
}
