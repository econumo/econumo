package mailer

import (
	"context"

	"github.com/econumo/econumo/internal/infra/i18n"
	"github.com/econumo/econumo/internal/shared/reqctx"
)

// DeletionSender sends the user-deletion emails. The app link is not a
// parameter: WithAppLink already appends it to every message at composition.
type DeletionSender struct {
	m       Mailer
	from    string
	replyTo string
}

func NewDeletionSender(m Mailer, from, replyTo string) *DeletionSender {
	return &DeletionSender{m: m, from: from, replyTo: replyTo}
}

// SendInactivityNotice takes lang explicitly: the inactivity pass runs outside
// any request, so there is no Accept-Language to read.
func (s *DeletionSender) SendInactivityNotice(ctx context.Context, lang, to, name string, days int) error {
	subject := i18n.T(lang, "emails.inactivity_notice.subject", nil)
	body := i18n.T(lang, "emails.inactivity_notice.body", map[string]any{"name": name, "days": days})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}

func (s *DeletionSender) SendDeletionCode(ctx context.Context, to, name, code string) error {
	lang := reqctx.Language(ctx)
	subject := i18n.T(lang, "emails.delete_data.subject", nil)
	body := i18n.T(lang, "emails.delete_data.body", map[string]any{"name": name, "code": code})
	return s.m.Send(ctx, Message{From: s.from, To: to, ReplyTo: s.replyTo, Subject: subject, Text: body})
}
