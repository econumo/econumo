# HTML Emails Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Send every transactional email as HTML (the Black Friday campaign look, light
only) plus a plain-text part, with the greeting, code box and footer owned by one template.

**Architecture:** Senders build a `Content` value from the catalogues; `RenderText` and
`RenderHTML` (an embedded `html/template` layout plus an embedded logo PNG) turn it into
the two parts of a `Message`. Resend sends `Html`+`Text`+an inline attachment; SMTP builds
`multipart/alternative` with a `multipart/related` HTML part. `WithAppLink` is replaced by
an `appURL` passed to the senders.

**Tech Stack:** Go stdlib (`html/template`, `embed`, `mime/multipart`,
`mime/quotedprintable`, `encoding/base64`), `resend-go/v3` v3.17.0 (`Attachment.ContentId`).

**Spec:** `docs/superpowers/specs/2026-11-27-html-emails-design.md`

## Global Constraints

- Light theme only: `<meta name="color-scheme" content="light">` and
  `<meta name="supported-color-schemes" content="light">`; no dark styles.
- Palette: page `#faf6ec`, card `#ffffff` radius 16 width 600, heading/code `#1f1b13`,
  body `#4a4436`, note `#7a6a45`, footer muted `#8a8270`, gold label `#a87a00`, code box
  `#fdf7e4` + 1px `#f1e3b4` radius 12, divider `#ebe5d6`, button `#f6f2e8` + 1px
  `#ebe5d6` + text `#1f1b13` radius 8.
- No remote resources: the only image is the inline logo, `cid:econumo-logo`.
- Every catalogue or user string goes through `html/template` escaping; no catalogue
  string carries markup.
- The plain text always carries the code line `emails.common.code_text`
  (en: `Your confirmation code is: {code}.`), unchanged from today's wording.
- No button and no URL line when `ECONUMO_URL` is empty.
- All 11 catalogues carry every key, in each catalogue's register (de Sie, nl u,
  fr vous, pt European).
- Comments only for non-obvious why (CLAUDE.md rule).

## Review Focus

1. A name or provider with `<`, `&` or quotes: escaped in HTML, literal in text
   (Task 3 test `TestRenderHTML_EscapesUserStrings`).
2. An empty name: greeting falls back to `emails.common.greeting_anonymous`
   (Task 1 test `TestCompose_EmptyNameUsesAnonymousGreeting`).
3. No `ECONUMO_URL`: no button in HTML, no URL line in text
   (Task 1 `TestRenderText_NoAppURL`, Task 3 `TestRenderHTML_NoAppURLNoButton`).
4. A long unbroken address in the change-email notice must not widen the card on a
   phone: body paragraphs carry `word-break:break-word`
   (Task 3 `TestRenderHTML_BodyWraps` asserts the style is present).
5. SMTP output parses as MIME with CRLF line endings, the text part decodes to the
   exact `RenderText` output, and the HTML's `cid:` matches the image part's
   `Content-ID` (Task 5 `TestSMTPRender_MultipartParses`).

---

### Task 1: Content model, plain text, senders, English catalogue

**Files:**
- Create: `internal/infra/mailer/content.go`, `internal/infra/mailer/content_test.go`
- Modify: `internal/infra/mailer/{reset,verify,change_email,identity,mailer}.go`,
  `internal/infra/mailer/mailer_test.go`, `locales/en.json`, `internal/server/server.go`,
  `internal/server/glue_oauth_notifier_test.go` and every other caller of the four
  `New*Sender` constructors, `internal/user/change_email_integration_test.go`

**Interfaces:**
- Produces:
  - `type Content struct { Lang, Heading, Greeting, Body, Code, CodeLabel, CodeText, Note, Tagline, OpenApp, AppURL string }`
  - `func Compose(lang, key string, params map[string]any, name, code, appURL string) Content`
    reads `emails.<key>.{heading,body,note}` with `params`, `emails.common.greeting`
    (`{name}`) or `greeting_anonymous` when `strings.TrimSpace(name)==""`, and when
    `code != ""` `code_label` and `code_text` (`{code}`); `tagline`, `open_app`.
  - `func RenderText(c Content) string`
  - `func (c Content) Paragraphs() []string` — `Body` split on `"\n\n"`, trimmed, empties dropped.
  - Constructors gain a trailing `appURL string`:
    `NewResetSender(m Mailer, from, replyTo, appURL string)`, same for
    `NewVerifySender`, `NewChangeEmailSender`, `NewIdentitySender`.
  - `EmailKeys` lists the common keys and `subject, heading, body, note` of all six emails.
  - `WithAppLink` and `linkMailer` are deleted.

Plain-text shape (exact, `\n` line endings; blocks for empty fields are omitted with
their blank line):

```
{Greeting}

{Body}

{CodeText}

{Note}

--
Econumo — {Tagline}
{AppURL}
```

English catalogue (`locales/en.json`, replaces the `emails` object; keep `subject`s as today):

| key | en |
|---|---|
| common.greeting | `Hi {name},` |
| common.greeting_anonymous | `Hi,` |
| common.code_label | `Confirmation code` |
| common.code_text | `Your confirmation code is: {code}.` |
| common.tagline | `Manage money. Together.` |
| common.open_app | `Open Econumo` |
| reset.heading / body / note | `Reset your password` / `Enter this code in Econumo to choose a new password.` / `If you didn't request this code, you can ignore this email. Your password stays the same.` |
| verify | `Verify your email` / `Enter this code on the sign-in screen to verify your email address.` / `If you didn't create an Econumo account, you can ignore this email.` |
| change_email | `Confirm your new email` / `Use this code to confirm your new email address. It expires in 10 minutes.` / `If you didn't request this, you can ignore this email.` |
| change_email_notice | `Email change requested` / `Someone requested to change the email on your account to {email}.` / `If this wasn't you, change your password right away.` |
| identity_linked | `New sign-in method added` / `Your {provider} account was just linked to your Econumo account and can now be used to sign in.` / `If this wasn't you, unlink it in Settings and change your password.` |
| identity_unlinked | `Sign-in method removed` / `Your {provider} account was just unlinked from your Econumo account and can no longer be used to sign in.` / `If this wasn't you, change your password and review the linked accounts in Settings.` |

- [ ] **Step 1: Write the failing tests** in `content_test.go`:
  - `TestRenderText_ResetEnglish`: `RenderText(Compose("en","reset",nil,"Anna","482913","https://app.econumo.test"))` equals
    `"Hi Anna,\n\nEnter this code in Econumo to choose a new password.\n\nYour confirmation code is: 482913.\n\nIf you didn't request this code, you can ignore this email. Your password stays the same.\n\n--\nEconumo — Manage money. Together.\nhttps://app.econumo.test\n"`.
  - `TestRenderText_NoAppURL`: same with `appURL ""` ends in `"Together.\n"`, no `http`.
  - `TestRenderText_NoticeHasNoCodeLine`: `Compose("en","identity_linked",{"provider":"Google"},"Anna","",u)` text has no `confirmation code`, contains `Your Google account`.
  - `TestCompose_EmptyNameUsesAnonymousGreeting`: name `"  "` → `Greeting == "Hi,"`.
  - `TestCompose_RussianCode`: `Compose("ru","reset",nil,"Анна","482913","")` text contains `482913` (fails until Task 2 only if ru lacks keys; it falls back to en, so it passes — keep it as a smoke test).
  - Update `mailer_test.go`: sender tests construct with the extra `appURL` and assert `msg.Text` via `RenderText`; delete `TestWithAppLink`; the `...EnglishUnchanged` tests become golden-free equality on the new shape.
- [ ] **Step 2:** `go test ./internal/infra/mailer/` → FAIL (undefined `Compose`).
- [ ] **Step 3:** Implement `content.go`, switch the four senders to
  `c := Compose(lang, key, params, name, code, s.appURL)` and
  `Message{..., Subject: i18n.T(lang, "emails."+key+".subject", nil), Text: RenderText(c)}`;
  update `locales/en.json`, `EmailKeys`, `server.go` (pass `cfg.AppURL`, drop the
  `WithAppLink` block), the other constructor callers, and `changeCodeFrom`'s marker to
  `"code is: "`.
- [ ] **Step 4:** `go test ./internal/infra/mailer/ ./internal/server/ ./internal/user/... ./internal/test/apiparity/` → PASS.
  `go test ./internal/test/i18ntest/` is expected to FAIL on key parity until Task 2.
- [ ] **Step 5: Commit** `feat(mailer): Compose emails from parts; greeting and footer in the template`

### Task 2: Translations for the ten other catalogues

**Files:** Modify `locales/{de,es,fr,it,nl,pl,pt,ru,uk,zh}.json`

**Interfaces:** Consumes the key set from Task 1's table.

- [ ] **Step 1:** `go test ./internal/test/i18ntest/` → FAIL (missing keys).
- [ ] **Step 2:** Replace each catalogue's `emails` object with the Task 1 key set,
  translated in that catalogue's existing register; reuse each catalogue's current
  wording where the meaning is unchanged (subjects stay as they are). Placeholders
  `{name} {code} {email} {provider}` exactly where en has them.
- [ ] **Step 3:** `go test ./internal/test/i18ntest/ ./internal/infra/mailer/` → PASS.
- [ ] **Step 4: Commit** `feat(i18n): HTML email copy in all languages`

### Task 3: HTML layout and logo

**Files:**
- Create: `internal/infra/mailer/templates/layout.html`, `internal/infra/mailer/templates/logo.png`,
  `internal/infra/mailer/html.go`, `internal/infra/mailer/html_test.go`,
  `internal/infra/mailer/testdata/golden/<email>.html` and `<email>.txt` for the six emails
- Copy: the logo from `/tmp/bfmail/logo/badge.png` (552x121 PNG). If it is gone, fetch
  `https://resend-attachments.s3.amazonaws.com/4MBvoAasoGDoRjnKQBwkPm/8ec248eb-4bdb-459a-b3c0-1f7ab72f0378`.

**Interfaces:**
- Produces: `const LogoContentID = "econumo-logo"`, `var Logo []byte` (embedded),
  `func RenderHTML(c Content) (string, error)`.

`layout.html` (the approved mock, light only; exact markup):

```html
<!DOCTYPE html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<meta name="color-scheme" content="light"><meta name="supported-color-schemes" content="light">
<meta name="x-apple-disable-message-reformatting">
<title>{{.Heading}}</title>
<style>
body,table,td,a,p,h1{font-family:'Plus Jakarta Sans',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;}
@media (max-width:660px){
 .container{width:100%!important;border-radius:0!important;}
 .px{padding-left:20px!important;padding-right:20px!important;}
 .px-logo{padding-left:9px!important;}
 .h1{font-size:26px!important;line-height:32px!important;}
 .code{font-size:30px!important;letter-spacing:6px!important;}
}
</style></head>
<body style="margin:0;padding:0;background:#faf6ec;">
<div style="display:none;max-height:0;overflow:hidden;mso-hide:all;color:#faf6ec;">{{if .Code}}{{.CodeText}}{{else}}{{.Body}}{{end}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#faf6ec;"><tr><td align="center" style="padding:28px 0;">
<table role="presentation" class="container" width="600" cellpadding="0" cellspacing="0" border="0" style="width:600px;max-width:600px;background:#ffffff;border-radius:16px;color:#1f1b13;">
<tr><td class="px px-logo" style="padding:28px 40px 0 29px;"><img src="cid:econumo-logo" width="150" height="33" alt="Econumo" style="display:block;border:0;"></td></tr>
<tr><td class="px" style="padding:32px 40px 0;">
 <h1 class="h1" style="margin:0 0 14px;font-size:30px;line-height:36px;font-weight:800;letter-spacing:-0.5px;color:#1f1b13;">{{.Heading}}</h1>
 <p style="margin:0 0 12px;font-size:17px;line-height:27px;color:#4a4436;">{{.Greeting}}</p>
 {{range .Paragraphs}}<p style="margin:0 0 12px;font-size:17px;line-height:27px;color:#4a4436;word-break:break-word;">{{.}}</p>{{end}}
</td></tr>
{{if .Code}}<tr><td class="px" style="padding:12px 40px 0;">
 <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#fdf7e4;border:1px solid #f1e3b4;border-radius:12px;"><tr><td align="center" style="padding:22px 16px;">
  <div style="font-size:12px;line-height:16px;font-weight:800;letter-spacing:1.2px;text-transform:uppercase;color:#a87a00;">{{.CodeLabel}}</div>
  <div class="code" style="margin-top:6px;font-family:'SFMono-Regular',Menlo,Consolas,'Liberation Mono',monospace;font-size:36px;line-height:44px;font-weight:700;letter-spacing:8px;color:#1f1b13;">{{.Code}}</div>
 </td></tr></table>
</td></tr>{{end}}
<tr><td class="px" style="padding:20px 40px 0;">
 <p style="margin:0;font-size:14px;line-height:21px;color:#7a6a45;">{{.Note}}</p>
</td></tr>
<tr><td class="px" style="padding:36px 40px 36px;">
 <div style="border-top:1px solid #ebe5d6;padding-top:24px;">
 <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0"><tr>
  <td style="font-size:14px;line-height:21px;color:#8a8270;"><strong style="color:#4a4436;">Econumo</strong><br>{{.Tagline}}</td>
  {{if .AppURL}}<td align="right"><a href="{{.AppURL}}" style="display:inline-block;padding:10px 18px;background:#f6f2e8;border:1px solid #ebe5d6;color:#1f1b13;font-size:15px;font-weight:700;border-radius:8px;text-decoration:none;white-space:nowrap;">{{.OpenApp}}</a></td>{{end}}
 </tr></table></div>
</td></tr>
</table>
</td></tr></table>
</body></html>
```

- [ ] **Step 1: Write the failing tests** in `html_test.go`:
  - `TestRenderHTML_Golden`: for each of the six emails, en, name `Anna`, code
    `482913` for the three code emails, params `{"email":"new@example.test"}` /
    `{"provider":"Google"}`, appURL `https://app.econumo.test`: compare `RenderHTML`
    and `RenderText` with `testdata/golden/<key>.{html,txt}`; `UPDATE_GOLDEN=1` writes them.
  - `TestRenderHTML_EscapesUserStrings`: name `<b>Ann & "Bo"</b>` → HTML contains
    `&lt;b&gt;Ann &amp; &#34;Bo&#34;&lt;/b&gt;` and no `<b>Ann`; `RenderText` contains the literal name.
  - `TestRenderHTML_NoAppURLNoButton`: appURL `""` → no `<a href`, no `Open Econumo`.
  - `TestRenderHTML_BodyWraps`: output contains `word-break:break-word`.
  - `TestRenderHTML_NoRemoteResources`: no `http://` or `https://` inside any `src=`; contains `src="cid:econumo-logo"`.
  - `TestLogoIsPNG`: `Logo` starts with `\x89PNG`.
- [ ] **Step 2:** `go test ./internal/infra/mailer/ -run 'RenderHTML|Logo'` → FAIL.
- [ ] **Step 3:** Implement `html.go` (`//go:embed templates/layout.html`,
  `//go:embed templates/logo.png`, template parsed once at package init with
  `template.Must`), then `UPDATE_GOLDEN=1 go test ./internal/infra/mailer/ -run Golden`
  and read every golden.
- [ ] **Step 4:** `go test ./internal/infra/mailer/` → PASS.
- [ ] **Step 5:** Screenshot `reset.html` and `identity_linked.html` goldens with
  `google-chrome --headless --no-sandbox --hide-scrollbars --force-device-scale-factor=2 --window-size=720,780 --screenshot=...`
  and at `--window-size=390,760` (swap `cid:econumo-logo` for a file path to the logo
  in a scratch copy). Look at them: they must match the approved mock.
- [ ] **Step 6: Commit** `feat(mailer): HTML layout with the inline logo`

### Task 4: Message carries HTML; Resend and console

**Files:** Modify `internal/infra/mailer/mailer.go`, the four sender files, `mailer_test.go`

**Interfaces:**
- Produces: `Message` gains `HTML string` and `Inline []InlineFile`;
  `type InlineFile struct { ContentID, Filename, ContentType string; Data []byte }`.
  Senders set `HTML: RenderHTML(c)` and
  `Inline: []InlineFile{{ContentID: LogoContentID, Filename: "econumo.png", ContentType: "image/png", Data: Logo}}`;
  a `RenderHTML` error is returned from the sender (never sends text-only silently).

- [ ] **Step 1: Write the failing tests:**
  - `TestResendRequest_CarriesHTMLTextAndInlineLogo`: extract the request building
    into `func resendParams(msg Message) *resend.SendEmailRequest`; assert `Html`, `Text`
    non-empty and one attachment with `ContentId == "econumo-logo"`, `ContentType == "image/png"`.
  - `TestResetSender_SendsHTML`: recorded message has `HTML` containing `482913` and `Inline` of length 1.
  - `TestConsole_RendersMessage` stays: console prints `Text` only.
- [ ] **Step 2:** `go test ./internal/infra/mailer/` → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go test ./internal/infra/mailer/` → PASS.
- [ ] **Step 5: Commit** `feat(mailer): Send HTML and the inline logo through Resend`

### Task 5: SMTP multipart

**Files:** Modify `internal/infra/mailer/smtp.go`, `smtp_test.go`

**Interfaces:** Consumes `Message.HTML`, `Message.Inline`.

Structure when `msg.HTML != ""` (text-only messages keep today's single part):

```
Content-Type: multipart/alternative; boundary=A
  text/plain; charset=UTF-8            quoted-printable
  multipart/related; boundary=R; type="text/html"
    text/html; charset=UTF-8           quoted-printable
    image/png                          base64 (76-char lines)
      Content-ID: <econumo-logo>
      Content-Disposition: inline; filename="econumo.png"
```

- [ ] **Step 1: Write the failing test** `TestSMTPRender_MultipartParses`: render a
  message with HTML and the logo; parse with `net/mail.ReadMessage` and
  `mime/multipart`; assert: top type `multipart/alternative`; first part decodes
  (quoted-printable) to exactly the input `Text`; second part `multipart/related` whose
  HTML decodes to the input `HTML`; image part has `Content-Id: <econumo-logo>` and
  base64-decodes to `Logo`; every line ends in `\r\n` and is at most 998 bytes.
  Keep the existing delivery tests passing.
- [ ] **Step 2:** `go test ./internal/infra/mailer/ -run SMTP` → FAIL.
- [ ] **Step 3:** Implement with `mime/multipart.Writer` (`SetBoundary` not needed;
  `CreatePart` with `textproto.MIMEHeader`), a QP writer per text part, and a
  base64 line-wrapping writer for the image. `multipart.Writer` writes `\r\n`.
- [ ] **Step 4:** `go test ./internal/infra/mailer/` → PASS.
- [ ] **Step 5: Commit** `feat(mailer): SMTP sends HTML with the plain-text alternative`

### Task 6: Docs and full check

**Files:** Modify `CLAUDE.md` (the `ECONUMO_URL` line: "shown as the emails' Open
Econumo button and last plain-text line" instead of "Appended to transactional
emails"), `docs/regression-test-plan.md` (forgot-password item at line ~78: the email
arrives as HTML with the logo, heading, code box and Open Econumo button; plain-text
part has the code), the `mailer.go` package comment if it mentions plain text only.

- [ ] **Step 1:** Edit the docs.
- [ ] **Step 2:** `make go-test` → PASS (build, vet, gofmt, OpenAPI fresh, sqlite tests, coverage gate).
- [ ] **Step 3: Commit** `docs: HTML emails in CLAUDE.md and the regression plan`
