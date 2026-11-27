# HTML emails

## Intent

Econumo's transactional emails go out as bare plain text. They should look like the
product: the layout of the Black Friday 2026 campaign email (cream page, white rounded
card, badge logo, heavy dark headings, warm grey body, gold accents). Self-hosted
instances send them too, so nothing may depend on a remote server.

## Scope

The six existing emails: reset code, verification code, change-email code,
change-email notice, sign-in method linked, sign-in method unlinked. Every future email
uses the same layout.

Non-goals: marketing or lifecycle emails, per-instance branding, web fonts.
Anti-goals: tracking pixels or any remote resource; a plain-text part that drifts from
the HTML; a code that gets lost in a client that blocks HTML.

## Approaches

1. **Go `html/template` layout embedded in the binary** (picked). One hand-written
   table-based layout with inline styles, filled from the catalogues. No new tooling,
   stdlib-first, testable with goldens.
2. React Email / MJML compiled at build time: nicer authoring, but puts Node into the
   Go build for six small emails, and the output is the same table markup.
3. HTML assembled in Go code: no template files, but markup scattered through code and
   painful to tweak.

## Design

**Layout** (one template, `internal/infra/mailer/templates/`):

```
cream page #faf6ec
  white card 600px, radius 16, text #1f1b13
    badge logo (inline CID image)
    heading (h1, 800 weight)
    "Hi {name},"            <- template
    message paragraph(s)    <- per email
    code box                <- only emails with a code; large, letter-spaced, monospace
    note (small, #7a6a45)   <- per email ("If you didn't request ...")
    divider
    footer: Econumo · Manage money. Together.   <- template
            [Open Econumo] button               <- only when ECONUMO_URL is set
```

Mobile: one `@media (max-width: 660px)` block like the reference (full-width card,
20px side padding, smaller h1). System font stack starting with Plus Jakarta Sans;
nothing is downloaded.

**Greeting and footer are the template's**, not the per-email text. Catalogue shape:

- `emails.common.greeting` ("Hi {name},"), `emails.common.greeting_anonymous` ("Hi,")
  for an empty name, `emails.common.tagline`, `emails.common.open_app`.
- Per email: `subject`, `heading`, `body` (message only, no greeting or signature),
  `note` (may be absent for an email with nothing to add). `{code}` is no longer in the
  body; the template shows it.
- All 11 catalogues updated; `EmailKeys` and the i18n guard follow.

**Two parts, one source.** Each sender builds one `Content` value (subject, heading,
greeting, body, code, note, app URL); the mailer renders both the HTML and the
plain-text part from it. The plain text keeps today's shape: greeting, body, code on
its own line, note, `--` signature, URL.

**Logo**: a small PNG of the badge logo, embedded with `go:embed` and attached inline
(`Content-ID`) to every HTML email. The badge has its own background, so a client that
inverts colours in dark mode (Gmail's app) never shows it as a hard white rectangle,
the lesson from the campaign email. The layout declares `color-scheme: light`.

**Transports**:
- Resend: `Html` + `Text` + one inline attachment with a content id.
- SMTP: `multipart/alternative` (text, then `multipart/related` with the HTML and the
  logo). Hand-built with `mime/multipart`, base64 for the image, quoted-printable for
  the text parts.
- Console: prints the plain-text part only, as today.

`WithAppLink` goes away: the app URL becomes a field of the content, rendered as the
footer button and the last plain-text line, so the behaviour without `ECONUMO_URL`
(no link) is unchanged.

## Testing

- Goldens of the HTML and plain text for each email in `en`
  (`UPDATE_GOLDEN=1` regenerates them).
- SMTP: the message parses as MIME with the expected parts; the CID in the HTML matches
  the attachment.
- Resend: the request carries html, text and the inline attachment.
- i18n guard over the new keys; plain text never loses the code.
- `docs/regression-test-plan.md`: the forgot-password item checks the HTML email too.

## Left to the builder

Exact spacing and type sizes within the reference's palette; the logo PNG size
(about 170x37 at 2x); quoted-printable vs base64 for the HTML part.
