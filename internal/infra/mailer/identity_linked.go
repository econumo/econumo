package mailer

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// IdentityLinkedSender notifies the account owner when an oauth provider is
// auto-linked to their existing password account, mirroring VerifySender so
// the oauth feature stays free of any mail dependency.
type IdentityLinkedSender struct {
	m       Mailer
	from    string
	replyTo string
}

// NewIdentityLinkedSender wires the identity-linked email sender over a Mailer
// with the configured From / Reply-To addresses (the from / reply_to query
// params of MAILER_DSN).
func NewIdentityLinkedSender(m Mailer, from, replyTo string) *IdentityLinkedSender {
	return &IdentityLinkedSender{m: m, from: from, replyTo: replyTo}
}

// SendIdentityLinked emails the account owner that provider was just linked to
// their account. lang is the account's stored language; when empty it falls
// back to reqctx.Language(ctx) (the caller's request language), since the
// auto-link happens on the OAUTH CALLBACK request, not one made by the account
// owner themselves.
func (s *IdentityLinkedSender) SendIdentityLinked(ctx context.Context, to, name, provider, lang string) error {
	if lang == "" {
		lang = reqctx.Language(ctx)
	}
	subject := i18n.T(lang, "emails.identity_linked.subject", nil)
	body := i18n.T(lang, "emails.identity_linked.body", map[string]any{"name": name, "provider": provider})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}
