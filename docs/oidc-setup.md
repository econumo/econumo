# Sign in with Google / Apple / SSO

Econumo can offer "Sign in with…" buttons on the login and registration
pages, in addition to email + password — or instead of it, see
[Provider-only sign-in](#provider-only-sign-in). Each provider is
an independent, optional slot configured entirely through environment
variables — there is no admin UI for this. All three slots can be enabled at
once; a self-hosted instance typically enables one.

Every slot requires `ECONUMO_URL` to be set first: it is this instance's
public URL, and every callback URI is derived from it —
`<ECONUMO_URL>/api/v1/oauth/callback-<google|apple|oidc>` — rather than
configured separately. That derived URL is the one thing you register with
each provider below. A slot is all-or-nothing: setting some but not all of
its variables fails the server at boot, naming the missing one.

An issuer whose ID tokens never carry an `email_verified` claim will reject
every sign-in with "The sign-in provider has not verified this email
address." unless you set `ECONUMO_OIDC_TRUST_EMAIL=true` for that slot (see
the Authentik and Cloudflare Access sections below for when that applies).

## Google

1. In the [Google Cloud console](https://console.cloud.google.com/), open
   **APIs & Services → Credentials** for your project (create one if you
   don't have one yet).
2. If prompted, configure the **OAuth consent screen** first (External user
   type is fine for a self-hosted instance; add your own email as a test
   user if the app stays in "Testing" mode).
3. **Create Credentials → OAuth client ID**, application type **Web
   application**.
4. Under **Authorized redirect URIs**, add the exact callback URL (see
   below).
5. Save, then copy the generated **Client ID** and **Client secret** into:

   ```
   ECONUMO_OAUTH_GOOGLE_CLIENT_ID=<client id>
   ECONUMO_OAUTH_GOOGLE_CLIENT_SECRET=<client secret>
   ```

Google is a fixed issuer (`https://accounts.google.com`) — there is no issuer
URL to configure. Google's email claim is used only when Google reports it as
verified (`email_verified`); an unverified Google address is rejected with
"The sign-in provider has not verified this email address."

**Callback URL to register:** `<ECONUMO_URL>/api/v1/oauth/callback-google`

## Apple

Apple's flow needs a *Services ID* (which is the OAuth client id Econumo
uses), plus a private key downloaded once from the developer portal — Apple
never lets you download it again, so save it somewhere safe immediately.

1. In [Apple's developer portal](https://developer.apple.com/account/resources/identifiers/list),
   note your **Team ID** (top right of the membership page) — this becomes
   `ECONUMO_OAUTH_APPLE_TEAM_ID`.
2. Under **Identifiers**, register an **App ID** if you don't already have
   one for this app, with **Sign in with Apple** enabled as a capability.
3. Under **Identifiers**, register a new **Services ID** (type "Services
   IDs"). This identifier string (e.g. `com.example.econumo.signin`) is
   `ECONUMO_OAUTH_APPLE_CLIENT_ID`.
4. Edit the new Services ID, enable **Sign in with Apple**, click
   **Configure**, and add your primary App ID as the associated app. Under
   **Website URLs**, set the **Return URL** to the exact callback URL below
   (Apple's flow posts back via a form, but the callback route is still what
   you register).
5. Under **Keys**, register a new key with **Sign in with Apple** enabled,
   associated with the App ID from step 2. Download the `.p8` file — this is
   a one-time download. Note the **Key ID** shown on the key's page —
   this becomes `ECONUMO_OAUTH_APPLE_KEY_ID`.
6. Put the `.p8` file somewhere the server can read it and point
   `ECONUMO_OAUTH_APPLE_PRIVATE_KEY_FILE` at that path. Keep the file exactly
   as Apple produced it — real newlines, no reformatting. It holds a signing
   credential, so restrict it to the account the server runs as:

   ```
   install -o econumo -g econumo -m 600 AuthKey_ABC1234567.p8 /etc/econumo/apple.p8
   ```

   In Docker, mount it into the container (for example
   `./secrets/apple.p8:/etc/econumo/apple.p8:ro`) and set the variable to the
   path *inside* the container.

   > The key is passed as a file rather than inline because a PEM is
   > multi-line. The earlier `ECONUMO_OAUTH_APPLE_PRIVATE_KEY` variable took
   > the key on one line with newlines written as `\n`, which worked under
   > Docker but not under systemd: `EnvironmentFile=` consumes the backslash
   > of an unquoted `\n`, so the key reached the process as one unbroken line
   > and the server refused to start with `apple private key is not PEM`.
   > That variable is no longer accepted — a server still configured with it
   > fails at boot with a message pointing here.

7. Put it all together:

   ```
   ECONUMO_OAUTH_APPLE_CLIENT_ID=com.example.econumo.signin
   ECONUMO_OAUTH_APPLE_TEAM_ID=<team id>
   ECONUMO_OAUTH_APPLE_KEY_ID=<key id>
   ECONUMO_OAUTH_APPLE_PRIVATE_KEY_FILE=/etc/econumo/apple.p8
   ```

Apple is a fixed issuer (`https://appleid.apple.com`) and its email claim
is always treated as verified. Apple only sends the user's
name on the very first sign-in — Econumo captures it then, so if the
first attempt fails for another reason, a later successful sign-in may not
have a name to use (it falls back to the email's local part).

**Callback URL to register:** `<ECONUMO_URL>/api/v1/oauth/callback-apple`

## A generic OpenID Connect provider (Authentik example)

The third slot works with any standards-compliant OpenID Connect provider —
Authentik, Keycloak, Zitadel, Okta, Auth0, and so on. This section walks
through [Authentik](https://goauthentik.io/) as the concrete example; the
shape is the same everywhere.

1. In Authentik, create a new **OAuth2/OpenID Provider** (**Applications →
   Providers → Create**).
2. Set **Redirect URIs** to the exact callback URL below.
3. Under **Advanced protocol settings**, set the **Signing Key** to an RSA
   key (Econumo verifies RS256- or ES256-signed ID tokens).
4. Note the provider's **Client ID** and **Client Secret**, and the
   **OpenID Configuration URL** it displays — the issuer URL is that same
   value with the trailing `/.well-known/openid-configuration` removed, e.g.
   `https://auth.example.com/application/o/econumo/`.
5. Create an **Application** (**Applications → Applications → Create**) and
   attach it to the provider from step 1.
6. If you want the "sign out here too" logout redirect to work (Econumo's
   RP-initiated logout, web only), add a **post-logout redirect URI** on the
   provider matching `<ECONUMO_URL>/login`.
7. Configure:

   ```
   ECONUMO_OIDC_ISSUER_URL=https://auth.example.com/application/o/econumo/
   ECONUMO_OIDC_CLIENT_ID=<client id>
   ECONUMO_OIDC_CLIENT_SECRET=<client secret>
   ECONUMO_OIDC_NAME=Authentik
   ECONUMO_OIDC_SCOPES=openid,profile,email
   ```

   Leave `ECONUMO_OIDC_TRUST_EMAIL` unset (`false`): Authentik's ID tokens
   carry `email_verified`, so Econumo can trust it directly.

**Callback URL to register:** `<ECONUMO_URL>/api/v1/oauth/callback-oidc`
**Post-logout redirect URI (optional, for RP-initiated logout):**
`<ECONUMO_URL>/login`

## Cloudflare Access for SaaS

[Cloudflare Access for SaaS](https://developers.cloudflare.com/cloudflare-one/applications/configure-apps/saas-apps/generic-oidc-saas-app/)
can front Econumo as a generic OIDC identity provider, so your whole
Cloudflare Access policy (any identity provider you've already wired into
Access, any Access group) becomes an Econumo sign-in method with no separate
user database.

1. In the Cloudflare Zero Trust dashboard, go to **Access → Applications →
   Add an application → SaaS**, and choose **OIDC** as the SaaS application
   type.
2. Set the **Redirect URL(s)** to the exact callback URL below.
3. Save. Cloudflare shows you a **Client ID**, **Client Secret**, and a
   **Issuer URL** for the application. The issuer URL has the shape:

   ```
   https://<your-team-name>.cloudflareaccess.com/cdn-cgi/access/sso/oidc/<client-id>
   ```

4. Configure:

   ```
   ECONUMO_OIDC_ISSUER_URL=https://<your-team-name>.cloudflareaccess.com/cdn-cgi/access/sso/oidc/<client-id>
   ECONUMO_OIDC_CLIENT_ID=<client id>
   ECONUMO_OIDC_CLIENT_SECRET=<client secret>
   ECONUMO_OIDC_NAME=SSO
   ECONUMO_OIDC_TRUST_EMAIL=true
   ```

   **Cloudflare Access's ID tokens do not include an `email_verified`
   claim**, so `ECONUMO_OIDC_TRUST_EMAIL` must be set to `true` for this
   slot — leaving it unset means every sign-in through Access is rejected
   with "The sign-in provider has not verified this email address." Only
   set this if you trust that Access itself only lets verified identities
   through (true for most upstream identity providers wired into Access).

**Callback URL to register:** `<ECONUMO_URL>/api/v1/oauth/callback-oidc`

## Mobile app return

The iOS and Android apps open the sign-in page in the system browser, so the
provider has to send the browser back to the app afterwards. Econumo does that
in one of two ways.

**Verified app links (preferred).** When `ECONUMO_APP_LINKS_IOS` or
`ECONUMO_APP_LINKS_ANDROID` is set, the return URL is
`<ECONUMO_URL>/oauth/app-return` — an ordinary https URL on your own domain
that the operating system hands to the app *only* if the app proves it belongs
to that domain. The server publishes the two association documents the
platforms fetch to check that:

| Path | Platform | Built from |
| --- | --- | --- |
| `/.well-known/apple-app-site-association` | iOS (Universal Links) | `ECONUMO_APP_LINKS_IOS` |
| `/.well-known/assetlinks.json` | Android (App Links) | `ECONUMO_APP_LINKS_ANDROID` |

Both are served with `Content-Type: application/json` and cached for an hour.
`ECONUMO_URL` must be `https://` — neither platform verifies a plain-http link,
and boot fails if it is not. It must also be a bare origin with no path
(`https://app.example.com`, not `https://example.com/econumo`): both platforms
fetch the association documents from the origin root, so an instance served
under a sub-path cannot use app links.

> **Unverified on iOS.** The app-link return has not been confirmed on a real
> iOS device yet — see "Sign-in with Google / Apple / SSO" in `mobile/README.md`
> for what to verify, the fallback if it does not work, and the change to make
> if it fails. Android App Links are unaffected.

```bash
# comma-separated <Team ID>.<bundle id>
ECONUMO_APP_LINKS_IOS=TEAMID1234.com.econumo.app
# comma-separated <package>=<SHA-256 signing fingerprint>
ECONUMO_APP_LINKS_ANDROID=com.econumo.app=AA:BB:...:ZZ
```

- **Apple Team ID**: Apple Developer → Membership (ten characters, e.g.
  `TEAMID1234`); the bundle id is the app target's, e.g. `com.econumo.app`.
- **Android signing fingerprint**: for a store build take it from Play Console →
  your app → Setup → App signing (SHA-256 certificate fingerprint); for a local
  keystore run
  `keytool -list -v -keystore my-release.keystore -alias my-alias` and copy the
  `SHA256:` line. A Play-signed app has two certificates (upload and app
  signing) — list both by repeating the package:
  `com.econumo.app=AA:...,com.econumo.app=BB:...`.

**Private URL scheme (fallback).** With neither variable set the return goes to
`com.econumo.app://oauth` instead, carrying the handoff in the URL **fragment**
(`#handoff=…`, so it never reaches a server or a log) and any failure in the
query (`?error=…`). This works without any per-domain setup, which is why it is
the default for self-hosted instances used from the store app — but a URL scheme is claimed globally on the device, so an app that
registers the same scheme can receive the return. The window is narrow (the
handoff code is one-shot, short-lived and must be presented together with the
flow secret the app kept), but if you run the app against your own backend and
control the domain, configure app links.

## Provider-only sign-in

Set `ECONUMO_PASSWORD_LOGIN=false` to switch email + password off entirely.
The login page then shows only the provider buttons, and the server refuses
password sign-in, password registration, "Forgot password" / "Set a password",
and password changes. Personal access tokens and existing sessions keep
working. The server refuses to start with this setting unless at least one
provider slot is configured.

`ECONUMO_ALLOW_REGISTRATION` keeps its meaning, now for providers alone:
`true` lets anyone who can sign in at your provider create an account on
first sign-in, `false` admits only accounts that already exist. So
`ECONUMO_PASSWORD_LOGIN=false` + `ECONUMO_ALLOW_REGISTRATION=true` is
"sign-up through my SSO only": user management stays at the identity provider.

**Switching over an instance that already has password accounts.** A provider
is never linked automatically to an account that has a password (see below),
and with passwords off that account can no longer sign in to link one. So,
before you set `ECONUMO_PASSWORD_LOGIN=false`:

1. have every existing user sign in with their password and link a provider
   under Settings → Profile → Sign-in methods;
2. then switch passwords off.

A user who missed step 1 sees "An account with this email address already
exists and can't be linked automatically. Contact your administrator." —
switch `ECONUMO_PASSWORD_LOGIN` back on for a moment so they can link, then
off again. Users created with `user:create` always have a password, so they
are in the same position: create accounts by letting people sign up through
the provider instead.

While passwords are off, a stored password no longer counts as a way in, so
Settings refuses to unlink an account's last provider even if it has one.

## How accounts are matched

- **Signing in with a provider whose email matches an existing account that
  has no password** links the two automatically — that account was itself
  created through a provider, so both sides have proven the same address.
  You're signed in to the existing account and the new provider appears under
  Settings → Profile → Linked accounts; the account owner gets an email saying
  so, because gaining a sign-in method unasked should be noticeable.
- **If the matching account HAS a password, the sign-in is refused** with "An
  account with this email address already exists. Sign in with your password
  (or reset it), then link this provider from Settings." Econumo will not merge
  a provider identity into an account whose owner has not authenticated:
  registration does not always verify email addresses, so a password on an
  account is no evidence that its holder owns the address — someone could have
  registered yours before you did. Signing in once with the password (use
  "Forgot password" if you never set one; the code goes to that mailbox) and
  then linking from Settings is the safe path, and only has to be done once.
- **A password reset is a full account reclaim.** Completing one is the only
  way to prove you control the address, so it also signs out every session,
  revokes every personal access token, cancels anything still pending on the
  account (an unfinished provider sign-in, a requested email change), and
  unlinks any sign-in method whose
  provider reports a *different* email address — otherwise someone who had
  registered your address first could keep a provider account linked to it and
  walk straight back in. A provider that reports the same address as the
  account survives the reset (only the mailbox owner could have linked it),
  which is why setting a password on a provider-created account does not
  disturb it. If you had linked a provider under a different address, link it
  again from Settings afterwards.
- **Linking a provider from Settings is finished by the browser that started
  it.** The provider's answer carries nothing that identifies you, so Econumo
  parks the result and only writes the link when the browser that began it
  comes back signed in and presenting its one-flow secret. A "link this
  account" URL someone else sends you therefore cannot attach your provider
  account to theirs.
- **A sign-in, too, can only be completed by the browser that started it.** The
  "Continue with ..." button receives a one-flow secret from the server and
  keeps it locally; the browser must present it together with the one-time
  handoff code to receive a session. Starting a sign-in in one browser and
  finishing it in another therefore fails with "The sign-in attempt expired or
  was already used" — as does a sign-in link someone else hands you.
- **Changing `ECONUMO_OIDC_ISSUER_URL` later is safe.** A linked identity
  records the issuer it came from, so a user at the new issuer can never be
  mistaken for a user at the old one, even if the two hand out the same
  subject id. Existing users are re-matched by their verified email on the next
  sign-in and their linked account moves to the new issuer by itself.
- **An email the provider has not verified is always rejected**, even if it
  would otherwise match an existing account. This is what
  `ECONUMO_OIDC_TRUST_EMAIL` is for: some providers (see Cloudflare Access
  above) never send the verification claim at all, so you tell Econumo to
  trust them explicitly.
- **Signing in for the first time with no matching account** creates a new
  Econumo account automatically, as long as registration is enabled
  (`ECONUMO_ALLOW_REGISTRATION=true`) — otherwise you'll see "Registration is
  disabled on this server. Sign in with an existing account first." An
  account created this way has no password: you can keep using the provider
  to sign in indefinitely, or add a password later.
- **Setting a password**: go to Settings → Profile and use the "Set a
  password" option (in place of "Change password" for a passwordless
  account) — it sends a reset code to your email the same way "Forgot
  password" does. Once set, the account behaves like any password account:
  you can sign in either way, and you can unlink every provider (Econumo
  always refuses to remove your *last* remaining sign-in method while you
  have no password, so you can never lock yourself out).
- **If the provider later reports a different email for the same
  identity** (you changed your email at the IdP), Econumo updates your
  Econumo email to match automatically, but only while the account is
  still passwordless, has exactly one linked provider, and the new address
  isn't already used by another Econumo account. Add a password or a second
  linked provider and this stops happening — after that, changing your
  email goes through Settings → Profile like any other account.
