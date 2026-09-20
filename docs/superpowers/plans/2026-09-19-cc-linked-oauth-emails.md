# CC Linked OAuth Addresses On Notice Emails — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The *notice* emails (a provider was linked to your account; a provider was unlinked; your email change was requested) are CC'd to every address the user has attached through Google / Apple / the custom OIDC slot, so a notice reaches the user even when their primary mailbox is not the one they read.

> **Amended 2026-09-20.** Written against `0a4f591`; `669e69a` ("email the owner when a provider is linked or unlinked", #267) landed on main first and was merged in. That commit renamed `IdentityLinkedSender` -> `IdentitySender` (`identity_linked.go` -> `identity.go`), routed both identity emails through a shared unexported `send`, gave `Notifier` a second method `IdentityUnlinked`, and collapsed the notifier glue onto a shared `notify` helper. It also added a SIXTH transactional email, `identity_unlinked` — a notice, so the agreed "notices only" rule covers it. The rule is unchanged; it now has three instances instead of two.

> **Executed 2026-09-20.** All five tasks are done and committed. The task bodies below are the plan AS WRITTEN against `0a4f591`; where #267 moved the ground under them, the code that actually landed differs in the ways listed under "As built" at the foot of this document. Read that section alongside any task body you are checking.

**Architecture:** `mailer.Message` gains a `Cc []string` field that both transports honour. Only the notice senders accept a CC list; the three *code* senders (reset, verify, change-email) keep their single-recipient signatures. The addresses come from `users_identities.email`, which `oauth.Identities.ListByUser` already reads. The `identity_linked` notice is sent from `internal/server/glue_oauth_notifier.go`, which is already in the composition root and can call the oauth service directly; the `change_email_notice` is sent from `internal/user`, which may not import `oauth`, so it gets a small consumer-side port wired by a new glue adapter — the same shape as the existing `OAuthReclaimer`.

**Tech Stack:** Go (stdlib + `github.com/resend/resend-go/v3`), sqlc-generated repos, `internal/test/dbtest` + `internal/test/fixture` for integration tests.

**Spec:** None. This is a bounded change whose design was agreed in chat on 2026-09-19; the agreed design is restated in full under "Design (agreed)" below, and the plan argues from that section.

## Global Constraints

- Branch: `feature/cc-linked-oauth-emails` (not a bug fix, so `feature/` per CLAUDE.md "Branch naming").
- **Code emails are never CC'd.** `SendResetPasswordCode`, `SendVerificationCode` and `SendEmailChangeCode` keep their exact current signatures and behaviour. The change-email code in particular exists solely to prove control of the proposed new mailbox; CC'ing it would defeat that check.
- **The unlink notice CCs the addresses still attached**, not the one just removed. `UnlinkIdentity` deletes the row inside its transaction and notifies after, so `ListIdentityEmails` no longer returns the removed address — which is the intended reading of "every address attached to the profile". Copying the just-removed address too would need a new parameter on the `Notifier` port; it is deliberately left out of this plan.
- No database migration: `users_identities.email` already exists (`internal/infra/storage/migrations/{sqlite,pgsql}/20260907000000.sql`).
- No new i18n catalogue keys; `mailer.EmailKeys` is unchanged. The rendered `Subject`/`Text` of every email must stay byte-identical — the existing `TestResetEmailEnglishUnchanged` / `TestIdentityLinkedEmailEnglishUnchanged` guards must keep passing untouched.
- No API/wire change, no OpenAPI regeneration, no golden regeneration. `internal/test/apiparity`'s `recordingMailer` records the whole `mailer.Message`, so the new field costs it nothing.
- Comments follow CLAUDE.md "Comments — write sparingly": only the *why* (frozen-contract rationale, the security reason a code email is excluded). No godoc that restates a signature.
- Go toolchain is not on `PATH`. Every Go command in this plan must be run as `export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 go …` from the repo root (the worktree root).
- Run every command from the worktree root: `/home/dmitry/dev/econumo/econumo/.claude/worktrees/bridge-cse_01VtJQeVFnFwceEKjjHd9t5k`.

---

## Design (agreed)

The six transactional emails today:

| Sender method | Recipient | Nature | CC'd? |
|---|---|---|---|
| `ResetSender.SendResetPasswordCode` | address typed at remind-password | secret code | **no** |
| `VerifySender.SendVerificationCode` | account email | secret code | **no** |
| `ChangeEmailSender.SendEmailChangeCode` | the proposed **new** address | proof of new mailbox | **no** |
| `ChangeEmailSender.SendEmailChangeNotice` | the **old** address | notice | **yes** |
| `IdentitySender.SendIdentityLinked` | account email | notice | **yes** |
| `IdentitySender.SendIdentityUnlinked` | account email | notice | **yes** |

Accepted consequences, agreed with the user:

- The `Cc` header discloses the user's linked addresses to each of their own mailboxes. They belong to one person, so this is a disclosure to the user about themselves.
- A stale provider address (one the user abandoned but never unlinked) keeps receiving these two notices until they unlink it.

For `change_email_notice`, the proposed **new** address is deliberately *not* CC'd: it is not a linked address, and the notice names the new address, so CC'ing it would tell the new-address holder before they have confirmed anything.

For `identity_linked`, the just-linked provider's own address **is** included. `Service.autoLink` calls the notifier *after* `saveLinkedIdentity` has committed (`internal/oauth/callback.go`), so `ListByUser` already returns the new row. The provider vouched for that address moments earlier, so it leaks nothing new; dedupe removes it when it equals the primary address.

For `identity_unlinked`, the mirror image: `UnlinkIdentity` notifies after the delete has committed, so the CC list is the addresses that remain. See the Global Constraints note on why the removed address is not added back.

Resolving the CC list is best-effort. A lookup failure degrades to the primary address alone and is logged — it never blocks or fails the email, matching the existing best-effort notice policy.

---

## File Structure

**Created**

- `internal/server/glue_user_identityemails.go` — adapts `*oauth.Service` to the user feature's new `IdentityEmailLister` port. Sibling of `glue_user_reclaim.go`, same shape and same reason (features never import features).
- `internal/server/glue_user_identityemails_test.go` — the adapter over a real oauth service and real identity rows.

**Modified**

- `internal/infra/mailer/mailer.go` — `Message.Cc`; `console.Send` renders it; `resendMailer.Send` forwards it; new unexported `ccAddresses` helper.
- `internal/infra/mailer/identity.go` — the shared `send` and both `SendIdentityLinked` / `SendIdentityUnlinked` take `cc []string`.
- `internal/infra/mailer/change_email.go` — `SendEmailChangeNotice` takes `cc []string`. `SendEmailChangeCode` untouched.
- `internal/infra/mailer/mailer_test.go` — `ccAddresses` table test, console/Resend CC coverage, updated notice-sender calls.
- `internal/oauth/identities.go` — new `Service.ListIdentityEmails`.
- `internal/oauth/identities_test.go` (create if absent) — coverage for `ListIdentityEmails`.
- `internal/user/ports.go` — new `IdentityEmailLister` port.
- `internal/user/usecase.go` — `identityEmails` field + `SetIdentityEmailLister` setter (mirrors `SetOAuthReclaimer`; keeps the long constructor untouched).
- `internal/user/change_email.go` — resolve the CC list before sending the notice.
- `internal/user/change_email_integration_test.go` — assert the notice carries the CCs and the code email does not.
- `internal/server/server.go` — wire `SetIdentityEmailLister`; pass the CC list into the identity-linked notice.
- `internal/server/glue_oauth_notifier.go` — resolve and pass the CC list.
- `internal/server/glue_oauth_notifier_test.go` — a linked-identity CC case.
- `docs/regression-test-plan.md` — two checklist items.
- `CLAUDE.md` — one line in the oauth notable-behaviours.

---

## Task 1: `Message.Cc` and the address-normalizing helper

Self-contained in `internal/infra/mailer`. No caller changes yet, so the tree stays green.

**Files:**
- Modify: `internal/infra/mailer/mailer.go`
- Test: `internal/infra/mailer/mailer_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `mailer.Message` gains exported field `Cc []string`.
  - unexported `func ccAddresses(to string, candidates []string) []string` — used by Tasks 2's senders.

- [x] **Step 1: Write the failing tests**

Append to `internal/infra/mailer/mailer_test.go`:

```go
func TestCcAddresses(t *testing.T) {
	cases := []struct {
		name       string
		to         string
		candidates []string
		want       []string
	}{
		{"nil candidates", "to@x.test", nil, nil},
		{"drops the To address, case-insensitively", "To@X.test", []string{"to@x.TEST"}, nil},
		{"keeps a distinct address", "to@x.test", []string{"other@x.test"}, []string{"other@x.test"}},
		{"trims and drops empties", "to@x.test", []string{"  ", "", "  other@x.test  "}, []string{"other@x.test"}},
		{"dedupes case-insensitively, first spelling wins", "to@x.test",
			[]string{"Other@X.test", "other@x.test"}, []string{"Other@X.test"}},
		{"preserves order", "to@x.test",
			[]string{"b@x.test", "a@x.test"}, []string{"b@x.test", "a@x.test"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ccAddresses(tc.to, tc.candidates)
			if len(got) != len(tc.want) {
				t.Fatalf("ccAddresses(%q, %v) = %v, want %v", tc.to, tc.candidates, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ccAddresses(%q, %v) = %v, want %v", tc.to, tc.candidates, got, tc.want)
				}
			}
		})
	}
}

func TestConsole_RendersCc(t *testing.T) {
	var buf bytes.Buffer
	c := console{out: &buf}
	msg := Message{From: "from@x.test", To: "to@x.test", Cc: []string{"a@x.test", "b@x.test"},
		Subject: "Hi", Text: "body"}
	if err := c.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "Cc: a@x.test, b@x.test") {
		t.Errorf("console output missing the Cc line\ngot:\n%s", got)
	}

	// No CCs: the line is omitted entirely, so the dev output of every existing
	// email is unchanged.
	buf.Reset()
	if err := c.Send(context.Background(), Message{To: "to@x.test", Text: "body"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if strings.Contains(buf.String(), "Cc:") {
		t.Errorf("empty Cc should print no Cc line\ngot:\n%s", buf.String())
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go test ./internal/infra/mailer/ -run 'TestCcAddresses|TestConsole_RendersCc' -v
```
Expected: FAIL to compile — `undefined: ccAddresses` and `unknown field Cc in struct literal of type Message`.

- [x] **Step 3: Implement**

In `internal/infra/mailer/mailer.go`, add `Cc` to `Message`:

```go
type Message struct {
	From    string
	To      string
	Cc      []string
	ReplyTo string
	Subject string
	Text    string
}
```

Add the helper below the `Mailer` interface:

```go
// ccAddresses normalizes a CC candidate list for a message addressed to `to`:
// trims, drops empties, drops `to` itself, and dedupes case-insensitively while
// preserving order. It lives here rather than in a transport so the two notice
// senders share one policy and WithAppLink cannot bypass it; transports stay
// policy-free. Only the NOTICE emails pass a list — the reset, verification and
// change-email codes each prove control of one specific mailbox, so copying
// them anywhere else would defeat the check they exist for.
func ccAddresses(to string, candidates []string) []string {
	seen := map[string]struct{}{strings.ToLower(strings.TrimSpace(to)): {}}
	var out []string
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		key := strings.ToLower(c)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	return out
}
```

Replace `console.Send` with:

```go
func (c console) Send(_ context.Context, msg Message) error {
	cc := ""
	if len(msg.Cc) > 0 {
		cc = "Cc: " + strings.Join(msg.Cc, ", ") + "\n"
	}
	_, err := fmt.Fprintf(c.out,
		"--- email (console transport) ---\nFrom: %s\nTo: %s\n%sReply-To: %s\nSubject: %s\n\n%s\n---------------------------------\n",
		msg.From, msg.To, cc, msg.ReplyTo, msg.Subject, msg.Text,
	)
	return err
}
```

In `resendMailer.Send`, add `Cc` to the request (the field is `json:"cc,omitempty"`, so a nil slice is omitted from the wire and every existing email's payload is unchanged):

```go
	params := &resend.SendEmailRequest{
		From:    msg.From,
		To:      []string{msg.To},
		Cc:      msg.Cc,
		Subject: msg.Subject,
		Text:    msg.Text,
	}
```

- [x] **Step 4: Run the tests to verify they pass**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go test ./internal/infra/mailer/ -v
```
Expected: PASS, including the pre-existing `TestConsole_RendersMessage`, `TestResetEmailEnglishUnchanged` and `TestIdentityLinkedEmailEnglishUnchanged`.

- [x] **Step 5: Commit**

```bash
git add internal/infra/mailer/mailer.go internal/infra/mailer/mailer_test.go
git commit -m "feat(mailer): carry a Cc list on Message

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

## Task 2: The two notice senders accept a CC list

**Files:**
- Modify: `internal/infra/mailer/identity_linked.go`
- Modify: `internal/infra/mailer/change_email.go`
- Modify: `internal/infra/mailer/mailer_test.go`
- Modify (call sites, pass `nil` for now): `internal/server/glue_oauth_notifier.go:38`, `internal/user/change_email.go:57`
- Modify (test call sites): `internal/server/glue_oauth_notifier_test.go`

**Interfaces:**
- Consumes: `ccAddresses` and `Message.Cc` from Task 1.
- Produces:
  - `func (s *IdentityLinkedSender) SendIdentityLinked(ctx context.Context, to, name, provider, lang string, cc []string) error`
  - `func (s *ChangeEmailSender) SendEmailChangeNotice(ctx context.Context, to, name, newEmail string, cc []string) error`
  - Unchanged: `SendEmailChangeCode(ctx, to, name, code string) error`, `SendVerificationCode(ctx, to, name, code string) error`, `SendResetPasswordCode(ctx, to, name, code string) error`.

- [x] **Step 1: Write the failing tests**

Append to `internal/infra/mailer/mailer_test.go`:

```go
func TestIdentityLinkedSender_CarriesNormalizedCc(t *testing.T) {
	c := &captureMailer{}
	s := NewIdentityLinkedSender(c, "from@econumo.test", "reply@econumo.test")
	// The primary address is repeated in the candidates (the provider that was
	// just linked may be the one that owns it) and must not be duplicated.
	cc := []string{"user@x.test", "google@x.test", "apple@x.test"}
	if err := s.SendIdentityLinked(context.Background(), "user@x.test", "Alice", "Google", "en", cc); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(c.msg.Cc) != 2 || c.msg.Cc[0] != "google@x.test" || c.msg.Cc[1] != "apple@x.test" {
		t.Errorf("Cc = %v, want the linked addresses minus the To address", c.msg.Cc)
	}
}

func TestChangeEmailSender_NoticeCcsLinkedAddressesButTheCodeDoesNot(t *testing.T) {
	c := &captureMailer{}
	s := NewChangeEmailSender(c, "from@econumo.test", "reply@econumo.test")

	if err := s.SendEmailChangeNotice(context.Background(), "old@x.test", "Alice", "new@x.test",
		[]string{"google@x.test"}); err != nil {
		t.Fatalf("notice: %v", err)
	}
	if c.msg.To != "old@x.test" {
		t.Errorf("notice To = %q, want the OLD address", c.msg.To)
	}
	if len(c.msg.Cc) != 1 || c.msg.Cc[0] != "google@x.test" {
		t.Errorf("notice Cc = %v, want the linked address", c.msg.Cc)
	}

	// The code proves control of the NEW mailbox, so it is never copied anywhere.
	if err := s.SendEmailChangeCode(context.Background(), "new@x.test", "Alice", "123456"); err != nil {
		t.Fatalf("code: %v", err)
	}
	if c.msg.To != "new@x.test" || len(c.msg.Cc) != 0 {
		t.Errorf("code message = To %q Cc %v, want the new address only", c.msg.To, c.msg.Cc)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go test ./internal/infra/mailer/ -run 'CarriesNormalizedCc|NoticeCcs' -v
```
Expected: FAIL to compile — "too many arguments in call to s.SendIdentityLinked" and the same for `SendEmailChangeNotice`.

- [x] **Step 3: Implement the two signatures**

In `internal/infra/mailer/identity_linked.go`, replace the `SendIdentityLinked` doc comment's last paragraph and signature:

```go
// SendIdentityLinked emails the account owner that provider was just linked to
// their account. lang is the account's stored language; when empty it falls
// back to reqctx.Language(ctx) (the caller's request language), since the
// auto-link happens on the OAUTH CALLBACK request, not one made by the account
// owner themselves. cc carries the user's other linked-provider addresses so a
// sign-in method gained without asking is noticeable even when the primary
// mailbox is not the one they read.
func (s *IdentityLinkedSender) SendIdentityLinked(ctx context.Context, to, name, provider, lang string, cc []string) error {
	if lang == "" {
		lang = reqctx.Language(ctx)
	}
	subject := i18n.T(lang, "emails.identity_linked.subject", nil)
	body := i18n.T(lang, "emails.identity_linked.body", map[string]any{"name": name, "provider": provider})
	return s.m.Send(ctx, Message{From: s.from, To: to, Cc: ccAddresses(to, cc), ReplyTo: s.replyTo,
		Subject: subject, Text: body})
}
```

In `internal/infra/mailer/change_email.go`, replace `SendEmailChangeNotice`:

```go
// SendEmailChangeNotice emails the OLD address that a change was requested,
// naming the proposed new address so an unwanted change is noticeable. cc adds
// the user's linked-provider addresses; the proposed NEW address is deliberately
// not among them — it has proved nothing yet.
func (s *ChangeEmailSender) SendEmailChangeNotice(ctx context.Context, to, name, newEmail string, cc []string) error {
	lang := reqctx.Language(ctx)
	subject := i18n.T(lang, "emails.change_email_notice.subject", nil)
	body := i18n.T(lang, "emails.change_email_notice.body", map[string]any{"name": name, "email": newEmail})
	return s.m.Send(ctx, Message{From: s.from, To: to, Cc: ccAddresses(to, cc), ReplyTo: s.replyTo,
		Subject: subject, Text: body})
}
```

Leave `SendEmailChangeCode` exactly as it is.

- [x] **Step 4: Update the three existing call sites to pass `nil`**

Tasks 3 and 4 replace these `nil`s with real lists; passing `nil` here keeps the build and the whole suite green between commits.

`internal/server/glue_oauth_notifier.go`, the last line of `IdentityLinked`:

```go
	return a.mail.SendIdentityLinked(ctx, email, u.Name, providerName, lang, nil)
```

`internal/user/change_email.go:57`:

```go
		if nerr := s.changeMailer.SendEmailChangeNotice(ctx, strings.TrimSpace(currentEmail), u.Name, newEmail, nil); nerr != nil {
```

`internal/server/glue_oauth_notifier_test.go` — there are no direct `SendIdentityLinked` calls in that file (it drives the notifier), so nothing to change. Then update the three direct sender calls in `internal/infra/mailer/mailer_test.go`'s existing tests (`TestIdentityLinkedSender`, `TestIdentityLinkedSender_LangFallsBackToRequestLanguage`, `TestIdentityLinkedEmailEnglishUnchanged`) by appending `, nil` to each.

- [x] **Step 5: Run the affected packages to verify they pass**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go build ./... && GOTOOLCHAIN=go1.27.1 go test ./internal/infra/mailer/ ./internal/user/ ./internal/server/
```
Expected: PASS everywhere.

- [x] **Step 6: Commit**

```bash
git add internal/infra/mailer internal/server/glue_oauth_notifier.go internal/user/change_email.go
git commit -m "feat(mailer): let the notice senders take a CC list

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

## Task 3: Linked-identity notice CCs the user's provider addresses

**Files:**
- Modify: `internal/oauth/identities.go`
- Create: `internal/oauth/identities_test.go` (if the file does not exist; otherwise append)
- Modify: `internal/server/glue_oauth_notifier.go`
- Modify: `internal/server/glue_oauth_notifier_test.go`

**Interfaces:**
- Consumes: `SendIdentityLinked(ctx, to, name, provider, lang string, cc []string) error` from Task 2.
- Produces: `func (s *Service) ListIdentityEmails(ctx context.Context, userID vo.Id) ([]string, error)` on `*oauth.Service` — also consumed by Task 4.

- [x] **Step 1: Write the failing test for `ListIdentityEmails`**

Create `internal/oauth/identities_test.go` (or append if it already exists). Match the package clause of any existing test file in `internal/oauth`; if the package has no test file yet, use `package oauth_test` and import the service as `appoauth "github.com/econumo/econumo/internal/oauth"`. Build the service the same way the nearest existing oauth test does. The behaviour to pin:

```go
// ListIdentityEmails returns one address per linked provider, skipping rows
// whose provider reported no address (users_identities.email defaults to "").
func TestListIdentityEmails(t *testing.T) {
	// Arrange: a user with two identities, one of which has an empty email.
	//   google -> "alice@gmail.test"
	//   oidc   -> ""
	// Act:    svc.ListIdentityEmails(ctx, userID)
	// Assert: exactly []string{"alice@gmail.test"}.
	//
	// And for a user with no identities at all: a nil/empty slice and no error.
}
```

Fill the arrange/act/assert in with the seeding helpers the neighbouring oauth tests use (`dbtest.NewSQLite` + `fixture.New` + `oauthrepo.NewIdentityRepo`), following whichever pattern that file already establishes.

- [x] **Step 2: Run it to verify it fails**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go test ./internal/oauth/ -run TestListIdentityEmails -v
```
Expected: FAIL to compile — `svc.ListIdentityEmails undefined`.

- [x] **Step 3: Implement `ListIdentityEmails`**

In `internal/oauth/identities.go`, directly below `ListIdentities`:

```go
// ListIdentityEmails returns the address each linked provider vouched for, for
// CC'ing the account's notice emails. Rows whose provider reported no address
// are skipped (the column defaults to ""). Unlike ListIdentities this is not a
// wire DTO: the caller is the mail path, not an HTTP handler.
func (s *Service) ListIdentityEmails(ctx context.Context, userID vo.Id) ([]string, error) {
	rows, err := s.identities.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if strings.TrimSpace(r.Email) == "" {
			continue
		}
		out = append(out, r.Email)
	}
	return out, nil
}
```

`strings` is already imported in that file.

- [x] **Step 4: Run it to verify it passes**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go test ./internal/oauth/ -run TestListIdentityEmails -v
```
Expected: PASS.

- [x] **Step 5: Write the failing notifier test**

Append to `internal/server/glue_oauth_notifier_test.go`. `NewOAuthNotifier` grows a third parameter in Step 6, so this test also pins the new constructor shape:

```go
// identityEmailsStub stands in for the oauth service's ListIdentityEmails: the
// notifier only needs the addresses, and wiring a whole oauth service here
// would test the oauth repo, not the glue.
type identityEmailsStub struct {
	emails []string
	err    error
}

func (s identityEmailsStub) ListIdentityEmails(_ context.Context, _ vo.Id) ([]string, error) {
	return s.emails, s.err
}

func TestOAuthNotifier_IdentityLinked_CcsTheLinkedAddresses(t *testing.T) {
	db := dbtest.NewSQLite(t)
	fx := fixture.New(t, db)
	userID := fx.User(fixture.User{Email: "owner3@example.test", Name: "Cid", Algorithm: "argon2id"})

	userSvc, _ := newTestUserSvc(db)
	c := &captureMailer{}
	sender := mailer.NewIdentityLinkedSender(c, "from@econumo.test", "reply@econumo.test")
	// The primary address is among the linked ones, as it is whenever the
	// account was created through a provider; it must not be duplicated.
	emails := identityEmailsStub{emails: []string{"owner3@example.test", "cid@gmail.test"}}
	notifier := NewOAuthNotifier(userSvc, sender, emails)

	if err := notifier.IdentityLinked(context.Background(), vo.MustParseId(userID), "Google"); err != nil {
		t.Fatalf("IdentityLinked: %v", err)
	}
	if c.msg.To != "owner3@example.test" {
		t.Errorf("To = %q", c.msg.To)
	}
	if len(c.msg.Cc) != 1 || c.msg.Cc[0] != "cid@gmail.test" {
		t.Errorf("Cc = %v, want the other linked address only", c.msg.Cc)
	}
}

func TestOAuthNotifier_IdentityLinked_StillSendsWhenTheLookupFails(t *testing.T) {
	db := dbtest.NewSQLite(t)
	fx := fixture.New(t, db)
	userID := fx.User(fixture.User{Email: "owner4@example.test", Name: "Dee", Algorithm: "argon2id"})

	userSvc, _ := newTestUserSvc(db)
	c := &captureMailer{}
	sender := mailer.NewIdentityLinkedSender(c, "from@econumo.test", "reply@econumo.test")
	notifier := NewOAuthNotifier(userSvc, sender, identityEmailsStub{err: errors.New("boom")})

	// The CC list is a nicety; losing it must not cost the user the notice.
	if err := notifier.IdentityLinked(context.Background(), vo.MustParseId(userID), "Google"); err != nil {
		t.Fatalf("IdentityLinked: %v", err)
	}
	if !c.called || c.msg.To != "owner4@example.test" || len(c.msg.Cc) != 0 {
		t.Errorf("message = To %q Cc %v called=%v", c.msg.To, c.msg.Cc, c.called)
	}
}
```

Add `"errors"` to that file's imports.

The two pre-existing tests in the file (`..._RendersInTheStoredLanguage`, `..._DefaultsToEnglish`) call `NewOAuthNotifier(userSvc, sender)` — update both to `NewOAuthNotifier(userSvc, sender, identityEmailsStub{})`.

- [x] **Step 6: Run it to verify it fails**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go test ./internal/server/ -run TestOAuthNotifier -v
```
Expected: FAIL to compile — "too many arguments in call to NewOAuthNotifier".

- [x] **Step 7: Implement the notifier change**

Replace the body of `internal/server/glue_oauth_notifier.go` below the imports:

```go
// identityEmailLister is the notifier's half of the oauth service: the linked
// addresses to copy the notice to. An interface rather than *appoauth.Service
// so the glue's own test can drive the failure path.
type identityEmailLister interface {
	ListIdentityEmails(ctx context.Context, userID vo.Id) ([]string, error)
}

type OAuthNotifier struct {
	users      *appuser.Service
	mail       *mailer.IdentityLinkedSender
	identities identityEmailLister
}

var _ appoauth.Notifier = (*OAuthNotifier)(nil)

func NewOAuthNotifier(users *appuser.Service, mail *mailer.IdentityLinkedSender, identities identityEmailLister) *OAuthNotifier {
	return &OAuthNotifier{users: users, mail: mail, identities: identities}
}

// IdentityLinked loads the account owner's plaintext email and stored
// language and sends the notice in that language (falling back to the
// callback request's language when none is stored yet), copied to the
// account's other linked-provider addresses. Resolving those is best-effort:
// the notice matters more than the copy list, so a lookup failure still sends.
func (a *OAuthNotifier) IdentityLinked(ctx context.Context, userID vo.Id, providerName string) error {
	u, email, err := a.users.AdminUserByID(ctx, userID)
	if err != nil {
		return err
	}
	lang, err := a.users.GetLanguage(ctx, userID)
	if err != nil {
		lang = ""
	}
	var cc []string
	if a.identities != nil {
		if addrs, lerr := a.identities.ListIdentityEmails(ctx, userID); lerr == nil {
			cc = addrs
		}
	}
	return a.mail.SendIdentityLinked(ctx, email, u.Name, providerName, lang, cc)
}
```

In `internal/server/server.go:219`, pass the oauth service as the third argument:

```go
	oauthSvc.SetNotifier(NewOAuthNotifier(userSvc, identityLinkedMailer, oauthSvc))
```

- [x] **Step 8: Run the tests to verify they pass**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go build ./... && GOTOOLCHAIN=go1.27.1 go test ./internal/oauth/ ./internal/server/
```
Expected: PASS.

- [x] **Step 9: Commit**

```bash
git add internal/oauth internal/server/glue_oauth_notifier.go internal/server/glue_oauth_notifier_test.go internal/server/server.go
git commit -m "feat(oauth): copy the identity-linked notice to every linked address

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

## Task 4: Change-email notice CCs the user's provider addresses

**Files:**
- Modify: `internal/user/ports.go`
- Modify: `internal/user/usecase.go`
- Modify: `internal/user/change_email.go`
- Modify: `internal/user/change_email_integration_test.go`
- Create: `internal/server/glue_user_identityemails.go`
- Create: `internal/server/glue_user_identityemails_test.go`
- Modify: `internal/server/server.go`

**Interfaces:**
- Consumes: `SendEmailChangeNotice(ctx, to, name, newEmail string, cc []string) error` (Task 2); `(*oauth.Service).ListIdentityEmails` (Task 3).
- Produces:
  - `type user.IdentityEmailLister interface { ListEmails(ctx context.Context, userID vo.Id) ([]string, error) }`
  - `func (s *user.Service) SetIdentityEmailLister(l IdentityEmailLister)`
  - `func server.NewIdentityEmailLister(svc *appoauth.Service) appuser.IdentityEmailLister`

- [x] **Step 1: Write the failing test**

Append to `internal/user/change_email_integration_test.go`. Use `newChangeEmailEnv(t)` exactly as the existing tests in that file do, then install a stub lister before requesting the change:

```go
// stubIdentityEmails is the oauth feature's lister as the user service sees it.
type stubIdentityEmails struct {
	emails []string
	err    error
}

func (s stubIdentityEmails) ListEmails(_ context.Context, _ vo.Id) ([]string, error) {
	return s.emails, s.err
}

func TestRequestEmailChange_NoticeCcsLinkedAddresses(t *testing.T) {
	svc, repo, tokens, _, _, cap, _ := newChangeEmailEnv(t)
	_ = repo
	_ = tokens
	// Seed a user through the same helper the neighbouring tests use, then:
	svc.SetIdentityEmailLister(stubIdentityEmails{emails: []string{"alice@gmail.test"}})

	// Request the change with the correct password and a fresh new address.
	// The LAST captured message is the notice to the OLD address (the code to
	// the new address is sent first, inside issueEmailChangeCode).
	// Assert:
	//   cap.msg.To  == the old address
	//   cap.msg.Cc  == []string{"alice@gmail.test"}
	_ = cap
	_ = svc
}

func TestRequestEmailChange_CodeToNewAddressIsNeverCcd(t *testing.T) {
	// Same setup, but capture the FIRST message (the code to the new address)
	// and assert len(msg.Cc) == 0 — the code proves control of the new mailbox,
	// so copying it anywhere defeats the check.
}

func TestRequestEmailChange_SendsTheNoticeWhenTheListerFails(t *testing.T) {
	// Same setup with stubIdentityEmails{err: errors.New("boom")}.
	// Assert the notice is still sent, To the old address, with no CCs.
}
```

Fill in the seeding and the request call by copying the arrange block of the nearest existing `RequestEmailChange` test in the same file (it already has a user, a password, and a `model.RequestEmailChangeRequest`). The capture helper in that file records only the *last* message, so for `..._CodeToNewAddressIsNeverCcd` either read the captured message between the two sends or extend the local capture type to keep a slice — whichever the file's existing style makes cleaner.

- [x] **Step 2: Run it to verify it fails**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go test ./internal/user/ -run TestRequestEmailChange -v
```
Expected: FAIL to compile — `svc.SetIdentityEmailLister undefined`.

- [x] **Step 3: Add the port**

Append to `internal/user/ports.go`, directly after the `OAuthReclaimer` block:

```go
// IdentityEmailLister is the oauth feature's list of addresses the user's
// linked providers vouched for. The change-email NOTICE is copied to them so a
// change requested without the user's knowledge is noticeable even when their
// primary mailbox is not the one they read; the change-email CODE is not, since
// it exists to prove control of the proposed new address. nil disables the copy
// (CLI, tests).
type IdentityEmailLister interface {
	ListEmails(ctx context.Context, userID vo.Id) ([]string, error)
}
```

- [x] **Step 4: Add the field and setter**

In `internal/user/usecase.go`, add to the `Service` struct after `oauthGrants`:

```go
	identityEmails      IdentityEmailLister
```

and after `SetOAuthReclaimer`:

```go
// SetIdentityEmailLister installs the oauth feature's linked-address lookup for
// the change-email notice, wired after construction like the two adapters above.
func (s *Service) SetIdentityEmailLister(l IdentityEmailLister) { s.identityEmails = l }
```

Leave `NewService`'s parameter list alone.

- [x] **Step 5: Use it in the notice**

In `internal/user/change_email.go`, replace the notice block at line 56-60:

```go
	if s.changeMailer != nil {
		oldEmail := strings.TrimSpace(currentEmail)
		if nerr := s.changeMailer.SendEmailChangeNotice(ctx, oldEmail, u.Name, newEmail, s.linkedEmails(ctx, userID)); nerr != nil {
			return nil, nerr
		}
	}
```

If the enclosing method's user-id variable is not named `userID`, use whatever it is named (it is the id already in scope for `issueEmailChangeCode`). Add the helper at the bottom of the same file:

```go
// linkedEmails resolves the addresses the user's providers vouched for. The
// copy list is a nicety, so a lookup failure yields none rather than failing
// the notice the user is waiting on.
func (s *Service) linkedEmails(ctx context.Context, userID vo.Id) []string {
	if s.identityEmails == nil {
		return nil
	}
	addrs, err := s.identityEmails.ListEmails(ctx, userID)
	if err != nil {
		return nil
	}
	return addrs
}
```

- [x] **Step 6: Run the user tests to verify they pass**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go test ./internal/user/ -v -run TestRequestEmailChange
```
Expected: PASS.

- [x] **Step 7: Write the failing glue test**

Create `internal/server/glue_user_identityemails_test.go`:

```go
package server

import (
	"context"
	"testing"

	appuser "github.com/econumo/econumo/internal/user"
)

// The adapter exists only to satisfy the user feature's port over the oauth
// service; the assertion that matters is that it still does.
func TestIdentityEmailLister_SatisfiesTheUserPort(t *testing.T) {
	var _ appuser.IdentityEmailLister = oauthIdentityEmails{}
}

func TestIdentityEmailLister_NilServiceIsNotConstructed(t *testing.T) {
	l := NewIdentityEmailLister(nil)
	if l == nil {
		t.Fatal("NewIdentityEmailLister should always return an adapter")
	}
	_ = context.Background()
}
```

- [x] **Step 8: Run it to verify it fails**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go test ./internal/server/ -run TestIdentityEmailLister -v
```
Expected: FAIL to compile — `undefined: oauthIdentityEmails`, `undefined: NewIdentityEmailLister`.

- [x] **Step 9: Create the glue adapter**

Create `internal/server/glue_user_identityemails.go`:

```go
package server

import (
	"context"

	appoauth "github.com/econumo/econumo/internal/oauth"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

// oauthIdentityEmails adapts the oauth service to the user feature's
// IdentityEmailLister port (the change-email notice is copied to every address
// the user's providers vouched for).
type oauthIdentityEmails struct{ oauth *appoauth.Service }

var _ appuser.IdentityEmailLister = oauthIdentityEmails{}

func (a oauthIdentityEmails) ListEmails(ctx context.Context, userID vo.Id) ([]string, error) {
	if a.oauth == nil {
		return nil, nil
	}
	return a.oauth.ListIdentityEmails(ctx, userID)
}

func NewIdentityEmailLister(svc *appoauth.Service) appuser.IdentityEmailLister {
	return oauthIdentityEmails{oauth: svc}
}
```

- [x] **Step 10: Wire it**

In `internal/server/server.go`, beside the existing setters at line 217-219:

```go
	userSvc.SetLogoutURLBuilder(oauthLogoutURLs{oauth: oauthSvc})
	userSvc.SetOAuthReclaimer(NewOAuthReclaimer(oauthSvc))
	userSvc.SetIdentityEmailLister(NewIdentityEmailLister(oauthSvc))
	oauthSvc.SetNotifier(NewOAuthNotifier(userSvc, identityLinkedMailer, oauthSvc))
```

The CLI container (`internal/cli/container.go`) is deliberately left alone: it wires no `ChangeEmailSender`, so it sends no notice and needs no lister.

- [x] **Step 11: Run the tests to verify they pass**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 \
  go build ./... && GOTOOLCHAIN=go1.27.1 go test ./internal/user/ ./internal/server/ ./internal/oauth/
```
Expected: PASS.

- [x] **Step 12: Commit**

```bash
git add internal/user internal/server
git commit -m "feat(user): copy the change-email notice to every linked address

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

## Task 5: Docs and the full gate

**Files:**
- Modify: `docs/regression-test-plan.md`
- Modify: `CLAUDE.md`

- [x] **Step 1: Update the regression test plan**

CLAUDE.md requires this for any user-observable change. In `docs/regression-test-plan.md`, extend the existing auto-link item (around line 124-129, "Sign-in through a provider whose verified email matches an existing PASSWORDLESS account …") so its last clause reads:

```
      owner's notice email (console transport prints it to server stdout in
      dev) naming the provider, addressed to the account email and CC'd to
      every OTHER address the account's linked providers reported (the
      account email itself is never duplicated into the Cc).
```

And extend the **Change email** item (around line 473-475) to:

```
- [ ] **Change email**: request (new email + password) → code sent to the new
      address → confirm; resend with cooldown; wrong code rejected; login works
      with the new email only. The heads-up notice to the OLD address is CC'd
      to the account's linked-provider addresses; the CODE to the new address
      is sent to that address alone, with no Cc.
```

- [x] **Step 2: Update CLAUDE.md**

In the "Notable behaviours" section, directly after the **OAuth email drift** bullet, add:

```markdown
- **Notice emails reach every linked address**: the two NOTICES — the
  identity-linked notice and the change-email heads-up to the old address — are
  CC'd to the addresses the user's linked providers reported
  (`users_identities.email`, via `oauth.Service.ListIdentityEmails`; the user
  feature reads it through the `IdentityEmailLister` port). The three CODE
  emails are not: a reset, verification or change-email code exists to prove
  control of one specific mailbox, so copying it elsewhere would defeat the
  check. Resolving the list is best-effort — a lookup failure sends the notice
  to the primary address alone rather than failing it.
```

- [x] **Step 3: Run the smoke gate**

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 make go-test
```
Expected: PASS — build, vet, gofmt, OpenAPI-docs-fresh, the sqlite unit/integration suite (including `apiparity` and `mcpparity` goldens, which must need no regeneration), and the `GO_COVER_MIN=80` coverage gate.

- [x] **Step 4: Run the engine-comparison tier**

No SQL changed, so this is a regression check rather than new coverage, but the plan is not done without it:

```bash
export PATH=/usr/local/go/bin:$PATH && GOTOOLCHAIN=go1.27.1 make test
```
Expected: PASS, including `test-repo-pgsql` and the `enginecompare` suite. If no PostgreSQL is reachable, say so explicitly in the completion report rather than reporting the tier as passed.

- [x] **Step 5: Commit and push**

```bash
git add docs/regression-test-plan.md CLAUDE.md
git commit -m "docs: record the notice-email CC behaviour

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
git push -u origin feature/cc-linked-oauth-emails
```

---

## Self-Review

**Spec coverage.** Every row of the Design table maps to a task: the two CC'd notices to Tasks 3 and 4, the three excluded codes to the explicit assertions in Tasks 2 and 4. The transport work is Task 1, the docs obligation Task 5.

**Placeholders.** Task 3 Step 1 and Task 4 Step 1 give the test *intent* and assertions but delegate the arrange block to the neighbouring tests' existing helpers, because those helpers (`newChangeEmailEnv`, the oauth package's seeding pattern) are long and already established — copying them verbatim here would rot. Every other step carries the literal code.

**Type consistency.** The lister is spelled two different ways on purpose, and this is the one thing to keep straight while implementing:
- oauth's own method: `ListIdentityEmails(ctx, userID) ([]string, error)` — Task 3.
- the user feature's port method: `ListEmails(ctx, userID) ([]string, error)` — Task 4; the glue adapter in Task 4 Step 9 bridges the two names.
- the server-internal notifier interface: `identityEmailLister` with `ListIdentityEmails`, satisfied by `*oauth.Service` directly — Task 3 Step 7.


---

## As built (2026-09-20)

Every task landed; the smoke gate and both engine tiers are green. Where the executed code diverges from the task bodies above, it is because #267 renamed or reshaped the thing the step described:

**Task 2 — notice senders.** `identity_linked.go` is now `identity.go` and the sender is `IdentitySender`, with `SendIdentityLinked` and `SendIdentityUnlinked` both delegating to one unexported `send`. So the `cc []string` parameter was added to all three: both exported methods and the shared `send`, which is the single place `Cc: ccAddresses(to, cc)` is set. The plan predicted one method on `IdentityLinkedSender`; three signatures changed instead of one.

**Task 3 — the notifier.** #267 collapsed the glue onto a shared `notify(ctx, userID, providerName, send)` helper behind both `IdentityLinked` and `IdentityUnlinked`. The CC lookup therefore went into `notify` — resolved once, used by both notices — rather than into a single `IdentityLinked` body. The `send` parameter's function type grew the `cc []string` argument to match. `NewOAuthNotifier` took the third parameter as planned, and all four pre-existing tests in the file were updated to pass `identityEmailsStub{}`.

**Task 3 — tests.** The new CC assertions are one table-driven test (`TestOAuthNotifier_IdentityNotices_CcTheLinkedAddresses`) covering the linked and unlinked notices as subtests, since both now run the same code path. `internal/oauth/identities_test.go` was created against the existing `newHarness` / `saveIdentity` / `users.seed` helpers in `service_test.go` rather than hand-rolled seeding.

**Task 4 — unchanged.** The port, the setter, the `linkedEmails` helper, the glue adapter and the wiring all landed exactly as written. The change-email tests assert on `captureMailer.msgs` (the user package's capture keeps every message, not just the last), so one request checks both the un-CC'd code and the CC'd notice.

**Verification.** `make go-test`: PASS, total coverage 84.3% (min 80), no golden regeneration needed. `make test`: the Go tiers (including `test-repo-pgsql` and `enginecompare`) PASS against a throwaway `postgres:17-alpine`; the frontend suite reports 1244/1245, its one failure being the pre-existing `web/src/api/transaction.test.ts` jsdom Blob-identity assertion on a file this branch does not touch.
