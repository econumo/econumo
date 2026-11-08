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
	c := model.OAuthClient{ID: vo.NewId(), Name: name, RedirectURIs: redirectURIs, CreatedAt: s.clock.Now().UTC(), IsGlobal: true}
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

func (s *Service) GetApp(ctx context.Context, id vo.Id) (model.OAuthApp, error) {
	c, err := s.getApp(ctx, id)
	if err != nil {
		return model.OAuthApp{}, err
	}
	grants, err := s.liveGrants(ctx, id)
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
// URIs when redirectURIs is empty.
func (s *Service) UpdateOfficialApp(ctx context.Context, id vo.Id, name *string, redirectURIs []string) error {
	c, err := s.getApp(ctx, id)
	if err != nil {
		return err
	}
	if !c.IsGlobal {
		return ErrAppNotOfficial
	}
	newName, uris := c.Name, c.RedirectURIs
	if name != nil {
		if newName, err = officialName(*name); err != nil {
			return err
		}
	}
	if len(redirectURIs) > 0 {
		if oe := checkRedirectURIs(redirectURIs); oe != nil {
			return plain(oe)
		}
		uris = redirectURIs
	}
	n, err := s.repo.UpdateGlobalClient(ctx, id, newName, uris)
	if err != nil {
		return err
	}
	if n == 0 {
		return appNotFound()
	}
	return nil
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
		return "", ErrAppPublic
	}
	return raw, nil
}

// DisconnectApp revokes the app's live grants, only userID's when it is set,
// and returns how many it revoked.
func (s *Service) DisconnectApp(ctx context.Context, id vo.Id, userID *vo.Id) (int, error) {
	if _, err := s.getApp(ctx, id); err != nil {
		return 0, err
	}
	grants, err := s.liveGrants(ctx, id)
	if err != nil {
		return 0, err
	}
	now := s.clock.Now()
	n := 0
	for _, g := range grants {
		if userID != nil && !g.UserID.Equal(*userID) {
			continue
		}
		if err := s.revokeGrant(ctx, g.UserID, g.ID, now); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// DeleteApp revokes every grant and its access tokens, then deletes the client.
// The client row lock closes the race with a concurrent code exchange: its
// grant INSERT either commits before the lock and is revoked here, or fails its
// foreign key once the client is gone.
func (s *Service) DeleteApp(ctx context.Context, id vo.Id) (int, error) {
	grants, err := s.repo.ListClientGrants(ctx, id)
	if err != nil {
		return 0, err
	}
	var users []vo.Id
	for _, g := range grants {
		if !slices.ContainsFunc(users, g.UserID.Equal) {
			users = append(users, g.UserID)
		}
	}
	slices.SortFunc(users, func(a, b vo.Id) int { return strings.Compare(a.String(), b.String()) })
	revoked := 0
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, u := range users {
			if _, err := s.creds.LockForOAuth(ctx, u); err != nil {
				return err
			}
		}
		if err := s.repo.LockClient(ctx, id); err != nil {
			if _, ok := errs.AsNotFound(err); ok {
				return appNotFound()
			}
			return err
		}
		ids, err := s.repo.RevokeClientGrants(ctx, id, s.clock.Now())
		if err != nil {
			return err
		}
		for _, g := range ids {
			if err := s.creds.RevokeOAuthGrantTokens(ctx, g); err != nil {
				return err
			}
		}
		if _, err := s.repo.DeleteClient(ctx, id); err != nil {
			return err
		}
		revoked = len(ids)
		return nil
	})
	return revoked, err
}
