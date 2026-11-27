package mailer

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// IdentitySender sends the two sign-in-method notices: one when an oauth
// provider is linked to an account, one when it is unlinked. Mirrors
// VerifySender so the oauth feature stays free of any mail dependency.
type IdentitySender struct {
	m       Mailer
	from    string
	replyTo string
	appURL  string
}

// NewIdentitySender wires the identity email sender over a Mailer with the
// configured From / Reply-To addresses (the from / reply_to query params of
// MAILER_DSN).
func NewIdentitySender(m Mailer, from, replyTo, appURL string) *IdentitySender {
	return &IdentitySender{m: m, from: from, replyTo: replyTo, appURL: appURL}
}

// SendIdentityLinked emails the account owner that provider was just linked to
// their account. lang is the account's stored language; when empty it falls
// back to reqctx.Language(ctx) (the caller's request language), since an
// auto-link happens on the OAUTH CALLBACK request, not one made by the account
// owner themselves. cc carries the account's other linked-provider addresses,
// so a sign-in method gained without asking is noticeable even when the
// primary mailbox is not the one the owner reads.
func (s *IdentitySender) SendIdentityLinked(ctx context.Context, to, name, provider, lang string, cc []string) error {
	return s.send(ctx, to, "identity_linked", map[string]any{"provider": provider}, name, lang, cc)
}

// SendIdentityUnlinked emails the account owner that provider can no longer be
// used to sign in. Losing a sign-in method is as worth detecting as gaining
// one — and for a passwordless account an unlink someone else performed is a
// lockout. lang and cc follow the same rules as SendIdentityLinked; cc holds
// the addresses still attached, the removed one having gone with the row.
func (s *IdentitySender) SendIdentityUnlinked(ctx context.Context, to, name, provider, lang string, cc []string) error {
	return s.send(ctx, to, "identity_unlinked", map[string]any{"provider": provider}, name, lang, cc)
}

func (s *IdentitySender) send(ctx context.Context, to, key string, params map[string]any, name, lang string, cc []string) error {
	if lang == "" {
		lang = reqctx.Language(ctx)
	}
	c := Compose(lang, key, params, name, "", s.appURL)
	msg, err := newMessage(s.from, to, s.replyTo, i18n.T(lang, "emails."+key+".subject", nil), ccAddresses(to, cc), c)
	if err != nil {
		return err
	}
	return s.m.Send(ctx, msg)
}
