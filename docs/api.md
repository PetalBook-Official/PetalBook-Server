# HTTP API

Status: milestone 1.3 (a space can follow a remote Git repository; page
history). Before it: 1.2 (sign-in, several spaces, drafts kept on the server).
Endpoints and fields will be added in later milestones; what is listed here is
not expected to change shape.

The web app talks to the server only through this API.

## Conventions

- JSON in, JSON out, UTF-8. Request bodies need `Content-Type: application/json`.
- Times are RFC 3339 strings in UTC (`2026-11-21T08:30:00Z`). `created` in a
  page's header is a date (`2026-11-21`).
- Errors: a non-2xx status and `{"error": "<code>", "message": "<for people>"}`.
  `message` is an English sentence (a capital first, a full stop last) for people
  who use the API and read logs; it is not a contract and may change. A client
  decides by `error`, and the web app shows its own text in the language of the
  page by that code, with what the answer says it is about (`limit`, `size`,
  `name`, `provider`, `permission`, `Retry-After`). The server's sentence is shown
  in two cases: alone, for a code the web app has no text for; and after the
  app's own text, as the detail, for the codes whose sentence says what exactly
  is wrong (`invalid`, `conflict`, `forbidden`, `provider_refused`,
  `provider_error`, `busy`, `unavailable`, `push_failed`, `quota_exceeded`). The codes the server can send are listed in
  `web/src/i18n/errorCodes.json`, which a test keeps equal to the source.
  Codes: `not_found` (404), `invalid` (400; also 405, 413 and 415),
  `conflict` (409), `exists` (409), `unauthenticated` (401: not signed in, or
  wrong credentials), `forbidden` (403: cross-site requests, see below, and
  actions the signed-in user may not take that are not a matter of a role in a
  space), `no_permission` (403: a member whose role in the space lacks the
  permission the request needs; the message names it), `space_disabled` (403: an
  administrator has disabled the space, see "Administration"), `feature_off`
  (403: the caller's tenant does not have a feature the route belongs to, see
  "Features"), `rate_limited` (429, with a
  `Retry-After` header in seconds), `unavailable` (503: the space follows a
  remote repository that cannot be reached or is paused, see "Remote
  repository"; nothing was changed), `internal` (500). Signing in has codes
  of its own (see "Sign-in error codes"), as do some sections; a section's own
  code takes the place of the general one in `error`, with the status given.
  Server storage (milestone 3.1, see "Server storage") adds
  `remote_required` (409), `not_cached` and `restoring` (503, with
  `Retry-After`) and `space_full` (507).
  Requests with an API token or an OAuth access token have codes of their
  own: `token_invalid`, `token_expired`, `token_revoked`,
  `token_owner_disabled` (401), `token_scope` and `tokens_off` (403); see
  "API tokens".
- **Cross-site requests.** These rules keep other web sites from using a
  browser to change pages, and in loopback-only mode from reading them. They
  apply to every request. With `--insecure-no-auth` a site that controls a DNS
  name can still make a browser read pages (DNS rebinding); do not use that
  flag on a network you do not trust until sign-in exists.
  - Every `POST`, `PUT`, `DELETE` and `PATCH` must send
    `Content-Type: application/json`, even with no body, or the answer is
    `415 invalid`. The exceptions are uploading a file (see "Files") and an
    import (see "Import"), which must send
    `Content-Type: application/octet-stream`; a plain HTML form
    cannot send that type either.
  - A request whose `Sec-Fetch-Site` header is present and is neither
    `same-origin` nor `none` gets `403 forbidden`.
  - A request carrying an `Origin` header gets `403 forbidden` unless that
    origin is accepted. An origin is accepted if the administrator listed it
    with `--allow-origin`. In loopback-only mode (the default: the server
    listens on a loopback address) a plain `http` loopback origin
    (`localhost`, `127.0.0.1`, `[::1]`, `*.localhost`) on the same port as the
    request's `Host` is accepted too. With `--insecure-no-auth` there is no
    implicit own origin: browsers must be listed with `--allow-origin`.
    In accounts mode the origin of `--public-url` is accepted as the server's
    own, on any listen address.
  - In loopback-only mode a request whose `Host` is neither a loopback name
    nor the host of an allowed origin gets `403 forbidden`.
  - Requests with no `Origin` and no `Sec-Fetch-Site`, such as `curl` or
    scripts, are not refused by these checks.
  - The exceptions are reached from an identity provider, so the
    `Sec-Fetch-Site` and `Origin` rules do not apply to them:
    `GET /api/auth/oidc/{slug}/callback` and
    `GET /api/auth/github/{slug}/callback` (a redirect), and
    `POST /api/auth/saml/{slug}/acs` (a form the identity provider posts, so it
    is `application/x-www-form-urlencoded`, not JSON). Each changes nothing
    unless it matches a sign-in this browser started (its `state`, or for SAML
    its `RelayState` and the attempt's cookie). `GET /api/auth/saml/{slug}/metadata`
    is read by the identity provider and changes nothing.
    `GET /api/account/links/{provider}/callback`, the platform's
    redirect back after a person connects their account for repositories (not
    a sign-in), is the same kind of exception under the same rule (see
    "Connections"). So is `GET /api/providers/github/manifest/callback`, where
    GitHub sends an administrator's browser after the App is made.
- A **space** is one tree of pages backed by one Git repository. Every page URL
  carries the space id. In single-user mode there is exactly one space, with
  the id `default`; in accounts mode a space id has the same form as a page id.
- A **page id** is the `id` from the page's header: 10 characters of Crockford
  base32 (`0-9`, `A-Z` without `I L O U`), e.g. `01JD3K8Q7Z`. It never changes.
- A **rev** identifies the exact bytes of a page file: the Git blob id of the
  file content (40 hex characters). Clients send the rev they started from
  when saving; the server refuses the save if the file has changed since.
- Folder paths are relative to the space root, `/`-separated, no leading or
  trailing slash. The root folder is the empty string.

## Modes

The server runs in one of two modes, chosen at start-up.

- **Single-user** (`serve --space DIR`): one space, the directory given; no
  sign-in. Every request acts as the built-in user. The server listens on a
  loopback address only unless `--insecure-no-auth` is given. The sign-in,
  account, user, invite and space-management endpoints answer `404`.
- **Accounts** (`serve --data DIR`): users sign in. Spaces live in
  `DIR/spaces/<space id>/`. Every endpoint except `GET /api/meta`,
  `GET /api/session` and the ones under "Signing in" needs a session and
  answers `401 unauthenticated` without one. A space the user is not a member
  of answers `404 not_found`, the same as a space that does not exist, unless
  the user is an administrator: from 3.5 an administrator manages every space of
  their scope (see "Members") and reads one only in a view they open with a
  reason (see "Viewing a space"). A member whose role lacks the permission a route needs
  answers `403 no_permission`, a code of its own: it is never confused with the
  cross-site refusal, which is `403 forbidden`.

In either mode a request may carry an API token instead of a session (see "API
tokens"); `/mcp` takes only tokens (see "MCP").

## Meta

### `GET /api/meta`

No session needed. `spaces` lists the spaces the caller can open (empty when
not signed in), in the shape of `GET /api/spaces`.

```json
{
  "version": "dev",
  "maxUpload": 10485760,
  "maxImport": 104857600,
  "spaces": [{"id": "default", "name": "My notes", "role": "owner", "permissions": ["read", "write", "members", "manage"], "personal": true, "sync": "local", "inbox": "Inbox"}]
}
```

`maxUpload` is the largest file, in bytes, the server accepts as an upload
(`--max-upload`, 10 MiB by default, at most 256 MiB). From 3.5 it is the
effective `files.maxBytes` of the caller's tenant (see "The size of one file"),
at most `--max-upload`. `maxImport` is the same for an import
(`--max-import`, 100 MiB by default, at most 512 MiB).

## Session

### User

```json
{"id": "01JD3K8Q7Z", "username": "kazuha", "name": "Kazuha", "email": "", "emailVerified": false, "role": "admin", "disabled": false, "passkeyRequired": false, "created": "2026-11-21T08:30:00Z"}
```

`role` is `admin` or `member`. `username` is 2–32 characters of `a-z`, `0-9`,
`.`, `_`, `-`, starting with a letter or digit; it is stored in lower case and
upper case in a request is folded. `email` may be empty. It never decides which
account an identity signs in to, with one exception the administrators switch on
per provider (see "Which account an identity signs in to"), and then only when
`emailVerified` is `true`: the address came from a provider that vouched for it,
or an administrator set or confirmed it. A user changing their own `email`
clears `emailVerified`. From 3.3 an address that only a tenant administrator
(not a site administrator) set or confirmed shows `emailVerified: true` but does
not count for that exception.

From 3.3 a User also has `siteAdmin` and `tenant` (see "Administrators" under
"Tenants, registration and mail").

`passkeyRequired` is whether this person must sign in with a passkey: set for
them alone (`PATCH /api/users/{id}`) or for their role (the 安全 settings,
`passkeys.requireRoles`). In a response it is the effective value, so it is
`true` for everyone of a required role; `PATCH` changes only the person's own
mark. See "When a passkey is required".

### `GET /api/session`

Never an error; no session needed. `Cache-Control: no-store`.

```json
{
  "mode": "accounts",
  "user": null,
  "setup": false,
  "hold": null,
  "methods": {"password": true}
}
```

- `mode`: `single` or `accounts`.
- `user`: the signed-in user, or `null`. In single-user mode it is always the
  built-in user: `{"id": "local", "username": "local", "name": "Local user", "role": "admin", …}`.
- `setup`: `true` while there is no user at all; the app then shows the
  first-run page.
- `hold`: `null`, or `"passkey"` when the signed-in user must register a passkey
  before they can do anything else (see "When a passkey is required"), or
  `"password"` when they signed in with a temporary password and must choose a
  new one (`POST /api/account/password`; an administrator set it with
  `POST /api/users/{id}/password`).
- `endedReason`: only when there is no `user` and the request's session cookie
  belonged to a session that ended (the same values as `reason` of a 401, below);
  absent for everyone else, so a cookie that never was a session learns nothing.
- `user.permissions` (3.6): the permission keys the user holds in their own
  tenant, site keys included (see "Permission keys"); `[]` with an API token.
- `methods` is `{"password": true}`, and absent in single-user mode. The
  sign-in page reads `GET /api/auth/providers` for the providers.

## Signing in

Every JSON sign-in (`POST /api/setup`, `POST /api/login`,
`POST /api/invites/accept`, `POST /api/register/confirm`,
`POST /api/auth/passkey/finish`) answers `{"user": {…}, "hold": null}` with the
same `user` (`permissions` included) and `hold` as `GET /api/session` gives the
new session, so a client needs no second request. The keys are the person's in
their own tenant. `hold` is `"passkey"`, `"password"` or `null`. Redirect flows
(OIDC, SAML, GitHub) answer no JSON: the page that loads reads
`GET /api/session`.

Accounts mode only. Single-user mode has no sign-in: every endpoint in this
section, in "Account" and the security settings of "Administration" answers
`404` there.

The session is a cookie (`HttpOnly`, `SameSite=Lax`, `Secure` when
`--public-url` is `https`); its value means nothing to the client. A session
ends after `sessions.idleDays` without use (30 by default) and at the latest
`sessions.maxDays` after sign-in (90 by default); both are layered settings,
and a change applies to the sessions that already exist at their next request.
They are the lifetimes of the person's own tenant: an organisation may shorten
them for its people (see "The sign-in policy of a tenant"), and a session is
checked against them on every request from the settings' cache, with no query
of its own. The session cookie's `Max-Age` is the tenant's `sessions.maxDays`.
Changing a password ends the user's other sessions; disabling a user ends all
of them.

A password is at least 8 characters and at most 1024 bytes; nothing else is
required of it.

Every way of signing in ends in the same place: a session for an enabled
account. They differ in what proves who the person is (a password, a passkey,
an identity provider) and in what the server checks before it spends any work
on that proof (the per-address request limit, the captcha, the failure caps;
see "Rate limit, failure caps and lockout").

### `GET /api/auth/providers`

Public; what the sign-in page draws. `Cache-Control: no-store`. Answered from
memory, without a database query.

```json
{
  "providers": [
    {"slug": "company", "kind": "oidc", "name": "Company SSO", "icon": "preset:microsoft"},
    {"slug": "campus", "kind": "saml", "name": "Campus login", "icon": ""},
    {"slug": "github", "kind": "github", "name": "GitHub", "icon": "preset:github"}
  ],
  "loginMode": "dual",
  "local": true,
  "sso": true,
  "passkey": true
}
```

- `providers` are the enabled ones, in the administrators' order. `kind` is
  `oidc`, `saml` or `github`. `icon` is `""` (none) or `preset:<name>`, one of
  the icons the web app ships (`google`, `microsoft`, `github`, `gitlab`,
  `okta`, `keycloak`, `authentik`, `key`); never an address, so the sign-in page
  never makes the browser ask another site for anything.
- `loginMode` is the mode in effect: `dual`, `sso_first`, `sso_redirect` or
  `local_only`. With no enabled provider it is `local_only`, whatever is stored.
- `local`: whether the page shows the password form without being asked
  (`false` only for `sso_redirect`).
- `sso`: whether the page shows the provider buttons (`false` for `local_only`,
  and whenever no provider is enabled).
- `passkey`: whether passkey sign-in is available (passkeys are switched on and
  the server has a public URL they can work with, see "Passkeys").
- From 3.3, `register` and `forgot`: whether the page links to registration and
  to password reset, for the tenant named by `?tenant={slug}` (the default
  tenant without it); see "Registration" and "Password reset".
- From 3.6 d3, with `?tenant={slug}` the providers are what that tenant's
  people may use: the site's social providers its `signin.social` allows, the
  site's SSO offered to it (`tenants`), and its own that can sign in now. With
  none of them the mode is still the site's, as the server enforces it (an
  `sso_redirect` site keeps `local: false`). Without `?tenant` they are the
  site's enabled providers, as before; a tenant's own are never listed there.
- From 3.6 d4, `identifierFirst` and `ssoEntry`: whether the page asks for the
  address (or username) first and offers "Sign in with SSO"; see "Email
  domains and the SSO entry". A tenant that requires its own single sign-on
  (`signin.ssoOnly`) is listed without the site's social providers.

Whether password sign-in is refused (`ssoOnly`) is never in this answer, nor
anything about a particular account: neither may help someone choose which
accounts to guess.

#### The sign-in page

What the page draws comes only from the answer above:

1. `dual`: username, password, the captcha when one is needed, "Sign in"; below
   it "or sign in with" and one "Sign in with {name}" button per provider; the
   passkey button when `passkey`.
2. `sso_first`: the provider buttons first; the password form is folded into a
   link "Use a password instead" and opens when it is clicked.
3. `sso_redirect`: with exactly one provider the page goes straight to its
   `start` and shows "Going to {name}…"; with several it does not go anywhere
   and lists the buttons. Either way there is a link "Use a password instead"
   (it loads `/login?local=1`, which shows the form). The page never goes to the
   provider when the address carries `?local=1` or `?sso_error=`, so a failed
   sign-in does not bounce back and forth.
4. `local_only`: the form only.

A button is a full-page navigation to `GET /api/auth/{kind}/{slug}/start`.

### `POST /api/setup`

Creates the first administrator. Works only while there are no users; after
that `409 exists`.

```json
{"token": "…", "username": "kazuha", "name": "Kazuha", "password": "…", "captcha": {"id": "…", "answer": "…"}}
```

`token` is the setup token the server prints in its log at start-up while
there are no users (it also prints a link of the form
`<public url>/setup#<token>`). Wrong token: `403 forbidden`, counted as a
failure of the address. `name` defaults to the username. `captcha` is needed
only when the `setup` context asks for one (see "Captcha"). `201` with
`{"user": {…}}` and a session cookie. The user gets a personal space.

### `POST /api/login`

```json
{"username": "kazuha", "password": "…", "captcha": {"id": "…", "answer": "…"}}
```

`captcha` is `{"id", "answer"}` for the self-hosted image, `{"token"}` for a
third-party one, and left out when `GET /api/captcha` said none is required.
`tenant` (a slug, optional) names the organisation the sign-in page was opened
for (`?tenant=`): the captcha asked is then that organisation's (see "The
sign-in policy of a tenant"); without it, the site's. It is never worked out
from the username, which would tell which organisation it is in. An unknown or
disabled `tenant` is `404`, counted against the address's requests first.

From 3.6 the field is `identifier`, an email address or a username, in place
of `username` (see "Signing in by email address" under "Tenant autonomy").

`200` with `{"user": {…}}` and a session cookie. The server checks, in this
order, and stops at the first refusal:

1. The address's requests this minute: `429 rate_limited`.
2. The captcha, when the `login` context requires one for this address or this
   username: `400 captcha_failed`. Nothing is counted against the password.
3. The address's failures: `429 rate_limited`.
4. With lockout on, the key the lockout scope names (see "Lockout"):
   `429 rate_limited`.
5. The password, against the stored hash (or a dummy hash for a username nobody
   has, so both take as long).
6. A wrong password (or no such user, or a user with no password) counts one
   failure against the address, the username and the pair. If the username or
   the pair has now reached the cap: `429 rate_limited`; otherwise
   `401 unauthenticated`. A disabled user is `401 unauthenticated` too; the
   message is the same in every case.
7. With the password right: password sign-in refused for this account
   (`ssoOnly`, see "Security settings"): `403 local_login_refused`; a passkey is
   required and the user has one: `403 passkey_required` (the page offers the
   passkey button).
8. Success clears the username's and the pair's failures and gives back the
   address's. A user who must have a passkey and has none gets the session with
   `hold: "passkey"`.

So the password is checked before anything that depends on the username, and a
person who types their password right is not stopped by someone else's
failures against their name, unless the administrators chose the lockout scope
`account`.

### `POST /api/logout`

`204`; ends the session if there is one. A session ended this way leaves no
record: it is the person's own doing, so nothing is said to its cookie later.

### Why a session ended

When a session ends without the person signing out, the server keeps one small
record for 30 days: the SHA-256 of the session cookie value (never the value),
whose session it was, the reason and the time. A daily sweep removes records older than that. A `401
unauthenticated` answer to a request that carries the cookie of such a session
has one more field, `reason`:

```json
{"error": "unauthenticated", "message": "Your session has ended; sign in again.", "reason": "expired"}
```

| `reason` | The session ended because |
| --- | --- |
| `expired` | it was idle for longer than the idle limit, or older than the absolute limit (see 安全 settings, `limits.sessionIdleDays`, `limits.sessionMaxDays`) |
| `signed_out_elsewhere` | the person signed out of every device, or a site administrator ended their session (`DELETE /api/users/{id}/sessions…`) |
| `password_changed` | the password was changed or reset (the session that made the change stays) |
| `disabled` | the account, or its tenant, was disabled |

Disabling an account or a tenant (`PATCH /api/users/{id}`, `PATCH
/api/admin/tenants/{id}` with `disabled: true`) ends the sessions of the people
concerned at once, as `disabled`, so a cookie that stays idle until after it
would have expired still says `disabled`. Enabling them again forgets those
records: the old cookies then get the plain 401.

An unknown cookie, a malformed one, one of a session that was signed out with
`POST /api/logout`, and a record older than 30 days get the plain 401 with no
`reason`: nothing is told to a cookie that cannot prove it was a session. `GET
/api/session` carries the same value as `endedReason`. The sign-in answers do
not change: a disabled account signing in is refused like any wrong password.

The web app keeps the last person's display name, username or email, avatar URL
and last sign-in method in this browser's `localStorage` (never a secret), shows
the reason on the sign-in page and offers that person's last method again, then
returns to the page they were on. Only an explicit `reason` does this: a 401
without one gives the plain sign-in page. "Use another account" adds `other=1`
to the sign-in page's address, which keeps the plain page for that visit. The
app forgets the person when they sign out on purpose, and never shows them for
`disabled`.

### Passkeys

A passkey is a WebAuthn credential. It signs in on its own (no password), for
local accounts and accounts made through a provider alike; a person can have up
to 20. The relying party is the host of `--public-url`, so passkeys need a
public URL that is `https` (or `http` on a loopback name for trying things out);
without one `passkey` is `false` in `GET /api/auth/providers` and these
endpoints answer `404`. Changing the host of `--public-url` makes every passkey
registered before unusable; the server logs a warning at start-up when it sees
the host change.

Sign-in asks the authenticator for user verification (a PIN or biometric), and
the server checks that it was given, because the passkey is the only factor.
Registration prefers a discoverable credential (one the browser can offer
without a username).

#### `POST /api/auth/passkey/begin`

Public. `{"username": "kazuha"}`, or `{}` for a discoverable sign-in (the
browser offers the passkeys it has for this site). Counts against the
address's requests this minute.

`200` with `{"ceremony": "…", "options": {…}}`: `options` is the
`PublicKeyCredentialRequestOptions` to pass to `navigator.credentials.get`. With
a username, `allowCredentials` lists that user's passkeys. A username nobody has,
or one with no passkey, gets one made-up credential id (derived from the
username and a server key, so it is the same every time), and the answer looks
the same as for a user with one passkey. A ceremony lasts 5 minutes and is used
once; at most 10,000 are open at a time (the oldest are dropped first).

#### `POST /api/auth/passkey/finish?ceremony={id}&return={path}`

Public. The body is the credential the browser returned
(`PublicKeyCredential` as JSON). Counts against the address's requests this
minute. `200` with `{"user": {…}, "return": "/s/01JD3K8Q7Z"}` and a session
cookie; `return` is made safe as for single sign-on.

- `401 passkey_failed`: the ceremony is unknown, used or expired, the
  credential is not registered, the signature or user verification does not
  check out, the authenticator's counter went backwards (a sign that the key was
  copied: refused, recorded in the audit log, and the stored counter is not
  moved), or the account is disabled. One code and one message for all.
- `403 local_login_refused`: `ssoOnly` is in effect and the account is not an
  administrator (or is a tenant administrator of a tenant whose
  `signin.ssoOnlyAdmins` is on; see "Security settings").

A passkey sign-in costs two indexed queries (the credential, by its id or by
the named user's handle, and the user, by its id: a browser that is signing in
has no session whose cache could answer), one write (the credential's counter
and last use) and the insert of the new session.

#### When a passkey is required

The administrators can require a passkey of chosen people
(`PATCH /api/users/{id}` with `passkeyRequired`) or of a role
(`passkeys.requireRoles` in the security settings). For such a person:

- With a passkey registered, password sign-in answers
  `403 passkey_required` after the password was checked (a wrong password gets
  the ordinary answers, so the code says nothing to someone who does not know
  it). The page then shows "Your account signs in with a passkey" and the
  passkey button.
- With no passkey yet, password sign-in succeeds into a held session:
  `GET /api/session` has `hold: "passkey"`, and the app shows only "Set up a
  passkey to continue" (the passkey part of the account's security page) and
  "Sign out". Every request answers `403 passkey_required` except
  `GET /api/session`, `GET /api/meta`, `GET /api/auth/providers`,
  `GET /api/account`, the account's passkey endpoints and `POST /api/logout`.
  The first registered passkey lifts the hold for the session at once.
- A temporary password (`POST /api/users/{id}/password`) does not skip this:
  with a passkey registered, signing in with it is `403 passkey_required` like
  any password; without one, the session is held for a new password first
  (`hold: "password"`), and choosing it turns the hold into `hold: "passkey"`.
  A session held for a new password may not use the passkey endpoints
  (`403 password_change_required`), so registering a passkey never lifts it.
- A held session makes no API token and approves no OAuth consent: those
  endpoints answer `403 passkey_required` like the rest (see "Held sessions,
  API tokens and OAuth grants").
- A held session cannot connect or disconnect a GitHub account for repos,
  list owners or repositories, finish GitHub's callback, or create a space
  with a repository: `403 passkey_required` like the rest (see "Connections"). A
  GitHub account connected before the hold stays connected.
- Sign-in through an identity provider is not held: the provider decides how
  its people prove who they are.
- If they lose every passkey, an administrator deletes the lost ones
  (`DELETE /api/users/{id}/passkeys/{passkey}`); the next password sign-in is
  held again until a new one is registered.

Whether a person is required and whether they have a passkey are kept with the
cached user, so the hold costs no query per request.

### Single sign-on

The administrators configure any number of providers (see "Sign-in
providers"), each with a `slug` that is part of its addresses. `kind` is `oidc`
(OpenID Connect: Google, Microsoft Entra ID, Keycloak, Authentik, …), `saml`
(SAML 2.0) or `github` (GitHub's OAuth sign-in, which is not OpenID Connect:
there is no ID token, so the server reads the user from GitHub's API). A
disabled or unknown provider answers `404` (for a navigation: a redirect to
`/login?sso_error=provider_unavailable`). SSO has no captcha: the provider
guards its own sign-in.

#### Starting: `GET /api/auth/{kind}/{slug}/start?return={path}&login_hint={address}`

A navigation from the sign-in page. Redirects (`303`) to the provider and sets
the short-lived cookie that ties the attempt to this browser. Only for signing
in (`intent: login`); the cross-site rules apply, so another site cannot start
one. Counts against the address's sign-in starts (30 a minute).

`login_hint` is optional: the sign-in page sends the email of the person who is
coming back after a session ended. For an `oidc` provider it is passed on as the
`login_hint` of the authorization request. For a `saml` provider whose
`nameIdFormat` is `emailAddress` it is the AuthnRequest's `<Subject><NameID>`
(3.6 d4). For either, a value that is over 254 bytes or has a control character
is dropped. `github` has no such parameter and ignores it.

#### Starting: `POST /api/auth/{kind}/{slug}/start`

The body, for every kind:

```json
{"intent": "login", "token": "", "return": "/s/01JD3K8Q7Z"}
```

- `intent`: `login` (default); `setup` (create the first administrator;
  `token` is the setup token); `invite` (create an account; `token` is the
  invite token); `link` (attach the identity to the signed-in user; needs a
  session).
- `return`: a path inside the app to end up on. Anything that is not a plain
  absolute path (`/…`, not `//…`, no backslash) is replaced by `/`.

`200` with `{"url": "https://idp.example/authorize?…"}` and the cookie. The
client navigates to `url`. Tokens never travel in an address: that is why
`setup`, `invite` and `link` use this form.

An attempt lasts 10 minutes and is used once; at most 10,000 are open (the
oldest go first). It is held in memory: a restart in the middle of a sign-in
sends the person back to `/login?sso_error=expired`.

#### OpenID Connect: `GET /api/auth/oidc/{slug}/callback`

The provider redirects here (the redirect URI to register at the provider is
`<public url>/api/auth/oidc/{slug}/callback`; the provider settings show it).
The server checks `state`, the browser's cookie, the code exchange with PKCE,
the ID token's signature, issuer, audience and nonce, and answers with a
redirect (`303`) into the app, never JSON:

- success: to the `return` path, with a session cookie (for `link`, the
  session stays as it is);
- failure: to `/login?sso_error=<code>` (for `link`:
  `/settings?sso_error=<code>`).

An OpenID Connect linked account is the provider's own id (the provider row's
id, never the issuer an administrator typed) and the `sub` claim.

#### GitHub: `GET /api/auth/github/{slug}/callback`

The same flow and outcomes as OpenID Connect. The server exchanges the code,
then reads `GET /user` (and `GET /user/emails` for a verified address, and
`GET /user/orgs` when the provider's groups are on) with the user's token,
which it keeps only for the length of the request. The identity is the
provider's base URL (`https://github.com`, or a GitHub Enterprise Server
address) and the numeric user id, never the login name, which a GitHub user can
change. The scopes asked for are `read:user` and `user:email`, plus `read:org`
only when the provider's groups are on (the organisations are the groups claim;
they never let anyone in or keep anyone out of an account).

#### SAML: `POST /api/auth/saml/{slug}/acs`

The identity provider posts the browser here (`application/x-www-form-urlencoded`,
`SAMLResponse` and `RelayState`; at most 1 MiB). The JSON-only and cross-site
rules do not apply to it (see "Conventions"). Counts against the address's
requests this minute.

The browser cookie of a SAML attempt is `__Secure-petalbook_saml_<slug>`,
`SameSite=None`, scoped to this path: the post comes from another site, and a
`Lax` cookie would not be sent with it. So SAML needs an `https` public URL.

A `link` started over SAML also sets `__Host-petalbook_saml_link_<slug>`
(`SameSite=None; Secure; Path=/`, host-only), and the attempt keeps a hash of
it. The session cookie is not sent with the cross-site post, and a sibling
subdomain could set the `__Secure-` attempt cookie in someone else's browser,
so the ACS links only when this cookie matches; otherwise it answers
`sso_error=expired`.

The server checks that `RelayState` equals the cookie, finds the attempt, and
has the response validated against the provider's metadata (signature with
SHA-256 or stronger, exactly one assertion, audience, destination, the
`InResponseTo` of this attempt, time window, not seen before). Then it answers
as the OpenID Connect callback does, with a `303`.

A SAML identity is the identity provider's entity ID and the `NameID`. A
transient `NameID` (a new value at every sign-in) is refused, because it cannot
name an account. Responses the identity provider sends without being asked
(IdP-initiated) are refused unless the provider allows them
(`allowIdpInitiated`); they can then only sign in, to `/`.

Validating a response needs no database query: the metadata is held in memory,
and the assertion ids already used are kept in memory (at most 65,536, each
until its assertion expires).

#### SAML: `GET /api/auth/saml/{slug}/metadata`

Public. The service provider metadata to give the identity provider
(`application/samlmetadata+xml`): entity ID, the ACS address, the signing
certificate (no encryption key is offered: encrypted assertions are refused). It is served for a provider that is not enabled yet, so it can be
set up at the identity provider first.

#### `?sso_error=` codes

The codes a failed sign-in or link sends to `/login?sso_error=` or
`/settings?sso_error=`. The page shows a sentence for each and offers to try
again. None says anything about an account that the person did not prove they
own.

| Code | Meaning |
|---|---|
| `provider_unavailable` | the provider is disabled or unknown, its discovery document or metadata cannot be fetched, or its stored secret cannot be opened with this server's key |
| `idp_error` | the provider refused, or its answer did not verify (OpenID Connect, GitHub) |
| `expired` | the attempt is unknown, used, older than 10 minutes, or was started in another browser |
| `no_account` | nobody has this identity, and no account can be made by a sign-in now: setup is not done (the first account is made with the setup token), or the tenant it would land in is disabled |
| `invite_required` | nobody has this identity, and the registration mode of the tenant the account would land in is `invite`: the sign-in carries no invitation (see "Registration") |
| `registration_closed` | the mode is `closed`: no sign-in makes an account, an invitation's included |
| `people_limit` | the tenant the account would land in is at its `quota.people` |
| `email_in_use` | from 3.6: nobody has this identity, the sign-in vouches for its address, and another account holds that address verified; sign in to that account and link the provider from its settings |
| `not_allowed` | the account or the tenant may not use this provider (a tenant's provider for another tenant's account, a social provider the tenant does not allow, a site SSO provider not offered to it) or the sign-in is not in `groups.allow` |
| `disabled` | the account is disabled |
| `identity_in_use` | `link`: another person has linked this account at this provider |
| `already_linked` | `link`: the person has an account linked at this provider already (one per provider per person; unlink it first to link another) |
| `saml_no_email` | SAML: the assertion has no address, neither in the attribute the provider names for it (`mail` by default) nor, with a `NameID` in the `emailAddress` format, as the `NameID`; the administrator maps an email attribute |
| `invite_invalid` | `invite`: the invite is unknown, used or expired |
| `setup_invalid` | `setup`: setup is done, or the token is wrong |
| `rate_limited` | the address made too many requests (SAML ACS, starts) |
| `sso_required` | from 3.6 d4: the site's social sign-in, for a person whose tenant requires its own single sign-on (`signin.ssoOnly`) |
| `join_pending` | a social sign-in whose address joins a tenant by request, when it has a request waiting there already |
| `join_limit` | the same, over the day's requests for the address or the tenant's waiting ones |
| `saml_destination` | the response has no `Destination`, or another one |
| `saml_weak_signature` | the response is signed or digested with something weaker than SHA-256 |
| `saml_encrypted` | the response carries an encrypted assertion, which the provider does not allow |
| `saml_replay` | the assertion was used before |
| `saml_multiple_assertions` | the response carries more than one assertion |
| `saml_attributes` | an attribute is declared more than once |
| `saml_transient_nameid` | the `NameID` is transient; set the identity provider to persistent, emailAddress or unspecified |
| `saml_no_nameid` | the assertion has no `NameID` |
| `saml_assertion_rejected` | any other reason the response did not validate (signature, audience, time) |
| `internal` | something failed on the server; it is in the log |

#### Which account an identity signs in to

For `intent: login`, in this order:

1. The account linked to this subject at this provider (by the provider's id and
   the subject: one query on the unique index).
2. From 3.6, never an account found by its address. When the sign-in vouches
   for the address it sent (see "Which sign-ins vouch for an address" in
   "Signing in by email address") and another account holds that address
   verified: `email_in_use` (the person signs in to that account and links the
   provider from their account settings).
3. When the registration mode of the tenant the account would land in is
   `open` (see "Registration"): a new member account (the
   `name` claim; the username from `preferred_username` or the
   part of `email` before `@`, made valid and unique; for SAML, the attributes
   the provider maps). Its `email` is the provider's address, verified (by
   `provider:<slug>`) when the sign-in vouches for it, and then taken from an
   account that holds it unverified; left out when another account keeps it,
   or when it is outside ASCII while `users.emailNonAscii` is off. Never the
   first account: that is made with the setup token (`no_account`). The tenant
   it would land in is, in this order: the tenant of a verified address on a
   domain that joins people to one (a site social provider only, see "Joining an
   organisation"); else the provider's own tenant (a tenant's own provider: that
   tenant; a site provider: its `tenant`, the default tenant unless set); the
   default tenant's mode is the site's value. Under `invite` the answer is
   `invite_required`, under `closed` `registration_closed`, and a tenant at its
   `quota.people` answers `people_limit`.
4. Otherwise (the first account, or a disabled tenant) `no_account`.

An address alone never joins accounts: the same address signing in from two
providers makes two linked accounts, and they are on one account only if the
person linked the second one from their account. A person has at most one
linked account per provider (`already_linked`; to switch, unlink first).

A linked account belongs to one provider row. Two providers may have the same
issuer string (an organisation can copy another's), and the same subject at
each is two different linked accounts: nothing is ever found by the issuer.
Deleting a provider deletes the accounts linked to it, and adding it again
brings none of them back.

### Captcha

The server's own image captcha (the default: nothing leaves the server), or
Cloudflare Turnstile, Google reCAPTCHA (v2) or hCaptcha. Each context is
switched on by itself:

| Context | Form | Counted failures |
|---|---|---|
| `login` | `POST /api/login` | wrong passwords, by address and by username |
| `invite` | `POST /api/invites/check`, `POST /api/invites/accept` | the address's failures |
| `setup` | `POST /api/setup` | the address's failures |
| `forgot` | `POST /api/password/forgot` (from 3.3; see "Password reset") | the address's failures |
| `register` | `POST /api/register` (from 3.3; see "Registration") | the address's failures |

Each context asks for a captcha either `always` or `after_failures` (the
default, after 3): once the address, or for `login` the username, has that many
failures in the current window. Passkey sign-in and single sign-on have no
captcha.

#### `GET /api/captcha?ctx={context}&account={username}&tenant={slug}`

Public. `Cache-Control: no-store`. Counts against the address's requests this
minute. `account` matters only for `login`; the page asks again when the
username changes (when the field loses focus, not at every key). `tenant` is
the organisation the form is opened for (optional, `404` if unknown): for
`login`, `forgot` and `register` the answer is the settings of that
organisation's `captcha.contexts`; without `tenant`, and always for `invite`
and `setup`, the site's.

```json
{"required": true, "provider": "image", "siteKey": "", "id": "…", "image": "data:image/png;base64,…"}
```

- `required: false`: send nothing; the other fields are absent.
- `provider: "image"`: show `image`; send `{"id", "answer"}`.
- `turnstile`, `recaptcha`, `hcaptcha`: render the provider's widget with
  `siteKey`; send `{"token"}`.
- An unknown `ctx`: `400 invalid`.

An image challenge lasts 5 minutes and can be answered once, right or wrong; at
most 10,000 are held in memory (the oldest are dropped first). A wrong answer is
`400 captcha_failed`; the page asks for a new challenge.

The captcha is checked before anything else costs work: before the failure
caps, the password hash and the token lookups, and a wrong captcha is not a
wrong password. A third-party provider that cannot be reached, answers
something unreadable, or reports a host name other than the host of
`--public-url` is a failure (`captcha_failed`): the form is not let through
when the check could not be made.

### Rate limit, failure caps and lockout

Every count is in memory, in this one process, and starts again from zero when
the server restarts. Each table holds at most 10,000 keys and drops the one
seen longest ago first (a key under lockout goes only when every other key is
locked too), so made-up addresses and names cannot grow memory. Addresses are
those of "Conventions" (`--trusted-proxy`; an IPv6 address counts by its /64).

**Requests per minute, per address** (`limits.perMinute`, 10 by default). One
allowance per address shared by `POST /api/login`, `POST /api/setup`,
`POST /api/invites/check`, `POST /api/invites/accept`, `GET /api/captcha`,
`POST /api/auth/passkey/begin`, `POST /api/auth/passkey/finish` and
`POST /api/auth/saml/{slug}/acs`. Over it: `429 rate_limited` with
`Retry-After` (for the ACS, a redirect to `/login?sso_error=rate_limited`). The
starts of single sign-on keep their own allowance (30 a minute per address).

**Failure caps** (`limits.maxFailures`, 10 by default, in a window of
`limits.windowMinutes`, 15 by default). Failures are counted three ways: by
address, by username, and by the pair. A window starts at a key's first failure
and ends `windowMinutes` later. The address's count is checked before any hash
or token is looked at; the username's and the pair's only after the password
turned out wrong. Over a cap: `429 rate_limited`, `Retry-After` until the
window ends. Wrong setup and invite tokens count against the address.

**Lockout** (`lockout.enabled`, off by default). When a key reaches the cap it
is locked for `lockout.minutes` (15 by default, 1 to 10,080), however the window
goes on, and the lock is checked before the password. `lockout.scope` decides
which key is locked:

- `ip_account` (the default, and the one to use): the address and the username
  together. Someone elsewhere cannot lock a person out, and an office behind one
  address is not locked out for one person's mistakes.
- `ip`: the address, for every username.
- `account`: the username, from every address. This refuses the right password
  for the length of the lock, so anyone who knows a username can keep that
  person out. The security page says so next to the choice and asks to confirm.

A locked attempt is `429 rate_limited` with `Retry-After` until the lock ends,
and is recorded in the audit log.

### Audit log

What the server's administrators and its people do that matters goes to the
audit log in the state database (see `GET /api/admin/audit`). Rows are written
in batches in the background, from a queue of at most 10,000; when the queue is
full, rows are dropped and counted, and a `dropped` row says how many.

Every kind of row is registered once, in `internal/auditlog`, with its
category and who sees it; a test fails when the code writes a kind that is not
registered there. A row is filed under its kind's category when it is written
and keeps it. The categories:

| Category | Kinds | Switch |
|---|---|---|
| `security` | sign-in failures (`login_failed`, `rate_limited`, `lockout`, `passkey_clone`), passkeys, identities, addresses, passwords, `sessions_ended`, sign-in providers, mail, `secret_changed`, email domains (`domain_*`) | core: always written |
| `admin` | settings and rules, plans, tenants, people, invites, groups, tokens and grants, bulk jobs | core |
| `data` | deleting, restoring and purging spaces, `space_taken_over`, `space_transferred`, space and page views, exports, `audit_downloaded` | core |
| `system` | `dropped`, `audit_purged` | core |
| `signins` | `login`, a session was started | `audit.signins`, on by default |
| `reads` | a member opening a page or file | `audit.reads`, off |
| `downloads` | attachments and file downloads | `audit.downloads`, off |
| `api` | token and MCP calls | `audit.api`, off |

A core category has no switch: nothing turns it off. An optional one is a
boolean setting with a site and a tenant layer (see "The layered keys"); while
it is off for a row's tenant the row is not written at all. `reads`,
`downloads` and `api` have their switches now and are written once their
events exist.

A tenant's administrators see the rows about their tenant, a site
administrator's changes to it included. Only the server's own doings are
site-only: `provider_created`, `provider_changed`, `provider_deleted` (the
site's own providers; a tenant's are `tenant_provider_*`, its own rows),
`connection_provider_changed`, `mail_changed`, `mail_test`, `rate_limited`,
`lockout`, `dropped` and `audit_purged`.

Retention is the server's, one table for every tenant, so site settings only,
and all of it the site's to set: there is no floor for any category.

| Key | Default | Meaning |
|---|---|---|
| `audit.keepDays` | 90 (0 to 3,650) | a row is kept while it is younger than this; `0` is no age limit (keep forever by age) |
| `audit.keepRows` | 100,000 | ... or while it is among this many newest rows |
| `audit.keepMode` | `or` | `or`: kept if either holds (deleted when old and outside the newest rows); `and`: kept only if both hold |
| `audit.optionalKeepDays` | 30 (1 to 3,650) | a row of an optional category is also deleted after this many days |
| `audit.keepDays.<category>`, one for each of the eight categories | none (0 to 3,650) | that category's own days, kept by them alone, whatever `keepRows` and `keepDays` say: `0` keeps it forever, `n` deletes its rows after `n` days; none (`null`) follows the rules above |

The daily sweep deletes in batches by `(category, id)`, one range for each
category; it never scans the table.

A site administrator can also purge by hand, see `POST /api/admin/audit/purge`.

| `kind` | When | `detail` |
|---|---|---|
| `connection_provider_changed` | a provider was configured, changed or removed (administrators; secrets as `"changed"`) | `provider`, `type` |
| `account_connected`, `account_disconnected` | a person connected or disconnected an account of a provider | `provider`, `login` |
| `login` | a session was started | `method` (`password`, `passkey`, `oidc`, `saml`, `github`), `provider` (slug), `created` (an account was made) |
| `login_failed` | a sign-in was refused | `method`, `reason` (`bad_password`, `disabled`, `local_login_refused`, `passkey_required`, `passkey_failed`, `captcha_failed`, `rate_limited` for an attempt turned away by a lockout or by the address's failure cap, an `sso_error` code), `provider`; the username as typed, also for a name nobody has. `captcha_failed` and an address's `rate_limited` are written at most once a minute per address (IPv6 by /64) and reason; a passkey sign-in whose ceremony was never begun, like a single sign-on callback that matches no sign-in, is not written |
| `rate_limited` | a key first went over the per-minute limit or a failure cap in a window (once per key and window, not per request) | `limit` (`per_minute`, `ip`, `username`, `pair`) |
| `lockout` | a key was locked | `scope`, `minutes` |
| `passkey_clone` | a passkey's counter went backwards | `passkey` (id) |
| `identity_linked`, `identity_unlinked` | an identity was attached or removed | `provider`, `by` (`user`, `email`, `admin`) |
| `sessions_ended` | a site administrator ended a person's session or all of them | `by` (`admin`), `count` |
| `passkey_added`, `passkey_removed` | a passkey was registered or deleted | `passkey`, `by` (`user`, `admin`) |
| `secret_changed` | a secret of the site or of a tenant was set or cleared (see "Secrets") | `name`, `scope` (`site`, `tenant`), `change` (`set`, `cleared`); never the value. The captcha's secret and the mail password also write it, through their own forms, as `setting_changed` with `key` `captcha.secret` and `to` `"changed"`, and `mail_changed` |
| `provider_created`, `provider_changed`, `provider_deleted` | a sign-in provider changed | `slug`, `kind`, the changed fields (secrets as `"changed"`) |
| `tenant_provider_created`, `tenant_provider_changed`, `tenant_provider_deleted` | a tenant's own sign-in provider changed (3.6 d3; its tenant's rows) | as `provider_*` |
| `user_changed` | an administrator changed `role`, `disabled`, `passkeyRequired`, `email` or `emailVerified` | the changed fields |
| `token_created` | an API token was made (`POST /api/account/tokens`) | `token` (id), `name`, `access`, `spaces` (ids); never the secret |
| `token_revoked` | an API token was revoked through the server (once: revoking one already revoked writes nothing) | `token`, `name`, `by` (`user`, `admin`) |
| `grant_revoked` | an OAuth grant was revoked through the server | `grant`, `client` (its name), `by` (`user`, `admin`) |
| `dropped` | rows were lost because the queue was full | `count` |
| `domain_added`, `domain_code_made`, `domain_changed`, `domain_removed` | a tenant's claim of an email domain was made, given a new code, changed (`restrictOrgs`, `provider`, `join`) or removed | `domain`; `restrictOrgs`, `provider`, `join`; `verified` (removed) |
| `domain_checked`, `domain_verified` | a check by hand ("Check now") asked DNS; it verified the claim | `domain`, `result`, `via`, `seen`, `claimed` (another tenant holds it); `domain`, `via` |
| `domain_missing`, `domain_lapsed` | the daily recheck first did not find a verified domain's record; 7 days later still not, and the claim lapsed (with a notification) | `domain`, `check`; `domain`, `missingSince`, `check` |
| `domain_assigned`, `domain_unassigned` | a site administrator moved a verified domain to the tenant, or away from it (each tenant's row names only the domain) | `domain` |
| `join_auto` | an address of a domain whose join is `auto` confirmed its registration: the account is in the tenant (3.6 d5) | `username`, `email`, `domain` |
| `join_requested` | a confirmed registration became a request to join (with a notification to the tenant's administrators) | `request`, `username`, `email`, `domain` |
| `join_approved`, `join_denied` | an administrator approved (the account is made; with a notification to it) or denied a request | `request`, `username`, `email`, `domain`; `role` (approved) |
| `join_withdrawn` | the person withdrew their request from the waiting page | `request`, `username`, `email`, `domain` |
| `join_cancelled` | a waiting request was cancelled because its domain went (one row each) | `request`, `username`, `email`, `domain`, `reason` (`join_off`, `removed`, `lapsed`, `moved`) |
| `audit_purged` | a site administrator purged the log by hand (`POST /api/admin/audit/purge`); one row for each purge, written when its job ends, never deleted by a purge or by the retention trim | `category` (`all` for every one), `before` (RFC 3339), `deleted`, and `partial: true` when the purge did not finish |

Each row has `at`, `kind`, `category`, `capacity` (`site_admin` or
`tenant_admin` when written while acting in that capacity, else `null`),
`actor` (the user who acted, if any), `user` (the account it is about, if any),
`username` (as typed, for refused sign-ins), `ip` and `detail`. Groups add rows of their own (see "Audit" under "Groups").

### Sign-in error codes

The codes of this section, besides those of "Conventions":

| Code | Status | When |
|---|---|---|
| `captcha_failed` | 400 | a captcha was required and missing, wrong, expired or used, or a third-party check failed |
| `rate_limited` | 429 | the per-minute limit, a failure cap or a lockout; `Retry-After` in seconds |
| `local_login_refused` | 403 | `ssoOnly` is in effect and a non-administrator signed in with a password or a passkey, or accepted an invite with a password |
| `passkey_required` | 403 | the password was right but this person must use a passkey; or the session is held until a passkey is registered |
| `passkey_failed` | 401 (sign-in), 400 (registration) | the ceremony is unknown or expired, or the credential did not verify |
| `recent_login_required` | 403 | registering a passkey needs a sign-in in the last 10 minutes |
| `too_many_passkeys` | 409 | the account has 20 passkeys |
| `unauthenticated` | 401 | wrong username or password, or a disabled account |

The codes of the security settings and the provider settings are listed with
them in "Administration".

## Account

Accounts mode, signed in.

### `GET /api/account`

```json
{
  "user": {…},
  "hasPassword": true,
  "passkeys": [{"id": "01JD3N7X2B", "name": "MacBook", "created": "2026-11-21T08:30:00Z", "lastUsed": "2026-11-22T09:10:00Z", "synced": true}],
  "emails": [{"email": "kazuha@example.com", "primary": true, "verified": true, "created": "2026-11-21T08:30:00Z"}]
}
```

`emails` are the account's addresses, the primary first (see "More than one
email address"): its primary alone while the tenant's
`accounts.multipleEmails` is off.

`synced` is whether the authenticator says the passkey is backed up (synced
between devices). The accounts the person linked to sign in are at
`GET /api/account/links`.

The page offers "Link" for every enabled provider of `GET /api/auth/providers`
that the user has no linked account at.

### `PATCH /api/account`

`{"name": "…", "username": "…", "email": "…"}`, each optional. `200` with
`{"user": {…}}`. A changed `email` makes `emailVerified` `false` (ask for the
link with `POST /api/account/email/verify`); the old address is sent a note. A
changed `username` is as at sign-up (`409 exists` when taken); the old one signs
in no more. Each change is audited as `user_changed` with `old`.

### `POST /api/account/password`

`{"current": "…", "new": "…"}`. `current` is required when the user has a
password and must be left out when they have none (an account made through
single sign-on). Wrong `current`: `403 forbidden`. `204`. The person's other
sessions end; their API tokens and OAuth grants stay (they changed it
themselves and still hold the account; an administrator's temporary password,
below, ends those too). In a session held for a new password, `new` equal to
`current` (the temporary password) is `400 invalid`; a person who must have a
passkey and has none of this host stays held, now with `hold: "passkey"`,
instead of the hold being lifted.

### Linked accounts

#### `GET /api/account/links`

```json
{"links": [{"id": "01JD3M2A4C", "providerId": "company", "provider": "Company SSO", "slug": "company",
            "kind": "oidc", "label": "kazuha@example.com", "signin": true, "created": "2026-11-21T08:30:00Z"}]}
```

Every account the person has at a provider, oldest first, with the providers they
may connect repositories at as `providers` (see "Connections", `GET
/api/account/links`, for the whole answer: a link also has `login`, `repos`,
`state`, `spaces`). `provider` is the provider's name, `providerId` its own id (a
site provider's is its slug). `label` is the `email` or `preferred_username`
claim (for SAML the address; for GitHub the login name) seen at the last
sign-in, for display only. `signin` is whether the link may sign in: a link made
by connecting repositories does not, until the person links it for sign-in.

#### `POST /api/account/links/{provider}/start`

`{"purpose": "signin" | "repos", "return": "/settings"}` (both optional; `repos`
connects repositories, see "Connections"). `200` with `{"url": "…"}`: where to send the browser
(the provider's authorisation or SAML address); the attempt cookies are set.
The provider's callback returns to `return` (default `/settings`), or to
`/settings?sso_error=<code>`. `{provider}` is the provider's id (its slug, for
the site's). `409 already_linked` when the person has an account linked at that
provider already, one per provider per person: to switch, unlink first (the
callback says the same when another tab got there first). `404 not_found` for an
unknown, disabled or organisation-owned provider. `POST /api/auth/{kind}/{slug}/start`
with `{"intent": "link"}` still starts the same attempt.

Linking ignores addresses: it attaches exactly the account the person proved.
Linking the account the person has linked already changes nothing; one another
person has linked at that provider is `identity_in_use`.

#### `DELETE /api/account/links/{id}`

`204`. Refused with `409 last_method` when it would leave the user with no way
to sign in (no password, no other linked account, no passkey): nothing is
changed then, the repository tokens included. `404 not_found` for one the person
does not have. Whether it may go and its removal are one step, and the
repository tokens it holds go with it; the platform is asked to revoke the
grant afterwards (best effort). Linking again afterwards works. `DELETE /api/account/links/{id}/repos` drops only the tokens.

### Account passkeys

`GET /api/account/passkeys` is `{"passkeys": […]}` as in `GET /api/account`.

#### `POST /api/account/passkeys/begin`

`{"name": "MacBook"}` (1 to 64 characters; default "Passkey"). Needs a session
signed in within the last 10 minutes, by any method: otherwise
`403 recent_login_required`, and the page sends the person through sign-in again
with `return` back to this page. `409 too_many_passkeys` at 20. `200` with
`{"ceremony": "…", "options": {…}}`: the `PublicKeyCredentialCreationOptions` for
`navigator.credentials.create`, which already excludes the user's passkeys.

#### `POST /api/account/passkeys/finish?ceremony={id}`

The body is the credential the browser returned. `201` with
`{"passkeys": […]}`. `400 passkey_failed` when it does not verify,
`409 exists` when that credential is already registered. A held session
(`hold: "passkey"`) is released.

#### `PATCH /api/account/passkeys/{id}`

`{"name": "…"}`. `200` with the passkey. `404` for another user's.

#### `DELETE /api/account/passkeys/{id}`

`204`. `409 conflict` when it would leave the user with no way to sign in.
Deleting the last passkey of someone who must have one is allowed: their next
sign-in is held until they register another.

## Users and invites

Accounts mode. Everything here needs the `admin` role (`403 forbidden`
otherwise) except the two invite endpoints marked public.

### `GET /api/users`

One keyset page of the people in the administrator's scope (their tenant;
every tenant for a site administrator, narrowed by `?tenant=`), who are not
deleted:

```json
{"users": [{…}], "next": "users.created.asc:1764000000000:6FAVM1F0R3"}
```

`next` is `""` after the last page; send it back as `cursor`. A cursor made for
another sort, order or list is `400 invalid`: it begins with the name of the list
(`users.`, `spaces.`, `people.`), so one list never takes the cursor of another.

| Parameter | |
|---|---|
| `limit` | 1 to 200, default 50 |
| `sort`, `order` | `created` (default, oldest first), `name` (by the name, ignoring case, then the id) or `lastActive` (most recent first; people who were never active last, in id order); `order` is `asc` or `desc`, the sort's own when left out |
| `q` | keeps the users whose `username`, `name` or email address contains the text, ignoring case (Unicode case folding, so `STRASSE` finds `Straße`, the same on SQLite and PostgreSQL) |
| `role` | `member`, `admin` (of a tenant or of the server) or `site` (administrators of the whole server) |
| `state` | `enabled` or `disabled` |
| `inactiveDays`, `activeDays` | last active before the last `n` days (or never), or within them; 0 to 36500 |
| `enabled` | `1` is `state=enabled`; the picker's older spelling (`0` keeps all) |

Anything else the list does not have is `400 invalid`. The sorts and the search
read an index (`users_live*`), and the passkey and linked-account counts are
looked up for the page's ids alone, so a page costs the same whatever its place
in the list. A picker that looks for one person sends `q` and `limit`.

Each user here also has `passkeys` (how many), `sso` (how many linked accounts),
`quota` and, once they have been active, `lastActive` (the time of their last
activity, written at most every few minutes).

### `GET /api/users/{id}`

`{"user": {…}}` as one row of the list has it, with `emails`, the person's
addresses as `GET /api/account` shows them (read-only here). A site
administrator across the whole site sees every address. A tenant's
administrator, and a site administrator acting in a tenant's scope, see the
primary address and the addresses in the tenant's verified domains
(`internal/auth`'s `OrgDomains`, which 3.6 (d) supplies; until a tenant has a
verified domain that is the primary address alone). The lists have the primary
address only; the audit rows of the address kinds (`email_added`,
`email_verified`, `email_removed`, `email_claim_withdrawn`,
`email_primary_changed`, `email_released`, `registered`, `user_changed` in
its `email` and `old.email`, and the typed `user` of each result of
`members_batch`, `group_members_batch` and `invites_batch`, when it is an
address; each kind declares its places in `internal/auditlog`) say `"(hidden)"` for an address such a reader may
not see, in the list and in the CSV. `404 not_found`
for a person who does not exist, is deleted, or is of another tenant than a
tenant administrator's.

### `GET /api/users/{id}/memberships`

`{"spaces": [{"id", "name", "role", "personal"}], "groups": [{"id", "name", "source"}]}`:
the spaces the person is a member of (their own role there) and their groups,
at most 200 of each. A tenant administrator's answer leaves the person's personal
space out.

### The sessions and connected accounts of a person (3.10)

**Site administrators only.** An administrator of a tenant never manages or audits
another person's sign-ins: they are answered `404 not_found` ("No such user."),
as for a person who does not exist, so nothing tells them the page is there.

#### `GET /api/users/{id}/sessions`

`{"sessions": [{"id", "userAgent", "method", "created", "lastUsed"}]}`, the one used
last first (a person keeps at most a few). `userAgent` is the browser's name for
itself, cut to 200 bytes, `""` when it sent none; `method` is how they signed in:
`password`, `passkey`, `sso:{provider slug}`, `invite`, `registration` or `setup`
(`""` for a session made before the server recorded it). `id` is the hash the server
keeps, never the cookie. A session held until the person registers a passkey has
`"held": true`. The session of the request itself (the administrator looking at
their own page) has `"current": true`, so a client can mark it "this device" and
ask before it is ended. The server does not know where a sign-in came from, so there is no
place.

#### `DELETE /api/users/{id}/sessions/{session}`

`204`. The browser that held it is answered `401` with `signed_out_elsewhere` the
next time it comes (see "Why a session ended"), and `sessions_ended` is written to
the audit log (`by: "admin"`, `count`). `404 not_found` for a session the person
does not have.

#### `DELETE /api/users/{id}/sessions`

Ends every session of the person: `200` with `{"ended": 2}`; each answers
`signed_out_elsewhere` afterwards, and one `sessions_ended` row is written. It
ends browser sessions only: API tokens and OAuth or MCP grants of the person keep
working until they are revoked on their own page. This includes the caller's own
sessions when the person is the caller.

#### `GET /api/users/{id}/links`

`{"links": [{"id", "providerId", "provider", "slug", "kind", "label", "signin", "created"}]}`: the
accounts they linked to sign in (the same as `GET /api/account/links` shows its owner).

#### `DELETE /api/users/{id}/links/{link}`

`204`; `identity_unlinked` (`by: "admin"`) is written. `409 last_method` when it is
the only way they can sign in now: what is left is counted only if it works today,
that is a password or a passkey while password sign-in is not refused to them, and a
connected account of a provider that is set up and enabled. `404 not_found` for one
they do not have.

### `PATCH /api/users/{id}`

`{"role": "member", "disabled": true, "passkeyRequired": true, "email": "…", "emailVerified": true}`,
each optional. `200` with `{"user": {…}}`. `409 conflict` when the change would
leave no enabled administrator, when an administrator tries to disable themself,
or, while `ssoOnly` is in effect, when it would leave no enabled administrator
with a password. `passkeyRequired: true` is refused with
`400 passkey_unavailable` while passkeys cannot be used (see "Passkeys").
Setting `email` without `emailVerified` makes it unverified. From 3.3 the body
may also have `quota` (see "Quotas"), and the users of the administrators' lists
and of this answer have `quota`: `{"spaces": null, "bytes": null, "pages": null}`
(the person's own limits; `null` follows the tenant). From 3.3,
`emailVerified: true` sent by a tenant administrator (not a site administrator)
is stored and shown, but the address does not count for merging a sign-in by
address (see "Which account an identity signs in to") until a site
administrator confirms it or the person does. From milestone 3.1 the body may also have `remoteExempt`
(site administrators only, `403 forbidden` for a tenant administrator; see
"Server storage", "Exemptions").

Editing a person (an administrator's page of them). The body may also have
`name` (1 to 100 characters) and `username` (as at sign-up; `409 exists` when
taken, also by a retired name). A site administrator edits anyone; changing
another site administrator needs `site.admins.manage`; a tenant administrator
edits only the people of their own tenant (`404` for another tenant's) and never
a site administrator (`403 forbidden`). `emailMode` says what to do with a new
`email`: `"verify"` (the default) sends the person a link that proves it (when
the server's mail is on) and `"mark"` says it is verified, which counts as
verified for a site administrator and, for a tenant administrator, is stored
and shown but does not hold the address (see "Email as the unique UPN"); any
other value is `400 invalid`. The address stays unique on the whole server
(`409 email_taken`). When the address changes the old one is sent a note (if it
was verified and mail is on), and the answer has `"verification"`: `"sent"` or
`"not_sent"` (mail is off or capped). Every change writes `user_changed` with
the new values as keys and `old`, the values they replaced, and the person gets
a notification (`kind: "account_changed"`, `fields`, `by`: never the values;
written with the `user_changed` row, in the same transaction). A change a person makes to their own profile is audited the
same way and notifies nobody.

### `POST /api/users/{id}/password`

`{"password": "…"}`: a temporary password an administrator chose, for who may
change the person as above. `204`. The administrator never sees a password
afterwards; it is in no audit row (`password_set_by_admin` has `temporary`). It
ends every session the person has, any reset link, their personal API tokens
(revoked) and their OAuth grants (ended), and the person gets a
notification (`kind: "password_set"`, `by`; written with the
`password_set_by_admin` row, in the same transaction). The next sign-in with it succeeds, but
that session is held: `GET /api/session` has `hold: "password"`, every route but
`POST /api/account/password` (the new password, with the temporary one as
`current`; the same one again is `400 invalid`), the session, meta, account and
sign-out answers `403 password_change_required`, the passkey endpoints included.
A passkey sign-in while the temporary password is in force is held the same
way, and a person who must use a passkey and has one is refused with it (see
"When a passkey is required"). Someone who signs in with a passkey while a
temporary password is in force is asked to change their password, and needs the
temporary password from their administrator to do it (it is `current`).
Choosing a password lifts the hold (or makes it a passkey hold, for someone who
must have a passkey and has none); so does any other way of setting one (a
reset link). To send a reset link instead, use `POST /api/users/{id}/password-reset`.

Until 3.5 an administrator could not read another user's space through the API;
from 3.5 they may, in a view that is recorded each time (see "Viewing a space").
Whoever can read the server's disk can read every file.

### `GET /api/users/{id}/passkeys`, `DELETE /api/users/{id}/passkeys/{passkey}`

The user's passkeys as in `GET /api/account` (names, dates, no key material),
and deleting one (`204`), for a person who lost a device. Deleting is recorded
in the audit log.

### Making an account by hand

#### `POST /api/admin/users`

An administrator makes an account for someone else. It is an administrator's act:
**no registration mode gates it** (it works under `closed`, where invites do not),
nor does `registration.domains`. It is bounded by `quota.people`
(`409 people_limit`, as every way an account is made). Needs `tenant.users.manage`
in the tenant.

```json
{"username": "erin", "name": "Erin", "email": "erin@example.com", "role": "member",
 "tenant": "01JD3M9RTE", "siteAdmin": false, "lang": "en"}
```

Only `username` is required. `name` defaults to the username; `role` is `member`
(the default) or `admin`; `lang` (`en` or `zh-Hans`; else `Accept-Language`) is the
language of the message sent to the person. The rules are sign-up's: the username
as in `POST /api/invites/accept` (`409 exists` when taken, or retired), the address
as in "Email as the unique UPN" (`409 email_taken` when any account holds it,
verified or not).

`tenant` is the tenant the account belongs to; left out it is the acting one: the
administrator's own, or the one the `PetalBook-Scope` header names (see "Acting
scope"). A **site administrator** may make accounts in the default tenant (personal
accounts) or in any organisation; an **organisation administrator** only in their own
(another tenant is `404 not_found`, as a tenant that does not exist). Either may
grant `member` or `admin`. `siteAdmin` follows an
invite's rule, but has to be said: an administrator of the default tenant made by
someone who manages site administrators (`site.admins.manage`) is one of the whole
server, so `role: "admin"` there without `siteAdmin` is `400 site_admin_explicit`;
`siteAdmin: true` makes a site administrator, `false` an administrator of the default
tenant only. Only someone who manages site administrators may send `true` (`403
forbidden`). A disabled tenant is
`409`.

The administrator never learns a password. The account starts with none and an
**unverified** address, and `201` answers:

```json
{"user": {…}, "signIn": "link", "link": "https://notes.example.com/reset-password#…",
 "expires": "2026-12-04T08:00:00Z", "mail": "sent"}
```

- `link` is a one-time link that sets the person's password
  (`POST /api/password/reset/check` and `POST /api/password/reset` take its token,
  as for a reset). It works once and for 7 days, and it is shown only in this answer;
  the server keeps a hash. It does **not** verify the address (nothing says it
  reached it). Giving the person a temporary password, or any password change, ends it. Until it is used, "forgot password" for the account (once its address is verified by a site administrator) sends an ordinary reset link, not "sign in with your provider".
- `mail` says whether the same was **mailed**: `sent` (mail is on and the account has
  an address: the person gets the message in their language, with a link of its own,
  and following that one **verifies the address** as well as setting the password;
  the first use of either ends both), `none` (mail is off, or there is no address:
  hand the link over yourself), or `limited` (the address has had its messages for
  the hour, or the queue was full).
- `signIn` is `sso`, with `link` and `expires` `null`, when the tenant requires its
  single sign-on for this person: `signin.ssoOnly` on and the person a member, or an
  administrator while `signin.ssoOnlyAdmins` is on (a site administrator keeps the
  password). No password link is made, since a password is refused there. The address
  is required (`400 invalid` without one) and must be one the organisation's SSO can
  vouch for: on a domain the tenant verified, or one routed to it (`400
  email_domain_not_verified` otherwise: any other address could never be claimed, and
  would only be kept from every other sign-in). In the default tenant no such account
  is made (`400 sso_default_tenant`): with the site's own `ssoOnly`, personal accounts
  sign in through the SSO themselves, as the registration mode allows. The account is marked as
  **awaiting its first sign-in**, which claims it (below).

**The first SSO sign-in claims it.** A sign-in that finds no identity link attaches its
identity to such an account, marks the address verified by the provider
(`verified_by` `provider:<slug>`), ends the wait and writes `identity_linked`
(`by: "sso_claim"`) and the login, when all of these hold: the provider is the tenant's
own SSO (OIDC or SAML), or a site SSO that routes the address's verified domain to that
tenant; the provider vouches for the address under the trust rules (a tenant's own SSO
only for the domains the tenant verified; an Apple relay never); that address is the
account's; and the account is still waiting, enabled, and in that tenant. The site's
social providers and the custom OAuth2 kind never claim. The registration mode does not
matter: the account exists. An address a waiting account holds is never taken from it by
a sign-in, nor made a second account's: any other sign-in that vouches for it is
refused as `email_in_use` (`?sso_error=email_in_use`). That holds only
while the account's own tenant can still vouch for the address (a verified domain of it,
or one routed to it): once the domain lapses or moves to another tenant, the account is
an ordinary unverified holder of the address again, and the address is not locked. A
`PATCH /api/users/{id}` of a waiting account's address to one the tenant cannot vouch
for (or to none) is `400 email_domain_not_verified`.

Audit: `user_created` (the actor, the person, the tenant; `role`, `siteAdmin`, `email`,
`signIn`, `mail`), and `password_reset` with `by: "welcome"` when the person sets the
password. Errors: `400 invalid` (username, address, role), `403 forbidden`, `404
not_found` (the tenant), `400 email_domain_not_verified`, `400 sso_default_tenant`, `400 site_admin_explicit`, `409 exists`, `409 email_taken`, `409 people_limit`.

### `POST /api/invites`

`{"role": "member", "days": 7}`, both optional (`days` 1–30). `201` with

```json
{"id": "01JD3M9RTE", "token": "…", "role": "member", "created": "…", "expires": "…"}
```

`token` is shown this once; the server keeps only a hash. The link to send is
`<public url>/invite#<token>`. An invite works once.

### `GET /api/invites`

`{"invites": [{"id", "role", "created", "expires", "createdBy"}]}`: the ones
neither used nor expired.

### `DELETE /api/invites/{id}`

`204`.

### `POST /api/invites/check` (public)

`{"token": "…", "captcha": {…}}`. `200` with
`{"role": "member", "expires": "…", "username": "", "name": "", "email": ""}`
(the last three are what the invite fills in: see `POST /api/admin/tenants`; an
ordinary invite has them empty), or `404 not_found` for a token that is
unknown, used or expired (a failure of the address). `captcha` as for the
`invite` context.

### `POST /api/invites/accept` (public)

```json
{"token": "…", "username": "tang", "name": "Tang", "password": "…", "captcha": {…}}
```

`201` with `{"user": {…}}` and a session cookie; the user gets a personal
space. A `username` or `name` left empty (or out) takes what the invite fills
in; the invite's `email`, if it has one, becomes the account's (not verified).
`404 not_found` for a bad token, `409 exists` when the username is
taken (the invite stays usable). While `ssoOnly` is in effect, an invite for
the `member` role cannot be accepted with a password: `403 local_login_refused`
(the page offers the providers instead, which accept it with
`intent: "invite"`); an account made that way could never sign in.

## Spaces

### Space

```json
{"id": "01JD3K8Q7Z", "name": "My notes", "role": "owner", "permissions": ["read", "write", "members", "manage"], "personal": true, "sync": "local", "inbox": "Inbox", "disabled": false}
```

`role` is the caller's role in the space: `owner`, `editor` or `viewer`, and
empty for an administrator who is not a member. `permissions` is what the
caller may do in the space, so a client shows or hides controls by it and never
by the name of a role:

| Permission | Allows |
|---|---|
| `read` | seeing pages, history, attachments and search |
| `write` | creating, changing, moving and deleting pages and folders; tags, attachments, imports, drafts |
| `members` | adding, changing and removing members |
| `manage` | renaming the space, its inbox, its remote and sync, rebuilding the index |

An `owner` has all four, an `editor` `read` and `write`, a `viewer` `read`.
In single-user mode the one user has all of them.
From 3.2 `role` and `permissions` are the caller's effective ones, which
include the roles of their groups, and the Space also has `directRole` (the
caller's own role, `""` when they reach the space only through groups) and
`viaGroups`; see "Effective permission" under "Groups".
`personal` marks the space made together with the account. `sync` is `local`
(the repository exists only on the server) or the `state` of its remote (see
"Remote repository"). `inbox` is the folder that quick notes go to: a folder
path, never empty. It is only a name: the folder is made when the first page
is created in it, and changing the setting moves nothing. In single-user mode
it is set with `--inbox` (default `Inbox`). `disabled` is true when an
administrator has switched the space off (see "Administration"): it stays in
its members' lists, with the permissions they would have, but every request
that opens it answers `403 space_disabled`.

From milestone 3.1 a space also has `storage` (see "Server storage"):

```json
"storage": {"cache": "partial", "assets": "repository",
            "remote": {"required": "github", "deadline": "2026-12-31T00:00:00Z", "overdue": false}}
```

`cache` is `local`, `full`, `partial`, `evicted` or `restoring`; `assets` is
`space` (attachments are files of the space's repository) or `repository`
(they are in an attachment repository). `remote` is `null` unless the storage
policy requires something of this space that it does not have yet.

### `GET /api/spaces`

`{"spaces": [{…}]}`: the caller's spaces, the personal one first, then by
name.

### `POST /api/spaces`

Accounts mode. `{"name": "Tang", "inbox": "收件箱"}`: `name` is 1–80
characters; `inbox` is optional (default `Inbox`) and must be a valid folder
path. `201` with the space.
The server makes the Git repository. The caller is its owner. `403 forbidden` when
the caller already owns as many spaces as the server allows (`--max-spaces`,
3 by default). From 3.3 this is `403 quota_exceeded` with `quota`, `limit` and
`used` (see "Quotas"), and the limit is the setting `quota.spacesPerPerson`, or
the person's own, or the tenant's total of spaces.

With `storage.sync`, the space follows a repository on a provider from its first
commit (see "Connections", "Spaces and remotes", and "The GitHub provider",
"Creating a space with a repository"):

```json
{"name": "Tang", "inbox": "收件箱",
 "storage": {"sync": {"provider": "github", "owner": "kazuhahub", "name": "tang-notes", "private": true,
                      "description": "Notes on Tang poetry", "create": true}}}
```

`201` with the space; its `sync` is then the state of its remote. When it
fails, no space and no record is left on the server; what can be left on
the provider, and how to retry, is described there.

From milestone 3.1 (see "Server storage"):

- `storage.sync.assets` asks for an attachment repository made with it, on the
  same owner, with the same visibility and credential: `true` names it
  `<name>-assets`, `{"name": "tang-notes-assets"}` names it, `false` makes
  none, and leaving it out follows the setting `storage.assets` (separate by
  default; see "Where attachments go"). A request that goes against a lock or a
  restriction of that setting answers `409 setting_locked` or
  `409 setting_restricted`; nothing is made.
- `remote`, shaped like the body of `PUT …/remote` but with a `token`
  credential only (an SSH key has to be added to the repository first, which
  needs a space to hold it: `400 invalid`), connects the new space to an
  existing repository on any allowed host before it becomes visible, under
  the same all-or-nothing rule. This is the "other (by address)" choice of
  "存在哪里". `remote` and `storage.sync` cannot both be given
  (`400 invalid`).
- While the storage policy requires a remote of new spaces, a request without
  what it requires answers `409 remote_required`, before anything is made.

### `PATCH /api/spaces/{space}`

Accounts mode, needs `manage`. `{"name": "…", "inbox": "…"}`, each optional; at
least one must be given. `200` with the space.

## Members

Accounts mode; single-user mode answers `404`. The people who belong to a
space, each with a role (`owner`, `editor` or `viewer`, see "Space"). A space
always keeps at least one owner. A personal space (`personal: true`) belongs to
its person alone and never has other members. An administrator is not a member
of the spaces they open, so they are not listed here and not in `GET /api/spaces`,
but may use these endpoints (the permissions an administrator has in a space
they are not a member of are `members` and `manage`), except for owners and
themselves: from 3.6 that is taking the space over, `403 takeover_required`
(see "Taking over a space"). From 3.2 a shared space's
members may also be groups, with endpoints of their own (see "Groups"); this
section is about people.

### Member

```json
{"userId": "01JD3K8Q7Z", "username": "tang", "name": "Tang", "role": "editor", "you": false}
```

`you` is true for the caller.

### `GET /api/spaces/{space}/members`

Needs `read`. `{"members": [{…}]}`: owners first, then by name.

### `POST /api/spaces/{space}/members`

Needs `members`. `{"username": "tang", "role": "editor"}`. `201` with the member.
`404 user_not_found` when no enabled user has that username; `409 exists` when
they are a member already; `409 personal_space` for a personal space;
`400 invalid` for a role that is not one of the three.

### `PATCH /api/spaces/{space}/members/{user}`

Needs `members`. `{"role": "viewer"}`, `{user}` being the member's `userId`.
`200` with the member. `404 not_found` when they are not a member;
`409 last_owner` when it would leave the space without an owner. The change
applies to the member's very next request.

### `DELETE /api/spaces/{space}/members/{user}`

Needs `members`, except that a member may remove themself (leave the space)
with no more than `read`; leaving only touches the membership, so it works in a
space an administrator has switched off too (removing someone else does not: that
goes through the space, which cannot be opened). `204`. `409 last_owner` when no
other owner who can sign in would be left (a disabled user cannot be the one
remaining owner; the same goes for `PATCH`): make someone else an owner first.
The removed member's very next request to the space answers `404 not_found`.

## Adding people in batches

The web app adds people from a dialog: a picker (`GET /api/people`, and
`GET /api/groups` for groups) or a CSV file checked row by row and then
applied. The file is read by the browser; the server takes rows.

### `GET /api/people?q={text}&limit={n}`

Any signed-in user (not an API token), accounts mode only (`404` in single-user
mode). The enabled people of the caller's **own tenant** whose username starts
with `q` or whose name contains it, ignoring case, by name:
`{"people": [{"id", "username", "name"}]}`, and nothing else of a person (no
address, no role, no groups). `q` is 2 to 80 characters (`400 invalid`);
`limit` 1 to 20 (default 20). A person of another tenant is never listed, by
any spelling. At most 20 searches in 10 seconds per person: `429 rate_limited`
with `Retry-After`. The search reads the tenant's name index and stops after
`limit` rows.

What a regular member sees: every enabled person of their own tenant can be
found by them this way, administrators included, as the people they could
share a space with (the same as the members lists of the spaces they are in
already show). Disabled and deleted people are not listed; nothing but the id,
username and name is.

`GET /api/people?q=&group={id}` searches the people of that group's tenant,
for who holds `tenant.groups.manage` there; anyone else (and a group that does
not exist) is `404 not_found`. `GET /api/spaces/{space}/people?q=` searches
the people of the space's tenant, for who holds `members` in the space (an
administrator of the whole server managing another tenant's space finds that
tenant's people); the same rules and rate otherwise. The dialogs of a group
and of a space's members use these, so the picker finds the people the batch
can add.

### Batch requests

Three endpoints take rows and answer for each of them:

| Endpoint | Needs | Rows |
|---|---|---|
| `POST /api/spaces/{space}/members/batch` | `members` in the space | `user`, `role` |
| `POST /api/admin/groups/{group}/members/batch` | `tenant.groups.manage` | `user` |
| `POST /api/admin/members/batch` | `tenant.invites.manage` in the tenant | `user`, `role`, `name`, `groups` |

The body is `{"dryRun": bool, "rows": [...], "tenant": "..."}` (`tenant` only on
the last, default the caller's). `rows` holds 1 to 1000 rows (`400 invalid`),
the body at most 1 MiB. A row's `user` is a username or an email address (any
address the account holds, compared as addresses are). With `dryRun: true`
nothing is written and the answer is the one the same request would give.
The three together take at most 5,000 rows in 10 minutes per person, dry runs
included: past that, `429 rate_limited` with `Retry-After`.

The answer is `{"dryRun", "results": [...], "summary": {status: count}}`, one
result per row, in order: `{"row" (from 1), "user" (as sent), "role", "status",
"userId", "username", "name", "detail"}`. A refused row never stops the others.
`status`:

| `status` | Meaning |
|---|---|
| `ok` | space or group batch: the person is (would be) added |
| `already_member` | already in the space or group (tenant batch: an account that exists, with no group to add) |
| `not_found` | nobody has this username or address; in a space or group batch also an account of another tenant or a disabled one (as the single add tells them, so that a batch cannot ask the site which accounts exist) |
| `other_tenant` | tenant batch: the account is in another tenant; nothing of it is shown |
| `disabled` | tenant batch: a disabled account of the tenant; it is not invited (it still holds its address) |
| `email_taken` | tenant batch: another account holds the address (a deleted one not yet purged, too); it is not invited |
| `already_invited` | tenant batch: the address has an open, unexpired invite of the tenant; no new one is made or mailed |
| `invalid` | an empty or malformed `user`, an unknown `role`, an unknown group name, a username in a tenant batch for someone with no account |
| `duplicate` | the same person (or address) as an earlier row |
| `forbidden` | the caller's own row in a group batch (use the single add, which asks a reason); an owner added by an administrator who is not a member of the space (a takeover); a tenant batch row that names groups, for a caller without `tenant.groups.manage` in the tenant |
| `added` | tenant batch: an existing person put into the groups of the row |
| `invited` | tenant batch: a new address; an invite is (would be) made |
| `failed` | tenant batch: the group could not take the person (`detail` says why) |

Tenant batch rows. `role` is `member` (default) or `admin`; `name` is the name
the invite starts with; `groups` are names of local groups of the tenant (an
unknown name is `invalid`). Naming groups needs `tenant.groups.manage` in the
tenant as well (a row with groups is `forbidden` without it, and no group is
looked up). An existing person of the tenant is put into the
groups, as `POST /api/admin/groups/{group}/members` would; their role is not
changed by `role` (that is managing people, on their page), and a row whose
`role` differs from theirs says so in `detail`. A new address gets an
invite (7 days) that names the groups, which the account joins when it is made.
The invitation goes by mail when mail is configured: `mail` on the result is
`sent` (queued; the answer carries no token), `none` (no mail is configured
anywhere: `token` is given, once, for the administrator to hand over as
`/invite#{token}`), or `limited` (the cap below is spent: `token` is given). The
answer's `mail` is `{"configured": bool, "sender": "site" | ""}`. The sender is
the server's mail; a tenant's own sender (the feature `mail.ownSender`) takes
its place when a tenant can configure one. Sending is capped: three messages
per address per hour (as every account mail) and 300 invitations per
administrator per hour.

Cost. One lookup per 200 rows for the people by username and one for the people
by address (unique indexes, no scan), one for who is a member already, one for
the group names; for a tenant batch's new addresses one lookup by key each for
an account holding the address (as a single invite does) and one per 200 for
open invites (the `invites_email` index); the writes are one transaction per 200 rows, under the space
lock for a space (one insert by key per person) or one insert per invite, and
two calls of at most 500 for a group. Notifications of a space batch are one
insert each.

Audit. One row per batch, a dry run too (it looks people up as much as an
applied one): `members_batch` (`space`, `name`), `group_members_batch`
(`group`) or `invites_batch` (`tenant`, `sender`), each with `dryRun`, `rows`,
`counts` (status to number) and `results` (`row`, `user`, `status`, `role`). No
token is ever written.

## Groups

Milestone 3.2. Accounts mode; single-user mode has no groups and answers `404`
on every endpoint here. A group is a set of people that can be made a member of a
shared space with a role, like a person. There are two kinds, fixed when the
group is made:

- **local**: made in the app; an administrator adds and removes its people (see
  "Groups" under "Administration").
- **synced**: its people come only from sign-in. A sign-in provider maps values
  of its groups claim to synced groups (see "Groups from sign-in" below), and
  each sign-in through that provider brings the person's groups up to date. Its
  people cannot be changed by hand (`409 group_synced`).

Groups do not nest: a group's people are users, never other groups.

### Group

```json
{"id": "01JD4A2K9P", "name": "Engineering", "description": "", "source": "synced",
 "people": 214, "spaces": 3,
 "mappedFrom": [{"provider": "company", "value": "5f1c…-engineering"}],
 "created": "2026-11-23T08:30:00Z", "updated": "2026-11-23T08:30:00Z"}
```

- `id` has the form of a page id.
- `name` is 1 to 80 characters, unique ignoring case (compared by Unicode case
  folding, the same on SQLite and PostgreSQL, as space names are).
  `description` is at most 500 characters.
- `source` is `local` or `synced`, fixed once made.
- `people` is how many users are in it; `spaces` how many spaces it is a member
  of.
- `mappedFrom` (synced groups only; `[]` for a local one) lists the providers'
  claim values that feed it, read from the providers' settings.

Only administrators see a whole group. Everyone else sees groups only as
`{"id", "name", "source"}`: in a space's members (with the group's role and
`people`), in `GET /api/groups` and in their own account.

### Effective permission

A person's permissions in a space are the union of the permissions of their
direct role, if they are a member, and the roles of every group of theirs that
is a member of the space. The built-in roles nest (`viewer` ⊂ `editor` ⊂
`owner`), so the union is the highest of those roles. In the Space object (see
"Spaces") `role` is the built-in role whose permissions are that union and
`permissions` lists them; `directRole` is the person's own role, `""` when they
reach the space only through groups; `viaGroups` is `true` when at least one of
their groups is a member. A space reached both ways is listed once.

Everything that asks what a person may do in a space uses this effective
permission: every route's check, `GET /api/spaces`, an API token or OAuth grant
(which is never more than its owner's permission now, see "API tokens"), MCP's
`tools/list`, and whether a waiting draft of theirs may be written. A space that
someone reaches only through a group can be put in a token; when the person
leaves the group, or the group leaves the space, the token's next request to the
space answers `404 not_found`, the same as for a removed member.

**What it costs.** A permission check stays the in-memory lookup of 1.7: the
server keeps, per person, the spaces they can reach and one byte of permissions
for each. On a miss the list is read with one query (direct memberships and
group memberships together) and kept; groups are never expanded per request.
A change drops exactly the people it touches, and the very next request of each
reads the database once:

| Change | Whose cached list is dropped |
|---|---|
| a person added to or removed from a local group | that person |
| a sign-in changes a person's synced groups (no change, nothing dropped) | that person |
| a group added to a space, its role changed, or removed | every person in the group (one indexed query for their ids) |
| a group deleted, or a provider's mapping entry removed | every person in it |
| a space deleted | its direct members and every person in its groups |
| a space renamed | nobody: names are kept apart from the per-person lists |

The cache is bounded by entries (2 million space–person pairs by default), not
by people, so a group in hundreds of spaces costs hundreds of pairs per person.
Like the 1.7 cache, it lives in one process.

### Groups as members of a space

Shared spaces only: a personal space never has groups (`409 personal_space`).
These need `members`, like the member endpoints, and an administrator may use
them in any space.

A space always keeps at least one **direct** owner who can sign in: an owner
through a group does not count, because the identity provider may empty the
group. For the same reason the first direct owner stays the Git author of a
space's default commits.

#### Group member

```json
{"groupId": "01JD4A2K9P", "name": "Engineering", "source": "synced", "role": "editor", "people": 214, "yours": true}
```

`yours` is true when the caller is in the group.

#### `GET /api/spaces/{space}/members`

Needs `read`, as before: `{"members": [{…}], "groups": [{…}]}`. `members` is
unchanged: the direct members (owners first, then by name). `groups` are the
groups that are members, by name. Neither is expanded or paged: a space has at
most 200 groups. Each Member gains `viaGroups` (whether one of their groups is
in the space too) and `effectiveRole`.

#### `GET /api/spaces/{space}/members/effective?q={text}&cursor={cursor}&limit={n}`

Needs `read`. Every person who can reach the space, with how:

```json
{"items": [{"userId": "01JD3K8Q7Z", "username": "tang", "name": "Tang", "role": "owner",
            "directRole": "editor", "groups": [{"groupId": "01JD4A2K9P", "name": "Engineering", "role": "owner"}],
            "you": false}],
 "next": "tang"}
```

By username; `q` keeps people whose `username` or `name` contains it, ignoring
case. `limit` 1 to 200 (default 50); `next` is the following page's `cursor`,
`""` after the last. One query a page however large the groups are. Disabled
users are left out. Administrators who are not members are not listed.

#### `POST /api/spaces/{space}/groups`

`{"groupId": "01JD4A2K9P", "role": "editor"}`. `201` with the group member.
`404 group_not_found` for no such group; `409 exists` when it is a member
already; `409 personal_space`; `409 group_limit` at 200 groups;
`400 invalid` for a role that is not one of the three. Its people's very next
request sees the space.

#### `PATCH /api/spaces/{space}/groups/{group}`

`{"role": "viewer"}`. `200` with the group member; `404 not_found` when the group
is not a member. Applies to its people's very next request.

#### `DELETE /api/spaces/{space}/groups/{group}`

`204`; `404 not_found` when the group is not a member. Its people who are not
direct members (and in no other group of the space) answer `404 not_found` on
their very next request to it; their drafts are kept, as for a removed member.

#### Leaving a space

`DELETE /api/spaces/{space}/members/{user}` for oneself removes the direct
membership only. A person who reaches the space only through groups cannot
leave it themself: `409 member_via_group` (the message names the groups). One
who is both a direct member and in a group leaves the direct membership and
keeps the group's role.

### `GET /api/groups?q={text}&limit={n}`

Any signed-in user (not an API token). For finding a group to add to a space:
`{"groups": [{"id", "name", "source"}]}`, the groups whose name contains `q`,
ignoring case, by name. `q` is required (1 to 80 characters, `400 invalid`
otherwise); `limit` 1 to 20 (default 20). Never the people of a group, nor its
mapping.

`GET /api/account` gains `groups`: `[{"id", "name", "source"}]`, the caller's
groups by name.

### Groups from sign-in

A sign-in provider (see "Sign-in providers") may carry a `groups` setting:

```json
"groups": {"enabled": true, "claim": "groups", "missing": "keep",
           "map": [{"value": "5f1c…-engineering", "groupId": "01JD4A2K9P"},
                   {"value": "/staff", "groupId": "01JD4A3B7Q"}],
           "allow": []}
```

- `claim`: where the values are read. OpenID Connect: a claim of the ID token
  (the userinfo response is not read), holding a string or an array of strings
  (default `groups`). SAML: an attribute name, every value of it
  (`memberOf`, or `http://schemas.microsoft.com/ws/2008/06/identity/claims/groups`
  for Entra ID). GitHub: not settable; the values are the organisations the
  person belongs to (their logins, at most 500), read with the `read:org` scope
  whenever the groups are synced or `allow` is set.
- `allow` (3.6 d3): when not empty, only people whose claim names one of these
  values sign in, checked at every sign-in (also with `enabled` off: the claim
  is then read for this alone); a missing claim lets nobody in
  (`sso_error=not_allowed`). On GitHub the values are organisation logins, so an
  `allow` of the company's organisation cuts off whoever leaves it at their
  next sign-in, through an existing link too (if GitHub's organisation list
  cannot be read, nobody passes).
- `missing`: what happens when the claim is not there at all: `keep` (default:
  the person's groups from this provider stay as they are) or `clear` (as if
  the list were empty). OpenID Connect's "groups overage" (Entra ID sends
  `_claim_names` instead of the list when a person is in too many groups) counts
  as missing.
- `map`: up to 1,000 entries, each a value (1 to 256 bytes, compared exactly) and
  a synced group. A value appears once per provider; a group may be fed by
  several values and several providers. Values in the claim with no entry are
  ignored: no group is ever made from a claim.

**Sync.** After every successful sign-in through the provider (`login`, and
`link` and `invite` too, which prove the same identity), with `enabled` on:

1. The values are read (at most 1,000; more are ignored and the
   `groups_claim_truncated` audit row says so) and mapped through `map`.
2. The person's memberships that came from this provider are read (one query)
   and compared: memberships for values no longer sent are removed, new ones
   added. Memberships from other providers and of local groups are untouched.
3. Only when something changed: one write, the person's cached list is dropped,
   and a `groups_synced` audit row is written.

The sync never refuses a sign-in. A person in more than 1,000 groups is not
added to the ones past that number (`group_limit` in the audit row).

**When a value disappears.** The person leaves that group at their next sign-in
through the provider, not before: until then their sessions, tokens and grants
keep the group's permissions. To take access away at once, disable the person,
remove the group from the space, or remove the mapping entry. Removing an entry
from `map` (or pointing it to another group) at once removes the memberships
that entry brought, from everyone; deleting a provider removes every membership
it brought, and the accounts people linked to it. Disabling a provider changes
nothing.

**Refusals of the setting** (`POST`/`PATCH /api/admin/providers`):
`400 invalid` for a bad `claim`, `missing` or value; `404 group_not_found` for an
unknown group; `409 group_not_synced` for a local group; `409 exists` for a
value given twice.

### Limits

| What | Limit |
|---|---|
| groups of a tenant (from 3.3; before, of the server) | 10,000 |
| people in a group | 100,000 |
| groups a person is in | 1,000 |
| groups that are members of one space | 200 |
| mapping entries of one provider | 1,000 |
| claim values read at one sign-in | 1,000, each at most 256 bytes |
| people added in one request | 500 |
| page size of every list | 1 to 200 (default 50); `GET /api/groups` 1 to 20 |

Going over a limit is `409 group_limit`, with the limit in the message. Every
list is paged with `cursor` and `next` and is one query a page.

### Audit

These rows join those of "Audit log":

| `kind` | When | `detail` |
|---|---|---|
| `group_created`, `group_changed`, `group_deleted` | an administrator made, renamed or deleted a group | `group`, `name`, `source`; for a change the changed fields; for a delete `people` and `spaces` |
| `group_member_added`, `group_member_removed` | an administrator changed a local group's people (one row per request) | `group`, `users` (ids, at most 500) |
| `members_batch`, `group_members_batch`, `invites_batch` | a batch of people by file was checked or applied ("Adding people in batches") | `dryRun`, `rows`, `counts`, `results` (one per row) and `space`, `group` or `tenant` |
| `group_self_added` | an administrator added themselves to a group that gave them more in some spaces, or a custom role, with a reason (see "Taking over a space") | `group`, `spaces`, `roles`, `reason` |
| `groups_synced` | a sign-in changed a person's synced groups | `provider`, `added`, `removed` (group ids) |
| `groups_claim_missing` | a provider with groups on sent no claim (or an overage) | `provider`, `missing` (`keep` or `clear`), `reason` (`absent`, `overage`) |
| `groups_claim_truncated` | more values than read, or more groups than a person may be in | `provider`, `limit` (`values`, `group_limit`) |
| `space_group_added`, `space_group_changed`, `space_group_removed` | a group became a member of a space, changed role, or left | `space`, `group`, `role` (old and new for a change) |

`user` is the person a sync or a membership change is about; `actor` the one who
acted (`null` for a sync).

### Group error codes

The codes of this section, besides those of "Conventions":

| Code | Status | When |
|---|---|---|
| `group_not_found` | 404 | the group in the body (or in a mapping entry) does not exist |
| `group_synced` | 409 | changing a synced group's people by hand |
| `group_not_synced` | 409 | a mapping entry points to a local group |
| `group_limit` | 409 | a limit of "Limits" would be passed |
| `group_in_use` | 409 | deleting a group that is a member of spaces or mapped, without `force=1` (with `{"spaces": n, "mappings": n}`) |
| `member_via_group` | 409 | leaving a space one reaches only through groups |
| `user_not_found` | 404 | adding people who do not exist (with `{"missing": [ids]}`; nobody is added) |
| `personal_space` | 409 | a group for a personal space |
| `exists` | 409 | the name is taken; the group is a member already; a mapping value given twice |
| `last_owner` | 409 | unchanged: counts direct owners only |

## Remote repository

Accounts mode; needs `manage` (`403 no_permission` for anyone else who can
open the space). A space either keeps its repository on the server only, or
follows a remote: then the remote branch is the truth.

- The server fetches the remote every minute (`--pull-interval`) and moves
  the space forward to it. Changes made on the remote directly (on GitHub,
  from another clone) show up after that, in pages, lists and search.
- Every change to a file made through this API (creating, saving, committing
  a draft, moving, deleting) is made on top of the remote's current state and
  pushed before the request is answered. If the remote moved in between, the
  change is made again on the new state; a save or commit whose page changed
  on the remote answers `409 conflict` exactly as it does for any other
  change to the file. The server never forces a push and never rewrites or
  deletes anything on the remote.
- When the remote cannot be reached, those requests answer `503 unavailable`
  and change nothing. Drafts are not affected: typing keeps being stored, and
  the server commits a draft once the remote is back.
- If the remote's history was rewritten (the branch no longer contains the
  last commit the server saw), the space is paused: reads keep working from
  what the server has, writes answer `503 unavailable`, until the owner
  accepts the remote's new history.
- An empty folder exists only on the server: Git does not carry it.

Only `https://host/path` and SSH addresses (`git@host:path`,
`ssh://[user@]host[:port]/path`) are accepted, without a password in them, and
only for hosts the administrator allows (`--remote-host`, repeatable; by
default `github.com`, `gitlab.com`, `codeberg.org`, `bitbucket.org`; `*`
allows any).

### Remote

```json
{
  "url": "git@github.com:kazuha/notes.git",
  "branch": "main",
  "provider": "git",
  "credential": {"type": "ssh", "publicKey": "ssh-ed25519 AAAA… petalbook-01JD3K8Q7Z", "fingerprint": "SHA256:…"},
  "state": "ok",
  "lastSync": "2026-11-21T08:30:00Z",
  "error": "",
  "errorCode": "",
  "hostKey": "SHA256:…",
  "rewritten": null
}
```

- `credential.type`: `ssh` (a key pair the server made for this space; the
  private key never leaves the server and is never returned; `publicKey` is
  what to add to the repository as a deploy key with write access), `token`
  (`{"type": "token", "username": "x-access-token"}`; the token is never
  returned) or `connection` (a provider's connection, here the GitHub App; see "Connections"):

  ```json
  {"type": "connection", "provider": "github", "via": "app", "installationId": 51234567, "account": "kazuhahub", "repoId": 881234567}
  {"type": "connection", "provider": "github", "via": "user", "user": {"id": "01JD3K7A2M", "username": "tang"}, "login": "tang-gh", "repoId": 881234567}
  ```

  `via: "app"` fetches and pushes with an installation token of the App's
  installation on `account`; `via: "user"` with the GitHub authorisation of the
  PetalBook user `user` (GitHub account `login`). No secret is stored for the
  space itself and none is returned. `repoId` is GitHub's id of the
  repository. With a GitHub connection, `url` is always `https://github.com/<owner>/<name>.git`.
- `provider`: the id of the provider the remote goes through (`github`), or
  `git` for a plain address with a key or a token; a client shows the
  platform's icon and name from it.
- `state`: `ok`; `offline` (the last attempt could not reach the remote;
  `error` says why, in words fit to show); `rewritten` (paused, see above;
  `rewritten` is then `{"lastGood": "<commit>", "remote": "<commit>"}`);
  `error` (anything else that needs attention: the credential is refused, the
  host key changed, the clone is not in the state the server left it in).
- `errorCode` says, for a program, what `state: "error"` needs; it is `""` in
  every other state. `auth` (the remote refused the credential), `hostkey`
  (the SSH host key changed), `clone` (the clone is not as the server left
  it), `rescued` (commits that could not be pushed were kept aside), and for a
  `connection` credential `connection_expired`, `connection_missing`,
  `installation_gone`, `access_lost`, `app_missing_permission`, `provider_not_configured` (see
  "The GitHub provider", "When a GitHub credential stops working"). The error
  has `provider` too. A client shows `error` and uses `errorCode` only to choose what
  to offer (reconnect GitHub, open the installation's settings, ...). Codes
  may be added; a client treats one it does not know like `auth`.
- `lastSync`: when the server last saw the remote (absent before the first).
- `hostKey`: for SSH, the fingerprint of the host key trusted on first
  connection, to compare with what the host publishes.

Secrets are stored encrypted with a key kept in the data directory
(`secret.key`, or `PETALBOOK_SECRET_KEY`), and are passed to Git through its
environment, never on a command line, in a URL, in Git's configuration or in
a log.

### `GET /api/spaces/{space}/remote`

`200` with the remote, or `404 not_found` when the space is local.

### `POST /api/spaces/{space}/remote/key`

Makes (or returns, if there is one already) the SSH key pair of this space,
so the public key can be added to the repository before connecting. `200`
with `{"publicKey": "…", "fingerprint": "…"}`. With `{"rotate": true}` a new
pair replaces the old one (only while the space is local).

### `PUT /api/spaces/{space}/remote`

```json
{"url": "https://github.com/kazuha/notes.git", "branch": "main",
 "credential": {"type": "token", "username": "x-access-token", "token": "…"}}
```

`branch` defaults to `main`. For `ssh` the credential is `{"type": "ssh"}`
(the key from `…/remote/key`). Leaving `credential.token` out while the space
already has a token keeps it. For a connection it is `{"type": "connection", "provider": "github", "via": "app"}`
or `{"type": "connection", "provider": "github", "via": "user"}` (see "The GitHub
provider", "Switching an existing space to GitHub"). On a space that already follows a remote, `PUT` replaces
its address and credential: this is how a space moves from a deploy key or a
token to the App, or back. The server connects before answering:

- the remote has no such branch (a new, empty repository): the space's
  history is pushed to it;
- the space is pristine (no page and no folder was ever made in it): the
  space takes the remote's content and history;
- one side's history contains the other's: the one behind is moved forward;
- otherwise `409 conflict`: both have content of their own, and the server
  will not merge two histories. Make a new space for that repository.

`200` with the remote. `400 invalid` for an address that is not accepted;
`403 forbidden` for a host that is not allowed; `503 unavailable` when the
remote cannot be reached or refuses the credential (nothing is changed, and
`message` says which).

### `DELETE /api/spaces/{space}/remote`

`204`. The space becomes local, with everything it has; the credential is
deleted. Nothing is changed on the remote. A `connection` credential has no secret
of its own: the App and the user's GitHub authorisation stay as they are.

From milestone 3.1: `409 remote_required` while the storage policy requires a
remote of this space (see "Server storage"), and `409 conflict` while the
space has an attachment repository. A space whose clone is partial or evicted
is made whole first (everything is fetched, as for a pinned space), because a
local space must be the complete copy; the answer waits for it like any
restore, and `503 unavailable` when the remote cannot be reached leaves the
space following it.
`PUT …/remote` with a credential the policy does not accept (anything but
`github` under `require: "github"`) also answers `409 remote_required`.

### `POST /api/spaces/{space}/remote/sync`

Fetches now. `200` with the remote.

### `POST /api/spaces/{space}/remote/accept`

`{"commit": "<the remote commit shown in rewritten.remote>"}`. Accepts a
rewritten history: the space moves to that commit. `200` with the remote;
`409 conflict` if the remote is no longer at that commit. Drafts are kept;
those whose page changed are stale and are shown as such.

## Connections

Milestone 2.7b. A *connection* is a person's account at a platform that holds
repositories, and a *provider* is the platform's side of it. This section is
the general shape; "The GitHub provider" below holds what is GitHub's. Accounts
mode only; single-user mode answers `404`.

**Storage & sync.** The product calls this area "存储与同步" (Storage & sync):

- the administrators' page "存储与同步" holds 3.1's storage rules (top) and the
  connections, one card per platform (bottom); 3.5's budget and pressure join
  the top part;
- the account page's section is "已连接的账号" (connected accounts), one row per
  platform, with the leftover reminders under their platform;
- a new space asks "存在哪里" (where it's stored): this server, sync to GitHub,
  sync to GitLab, …, then "other (by address)";
- ordinary users never see the words Git or repository ("同步到 GitHub"); the
  administrators' page and the "other (by address)" option may.

**One provider interface.** The server holds its providers in memory, by id. A
provider describes itself, connects a person (start, callback, renewal,
disconnect), lists the places it may create repositories in and the
repositories it reaches, creates a repository, gives Git a credential, and
checks its own configuration. GitHub (the App: manifest creation, install and
authorise in one step, installations shared per tenant) is the first
implementation; GitLab and Gitea follow in 3.8; plain Git by address, with a
deploy key or a token, is the provider `git`. Nothing is released, so the
generic shape replaces the GitHub-only one; there are no aliases.

### Provider

```json
{"id": "github", "type": "github", "host": "github.com", "name": "GitHub",
 "owner": "site",
 "capabilities": {"createRepo": true, "installations": true, "connectAccount": true,
                  "storageTarget": true, "partialSync": "yes"},
 "configured": true,
 "app": {"name": "PetalBook Notes", "slug": "petalbook-notes",
         "htmlUrl": "https://github.com/apps/petalbook-notes",
         "installUrl": "https://github.com/apps/petalbook-notes/installations/new"},
 "userAuthorization": true,
 "connection": {"login": "tang-gh", "accountId": "1234567", "state": "ok",
                "connected": "2026-11-23T08:30:00Z", "refreshExpires": "2027-05-23T08:30:00Z", "spaces": 2},
 "leftovers": [{"repoId": 881234567, "fullName": "kazuhahub/tang-notes",
                "htmlUrl": "https://github.com/kazuhahub/tang-notes",
                "spaceName": "Tang notes", "created": "2026-11-23T08:45:00Z"}]}
```

- `id`: the provider's id, used in every path below. `github` is the site's
  GitHub App; a tenant's own GitHub App (3.6) has the id `t-<tenant slug>-github`;
  3.8's are chosen by the administrator, of the site or of a tenant (`gitlab`,
  `gitlab-acme`, `codeberg`, …; lowercase letters, digits and `-`, at most 40,
  unique on the server). `git` is always there and has no connection.
- `type`: `github`, `gitlab`, `gitea` (Forgejo and Codeberg too), `git`.
- `owner`: `site`; from 3.6 also `{"tenant": "<id>"}` for a tenant's own
  provider (see "Tenant-owned resources").
- `capabilities`: what the client may offer; it shows options by capability,
  never by the platform's name. `partialSync` is `yes`, `no` or `unknown` (not
  yet detected; 3.5 detects it).
- `configured`: whether the provider is set up and can be used. Everything else
  of the provider answers `409 provider_not_configured` while it is `false`.
- `app` is the provider's App when it has one (GitHub) and is set up, else
  `null`: `installUrl` is where a person installs it. `userAuthorization` is
  whether people can connect their account: for GitHub the App's client id and
  secret are saved and the server has `--public-url`.
- `connection`: the caller's connection or `null`; `state` is `ok` or
  `expired` (see "Renewal"); `accountId` is the platform's id as a string;
  `refreshExpires` is absent when the provider does not expire tokens;
  `spaces` counts the spaces that sync through it, so a client can warn before
  a disconnect.
- `leftovers`: the caller's reminders of empty repositories a failed attempt to
  make a space left on the platform (see "Leftover repositories"): newest
  first, at most 20, `[]` when there are none and always present.

### `GET /api/account/links`

Any signed-in user: `{"links": [Link], "providers": [Provider]}`.

- `links` is every account the caller has at a provider (consolidation 5c: one
  list for sign-in and repositories, one link per provider and person, one
  person per account at a provider). A link has `id`, `providerId`, `provider`
  (its name), `slug`, `kind`, `label`, `login` (the account's name on the
  platform), `signin` (it signs the person in), `repos` (repositories are
  connected through it, or were and the person must authorise again), `state`
  (`ok` or `expired`), `refreshExpires`, `spaces` (how many spaces sync through
  it), `admin` and `created`. The GitHub link is one row for both: signing in
  with GitHub makes it with `signin` and **no token**; "connect repositories"
  authorises again on the same link and keeps its sign-in. Connecting with
  another account moves the link to that account (refused with
  `connection_error=last_method` when the link is the person's only way to sign
  in, and with `different_account` while spaces sync through the old one).
- `providers`, in a stable order (GitHub, GitLab, Gitea, then by name, `git`
  last). From 3.6 the list is the providers the caller may use: the site's while
  `connections.useSiteApps` is in effect for their tenant, and their tenant's
  own. The providers are the ones the server has, whether set up or not
  (`configured`), so a client can say there is nothing to connect to. Read from
  memory; one query for the caller's connections and leftovers.

`POST /api/account/links/{provider}/start` takes `{"purpose": "signin" | "repos",
"return": "/path", "install": true}` and answers `{"url"}`: `signin` links the
account to sign in with (sign-in kinds), `repos` connects repositories
(GitHub, GitLab, Gitea; `install` is GitHub's). A provider that holds no
repositories answers `400 invalid` to `repos`. An account connected for
repositories only does not sign in (`sso_error=not_for_signin`) until the person
links it for sign-in, which turns it on on the same link.

`DELETE /api/account/links/{id}` removes the link whatever it is for: the tokens
are revoked and the sign-in unlinked, and nothing at all is changed when that
would leave the person no way to sign in (`409 last_method`).
`DELETE /api/account/links/{id}/repos` drops the repository tokens and keeps a
sign-in (a link that only held repositories goes). `{id}` is a link of the
caller's, else `404`. `GET /api/account/links/{provider}/owners|repos` take the
**provider's** id: a person with no link at it still lists what their tenant's
shared installations reach.

`GET /api/admin/providers` (and `POST`, `GET|PUT|PATCH|DELETE .../{id}`,
`.../{id}/check`) is the one administration of every kind of provider
(replacing `/api/admin/connections`): the items are the providers people sign in
with (with their `repo` side, `{host, configured, partialSync, connections,
spaces}`, when they also hold repositories) and the hosts that only hold
repositories (flat, with `signin: false`, `repos`). A GitLab or Gitea host is
made with `POST` (`kind`/`type` `gitlab` or `gitea`, `id`, `host`, `clientId`,
`clientSecret`) and changed with `PATCH` or `PUT`; the GitHub App is saved with
`PUT /api/admin/providers/github` (`appId`, `privateKey`, `clientId`,
`clientSecret`), which makes the GitHub row: a sign-in provider that is off
(`enabled: false`) and holds repositories (`repos: true`), whose client is the
App's (changing it with `PATCH` is refused with `github_app_client`). Its GET
answers the App's settings with the sign-in side as `provider`. The manifest,
its callback (`GET /api/providers/github/manifest/callback`) and the
installations stay under it. Deleting GitHub ends everyone's link (the sign-ins
too): refused while spaces sync through it (`provider_in_use`) and, as for any
sign-in provider, while it is somebody's only way in unless `?force=1`.

### Routes by provider

| Work | Route |
|---|---|
| list the providers | `GET /api/account/links` |
| connect | `POST /api/account/links/{provider}/start` (`purpose: "repos"`) |
| the platform's redirect back | `GET /api/account/links/{provider}/callback` |
| disconnect | `DELETE /api/account/links/{id}/repos` |
| where a repository can be made | `GET /api/account/links/{provider}/owners` |
| repositories that can be connected | `GET /api/account/links/{provider}/repos` |
| dismiss a reminder | `DELETE /api/account/leftovers/{provider}/{repoId}` |
| the provider's configuration | `/api/admin/providers/{provider}…` (site administrators) |

- `GET /api/admin/providers`: site administrators; every provider with its
  configuration state, `partialSync`, and counts (connections, spaces; for
  `git` there are no connections, and `spaces` counts the spaces on a plain
  address). From 3.6 a tenant administrator gets their tenant's own providers
  and, read only, the site's they may use.
- A `{provider}` that does not exist (from 3.6: or that the caller may not
  use) answers `404 not_found`; a route a
  provider's type does not have (installations on a type without them, anything
  but `repos` on `git`) answers `404 not_found` too. `repos` on `git` is an
  empty list.
- The callbacks stay exceptions to the cross-site rule; the state names the
  provider.

### Spaces and remotes

- `POST /api/spaces` takes `storage.sync`:

  ```json
  {"name": "Tang notes",
   "storage": {"sync": {"provider": "github", "owner": "kazuhahub", "name": "tang-notes",
                        "private": true, "description": "Notes on Tang poetry", "create": true}}}
  ```

  Without `sync` the space is on this server. From 3.6 `storage` also takes
  `mode` (`server`, `own`, `tenant`; see "Storage modes"): `sync` and `remote`
  are the `own` mode. The fields are the GitHub
  provider's, see "Creating a space with a repository"; from 3.1 it may also
  have `assets` (see "Spaces", `POST /api/spaces`). `provider: "git"` is
  `400 invalid` here: a plain address is given as `remote` (3.1, with a
  token), or connected after the space is made with `PUT …/remote`. Failures
  that leave a repository behind answer with `repo` (and, from 3.1,
  `assetsRepo`).
- The Remote object's credential is `{"type": "connection", "provider": "github",
  "via": "app" | "user", …}`; the other fields are the provider's (for GitHub
  `installationId`, `account`, `user`, `login`, `repoId`); 3.6 adds `via:
  "tenant"` (a tenant's storage target). `ssh` and `token` stay.
  `PUT …/remote` takes the same shape.
- The Remote object has `provider` (the id; `git` for a plain address) so the
  client shows the platform's icon and name (GitHub's from 2.7b, the others'
  with 3.9).
- 3.1's `remotes.require` takes `null`, `"any"` or a provider's id (see "Server
  storage"); from 3.6 it is derived from `storage.mode` (see "Storage modes").

### Error codes

Every error of a connection has `provider` (the id) next to `error` and
`message`. The codes do not carry the platform's name:

| Code | Meaning |
|---|---|
| `provider_not_configured` | the provider is not set up (or, for user flows, lacks what connecting needs) |
| `connection_expired` | the person's connection ran out or was revoked (as a remote's `errorCode`) |
| `connection_missing` | the person it names has no connection (any more) (as a remote's `errorCode`) |
| `not_connected` | the request needs the caller's connection and there is none, or it is expired |
| `installation_gone` | the App was uninstalled or suspended, or the repository left its selection |
| `access_lost` | the installation is no longer connected to the space's owner here (disconnected), or the credential is not the owner's own (a person's token for an organisation's space), or the person whose own tokens it synced with (`via: "user"`) no longer holds any at that provider (disconnected, unlinked, expired, deleted) |
| `repo_not_in_installation` | no installation reaches the repository and it could not be added |
| `app_not_installed` | the App is not installed where the caller wants a repository (`installUrl`): their own account, or an organisation the App was removed from |
| `app_missing_permission` | the App or its installation lacks a permission (`permission` names the first, `missing` all) |
| `provider_refused` | the platform refused a person (`message` passes on what it said) |
| `provider_error` | the platform failed or answered something the server could not use |
| `name_taken` | the owner has a repository of that name |
| `push_failed` | the repository was made but the first push failed |
| `app_rejected`, `client_rejected`, `key_invalid` | the platform does not accept what the administrator entered |
| `public_url_needed` | the flow needs `--public-url` |
| `owner_not_allowed` | the owner is not one of the caller's |
| `provider_in_use` | the provider cannot be changed or removed while spaces or people use it |
| `provider_host_mismatch` | the GitHub App is on another host than the GitHub sign-in provider people linked their accounts on |
| `account_in_use` | the platform account is connected to another person (any tenant) |
| `not_installer` | (a callback's `connection_error`, or a pending claim's `error`) the claimer of an organisation's installation is not the GitHub user who installed it, as GitHub's signed webhook said, or no webhook said who did |

### Storage

`remotes.cred_type` is `ssh`, `token` or `connection`, and `remotes.provider` is
the provider's id (`github` for a connection, `git` for the others). GitHub is
one row of `providers` (id `github`, `signin` and `repos`; the App's record is
`config.app`, its client id `config.clientId`, its key, client secret and webhook
secret are in `secrets`), a person's account at it one row of `linked_accounts`
(tokens on the same row as the sign-in), its installations rows of
`provider_installations`, each with its one owner here (`owner_user` or
`owner_tenant`), who installed each one (from the webhook) a row of
`github_installers`, an organisation's claim that waits for the webhook a row
of `github_pending_claims` (expires after 10 minutes), and the reminders of empty repositories rows of
`repo_leftovers`.

### Audit

`connection_provider_changed` (a provider configured, changed or removed:
`provider`, `type`, secrets as `"changed"`), `account_connected` and
`account_disconnected` (`provider`, `user`, `login`),
`github_installation_connected` (`installation`, `account`; `via: "webhook"`
when GitHub's event completed a pending claim, the claimer the actor) and
`github_installation_disconnected` (`installation`, `spaces`: how many lost
access; `by: "github"`, with no actor, when the App was uninstalled on GitHub),
the last two in the owner's tenant (a person's, or the
organisation's). `connection_provider_changed` is the server's own kind: a
tenant administrator's `GET /api/admin/audit` does not list it.

## The GitHub provider

Accounts mode; single-user mode answers `404` here, as it does for the whole of
"Remote repository" (so a `connection` credential in `PUT …/remote` is `404` there
too). The server has at most one
GitHub App, which an administrator configures (see "Administration", "Connections", "The GitHub App"); only `github.com` is supported. Three things come from it:

- **Installations**: where the App is installed. Anyone installs it on their own
  GitHub account, and an organisation's administrators on the organisation's
  GitHub organisation ("Installations" below); each installation belongs to that
  one owner here. An installation token pushes and fetches a space's repository,
  and creates repositories in an organisation's own installation.
- **GitHub authorisation**: a PetalBook user connects their GitHub account to
  the App. The user token creates repositories in the user's own account and
  in the organisations GitHub lets that user create them in, and syncs a space
  when no installation reaches its repository. What a user token may do is
  what both the user and the App may do: GitHub checks the user's own rights.
- **Spaces made with a new repository** (`POST /api/spaces` with `storage.sync`),
  and existing spaces switched to the App (`PUT …/remote`).

**Whose spaces sync through what (owner decision 2026-11-26).** The App is the
server's: only site administrators reach `/api/admin/providers/github…` (a
tenant administrator gets `403 forbidden`, as for the other site-only routes).
Its installations are not: each has one owner here.

- A **person's space** (a space of the default tenant, or someone's personal
  space) syncs only through that person's own GitHub link (`via: "user"`) or an
  installation connected to that person (`via: "app"`, the remote naming the
  person, who must still be able to manage the space).
- An **organisation's space** (a space of any other tenant) syncs only through
  an installation connected to that tenant (`via: "app"`), never through a
  member's own token: `via: "user"` is `403 owner_not_allowed`, and a remote
  that has one anyway fails with `access_lost`.
- One GitHub account links to one PetalBook user on the whole server, whatever
  their tenants; `account_in_use` says only that it is taken. A space made with
  a repository is the creator's tenant's (the default tenant: the creator's own)
  and counts toward its quotas.
- `features.githubConnect` (site, with a rule per tenant; on by default) says
  whether people and organisations may install the App: off, the install routes
  answer `403 feature_off` and the app offers no button; what is connected stays
  listed and can be disconnected.

Every route here is a person's, not a token's: with an API token, `/api/connections/github/…`
and `/api/admin/providers/github…` answer `403 token_scope` (see "API tokens"), and so
do `POST /api/spaces` and `PUT …/remote`.

| Work | Token |
|---|---|
| Fetching and pushing a space with `via: "app"` | installation token, narrowed to the one repository, `contents: write` |
| Fetching and pushing a space with `via: "user"` | the user token of the PetalBook user the remote names |
| Creating a repository in the user's account | user token (`POST /user/repos`) |
| Creating a repository in an organisation the user's authorisation lists (a person's space) | user token (`POST /orgs/{org}/repos`) |
| Creating a repository in the organisation's own installation (an organisation's space) | installation token, `administration: write` (`POST /orgs/{org}/repos`) |
| Checking a claimed installation on the setup callback | the claimer's user token (`GET /user/installations`) |
| Listing the caller's owners and repositories | user token (a person); the organisation's installations from the database and their installation tokens (an organisation's member) |

### Installations

Anyone may install the server's App on their own GitHub account, and an
organisation's administrators (key `tenant.github.manage`, held by every tenant
administrator) on the organisation's GitHub organisation. The installation is
then connected to that one owner: the person, or the tenant (not the
administrator who installed it; any of its administrators manages it).

- `POST /api/account/github/install` `{return?}` (the person's own account) and
  `POST /api/admin/github/install` `{account, return?}` (the acting tenant;
  `account` is the GitHub organisation's login): `200 {"url": …}`, the App's
  installation page with a `state`, and a cookie that binds the attempt to the
  browser (10 minutes). Both need `features.githubConnect`
  (`403 feature_off`); the default tenant has no organisation installations
  (`403 forbidden`). `POST /api/account/links/github/start` with
  `install: true` is the person's install too.
- The callback is `GET /api/account/links/github/callback` (the App's callback
  URL; GitHub sends `installation_id` and `setup_action`, and a `code` when the
  App asks for authorisation during installation). The installation is
  connected only when every check holds, in this order:
  1. the attempt is this browser's and this person's (`expired` otherwise);
  2. the person may still connect one for that owner (the feature, and for an
     organisation the key there): `forbidden`;
  3. GitHub lists the installation for the person's **own** user token
     (`GET /user/installations`; the token of the `code`, or the person's link
     when there is none: `not_connected` without either). The
     `installation_id` of the query string alone is never trusted:
     `installation_not_yours`;
  4. it is on the account asked for: the person's own GitHub account (type
     `User`, the id of the token's user) or the organisation named at the start
     (type `Organization`): `wrong_account`; for an organisation, the claimer
     must also be the GitHub user who installed it: the id of their token's
     user equals the `sender` of GitHub's signed `installation` event
     (`created`; see "Webhook" below). GitHub lists an organisation's
     installation for any member who reaches one of its repositories, but only
     the organisation's owners install an App on it (a member's request needs
     an owner's approval, and the approver is then the sender), so this proves
     an owner's act without the organisation's member list. Anyone else gets
     `not_installer` ("the person who installed the App on GitHub connects
     it"), and so does a claim of an installation no event said the installer
     of. GitHub's redirect may come before its event: when no event has come
     and GitHub says the installation was just made (`setup_action=install`),
     the claim **waits** (a pending claim: the attempt, the claimer, their
     GitHub id and the owner, kept 10 minutes like an attempt) and the browser
     goes to `return?connection_pending=<account>`; the event completes it
     when it comes, if its sender is the claimer (else the pending claim
     shows `not_installer`) and the claimer may still claim for that owner,
     asked again then: an active account, GitHub connections on for the
     owner's tenant (the site's switch and the tenant's rule) and, for an
     organisation, `tenant.github.manage` in that tenant (else it shows
     `forbidden`). The
     server never polls GitHub for it. Any other
     `setup_action` with no event is `not_installer` at once (or
     `installation_taken` when another owner here has it). An organisation's
     claim needs the App to ask for Repository administration (only then do
     an organisation's owners alone install it there; GitHub lets a member
     install an App that does not): without it, on the callback and when the
     event completes a claim, `app_missing_permission`;
  5. no other person or organisation on this server has it (one statement; of
     two at once one wins): `installation_taken`, "already connected to another
     organisation or person on this server".
  A person's install also stores their link, as connecting does (its own
  refusals apply); an organisation's stores nothing of the administrator's. The
  browser is sent back to `return` (failures: the person's to
  `/settings?provider=github&connection_error=<code>`, an organisation's to
  `return?connection_error=<code>`). `setup_action=request` (an organisation's
  owners must approve first) is `requested`.
- `GET …/installations` (`/api/account/github` or `/api/admin/github`):
  `{"items": [{"id", "account", "accountType", "owner": {"kind": "person" |
  "organization", "id"}, "claimedBy", "connected", "spaces", "manageUrl"}],
  "pending": [{"id", "account", "claimedBy", "started", "expires", "error"}]}`,
  from the database (at most 100 items and 20 pending). `pending` are the
  owner's claims waiting for GitHub's event (`error: ""`), or that failed when
  it came (`not_installer`, `forbidden`, `installation_taken`), until they
  expire. A client shows them as "connecting" and polls nothing: it reads the
  list again a few times right after GitHub sends the browser back (at 1, 2
  and 4 seconds), when the window gets the focus, and on "Refresh". `manageUrl` is the installation's settings
  on GitHub, where its repositories are chosen and where it is uninstalled.
- `GET …/installations/{id}/repos[?cursor=]`: one page of the repositories it
  reaches, with its own token. `GET …/installations/{id}/spaces`:
  `{"items": [{"id", "name"}], "total"}`, the spaces that sync through it (at
  most 200 named): what disconnecting it affects, for the client to list before
  it asks to confirm.
- `DELETE …/installations/{id}`: `200 {"spaces": n}`. The installation is no
  longer connected here (it stays installed on GitHub). Every remote that synced
  through it keeps its row and the space its notes; they show `state: "error"`
  and `errorCode: "access_lost"` at once, and a later sync fails the same way.
  Under a storage requirement of a remote, such a space counts as having none
  and is in the requirement's grace like any space without one (`InGrace`, then
  `Overdue`; exemptions of spaces and people as always). The grace starts again
  from the disconnect: its deadline is the disconnect plus `graceDays` (the
  space's `remote.deadline` shows it), even for a space that was past its
  deadline before, so that it has time to get a new repository; the later of
  its creation, the requirement's `since` and the disconnect counts. The same
  holds when GitHub reports the installation uninstalled (the webhook's
  `deleted`) and when a person disconnects their own installation. A space
  that is connected again and disconnected again without a successful sync in
  between keeps its first loss time (no endless grace by toggling); a loss after
  a successful sync starts a grace of its own. A loss starts a grace only
  when the requirement accepts the lost remote (`require` is `any` or the
  remote's provider): under `require: "gitlab"`, losing a GitHub remote
  leaves a space that was overdue overdue, so a remote that does not count
  cannot be used to restart the grace. Connecting the same
  installation again by the same owner restores the spaces; for an organisation that needs the App uninstalled on GitHub
  and installed again from here, since who installed it proved one claim (the
  record is used up when the claim succeeds). The spaces listed, counted and marked are the owner's only: a
  space of an earlier owner that still names the installation is not.
- **A person's own tokens** (`via: "user"`, at GitHub, GitLab or Gitea) are the
  same: a remote that syncs with them is usable only while that person holds
  tokens at its provider. When they go, by the person disconnecting
  (`DELETE /api/account/links/{id}/repos`, or unlinking the account), an
  administrator unlinking it (`DELETE /api/users/{id}/links/{link}`), the
  authorisation expiring or being revoked, the provider's application being
  replaced, or the person being deleted, then, in the same transaction, every
  remote of theirs that synced with them at that provider shows
  `state: "error"` and `errorCode: "access_lost"` (the rows and the notes stay),
  and under a storage requirement the space counts as having no remote and is
  in the grace again from that moment, by the rule above (only for a remote the
  requirement accepts; never cleared when they connect again; kept when there
  was no successful sync since the last loss). Spaces that sync through an installation, and other people's remotes,
  are not affected. The person connecting again restores the remotes with the
  next sync (open spaces fetch at once).
- Another owner's installation is as good as not there: `404 not_found`.

### Webhook

`POST /api/github/webhook` is where GitHub sends the App's events (the App's
webhook URL, `webhookUrl` in `GET /api/admin/providers/github`; the manifest
turns it on). It is public (no session, no token) and answers GitHub only:

- The body is capped at 2 MiB (`413 too_large`) and read whole; its
  `X-Hub-Signature-256` (`sha256=` and the hex HMAC-SHA256 of the body with the
  App's webhook secret) is compared in constant time **before anything of the
  body is parsed**: a missing or wrong signature is `401 unauthenticated` and
  records nothing. No App or no webhook secret kept: `409
  provider_not_configured`. While the site's own `features.githubConnect` is
  off, a new installation's event (`created`, and so the claims it would
  complete) is refused with `403 feature_off`; an uninstall (`deleted`) is
  always taken: recording one is always safe.
- `X-GitHub-Event: installation`, `action: created`: who installed it
  (`installation.id`, `installation.account`, `sender`) is recorded, the same
  again when GitHub redelivers, and the pending claims of that installation
  are completed (each by one request: the event, or a callback that saw it;
  a second callback of the same person does not reset a claim being
  completed). The record proves one claim: it is removed when a claim
  succeeds, and an event for an installation connected here records nothing.
  `action: deleted`: the installation is disconnected from whoever has it here,
  as `DELETE …/installations/{id}` does (its spaces show `access_lost`), and
  who installed it, and the claims waiting for it, are forgotten.
- Any other event or action: `204`, nothing done. Every handled one: `204`.
  A delivery is a few statements on the state database; GitHub is not asked
  anything.

### Tokens, and how they reach Git

- **App JWT**: RS256, signed with the App's private key, `iat` a minute in the
  past, `exp` nine minutes ahead. Made in memory, reused for eight minutes.
- **Installation tokens** are made with
  `POST /app/installations/{id}/access_tokens`, for a space's repository
  narrowed with `repository_ids: [repoId]` and
  `permissions: {"contents": "write"}`; for creating a repository, with
  `permissions: {"administration": "write"}` and no repository (one that does
  not exist yet cannot be named), only for that request. They live in memory
  only: never in the state database, an answer, an error message or a log.
  A token is reused until 5 minutes before its `expires_at` (GitHub gives an
  hour). Requests that need the same token at once share one call to GitHub,
  and no lock is held while the network is used. The cache holds at most
  10 000 tokens, drops expired ones, and when full drops the one that expires
  first. Which installation reaches a repository is remembered for 10 minutes;
  a `404` when minting forgets it.
- **User tokens** are stored sealed (below); the decrypted access token is
  kept in memory until 5 minutes before it expires.
- **To Git**: exactly as a `token` credential today. Git reads the user name
  (`x-access-token`) and the token from its environment through a credential
  helper scoped to `https://github.com`, redirects are followed only for the
  first request, and the token is scrubbed from Git's output. It is never on a
  command line, in the remote address, in `.git/config` or in a log. Each Git
  command takes the token from the cache, so a pull every minute costs no call
  to GitHub until the token is near its end.
- **Cost.** Only open spaces are pulled (see "Remote repository"), so a space
  costs at most one minted token per 55 minutes while someone uses it, and none
  while it is closed.

### The provider object

`GET /api/account/links` (see "Connections") shows GitHub as the provider
`github`. `app` is `null` when no App is configured; then nothing else of the
provider works and its other endpoints answer `409 provider_not_configured`.
`userAuthorization` is whether users can connect their GitHub account: the
App's client id and secret are saved and the server has `--public-url`.
`connection.state` is `ok` or `expired` (see "Renewal"). `connection.spaces`
counts the spaces that sync through it (`via: "user"` naming the caller), so a
client can warn before a disconnect. `leftovers` are described under "Leftover
repositories".

### `POST /api/account/links/github/start`

`{"return": "/s/01JD3K8Q7Z/settings", "install": true}`, with `return` treated as
in `POST /api/auth/{kind}/{slug}/start`. `200` with `{"url": "…"}` and a short-lived
cookie that ties the attempt to this browser; the client navigates to `url`.

- **`install: true`** (what "Connect GitHub" uses): `url` is the App's
  installation page,
  `https://github.com/apps/<slug>/installations/new?state=…`. The App asks for
  user authorisation during installation (`request_oauth_on_install`), so a
  person installs the App on their own account (or an organisation) and
  authorises it in one GitHub flow, and GitHub sends the browser to the callback
  below with `code`, `state`, `installation_id` and `setup_action=install`. An
  App entered by hand needs "Request user authorization (OAuth) during
  installation" ticked in its settings (the one made by the manifest button has
  it). The code of such a flow has no PKCE challenge, so none is sent back.
  A person who already has the App installed everywhere can still authorise
  alone with:
- **`install` false or left out**: `url` is
  `https://github.com/login/oauth/authorize?client_id=…&redirect_uri=…&state=…&code_challenge=…&code_challenge_method=S256`.

`redirect_uri` is `redirect_uri` is
`<public-url>/api/account/links/github/callback`. `state` is 32 random
bytes; the PKCE verifier is 64 random characters and its S256 challenge is
sent (not for `install`). Both are held in memory only, for 10 minutes, used once, bound to the
cookie and to the signed-in user (at most 10 000 attempts at a time: the
oldest goes first). When reconnecting, `login=<the current login>` is sent so
GitHub suggests the same account. `409 provider_not_configured` when
`userAuthorization` is false.

### `GET /api/account/links/github/callback`

GitHub redirects here. The server answers with a redirect (`303`), never JSON:
to the `return` path on success, to `/settings?connection_error=<code>&provider=github` on failure.
It needs the session of the user who started the attempt; another session or
none is `expired`. The same callback serves both starts: a plain authorisation
(`code` and `state`) and an installation (`code`, `state`, `installation_id`,
`setup_action=install`). The server exchanges the code
(`POST https://github.com/login/oauth/access_token` with the client id and
secret, the code, `redirect_uri` and, for a plain authorisation, the PKCE
verifier), then reads the account with `GET /user`. An installation that
brought no `code` (the App does not ask for authorisation during installation)
is `installed`; one an organisation still has to approve
(`setup_action=request`) is `requested`.

Codes: `installed` (the App was installed but GitHub sent no code: authorise
once more with `install` false; the App needs "Request user authorization
(OAuth) during installation"), `requested` (the organisation's owners must
approve the installation first), `denied` (the user cancelled on GitHub), `expired` (the attempt is
unknown, used, older than 10 minutes, or of another user), `provider_error`
(GitHub refused the exchange or could not be reached), `account_in_use`
(another PetalBook user has connected this GitHub account, in any tenant: the
GitHub account id is unique on the server; the answer names neither the user
nor the tenant), `different_account`
(the caller has spaces syncing through another GitHub account: disconnect
first, or switch those spaces).

A user has at most one connection. It is stored in the state database: the
GitHub account's id and login, the access token and the refresh token
(each sealed with the server's key, bound to the user and to its purpose),
their expiry times and the `state`. Connecting the same account again replaces
the tokens; spaces that were waiting on it are retried at once.

### `DELETE /api/account/links/{id}/repos`

`204`. The server asks GitHub to revoke the grant
(`DELETE /applications/{client_id}/grant`, best effort) and deletes the tokens
from the database and from memory whatever GitHub answers. Spaces that sync
through it go to `state: "error"`, `errorCode: "connection_missing"`
the next time they talk to GitHub; nothing in them is deleted (see "When a
GitHub credential stops working").

### Renewal

GitHub's user tokens last 8 hours and their refresh tokens 6 months, and every
renewal hands out a new refresh token, valid 6 months again, while the old one
stops working.

- The access token is renewed when it is needed and has less than 5 minutes
  left (`POST https://github.com/login/oauth/access_token` with
  `grant_type=refresh_token`). One renewal per user at a time; others wait for
  it. The new pair is written to the database, replacing the old one only if
  it is still the one renewed from, before the new access token is used. A
  renewal that cannot be stored is retried; the old refresh token is no longer
  valid at that point, so failing to store it means the connection is
  `expired`.
- Once a day the server renews, one at a time, the connections whose refresh
  token ends within 30 days, so someone whose spaces sit closed for months
  stays connected. That is one request per user every five months or so.
- A renewal GitHub refuses (`bad_refresh_token`: expired or revoked) makes the
  connection `expired` and drops its tokens. A renewal that cannot reach GitHub
  changes nothing: the space is `offline`, as for any unreachable remote.
- An App that does not expire user tokens gives no refresh token; the token is
  then kept until it is revoked, and none of the above applies.
- The device flow is not offered: the web app does not need it. It is left for
  the clients of phase 4.

### `GET /api/account/links/github/owners?cursor={cursor}&recheck=true`

The accounts the caller can create a repository in, one page at a time:

```json
{"items": [
  {"login": "tang-gh", "type": "User", "via": "user", "installationId": null, "needsInstall": false},
  {"login": "kazuhahub", "type": "Organization", "via": "user", "installationId": 51234567, "needsInstall": false},
  {"login": "petal-team", "type": "Organization", "via": "app", "installationId": 51239999, "needsInstall": false}],
 "next": "…"}
```

For a person (whose new spaces are their own): the caller's own GitHub account,
when connected, then the organisations among the installations GitHub lists for
the caller's connection (`GET /user/installations`, 30 to a page), all
`via: "user"`. For a member of an organisation (whose new spaces are the
organisation's): the organisation's own installations on this server only, from
the database, all `via: "app"`; never the member's own account. `needsInstall` is `true` on the caller's own account when the
App is not installed there: GitHub lets a user token create repositories only
in accounts the App is installed on, so a repository in that account is refused
with `409 app_not_installed` until the person installs it (the web app
offers "Install on my account": `installUrl` of `GET /api/account/links`, or the
`install` start above). It is found with one call to GitHub on the first page
(`GET /users/{login}/installation`, remembered while the App is there; the answer "not installed there" is remembered for a minute per person, so a person without the App does not cost a call or a log line at every visit). `recheck=true`, on the first page, forgets that memory for the caller first: a person who has just installed the App asks again instead of waiting the minute (the web app's "check again"). It is honoured at most once a minute per person: within the minute `recheck=true` is an ordinary call that uses the remembered answer, with no call to GitHub and no error, so looping it cannot use up the App's shared rate limit. Creating a space is never refused by the client on this flag; the server answers `409 app_not_installed`.
`via` says which token creates the repository there. Each page is at most one call
to GitHub. `cursor` is opaque; `next` is `""` after the last page. A person
without a connection has none. A person's repository is created with their own
token, so GitHub's rules for that person in that organisation (who may create
repositories, whether private ones are allowed) apply; an organisation's with
its installation's token.

### `GET /api/account/links/github/repos?installation={id}&cursor={cursor}` or `?mine=true`

Repositories the caller can connect a space to, 30 to a page, for choosing one
in `PUT …/remote` or `POST /api/spaces` with `create: false`. `installation`:
all the repositories of an installation of the space owner's own (the person's,
or for a member the organisation's; installation token), or for a person those of
another installation their connection reaches
(`GET /user/installations/{id}/repositories`). `mine=true`: the caller's own
repositories (`GET /user/repos?affiliation=owner`); a member of an organisation
gets `403 owner_not_allowed`.

```json
{"items": [{"id": 881234567, "fullName": "kazuhahub/tang-notes", "private": true,
            "defaultBranch": "main", "htmlUrl": "https://github.com/kazuhahub/tang-notes", "canPush": true}],
 "next": "…"}
```

`404 not_found` for an installation the caller can neither reach nor has. GitHub cannot filter these lists by name, so there is no `q`: a client
filters the page it has, and the user can type `owner/name` instead.

### Creating a space with a repository

`POST /api/spaces` with `storage.sync` and `provider: "github"` (see "Connections", "Spaces and remotes"):

- `owner`: a `login` from `GET /api/account/links/github/owners` (case is ignored).
  `403 owner_not_allowed` for anything else.
- `name`: 1–100 characters of `A-Z a-z 0-9 . _ -`, not `.` or `..`, not
  ending in `.git`. Anything else is `400 invalid`: the server does not let
  GitHub rename it.
- `private`: `true` when left out.
- `description`: optional, at most 350 characters, no control characters.
- `create`: `true` when left out. `false` uses the repository `owner/name`
  that exists already, under the rules of `PUT …/remote`: an empty repository
  gets the space's history; one with content gives it to the new space, which
  is pristine. The credential is chosen as `PUT …/remote` would with no `via`
  given: the App when the installation on that repository is the space owner's
  own (the person's, or the organisation's), otherwise, for a person's space
  only, the caller's own connection.

The repository is created empty (`auto_init: false`), so the space's first
commit is the repository's first commit, on `main`. The order:

1. Checks that need no network: the fields, that `github.com` is among the
   hosts the administrator allows (`--remote-host`; otherwise `403 forbidden`,
   as for any other remote on a host that is not allowed), the spaces quota
   (the person's and the tenant's, as for any `POST /api/spaces`:
   `403 quota_exceeded`; spaces still being made with a repository count as
   made, so parallel requests cannot pass it), the
   owner, and that the tokens it needs exist (App configured, caller connected and not
   `expired` when `via` is `user`).
2. The space is made on disk (`git init`, `space.yml`, the first commit). No
   one can see it and there is no record of it yet.
3. The repository is created on GitHub (`POST /orgs/{org}/repos` or
   `POST /user/repos`), with the token `via` names.
4. The space's credential is chosen and checked. For an organisation:
   `via: "app"`, with a token minted for the new repository alone. For the
   caller's own account: `via: "app"` when the App is installed on that
   account and reaches the repository, otherwise `via: "user"`. When an
   installation with "only selected repositories" does not reach the new
   repository, and the caller's connection can see that installation, the
   server adds it (`PUT /user/installations/{id}/repositories/{repoId}`, user
   token) and mints again; when it cannot, the caller's own account falls back
   to `via: "user"` and an organisation fails with
   `409 repo_not_in_installation`. This works whether or not GitHub adds
   a repository an App created to its installation by itself.
5. `main` is pushed. A repository GitHub has just made is sometimes not found
   for a few seconds, so "not found", GitHub's `5xx` and an unreachable GitHub
   are tried again after 1, 3 and 7 seconds (up to 4 tries). One deadline of
   15 seconds covers the tries, the waits and each push itself: a push still
   running then is stopped, and no try starts that could not finish before it.
   GitHub also says "Repository not found" for a credential it does not accept
   on a private repository, which looks the same; the deadline is what bounds
   that case. Every other refusal of the credential (`401`, `403`,
   "Authentication failed") and a credential the server could not get at all
   are not retried.
6. In one transaction: the space (in the caller's tenant), the caller as its
   owner, and its remote (`connection`, provider `github`, `via`, the installation, the repository id,
   the user); the usage rows count it as any new space. Then the space becomes
   visible.

When a step fails:

- Steps 1 and 2: nothing was made anywhere.
- Step 3: the space on disk is removed. `409 name_taken` when the
  owner already has a repository of that name (a client can offer
  `create: false` when it is the caller's); `403 app_missing_permission`
  when the App or its installation lacks the Administration permission
  (`missing` names it); `403 provider_refused` when GitHub refuses a person
  (the organisation does not let this person create repositories, or does not
  allow private ones: `message` passes on what GitHub said); `400 invalid` for any other refusal
  of the fields; `503 unavailable` when GitHub cannot be reached;
  `429 rate_limited` when GitHub limits the server.
- Steps 4 to 6: the space on disk is removed. The repository stays on GitHub,
  empty: the server never deletes anything on a remote, and may not even be
  allowed to. The answer names it, so the same request with `create: false`
  connects a new space to it once the cause is fixed, and the server keeps a
  reminder of it for the caller (see "Leftover repositories"):

  ```json
  {"error": "push_failed", "message": "…",
   "repo": {"id": 881234567, "fullName": "kazuhahub/tang-notes", "htmlUrl": "https://github.com/kazuhahub/tang-notes"}}
  ```

  Codes: `409 repo_not_in_installation`, `502 push_failed`,
  `500 internal` (the transaction failed).
- A crash between steps 3 and 6 is the same failure without an answer: the
  directory on disk has no record and no one sees it (the next start of the
  server removes it once it is a quarter of an hour old; `petalbook user …`
  does not), and the empty repository is on
  GitHub, ready for `create: false`.

Two requests for the same name: the second gets `409 name_taken`. The
server finishes the work when the client goes away, as `PUT …/remote` does.

#### Leftover repositories

The repository a failed attempt leaves is kept (the server never deletes on a
remote). So that the person does not have to remember it, the server also
records a reminder, per user, when the failure came after the repository was
made (steps 4 to 6, the same cases that put `repo` in the answer; a crash has
no answer and no reminder). A client shows the reminders in
`GET /api/account/links`'s `leftovers`, for instance on the account page or beside the
list of spaces, with the repository's name and link and two actions:

- **Connect it and try again**: `POST /api/spaces` with the reminder's
  `spaceName` as `name` and `storage: {"sync": {"provider": "github", "owner", "name", "create": false}}` taken
  from `fullName`. Nothing else is needed; the reminder is cleared by the
  space connecting (below). When it fails again the reminder stays, and the
  answer says why.
- **I deleted it**: `DELETE /api/account/leftovers/github/{repoId}`.

A reminder is cleared when the person dismisses it, and by itself when any
space is connected to that repository with a `connection` credential (by
`create: false`, or by `PUT …/remote`), whoever made the reminder. A space
connected with a plain URL and a token or key does not clear it. Only the newest 20 per user are kept; a
second failure on the same repository replaces its reminder rather than adding
one. Reminders are the user's own: they are deleted with the user and no one
else sees them, in any tenant.

### `DELETE /api/account/leftovers/github/{repoId}`

`204`. Dismisses the caller's reminder of that repository (the id from
`leftovers[].repoId`). Nothing happens on GitHub. It is `204` also when there
is no such reminder (it was cleared meanwhile), as for `DELETE
/api/account/links/{id}/repos`. `repoId` that is not a positive number is
`404 not_found`.

### Switching an existing space to GitHub

`PUT /api/spaces/{space}/remote` (needs `manage`):

```json
{"url": "https://github.com/kazuhahub/notes.git", "credential": {"type": "connection", "provider": "github", "via": "app"}}
```

`url` must be a repository on `github.com`; `git@github.com:owner/name.git` is
accepted and stored as `https://github.com/owner/name.git`. A space that
already follows that repository with a deploy key or a token keeps its history
(the histories are the same) and only its credential changes; the old key or
token is deleted.

- `via: "app"`: the installation that reaches the repository
  (`GET /repos/{owner}/{name}/installation`). It must be connected to the
  space's owner here: for an organisation's space the organisation's, for a
  person's space the caller's own (the remote then names the caller, who must
  keep `manage`). Otherwise anyone with `manage` in a space could tie it to any
  repository the App reaches.
- `via: "user"` (a person's space only; an organisation's is `403
  owner_not_allowed`): the caller's connection, which must be able to push. The
  space then syncs as the caller on GitHub. Someone else with `manage` who
  `PUT`s it again ties it to themself. It keeps working only while that user
  can sign in, still has `manage` in the space (directly or through a group:
  the union of their roles counts), and is connected. The caller must be in the
  space's tenant (which only a site administrator, who may open a space of any
  tenant, can fail): otherwise `403 forbidden`, before GitHub is asked.

A GitHub connection does not get around `--remote-host`: when `github.com`
is not an allowed host the answer is `403 forbidden`, as for any other remote
on a host that is not allowed, and GitHub is not asked.

Errors, besides those of `PUT …/remote`: `409 provider_not_configured`;
`409 not_connected` (the caller has no connection, or it is
`expired`); `409 repo_not_in_installation` (`via: "app"` and no
installation reaches the repository); `403 owner_not_allowed` (the
installation is not the space owner's own, `via: "user"` for an organisation's
space, or the caller's connection cannot push there);
`403 forbidden` (`via: "user"` and the caller is not in the space's tenant).

### When a GitHub credential stops working

The space goes to `state: "error"` with one of these `errorCode`s, found when
the server next mints or renews a token, or when Git is refused:

| `errorCode` | Cause | Fix |
|---|---|---|
| `connection_expired` | the refresh token expired or was revoked | the user the remote names connects again |
| `connection_missing` | that user disconnected, was disabled or deleted, or lost `manage` in the space (directly and through every group) | they connect again, or someone with `manage` switches the credential |
| `installation_gone` | the App was uninstalled or suspended there, or the repository was taken out of its selection | install it again, or add the repository |
| `access_lost` | the installation it synced through was disconnected from its owner here, or is not the space owner's own; or the person whose own tokens it synced with no longer has any (it is shown at once when they go; the next sync then reports `connection_missing` or `connection_expired`) | connect GitHub again (the same installation, or the same person connecting again, restores it), or connect another repository |
| `app_missing_permission` | GitHub refuses the installation's token because the App or its installation lacks Contents | an administrator gives the App Contents (write) and accepts the change in the installation's settings; then re-check in the administration |
| `provider_not_configured` | the server's App is not configured, or its key no longer works | an administrator fixes the App |

In every case nothing is lost: reads go on from what the server has; writes
answer `503 unavailable` and change nothing, as for an unreachable remote;
drafts keep being saved and are committed and pushed once the credential
works again. While a space is in one of these states it is retried every 10
minutes instead of every `--pull-interval`, and at once when its cause is
fixed (a user connects again, an administrator saves the App).
`POST …/remote/sync` retries now. The first sync that works clears the state.

### Errors of this section

| Code | Status | When |
|---|---|---|
| `provider_not_configured` | 409 | no App, or (for user flows) no client id and secret or no `--public-url` |
| `not_connected` | 409 | the request needs the caller's GitHub connection and there is none, or it is `expired` |
| `owner_not_allowed` | 403 | the owner is not one of the caller's owners; the installation that reaches the repository is not the space owner's own; an organisation's space through a person's own connection |
| `installation_taken` | — | (a callback's `connection_error`) the installation is connected to another person or organisation on this server |
| `name_taken` | 409 | the owner has a repository of that name |
| `provider_refused` | 403 | GitHub refused to create the repository (`message` says what GitHub said) |
| `app_missing_permission` | 403 | the App, or its installation, lacks a permission the call needs (a token GitHub refused with a 403 or 422 that talks about permissions); `missing` lists the names (`administration`, `contents`), `repo` is set when the server had made a repository. It is not the same as the next one: the permission is the problem, not the repository. As the `errorCode` of a space's remote it says syncing needs a permission |
| `app_not_installed` | 409 | the owner is the caller's own account and the App is not installed there; `installUrl` is where to install it (instead of GitHub's raw `403 Resource not accessible by integration`). Installed, but without Administration, it is `app_missing_permission` |
| `repo_not_in_installation` | 409 | no installation reaches the repository and it could not be added; the answer has `repo` when the server made it |
| `push_failed` | 502 | the repository was made but the first push failed; the answer has `repo` |
| `invalid` | 400 | a field breaks a rule above, or GitHub refused one |
| `rate_limited` | 429 | GitHub limits the server; `Retry-After` is taken from GitHub's reset time |
| `unavailable` | 503 | GitHub cannot be reached; nothing was changed |

The administrators' endpoints add `key_invalid`, `app_rejected`,
`client_rejected`, `provider_in_use` and `public_url_needed`
(see "Administration").

### A site policy for remotes (milestone 3.1)

Reserved in 2.7, specified in "Server storage": `GET /api/admin/storage` and the settings `storage.*`
and `409 remote_required`. 2.7 already makes a space and its repository in one
step that either succeeds whole or leaves no space, so 3.1 adds a check before
step 1 and nothing else to that order.

## Server storage

Milestone 3.1. Accounts mode; in single-user mode every endpoint here answers
`404` and none of this applies (its one space is a directory the user owns).

The server's disk should grow with what people are using, not with everything
they have ever stored. Three things do that:

- a **policy** that requires spaces to follow a remote, so that the server is
  never the only copy and may drop its own;
- **partial caching** of spaces that follow a remote: the server keeps the
  current text and the index; old versions and attachments are fetched when
  someone asks for them, and a space that nobody has used for a while is
  **evicted** down to its index;
- an **attachment repository**: a second remote that holds a space's
  `assets/`, read through a cache of bounded size.

A local space (`sync: "local"`) is the only copy of its pages. It is never
partially cached or evicted; the policy can only limit its size.

### What a space costs on disk

For the 50 000-page corpus of `spikes/search-scale` (62.2 million characters).
The text and the index are measured; the rest are estimates under stated
assumptions (five versions per page, 250 000 commits; 5 GB of attachments in
20 000 files), to be measured on that corpus before 3.1 is merged.

| Part | Full clone (now) | Partial clone | Evicted |
|---|---|---|---|
| Index (`DATA/cache/<id>`) | 166 MB | 166 MB | 166 MB |
| Text in the working tree | 192 MB | 192 MB | 0 |
| Current text in `.git` (zlib) | ~100 MB | ~100 MB | 0 |
| Commits and trees | ~60–120 MB | ~60–120 MB | 0 |
| Older versions of the text | ~100–400 MB | only those someone looked at | 0 |
| Attachments in `assets/` of the space | 5 GB in the working tree + ~5 GB in `.git` (images do not compress) + every older version | the current ones twice, older versions only when looked at | 0 |
| Attachments in an attachment repository | — | the space's share of the attachment cache | 0 |

So such a space takes about 11 GB as a full clone, about 0.6 GB as a partial
clone with an attachment repository (plus what of it is in the attachment
cache), and 166 MB evicted. A more usual space of 2 000 pages and 200 MB of
attachments: about 450 MB full, about 25 MB partial with an attachment
repository, about 7 MB evicted. A server with 1 000 such spaces of which 100
are in use goes from about 450 GB to about 2.5 GB of clones, 7 GB of indexes
and the attachment cache.

What a request costs does not change for a space that is cached (`partial` or
`full`): pages, lists, search, links, checks and current attachments are read
from the working tree and the index exactly as now. Only reads of what is not
on the server (an old version, an attachment not in the cache, any file of an
evicted space) wait for the network.

### The storage policy and `GET /api/admin/storage`

Site administrators write the policy; the server's disk, its policy and its
cache are the server's own, not a tenant's. In the administrators' web page they
are the storage part of "存储与同步" (Storage & sync), above the connections
(see "Connections"). The policy is layered settings of the site, written with
`PUT /api/admin/settings/{key}`; `PUT /api/admin/storage` is gone (`405`).

| Key | Kind | Default |
|---|---|---|
| `storage.remotes.require` | text | `""` (none), `"any"` or the id of a provider |
| `storage.remotes.scope` | choice | `new` (or `all`) |
| `storage.remotes.graceDays` | limit | 30 (0–365) |
| `storage.hosts` | list | the hosts of `--remote-host`, else `github.com`, `gitlab.com`, `codeberg.org`, `bitbucket.org` |
| `storage.localSpaces.maxBytes` | limit or null | `null` (else at least 1 MiB) |
| `cache.partial` | boolean | `true` where Git can |
| `cache.evictAfterDays` | limit or null | 14 (1–365) |
| `cache.minIdleHours` | limit | 24 (1–720) |
| `cache.clonesBytes` | limit or null | `null` (else at least 1 GiB) |
| `cache.minFreeBytes` | limit or null | 10 GiB (at least 0) |
| `cache.assetsBytes` | limit | 5 GiB (at least 64 MiB) |

They apply to the next request, without a restart. `400 invalid` for a value
outside its range. All are server-wide for now (the requirement is worked out
over every space at once); a tenant's own tightening of it is later work.

`GET /api/admin/storage` keeps what is not a setting:

```json
{"remotes": {"since": "2026-12-01T00:00:00Z"},
 "git": {"version": "2.53.0", "partialClone": true},
 "affected": {"inGrace": 3, "overdue": 0, "exempt": 1},
 "summary": { … }}
```

(`summary`, below, is as before; `git.version` is for site administrators
only.) A write of `storage.remotes.require` or `.scope` answers the setting
with `remotes: {"InGrace": 3, "Overdue": 0, "Exempt": 1}` besides: how many
spaces the requirement now asks something of.

- `storage.remotes.require`: `""` (no requirement, the default), `"any"` (a
  space must follow a remote on an allowed host) or the id of a provider, such
  as `"github"` (a repository on that platform with a `connection` credential
  through it, see "Connections"; `409 provider_not_configured`, with
  `provider`, while the provider is not set up; `"git"` is `400 invalid`,
  `"any"` is what takes a plain address).
- `storage.remotes.scope`: `"new"` (spaces made from now on; the default) or
  `"all"`.
- `storage.remotes.graceDays`: how long an existing space has to meet a
  requirement that `scope: "all"` puts on it. `since` (in `GET`, a record of the
  server, never a setting) is when the requirement took its present form
  (`require` or `scope` changed): the server sets it in the transaction of the
  write; a space's deadline is `since + graceDays` (below).
- `storage.hosts`: the hosts a remote may be on, what `--remote-host` sets
  (`"*"` allows any). When the server was started with `--remote-host`, the flag
  wins: the setting's site view has `fromFlag: true` and a write answers
  `409 conflict`. A connection's provider is held to the same list (its host,
  as 2.7b checks `--remote-host`): a provider whose host is not allowed cannot
  be used to connect or make a space.
- `storage.localSpaces.maxBytes`: see "A limit on local spaces".
- `cache.partial`: whether spaces that follow a remote are cloned partially
  (see "Partial clones"). `git.partialClone` is `false` when Git is older than
  2.44 (it cannot refuse to fetch on its own); then a write of `true` answers
  `409 conflict` and the value in force is `false`. Eviction works with any Git.
- `cache.evictAfterDays`: `null` for never. A space not used for that long is
  evicted. `cache.minIdleHours`: a space used more recently than this is never
  evicted, also when the disk is short (no space is fetched and dropped over and
  over). `cache.clonesBytes`: the most that the clones of spaces that follow a
  remote may take together. `cache.minFreeBytes`: when the disk of the data
  directory has less room than this, spaces are evicted as for an exceeded
  budget. `cache.assetsBytes`: the attachment cache, shared by all spaces with an
  attachment repository.
- Where a new space's attachments go is not here: it is the layered setting
  `storage.assets` (see "Where attachments go").
- `affected` counts the spaces the policy now asks something of: in their grace
  period, past their deadline, and exempted.

### Requiring a remote

While `require` applies to a space, it must follow a remote of that kind.
Exempt users' spaces and exempt spaces (below) are left out.

- **New spaces** (any `scope`): `POST /api/spaces` without what is required
  answers `409 remote_required` before anything is made: under `"any"` it
  needs `storage.sync` or `remote`, under a provider's id it needs
  `storage.sync` with that `provider`. `remote` is the body of `PUT …/remote`
  with a token (an SSH key needs the space first, to hold the key); it is
  `400 invalid` together with `storage.sync`.
- **The personal space** is made at a user's first sign-in, when they cannot
  have connected anything yet. It is made local, as now, and is given a
  deadline of its creation plus `graceDays`; the app offers the one step that
  meets the requirement (connect the provider, then a private repository
  `petalbook-<username>` in their account).
- **Existing spaces**, with `scope: "all"`: a space that does not meet the
  requirement gets the deadline `since + graceDays` (a space that lost its
  remote, below, the later of that and the loss plus `graceDays`). Until then nothing
  changes but the notice: `storage.remote` in the space
  (`{"required": "github", "deadline": "…", "overdue": false}`), shown to every
  member, with the way to connect offered to those with `manage`.
- **After the deadline** the space is read-only: it is listed, opened, read,
  and searched as before; drafts keep being saved (they are in the
  state database, not in the space). Every change to its files answers
  `409 remote_required` with the deadline:

  ```json
  {"error": "remote_required", "message": "…", "require": "github", "deadline": "2026-12-31T00:00:00Z"}
  ```

  Connecting it (`PUT …/remote`, `POST …/remote/key`) always works, and once it
  follows a remote of the required kind it is writable again, drafts are
  committed as usual. **Nothing is deleted** at the deadline or after it: not
  the space, not a page, not a draft, not a setting.
- **Removing** a remote (`DELETE …/remote`) and switching to a credential the
  requirement does not accept (`PUT …/remote`) answer `409 remote_required`.
- **Changing the policy.** Relaxing it (`require: null`, `scope: "new"`)
  clears the deadlines at once, and overdue spaces are writable again.
  Tightening it (`"any"` to `"github"`, `"new"` to `"all"`) starts a new
  `since`, so nobody's grace period starts in the past. A new `graceDays`
  moves the deadlines of spaces still in their grace period; when it puts a
  deadline in the past, that space is overdue at once, which `affected` in the
  answer shows before anyone else sees it.
- A space whose remote stops working (offline, `rewritten`, an `errorCode`)
  still follows it and meets the requirement: those states are handled as in
  "Remote repository" and "The GitHub provider".

#### Exemptions

- `PATCH /api/users/{id}` takes `{"remoteExempt": true}`: the spaces this user
  owns are not asked for a remote (new ones included). Site administrators
  only, as the requirement is the site's (`403 forbidden` for a tenant
  administrator). The User object has `remoteExempt` (absent when `false`).
- `PUT /api/admin/spaces/{space}/storage` takes `{"exempt": true}` for one
  space (below).

### A limit on local spaces

With `localSpaces.maxBytes`, an upload (`POST …/assets`) or an import into a
local space whose measured size (`sizeBytes`, see "Administration") plus the
request's body would exceed it answers `507 space_full`. Saving text is never
refused, so nobody loses typing; the measurement can be up to 6 hours old,
and an upload or import measures the space again first when the stored size
is within 10 % of the limit. Spaces that follow a remote are not limited by
it: their copy on the server is a cache.

### Partial clones

A space that follows a remote is cloned with `--filter=blob:none` (from Git
2.44; with `cache.partial` on). The server has every commit and every tree,
and the contents of the files only of the current version, which it checks
out as now. Older versions of a file are fetched when someone opens them, and
kept until the space is evicted.

- **Why not a shallow clone.** A shallow clone (only the latest commits)
  would save the commits and trees too, about 60–120 MB at 50 000 pages, but
  the history of a page, following a page through renames, detecting a
  rewritten history, and counting what is not pushed all walk commits. Each
  page of history would cost a `--deepen` round trip, which also brings every
  file of those commits. With the commits local, all of that stays local and
  only file contents are fetched, a few kilobytes each.
- **Git never fetches on its own.** Every Git command on the server runs with
  `GIT_NO_LAZY_FETCH=1`. A missing object is an error the server handles, not
  a silent network request without credentials or a deadline in the middle of
  a read. The server fetches missing contents itself, by their ids, through
  the same credential, deadline and error handling as every pull, in one
  request per batch (`git fetch --filter=blob:none --stdin` with the ids).
  The restricted Git runner allows these options and nothing more.
- **Pulls.** The pull every minute (open spaces only, as now) fetches the new
  commits and trees; when they changed files, the new contents of those files
  are fetched in one more request before the space moves forward. A pull that
  brings nothing costs what it costs now.
- **A host that cannot filter** (it says so, and Git fetches everything)
  leaves the space a full clone: `cache: "full"`. Nothing else changes.
- **Existing clones** stay full until they are evicted and restored, which
  makes them partial; `POST /api/admin/spaces/{space}/evict` does it now.
  Git cannot drop contents from a clone in place, and re-cloning a space in use
  is a restore anyway.

#### History and versions

`GET …/history` and `GET …/versions/{commit}` (and `page_history`,
`page_version` in MCP) read the commits locally. For each page of history the
server lists the versions with their content ids (one `git log`, as now) and
fetches those it does not have, at most `limit` plus half of it, in one
request (a page of 30 versions of a 4 KB text is about 100–200 KB). A
version, once fetched, is not fetched again. Following a page through renames
uses Git's rename detection only for exact renames, which needs no content;
the page's id recognises the rest, as it already does when Git's detection
gives up (the heads of the files deleted in one commit, fetched in one
request). The versions listed are the same as with a full clone.

While the server fetches, the request waits, up to 15 seconds. When the
remote cannot be reached, refuses the credential or takes longer:

```json
{"error": "not_cached", "message": "This version is not on the server and the remote repository cannot be reached.", "fetching": false}
```

`503`, with `Retry-After` (5 seconds when `fetching` is `true`: the fetch goes
on and the next try usually finds it; 60 seconds otherwise). A page of
history is complete or not given: there is no partial list. Versions already
on the server are read as now, also while the remote is down. Repairing a
settings file from its earlier versions (see "Settings files that cannot be
used") looks only at versions on the server when the remote cannot be
reached, and tries again at the next opening.

### Eviction

An evicted space has no clone on the server: `DATA/spaces/<id>` is removed. Its
index, its records (members, remote, drafts) and its settings in the state
database stay. Lists, the tree, tags, search (the space's and the global one),
links and backlinks, the checks, and the offline sync's catalogue, changes
and heads (see "Offline sync") are answered from the index without
fetching anything, as fast as for a cached space. What the index does not
hold is left out rather than fetched: a backlink's `excerpt` is `""` (it is
the paragraph, read from the file), a listing has no tag colours and no type
a folder gives its pages (both are in files of the space). Everything that
needs a file restores the space first.

**Which spaces.** Only spaces that follow a remote, and only when all of this
holds at the moment of eviction, checked while the space is closed and held
so that nothing can open it:

- its remote is `ok` (not `offline`, `rewritten` or `error`);
- the committer has nothing pending, the journal is empty, and a fetch made
  just then shows the remote's branch at the space's `HEAD`: every commit of
  the space is on the remote;
- it holds no rescued commits (`refs/petalbook/unpushed/…`): those exist
  nowhere else;
- it has no draft, in the state database, of anyone's (a draft is safe there,
  but an eviction never sits between a person and what they were typing);
- it is not pinned, and it was not used for `minIdleHours`.

When any of these fails, the space stays and is tried again at the next
round. A space is evicted when it has not been used for `evictAfterDays`,
and earlier, the least recently used first, while the clones together exceed
`clonesBytes` or the disk has less than `minFreeBytes` free. "Used" is any
request that opens the space; a pull is not a use. The time of the last use
is in the state database, written at most once an hour per space. One round
runs every 10 minutes, evicts one space at a time and takes a few seconds per
space (removing its files); the sizes it adds up are the stored measurements,
one query. The attachment cache of an evicted space goes with it.

**Restoring.** The first request that needs a file of an evicted space starts a
restore and waits for it, up to 10 seconds:

1. a partial clone of the remote's branch (commits, trees, and the current
   contents of everything but an attachment repository's files);
2. the checkout;
3. the files whose content id is the one the index has get back the
   modification time the index recorded, so the index sees them as unchanged
   and the order "by update" is what it was; only the files that changed on
   the remote meanwhile are read again, as after a pull;
4. the space opens as usual. Its index is the one it had: the change cursors
   of offline clients stay good (no `410`), and what changed on the remote
   meanwhile comes to them as changes (see "Offline sync").

For a space of 2 000 pages that is about 10 MB to fetch and 1 to 3 seconds;
for 50 000 pages about 200 MB and some 20 to 40 seconds on a 100 Mbit/s link.
At most two restores run at a time; others wait their turn. Requests that are
still waiting after 10 seconds answer:

```json
{"error": "restoring", "message": "…", "phase": "fetching"}
```

`503` with `Retry-After: 5`; the restore goes on. `phase` is `waiting`,
`fetching`, `checkout` or `indexing`. The space's `storage.cache` is
`restoring` meanwhile. When the remote cannot be reached or the credential
does not work, the restore fails at once: `503 unavailable` with the remote's
`errorCode` where there is one (a connection that expired is fixed as in
"When a GitHub credential stops working"), and the index-only answers
above go on working. One restore is given 5 minutes once it has its turn; a
remote that does not answer in that time fails it the same way. After a
failure the space is not tried again for a while (3 seconds, doubled with each
failure in a row, at most 2 minutes): requests meanwhile answer `503
unavailable` at once, with `Retry-After` set to the seconds left. A failed restore leaves no half clone: the clone is made
in a directory of its own and moved into place when it is complete.

The same answer is given for a space whose index has to be rebuilt when it is
opened (after a schema change of the index, a lost index, or the first open of
a large space): such rebuilds are limited server-wide (`--max-rebuilds`, 2 by
default), and a request that is still waiting after 10 seconds answers `503
restoring` with `Retry-After: 5`, `phase` `waiting` (its turn has not come) or
`rebuilding` (the index is being built). The rebuild goes on in the background,
and once it is done the space opens as usual. Opens that rebuild nothing (an
indexed space, a few changed files) never answer this.

What a crash leaves (an eviction cut short, a restore's directory) is finished
or removed by the next start of the server, and by nothing else: `petalbook
user …` run against the data directory of a running server touches none of
it. The server holds `DATA/serve.lock` while it runs; a second server on the
same data directory does not start.

Drafts are not affected by any of this: they are in the state database, with
the rev they started from, and are committed after the restore like any other
draft.

### The attachment cache

The contents of attachments from an attachment repository live in
`DATA/assetcache/<space>/<content id>`, one file each, named by the content id
Git gave it, so the same file is stored once however many times it is
referred to. All spaces share one budget, `cache.assetsBytes`; when it is
exceeded, the least recently read files go first. A file's modification time
is its last read (updated at most once an hour), so the order survives a
restart without a database. The total is kept in memory and is exact: the
server writes every file in the cache itself.

### Attachment repository

A space that follows a remote can keep its attachments in a second Git
repository. The space's repository then holds only the text and stays small:
fetched quickly, restored quickly, cheap to keep in a partial clone.

**A plain Git repository, not Git LFS.** It works on every host a space can
use, with the same kinds of credential (a `connection` too), the same
restricted Git runner and the same provider; its files can be browsed on the
platform like any others. LFS needs
a second protocol and service beside Git, has its own storage and bandwidth
quotas on GitHub billed apart from the repository, is not offered by every
host, and puts pointer files in the space's repository that other clones show
as text unless they have LFS installed.

**Layout.** The repository's root is the content of `assets/`:
`<page id>/<name>`, on `main`. Someone who clones the space's repository and
then this one into its `assets/` folder has the same files as before, and every
link works in any editor. Commits are made as in the space (the editor is the
author, the service the committer), and are pushed before the request is
answered.

**Links do not change.** A page refers to `/assets/01JD3K8Q7Z/street-3f9a1c2b.jpg`
or to a relative path that leads into `assets/`, as before; no page is
rewritten. `GET …/files/assets/{page id}/{name}` finds the file in the
attachment repository instead of the working tree: the path is looked up in
a table of the repository's current files (path, content id, size) that the
server keeps in memory, made when the space opens (20 000 files: about 2 MB,
some 50 ms) and updated by each pull and upload; then the file is sent from
the attachment cache. The size of a file is known to the table only once its
contents have been in the cache (a partial clone cannot say how large a file
it does not have); the answer's `Content-Length` is that of the cached file. A file that is not in the cache is fetched by its id
first (as for an old version), and later readers share that fetch. The answer
is the same as now except that it carries `ETag` (the content id) instead of
`Last-Modified`, so a browser revalidates with `If-None-Match` and gets `304`
(finding a file's last commit would cost a walk of the history); `Range` and
the other headers are as now. A path under `assets/` that the
table does not have is `404 not_found`, without asking the remote.

**Checks** ("attachment reference to a file that does not exist") use the same
table, so they cost what they cost now. While the attachment repository
cannot be reached, the table is the last one the server had: an unreachable
repository does not make attachments look missing.

**Unreachable.** The page text is read and written as usual. Attachments in the
cache are served. Others answer `503 not_cached` with `Retry-After`, as an old
version does; a client shows a placeholder where an image would be. Uploads
answer `503 unavailable` and store nothing. The attachment repository has a
state of its own (`ok`, `offline`, `error` with `errorCode`, as a Remote); it
does not pause the space.

**Order of writes.** An upload is pushed to the attachment repository before
the request is answered, so a page saved after it never refers to an attachment
that is not on its remote. An import pushes its attachments first, in one
commit, then the pages in one commit of the space; when the second step fails,
the attachments stay on the remote, referred to by nothing, and the import
answers as a failed import does now (no page made). Nothing is ever pushed
with force or deleted on either remote.

#### `GET /api/spaces/{space}/remote/assets`

Needs `manage`. `200` with a Remote object (see "Remote repository") with one
more field, `move` (below); `404 not_found` when the space has none.

#### `PUT /api/spaces/{space}/remote/assets`

Needs `manage`. The body of `PUT …/remote` (a `connection` credential
included), or to make a repository on a provider:

```json
{"sync": {"provider": "github", "owner": "kazuhahub", "name": "tang-notes-assets", "private": true, "create": true}}
```

with the fields and rules of `storage.sync` in "Creating a space with a
repository" (`via` as the provider chooses, unless given; a `via: "user"`
connection syncs only spaces of its person's tenant, as for the space's own
remote). A repository that was made and could not be connected is named in
`repo` and is remembered as a leftover (see "Leftover repositories"). The space must follow a
remote: `409 remote_required` otherwise. Connecting follows the rules of
`PUT …/remote`: an empty repository is used as it is; one with content is
accepted while the space has never used an attachment repository and its
`assets/` is empty (the space takes its files); otherwise `409 conflict`. On a
space that has one, `PUT` replaces its address and credential, the same
repository with another credential. `200` with the Remote object. Connecting
does not move anything: the space's existing attachments stay in `assets/`
of the space and are served from there until they are moved; new uploads go
to the attachment repository.

#### `POST /api/spaces/{space}/remote/assets/move`

Needs `manage`. Moves the attachments in `assets/` of the space into the
attachment repository. `202` with the Remote object; the move runs in the
background, and `move` says how far it is:

```json
"move": {"state": "copying", "files": 20000, "filesDone": 8000, "bytes": 5368709120, "bytesDone": 2147483648, "error": ""}
```

`state` is `copying`, `switching`, `done` or `failed`.

1. **Copying.** The files of `assets/` at the current commit are committed to
   the attachment repository and pushed, in commits of at most 500 MB
   (GitHub refuses a push over 2 GB). The space stays writable; uploads still
   go into `assets/` of the space meanwhile.
2. **Switching.** With the space's writes held for a moment, whatever
   `assets/` gained since the copy started is pushed too; then one commit of
   the space removes `assets/` and adds `/assets/` to its `.gitignore`, and
   is pushed. From that commit on, attachments are read from the attachment
   repository.
3. The server replaces its clone of the space's repository by a new partial
   clone, as a restore does, while the old clone goes on serving; the disk the
   attachments took is free when the new one is in place.

The space's history still has the attachments, and so does its remote: history
is never rewritten. A failure before the switch changes nothing in the space
(`failed`, `error` says why; the files already pushed to the attachment
repository stay there, and the next `POST …/move` goes on from them). Only one
move at a time: `409 conflict` while one runs, and also for `PUT` or `DELETE`
of the attachment repository. `200` with nothing to do when `assets/` is empty.
The move is a job of the kind `assets.move` (`GET /api/admin/jobs`): it waits
for its turn with the tenant's other storage work (one at a time per
organisation, two on the server; `copying` while it waits), and a stop of the
server, or a crash, leaves it to go on at the next start. `503 unavailable`
while the server is stopping: nothing is added.

#### `DELETE /api/spaces/{space}/remote/assets`

Needs `manage`. `204` while no attachment has gone to it (nothing was uploaded
or moved): the space keeps its attachments in `assets/`. Otherwise
`409 conflict`: moving the attachments back into the space is not part of 3.1.

#### With a new space

`POST /api/spaces` with `storage.sync.assets` (`true`, `false` or
`{"name": "…"}`; absent follows `storage.assets`, and the name defaults to
`<name>-assets`) makes the attachment repository right after the space's
repository (step 3 of "Creating a space with a repository"), with the same
owner, visibility and credential, and checks it in step 4 the same way (an
installation with "only selected repositories" gets it added, or the request
fails with `409 repo_not_in_installation`).
Failures are those of that section; once either repository has been made,
the answer names what was made, and the same request with `create: false`
uses both. Each repository that was made is also remembered as a leftover
until a space uses it:

```json
{"error": "push_failed", "provider": "github", "message": "…",
 "repo": {"id": 881234567, "fullName": "kazuhahub/tang-notes", "htmlUrl": "…"},
 "assetsRepo": {"id": 881234568, "fullName": "kazuhahub/tang-notes-assets", "htmlUrl": "…"}}
```

When the platform refuses to make the attachment repository after the space's
was made, the answer is the provider's own refusal as step 3 gives it (`409
name_taken`, `403 provider_refused`, `429 rate_limited` with its
`Retry-After`, …), with `repo` naming the space's repository and no
`assetsRepo`. The same request with `create: false` then finds the space's
repository but not the attachment one, and fails with the error of
connecting to it (`403 owner_not_allowed`, `409 repo_not_in_installation`,
…), whose `message` names the attachment repository that is missing and whose
`repo` names the space's: make that repository on the platform (or drop
`storage.sync.assets`) and send the request again.

### Administrators: per space

Site administrators only, as `/api/admin/storage` (the pin keeps the server's
disk, the exemption is from the site's requirement, eviction is the server's
cache): a tenant administrator gets `403 forbidden`.

#### `PUT /api/admin/spaces/{space}/storage`

```json
{"pin": true, "exempt": false}
```

Each optional. `pin` keeps everything of the space on the server: it is never
evicted, its clone becomes a full one (all versions are fetched in the
background, and from then on every pull brings every version) and its attachments are all kept in the cache, outside
`assetsBytes`. A pinned local space is the same as an unpinned one.
`exempt` leaves the space out of the requirement of a remote. `200` with the
item of `GET /api/admin/spaces`. Pinning while the remote cannot be reached is
accepted; the full fetch waits for the remote, and an evicted space is
restored at the next working fetch.

#### `POST /api/admin/spaces/{space}/evict`

Evicts the space now, if it may be evicted (the conditions of "Eviction",
except `minIdleHours`). `204`; `409 busy` while requests use it or it holds
something not on the remote (`message` says what); `409 conflict` for a local or
pinned space; `503 unavailable` when the remote cannot be reached (it is not
evicted without checking that the remote has everything).

#### In the list of spaces and the status

`GET /api/admin/spaces` and `GET /api/admin/spaces/{space}` (a tenant
administrator's included, for the spaces of their tenant) add to each item:

```json
"storage": {"cache": "partial", "pinned": false, "exempt": false, "lastUsed": "2026-11-22T02:10:00Z",
            "cloneBytes": 41943040, "indexBytes": 7340032, "assetCacheBytes": 15728640,
            "assets": "repository", "remote": {"required": "github", "deadline": "2026-12-31T00:00:00Z", "overdue": false}}
```

`cloneBytes` is the `sizeBytes` measured as now (the directory of the space,
0 once evicted), `indexBytes` the index measured with it, `assetCacheBytes`
the space's files in the attachment cache (exact). `?cache=evicted` (or any
other `cache` value) and `?overdue=1` filter the list, in the same one query.

`GET /api/admin/status` (site administrators) adds:

```json
"storage": {"spaces": {"local": 12, "full": 3, "partial": 41, "evicted": 156, "restoring": 0},
            "cloneBytes": 2147483648, "indexBytes": 1288490188, "assetCacheBytes": 4831838208,
            "clonesBudget": null, "assetsBudget": 5368709120, "restoresWaiting": 0,
            "remotes": {"inGrace": 3, "overdue": 0}}
```

the sums of the stored measurements (one query) and of the cache in memory.

### Offline and expired credentials

| Situation | Cached space | Evicted space |
|---|---|---|
| Remote unreachable | reads as now; writes `503 unavailable`; old versions and attachments not on the server `503 not_cached` | lists and search from the index; anything needing a file `503 unavailable` |
| Credential refused, a connection expired or missing (`connection_expired`, `connection_missing`) | the same, with the remote's `errorCode` | the same, with the remote's `errorCode` |
| Attachment repository unreachable | text as usual; cached attachments served; others `503 not_cached`; uploads `503 unavailable` | as above once restored |

A space is never evicted in any of these states, so a space in use when its
remote goes away keeps working from its clone. Drafts keep being saved in every
case.

### Errors of this section

| Code | Status | When |
|---|---|---|
| `remote_required` | 409 | the policy requires a remote this request would not have: a new space without one, removing one, a credential the policy does not accept, a change to an overdue space; an attachment repository for a space without a remote. The body has `require` and, for an existing space, `deadline` |
| `not_cached` | 503 | an old version or an attachment is not on the server and cannot be fetched now; `fetching` says whether a fetch is still running; `Retry-After` |
| `restoring` | 503 | an evicted space is still being restored, or a space's index rebuilt, after the request's wait; `phase` (`waiting`, `fetching`, `checkout`, `indexing`, `rebuilding`); `Retry-After` |
| `space_full` | 507 | an upload or import would take a local space over `localSpaces.maxBytes` |
| `unavailable` | 503 | as everywhere: the remote cannot be reached or refuses the credential; for an evicted space, it cannot be restored now (`Retry-After` while a failed restore is not tried again) |
| `busy` | 409 | an eviction asked for while the space is in use or holds what its remote does not |
| `conflict` | 409 | a second move; a change of the attachment repository during a move or after files went to it; `hosts` set by the flag; partial clones without Git 2.44 |
| `invalid` | 400 | a setting outside its range, `remote` and `storage.sync` together |

The codes of "Connections" apply to making an attachment repository as they
do to making a space's, with `provider`.

**Usage and quotas (3.3).** What the quotas count of a space (`usage_bytes`)
is written only by a measurement of a whole copy: a partial clone or an
evicted space is measured for the list (`sizeBytes`, `cloneBytes`) only, and
its usage keeps the last full measurement, so neither shrinks what a person
uses.

## Pages

### Page summary

Used in listings. `title` and `subtitle` are absent when the page has none.
`source` is always there: a list of texts saying where the text comes from (a
person, a book, a film), `[]` for none, like `tags`. In the file it is a list
too; a single text written by hand there (`source: 雪国`) is read as one source,
and the same page is a filter, a sort (`prop:source`, by its first source) and
the column of a board (`key=source`) like a list property. The sources are not
among `props`.
`label` is always present: the title, or for an untitled page the first line
of the body cut to about 30 characters (`untitled` says which). It is an
empty string for an untitled page with an empty body. `created` is absent
when the header has none. `updated` is when the file was last written.
`icon` and `color` are the page's own, from its header, and are left out when it has none (see
"Icons, colours and the presets of the sources"). `excerpt` keeps the line breaks of the text as single `\n`; blank lines
between paragraphs are not kept. A task item keeps its state at the start of its line, as `[x] ` (done, whatever the case of the x in the file) or `[ ] ` (not done), without the list dash or the indentation; the marker counts toward the 400 characters. A line that starts with `[x] ` or `[ ] ` in the excerpt is shown as a task, and a client cannot tell it from the same literal text in the page.

`type` is what the header says (`""` when it says nothing). `effectiveType` is
the type the page has: the header's, else the type of the nearest folder
above the page that is bound to one (see Page types), else `""`. It names a
type; whether a type of that name exists is not looked at.
`props` and `rev` are there only when the listing was asked for them with
`props=1` (the table view does): `props` is an object of the header's keys
that are not built in (everything except `id`, `title`, `subtitle`, `source`,
`tags`, `created`, `type`, `icon` and `color`), in header order, `{}` when there are none, and
`rev` is the page's rev, which a change of a property needs. Only values a
table cell can show are in it: text (cut to 300 characters), numbers, true
and false, and lists of those (the first 100 elements). A nested object, a
null, blank text and an empty list are left out, as is anything inside a list
that is not a plain value. Without `props=1` the summary is as small as it
was.

`body` is there only when the listing was asked for it with `body=1` (the
continuous reading does): the page's Markdown as the file has it now, without
the header, `""` for a page with no body. See `GET …/pages`.

`partialProps` comes with `props` and lists, in header order, the keys whose
value in `props` is not exactly what the file holds: text that was cut, a list
that was cut or lost an element, a value that is an object (it has no entry in
`props` at all), a number JSON cannot say, a key beyond the first 200, and any
value that would not come back the same through JSON and the browser: an
integer beyond 2^53, a number not written the way the server writes it
(`0x10`, `1.0`, `+5`, `1e3`, `.5`), a value with a tag of its own (`!custom x`),
a bool not written `true` or `false`. A text, a bool or a number written the
canonical way, and a date written `YYYY-MM-DD` (alone or in a list; a list of
dates is written back as dates), are whole. One such element
makes the key of a list partial. It is
left out when there are none. Such a key must not be written back from the
copy: a client shows it as it is and leaves it to be changed by opening the
page. A key whose value is null, blank text or an empty list has nothing to
lose and is not listed.

```json
{
  "id": "01JD3K8Q7Z",
  "path": "Tang/暮雪.md",
  "folder": "Tang",
  "title": "暮雪",
  "subtitle": "…",
  "source": ["书", "电影"],
  "label": "暮雪",
  "untitled": false,
  "tags": ["玉兰", "黄昏"],
  "type": "",
  "effectiveType": "文案",
  "created": "2025-12-03",
  "updated": "2026-11-21T08:30:00Z",
  "excerpt": "first 400 characters of the body as plain text",
  "props": {"status": "draft", "points": 3, "labels": ["a", "b"]},
  "rev": "3b18e512dba79e4c8300dd08aeb37f8e728b8dad"
}
```

### Page

Returned by get, create, save and commit. `meta` is the header as an object,
in file order, including fields the server does not know about; `id` is in it.
`body` is the Markdown after the header. `header` is the text of the header
exactly as it is in the file (the lines between the two `---` lines, each
ending in `\n`), for showing the file as it was written; it is always the
file's, and what a save writes is decided by `meta` alone. `GET` adds `draft` when the caller
has one for the page (see Drafts); `meta` and `body` are always the file's.

```json
{
  "id": "01JD3K8Q7Z",
  "path": "Tang/暮雪.md",
  "folder": "Tang",
  "rev": "3b18e512dba79e4c8300dd08aeb37f8e728b8dad",
  "updated": "2026-11-21T08:30:00Z",
  "meta": {"id": "01JD3K8Q7Z", "title": "暮雪", "tags": ["玉兰"], "created": "2025-12-03"},
  "header": "id: 01JD3K8Q7Z\ntitle: 暮雪\ntags: [玉兰]\ncreated: 2025-12-03\n",
  "body": "…\n",
  "effectiveType": "文案"
}
```

`effectiveType` is the type the page has: the header's `type`, else that of
the nearest folder above it bound to a type, else `""` (see Page types). A
new page is not given a `type` in its header; it takes its folder's.

`partialProps` is there when some keys of `meta` are held only in part by a
listing: the same list, judged by the server from the file, as a summary's
(see Page summary), in every response that returns a page (get, create, save,
`PATCH …/props`, move, and the `current` of a 409), and left out when there
are none. A client takes it from the response and never works it out from
`meta`: `0x10` and `16` look the same there.

### `GET /api/spaces/{space}/pages`

Query parameters, all optional:

| name | meaning |
|---|---|
| `folder` | only pages directly in this folder (empty string = root) |
| `recursive` | `true` to include subfolders of `folder` |
| `tag` | only pages with this tag (repeatable: all must match; at most 10 different tags, repeats count once, more is `400 invalid`) |
| `q` | full-text search over title, subtitle, source and body |
| `in` | with `q`: `title` searches titles only |
| `updatedFrom`, `updatedTo` | only pages last written on or after / on or before this day (`2026-11-01`, UTC) |
| `createdFrom`, `createdTo` | the same for the `created` date of the header; a page without one never matches |

A date that is not a day written `YYYY-MM-DD`, or lies outside the years 1970
to 2261, is `400 invalid`.
| `sort` | `updated` (default), `created`, `title`, or `prop:{key}` for a property of the header; prefix `-` for descending. Default is `-updated`. With `q` and no `sort`, results are ordered by relevance |
| `filter` | a JSON array of conditions on properties, all of which must hold (see below), at most 11 (a saved view has at most 10: a board's column adds its own condition), at most 128 KiB (room for a board's "other values" column on a field with the most options a type may give) |
| `props` | `1` to put `props` and `rev` in the summaries (see Page summary); default `0` |
| `body` | `1` to put the Markdown `body` of each page in the summaries (see below); default `0` |
| `limit` | 1–200, default 50; with `body=1` at most 20, default 20 |
| `cursor` | the `next` value of the previous response |

```json
{"items": [/* page summaries */], "next": "opaque-or-empty"}
```

`next` is an empty string when there are no more results. Sorting by title
puts untitled pages last in both directions, ordered by `created`; titles
compare by code point, case-insensitively. Search terms separated by spaces
must all match; a term matches as a run of adjacent characters and does not
match across punctuation or a line break.

**Sorting by a property.** `sort=prop:status` orders by the value of the
header key `status`; `sort=-prop:status` reverses that. A page that has no
value for the key (the key is missing, null, blank text or an empty list)
comes after all that have one, in both directions, in the order the server
holds the pages in; ties among the others are broken the same way, following
the direction of the sort. Text sorts like titles do (by code point,
case-insensitively), numbers as numbers, dates (`2026-11-22`) as text, true
after false. A list sorts by its first element. The key is any key; it need
not belong to a type. The cursor stays valid while pages change: it names the
position, not an offset, and a cursor made for one sort is refused for
another. Every page of such a listing is read from an index, however many
pages the folder holds.

**Sorting by a select field.** When the listing has a `folder`, and the
folder's effective type (the type of the folder, or of the nearest folder above
it that is bound; see Page types) has a `select` or `multiselect` field with
the key sorted by and at least one option, `sort=prop:{key}` follows the order
of the field's options instead of the order of the text: the options in turn,
then the values that are not options (by text, as above), then the pages
without a value. `sort=-prop:{key}` is the reverse of the first two groups
(the other values first, then the options from the last) and still puts the
pages without a value last. A multiselect counts by its first value. Within an
option the pages are in the order the server holds them, reversed for `-`. The
listing is read from the index one option at a time, with a cursor that names
the option and the place in it, so a page costs the same however many pages
the folder holds (measured: about a millisecond in a folder of 50 000, in
either direction, with 5 options or 500). With filters, or in a small folder
among many pages, it is read like any filtered listing (see "What a filtered
listing costs" below). A field that lists an option twice (a type file edited by hand) has
it once, the first writing counting, and at most 500 options count.
The cursor also says how many options there were: a cursor made for another
list of options, or from a listing that was not in option order (the type was
changed, or the folder has none), is `400 invalid` and the client starts over.
Pages of subfolders listed with `recursive=true` follow the order of the
options of the folder asked for, whatever type they have. Without `folder`,
or for any other field, the order is that of the text.

**Filtering by properties.** `filter` is a JSON array, sent as the value of
the parameter (percent-encoded), each item `{"key", "op", "values"}`:

| `op` | `values` | keeps pages whose property |
|---|---|---|
| `in` | up to 500 texts, numbers or booleans | equals one of them (a list: holds one of them) |
| `notIn` | up to 500 texts (as many as a select field has options), numbers or booleans | has a value and none of its values (a list has several) is one of them: not empty, and not an option; the "other values" column of a board |
| `range` | `[from, to]`, both numbers or both texts (dates), either end `null` or `""`, not both; ends are included | has a value in the range (a list: an element is in it) |
| `empty` | none | has no value |
| `notEmpty` | none | has a value (`false` and `0` are values) |
| `contains` | one text | has a value that contains it; letters outside ASCII match in the case they are written |

Several items must all hold. An unknown `op`, a missing key, the wrong number
or kind of `values`, or text that is not that JSON is `400 invalid`. The
filter works together with every other parameter (`folder`, `tag`, `q`, the
sort and the cursor). A text is compared as the first 300 characters of it.
Both `sort` and `filter` answer from the index: nothing reads the page files.

**What a filtered listing costs.** A listing with filters, tags, dates, a
recursive folder or a sort by a property is read in the cheapest of three
ways, each bounded, so that no combination reads a folder page by page.
Measured in a space of 50 000 pages, the median of a page of the listing:

- *From the smallest condition.* When a folder, a tag, or an `in`, `range`,
  `notEmpty` or `notIn` filter keeps at most 2000 pages, the read starts from
  those and checks the rest for each: a folder of 100 among 50 000 in any
  order, a filter few pages match, the "other values" column of a board,
  about 1 ms; 1100 matches put in option order, 6 ms. A filter no page
  matches (`in` with 500 values none of which exists, a column for an option
  nobody uses) answers in under a millisecond.
- *By walking the order.* When every condition keeps many pages, the order is
  walked from the cursor and each page checked, for at most 1000 pages (or 5
  per item asked for): a column of a board for an option in use, a sort with
  a filter half the pages meet, any page of it by cursor, 1 to 3 ms.
- *As sets.* When the walk does not fill the page (few pages match and no
  one condition says so, as with `contains` or `empty`; or the matches come
  last in the order), each condition is read as one range of its index and
  the ranges are intersected, about 6 ms for each 50 000 rows, four at a
  time: `contains` that nothing matches 15 ms, a sort by a property with a
  filter on the same property 20 ms, a search with a sort 25 ms, ten
  `contains` filters at once 50 ms.

The dear case is a request whose every condition keeps most of the pages
while together they keep few: ten tags and ten filters (with lists of 500
values), each on nine pages in ten, 200 items asked for, took 40 to 140 ms.

**Bodies.** With `body=1` each summary has `body`, the page's Markdown, read
from its file. That is a file read per item, so `limit` is at most 20 (also
the default) and a larger one is `400 invalid`; the other parameters work as
usual, so the reading of a folder, a tag or a set of filters goes on with the
`next` cursor. A page whose file is not that page any more (it changed outside
the server since the index looked) has `""`.

### `GET /api/spaces/{space}/pages/groups`

Needs `read`. How many pages of a folder or a tag have each value of a
property, for the heads of the columns of a board. Parameters: `key` (required,
the property), `folder` and `recursive`, `tag` (repeatable, at most 10 different), `filter` (as for a
listing); a folder or a tag is required.

```json
{"groups": [{"value": "草稿", "count": 12}, {"value": "定稿", "count": 7}, {"value": "已废弃", "count": 1}],
 "empty": 4, "truncated": false}
```

`groups` are the values the pages have, most pages first, then by the text of
the value (numbers and booleans as the index holds them: `"3"`, `"true"`); a
list property counts a page once for each distinct value it holds. `empty` is
the number of pages in the scope that have no value for the key (the same pages
as the filter `empty`). Both come from the index, no file is read; the answer
is kept for as long as the index is not written to, so a board opened again
costs nothing. At most 1000
values are returned, the rarest left out, and `truncated` says so. The cost,
when the answer is not a kept one, is one read of the index range of each
condition and one of the rows of the property, no lookup page by page.
Measured in a space of 50 000 pages: 25 ms for a recursive folder or a tag
that holds them all, 30 ms for a list property, 40 to 50 ms for a property
with thousands of distinct values; 15 to 35 ms with a `range`, `contains` or
`notIn` filter (100 or 500 values alike); 10 ms for one folder of 5000; under
a millisecond for a small folder or a tag of a hundred pages; 3 ms for the
"other values" of a board. With ten tags and ten filters that each keep
most of the pages it took 100 to 135 ms: twenty ranges of the size of the
space.
`400 invalid` for a missing `key`, no folder and no tag, more than 10 different tags, or a bad `filter`.

### `GET /api/spaces/{space}/pages/{id}`

The page, or `404 not_found`. With the caller's draft, if any:

```json
{"id": "…", "rev": "3b18e5…", "meta": {…}, "body": "…", "…": "…",
 "draft": {"version": "k3f9…", "baseRev": "3b18e5…", "meta": {…}, "body": "…", "updated": "2026-11-21T08:31:10Z", "stale": false}}
```

### `POST /api/spaces/{space}/pages`

```json
{"folder": "Tang", "meta": {"title": "暮雪", "tags": ["玉兰"]}, "body": "…"}
```

All fields optional. The server assigns `id` and `created`, picks the file
name (from the title, or from the date and id when there is no title), creates
missing folders, and returns `201` with the page. A client-supplied `id` or
`created` in `meta` is ignored.

### `PUT /api/spaces/{space}/pages/{id}`

```json
{"baseRev": "3b18e5…", "meta": {"id": "01JD3K8Q7Z", "title": "暮雪"}, "body": "…"}
```

Replaces header and body; both `meta` and `body` are required, as is
`baseRev`. `meta.id` must be absent or equal to `{id}`; `created` is kept
from the file when left out.
Header fields that are already in the file keep the file's order, and fields
new in the request are appended in the order they appear in it; fields the
request leaves out are removed. Unknown fields are written back as given.
Changing the title renames the file; the id does not change.

Saving this way leaves the caller's draft for the page alone, unless the draft
says exactly what was saved (then it is removed). A draft holds text that may
exist nowhere else: the server never discards one as a side effect. After a
save the draft is usually stale, and the app shows both versions when the page
is opened. A save that would write the bytes the file already has changes
nothing and returns the same `rev`.

- `200` with the page (new `rev`, possibly new `path`) and `textRev` (see
  below).
- `409 conflict` when `baseRev` is not the file's current rev, unless only the
  server's link rewrites changed the file since (see "Links between pages").
  The response is `{"error": "conflict", "message": "…", "current": {/* page */}}`
  so the client can show both versions.
- `400 invalid` for a body that is not valid UTF-8, an `id` mismatch, or a
  request without `baseRev`, `meta` or `body`.

#### Typing during a save

A save can write other text than it was sent. A save whose `baseRev` is behind
the file only by the server's link rewrites (a page it links to was renamed or
moved meanwhile, see "Links between pages") gets the same rewrites made to its
text; and when the save moves the file, the links in it that the move changes
are rewritten too. The answer to a save
(`PUT …/pages/{id}`) and to a commit (`POST …/pages/{id}/commit`) therefore
carries `textRev` next to `rev`:

```json
{"id": "01JD3K8Q7Z", "path": "Tang/新名.md", "rev": "9c41…", "textRev": "57ad…", "body": "…", "…": "…"}
```

`textRev` is the rev of the page file as the client's text makes it (its
`meta` and `body` as sent), before this save's link rewrites; `rev` is the file
after them. When nothing was rewritten the two are equal. (Ids the server adds
to links on save do not count: they are added again on the next save.) Only these two answers carry it: `GET`, `PATCH
…/props` (which never rewrites links) and the other answers that return a page
leave it out, and a client reads a missing `textRev` as `rev`.

The rule for a client that keeps editing while the save is out:

- nothing was typed since the save was sent: take the answer's `body`,
  `meta` and `rev`, as for any save, and show them (the rewritten links
  included);
- something was typed: the text in hand is the sent text plus the typing, so
  it is based on `textRev`, not on `rev`. The next save (or draft) sends
  `baseRev = textRev`. The server has logged the rewrites as steps from
  `textRev` to `rev`, makes them again to that text, and keeps the typing (the
  same rebase as "Links between pages" describes). If the file changed in any other way since, the answer is the usual
  `409 conflict` (or a stale draft), with nothing new to handle.

Sending `baseRev = rev` with the old text would write the old link back: a
link that carries an `id` is put right again on save, but one without (written
outside PetalBook, or with a title of its own) would point at the old name.

The cost is one field in two answers. A save that rewrote nothing does no
more work than before. One that was rebased logs one rebase step more per
rewrite it made (the rewrites themselves are made once, as before), so that
`textRev` leads to `rev`.

Other writers send their own base. The MCP `page_update` tool takes the `rev`
the agent read; its `edits` are applied to the file as it is, and a whole
`body` is rebased over the server's rewrites since that `rev`. An agent that
changes the title and then sends a whole body reads the page again first (its
answer carries the new `rev` only). The offline copy (`/sync/…`) only reads:
writing offline is not supported, so it saves nothing.

### `PATCH /api/spaces/{space}/pages/{id}/props`

Needs `write`. Changes properties of the header and nothing else: the body,
the built-in fields and the other properties stay exactly as the file has
them, and the body does not travel. The table view's cells use it.

```json
{"baseRev": "3b18e5…", "set": {"status": "done", "points": 3, "labels": ["a", "b"]}, "unset": ["old"]}
```

`baseRev` is required, and `set` or `unset` must name something. A value in
`set` is any JSON value a header can hold (text, number, `true`/`false`, a
list, an object). A value that is `null`, blank text or an empty list removes
the key, as `unset` does; `false` and `0` are values. A key that is new goes
at the end of the header; a key that is in the file keeps its place. A date
written `2026-11-22` (a real date, exactly `YYYY-MM-DD`) is written to the file
as a YAML date; text that only looks like one (`2026-13-45`, `2026-1-5`) is
written as a quoted string, and the file never gets an explicit `!!` tag. The keys are any
that are not built in (`id`, `title`, `subtitle`, `source`, `tags`,
`created`, `type`: save the page to change those) and not reserved (`publish`,
`slug`); at most 200 keys at once, none both set and unset. A change that
would leave the file as it is writes nothing and returns the same `rev`.

- `200` with the page (new `rev`).
- `409 conflict` when `baseRev` is not the file's current rev, as for `PUT`:
  `{"error": "conflict", "message": "…", "current": {/* page */}}`.
- `400 invalid` for a request without `baseRev` or without anything to change,
  a key that may not be changed this way, or a value that cannot be written.
- `403 no_permission` without `write`; `404 not_found` for a page that is not
  there.

The caller's draft of the page is handled as `PUT` does: kept, unless it says
exactly what the file now says. After a change it is usually stale, and the
app shows both versions when the page is opened. The change is journaled and
committed like a save, and in a space that follows a remote it is pushed
before it is answered.

### Drafts

While someone edits, the app does not rewrite the page file on every pause. It
stores a draft: one per page and user, kept in the server's database, not in
the space. Nobody else sees it, and lists and search keep showing the file.
A draft becomes the file's content, and a Git commit, when it is committed:

- the app asks for it (the user leaves the page or chooses to save a version);
- or the server does it by itself once the draft has not changed for
  2 minutes, or 10 minutes after the draft was first stored
  (`--draft-idle`, `--draft-max`). The server does it only while the author may
  still write in the space: a draft of someone who was removed, made a viewer or
  disabled in the meantime is not applied and not deleted. It stays in the
  database (`petalbook draft list|export` shows it), is looked at again after 10
  minutes, and is applied then if the person has the write permission again.

#### `PUT /api/spaces/{space}/pages/{id}/draft`

```json
{"prev": "k3f9…", "baseRev": "3b18e5…", "meta": {"id": "01JD3K8Q7Z", "title": "暮雪"}, "body": "…"}
```

Stores or replaces the caller's draft. `baseRev`, `meta` and `body` mean what
they mean in `PUT …/pages/{id}` and get the same checks, but the file is not
touched. `baseRev` is the rev of the file the edit started from.

Every stored draft has a `version`: an opaque string that is new each time the
draft is stored. `prev` is the version this store replaces: the one the client
last stored or loaded, or `""` (or absent) when it knows of no draft. The
store is refused when that is not what the server has, so the same person
typing on two devices cannot silently replace one draft with the other:

- `200` with `{"version": "p81c…", "baseRev": "3b18e5…", "updated": "2026-11-21T08:31:10Z", "stale": false}`.
  `stale` is `true` when the file's rev is no longer `baseRev` (and not only
  because of the server's link rewrites, see "Links between pages"): the draft
  is kept, but it cannot be committed as it is.
- `409 conflict` with `{"error": "conflict", "message": "…", "draft": {…} or null, "current": {/* page */}}`
  when the caller's draft on the server is not version `prev`: there is
  another one (`draft`, in the shape `GET` gives), or there is none any more
  (`draft` is `null`: it was committed or deleted in the meantime, and
  `current` shows what the file holds now). Nothing is stored.
- `404 not_found` when the page does not exist.

#### `DELETE /api/spaces/{space}/pages/{id}/draft`

`204`. Needs only `read`: whoever lost the `write` permission may still discard
their own draft (drafts are per user, so it is always the caller's). See below
for `?version=`.

#### `POST /api/spaces/{space}/pages/{id}/commit`

```json
{"version": "p81c…"}
```

Writes the caller's draft to the file, exactly as `PUT …/pages/{id}` with the
draft's `baseRev`, `meta` and `body` would, and removes the draft. `version`
is required: the draft is committed only if it is that one.

- `200` with the page and `textRev`, as `PUT …/pages/{id}` answers (see
  "Typing during a save").
- `409 conflict` with `current` (the page) and `draft` (the caller's draft on
  the server, or `null`), when the draft is not version `version` — including
  when there is none because the server committed it already — or when it is
  stale. Nothing is written and a draft that exists is kept. After a stale
  draft was refused the server stops trying to commit it by itself until it
  is stored again.

A client that gets `409` with `"draft": null` compares `current` with what it
meant to commit: equal means its draft is already the file.

To get past a stale draft the app shows both versions and then either deletes
the draft, or stores it again with the current rev as `baseRev`.

`DELETE …/draft` takes an optional `?version=`; when given and the caller's
draft is another version, or there is none any more (the server committed it
already: `"draft": null`, and `current` shows the file), the answer is
`409 conflict` as above and nothing is deleted. Without `version` the answer
is `204` whether or not there was a draft. With `version`, `&rev=` may be
added: the file rev the client believes the page is at. If the file is at
another rev the answer is the same `409` (with the draft and `current`) and
nothing is deleted, so a client that drops its draft because its text equals
the file finds out when the file is no longer the one it knows.

Deleting a page deletes its drafts.

### History

A version of a page is a commit that changed its file. The history follows
the page through renames and moves.

#### `GET /api/spaces/{space}/pages/{id}/history`

Query: `limit` (1–100, default 30), `cursor`.

```json
{"items": [{"commit": "9fceb02d0ae598e95dc970b74767f19372d61af8", "time": "2026-11-21T08:30:00Z",
            "author": "Kazuha", "path": "Tang/暮雪.md", "rev": "3b18e5…", "change": "edit"}],
 "next": ""}
```

Newest first. `author` is the name on the commit. `rev` is the rev of the
file in that version. `change` is `add`, `edit` (content changed), `move`
(only the path changed) or `delete`. A version in which the page was deleted
has no `rev`. `via` is `mcp` or `token` for a commit made through MCP or with an API
token (its `Changed-via` trailer, see "MCP"), and absent otherwise.

#### `GET /api/spaces/{space}/pages/{id}/versions/{commit}`

The page as it was in that version: `{"commit", "time", "author", "path",
"rev", "meta", "body"}`. `404 not_found` when that commit is not a version of
this page (it is not possible to read any other file through this endpoint).

In a space whose clone is partial (milestone 3.1), both endpoints may fetch
the contents of old versions from the remote first, and answer
`503 not_cached` (with `Retry-After`) when it cannot be reached; see "Server
storage", "History and versions".

Going back to an old version is an ordinary edit: the app loads the old
content into the editor and it is stored and committed like anything typed, as
a new version. History is never rewritten.

### Links between pages

A page links to another with an ordinary relative Markdown link to the other
page's file, with the target's id in the title position:
`[暮雪](../Tang/暮雪.md#小节 "01JD3K8Q7Z")`. Only links whose destination ends in
`.md` and stays inside the space count; links to the web, images, attachments
and anything in code do not. A title that is exactly an id is the id (the link
then follows the page by id); any other title is kept and the link is read by
its path alone. A link without an id is read by path and gets the id the next
time the page is saved, or when it is rewritten; a link whose id is that of a
page at another path gets that path when the page is saved (only the path
changes). A reference-style link (`[暮雪][r]` with `[r]: ../Tang/暮雪.md`
elsewhere in the page) counts too: its definition holds the path and the id,
and is what is rewritten.

Whenever a page is renamed (a new title in `PUT`), moved (`move`, `batch`, folder
move), the links of the space follow it in the same change, so in the same
commit (and the same push in a space that follows a remote): the files that link
to it have the path of those links rewritten (the text, the `#anchor`, the
other bytes of the file are kept), and so do the links of the moved page itself,
which are relative. If the server stops half way, the next start finishes it.
A page that is moved or renamed outside PetalBook (a pull, an edit in the
folder) has the paths of the links that carry its id put right by the server,
in one commit of its own; a link whose id two files claim, or no page has, is
left as it is, by a rename or move as well. Deleting a page changes no other
file: links to it become broken.

These rewrites are the server's, not a person's, so they never make anyone's
work conflict: a `PUT` page or a draft whose `baseRev` is behind the file only
by such rewrites is accepted as if it were current, with the same rewrites made
to its text (the old path is not written back), and such a draft is not
`stale`; `GET` page returns it with the rewrites made and the file's `rev`
as its `baseRev`. The server remembers the last 20,000 rewrites for this,
across restarts; a draft older than all of them is shown as stale as before.
Reading these needs `read`; the rewrites need nothing of their own (they are part
of the write the caller is allowed).

#### `GET /api/spaces/{space}/pages/{id}/backlinks?cursor=`

The pages that link to this one, the most recently changed first, 20 at a time:

```json
{"items": [{"id": "…", "path": "Notes/引用.md", "title": "引用", "untitled": false,
            "excerpt": "见暮雪 和无"}],
 "next": "", "total": 1}
```

`excerpt` is the paragraph the link is in, as plain text, cut to about 240
characters around the link (`""` when the file no longer has the link, and
while the space is evicted: see "Eviction"). A page
that links several times is listed once. `next` is `""` at the end, else the
`cursor` of the next request; a `cursor` that was not handed out is `400`. `404`
for an unknown page.

#### `GET /api/spaces/{space}/pages/{id}/links`

The links of this page, in the order of the text, for marking broken ones and
for hover text:

```json
{"items": [{"text": "暮雪", "path": "Tang/暮雪.md", "id": "01JD…", "anchor": "小节",
            "broken": false,
            "target": {"id": "01JD…", "path": "Tang/暮雪.md", "title": "暮雪", "untitled": false}},
           {"text": "无", "path": "Notes/无.md", "broken": true}]}
```

`path` is the file the link names, from the root of the space. The target is
found by `id` when the link has one, else by `path`; `title` is the target's
current title (or what stands for it, as in a page summary). `broken` is true,
and there is no `target`, when no page is there.

### `POST /api/spaces/{space}/pages/{id}/move`

```json
{"folder": "随笔/2026"}
```

Moves the file, creating the folder if needed. Returns the page.

### `DELETE /api/spaces/{space}/pages/{id}`

`204`. The file is removed from the working tree; it remains in Git history.

### `POST /api/spaces/{space}/pages/batch`

Moves or deletes several pages as one change: one commit, and in a space
that follows a remote one push, or nothing at all. In a local space a failure
half way (a full disk) leaves the pages handled so far moved or deleted; what
can be refused beforehand (an unknown id, a bad folder) is refused before
anything changes.

```json
{"op": "move", "ids": ["01JD3K8Q7Z", "01JD3K8Q80"], "folder": "随笔/2026"}
{"op": "delete", "ids": ["01JD3K8Q7Z"]}
```

`ids` holds 1 to 500 page ids, each once. `200` with `{"pages": 2}`: how
many were moved or deleted. A page that is already in `folder` counts and is
left where it is. `404 not_found` when one of the ids is not a page, and
`400 invalid` for a bad `op`, a list that is empty, too long or names an id
twice, a `move` without `folder` (`""` is the top level), or a `folder` that
cannot be one: in both cases nothing is changed. Deleting a page also removes the drafts of it, as deleting one page
does.

## Page types

A page type says which properties the pages of a kind have. It is a file of
the space, `.petalbook/types/{name}.json`, a JSON Schema of which the server
understands a small part. The file name (without `.json`) is the **name** of
the type, the value of `type` in a page's header; it never changes once the
type is made. The `title` in the file is the name people see and can change.
The properties are the keys of `properties`, and their order in the file is
their order everywhere.

Each field has a **kind**, which the server recognises from its schema:

| kind | schema |
|---|---|
| `text` | `{"type": "string"}` |
| `longtext` | `{"type": "string", "x-multiline": true}` |
| `select` | `{"type": "string", "enum": […]}` |
| `multiselect` | `{"type": "array", "items": {"type": "string", "enum": […]}, "uniqueItems": true}` |
| `date` | `{"type": "string", "format": "date"}` |
| `number` | `{"type": "number"}` (`"integer"` is read as a number too) |
| `checkbox` | `{"type": "boolean"}` |
| `url` | `{"type": "string", "format": "uri"}` |
| `other` | anything else: shown and edited as the YAML in the header, never rewritten |

A field may have a `title`. Everything else in a schema (other keywords of a
field, `$schema`, `description`, `required`, …) stays in the file as it was,
and the server changes only the keys it knows when it writes one. A value that
does not fit its field's schema is saved all the same: nothing here checks
page values.

A folder is bound to a type in `.petalbook/space.yml`:

```yaml
folders:
  Tang:
    type: 文案
```

A folder takes the type of the nearest folder above it that is bound (the
root, `""`, may be bound too); a page whose header has a `type` has that.
Renaming or moving a folder changes its bindings, and those of the folders in
it, in the same commit; deleting one removes them (and its views, see Saved
views). Other settings an entry has stay. The move and the change of the bindings are one change: if the server
stops between the two, it finishes the bindings when it starts again, and if
the bindings cannot be written the folder is not moved and the request fails
(also when `space.yml` is so full that the new key would make it larger than the
1 MiB the server reads). Whether the settings can follow is found out before
the folder is touched: a move or a delete whose edit of `space.yml` cannot be
made is `400 invalid` with nothing renamed and nothing deleted, and the message
says which it was ("the folder was not moved: …", "the folder was not
deleted: …"). For a delete that is found out for the folders that will be
gone afterwards: a folder that holds files that are not pages stays, and so
does its entry. Should the folders turn out otherwise than was worked out
before the delete (a file came to lie in one meanwhile) and the settings then
cannot be taken out as the folders are, they are taken out as was worked out.
Moving or deleting a folder removes entries of `space.yml`; what
that does to anchors and aliases is under Anchors, aliases and merge keys below.
A move that a crash interrupted and whose
bindings then do not fit in `space.yml` is taken back at the next start: the
folder goes back to its old name, and the error is logged. A `space.yml` that
cannot be used at the next start is restored there as it is by any other
change (see Settings files that cannot be used), and the space opens.
Nothing was at the place a folder is moved to, so every binding at that place
and under it is stale and is dropped (the moved folder takes the bindings it
had, and none it did not); the journal of the move is kept until the bindings
are done, so a crash at any point, also while a failed change in a space that
follows a remote is undone, is finished or taken back with folder and bindings
agreeing. If the move fails and is taken back, the parent folders it made for
the target are removed again. The same place, written twice, is its last entry; a folder written twice in `space.yml` counts as its last entry, and
removing a binding removes all of them. Comments in `space.yml` and `tags.yml`
stay, also in a file that has nothing else.

Limits: a space has at most 100 types; a type file is at most 256 KiB and has
at most 200 fields; a field key is 1 to 64 bytes, has no control character
and no `:`, and is not `id`, `title`, `subtitle`, `source`, `tags`, `created`,
`type`, `publish` or `slug`; a select has 1 to 500 options of at most 200
bytes each, none blank, none twice; titles are one line of at most 200 bytes;
a type name is 1 to 100 bytes, not starting with a dot or ending in a dot or
space, with none of `< > : " / \ | ? *`; two names that differ only in case
are one name. The key rules are for a field that is new: a field the file
already has keeps its key whatever it is (written by hand, say), and a request
may list it as it is.

#### Settings files that cannot be used

`space.yml`, `tags.yml` and the type files are written by hand too, and come in
with a pull. A type file that cannot be used (not JSON, not an object, its
`properties` not an object, more than 200 fields, too large, not a regular
file) is treated as below, like the two settings files: it is read as its
newest version in the history that can be used (with none, there is no such
type), the next change of the type writes it and keeps the file as it was as
`.petalbook/types/{name}.json.unreadable-{time}`, and so does deleting the
type. A field whose schema
is not an object is kept as it is until a request gives the field a kind;
then the schema is written anew and the file as it was is kept.

A settings file that cannot be used (it is not YAML with keys at the top, has a second
document, its `folders` or `tags` is not a list of entries, an alias is inside
its own anchor, it is over a limit below, larger than 1 MiB, or not a regular
file) is nobody's to fix before the space works again, and no request is
refused because of it:

- **Reading.** The settings are those of the newest version of the file in the
  history of the space that can be used, looked for among the last 50 commits
  that changed the file, in the order of the commits (the clocks of the
  machines that made them are never compared); with none, the space has no
  settings, like one that never had the file. What the server wrote itself
  and has not committed yet counts as newer than any commit, until it is
  committed. Onto that version goes what can still be read of the file as it
  is, entry by entry, the file winning for each entry it has: in a file that
  is YAML but cannot be used, every top-level key and every entry of
  `folders` or `tags` that can be used by itself (with its aliases written
  out); in a file that is not YAML, every top-level key, and every entry of
  `folders` or `tags`, that reads by itself when the text is cut by its
  indentation (a leading byte order mark is not part of the first key, and
  the lines of a quoted value that runs over several lines stay with it,
  wherever they start, so a line `folders:` inside a value never makes up
  entries; a quote never closed is a typo and closes nothing). This takes
  time in proportion to the file, also at the 4 MiB most that is read of
  it. So thirty new bindings and one typo keep the thirty; only the
  entry with the typo is not read (`lost` in the notice), and it is in the
  file and in the copy kept of it. A type file is one JSON document and is
  read as the version restored. Bindings, views and colours are read from
  those settings until the file is put right. What was found is remembered for
  the file as it is and the newest commit that changed it, so the history is
  searched once, not for every request, and not again for commits of other
  files.
- **Repair.** The next change that writes the file (a binding, views, a
  colour, a folder with settings moved or deleted, a type deleted) is made on
  those settings and written in its place. In the same commit the file as it
  was is kept beside it as `.petalbook/space.yml.unreadable-{time}` (or
  `tags.yml.unreadable-{time}`; the time is UTC, `20261122T163159Z`), so
  nothing is lost. A read writes nothing, so it makes no commit and, in a
  space that follows a remote, no push; a change that needs nothing written
  (a folder without settings is moved or deleted) leaves the file as it is.
  A content that is already kept is not kept again: a pull that brings the
  same file back adds no second copy.
- **Folders.** Moving or deleting a folder is never refused because of such a
  file, and never leaves settings behind: they follow the folder, or go with
  it, in the restored settings.
- **After a crash.** The start that finishes an interrupted move or delete
  goes the same way: it restores, writes and keeps the copy, and commits both.
- **A link or a file in the place of a folder.** Nothing is ever read or
  written through a link, or a file, where `.petalbook` or `.petalbook/types`
  belongs (a pull can bring one, or a person put it there), wherever it leads.
  The settings files and types behind it read as restored from the history
  above (the commit that put the link there does not count as deleting them),
  and the types listed are those the folder had. The next change that writes
  there moves what is in the place of the folder aside, inside the space, as
  `.petalbook.unreadable-{time}` (or `.petalbook/types.unreadable-{time}`),
  commits that, makes the folder again with everything it had (the settings
  as they are read, the other files as they were), and makes the change in
  it. Every read and write there is made in the folder as it was opened, so
  a link put in its place while a file is being read or written is not
  followed either. In a space that follows a remote, that repair is pushed
  before the change that needed it; when its push does not go through (the
  remote moved on, or cannot be reached) it is taken back, the clone is as
  the remote again, and the change is made again on the new tip (or fails as
  any change fails without the network), so a repair is never kept as a
  change made here that was not pushed. A local space puts a link where
  `.petalbook` belongs right that way when it starts, before it writes
  `space.yml`.
- **The server's own files are not in `.petalbook`.** `.petalbook` holds the
  settings and the types only: repository content. The index, the journal of
  what is not committed yet, the notices below and, in single-user mode, the
  state database are kept in the Git directory (`.git/petalbook-state/`;
  `--state-dir` puts the state elsewhere), where no pull or checkout writes,
  so a pull that brings a link where `.petalbook` was takes none of it away.
  The Git directory is the one git uses for the space: for a checkout made
  with `git worktree add` its own one (two checkouts of one repository never
  share state), and for a submodule, a `--separate-git-dir` repository or a
  `.git` link, the folder their `.git` names; it may be on another file
  system than the space. A file of the space (a page, an attachment, a
  setting) is written in its own folder under a new hidden name
  (`.petalbook-w-<random>`, made exclusively, never through a link) and then
  renamed into place, so a save works wherever the Git directory is and
  whatever a pull put anywhere else. Those names are not pages, are kept out
  of the repository by `info/exclude` and are never committed; one a crash
  left is removed when the space starts, unless the repository has a file of
  that name.

What happened is told in `settingsNotices` of `GET /api/spaces/{space}/types`,
to callers who have `manage` in the space (in single-user mode, always); for
anyone else the key is absent. Nobody is asked to do anything:

```json
{"settingsNotices": [
  {"file": ".petalbook/space.yml", "unreadable": true,
   "since": {"commit": "9f2c…", "time": "2026-11-22T16:20:11Z"},
   "restoredFrom": {"commit": "41ab…", "time": "2026-11-20T09:02:40Z"}},
  {"file": ".petalbook/tags.yml", "unreadable": false,
   "since": {"time": "2026-11-21T08:00:00Z"},
   "restoredFrom": {"commit": "77d0…", "time": "2026-11-19T12:00:00Z"},
   "copy": ".petalbook/tags.yml.unreadable-20261121T081502Z",
   "repairedAt": "2026-11-21T08:15:02Z"}
]}
```

`unreadable` is `true` while the file in the space is still the one that cannot
be used (the server reads the restored settings, and the next change puts the
file right), and `false` for a file that was put right. `since` is the oldest
version in a row that could not be used (without `commit`: the file was changed
in the folder and is in no commit; `time` is then when it was changed).
`restoredFrom` is the version the settings were taken from; it is absent when
the history has none that can be used and the settings started empty, and has
no `commit` when they are what the server itself wrote last and had not
committed yet. `lost` lists the entries of the file as it is (was) that could
not be read even one by one, so they are not in the settings in use and are
only in the file and its copy (`"*"`: nothing of it could be read, as for a
file more than four times as large as the server reads); it is absent when
everything that could be read was kept. `copy` is where the file as it was is
kept. A link or a file that was in the place of `.petalbook` or
`.petalbook/types` has a notice with that `file`, `entry`
`"not a folder"` and its `copy`. A notice of a
repair stays for as long as its copy is in the space: deleting the copy puts
the notice away. `since`, `restoredFrom` and `repairedAt` of a repair are kept
by the server that made it; a copy that came with a pull from another server
is listed with `file` and `copy` only. The list is empty, or absent, when
there is nothing to tell.

One entry of a file that reads can be unusable by itself: an entry that is
not a list of settings (`Tang: yes`), `views` that is not a list, a `type` or
`color` that is not text. It reads as not there, everything else in the file
works, and the change that puts something in its place (binding the folder,
saving its views, setting the colour) writes over it and keeps the file as it
was, as a copy like the ones above. Such a notice has `entry`: `"Tang"` for the
entry, `"Tang: views"` or `"Tang: type"` for a setting of it (`"properties: {key}"`
for a field of a type), with `unreadable` `true` until it is written over and
`false`, with `copy` and `repairedAt`, afterwards.

#### Anchors, aliases and merge keys

`space.yml` and `tags.yml` are written by hand too, and come in with a pull, so
they may hold YAML anchors (`&name`), aliases (`*name`) and merge keys (`<<`).
One rule holds for both files, for everything the server reads from them and
every change it makes to them:

- **Reading.** An alias means what its anchor means, and a mapping with a merge
  key has the keys it merges, wherever the server looks: `folders` and `tags`,
  an entry, its `type` or `color`, its `views`, a view, a filter, a value. With
  `A: {type: &t wenan}` and `B: {type: *t}`, both folders are bound to `wenan`.
- **Cost and limits.** What an anchor means is worked out once, however many
  aliases refer to it, so reading a file costs what is written in it: an entry
  that ten thousand folders share by alias (`Tang: *chapter`) is read as fast
  as one written once, and no number of aliases makes a file unusable. The
  only limits are ones a file that a person keeps does not reach: 100 levels
  deep with the aliases followed, no alias inside its own anchor, and
  4 194 304 keys brought in by merge keys (each mapping with a merge key
  counted by itself: ten thousand folders that each merge a template of four
  hundred keys). A file over one of them cannot be used; see Settings files
  that cannot be used. That is found when the file is loaded, in time that
  grows with its size only.
- **Writing.** A change changes what it targets and nothing else. An entry, a
  list of views or a view that is an alias, carries an anchor or has keys from
  a merge key is first written out in its own place (aliases inside it stay
  aliases), and the change is made there; what else refers to the anchor keeps
  what it had. When the node that carries an anchor is removed or replaced (a
  folder deleted or moved onto, a view removed, a type or colour changed) and
  aliases of it remain, the anchor moves to the first of them and the others
  refer to it there; nothing is copied. Deleting `A` above leaves `B: {type: &t
  wenan}`. The comments of a merge key that is written out stay where it was:
  the one above it above the first key written in its place, the one on its
  line on the line of that key, the one below it below the last of them.
- **Check.** Before the file is written the server checks that every alias
  left in it stands for what it stood for, and that the text reads back as
  what was meant. If not (an anchor name used twice can do that), the change is
  made again with every alias of the file written out. If that does not fit in
  1 MiB the request is `400 invalid` and nothing is changed, on disk or in the
  file; no more is written out to find that than 1 MiB holds.
- **Shared entries stay shared.** When one change takes a setting off many
  entries that are aliases of one anchor (a type is deleted that ten thousand
  folders have through `Tang: *chapter`), the first of them gets what the
  anchor is without that setting, under an anchor of its own (`&chapter-no-type`,
  with a number after it if that name is taken), and the others become aliases
  of that: the file stays as small as it was, and what else refers to the old
  anchor keeps what it had. These are the only aliases the server writes.
  Entries that share through a merge key (`{<<: *chapter}`) are written out
  each for itself, and such a change is `400 invalid` when the result would not
  fit in 1 MiB.

### Type

```json
{
  "name": "task",
  "title": "任务",
  "rev": "9c2f…",
  "fields": [
    {"key": "status", "title": "状态", "kind": "select", "options": ["todo", "doing", "done"],
     "schema": {"title": "状态", "type": "string", "enum": ["todo", "doing", "done"]}},
    {"key": "points", "title": "", "kind": "number", "schema": {"type": "number"}}
  ]
}
```

`rev` is the Git blob id of the type's file, as the `rev` of a page is: it
changes whenever the file does, and a change or a delete of the type is made
from it (`baseRev`, below). A file that cannot be used is the type of its last
version that can, with that version's `rev`. `title` is `""` when the file gives none or gives
something that is not text (show the name or key); a `title` that is not text
stays in the file when a request leaves the title empty. `options` is
there for `select` and `multiselect`. `schema` is the field's schema as the
file has it. A type listed as
`{"name": "…", "title": "", "fields": [], "error": "…"}` is a file beyond the
first 100, or one that cannot be used while the history could not be read for
its earlier version just now.

### `GET /api/spaces/{space}/types`

Needs `read`.

```json
{"types": [/* types, in name order */], "bindings": [{"folder": "Work", "type": "task"}]}
```

`bindings` are all the folders that are bound, in the order of the file.
`settingsNotices` is there when a settings file could not be used (see Settings
files that cannot be used). Only
the first 100 type files are read; more than that (put there by hand) are
listed with an `error`.

### `PUT /api/spaces/{space}/types/{name}`

Needs `manage`. Makes the type, or changes it. The request is the whole of
what the type is to be:

```json
{"baseRev": "9c2f…", "title": "任务", "fields": [
  {"key": "status", "title": "状态", "kind": "select", "options": ["todo", "doing", "done"]},
  {"key": "points", "kind": "number"}
]}
```

`baseRev` is the `rev` of the type the request was made from, and `""` or
absent to make a type that is not there yet. When the type is not as that says
(changed, made or removed since) nothing is written and the answer is `409
conflict`.

The fields are the type's fields, in this order: one the type has that is not
listed is removed from the type (pages keep the values; the app shows them as
not in the type), and a `title` that is left out or empty removes the title.
The server merges this into the file: a field that exists and keeps its kind
has only its `title` and options changed, a field whose `kind` changes gets
the keys of the new kind and keeps its others, a field that is new is made.
`kind` may be left out for a field that exists (it keeps its kind) and is
required for a new one; `other` cannot be made, and a field cannot be changed
to it. `options` goes with `select` and `multiselect` only. Renaming a field
is removing one and adding another; the pages are not rewritten. `200` with
the type.

Errors: `400 invalid` for anything above that is out of limits or malformed;
`409 conflict` when `baseRev` is
not the file's current rev, or the type is there and `baseRev` is empty, or
the type is not there and `baseRev` is not: `{"error": "conflict", "message":
"…", "current": {/* type */}}`, `current` being the type as it is now (absent
when it is gone); `409 exists` when another name
differs from `{name}` only in case. The change is journaled and committed like
any other.

### `DELETE /api/spaces/{space}/types/{name}?baseRev=…`

Needs `manage`. Removes the file and, in the same commit, every binding to the
type. `baseRev` is the `rev` of the type the request was made from, as for
`PUT`; `409 conflict` (with `current`, as there) when it is not the file's
current rev. For a type whose file is gone and only bindings are left, it is
`""`. The headers of pages that name the type are not touched. `204`;
`404 not_found` when there is no such type and no folder is bound to it. The
file of a type that cannot be used is kept as a copy when the type is deleted
(see Settings files that cannot be used); with no version that can be used it
is no type, and is deleted with `baseRev` `""`.

### `PUT /api/spaces/{space}/folders/type`

Needs `manage`.

```json
{"folder": "Work", "type": "task"}
```

Binds a folder (and, by inheritance, the folders below it) to a type, or
removes the binding when `type` is `""`. `200` with `{"folder": "Work",
"type": "task"}`, the folder as it is spelled on disk. `404 not_found` when
the folder does not exist; `400 invalid` when the type does not exist, or the
entry of the folder in `space.yml` is not a list of settings. Pages are not changed: their
`effectiveType` is.

### Icons, colours and the presets of the sources (3.10)

A folder has an icon and a colour, written under its entry in
`.petalbook/space.yml` next to its type and views, and a page has `icon` and
`color` in its header (fields of every page, not properties of a type). An icon
is the name of a Lucide icon the web app ships (`book-open`; the list is
`internal/icons/lucide.txt`) or one emoji, one grapheme cluster of at most 64
bytes with no control character, space or markup (`<`, `>`, `&`, quotes); a
colour is one of eight: `red`, `orange`, `amber`, `green`, `teal`, `blue`,
`purple`, `pink`. A folder style, a page save or create, and a type's default
that is none of these is `400 invalid`. An icon or colour that is none, read
from a `space.yml`, a header or a type file written by hand, is read as no icon
(no colour), and a save that leaves such a value in a header as it was is not
refused for it. The
entry of a folder moves with the folder, in the same commit (see Moving a
folder), so its icon and colour follow it.

A page type may give the default icon of its pages: `x-icon` and `x-color` at the top of its file,
answered as `icon` and `color` of the type, and written by `PUT …/types/{name}` with `icon` and `color`
(left out keeps what the file has; `""` takes it away). A page with no icon of its own is drawn with its type's.

`GET /api/spaces/{space}/appearance` (read) answers
`{"folders": [{"folder", "icon", "color"}], "sourcePresets": [...] | null,
"iconColors": [...]}`. `sourcePresets` is the list the space chose under
`sources:` in `space.yml`; `null` when it chose none, and the client offers its
own defaults; `[]` when it chose to have none (`sources: []`), and the client
offers none.

`PUT /api/spaces/{space}/folders/style` (write) takes
`{"folder", "icon", "color"}`; `""` takes one away. `200` with the same,
the folder as spelled on disk; `404 not_found` for a folder that is not there;
`400 invalid` for a colour that is none of the eight or an icon that is none (see above).

`GET /api/spaces/{space}/sources` (read) answers
`{"sources": [{"source", "pages"}]}`, the sources the pages use, most used first
(at most 500), for the suggestions of the input.

`PUT /api/spaces/{space}/sources/presets` (manage) takes `{"presets": [...]}`,
or `null` to go back to the defaults; blank and repeated ones are dropped, at
most 100 of at most 200 bytes. `200` with `{"presets"}` as stored: `[]` for an
empty list (none), `null` for the defaults.

`POST /api/spaces/{space}/types/{name}/rename-field` (manage) takes
`{"baseRev", "from", "to"}` and gives a field of a type another key: in the
type's file and, in one commit, in the header of every page of that type that
has a value under the old key (the field keeps its place in the header), and in
the saved views of the folders of the type (their `sort` `prop:<key>`, `group`,
the `key` of their filters, `hidden` and the fields of a card). The views of
tags are left as they are: a tag's pages may be of other types. The change is
journaled whole before the first write and the type's file is written first,
so a crash part way leaves either nothing changed or the rest finished at the
next start.
`200` with `{"type", "pages", "skipped"}`: `pages` is how many were rewritten,
`skipped` the pages of the type left as they were, by path, each
`{"id", "path", "reason"}`: `has_key` for a page that already has a value under
the new key (both keys stay, for the person to merge), `unreadable` for a file
of a folder of the type whose header cannot be read (it has no `id`). `404` for
a type or field that is not there, `409 conflict` for a stale `baseRev`, `400
invalid` when the type has a field of the new key or the key is one of every
page, or when the header of a page cannot be written with the new key (the
message names the page; nothing is changed).

## Saved views

A view is a saved way of looking at the pages of a folder or of a tag: a
layout, a sort, filters, the field a board is grouped by, the hidden columns of
a table. A folder's views are written under its entry in
`.petalbook/space.yml`, next to its type; a tag's under its entry in
`.petalbook/tags.yml`, next to its colour. The root folder is the key `""`.

```yaml
folders:
  文案:
    type: wenan
    views:
      - id: k3f9
        name: 按状态
        layout: board
        group: status
        sort: -prop:written
        filter: [{key: series, op: in, values: [冬季]}]
```

The first view is the default one; a folder or tag with none shows the
implicit default. Moving or renaming a folder carries its views, and those of
the folders in it, in the same commit and the same journal record as its
bindings (a crash in between is finished when the server starts); deleting a
folder removes them (a folder that stays, because it holds files that are not
pages, keeps them). Deleting a type keeps the views of the folders that were
bound to it; unbinding a folder keeps its views. Comments, keys the server does
not know (in a view, in the entry) and other settings stay in the file: a
view that is saved again is changed in its own YAML. The settings files are
read and written the way bindings and tag colours are (a file that cannot be
used is restored from the history, see Settings files that cannot be used),
and a change is journaled and committed like any other, in a space that
follows a remote too. There is no tag rename in the API, so nothing carries a
tag's views to another name.

### View

```json
{"id": "k3f9a1", "name": "按状态", "layout": "board", "sort": "-prop:written", "group": "status",
 "filter": [{"key": "series", "op": "in", "values": ["冬季"]}], "hidden": []}
```

Every key is always there when the server answers (`sort` and `group` as `""`,
`filter` and `hidden` as `[]` when empty). `id` is made by the server: six
lower-case letters and digits, unique in the space (folders and tags together).
`name` is one line of 1 to 200 bytes. `layout` is `list`, `cards`, `table`,
`board` or `read`. `sort` is `""` (the default order of the listing), `updated`,
`created`, `title` or `prop:{key}`, each with an optional leading `-`, as for a
listing. `filter` is written as the `filter` of a listing (at most 10, each
checked the same way, `notIn` included). `group` is the field a board is
grouped by, at most 256 bytes; it is kept whatever the layout. `hidden` are the
keys of the columns a table hides, each once, at most 200. A key that is not
one of these is kept as it is (`"color": "teal"` comes back) and is written to
the file; the server does not look at its meaning, but it limits its size: at
most 16 such keys, each value at most 4 KiB as JSON, and a view at most 16 KiB
as JSON (the views a request changes, 128 KiB together). A view that a request
gives back exactly as it is stored is not checked again.

Views written by hand are read so that a client can work with them: a view
whose `id` is missing, malformed or repeated has the id `@{n}`, `n` its
position (from 0) in the folder's list, and a missing or unknown `layout` reads
as `list`; every view in an answer therefore has a unique id and a layout. A
write that names such an id changes that view and gives it a new one. Only
the first 20 views of an entry are read (the rest of a longer list written by
hand stays in the file, after the views, and is not in any answer). A value
of a key the server does not know that stands for more than 10 000 YAML nodes
(aliases counted each time they are used) is left out of the answer, and stays
in the file. Aliases and merge keys are read as what they stand for (see
Anchors, aliases and merge keys).

### `GET /api/spaces/{space}/views?folder={path}` or `?tag={tag}`

Needs `read`. Exactly one of `folder` (the root is `folder=`) and `tag`.

```json
{"views": [/* views, the first being the default */], "rev": "6a6d14f7db61e4f5578c38cad4f75dfc5a97a9a6"}
```

`rev` is a digest of the views as the server reads them (a list with none has
one too), so a change to the file by hand is a new rev as well. A folder that
does not exist has no views (it is not looked for on disk). A settings file
that cannot be used gives those of its last version that can. `400 invalid` for neither or both of `folder`
and `tag`, or a path that cannot be a folder.

### `PUT /api/spaces/{space}/views`

Needs `write`: views are a shared way of arranging pages, and whoever can write
pages can change them. A member who can only read can switch between views and
change the sort and filter for themselves; they cannot save them.

```json
{"folder": "文案", "baseRev": "6a6d…", "views": [/* the whole list */]}
```

`folder` (or, instead, `tag`) says whose views; `baseRev` is the `rev` of the
views the request was made from; `views` is the whole list the folder or tag is
to have, in order (`[]` removes them all, and the entry of the folder when
nothing else is left in it). A view with an `id` must be one of the views the
folder or tag has now (`@{n}` ids included, see above); one without gets a new id. A view that keeps its `id`
keeps what the request does not mention: its comments, and the keys the server
does not know that the request leaves out. A view that is not listed is
removed.

`200` with `{"views": […], "rev": "…"}`, the views as they are now. When the
views are not as `baseRev` says nothing is written and the answer is `409
conflict` with `current` the same object as a `GET` answers:
`{"error": "conflict", "message": "…", "current": {"views": […], "rev": "…"}}`.
`400 invalid` for a missing `baseRev`, `views` or scope, both a folder and a
tag, more than 20 views, any view out of the limits above, an `id` the folder
or tag does not have, an `id` twice, a change that would make `space.yml` or
`tags.yml` larger than the 1 MiB the server reads (nothing is written, so the
settings stay readable), or an entry that is not a list of settings. A view
that carries an anchor, or is written with an alias or a merge key, can be
changed and removed like any other: what else in the file refers to the anchor
keeps what it means (see Anchors, aliases and merge keys). The body of the request is
at most 20 views at their largest and 512 KiB more (`413` above that), and the
list is refused as soon as it has a 21st entry, before any of them is built.
A view that the request gives back as it is stored is left as it is in the file,
also what the server could not read of it (a scalar where a list
belongs), and what is written as an alias stays one; a key is removed or rewritten only when the request changes it. `[]` removes
the views from every writing of the folder in `space.yml`, so none comes back.
`404 not_found` for a folder that does not exist (a tag need not be in use).
A write that changes nothing writes nothing.

## Home: the daily text and favourites

### The daily text

A space shows one short text a day on its home page. Where it comes from is
decided, first match wins, by:

1. **the space's own source**, written under `daily` in `.petalbook/space.yml`
   (it is part of the space and is read and written like the bindings and the
   views: a file that cannot be used is restored from its history, anchors and
   aliases are kept, a change is journaled and committed like any other);
2. **an external interface the space chose** among those the administrators
   set up (accounts mode);
3. **the server's default**, which administrators set, unless the space opted
   out (accounts mode).

With none of them the answer is `{"page": null, "reason": "no_source"}` and the
block is not shown. A space with a source of its own that meets no page is
`no_match` (it does not fall through: it was asked for). Single-user mode has
only the first.

```yaml
daily:
  folder: 文案        # or tag: … or type: …, one of the three
  filter: [{key: status, op: in, values: [定稿]}]   # optional, as the filter of a listing
```

`folder` means the folder and the folders below it; `type` is the effective
type (the header's, else the folder's, see Page types). **By language:**
`daily: {zh: {folder: 文案}, en: {tag: quotes}}`. A source written beside the
language keys (`daily: {folder: x, zh: {…}}`) is "all languages": the fallback
for the languages that have none of their own; written alone it is the source
of every language. For a language the source is that of the language, else
that of the language without its region (`zh` for `zh-CN`), else "all
languages", else, so that a language nobody wrote a source for still gets a
text, the first by the order of the codes. Two spellings of one language
(`zh:` beside `zh-Hans:`) are read as the one under the canonical key, and the
next save writes that one key only. Moving or renaming a folder carries the `folder` of
every source with it, in the same commit as its bindings (and, after a crash,
at the next start). Deleting a folder leaves the source as it is; it then meets
no page.

#### `GET /api/spaces/{space}/daily/settings`, `PUT …/daily/settings`

`GET` needs `read`, `PUT` needs `manage`.

```json
{"daily": {"folder": "文案", "filter": [{"key": "status", "op": "in", "values": ["定稿"]}]}}
{"daily": {"zh": {"folder": "文案", "filter": []}, "en-us": {"tag": "quotes", "filter": []}}}
{"daily": {"folder": "all", "filter": [], "zh": {"tag": "诗", "filter": []}}}
{"daily": null}
```

`PUT` takes the same object and answers with it as it is now; `null` removes
the source. Language codes are lower-cased. `400 invalid` for none of
`folder`, `tag`, `type` or more than one, an unknown key, a tag that is no tag,
a type that does not exist, a filter that is not valid (as for a listing, at
most 10), a language key that is not a code (`zh`, `en`, `zh-CN`) or given twice, more than
20 languages, or a body without `daily`; `404 not_found` for a folder that is
not there. A source that is already as asked writes nothing.

#### `GET /api/spaces/{space}/daily?date=2026-11-23&skip=0&lang=zh`

Needs `read`. `date` is the day in the browser's time zone (today in UTC when
left out); `skip` (0 up to 1 000) asks for another text than the day's and
only affects what the asker is shown; `lang` is the language of the interface
(`zh`, `en`, `zh-CN`; left out, no language); `tz` is the browser's offset in
minutes east of UTC (`-480` for UTC-8, from -840 to 840).

The answer carries `Cache-Control: private, no-cache` and an `ETag`, and a
request with a matching `If-None-Match` gets `304` with no body. A browser
keeps the answer but asks again every time, so a change to what the
administrators or the space chose (sources, languages, providers) shows at
once and not at the end of the day. Asking is cheap: the answer for a period
is kept by the server, and one that has not changed is a `304`.

From the space's own source (and that is also how a page of this space is
returned):

```json
{"page": {"id": "…", "path": "文案/暮雪.md", "title": "暮雪", "label": "暮雪", "…": "(a page summary)",
          "body": "《暮雪》｜…\n", "bodyTruncated": false},
 "count": 12, "origin": "space"}
```

`page` is a page summary (see Page summary) with the Markdown `body` (only
that one file is read) cut to 2 000 characters (`bodyTruncated` says it was).
`count` is how many pages meet the source. Which one is picked: a number is
made from the space and the day (the first 8 bytes of a SHA-256) plus `skip`,
and the page is the one at that number modulo `count` among the pages that meet
the source, in the order of the index; both are read from the index (the
cost is the pages that meet the condition, tens of milliseconds for 50 000
pages). What a day showed is remembered in memory (per source, day and `skip`),
so pages that are added or deleted during the day do not change it; a page that
is gone is replaced by another pick. The picks everybody sees (`skip` 0) and the
others are remembered apart, 512 of each in a space; when either is full, the
half whose days are farthest from today is forgotten, so asking for other texts
or other days never changes today's.

From the administrators' settings the answer has no page but a `quote`:

```json
{"page": null, "quote": {"text": "…", "source": "…", "author": "…", "link": "https://…",
                         "fetched": "2026-11-23T08:00:00Z", "stale": false}, "origin": "external"}
{"page": null, "quote": {"text": "…", "source": "…"}, "origin": "site"}
```

`origin` is `external` for an interface, `site` for the default (a line the
administrators pasted, or a page of the space they named: its body as `text`,
its title as `source`; shown to everybody, members of that space or not).

Nothing to show: `{"page": null, "reason": "no_source" | "no_match" | "unavailable" | "out_of_range"}`;
`unavailable` is an interface that did not answer or a default space that is gone;
`out_of_range` is a `date` an interface is not asked about: only today,
yesterday and tomorrow are (today where the browser is, by `tz`; in UTC
without it), since each other day would be a request and a row kept. A client
hides the block for any of them. `429 rate_limited` (with `Retry-After`) when
another text from an interface is asked for too often. `400 invalid` for a
`date` that is not a day (`YYYY-MM-DD`) from 1970 to 2261, a `skip` that is not
a number from 0 up, or a `lang` that cannot be; `400 skip_too_large` for a
`skip` above 1 000 (see "Daily text refusals").

#### `GET /api/spaces/{space}/daily/prefs`, `PUT …/daily/prefs`

Accounts mode only (single-user mode answers `404`). `GET` needs `read`, `PUT`
needs `manage`: the owner of a space chooses what it takes of the
administrators' settings. Owners never enter addresses; they choose among the
interfaces that are set up.

```json
{"inherit": true, "apis": ["hitokoto"], "options": {"hitokoto": {"categories": ["d", "i"]}},
 "available": [{"id": "hitokoto", "name": "一言", "lang": "zh-Hans", "languages": ["zh-Hans", "zh-Hant"],
                "options": [{"key": "categories", "param": "c", "kind": "multi",
                             "values": [{"value": "a", "label": {"zh": "动画", "en": "Anime"}}, "…"]}]},
               {"id": "quotes", "name": "Quotes", "lang": "en"}],
 "languages": ["zh-Hans", "en"], "allowed": {"own": true, "api": true}, "default": {"kind": "list"}}
```

`inherit` is whether the space shows the server's default when it has nothing
else; `apis` are the ids of the interfaces it uses (for a language, the one
made for it, else one that takes every language, and none when there is no such: never one made for another language; with several for the language
they are in this order, see "Several interfaces for a language"); `options` are
the values chosen for the built-in interfaces of `apis`, by interface id and
option key (see "Options of a built-in interface"; absent is all); `available` are all
that are set up (never their addresses), a built-in one with each of the
languages it answers in (`languages`; `lang` is the first of them); `default` is `null` when the
administrators set none. `languages` are the language codes the settings pages
have a tab for (the administrators' `languages`, below; nothing is limited to
them). `allowed` is what the administrators let spaces do (below): a field that
is `false` is set for the whole server, and what the space has there is saved
when asked but has no effect until it is `true` again. `PUT` takes `{"inherit": true, "apis": []}` (both are
needed) and optionally `options` and `sources`, and answers as `GET`; `400 invalid` for an id
that is not in `available`, and for an option or value its interface does not
declare, or one for an interface that is not in `apis`; it saves `apis` even when `allowed.api` is `false`.

**Interfaces that are off or gone.** What a space chose and the administrators
switched off (or deleted) since is not lost by a save from the settings page.
`GET` keeps an interface that is switched off in `apis` and in `sources`, with
`"available": false, "reason": "disabled"` on a source, and lists it in
`unavailable` (`{"favqs": "disabled"}`); it is skipped when the text is read, and
`PUT` takes it back as it is (it cannot be newly chosen while it is off: `400
invalid`). One that no longer exists is left out of the answer and listed in
`unavailable` as `"gone"`, so the settings page can say so. Reading then gives no
text with the reason `unavailable` when nothing the space chose can be used: the
default is not shown in its place, unless the space's list has another source
that works (the lines, its own pages). Locks (`allowed`) are another matter:
what a lock stops is kept and not used, and the default is shown.

**A space's own list of sources.** `sources` is, as in the administrators'
default, a list for each language in order (`""` for the languages not listed;
at most 8 each), the sources taking turns by day with the same rotation and
fallback ("Several interfaces for a language"), but its kinds are
`{"kind": "api", "id", "options"?}` (an interface that is set up and enabled),
`{"kind": "list"}` (the administrators' lines) and
`{"kind": "own", "source": {"folder"|"tag"|"type": …, "filter"?}}` (pages of this
space, checked in it; the text has the page's body, its `author`/`作者` as the
author and its source or title as the source, and no link). A space that
chooses no `sources` keeps its `apis` as before (an ordered list of interface
ids and their `options`, read in order for the language); a language that has a
list uses it in place of `apis`. `GET` gives `sources` when the space has them,
without the interfaces that are no longer set up. The locks apply to what is
listed and leave it saved: with `allowed.api` off a listed interface is skipped,
with `allowed.own` off the pages of the space are, and what is left is used
(`list` is the administrators' own). The tenant's default takes the same lists
(`kind: "sources"`, with `space` sources of the tenant's own spaces and no
`list`), through the settings API only: its editor comes with 3.6.

### The administrators' settings of the daily text

All `admin` only, accounts mode only. They are in the state database, not in
any space. The server is the only one that ever asks an external interface:
browsers never do (a key stays on the server, an interface is asked once a
period however many people look, everybody sees the same text, and the
protection of the network is in one place).

#### `GET /api/admin/daily`

```json
{"knownLanguages": [ /* below */ ],
 "apis": [ /* interfaces, below */ ],
 "providers": [ /* built-in interfaces, below */ ],
 "lines": {"total": 6, "byLang": {"zh-Hans": 3, "en": 2, "": 1}, "max": 5000}}
```

The interfaces, the providers and the lines only. The default, the languages and
what spaces may do are layered settings of the site: `daily.default`,
`daily.languages`, `daily.own` and `daily.api`, written with
`PUT /api/admin/settings/{key}` (the value of each is what the field below
describes, `daily.own` and `daily.api` being `allow.own` and `allow.api`);
`PUT /api/admin/daily` is gone (`405`). The record of the daily text in the
state database holds the interfaces and the built-in providers' state, and none
of these. `daily.default` and `daily.languages` are checked by the daily text's
code, with its error codes (`bad_language` and the others below), before the
write.

- `default` is `null`, `{"kind": "list"}` (the lines below),
  `{"kind": "sources", "sources": {"": [ … ], "zh": [ … ]}}`, a list of sources
  for each language in order, each `{"kind": "api", "id": "hitokoto", "options":
  {…}}`, `{"kind": "list"}` (the lines; once in a list is enough) or
  `{"kind": "space", "space": id, "source": {"folder"|"tag"|"type": …}}`
  (1 to 8 for each language, always a list of source objects; `400 invalid`
  for a source that is none, an interface that is not set up or a space that
  is not there; a tenant's default may not have `list`, and its spaces are its
  own). A `list`, `space` or `api` default is used as a list of one, or of the
  interfaces it lists. The
  sources of a language take turns by day, the way the interfaces of one do
  (see "Several interfaces for a language"): the one the day starts with is
  made from the day and the scope, "another" moves to the next source that gives
  a text after the one that gave today's (a source that had nothing is not
  stopped at twice), and a source
  that fails, is asked too often or has nothing (no lines, a space with no
  page that meets the source) is passed over for the next. Only one text is
  ever given. The source of a page of a space says who wrote it and where
  from, only when the page does (below),
  `{"kind": "space", "space": id, "source": …}` where `source` is a source or
  sources by language as above, checked in that space (choosing a space shows
  its content to everybody; a client says so), or
  `{"kind": "api", "apis": {"": ["favqs", "dummyjson"], "zh": ["hitokoto"]}, "options": {"hitokoto": {"categories": ["i"]}}}`:
  the interface ids per language, in order, `""` for all languages not listed
  (a list of 1 to 8 for each language). Each must be an interface of `apis` (`400 invalid` with
  `error: "api_not_found"` otherwise); `options` as for a space's choice. When
  none of the listed interfaces is enabled the daily reason is `unavailable`.
  The interface is used exactly as when a space picks it (same cache, refresh
  and rate limit), and the `allow.api` lock does not apply to it: it limits
  what spaces may choose, not the administrators' own default.
- `languages` are the languages the settings pages have a tab for besides
  "all languages", as BCP 47 language tags in canonical form (`zh-Hans`,
  `zh-Hant`, `en`, `pt-BR`; each once, at most 30), `["zh-Hans", "en"]` to begin
  with. Every language tag anywhere in the daily text (this list, per-language
  sources, interfaces, lines, the `lang` of `GET …/daily`) must be a well-formed
  tag whose language subtag is in the IANA registry: `cn`, `english` or
  `zh_CN` are refused with `400 invalid`, `error: "bad_language"`. The server
  writes tags in canonical form (`ZH-cn` is stored as `zh-CN`). A reader's
  language is matched to the configured ones the usual BCP 47 way (`zh-CN` and
  `zh-SG` find `zh-Hans`, `zh-TW` and `zh-HK` find `zh-Hant`, `en-GB` finds
  `en`); another script of the same language is tried before "all languages"
  (`zh-TW` finds `zh-Hans` when there is no `zh-Hant`), then "all languages". `GET /api/admin/daily` also returns
  `knownLanguages`: `[{"tag": "zh-Hans", "name": "中文（简体）", "english": "Chinese (Simplified)"}, …]`,
  the common languages a client offers to pick from (any valid tag may still be
  typed).
- `allow` is what the spaces may do: `own` is whether a space may have a source
  of its own (`daily` in its `space.yml`), `api` whether it may choose an
  external interface. Both are `true` until set; `PUT` takes either or both
  (`{"own": false}` leaves `api` as it is; `400 invalid` for neither, or a value
  that is not a boolean). A lock does not remove anything: the space's `daily`
  stays in `space.yml` and its choice of interfaces stays, they are ignored, and
  the order of `GET …/daily` goes on to the next one (own, interface, default)
  as if they were not set; unlocked, they count again. A space's
  `PUT …/daily/settings` and `PUT …/daily/prefs` still save while locked.
  `GET` and `PUT …/daily/settings` say whether the source is in effect:
  `{"daily": …, "inEffect": false}`. In single-user mode there are no
  administrators and everything is in effect.

**Languages everywhere.** A space's `daily`, the default's `source`, the lines
and the interfaces are all by language, with "all languages" (an unkeyed
source, lines with `lang` `""`, an interface with `lang` `""`) as the fallback
for the languages that have none of their own, as in `GET …/daily/settings`
above.

#### Interfaces: `PUT /api/admin/daily/apis/{id}`, `DELETE …/apis/{id}`

Any number of interfaces (at most 50). An interface in `apis`:

```json
{"id": "hitokoto", "name": "一言", "lang": "zh",
 "url": "https://international.v1.hitokoto.cn/?max_length=80&d={date}",
 "headers": [{"name": "X-Api-Key", "set": true}],
 "text": "hitokoto", "source": "from", "author": "from_who",
 "link": "https://hitokoto.cn?uuid={uuid}",
 "refresh": "daily", "refreshMinutes": 0, "skipNew": true, "rps": 20, "enabled": true}
```

- `id`: 1 to 32 lower-case letters, digits, `-`, `_`; it is the one in the
  address. `name` at most 100 characters. `lang` is the language it answers
  in, `""` for every language.
- `url` is fetched with `GET`. It is `https`, with no user name, and `{lang}`
  (the language asked for, `en` when none) and `{date}` (the day, `2026-11-23`)
  may be in its path or query, not in its scheme, user, host or port (as the
  address is written, in any case); they are filled in only as a checked
  language code and day, escaped, and the request may go only to the host
  (and port) of `url` as written: **there is no separate list of trusted
  hosts** — saving an interface is what trusts its host, and only for that
  interface. An address that is an IP address
  that is not public, `localhost` and its subdomains are refused when it is saved;
  whatever a name resolves to is checked again when the connection is made (below).
- `headers` are request headers, at most 10. A value is **never returned**, and
  never reaches any output (below). It is encrypted with the server's key
  (`DATA/secret.key`, or `PETALBOOK_SECRET_KEY`), bound to the exact request it
  was given for: the interface's `id`, the header's name and the `url` exactly
  as written (scheme, host, port, path, query and placeholders). The interface
  answers with `{"name": …, "set": true}`. In a request
  `{"name": "X-Api-Key", "value": "…"}` sets or replaces a value, and
  `{"name": "X-Api-Key"}` keeps the one it has: `400 header_missing` when it has
  none (another `id` has none), `400 header_reenter` when `url` is not, to the
  character, the one the value was given for. Any change of the address, even
  of its path or query, drops every kept value of the interface: they are
  given again by whoever has them. `text`, `source`, `author` and `link` may
  change freely. A header that is left out is removed. `Host`, `Content-Length`,
  `Content-Type`, `Transfer-Encoding`, `Connection` and the like cannot be set.
- `text` (needed), `source` and `author` are paths in the JSON answer: names
  and positions with dots and brackets (`hitokoto`, `data.0.q`, `[0].content`);
  the value must be text or a number; the text is cut to 2 000 characters, the
  others to 200.
- `link` is an optional template of a link shown with the text (an attribution):
  `{lang}` and `{date}` are those of the request, any other `{path}` the value at
  that path of the answer (URL-escaped); it is `http` or `https`, and is left out
  when a value it needs is missing. It never takes a request header:
  `400 link_uses_header` (`field` `"link"`) for a placeholder any part of whose
  path is `header`, `headers` or the name of one of the interface's headers
  (`{headers.Authorization}`, `{args.X-Api-Key}`), in any case.

**No header value reaches an output.** Each value the server shows (`text`,
`source`, `author`, each value of `link`, the `raw` of a trial) is taken in
this order: the value at its path, decoded from JSON (escapes such as `\/` and
`A` undone); checked; cut; and, in a link, escaped. A value that contains a
header's value (of 4 characters and more), or any 8 characters in a row of
one, in any case, as it is or with its URL escapes undone (`%2B`, `+` read as a
space), is refused: a trial says so (`path_reveals_secret`), and an answer to
readers with such a value is not used at all (as a failure: the last good
text, or `unavailable`). The old blotting out of values (`•••`) is kept as a
second line only.
- `refresh` is how long the text everybody sees stays the same: `"daily"` (the day
  of the request: the `date` parameter), `"hourly"`, or `"minutes"` with `refreshMinutes`
  1 to 1440 (by the server's clock). `skipNew` says "another" (`skip` above 0)
  asks the interface for a new text each time (at most `skip` 100); without it
  "another" is the same text. `rps` is the most requests a second this server
  sends the interface, 0.01 to 100, 1 when left out: a token bucket (as many
  tokens as `rps`, at least one) shared by everybody who looks and every
  language. With the bucket empty nothing waits: the last text the interface said
  is used. `enabled` (true when a new interface leaves it out) says whether it may
  be chosen by spaces and is used.

`PUT` makes the interface or replaces it, and answers as
`GET /api/admin/daily`. Changes of the settings (these
and `DELETE`) are made one at a time, in a transaction that holds the
settings: two administrators saving at once both keep their change. `DELETE` removes it and
answers the same; `404` for an id that is not there. `400 invalid` for anything
above that breaks a rule (the message says which), or one of the codes of
"Daily text refusals".

**Built-in interfaces** (`providers`): services the server knows how to ask,
so an administrator only switches one on — no address, paths or language
codes to fill in. `GET /api/admin/daily` lists them as
`{"id": "hitokoto", "name": "一言", "languages": ["zh-Hans", "zh-Hant"], "site": "https://hitokoto.cn", "note": "…", "enabled": false, "refresh": "daily", "minutes": 0, "rps": 20, "maxRps": 20, "skipNew": true, "options": [{"key": "categories", "param": "c", "kind": "multi", "values": [{"value": "a", "label": {"zh": "动画", "en": "Anime"}}, "…"]}]}`
(`options` are what a reader may choose of the service, `[]` for none; `minutes` is set when `refresh` is `"minutes"`; `maxRps` is the service's own limit, the most `rps` may be).
`PUT /api/admin/daily/providers/{id}` takes `{"enabled": true}` and optionally
`refresh`, `minutes`, `rps` (not above the service's own limit) and
`skipNew`; needs admin; answers as `GET /api/admin/daily`. An enabled built-in
interface appears in `apis` with `"builtin": true` and can be chosen by spaces
and as the site default like any other; its address, paths, link and language
mapping cannot be changed (`400 invalid`, `error: "builtin"`). None is enabled
until an administrator enables it, and no service is contacted before that.
Today: `hitokoto` (一言, the international line; `zh-Hans`, `zh-Hant`; link
back to the sentence as the service asks, kept in the answer but never shown as a
link by the web app) `zenquotes` (ZenQuotes, quote of the day; `en`; its terms
require a link back to ZenQuotes, which PetalBook does not show, so it is in no
default and an administrator enables it at their own judgement; it allows 5
requests per 30 seconds per address, so `rps` is at most 0.1), `favqs` (FavQs, quote of the day; `en`), `animechan`
(Animechan, anime quotes with the anime as `source` and the character as
`author`; `en`; 100 requests a day per address, so `rps` is at most 1/60 and
"another" does not ask it again), `dummyjson` (DummyJSON, about 1,400 quotes made
for developers' tests; `en`; `skipNew`), `jinrishici` (今日诗词, classical
poetry through the service's older interface; `zh-Hans`, `zh-Hant`; `skipNew`;
option `category`), `shanbay` (扇贝每日一句; `zh-Hans`, `en`) and `iciba`
(金山词霸每日一句; `zh-Hans`, `en`). The last two answer in both languages at
once: each language is a lane of its own, and a reader gets the one text in
their language (the translation or note for `zh-Hans`, the English for `en`),
never both. `shanbay` and `iciba` serve only the languages they have, `zh-Hans`
and `en`, matched the way languages are everywhere (`zh-Hant` and `zh-TW` are
matched to `zh-Hans`, `en-GB` to `en`): a reader of another language (`fr`) is
given none of their text, they are skipped, and the next source of the list
answers (with none the reason is `no_source`). `shanbay` is the interface the service's own mini program uses, not
a published API, so it may change. `iciba` labels its JSON `text/html`, which is
accepted for it only (a flag only the program sets on a built-in interface; the
body must still parse strictly as JSON, with the same size limit). None of
them is in any default. An id that names no interface (one that was
deleted) is an interface that does not exist: in a default, a choice that names it gives the reason
`unavailable` when read; and a space that chose it is shown nothing, with the
reason `unavailable`: nothing else is picked in its place, not the tenant's or
the site's default (see "Interfaces that are off or gone", below).

**Options of a built-in interface.** A provider may declare options: a `key`, the
query parameter `param` the values are sent as, a `kind` (`multi`: any number of
the `values`) and the values with a label in `zh` and `en`. `hitokoto` has
`categories` (`c`): a 动画, b 漫画, c 游戏, d 文学, e 原创, f 来自网络, g 其他,
h 影视, i 诗词, j 网易云, k 哲学, l 抖机灵. What is chosen is stored with the
choice (a space's `options`, a default's `options`, which a tenant's default
has as well), by interface id and option key. Choosing none, or every value, is
all, and is what is stored when nothing is chosen: settings saved before options
behave as before. Values are kept once each in the order they are declared and
are added to the address as repeated, URL-encoded parameters
(`…?max_length=80&c=d&c=i`); a value that is not declared is `400 invalid`. An
address with other values is another thing to the cache: two spaces with
different categories never share or replace an item, and the same categories
share one. The rate limit is the interface's, across all sets of values.
Options of an interface that is no longer chosen are dropped when saved.

An option of kind `path` is one choice (a list of one value), put in the address
where `{key}` stands, with its `default` when none is chosen (and the default is
stored as none). `jinrishici` has `category`: `all`, the first level (`shuqing`,
`siji`, …) and the second (`shuqing/libie`), the categories the service
publishes that answered with JSON when checked.

**Only the reader's language.** The daily text is in the language of the reader
and no other: the source for the language they are matched to (BCP 47, so
`zh-Hant` finds `zh-Hans`), else the one for "all other languages", and when
there is none the answer has no text and the page shows its greeting alone. Lines
and pages of another language are not used in its place, and neither is an
interface a space chose that is made for another language. A built-in interface
serves the languages it has (`languages` of the provider): `hitokoto` and
`jinrishici` have Chinese only, `zenquotes`, `favqs`, `animechan` and
`dummyjson` English only; listing one under a language it does not have is `400
invalid`, and one listed under `""` is skipped for the readers it has no text for.
The web app shows the author and source as text, `—— 作者《出处》`, and never as
a link: `link` is kept in the answer and not drawn.

**Several interfaces for a language.** A language's list is in order. The
interfaces take turns by day: the one the day starts with is the day number
plus a number made from the scope (the space, the tenant or the site) modulo
the length of the list, so everybody in a scope sees the same one that day and
a space with a choice of its own is in its own scope. One that is switched off,
fails, is asked too often or has no usable answer is passed over for the next,
within the same request; its last answer, shown as `stale`, is used only if no
other has a fresh one, and when none answers the reason is `unavailable` (or
`429 rate_limited` when every one was asked too often). "Another" (`skip`) moves
to the next of the list; after the last it is the first again, which asks for a
new item if its `skipNew` allows. With one interface nothing changes. For a
space the list is its `apis` in order, for the reader's language (made for it,
or else for every language, and none otherwise).

**Language codes in the address**: `{lang}` is filled with the reader's
language as matched (a canonical BCP 47 tag, e.g. `zh-Hans`). Services that
want another form are given a `langMap` on the interface,
`{"zh-Hans": "zh", "en": "en"}`, applied before filling in (built-in
interfaces have theirs set; custom interfaces may set one, keys checked as
language tags, values 1–35 characters of `[A-Za-z0-9_-]`).

#### `POST /api/admin/daily/try`

```json
{"api": {"id": "hitokoto", "name": "…", "url": "…", "headers": [{"name": "X-Api-Key"}], "text": "hitokoto"},
 "lang": "zh", "date": "2026-11-23"}
```

Makes one request now with the interface as it is in the form (saved or not;
a header without a value uses the value saved for that `id` and that exact
`url` only), at the same rules as any request, and shows what came back. It goes only to
the host of the address in the form, under the same public-address checks. It
keeps nothing and is limited to a few a second (`429`). `400 invalid`, or a
code of "Daily text refusals", when the interface itself breaks a rule.

A trial that worked, or failed for a reason that has no code (the service did
not answer, answered with an error status, or said a header's value back
outside every path), is a `200`:

```json
{"ok": true, "status": 200, "url": "https://…?l=zh", "raw": "{\"hitokoto\": …", "rawTruncated": false,
 "quote": {"text": "…", "source": "…", "author": "…", "link": "https://hitokoto.cn?uuid=…"}}
{"ok": false, "message": "status 503", "status": 503, "url": "…"}
{"ok": false, "status": 200, "url": "…", "quote": {"text": "…"},
 "message": "the answer repeats the value of a request header, so it is not shown"}
```

A trial whose answer is refused is a `400` with the code of the refusal, what
it is about beside it, and what the trial showed in `result` (the shape
above). With several path errors, the code and `field` are those of the first:
a `path_reveals_secret` before a `path_error`, in the order text, source,
author, link.

```json
{"error": "path_error", "message": "no text or number at hitokoto", "field": "text",
 "result": {"ok": false, "status": 200, "url": "…", "raw": "…",
            "pathErrors": [{"field": "text", "code": "path_error", "message": "no text or number at hitokoto"}]}}
{"error": "not_json", "message": "the answer is not JSON",
 "result": {"ok": false, "status": 200, "url": "…", "message": "…"}}
```

`raw` is the start of the answer (4 000 characters), shown only when it is
valid JSON and none of its decoded texts, names and numbers repeats a request
header's value (see above); otherwise it is left out, and `message` says why.
Only the `text` path is required: a `source`, `author` or `link` whose path
finds a null, nothing or an empty text (or whose link needs a value that is
missing) gives no value for that field, and is no error.
`pathErrors` is a list of `{field, code, message}` (`field` one of `text`,
`source`, `author`, `link`; `code` `path_error`, for `text` only, or `path_reveals_secret`); the
fields that were found are still in `quote`. `message` says what went wrong
with the request or the answer.

#### Daily text refusals

The code is the `error` of the error body; what the refusal is about is beside
it. All are `400`.

| `error` | beside it | when |
|---|---|---|
| `header_reenter` | `header` | a header without a value whose kept value was given for another `url` |
| `header_missing` | `header` | a header without a value, and none is kept for it (for this `id`) |
| `not_json` | | a trial's answer does not declare JSON, or is not valid JSON |
| `too_large` | | a trial's answer is larger than 64 KB |
| `path_error` | `field` (`"text"`) | a trial's text path finds no text or number |
| `path_reveals_secret` | `field` | a trial's path (or a value of the link) would show a request header's value |
| `link_uses_header` | `field` (`"link"`) | a link template with a request header in it (saving or trying) |
| `skip_too_large` | | `GET …/daily` with a `skip` above 1 000 |

A `date` that is a day but not today, yesterday or tomorrow, for an external
interface, is not a refusal: `GET …/daily` answers `200` with `"reason":
"out_of_range"`. A `date` that is not a day is `400 invalid`.

**How it is fetched.** Only the interfaces the administrators wrote, only from
`https`, only to the host of the interface's own address. The address a name resolves to is checked
when the connection is made, for every address (so a name that answers
differently the second time is no way round it): loopback, private, link-local
(the cloud metadata address included), unspecified, multicast, carrier-grade
NAT, documentation and benchmarking addresses are refused, an IPv4 address
written as IPv6 being judged as the IPv4 one, always, for every request and every
trial. No redirect is followed, no proxy used. The answer must come within 5
seconds, be `200`, declare JSON, and be at most 64 KB.

**What is kept.** (1) In memory, by interface (as it is asked: its id, address,
paths and header names), language, refresh period and skip: inside a period
nobody asks again, however many look; requests in flight for the same item are
one, and that one request is not cut short (nor counted as a failure) when the
reader who started it goes away. The language is the one the answer can differ
in: none for an address without `{lang}`, the interface's own `lang` when it
has one, else the reader's language or its base when it is one of
`languages`, else the first of them. What is kept by interface and language
(last answers, waits after failures, rate limits) is bounded (1 024 of each;
the oldest go). (2) In the state database, the same: a restart asks for nothing
again (rows older than 7 days are removed, and at most 2 000 rows of an
interface are kept, the newest). (3) In the browser, by `GET …/daily` (below).
When an interface fails (an error, a bad answer, no answer in time) the last
text it said in that language is given, marked `"stale": true` with the time it
was got (`"fetched"`); with none, or none from the last 7 days, the answer is
`unavailable` and the block is hidden. After a failure the interface is not asked
again for 30 seconds, then 1, 2, 4 … minutes up to an hour, never at every
request; a success clears the wait. A space's own source and the default
that is a space or lines are cheap (the index, a row) and use only the memory of
the day and the browser's.

#### `GET /api/admin/daily/lines?lang=`, `POST …/lines/preview`, `PUT …/lines`, `DELETE …/lines?lang=zh`

The lines are a source of the default that is stored here: pasted or uploaded
text, each entry with the language it is in (`""` for any language). `PUT`
replaces the lines of one language:

```json
{"lang": "zh", "text": "山色空蒙雨亦奇\t苏轼\n行到水穷处", "format": "lines"}
```

`format` is `lines` (each line that is not blank is an entry; a tab separates the
text from where it is from), `markdown` (entries are cut at the lines that
are `---`, or at the blank lines when there are none) or `csv` (columns text,
author, source; a first row that says `text` (or `文案`, `正文`, `内容`) is a
header). Entries are at most 8 000 bytes, their author and source 200
characters each, and the server keeps at most 5 000 in all.

**Author and source.** An entry has `text`, `author` and `source`. In `lines`
and `markdown` they are cut off the end of the entry when it ends with
` —— `, ` — ` or ` -- ` (with the spaces) and an attribution, the last mark
only, and only when a dash is not inside a sentence: what is before the mark
ends a sentence or a quotation (`。！？…」』”’` or `. ! ? " '`), or what is
after it has a `《source》`. So `春眠不觉晓 —— 处处闻啼鸟`, `人生如梦 —— 一尊还酹江月`,
`Never give up — Never` and `War — Peace and Love` are text only, while
`春眠不觉晓 —— 孟浩然《春晓》` is split: `人到情多情转薄，而今真个不多情。 —— 纳兰性德《山花子》` is the author 纳兰性德
and the source 山花子 (the source is what is in `《…》`); `The best way out is always
through. — Robert Frost` has the author Robert Frost, and `… — Author, Source`
both. What follows the mark must look like an attribution, else the entry is the
text only: a name of a few words each beginning with a capital letter (the small
words of names such as `van` and `de` allowed), or in Chinese up to 12
characters, or anything with `《…》` that ends there, and no sentence
punctuation, so `I think — therefore I am` and `Wait — what? — Douglas Adams`
(split only at the last mark) are cut right. A tab gives the source and
leaves the text to be cut for its author. `POST …/lines/preview` takes
`{"text", "format"}` and answers `{"lines": [{"text", "author"?, "source"?}]}`,
the split as it would be kept, and keeps nothing; so a wrong split can be
seen. `PUT` takes `"lines": [{"text", "author", "source"}]` in place of `text`
and `format`, to keep the lines as the administrators corrected them, and
`GET` gives the lines kept for a language in that shape. All of these show
the text as `{"text", "author", "source"}` of the daily quote (below).

`DELETE` removes the lines of one language. `PUT` and `DELETE` answer as `GET /api/admin/daily`.
For a language the lines used are those of the language (matched as languages
are everywhere), else those for any language ("all other languages"); never
those of another language: a reader of a language with none of its own and no
lines for any language is given none. The line is picked as a page is (from the day and
`skip`, modulo how many there are).

### Favourites

A person's own list of pages, folders and saved views in a space, in the order
they put them. It is kept in the state database, not in the space (nothing is
committed), for a person and a space. At most 200 in a space. It is deleted
with the space or the user; a person who leaves a space keeps theirs in the
database but they are not returned, and are back if the person is added again.
(An administrator who is not a member of the space has none, and cannot add
any: `403 forbidden`.) Everything here needs `read` and works in both modes.

```json
{"items": [
  {"kind": "page", "target": "01JD3K8Q7Z", "name": "暮雪", "path": "Tang/暮雪.md", "missing": false, "created": "2026-11-23T08:30:00Z"},
  {"kind": "folder", "target": "Tang/七言", "name": "七言", "path": "Tang/七言", "missing": false, "created": "…"},
  {"kind": "view", "target": "folder:Tang#k3f9a1", "name": "按作者", "of": "Tang", "ofKind": "folder", "missing": false, "created": "…"}
 ], "limit": 200}
```

`target` is a page id, a folder path as it is on disk, or a saved view as
`folder:<path>#<view id>` or `tag:<tag>#<view id>`. `name` is a page's label, a
folder's last name, a view's name; `missing` is true when the page, the folder
or the view is gone (the name is then `""` for a page or a view and the folder's
name for a folder). The names and `missing` of the whole list are looked up in
the index and the settings in one go: no page file is read.

- `GET /api/spaces/{space}/favorites` answers the list.
- `POST …/favorites` with `{"kind": "page", "target": "…"}` adds one at the end
  (one that is there stays where it is) and answers the list. `404` for a page,
  folder or view that is not there, `400 invalid` for a target that cannot be
  one, the root folder, and a 201st favourite.
- `DELETE …/favorites?kind=page&target=…` takes one off (target URL-encoded;
  also one that is `missing`; one that is not there is not an error) and
  answers the list.
- `PUT …/favorites/order` with `{"items": [{"kind": "page", "target": "…"}, …]}`
  puts the favourites in that order: those named first, the rest after in the
  order they had; names that are not favourites are ignored. Answers the list.

Moving or renaming a folder (see `POST …/folders/move`) changes, once it has
succeeded, the paths of the favourites of every person in the space: of the
folder, of the folders below it, and of the saved views of those folders. A
person who already has a favourite at the new path keeps it, and their old one
goes. The move is recorded in the state database before it is made, and the
record goes with the change of the favourites, whether or not the client is
still there; a record left by a crash or a failure is settled the next time
the favourites of the space are read or a folder of it is moved. All records
of a space are settled together, oldest first, in one transaction (all or
none): a move is taken as made when its folder is gone from the old place,
whatever its age, or a later move put a folder there, or a folder is at both
places (the one at the old place is new: it keeps none of the old one's
favourites); its favourites go to where the folder is, or to its new place
from which a later move carries them on, so moves A to B and B to C take A's
favourites to C, settled in any order. A record of a folder still where it was
and not at the new place is forgotten after ten minutes. A move onto a folder
that is there is refused and not recorded. Deleting a folder leaves the
favourites, now `missing`.

## Files

Images and other attachments are ordinary files of the space. The ones the
server stores live in `assets/<page id>/` at the root of the space, and a page
refers to them by a path from the root of the space:

```markdown
![a rainy street](/assets/01JD3K8Q7Z/street-3f9a1c2b.jpg)
[the contract](/assets/01JD3K8Q7Z/contract-91be02aa.pdf)
```

That path stays right when the page is moved or renamed. A client showing a
page resolves the address of an image or link like this: one with a scheme
(`https:`, `data:`, `mailto:` …) or starting with `//` or `#` is left alone;
one starting with `/` is a path from the root of the space; anything else is
relative to the folder of the page. The resulting path is fetched with
`GET …/files/{path}`.

### `POST /api/spaces/{space}/pages/{id}/assets?name={file name}`

Stores a file for the page. The body is the file's bytes, sent with
`Content-Type: application/octet-stream`. `name` is the name the file had;
only its last path segment is used.

`201` with

```json
{"path": "/assets/01JD3K8Q7Z/street-3f9a1c2b.jpg", "name": "street-3f9a1c2b.jpg", "size": 48213}
```

`path` is what goes into the page. The stored name is made by the server:
the given name with characters that file systems refuse replaced, cut to a
reasonable length, the extension in lower case, and the first 8 hexadecimal
digits of the SHA-256 of the content before the extension. Uploading the same
bytes under the same name again answers `200` with the same body and stores
nothing new.

Errors: `404 not_found` when the page does not exist; `400 invalid` for an
empty body or a missing name, and when `assets` or the page's directory in it
is not a plain directory (a symbolic link, a file, or a folder named the same
in another letter case on a file system that ignores case); `409 exists` when
another file already has the stored name (same name, same first 8 digits,
other bytes); `413 invalid` when the file is larger than
`maxUpload` (see `GET /api/meta`); `415 invalid` for another content type;
`503 unavailable` as for every change in a space that follows a remote.

The file is committed like a page is: at once and pushed in a space that
follows a remote, otherwise by the next background commit. It is stored
whether or not the page ends up referring to it; nothing removes files that
no page refers to (a later milestone).

### `GET /api/spaces/{space}/files/{path}`

The bytes of a file of the space. `path` is from the root of the space, each
segment percent-encoded. `404 not_found` for a path that does not exist, is
not a regular file, has a segment starting with a dot, goes through a
symbolic link, or is a page (a `.md` file outside `assets/`): pages are read
through the page endpoints.

The answer carries `X-Content-Type-Options: nosniff`, a
`Content-Security-Policy` that forbids scripts and sandboxes the document,
and `Cache-Control: private, no-cache` with `Last-Modified`, so a browser
revalidates with `If-Modified-Since` and gets `304`. `Range` requests work.
The content type comes from the extension, for a fixed list of image, audio,
video, PDF and plain-text types; those are sent `Content-Disposition:
inline`. A PDF is sent without the policy header, because browsers' PDF
viewers do not start under it. Everything else is `application/octet-stream` and
`Content-Disposition: attachment`.

From milestone 3.1 a space may keep its attachments in an attachment
repository (see "Server storage", "Attachment repository"). The paths and
links stay the same; `assets/…` is then read from the attachment cache, an
attachment not in it is fetched first, and `503 not_cached` (with
`Retry-After`) is the answer while that repository cannot be reached. Uploads
are pushed to the attachment repository, and a local space over the
administrator's limit answers `507 space_full`.

## Import

### `POST /api/spaces/{space}/import?folder={folder}&name={file name}`

Brings Markdown files into the space as pages. The body is either one
Markdown file or a `.zip` of files and folders (which it is, is told from the
content: a zip starts with its signature), sent with
`Content-Type: application/octet-stream` like an upload. It may be at most
`maxImport` bytes (see `GET /api/meta`; `--max-import`, 100 MiB by default).

| name | meaning |
|---|---|
| `folder` | the folder the pages go into; optional, the root by default. It is made if missing |
| `name` | the name the file had; required for a single Markdown file (it gives the title when the file has none), ignored for a zip |

Every file whose name ends in `.md` or `.markdown` becomes one page; folders
of a zip become folders below `folder` (a file in a folder the server would
not make, see `POST …/folders`, is left out). The server works on one import
at a time; a second one waits its turn. Nothing that exists is replaced: a
page whose file name is taken gets a numbered one, as when creating a page.
For each file:

- A header (YAML between `---` lines at the top) is kept, key by key, except
  `id`: every imported page gets a new id. A `created` that is not a date is
  dropped, and one is assigned as for a new page. A header that does not
  parse is left in the body as text.
- A Markdown file larger than a page may be (8 MiB) is left out of a zip,
  and refused (`400 invalid`) when sent alone.
- The title is the header's `title`; else the first line of the body when it
  is a `# ` heading (which is then taken out of the body); else the file's
  name without its extension.
- An image or link whose address is a relative path to another file in the
  zip (not a page) makes that file an attachment of the page, and the
  address is rewritten to it (see "Files"). A file larger than `maxUpload`
  is left out and the address stays as written. Addresses with a scheme,
  absolute paths, and links to other Markdown files stay as written.

Files that are neither pages nor referred to by one are not imported; hidden
files and folders (a name starting with a dot, `__MACOSX`) are ignored.

`200` with a report:

```json
{
  "pages": 104,
  "files": 3,
  "skipped": [{"path": "notes/draft.docx", "reason": "not a Markdown file and no page refers to it"}]
}
```

`files` counts attachments stored. `skipped` lists what was left out and
why, for a person to check (at most 200 entries; `skippedMore` is how many
further ones there were, absent when none).

Errors: `400 invalid` when the body is empty, is a single file without
`name` or with a name that is not Markdown, is a zip without any Markdown
file, unpacks to more than four times `maxImport`, has more than 20,000
entries, or `folder` is not a valid folder path; `413 invalid` over
`maxImport`; `415 invalid` for another content type; `503 unavailable` as
for every change in a space that follows a remote.

The whole import is one change: one commit, and in a space that follows a
remote one push, with nothing left behind if it fails. In a local space a
failure half way (a full disk) can leave the pages created so far. With an
attachment repository (milestone 3.1) the attachments are one commit and one
push there first; if the pages then fail, those attachments stay on that
remote, referred to by nothing.

## Folders and tags

### `GET /api/spaces/{space}/tree`

Every folder that exists, including empty ones, with the number of pages
directly inside it and `updated`: the `updated` of the newest page in the
folder or in any folder below it, at any depth, in the format a page has. A
folder with no page in it or below it has no `updated` key. The server works
it out in one query and one pass over the folders, so the cost grows with the
number of folders, not pages.

```json
{"folders": [{"path": "", "pages": 3, "updated": "2026-11-21T08:30:00Z"}, {"path": "Tang", "pages": 95, "updated": "2026-11-20T10:02:11Z"}, {"path": "随笔", "pages": 0}]}
```

### `POST /api/spaces/{space}/folders`

```json
{"path": "随笔/2027"}
```

`201` with `{"path": "随笔/2027"}`; `409 exists` if it is already there;
`400 invalid` for a name starting with a dot, `assets` at the root in any
letter case (that name is reserved for attachments), or a name ending in
`.md` (that is how page files are named). The same applies to folders made
on the way by creating or moving a page.

### `POST /api/spaces/{space}/folders/move`

```json
{"from": "随笔/2026", "to": "归档/2026"}
```

Moves a folder with everything in it, which is also how a folder is renamed
(`{"from": "随笔", "to": "散文"}`). Missing parents of `to` are made. `200`
with `{"path": "归档/2026"}`. Pages keep their ids, so links, drafts and
attachments are not affected. It is one change: one commit, and one push in
a space that follows a remote.

Errors: `404 not_found` when `from` is not a folder; `409 exists` when `to`
already exists (folders are not merged), except that `to` may be another
spelling of `from` itself (a change of letter case); `400 invalid` when
`from` is the root, when `to` is `from` or inside it, or when `to` is not a
folder name the server makes (see `POST …/folders`).

### `DELETE /api/spaces/{space}/folders?path={path}`

Removes a folder. `204`. Without `recursive=true` the folder must hold no
pages, in it or below it (`409 conflict` otherwise; empty subfolders go with
it). With `recursive=true` the pages in it and below it are deleted like
`DELETE …/pages/{id}` deletes one, drafts included; they remain in Git
history. Files in the folder that are not pages are left where they are, and
then so is the folder that holds them. `404 not_found` when it is not a
folder, `400 invalid` for the root.

### `GET /api/spaces/{space}/tags`

```json
{"tags": [{"tag": "栀子", "pages": 41, "color": "rose"}]}
```

Ordered by count, descending. `color` is absent when the tag has none.

### `PUT /api/spaces/{space}/tags/{tag}`

```json
{"color": "rose"}
```

Sets the colour of a tag, or removes it with `{"color": null}`. The colour
is either one of the names `gray`, `red`, `orange`, `amber`, `green`, `teal`,
`blue`, `purple`, `pink`, `rose` (how each looks is the client's business,
and follows its theme), or a colour of the person's own as `#rrggbb` (six
hexadecimal digits; stored in lower case). `200`
with `{"tag": "栀子", "color": "rose"}` (`color` absent when removed).
`400 invalid` for another value, for a tag that is empty, longer than 200
bytes or has a line break in it. A `.petalbook/tags.yml` that cannot be used
does not stop the change (see Settings files that cannot be used). The
tag in the address is percent-encoded, so it may contain `/`. The tag need
not be in use (it is listed, with its colour, once a page uses it). Colours are
kept in `.petalbook/tags.yml` in the space, so they travel with the
repository; a change is committed like any other. The same entry of the file
holds the tag's saved views (see Saved views), which setting or removing the
colour leaves alone. A colour written as an alias (`color: *c`) is the colour
of its anchor, and changing or removing the colour of one tag never changes
another's (see Anchors, aliases and merge keys).

## Templates

Templates are ordinary pages under `.petalbook/templates/` of the space (header
and body, type fields allowed). They are not listed as pages, not indexed for
search, and travel with the repository. Variables in the title, the header
values and the body: `{{date}}` (YYYY-MM-DD), `{{time}}` (HH:MM) and
`{{datetime}}`, in the time zone the client sends (`tz`, minutes east of
UTC, as for the daily text); nothing else is substituted, and `{{` that is not
one of these stays as written.

### Template

```json
{"name": "日记", "path": ".petalbook/templates/日记.md", "rev": "…", "meta": {"title": "{{date}}", "tags": ["日记"], "type": "diary"}, "body": "## 今天\n"}
```

`name` is the file name without `.md`; it is the template's id.

### `GET /api/spaces/{space}/templates`

`{"templates": [/* Template without body */], "defaults": {"folders": {"随笔": "日记"}, "quickNote": "随记"}}`.
Needs `read`. Defaults are kept in `space.yml` (`templates: {folders: {…},
quickNote: …}`), with the settings-file rules of 2.2.

### `GET /api/spaces/{space}/templates/{name}`

The template with its body. `404 not_found`.

### `PUT /api/spaces/{space}/templates/{name}`

`{"baseRev": "…" | "", "meta": {…}, "body": "…"}`. `baseRev` `""` creates;
otherwise it must match (`409 conflict` with `current`). Needs `manage`.
Names follow page-name rules; at most 200 templates, 256 KiB each
(`400 invalid` with `error: "template_limit"` / `"template_too_large"`).

### `DELETE /api/spaces/{space}/templates/{name}?baseRev=…`

Needs `manage`. Folder defaults and the quick-note default that name it are
removed in the same change.

### `PUT /api/spaces/{space}/templates/defaults`

`{"folder": "随笔", "template": "日记" | ""}` or `{"quickNote": "随记" | ""}`.
Needs `manage`. `400 invalid` with `error: "template_not_found"` for an
unknown template.

### Creating a page from a template

`POST /api/spaces/{space}/pages` takes `"template": "日记"` and `"tz"`.
Without `template`, the folder's default template applies (send
`"template": ""` to create a blank page); quick notes use the quick-note
default. The request's `meta` and `body` win over the template's per field;
the template's `type` is kept. `400 invalid` with `error:
"template_not_found"` for an unknown name.

## Checks

The check page lists what is wrong in a space. Everything comes from the
index (problems found when files are indexed), so it never scans files.

### `GET /api/spaces/{space}/checks?kind={kind}&cursor={cursor}`

Needs `read`. `{"counts": {"no_id": 2, …}, "items": [Problem], "next": "…"}`,
50 per page, ordered by kind then path. `kind` filters.

### Problem

`{"kind": "…", "path": "…", "id": "…"?, "rev": "…", "message": "…", "detail": {…}?, "fix": "add_id" | "remove_fields" | null}`

Every problem concerns one file, `path`, and `rev` is that file's current rev
as the index has it: the `baseRev` of `POST …/checks/fix`. A file without an
`id` is not a page, so this is the only place a client can read its rev from.
`id` is the page's (or, for `duplicate_id`, the id the file shares).

Kinds:

| kind | meaning | detail |
|---|---|---|
| `no_id` | a page file without an `id` | |
| `duplicate_id` | two or more files share an `id` | `{"paths": […]}` |
| `bad_header` | the header is not valid YAML | `{"line": n}` |
| `type_mismatch` | type fields that do not fit the type's schema | `{"fields": […]}` |
| `not_in_type` | header fields that are not in the page's type | `{"fields": […]}` |
| `broken_link` | a link to a page that does not exist: its id is no page's, or, written with a path only, no page is at the path | `{"target": "…", "line": n}`: the path the link names, from the space root, and the line of the file it is on; at most 20 a page |
| `missing_asset` | an image or attachment that does not exist | `{"target": "…"}` |
| `duplicate_title` | two pages in one folder with the same title | `{"paths": […]}` |

### `POST /api/spaces/{space}/checks/fix`

`{"kind": "no_id" | "not_in_type", "path": "…", "baseRev": "…"}` (`baseRev`
the problem's `rev`). Needs
`write`. `no_id` gives the page a new id; `not_in_type` removes the listed
fields (only those, the rest of the header byte-for-byte). Answers the page
like a save; `409 conflict` on a stale `baseRev`; `400 invalid` with
`error: "not_fixable"` for other kinds.

## Notifications

The notification centre (3.10): what happened to the signed-in person that
they have not read. Every route here is the caller's own: rows are read by the
caller's id and tenant, never by an id from the request, so another person's
(in any tenant) cannot be listed or marked, and a token (`token_scope`) or an
administrator's view of a space has no way in. Single-user mode has them too.
Nothing is written as prose: a row says the kind, what it is about and the
facts, and the web app writes the sentence in the person's language.

### Notification

```json
{"id": "019354a1b2c3000007", "kind": "invite", "ref": "6FAVM4ZE32",
 "payload": {"space": "Team", "role": "editor", "by": "Olive"},
 "at": "2026-11-24T08:00:00Z", "read": false}
```

`ref` is the space the notice is about. `at` is null for a derived one. A
derived one also has `"derived": true` and the id `problems:<space id>`. The
kinds (a registry in the server's code, `store.NotifyKinds`):

| kind | made by | payload |
|---|---|---|
| `problems` | derived: the problems in the index of each space the person can write in; no row | `{"space": name, "total": n, "counts": {"no_id": 1, …}, "sig": "…"}` |
| `invite` | adding the person to a space (`POST …/members`), unless they added themselves; a change of role is not one | `{"space", "role", "by"}` |
| `sync` | the remote of a space the person owns needs attention (`error`, `rewritten`), and works again after it did; "offline" comes back by itself and is not told, and a state is compared with the last one that was not "offline" (error, offline, error is told once; error, offline, ok is told "ok") | `{"space", "event": "failed" \| "ok", "state", "kind"}` (`kind` is the error's kind: `auth`, `hostkey`, `clone`, a connection code) |
| `mention` | reserved: there are no mentions yet | |
| `todo` | reserved: the sidebar's reminders (5.x) | |
| `space_taken_over` | an administrator took over a space the person owned (`POST /api/admin/spaces/{space}/takeover`); `ref` is the space; written with the `space_taken_over` audit row | `{"space", "by", "reason"}` |
| `space_transferred` | an administrator transferred a space the person owned (`…/transfer`); as above, with the `space_transferred` row | `{"space", "by", "reason"}` |
| `account_changed` | someone else changed the person's name, username or address (`PATCH /api/users/{id}`); nothing is said of what they did themselves; written with the `user_changed` row | `{"fields": ["name", "username", "email", "emailVerified"], "by"}`, never the values |
| `password_set` | an administrator gave the person a temporary password; written with the `password_set_by_admin` row | `{"by"}` |
| `email_released` | an address the person held unverified moved to the account that proved it; written with the `email_released` row | `{"email"}` |
| `link_reauth` | a linked account's tokens could not be renewed for good (the host revoked or ended the grant): the person connects it again; written once, by the write that marks the link `expired`; `ref` is the provider | `{"provider", "name", "login"}` |
| `domain_lapsed` | an email domain the person's organisation had verified lapsed: the daily recheck did not find its DNS record for 7 days (see "Email domains and the SSO entry"); to each of the tenant's administrators, with the `domain_lapsed` audit row; `ref` is the domain's key | `{"domain", "missingSince"}` (`domain` as people read it) |
| `join_requested` | someone with an address on a verified domain asked to join the person's organisation (see "Joining an organisation"); to each of the tenant's enabled administrators, with the `join_requested` audit row; `ref` is the request | `{"username", "name", "domain"}` |
| `join_approved` | the person's request to join was approved, which made this account; with the `join_approved` audit row; `ref` is the request | `{"tenant", "by"}` (names) |
| `viewed_as` | reserved: an administrator viewed the app as the person; made only once the owner decides the person is told (see "Viewing as a person"); it would be written with the `view_as_started` row | `{"by", "reason", "until"}` |

There is no catch-all kind: each event of an account or a space is a kind of its
own, so the filter (`?kind=`) can tell them apart. Each is written by one helper
(`notify.WithAudit`) that inserts the audit row and the notification in one
transaction from one definition, so the two cannot drift apart or exist alone;
`web/src/i18n/notifyKinds.json` is the registry's list, kept current by a Go
test, and a web test fails for a kind without its text in both languages. A
kind this client does not know is shown as a plain notice. Invitations to an
organisation are not a kind yet: an invite makes an account, so it has no
recipient who is signed in; the kind is added with the first invitation of an
existing person.

**The problems are not copied to the members.** They are read when the list or
the count is asked for: for each space the person can write in (a disabled one,
and one reached only as an administrator, left out), from the live index when the space is open, else from
what it said when it was last open or closed (in memory; a restart forgets it
until the space is opened again; a deleted space is forgotten). Nothing is
opened for this, and a count of an open space is reused for 20 seconds. At most
50 spaces with problems are listed; how many more have some is
`moreProblems` of the list. What the person has read is a cursor per
person and space (`notification_seen`): the signature of the problems then.
When the problems change, the notice is unread again; when they are all fixed,
it is gone.

### `GET /api/notifications?kind={kind}&unread=1&cursor={cursor}&limit={n}`

Signed in. `{"items": [Notification], "next": "…", "unread": n, "moreProblems": n}`
(`moreProblems` left out when 0, and on pages after the first), newest first
(the derived `problems` first, on the first page only), `limit` 1 to 100
(default 20). `next` is the cursor of the following page (`""` after the last),
one range scan of the index `notifications (user_id, id DESC)`. `unread` is the
badge count, at most 100. `kind` is one of the registry (`400 invalid`
otherwise; `kind=problems` lists only the derived ones, which have no history).

### `GET /api/notifications/unread`

`{"count": n}`: the stored unread rows (one count on the partial index
`notifications (user_id) WHERE read_at IS NULL`, stopping at 100) and the
derived ones not read as they are now; at most 100 (the badge says "99+"). The
web app asks once a minute while the window is in front.

### `POST /api/notifications/read`

`{"ids": ["…", "problems:6FAVM4ZE32"]}`, 1 to 100. Marks the caller's own rows
as read, and records what the caller has read of each derived one, as it is now.
An id that is not the caller's, or not there, changes nothing. `{"unread": n}`.

### `POST /api/notifications/read-all`

`{"kind": "sync"}` (optional: all kinds without it). Marks every unread row (of
the kind) and the derived ones as read. `{"unread": n}`.

Rows are kept for six months at most, read ones for 30 days (the daily job
deletes the older ones in batches), and at most 500 for each person (the
newest: the oldest past that goes when a row is added). A person's rows go with
them when they are deleted.

### Browser notices

The opt-in is the person's, one of their preferences (`pb.notify.browser`, see
"Preferences"), so another person who signs in on the same browser gets none for
it; the permission is the browser's, and both are needed. It works through the
Notifications API: an open tab shows a notice when the unread count goes up
while nobody is looking. By default the notice only says how many came ("You
have N new notifications"), so no space or person is named on a screen others
may see; with the preference `pb.notify.details` it says what the newest event
is (not a problem of a space that was there before). There is
no push server yet. **Reserved:** Web Push with a VAPID key would reach a
browser with no tab open: a `GET /api/notifications/push` that gives the public
key, and `PUT`/`DELETE` of a subscription. Nothing of it is built; email and
phone push come with the todo reminders.

## Preferences

What a person sets for themselves in the interface (the columns, order and
widths of their tables, whether folders come first, how far the tour got) is
kept in a file of **their own personal space**, `.petalbook/prefs.yml`, never in
the database. The route takes no space and no person: it is always the
caller's own, so nobody else's can be named; an API token or an
administrator's view cannot reach it (`token_scope`); the file is no page, no
attachment and no setting of the space, so no route serves it (a path with a
dot in it is `404 not_found`), the index and the offline sync's catalogue do not
see it, and an export an administrator makes of a personal space leaves it out
(an export with `history` carries the repository's history, which names the
file in the commits that wrote it: a site administrator's export of a personal
space is the one existing way to read it, as for everything in one).

### `GET /api/prefs`

Signed in. `{"personal": true, "prefs": {"pb.table.users": {…}, …}}`, read from
the file (what was written and is not committed yet, on top). `personal: false`
with `{}` when the person has no personal space to keep them in (or it is
switched off or evicted): the browser keeps them. The web app asks once a
session and caches the answer.

### `PATCH /api/prefs`

`{"set": {"pb.table.users": {…}, "pb.files.mixed": true, "pb.table.x": null}}`:
from 1 to 100 keys, each a name of up to 120 letters, digits and `. _ : / -`
(no `..`) and a JSON value of at most 32 KiB; `null` removes the key (`400
invalid` otherwise). `{"personal": bool}`, `false` when there is nowhere to
keep it or it cannot be written now (no personal space, switched off, evicted,
or read only past its deadline for a remote): nothing is then kept, and the
browser keeps it. `400 prefs_full` when the file, with what is pending and this
write, would have more than 100 keys or be larger than 256 KiB: nothing of the
write is kept. The write is kept in memory (a read shows it at once) and written in
**one batch** `3 minutes` after the first of a burst (`DefaultPrefsQuiet`), so a
person dragging a column makes one small commit in a few minutes: the commit is
"Update prefs.yml", by the person, and is not in the history of any page; the
file is written by the space's own committer and, when the space follows a
remote, pushed with it (the personal space's own repository). What is pending
is written when the server closes. The file is at most 256 KiB.

There is one write of a person's batch at a time, and a value written while it
runs stays for the next one (the newer value wins). A batch that can never be
written (the file would be too large, grown by hand meanwhile; the space is
read only; the person has no personal space) is dropped and logged; one that
may work later (the space is evicted for a while) is tried again up to 6 times,
each after twice the wait before (3, 6, 12 … 192 minutes), then dropped and
logged. The browser keeps its copy either way.

## API tokens

Milestone 2.6. A token lets a script or an MCP client act for one person in
a few of their spaces, without that person's password or session. It is never
more than its owner: what a request with a token may do is what the owner may
do now, narrowed by the token.

### Held sessions, API tokens and OAuth grants

A hold (see "Passkeys") belongs to a session, not to the person: tokens and
grants are independent of it. A person who is held cannot make a token or
approve a consent (`403 passkey_required`), so nobody who only has the
password gets a new credential that way. The tokens and grants the person
made before a passkey was required of them keep working, with the same rules
as always; requiring a passkey does not revoke them. Revoke them
(`DELETE /api/admin/tokens/{id}`, `DELETE /api/admin/grants/{id}`) to stop
them. This keeps a token request free of the passkey check (no query per
request) and leaves running scripts and MCP clients alone when an
administrator raises the sign-in requirements.

The same holds for a lockout and for `ssoOnly`: both govern signing in, not
credentials already made. A locked-out account, or a non-administrator under
`ssoOnly`, keeps its tokens and grants; revoke them as above. Only a disabled
owner is refused at once (`token_owner_disabled`, `invalid_grant`).

`GET /oauth/authorize` with a held session goes on to the consent page as
usual; the web app shows the passkey page there instead, and the consent page
once a passkey is registered (the address is kept, also through the fresh
sign-in that registering may ask for).

### How a token is presented

`Authorization: Bearer pbk_…` on the request. Nothing else carries one (no
query parameter, no cookie). A request with an `Authorization: Bearer` header
(even one with nothing after the word, which is a token that is not valid) is
judged by that header alone: a session cookie sent with it is ignored, and a
token that does not work never falls back to the cookie. The cross-site rules of
"Conventions" apply unchanged (`Content-Type: application/json` on changes; no
CORS on `/api`).

The secret is `pbk_` and 43 characters of `0-9A-Za-z` (32 random bytes). The
server keeps only its SHA-256, never the secret; the first 12 characters
(`pbk_` and 8 more, the `prefix`) are kept to tell tokens apart in lists.

**What a token reaches.** A token is accepted on:

- every route under `/api/spaces/{space}/…` whose permission is `read` or
  `write` (see "Space"), for a space the token names;
- `GET /api/spaces` and `GET /api/meta`: only the token's spaces, with the
  permissions the token has in them (below);
- `GET /api/session`: the owner as `user`, plus
  `"token": {"id", "name", "access", "spaces": ["…"]}` (`null` for a session);
- `/mcp` (see "MCP").

The drafts are not among them: `PUT` and `DELETE …/pages/{id}/draft` and
`POST …/pages/{id}/commit` answer `403 token_scope` to a token. A draft is a
person's, kept for the browser; what a token writes goes straight to a commit
(with its own `via`, like `/mcp`), so the server never commits a token's draft
under no name, and a token never commits a browser draft as "API token".

Everything else, a route that needs `members` or `manage` (the remote of a
space among them), and anything under `/api/account`, `/api/account/links`,
`/api/users`, `/api/invites`, `/api/admin`, `/api/oauth` and the sign-in
routes, answers `403 token_scope`: a token cannot make tokens, change members,
settings of a space, types, templates or remotes, connect or use its owner's
GitHub account, or act as an administrator.

**Permission of a request.** For a space the token names, the permissions are
those the owner has in the space now (through the cached memberships of 1.7:
no query per request), intersected with the token's `access`: `read` gives
`read`; `write` gives `read` and `write`. An administrator's token reaches only
the spaces its owner is a member of: the administrator's way into other spaces
is not open to tokens. So:

- a space the token does not name: `403 token_scope`;
- a space it names but whose member the owner no longer is (or that no longer
  exists): `404 not_found`, the same as for a session, from the owner's very
  next request after the removal;
- a write with a `read` token: `403 token_scope`. Any request that is not
  `GET` or `HEAD` is a write here, whatever permission the route asks: a
  `read` token cannot delete a draft, add or remove a favourite or reorder
  them either;
- a write with a `write` token whose owner is now a viewer there:
  `403 no_permission`, from the next request after the change of role;
- a disabled space: `403 space_disabled`.

**Refusals of the token itself** answer `401` with
`WWW-Authenticate: Bearer error="invalid_token"`:

| code | when |
|---|---|
| `token_invalid` | not a token this server issued (malformed, unknown, or revoked or expired more than 90 days ago and removed) |
| `token_expired` | past its `expires` |
| `token_revoked` | revoked by its owner or an administrator |
| `token_owner_disabled` | the owner's account is disabled; the token works again if it is enabled |

`403 tokens_off` when an administrator has switched tokens off (see
"Machine access" under "Administration"); the tokens are kept. After 10
failed tokens (`token_invalid`, `token_expired`, `token_revoked`) from one
client address in a minute, further token requests from it answer
`429 rate_limited` with `Retry-After`, as sign-in does. OAuth access tokens on
`/mcp` are counted on their own, so one address may try up to 10 of each in a
minute.

A token is checked once, when a request arrives: revoking it, or demoting its
owner, while a request is already being handled lets that request finish (at
most one write), and the next one is refused.

**What a lookup costs.** The server hashes the secret and looks the hash up in
memory; a hash not seen yet costs one query on a unique index of the state
database, and an unknown one is remembered as unknown for 30 seconds. A
token found is kept in memory for at most 30 seconds before it is read again,
so a revocation made by another process (the `petalbook token` command) takes
effect within 30 seconds; one made through the server takes effect at once.
The owner and their memberships come from the caches sessions use. When the
token was last used is written at most once an hour. A request with a token
that is in memory costs no query.

### Token

```json
{"id": "01JD3M9T2B", "name": "Desktop notes", "prefix": "pbk_3fK9a2Lq", "access": "write",
 "spaces": [{"id": "01JD3K8Q7Z", "name": "My notes"}],
 "created": "2026-11-23T08:30:00Z", "expires": "2027-02-21T08:30:00Z", "lastUsed": null,
 "revoked": null, "state": "active"}
```

- `name`: 1–80 characters, for the owner to recognise it; need not be unique.
- `access`: `read` or `write`.
- `spaces`: 1–100 spaces, each by id with its current name (`""` for a space
  that no longer exists or the owner has left; it stays listed, and does
  nothing). There is no "all my spaces": a token names its spaces.
- `expires`: a time, or `null` for never.
- `lastUsed`: when it was last used; it is updated at most once an hour, so it
  is right within an hour; `null` if never.
- `revoked`: when it was revoked, or `null`.
- `state`: `active`, `expired` or `revoked`.

Revoked and expired tokens stay listed for 90 days, then are removed.

### `GET /api/account/tokens`

Accounts mode, signed in with a session. `{"tokens": [Token]}`, newest first:
the caller's tokens, including revoked and expired ones still kept.

### `POST /api/account/tokens`

```json
{"name": "Desktop notes", "access": "write", "spaces": ["01JD3K8Q7Z"], "expiresInDays": 90}
```

`access` defaults to `read`. `expiresInDays` is 1–365, default 90; `0` means
never. Every space must be one the caller is a member of now
(`404 not_found` naming the first that is not); `write` is accepted for a
space where the caller is only a viewer, since what counts is the role when the
token is used.

`201` with `{"token": Token, "secret": "pbk_…"}` and `Cache-Control: no-store`.
The secret is in this answer only; it cannot be shown again.

`400 invalid` for a bad name, access, list of spaces or expiry;
`400 token_limit` when the caller has 50 tokens that are not revoked or
expired.

### `DELETE /api/account/tokens/{id}`

Revokes one of the caller's tokens. `204`, also when it was revoked already.
The next request with it answers `401 token_revoked`. `404 not_found` for a
token that is not the caller's.

### `petalbook token` (single-user mode)

Single-user mode has no accounts and no token endpoints (they answer `404`).
A token is made on the command line, for the built-in user and the one space
`default`. The commands read and write the state database directly and work
while the server runs. Requests without a token to `/api` keep working as
before; `/mcp` always needs one.

```
petalbook token create --space DIR [--state-dir DIR] [--database URL] --name NAME [--access read|write] [--expires 90d|never]
petalbook token list   --space DIR [--state-dir DIR] [--database URL] [--all]
petalbook token revoke --space DIR [--state-dir DIR] [--database URL] ID|PREFIX
```

The three flags that find the state database are those of `petalbook draft`.
For a server in accounts mode, `--data DIR [--database URL]` with
`--user USERNAME` lists or revokes that user's tokens (for an administrator who
cannot sign in); tokens in accounts mode are created through the app.

- `create`: `--access` defaults to `read`, `--expires` to `90d` (`Nd` with `N`
  from 1 to 365, or `never`). Prints the secret alone on one line on standard
  output, and what it is on standard error:

  ```
  pbk_3fK9a2LqV0eXw7Ty1MbN4cRj8sHk2Gd6Pz5uQaYo
  token 01JD3M9T2B "Desktop notes" (write, expires 2027-02-21): shown once, keep it now
  ```

- `list`: one line per token, active ones only unless `--all`:

  ```
  ID          PREFIX        NAME           ACCESS  EXPIRES     LAST USED         STATE
  01JD3M9T2B  pbk_3fK9a2Lq  Desktop notes  write   2027-02-21  2026-11-23 09:00  active
  ```

- `revoke`: by id, or by prefix when exactly one token has it; a prefix must be
  the full 12 characters (a shorter one is an error that says so, and revokes
  nothing). Prints `revoked 01JD3M9T2B "Desktop notes"`; the running server
  refuses the token within 30 seconds. An id or prefix that matches no token,
  or a prefix that matches several, exits with status 1 and says so.

## OAuth for MCP clients

Accounts mode only, and only with `--public-url` (it is the issuer and the
address tokens are bound to). It lets an MCP client be connected by pasting the
address of `/mcp`: the client discovers this server, the person signs in and
approves, and the client gets tokens. It follows what the MCP authorization
specification asks of a server: OAuth 2.1 with PKCE (`S256` only), resource
indicators (RFC 8707), the `iss` parameter in the response (RFC 9207), client
ID metadata documents, and dynamic client registration (RFC 7591) as the
fallback. Clients are public (no client secret); the only grants are
`authorization_code` and `refresh_token`. It is Rackbook's authorization
server with grants and clients kept in the state database (SQLite or
PostgreSQL) instead of a file.

These endpoints answer errors in the OAuth shape,
`{"error": "<code>", "error_description": "…"}`, not the API's, and with the
OAuth codes (`invalid_request`, `invalid_client`, `invalid_grant`,
`invalid_scope`, `invalid_target`, `unsupported_grant_type`,
`unsupported_response_type`, `access_denied`, `temporarily_unavailable`,
`server_error`). The discovery documents, `/oauth/token` and `/oauth/register`
take no cookies and answer every origin (`Access-Control-Allow-Origin: *`, and
the cross-site rules of `/api` do not apply to them), so a browser-based MCP
client can use them. When MCP is switched off (see "Machine access") all of
them, the authorization and consent endpoints included, answer `404`; the
grants (below) are kept and can still be listed and revoked. In single-user
mode, and in accounts mode without `--public-url`, there is no authorization
server: the same paths answer `404`.

### Scopes and what a grant may do

| scope | gives |
|---|---|
| `read` | `read` in the spaces of the grant |
| `write` | `read` and `write` in the spaces of the grant |

The spaces are not in the scope: the person picks them on the consent page. A
grant then works exactly as an API token with that `access` and those spaces
(see "API tokens": same narrowing by the owner's current role, same codes,
same reach on `/mcp`). An OAuth access token is accepted on `/mcp` only; on
`/api` it answers `403 token_scope`. A request without `scope` asks for
`read write`; the person may still approve read only.

### Discovery

- `GET /.well-known/oauth-protected-resource/mcp` and
  `GET /.well-known/oauth-protected-resource` (RFC 9728):

  ```json
  {"resource": "https://notes.example.com/mcp", "authorization_servers": ["https://notes.example.com"],
   "bearer_methods_supported": ["header"], "scopes_supported": ["read", "write"]}
  ```

- `GET /.well-known/oauth-authorization-server` (RFC 8414): `issuer`,
  `authorization_endpoint` (`/oauth/authorize`), `token_endpoint`
  (`/oauth/token`), `registration_endpoint` (`/oauth/register`), `response_types_supported: ["code"]`,
  `grant_types_supported: ["authorization_code", "refresh_token"]`,
  `code_challenge_methods_supported: ["S256"]`,
  `token_endpoint_auth_methods_supported: ["none"]`,
  `client_id_metadata_document_supported: true`,
  `authorization_response_iss_parameter_supported: true`, `scopes_supported`.

Both are cacheable for 5 minutes. A request to `/mcp` without a token answers
`401` with
`WWW-Authenticate: Bearer resource_metadata="https://notes.example.com/.well-known/oauth-protected-resource/mcp", scope="read write"`.

### `POST /oauth/register`

RFC 7591, for clients that have no client ID metadata document.
`{"client_name": "…", "redirect_uris": ["…"], "token_endpoint_auth_method": "none", "grant_types": [...], "response_types": ["code"]}`
(body at most 16 KiB; 1–10 redirect URIs, each `https`, `http` on a loopback
address with any port, or the private-use scheme of a native app such as
`cursor://…`, RFC 8252; never a fragment, credentials, `javascript:`, `data:`
and the like, nor a host with look-alike letters). `201` with the client
(`client_id` is `pbc_…`); `200` with the existing client when the same name and
redirect URIs are registered again, which costs nothing. At most 30 new
registrations per client address in 10 minutes (`429 temporarily_unavailable`
with `Retry-After`) and 1000 clients in all; a client never authorized is
removed after a day, one with no grant left after 30 days.
`400 invalid_client_metadata` (also for a body that is not sent as
`application/json`) or `invalid_redirect_uri` for a bad request. When
the 1000 are all in use, `503 temporarily_unavailable` with `Retry-After`;
otherwise the oldest client nobody authorized makes room. Clients are kept in the
state database.

A `client_id` that is an `https` URL is a client ID metadata document: the
server fetches it (at most 8 KiB, no private or loopback addresses, cached as
its `Cache-Control` says, between 1 minute and 1 hour; a fetch that failed is
remembered for 1 minute, and requests for the same `client_id` at the same
moment share one fetch) and takes the name and redirect URIs from it. Nothing
is registered.

### `GET /oauth/authorize`

The browser is sent here by the client with `response_type=code`,
`client_id`, `redirect_uri`, `code_challenge`, `code_challenge_method=S256`,
`resource` (the URL of `/mcp`), `scope` and `state`. Without a session it
redirects to the sign-in page, which comes back here. With a bad `client_id` or
a `redirect_uri` the client did not register, it shows an error page and
redirects nowhere (the page is in English or, for a browser that asks for it,
Chinese). Any other bad parameter is shown on the same page, with its OAuth
code, and is not sent back to the client either: `400` with
`invalid_request` for missing or non-`S256` PKCE, `invalid_target` for a
missing or other `resource`, `invalid_scope`, `unsupported_response_type`.
Before the person has consented the server never redirects with an error, so
a client that registered itself, or that names any metadata document, cannot
use it as an open redirector (RFC 9700 section 4.11.2); the only error that
goes back to the client's `redirect_uri` is `access_denied`, once the request
was well-formed and the person refused it on the consent page. Otherwise it
redirects to the consent page, `/oauth/consent?req=…`; the request is kept 10
minutes, in memory, for this person only. One person may have at most 20
requests waiting for a decision, and the server 1000 in all: past either, the
page shows `temporarily_unavailable` (`429` for the person's own 20, `503` for
the server's 1000) until one is decided or expires. The browser of someone who is not signed in is sent to
`/login?return=<this address>`; the web app turns that return into a full page
load of `/oauth/authorize`, which the server answers. A link from another site
may open `/oauth/authorize` (it only asks the person); the rest of `/api` never
accepts requests from other sites.

### Consent

The consent page reads `GET /api/oauth/consent?req=…`:

```json
{"client": {"name": "Notes helper", "id": "https://app.example/client.json", "host": "app.example"},
 "redirectHost": "localhost", "punycode": false, "scopes": ["read", "write"],
 "spaces": [{"id": "01JD3K8Q7Z", "name": "My notes", "permissions": ["read", "write", "members", "manage"], "personal": true}],
 "user": {"id": "01JD3K7A2M", "name": "Kazuha"}}
```

The page shows who asks (the name is the client's own claim; the host is what
can be trusted, and `punycode` warns of a host that may look like another), where
the answer goes, and the person's spaces to choose from (none chosen at first
except the personal one), and offers read or read-and-write when `write` was
asked for. It answers with `POST /api/oauth/consent`:

```json
{"req": "…", "approve": true, "spaces": ["01JD3K8Q7Z"], "access": "write"}
```

`200` with `{"redirect": "https://…?code=…&state=…&iss=…"}` (or
`error=access_denied` when `approve` is false), to which the page sends the
browser. `404 not_found` when the request has expired, was decided already or
is someone else's; `400 invalid` for no space, a space the person is not a
member of, or `write` when only `read` was asked for. The code is good once,
for 60 seconds. These two are ordinary API routes: they need a session, never
take a token, and follow the cross-site rules.

### `POST /oauth/token`

`application/x-www-form-urlencoded`, at most 16 KiB, with `client_id` and no
secret.

- `grant_type=authorization_code` with `code`, `redirect_uri`,
  `code_verifier`, `resource`. A code used twice ends the grant made with it.
- `grant_type=refresh_token` with `refresh_token` (and optionally `resource`,
  `scope` no wider than the grant's).

`200`:

```json
{"access_token": "…", "token_type": "Bearer", "expires_in": 600, "refresh_token": "pbr_…", "scope": "read write"}
```

The access token is a JWT signed with the server's ES256 key (made at the first
start and kept in the state database, sealed with the server key like the other
secrets: `DATA/secret.key` or `PETALBOOK_SECRET_KEY`; with another server key it
cannot be opened and the server does not start), valid 10 minutes, bound to the
`/mcp` URL and to the grant. There is no JWKS endpoint: only this server checks
it, in process. A token whose signature was checked is kept in memory by its
hash until it expires, and the grant it names is looked up in the same cache
as API tokens (30 seconds), so a check costs no query, and revoking a grant
through this server stops its access tokens at once (one made by another
process, within 30 seconds). A refused access token answers with the codes of
"API tokens": `token_invalid`, `token_expired`, `token_revoked` (the grant is
gone), `token_owner_disabled`. The refresh token is replaced at every refresh; the
replaced one, sent again within 60 seconds, gets the same answer (a response
that was lost), and after that ends the grant. A grant not refreshed for 90
days ends. The refresh token is `pbr_<grant id>.<secret>`; only its SHA-256 is kept.
`invalid_grant` for a bad, used or ended code or refresh token, a verifier that
does not match, and when the owner is disabled or the grant was revoked (the
client must authorize again). A disabled owner does not end the grant: it works
again when the account does.

### Grants

A grant is one approval of one client by one person. They are listed and
revoked like tokens:

- `GET /api/account/grants`: `{"grants": [{"id", "client": {"name", "id"}, "access", "spaces": [{"id", "name"}], "created", "lastUsed"}]}`,
  newest first. `lastUsed` is the last refresh (the creation until the first
  one); `name` of a space is `""` as for tokens.
- `DELETE /api/account/grants/{id}`: `204`; the client's access and refresh
  tokens stop working at once.

## MCP

Access through MCPHub (its tokens, its users bound as an external identity) is
deferred to after 2.6, until MCPHub's token format is confirmed.

`/mcp` serves the Model Context Protocol over the Streamable HTTP transport,
stateless: every `POST /mcp` carries one JSON-RPC message and is answered with
one JSON response (`Content-Type: application/json`), with no MCP session and
no event stream (every method but `POST` answers `405` with `Allow: POST`,
before any credential is looked at). Nothing is kept
per client between calls, so any number of clients cost nothing while idle.
It is on in both modes unless switched off (`404` then): `serve --mcp=false` in
single-user mode, the administrators' setting in accounts mode (see
"Machine access").

**Authentication.** Every request needs `Authorization: Bearer` with an API
token or (accounts mode) an OAuth access token; a session cookie is not
accepted. Without one: `401` and the `WWW-Authenticate` challenge of
"Discovery" (in single-user mode without `resource_metadata`, since there is no
authorization server). A token refused for itself answers `401` with the codes
of "API tokens" as an HTTP response, before any tool runs. The `Origin` and
`Sec-Fetch-Site` rules of "Conventions" apply (they guard against DNS
rebinding); a client that is not a browser sends neither.

**Tools** are the same for both kinds of credential. Each works in one space,
named by `space` (in single-user mode `default`, which may be left out). What a
tool may do is the permission of "API tokens": `tools/list` leaves out the
write tools when the credential cannot write in any of its spaces.

**Errors.** A failing tool returns a result with `isError: true` and, as
`structuredContent` (and its JSON as text), `{"error": "<code>", "message": "…", "details": {…}?}`
with the codes of this API: `not_found`, `invalid`, `conflict`,
`no_permission`, `token_scope`, `space_disabled`, `unavailable`,
`rate_limited`, `internal`, and three of its own, `too_large`,
`edit_no_match` and `edit_ambiguous` (below). A space the credential does not
name is `token_scope`; one whose member the owner no longer is, `not_found`.
Malformed arguments (a missing or unknown argument, one of the wrong type)
are JSON-RPC errors (`-32602`); an argument of the right type that is refused
(a `limit` out of range, a bad `cursor`, an unknown `sort`) is `invalid`.

**Limits.** No tool returns everything: lists are paged with a `cursor` (the
`next` of the previous result, `""` after the last), and each list has its own
default and largest `limit`. The structured result (`structuredContent`) is
at most 1 MiB as JSON; a larger one is refused with `too_large` and
`details: {"limitBytes", "sizeBytes", "suggestedLimit"?}`, to repeat with the
same cursor and a smaller limit. The cap is that of the structured result
only: the response also carries the same JSON as text (escaped once more, as
a JSON string), so the whole JSON-RPC response is about twice the result, a
little over 2 MiB at most for most results, and never more than three times
the result (a result made only of quotes and backslashes) and the envelope. A page body is returned at
most 128 KiB at a time (see `page_get`). A request is at most 2 MiB (`413`).
Lists and search are read from the index, never by scanning files, so they
cost the same in a space of 50 000 pages as the matching REST listing.

**Writes** are made as the REST saves make them (`PUT …/pages/{id}`,
`PATCH …/props`, `move`): checked against `baseRev`, journaled, committed
(and pushed, in a space that follows a remote, before the tool answers). They
do not go through the owner's drafts: a write by MCP leaves the owner's draft of
the page alone, as a `PUT` does, and the draft shows as stale in the app. The
author of the commit is the owner of the credential, as for their own saves.
Changes made through MCP are never committed together with changes made another
way: when the source of the pending changes of a space changes, what is pending
is committed first, as when the author changes (1.7). The commit message ends
with a trailer:

```
Edit 暮雪

Changed-via: MCP
```

A change made on `/api` with an API token ends with `Changed-via: API token`
in the same way. History items carry `"via": "mcp"` or `"via": "token"` for
such commits (absent otherwise), in `GET …/history` and in `page_history`.

### Page item

Lists and search return page items: the "Page summary" with the space, the
rev, and `props` (as with `props=1`: text cut to 300 characters, lists to 100
elements, `partialProps` naming what was cut):

```json
{"space": "01JD3K8Q7Z", "id": "01JD3K8Q7Z", "path": "Tang/暮雪.md", "folder": "Tang", "label": "暮雪", "title": "暮雪",
 "tags": ["玉兰"], "type": "", "effectiveType": "文案", "created": "2025-12-03", "updated": "2026-11-21T08:30:00Z",
 "excerpt": "…", "props": {"status": "draft"}, "rev": "3b18e5…"}
```

`excerpt` is at most 400 characters. No item carries a body.

### `spaces_list`

No arguments. What the credential reaches and the limits:

```json
{"user": {"id": "01JD3K7A2M", "name": "Kazuha"}, "credential": {"kind": "token", "access": "write"},
 "spaces": [{"id": "01JD3K8Q7Z", "name": "My notes", "permissions": ["read", "write"], "inbox": "Inbox"}],
 "limits": {"maxResultBytes": 1048576, "maxBodyChunk": 131072, "maxWriteBody": 1048576}}
```

`kind` is `token` or `oauth`. `permissions` is the narrowed set. A space named
by the credential that the owner has left is not listed. At most 100 spaces, so
never paged.

### `folders_list`

`{"space", "parent": "", "recursive": false, "cursor", "limit"}`: the folders
directly in `parent` (or, with `recursive`, all below it, in path order), as
`{"items": [{"path", "pages"}], "next"}` with the number of pages directly in
each. `limit` 1–1000, default 200. `not_found` for a `parent` that is no
folder (found as the app finds a folder, whatever its case).

### `tags_list`

`{"space", "cursor", "limit"}`: `{"items": [{"tag", "pages"}], "next"}`, most
used first. `limit` 1–500, default 100.

### `types_list`

`{"space"}`: `{"types": [...]}`, the page types of the space (at most 100), each
`{"name", "title", "fields": [{"key", "title", "kind", "options"?}], "folders": ["…"]}`
with the folders bound to the type, so a client knows which fields
`pages_query` can filter on. `options` at most 500 per field.

### `pages_search`

`{"space", "q", "in": "title"?, "folder"?, "recursive"?, "tag": [...]?, "cursor", "limit"}`:
full-text search, as `GET …/pages?q=` (same matching, ordered by relevance).
`q` is required, at most 500 characters. `limit` 1–50, default 20.
`{"items": [Page item], "next"}`.

### `pages_query`

`{"space", "folder"?, "recursive"?, "tag": [...]?, "filter": [...]?, "sort"?, "updatedFrom"?, "updatedTo"?, "createdFrom"?, "createdTo"?, "cursor", "limit"}`:
the listing of `GET …/pages` with its parameters and meanings, `filter` given as
a JSON array (not text): by folder, tags, dates, and conditions on type fields
(`in`, `notIn`, `range`, `empty`, `notEmpty`, `contains`), sorted by
`updated`, `created`, `title` or `prop:{key}`. Each is bounded as the REST
listing is (at most 10 tags, 11 conditions, 500 values each). `limit` 1–200,
default 50. `{"items": [Page item], "next"}`. `invalid` for what the REST
listing refuses.

### `page_get`

`{"space", "id"?, "path"?, "offset": 0}`: one page, by `id` or by `path`
(as links name it, from the space root; one of the two). The answer is the
"Page" of REST without `header` and without the caller's draft, with the body
in pieces:

```json
{"space": "…", "id": "…", "path": "Tang/暮雪.md", "rev": "3b18e5…", "updated": "…", "meta": {…},
 "effectiveType": "文案", "body": "…", "bodyBytes": 523114, "offset": 0, "next": 131072}
```

`body` holds at most 128 KiB of the body from byte `offset` (cut on a
character boundary), less when the header leaves less room in the 1 MiB of a
result; `next` is the offset of the rest, or `null` when this is the end;
`bodyBytes` is the size of the whole body. A header so large that no part of
the body fits beside it in 1 MiB is `too_large` (the page cannot be read
through MCP; the app and REST read it). The rev is the same in
every piece; if the file changed between two reads, a later piece with a
different `rev` means to start over. `not_found`; `invalid` for an `offset`
that is not a character boundary within the body; `too_large` as above.

### `page_history`

`{"space", "id", "cursor", "limit"}`: as `GET …/history`, newest first, with
`via`. `limit` 1–100, default 20.

### `page_version`

`{"space", "id", "commit", "offset": 0}`: the page as it was in that version,
as `GET …/versions/{commit}`, with the body in pieces as `page_get`.
`not_found` when the commit is not a version of the page.

### `page_create`

```json
{"space": "…", "folder": "Tang", "title": "暮雪", "subtitle": "…", "source": "…", "tags": ["玉兰"], "type": "文案",
 "props": {"status": "draft"}, "body": "…", "template": "日记", "tz": 480, "requestId": "…"}
```

Everything but `space` optional; as `POST …/pages` (the server picks the id,
`created` and the file name; templates and folder defaults apply the same way).
`body` at most 1 MiB (`too_large`). `props` are header fields that are not
built in, with the value rules of `PATCH …/props`. `requestId` (at most 100
characters): a second call with the same `requestId` by the same credential
within an hour returns the first result instead of making another page
(remembered in memory, at most 10 000, not across a restart). Returns
`{"page": Page item, "replayed": false}`. Needs `write`.

### `page_update`

```json
{"space": "…", "id": "…", "baseRev": "3b18e5…",
 "title": "…", "subtitle": "…", "source": "…", "tags": ["…"], "type": "…",
 "set": {"status": "done"}, "unset": ["old"],
 "body": "…", "edits": [{"old": "黄昏", "new": "暮色"}]}
```

`baseRev` is required (the `rev` from `page_get` or a page item); the rest
says what changes, and what is not given stays as the file has it, byte for
byte. Built-in fields are replaced when given (`""` or `[]` removes them);
`set`/`unset` change other header fields as `PATCH …/props` does; the body is
either replaced whole (`body`, at most 1 MiB) or changed by `edits` (not both):
at most 50, the `old` and `new` texts together at most 1 MiB (the page itself
may be larger), applied in order, each `old` (not empty) replaced by `new`, and each
`old` must occur exactly once in the body as it is by then (`edit_no_match`,
`edit_ambiguous`, with `details.index`; nothing is written). Changing the title
renames the file and the links to it follow (see "Links between pages"). A
change that leaves the file as it is writes nothing and returns the same
`rev`. Returns `{"page": Page item}`. Needs `write`.

`conflict` when `baseRev` is not the file's rev (unless only the server's link
rewrites changed it), with `details.current` holding the current `rev`,
`path` and `meta` but not the body: read it again with `page_get`.
`invalid` for an update that changes nothing named, built-in fields in `set`,
both `body` and `edits`.

### `page_move`

`{"space", "id", "folder", "baseRev"?}`: as `POST …/move`; the folder is made
if needed and links follow. With `baseRev`, `conflict` if the page changed
since. Returns `{"page": Page item}`. Needs `write`.

### `page_tag`

`{"space", "id", "add": ["…"], "remove": ["…"], "baseRev"?}`: adds and removes
tags (at most 50 each, the rules of tag names of `PUT …/tags/{tag}`), keeping
the other tags in their order and everything else byte for byte. Without
`baseRev` it applies to the file as it is now (adding a tag twice, or removing
one that is not there, changes nothing); with it, `conflict` if the page
changed since. Returns `{"page": Page item}`. Needs `write`.

There is no tool to delete pages or folders, move folders, or change types,
templates, settings or members.

## Offline sync

Milestone 4.1. These endpoints let a client keep a copy of some spaces on the
device and bring it up to date by asking only for what changed. They change
nothing on the server; writing back what was done offline is milestone 4.2.
Every endpoint here works in both modes (in single-user mode the space is
`default`) and with an API token; an OAuth access token is still for `/mcp`
only (`403 token_scope`).

They follow the rules of reads, tenants included (see "Tenants, registration
and mail"): a space of another tenant answers as one that does not exist, and
a person of a disabled tenant, or their token, is refused as on every other
route (`401`). The spaces of a disabled tenant are `403 space_disabled` to
everyone, as for their pages, and are never in `heads`.

**Allowing offline copies.** The layered setting `features.offline` (site, tenant;
allowed unless turned off, see "Settings by layer") decides whether the people
of a tenant may keep copies on a device. A tenant administrator can turn it off
for its own tenant. A site administrator (`PUT /api/admin/settings/features.offline`)
sets what tenants that say nothing follow, and can lock it (`lower` `lock`: the
site's value is the tenant's, so a locked `false` is off for every tenant) or
restrict it to off (`lower` `restrict`, `options` `[false]`: a tenant may only
turn it off). When it is
off for the caller's tenant, **all four endpoints** (`sync/catalog`, `changes`,
`sync/pages`, `heads`) answer `403 feature_off` with `feature` `features.offline`. This is decided
after the sign-in and the token's own checks but before the space is opened: an evicted
space is never restored for a request that is refused, and the answer is the
same for every space id (one of another tenant, or one that does not exist,
answers it too), so it tells nothing about the space. Nothing else changes: the rest of the API works as before. A client
reads the effective value from `features` of `GET /api/settings`; when it is off it stops
syncing and polling, and deletes the offline copies of the signed-in account
on the next start or poll. The app does not encrypt the copies; the client
clears them on sign-out.

The server keeps no record of devices. A client holds a **change cursor** per
space and asks what changed after it; nothing is registered, and a device that
is never seen again leaves nothing behind.

A client typically:

1. walks the **catalogue** of a space once (`GET …/sync/catalog`), which also
   hands it the change cursor taken before the walk began;
2. fetches the bodies of the pages it wants (`GET …/sync/pages`);
3. from then on asks `GET /api/sync/heads` for all its spaces at once, and
   `GET …/changes` only for a space whose head differs from its cursor;
4. when told `410 changes_expired`, walks the catalogue again and compares
   `rev`s, fetching only the bodies whose `rev` changed.

### Change cursor

An opaque string. It names a point in the index of one space: an epoch, which
is new every time the index is built from scratch (at an upgrade that changes
the index, `POST …/reindex`, or when the index does not match the files), when
the server opens an index that was not closed cleanly (a crash or a power cut
may have taken its last writes), and when the first look at the files after
the space is opened finds changes the index did not record (except after a
restore of an evicted space, see below, whose changes come from its remote);
and a sequence
number. Every write of a page to the index (added, its file changed,
renamed or moved, its links rewritten by the server, read again after a change
made outside the server) gives the page a new sequence number, higher than any
before; a page that is removed leaves a record of its removal. So "what changed
after a cursor" is one range read of an index, whatever the size of the space,
and a page that changed many times since is reported once, as it is now.

A cursor is good for as long as the index keeps its epoch and the server still
holds the removals after it: the last 100,000, and none older than 180 days.
Otherwise the answer is `410 changes_expired`, and the client walks the
catalogue again. A string that is not a cursor this server made is
`400 invalid`.

### Catalogue entry

The light summary that a catalogue and a change list hold for a page: what a
tree, a list, a search by title and a check of `rev` need, without the
excerpt, the properties or the body.

```json
{"id": "01JD3K8Q7Z", "path": "Tang/暮雪.md", "folder": "Tang", "title": "暮雪",
 "label": "暮雪", "untitled": false, "tags": ["玉兰", "黄昏"], "type": "",
 "created": "2025-12-03", "updated": "2026-11-21T08:30:00Z",
 "rev": "3b18e512dba79e4c8300dd08aeb37f8e728b8dad"}
```

The fields mean what they mean in a page summary (see "Page summary");
`title` and `created` are absent when the page has none. A page that was
removed is `{"id": "01JD3K8Q7Z", "deleted": true}` in a change list (never in
a catalogue).

### `GET /api/spaces/{space}/sync/catalog`

Needs `read`. Every page of the space, in the order the server holds them, a
batch at a time.

| name | meaning |
|---|---|
| `folder` | only pages in this folder and below it (repeatable, at most 50; the root, `""`, means all) |
| `limit` | 1–5000, default 2000 |
| `cursor` | the `next` of the previous batch |

```json
{"items": [/* catalogue entries */], "next": "opaque-or-empty",
 "changes": "change cursor", "tree": "k2f0…", "settings": "9a1c…"}
```

`changes` is in the first batch only (the one asked for without `cursor`): the
change cursor as it was before the walk began. A page that changes while the
client walks may be in the catalogue as it was or as it is; either way it is
in `GET …/changes?since=` that cursor, so nothing is missed. `tree` and
`settings` are as in a change list (below). `next` is `""` after the last
batch. A batch is one range read of the index; no file is read.

A `cursor` made for another set of `folder`s, or that is not one this server
made, is `400 invalid`. A cursor outlives changes to the space (it names a
position, not an offset), but not a rebuild of the index: then `410
changes_expired`, and the walk starts again.

### `GET /api/spaces/{space}/changes?since={cursor}&limit={n}`

Needs `read`. What changed in the space after the change cursor `since`
(required): the pages added, changed, renamed or moved, as catalogue entries,
and the pages removed, in the order they last changed.

| name | meaning |
|---|---|
| `since` | a change cursor (required) |
| `limit` | 1–5000, default 1000 |

```json
{"items": [{"id": "01JD3K8Q7Z", "path": "Tang/暮雪.md", "…": "…", "rev": "3b18e5…"},
           {"id": "01JD3K8Q80", "deleted": true}],
 "next": "change cursor", "more": false, "tree": "k2f0…", "settings": "9a1c…"}
```

- `next` is the cursor to keep: the client asks with it next time. It is the
  head the answer was read at (so `GET /api/sync/heads` gives the same string
  while nothing else changes), or, when `more` is `true`, the position of the
  last change in `items`. When `since` is already the head, `items` is empty and
  `next` is `since`.
- `more` is `true` when there were more changes than `limit`; the client asks
  again with `next` at once.
- Each page is in the answer at most once, as it is now. A page removed and
  then made again with the same id (moved out of the space and back, outside
  the server) is a catalogue entry, not a removal.
- `tree` changes whenever a folder is made, moved or removed, or the number of
  pages in a folder changes. A folder's `updated` in that answer can change
  without `tree` changing (a page was edited), so a client working offline
  derives each folder's `updated` from the catalogue entries' `updated`
  instead of from the tree it saved. The client then reads `GET …/tree` (and
  `GET …/tags` if it shows tags). `settings` changes whenever a file the server
  reads as settings changes (`.petalbook/space.yml`, `tags.yml`, the types and
  the templates); the client then reads again what it uses of them (`…/types`,
  `…/views`, `…/templates`, `…/tags`). Neither is a change of a page, so
  neither is in `items`.
- A change of a page's effective type that comes only from a folder binding
  is a change of `settings`, not of the page.

What counts is the index. In a space that follows a remote, changes made on
the remote arrive with the space's next pull, which happens only while the
space is open (see "Remote repository"). Changes made to the files outside the
server arrive when the index sees them (see "Maintenance").

Errors: `400 invalid` without `since`, or for a `since` or `limit` that cannot
be used; `410 changes_expired` (see "Change cursor").

### `GET /api/spaces/{space}/sync/pages?ids={id},{id},…`

Needs `read`. Several pages at once, for filling the device's copy: 1 to 100
page ids, comma-separated, each once.

```json
{"pages": [/* pages, as GET …/pages/{id} returns them, with the caller's draft */],
 "missing": ["01JD3K8Q81"], "rest": ["01JD3K8Q90"]}
```

- Each page is what `GET …/pages/{id}` returns, `draft` included when the
  caller has one (the drafts of the whole request are read in one query).
- `missing` lists the ids that are not pages of the space (any more). Left out
  when empty.
- An answer holds at most 8 MiB of pages; the pages that did not fit are listed
  in `rest`, in the order asked, for the next request. The first page always
  fits, whatever its size (a page file is at most 8 MiB). Left out when empty.

`400 invalid` for no ids, more than 100, an id given twice, or one that is not
in the form of a page id.

### `GET /api/sync/heads?space={id}&space={id}…`

The change cursor each space is at now, so that a client with many spaces can
ask once and then read changes only where something changed. 1 to 100
`space` parameters, each once.

```json
{"heads": {"01JD4A2K9P": "change cursor", "default": "change cursor"},
 "missing": ["01JD4A2K9Q"]}
```

- `heads` holds every space the caller may `read`; `missing` (left out when
  empty) every other one, not telling a space that does not exist from one the
  caller may not see. A disabled space is in `missing`, and so is every space
  of a disabled tenant.
- A head equal to the client's cursor means nothing changed; a different one
  means the client calls `GET …/changes`. A head is not a promise that there are
  changes: comparing it is cheap, reading the changes settles it.
- A space that has not been open since the server started answers the head
  its index was closed at, read from the index without opening the space. A
  space with no index, or whose index was not closed cleanly, has no head to
  give: its head is `""`, which is never a cursor, so the client asks for its
  changes (which opens it); from then on its head is known, also after it
  closes. A head read this way can be followed, when the space is opened, by
  a new epoch (see "Change cursor"): the client then gets `410` and walks the
  catalogue again.
- A person who reaches a space only as an administrator, without being a member
  of it, has no head for it (it is in `missing`): heads come from the
  membership the server holds in memory, and a client that can read such a
  space asks its changes directly.
- The answer never opens or pulls a space: a space that is closed (idle, or
  cleared from the disk by milestone 3.1) answers the head it had when it
  closed, from memory, or after a restart from its index as above. So a device polling heads keeps no space open, and
  a space that follows a remote and is closed shows the remote's changes only
  once someone opens it.

`400 invalid` for no `space`, more than 100, or one given twice.

### Evicted spaces

A space that milestone 3.1 evicted (see "Eviction") keeps its index, closed,
and the sync treats it like any other closed space:

- `heads` answers the head it was evicted at (from memory, or after a restart
  from its index, read without opening it), and never opens or restores it.
  A space with no index on disk answers `""`.
- `sync/catalog` and `changes` are answered from its index without restoring
  it. Their `settings` stamp is the one the space had when it was last open
  with its files (`""` for a space evicted before the server kept it).
- `sync/pages` reads the page files, so it restores the space first, as every
  other read of a page does: it waits up to 10 seconds, then answers `503
  restoring` with `Retry-After` (or `503 unavailable` when the remote cannot be
  reached), and the client tries again later.
- An index that cannot answer as it is (missing, or of another index version
  because the server was upgraded while the space was evicted) is not used for
  `sync/catalog`, `changes` or any other route answered from the index: the
  space is restored instead, exactly as for `sync/pages` (`503 restoring`
  while it takes longer than the wait). The old index is only looked at,
  read-only, and is replaced only by the rebuild from the restored files; it is
  never answered from empty. Such a rebuild starts a new epoch (`410` for older
  cursors), like any index that must be built again.
- While a restore puts the files in place (from the moment it closes the
  index-only reader until the space is open in full), the routes answered from
  the index answer `503 restoring` too, instead of reading the index the full
  space is about to own.
- A restore keeps the index: its epoch and sequence numbers stay, so a cursor
  from before the eviction stays good, and the pages that changed on the remote
  meanwhile are in the next `changes` with new sequence numbers. Only an index
  that was not closed cleanly, or one that must be built again, starts a new
  epoch (`410`). The `settings` stamp usually changes once after a restore
  (the files were written again).

### What a client is expected to do

Not enforced by the server, but what it is built for:

- Ask `heads` once a minute while the app is visible and online, and at once
  when it comes back to the foreground or the network returns. Do not poll
  `changes` for spaces whose head did not move.
- Walk a catalogue in batches one after another, not in parallel; fetch bodies
  at most 4 requests at a time. A walk that is cut off goes on with the `next`
  it had (the batch cursor outlives changes to the space).
- After a failure that is not the network (a `5xx`, say), wait before asking
  that space again, longer each time (the web app: one poll period, doubling,
  at most 30 minutes).
- On `404` from any of these for a space, remove that space's copy from the
  device (the caller can no longer read it). On `403 space_disabled`, keep the
  copy, show it as read-only and stop asking until the space is in `heads`
  again.

**What it costs**, in a space of 50 000 pages (measured with
`PETALBOOK_TIMING=1 go test -run TestChangeFeedInALargeSpace ./internal/space/`;
the numbers are in docs/design/milestones/phase-4.md, "4.1 as built"): a
`changes` with nothing new under a millisecond; a catalogue batch of 2000 about 15 to 20 ms; 100 bodies
about 20 ms on an SSD; `heads` for 100 spaces a few microseconds each, and no
disk (after a restart, about a millisecond once for each closed space, to read
its head from its index). The index grows by one column and one small table, under 1% of its size.
The `settings` stamp of an answer is made from the names, sizes and times of
the files in `.petalbook/`, `.petalbook/types/` and `.petalbook/templates/` (three
directory listings, as many entries as the space has settings, not as many as
it has pages).

## Maintenance

### `POST /api/spaces/{space}/reindex`

Rebuilds the index from the files. `200` with `{"pages": 1234, "ms": 850}`.
The index is otherwise kept current by the server's own writes and is rebuilt
automatically at start-up when it is missing or does not match the files.

The server does not watch the file system. Edits, additions, deletions and
moves made outside it show up in listings, search, the tree and tags after a
restart or a reindex. Reaching a page by id repairs the index sooner: a `GET`,
`PUT`, move or `DELETE` of an id whose file is no longer where the index
expects it re-reads the files at once, and an id the index has never seen
triggers a re-read at most once every 2 seconds (in between, the answer is
`404`). Reading, saving, moving and deleting always check the file's own id
before acting, so a request never touches another page's file.

A `.md` file with no header, a header that does not parse, no `id`, or an `id`
another file already uses is not served as a page. The server logs each one
at start-up, and the check page lists them (see "Checks").

## Tenants, registration and mail

Milestone 3.3. Accounts mode only. **Single-user mode has none of this**: there
are no tenants, no registration, no mail and no quotas, every endpoint of this
section answers `404`, and settings resolve site → space → personal with no
tenant layer between (in single-user mode there are no administrators, so in
practice only the space's own settings count, as before).

What 3.3 adds is the plumbing for many tenants and for public sign-up, built so
that a server with one tenant and registration closed (the state after the
upgrade, and the default) behaves and costs exactly as before:

- every user, space, group and invite belongs to a **tenant**; a **default
  tenant** exists, and everything that existed before 3.3 is in it;
- administrators are **site administrators** or **tenant administrators**;
- settings resolve **site → tenant → space → personal**, with locks;
- a tenant can open **registration** (off by default), which needs **outgoing
  mail** (off until an administrator configures SMTP), as do **email
  verification** and **password reset**;
- **quotas** are measured per person and per tenant; only the number of spaces
  is refused, the rest warns.

**Nothing sends mail until a site administrator configures and enables SMTP**
(see "Mail"). Until then the registration, verification and reset endpoints
answer as switched off, and no setting can open registration.

### Tenant

```json
{"id": "0000000000", "slug": "default", "name": "Default", "default": true, "disabled": false,
 "registration": {"open": false},
 "quota": {"people": null, "spaces": null, "bytes": null, "pages": null},
 "usage": {"people": 40, "spaces": 212, "bytes": 18304211000, "pages": 61234, "measuredAt": "2026-11-23T02:10:00Z", "over": []},
 "created": "2026-11-23T08:30:00Z", "updated": "2026-11-23T08:30:00Z"}
```

- `id` has the form of a page id. The default tenant's id is always
  `0000000000` and its slug `default`; it cannot be disabled or deleted.
- `slug`: 2 to 32 characters of `a-z`, `0-9` and `-`, starting with a letter or
  digit, unique. It is what the public endpoints name a tenant by (below). It may
  change; links that carry the old slug stop working.
- `name`: 1 to 80 characters.
- `disabled`: nobody of the tenant can sign in (`401 unauthenticated`, the same
  message as a wrong password), their sessions end at their next request, their
  API tokens and OAuth grants answer `401 token_owner_disabled`, and its spaces
  cannot be opened. Nothing is deleted; enabling brings everything back.
- `registration` is `{"mode": "open" | "invite" | "closed"}`, the effective value
  of `registration.mode` for this tenant (the site's value for the default
  tenant: see "Registration").
- `quota`: the tenant's totals, set by site administrators only; `null` is no
  limit. They are the settings `quota.people`, `.spaces`, `.bytes` and `.pages` of the
  tenant (see "Rules for a tenant") and `quota` here is read only. `usage` is what is measured (see "Quotas"): `measuredAt` is when a space
  of the tenant was last measured (`null`: never; a measurement that failed does
  not count), and `over` lists the totals it
  is over (`people`, `spaces`, `bytes`, `pages`).
- The host a tenant is reached on is reserved: in 3.3 every tenant is served on
  the one `--public-url`, and a public endpoint names its tenant by slug.

### Where a request's tenant comes from

- **Signed in** (session, API token, OAuth grant): the user's tenant. A user
  belongs to exactly one tenant and never changes it in 3.3. (3.7 replaces this
  with memberships and a current organisation; see "Several organisations per
  account".)
- **Public endpoints** (`GET /api/auth/providers`, `GET /api/register`,
  `POST /api/register`, `POST /api/password/forgot`): the `tenant` query
  parameter or body field, a slug; absent means the default tenant. An unknown
  or disabled slug is `404 not_found`.
- **Usernames stay unique on the whole server** (and from 3.6 email addresses),
  so signing in needs no tenant:
  `POST /api/login`, passkeys and single sign-on are unchanged, and the account
  that signs in decides the tenant.
- **New accounts**: `POST /api/setup` makes the first site administrator in the
  default tenant; an invite makes the account in the invite's tenant; a
  sign-in of a provider makes it in the tenant it lands in (the provider's
  `tenant`, below; default the default tenant), when that tenant's registration
  mode is `open`; registration in the tenant registered for.

### Data isolation

A request sees only its own tenant's users, spaces, groups, invites, audit rows,
API tokens and OAuth grants. A site administrator sees every tenant's, and may
narrow a list with `?tenant={id}`. An id from another tenant answers exactly
like an id that does not exist (`404 not_found`, `404 user_not_found`,
`404 group_not_found`), so a tenant cannot learn what another holds.

How it is held (this is a contract on the server's code, listed so it can be
reviewed and tested):

- `users`, `spaces`, `groups`, `invites` and `audit_log` have a `tenant_id`.
  Everything else belongs to one of them (members, drafts, remotes, favourites,
  tokens, grants, passkeys, identities) and is reached through it.
- Every store method that reads or lists those tables by anything a client
  sends takes a **scope** (one tenant, or every tenant for a site
  administrator); a method without one does not compile. The few that must look
  across tenants say why next to the code: a session or token by its id, a
  sign-in by username or identity, the background jobs.
  2.7's tables follow the same rule: a GitHub connection hangs off its user and
  a remote off its space, so they carry no `tenant_id`; the methods that reach
  across users (the renewal of every person's tokens, the App's counts and the revocation when the
  App is removed, both site administrators only) and the App's installations
  (site-wide) are on the same list with their reasons.
- **Links between rows are checked by the database write itself**: adding a
  member, a group to a space, a person to a group, or transferring a space is an
  `INSERT … SELECT` that joins both rows on the same `tenant_id`, so a missed
  check in the API cannot create a cross-tenant link; it writes nothing and the
  request answers as for an unknown id.
- **The per-request permission check does not change**: the 1.7/3.2 cache of a
  person's spaces only ever holds spaces of their tenant (memberships are
  same-tenant by construction), so no tenant comparison is added to the hot
  path. The tenant's `disabled` state is in memory (one small map, re-read on
  change), checked where the session is loaded.
- Caches keyed by user id or space id need no tenant in the key (ids are unique
  on the server); the settings cache is keyed by tenant. The daily text's
  external interfaces are site-wide and so is their cache.
- **Strength**: one process and one database, separated by query conditions.
  Whoever can read the server's disk or database reads every tenant. Nothing
  stronger is promised (no database per tenant, no encryption per tenant).

- 3.10's notifications (`notifications`, with the person's `tenant_id`, and
  `notification_seen`) and the preferences are the caller's own: every query
  names the caller's id and tenant from the session, no route takes the id of
  a person, and a row of another tenant is never listed or marked whatever id
  is sent (`TestNotificationsAreTheCallersOwnAcrossTenants`). Their routes are
  in the isolation table with the reason.

How it is tested (SQLite and PostgreSQL both):

1. A fixture with two tenants holding the same shapes of data (people, shared
   and personal spaces, local and synced groups, invites, tokens, grants, audit
   rows). Every route of `internal/api/routes.go` is called by tenant A's
   administrator, member and token with tenant B's ids: each answers as for an
   unknown id, and no list contains an id of B. A test fails when a route is
   added without an entry in this table.
2. A test over the store's package that lists the exported methods touching the
   five tables and fails for one that takes no scope and is not on the
   commented allowlist.
3. After every test of the fixture, one query asserts that no membership,
   group membership, space group or transfer joins two tenants.
4. The single-tenant suite runs unchanged: with one tenant nothing differs.

### Administrators

The User object gains:

```json
{"…": "…", "role": "admin", "siteAdmin": true, "tenant": {"id": "0000000000", "slug": "default", "name": "Default"}}
```

- `role` keeps its meaning inside the user's tenant: `admin` or `member`.
- `siteAdmin`: an administrator of the whole server. It needs `role: "admin"`.
  Only a site administrator can set `siteAdmin`
  (`PATCH /api/users/{id}`); the rule "at least one enabled administrator" of
  1.7 and 2.8 now counts enabled site administrators of tenants that are not
  disabled (one of a disabled tenant cannot sign in).
- A **tenant administrator** (`role: "admin"`, `siteAdmin: false`) has, inside
  their tenant, everything an administrator had before 3.3 except what is the
  server's own. A site administrator has all of it, in every tenant.

| Endpoints | Tenant administrator | Site administrator |
|---|---|---|
| `/api/users…`, `/api/invites…` | their tenant | every tenant (`?tenant=`; `POST /api/invites` may name `tenant`) |
| `/api/admin/spaces…`, `/api/admin/groups…`, `/api/admin/tokens…`, `/api/admin/grants…`, `/api/admin/usage` | their tenant | every tenant (`?tenant=`) |
| `GET /api/admin/audit` | their tenant's rows, without the site's kinds (security, providers, mail, tenants, site settings) | every row (`?tenant=`) |
| `GET /api/admin/tenants/{own}`, `PATCH` of its `name`, `…/settings` of their tenant | yes | every tenant |
| `/api/admin/security`, `/api/admin/providers…`, `/api/admin/mail…`, `/api/admin/daily…`, `/api/admin/providers/github…` (2.7), `/api/admin/status`, `/api/admin/settings…` (site layer, which holds machine access, tenancy, the sign-in and storage policy), `/api/admin/secrets…`, the tenant list, create, delete, `disabled`, `slug`, `quota`, and 3.1's `/api/admin/storage`, `PUT /api/admin/spaces/{space}/storage`, `POST /api/admin/spaces/{space}/evict` and a user's `remoteExempt` | `403 forbidden` | yes |

The daily text's tenant default is a tenant setting (see "Settings by layer"),
not `/api/admin/daily`, which stays the site's.

From 3.5 and 3.6 this table is replaced by the one in "Tenant autonomy"
("Updated administrators' table").

`GET /api/session` returns the user with these fields; the web app shows the
site-only pages of `/admin` to site administrators only.

### Tenancy: the setting `features.tenancy`

`features.tenancy` (boolean, default `false`, site only: whether tenants other
than the default one exist is the server's) is written with
`PUT /api/admin/settings/features.tenancy` (site administrators; see "Settings
by layer"). It is read from the settings' cache: a request that makes or lists
tenants costs no query for it. Switching it off does not touch tenants that
exist: they keep working, and no new one can be made (`403 feature_off`, `feature` `features.tenancy`).
`GET /api/admin/tenancy` and `PUT /api/admin/tenancy` are gone (`404`); the
number of tenants is `count` on the list below.

### `GET /api/admin/tenants?q={text}&cursor={cursor}&limit={n}`

The answer is `{"items": […], "next": "…", "count": 2}`: `count` is how many
tenants the server has, the default one included (it was `tenants` of
`/api/admin/tenancy`).

Site administrators. `{"items": [Tenant], "next": "…"}`, the default tenant
first, then by name; `q` keeps names or slugs containing it (ignoring case, as
for spaces). `limit` 1 to 200 (default 50). One query a page; `usage` is read
from the stored measurements.

### `GET /api/admin/tenants/{tenant}`

A site administrator, or an administrator of that tenant (another tenant's
answers `404 not_found`). `{tenant}` is the id.

### `GET /api/admin/tenants/{tenant}/overview`

The tenant's page in one call; who may read `GET …/tenants/{tenant}` may read it
(another tenant's answers `404 not_found`). `{"tenant": Tenant, "createdByName":
"…", "groups": n, "admins": [{"id", "username", "name", "siteAdmin",
"disabled"}], "adminCount": n, "featuresOff": ["features.tokens", …],
"recentAdminActivity": [{"id", "at", "kind", "category", "capacity", "actor",
"username"}]}`. The counts of people, spaces, pages and bytes are the stored
`tenant.usage`; `admins` are the oldest 10 (read through `users_admins`),
`recentAdminActivity` the newest 10 audit rows, which only those who hold
`tenant.audit.read` get (`[]` for the rest). No scan of the tenant's people,
spaces or pages.

### `POST /api/admin/tenants`

Site administrators; `403 feature_off` while `features.tenancy` is `false`.

```json
{"slug": "acme", "name": "Acme", "admin": {"username": "wu", "name": "Wu", "email": "wu@acme.example"}}
```

`201` with the tenant. `admin` is optional: when given, the answer also has an
`invite` (as `POST /api/invites` answers, role `admin`, 7 days) for the first
administrator of the tenant; the link is shown once, and nothing is mailed.
The invite **fills in** the form that accepts it: it has `username` (normalised
as a username is), `name` and `email` (each `""` when not given), `POST
/api/invites/check` answers them (below), and the account the invite makes gets
the `email` (not verified). Each of the three may be left out, `username`
too: the person then picks one when they accept (`POST /api/invites/accept`
needs it). They are checked as `POST /api/invites` checks them (an ASCII
address with something on both sides of the `@`, a name of 1 to 100 characters
with no control characters), before the tenant is made: `400 invalid` for one
that is not valid, and no tenant is made. `409 exists` for a slug taken, `409 slug_reserved` for a reserved one (admin, api, support, petalbook, login and the like);
`409 tenant_limit` at 1,000 tenants.

### `PATCH /api/admin/tenants/{tenant}`

`{"name": "…", "slug": "…", "disabled": true}`, each optional. A
tenant administrator may send `name` only (`403 forbidden` for the rest). From
3.6 the totals are the settings `quota.people`, `quota.spaces`, `quota.bytes`
and `quota.pages`, which a site administrator sets for the tenant with
`PUT /api/admin/tenants/{tenant}/settings/{key}` (a rule; they have no tenant
layer); this endpoint no longer takes `quota` (`400 invalid`), and the tenant's
`quota` shows the values in force, read only.
`200` with the tenant. `409 default_tenant` for disabling the default tenant;
`409 exists` for a slug taken; `409 conflict` when disabling the tenant of the
caller, or when no enabled site administrator would be left outside the tenant.

### `DELETE /api/admin/tenants/{tenant}`

Site administrators. Only an empty tenant: no users, spaces, groups or invites
(pending registrations are deleted with it). `204`. `409 tenant_not_empty` with
`{"users": n, "spaces": n, "groups": n, "invites": n}`; `409 default_tenant`
for the default tenant. 3.3 has no way to move people or spaces between
tenants.

### Sign-in providers and groups

- (3.3; from 3.6 providers are social or SSO, and a tenant may own SSO
  providers: see "Tenant autonomy".)
- Providers stay the server's (site administrators only) and every tenant's
  sign-in page lists the same ones. A provider gains `tenant` (an id, default
  the default tenant): where the provider's new accounts land, as that tenant's
  `registration.mode` allows. An identity signs in to its account whatever
  tenant that is in.
- A provider's groups mapping may point to groups of any tenant; at sign-in
  only the entries whose group is in the person's tenant are applied, the rest
  are ignored for that person (not an error, not audited).
- A group belongs to a tenant; group names are unique within a tenant (no
  longer on the whole server). `GET /api/groups?q=` finds only the caller's
  tenant's groups.

### Features

A **feature** is a whole thing a tenant may or may not have: a boolean setting
named `features.*` (and `mail.ownSender` and `accounts.multipleEmails`, which
are off by default), on unless a rule, a plan or the tenant says otherwise (see "Settings by layer"). Every route of the API says in
the server's route table which feature it belongs to, or that it belongs to
none, and one check answers for all of them: **while the caller's tenant does
not have the feature, the route is `403 feature_off` before its handler runs**
(and before a space is opened, so nothing is restored for a refused request).

```json
{"error": "feature_off", "message": "The API tokens feature is turned off for this organization.", "feature": "features.tokens"}
```

- The check is one read of the settings' cache for the caller's tenant (taken
  from the session or the token): no query. In single-user mode every feature
  is on.
- It binds everybody, a site administrator included, who is told about the
  tenant they are in. The one exception is a route that asks a permission key
  with a feature of its own (`tenant.data.view`: `features.adminView`): a holder
  of the key across the site is not bound by it, see "Permission keys".
- A route a person must still be able to reach while its feature is off does
  not belong to it: listing and revoking one's tokens and grants, the
  administration of the settings themselves.
- Routes that belong to a feature: `POST /api/account/tokens`
  (`features.tokens`); `GET`/`POST /api/oauth/consent` (`features.mcp`);
  `GET /api/sync/heads`, `…/sync/catalog`, `…/changes`, `…/sync/pages`
  (`features.offline`); `POST /api/admin/tenants` (`features.tenancy`);
  `POST /api/account/emails`, `…/emails/verify`, `…/emails/primary` and
  `DELETE /api/account/emails/{email}` (`accounts.multipleEmails`, a feature
  that is off by default);
  `POST`/`DELETE /api/admin/spaces/{space}/view` (`features.adminView`, through
  its key). Checks that depend on more than the switch stay where they are: a
  token or an OAuth access token is refused with `tokens_off` or `404` at `/mcp`
  when its owner's tenant has the feature off, registration is also held to mail
  being ready and answers before anybody is known, and a secret of a tenant is
  refused by the feature of the secret named in the path.
- `GET /api/settings` says which features the caller's tenant has, with an
  `ETag` (see below).

### Settings by layer

The general rule of docs/design/milestones/phase-2.md ("Layered settings and locking") as an API. A **setting** has a
key, a kind and the layers it exists in. Each layer either sets a value or
follows the layer above (`null`), and each layer above the last says what the
layers below it may do (`lower`):

```json
{"mode": "allow"}
{"mode": "restrict", "options": ["any", "github"]}
{"mode": "restrict", "max": 20}
{"mode": "lock"}
```

- `allow`: the layers below may set any valid value.
- `tighten` (3.6): a value at least as strict as the one above it, for a key
  that has an order of strictness; see "Forced, minimum, default".
- `restrict`: for a `choice` setting, one of `options`; for a `limit` setting
  (a number where smaller is stricter), at most `max`.
- `lock`: the layers below cannot set it.

A lower layer may only tighten what is above it: a tenant's `lower` must be at
least as strict as the site's (fewer options, a smaller `max`, or `lock`). A
value a lower layer set that a lock or a restriction now forbids is **kept**,
with `inEffect: false`, and counts again when the rule is lifted; nobody's
setting is lost. Space values stay in `space.yml` and personal ones in the
state database, as before. The effective value is computed once per (tenant,
settings revision, space settings version) and cached; a request does not walk
the layers.

The settings of 3.3 (others join this table as they are made):

| Key | Kind | Layers | Site default | Site's `lower` by default |
|---|---|---|---|---|
| `registration.mode` | choice: `open`, `invite`, `closed` | site, tenant | `invite` | `allow` |
| `registration.domains` | list of domains (`[]`: any) | site, tenant | `[]` | `allow` |
| `registration.perHour` | limit, 1 to 10,000 | site, tenant | 50 | `restrict`, `max` 50 |
| `legal.termsUrl`, `legal.privacyUrl` | an `https` address or `""` | site, tenant | `""` | `allow` |
| `quota.spacesPerPerson` | limit, 0 (none) to 10,000 | site, tenant, person | 3 (`--max-spaces`) | `allow` |
| `quota.bytesPerPerson` | limit in bytes, or `null` | site, tenant, person | `null` | `allow` |
| `quota.pagesPerPerson` | limit, or `null` | site, tenant, person | `null` | `allow` |
| `daily.own`, `daily.api` | boolean (what a space may do) | site, tenant | `true` | `allow` |
| `features.offline` | boolean (may people keep offline copies; see "Offline sync") | site, tenant | `true` | `allow` |
| `daily.default` | the shape of `/api/admin/daily`'s `default`; a tenant may name a space of its own tenant or an interface | site, tenant | `null` | `allow` |

- `daily.own` and `daily.api` are the 2.3 `allow` switches; `PUT
  /api/admin/daily` with `allow` and `PUT /api/admin/settings/daily.own` write
  the same record. The tenant's `daily.default` comes between the space's and
  the site's (2.3's "以后…插在空间和全站之间"); pasted lines stay site-only.
- `quota.spacesPerPerson` at the site is `--max-spaces` when the flag is given
  (then it is read only, `fromFlag: true`), else the stored value, 20 to begin
  with. The person layer is a site or tenant administrator's override on the
  user (`PATCH /api/users/{id}` with `quota`), not something people set for
  themselves.
- **Reserved for 3.1** (merged; its settings are now `storage.remotes.*`,
  `storage.localSpaces.maxBytes`, `storage.hosts` and `cache.*`, see "The storage policy"):
  `storage.remotes.require` (choice, ordered `null` < `any` < a provider's id,
  2.7b), `storage.remotes.graceDays` and `storage.localSpaces.maxBytes`
  (limits) join with layers site and tenant, and the site's `lower` defaults
  to `lock`, as 3.1 decided; a tenant may only tighten them. 3.1's
  `hosts.allowed` (what `--remote-host` sets) and its cache settings stay the
  site's only. **Reserved for 2.7**: none of
  the GitHub App's settings have a tenant layer.
- Security settings (`/api/admin/security`, providers, mail) are the site's
  only and are not in this table.

#### `GET /api/admin/settings`, `PUT /api/admin/settings/{key}`

Site administrators. `GET` answers every setting:

```json
{"settings": [{"key": "registration.mode", "kind": "choice", "layers": ["site", "tenant"],
               "choices": ["open", "invite", "closed"],
               "site": {"value": "invite", "lower": {"mode": "allow"}, "fromFlag": false}}]}
```

`PUT` takes `{"value": …, "lower": {…}}`, either or both. `200` with that
setting. `400 invalid` for a value of the wrong kind or out of range;
`404 not_found` for an unknown key; `409 conflict` for a setting set by a flag;
the checks of "Registration" for `features.orgCreation`. A change of `lower` that
makes tenants' values ineffective answers `affected: n` (how many tenants).

#### `GET /api/admin/tenants/{tenant}/settings`, `PUT /api/admin/tenants/{tenant}/settings/{key}`

A site administrator or the tenant's administrator. `GET` answers every setting
that can differ between tenants, with the site's part and the tenant's: those
that have a tenant layer, and those with layers `["site"]` that a site
administrator may set for one tenant (`forTenant: true` in `GET
/api/admin/settings`: `storage.assets.ownStorageExempt`, `exports.days`). A
site-only setting that must be the same for every tenant says why in
`serverWide` there (`recycle.days`, `exports.maxBytes`, `users.reuseUsernames`,
`users.emailNonAscii`) and is not in this list.

```json
{"settings": [{"key": "features.offline", "kind": "boolean", "layers": ["site", "tenant"],
               "site": {"value": true, "lower": {"mode": "allow"}},
               "tenant": {"value": true, "lower": null, "inEffect": false, "siteLocked": true,
                          "siteRule": {"value": false, "lower": {"mode": "lock"}, "inEffect": true}},
               "effective": false},
              {"key": "quota.spacesPerPerson", "kind": "limit", "layers": ["site", "tenant", "person"],
               "site": {"value": 20, "lower": {"mode": "allow"}},
               "tenant": {"value": 15, "lower": null, "inEffect": false, "siteLocked": false,
                          "siteRule": {"value": null, "lower": {"mode": "restrict", "max": 10}, "inEffect": true}},
               "effective": 10}]}
```

`tenant.value` and `tenant.lower` are the tenant's own. `tenant.siteRule` is the
**site's rule for this one tenant**, absent when the site has none: a `value` in
place of the site's (`null`: the site's), and a `lower` the tenant's layer is
held to in place of the site's (`null`: the site's). It is the site's own
choice for that tenant, so it is not held to the site's `lower` for tenants;
only the site's lock over everyone beats it (`siteRule.inEffect: false`, kept).
This is how the site decides what one organisation may have: "Acme has no
offline copies" is `features.offline` locked to `false` for Acme, "at most 10
spaces a person in Acme" is `quota.spacesPerPerson` restricted to `max: 10` for
Acme. `siteLocked` says the rule is a lock. Strongest first: the site's lock
over everyone > the site's rule for the tenant > the tenant's value > the site's
value for the tenant > the site's value.

A restriction bounds `effective`, not only what the tenant may store: when the
value in effect would be one the restriction the tenant's layer is held to
forbids (the site's `lower`, or the rule's), it is the nearest one allowed. For
a limit that is `max` (also in place of `null`, no limit); for a boolean,
`false`; for a choice, the next stricter allowed choice (`choices` are loosest
first), or the strictest allowed when every allowed one is looser. Above, the
site's 20 for a tenant held to `max: 10` is 10, and the tenant's own 15 is kept,
not in effect.

`PUT` takes `{"value": …, "lower": {…}}` for the tenant's own layer; `value:
null` follows the site. A site administrator may also give `"siteRule":
{"value": …, "lower": {…}}` (either or both; `null` removes the rule), or its
short form `"siteLock": true` (the rule becomes a lock of `value`, or without
one of `effective` as it is then, so locking never changes what the tenant
sees) or `"siteLock": false` (the rule is removed). The tenant's own value stays as it was beside the rule. A tenant
administrator giving either gets `409 setting_locked`, and so does its write
while the site's rule for the tenant is a lock. `409 setting_locked` when the
rule the tenant's layer is held to locks (a site administrator is refused too:
lift it first); `409 setting_restricted` for a value outside its `options` or
above its `max`, or a `lower` looser than it. A `siteRule` whose `value` is
above the `max` it is held to (its own `lower`, else the site's) is `400
setting_above_max`, not stored and capped; so is a site `value` above the
site's own `max` in `PUT /api/admin/settings/{key}` (raise the maximum first).
Lowering a `max` alone below a value still bounds `effective` as above.

A setting with layers `["site"]` and `forTenant` has only the site's rule: a
site administrator's `value` there is the site's value for the tenant (the
rule's), `lower` is `400 invalid`, and the tenant's administrator gets `409
setting_locked`. A server-wide setting is `404 not_found` here.

#### `GET /api/settings?space={space}`

Any signed-in user; `space` optional (needs `read`). What the caller may know of
the settings in their context, made from their tenant's cached settings (no
query):

- `features`: every feature (`features.*`, `mail.ownSender`, see "Features") as
  the caller's tenant has it, `true` or `false`. A client shows or hides what
  needs a feature from it (the web app's `useFeature`). In single-user mode every
  feature is `true`.
- `settings`: the effective value of every setting that has a space or person
  layer, and whether that layer can change it, so a client greys out "set by the
  administrators".

```json
{"features": {"features.tokens": true, "features.offline": false},
 "settings": [{"key": "daily.own", "effective": false, "space": {"canChange": false, "inEffect": false, "source": "tenant"}}]}
```

The answer carries the version of the tenant's settings as its `ETag`
(`Cache-Control: private, no-cache`): a request with that `If-None-Match` is
`304` with no body, and costs one read of the settings' cache. The version
changes when the tenant's settings are computed again (a write by the tenant or
the site, a rule's window starting or ending, a change by another process), and
is never one another tenant or another run of the server answers.

`source` is the layer (`site` or `tenant`) whose lock, or whose value that
switches it off, keeps the layer from changing it; absent when nothing does.

`storage.assets` is also in the list, as `{"key": "storage.assets", "effective":
"repository", "allowed": ["space", "repository"], "locked": false}`: the default
a new space of the caller's tenant gets, what the owner may ask for, and whether
a lock leaves no choice. The web app's new-space dialog starts its attachments
box from `effective`, sends `storage.sync.assets` only when the person changes
it, disables the box when `locked`, and hides it when `allowed` has no
`repository`.

`storage.mode` is in the list too, as `{"key": "storage.mode", "effective":
"server", "allowed": ["server", "own", "tenant"], "locked": false, "missing":
["tenant"]}`: the default a new space of the caller's tenant gets, the modes its
creator may pick (see "Storage modes"), whether a lock leaves no choice, and the
allowed modes that cannot be used now (`tenant` while the organisation has no
storage target). The new-space dialog offers only what is allowed and usable,
starts at `effective`, and sends `storage.mode` only when the person picks
another; a space made with a repository is `own` without being told.

### Mail

Site administrators. Outgoing mail is what verification, password reset and
registration use; **it is off until configured**, and nothing in 3.3 sends a
message on its own initiative except as a reply to a person's request (a link
they asked for) or an administrator's test.

#### `GET /api/admin/mail`, `PUT /api/admin/mail`

```json
{"enabled": false, "host": "smtp.example.com", "port": 587, "security": "starttls",
 "username": "notes@example.com", "hasPassword": true,
 "from": "notes@example.com", "fromName": "PetalBook", "replyTo": "",
 "status": {"ok": true, "checked": "2026-11-23T08:00:00Z", "message": ""},
 "queue": {"waiting": 0, "sentLastHour": 12, "failedLastHour": 0}}
```

- `security`: `starttls` (the default; the server refuses to go on if the
  server does not offer it), `tls` (TLS from the first byte, usually port 465),
  or `none`, allowed only when `host` is a loopback address (a local relay). The
  server's certificate is always verified; there is no option to skip that.
- `username` and the password (write-only `password`; absent keeps the stored
  one, `""` clears it) are sent with `PLAIN` over TLS only. The password is
  sealed with the server's key (`secret.key`) like other secrets.
- `from` is required to enable; `fromName` (at most 60 characters) and
  `replyTo` are optional. One sender for the whole server in 3.3.
- `enabled: true` needs `host`, `from` and `--public-url` (the links point to
  it). `PUT` takes any of the fields; all are checked before anything is stored.
- Mail waits in a queue **in memory** (at most 1,000 messages), sent one
  connection at a time, each tried up to 3 times over 10 minutes and then
  dropped and logged. A restart loses what is waiting; every message is a link
  the person can ask for again, so nothing that cannot be redone is lost, and
  no token sits in the database in readable form.
- Messages are plain text plus a simple HTML part, in the language of the
  request that caused them (`lang` in the body, else `Accept-Language`, else
  English), from templates in the i18n files.

Refusals: `400 invalid`, `400 mail_needs_public_url`, `400 mail_insecure`
(`security: "none"` to a host that is not loopback), `400 mail_from_required`.
A change writes `mail_changed` to the audit log (the password as `"changed"`).

#### `POST /api/admin/mail/test`

`{"to": "admin@example.com", "lang": "en"}` (`lang` optional). Sends one message now, waiting up to 15 seconds,
to the address the administrator typed, and answers `200` with
`{"ok": false, "message": "535 authentication failed"}` (the SMTP server's
answer, never the password) or `{"ok": true}`, and updates `status`. Works also
while `enabled` is `false`, so the settings can be tried before anyone gets
mail. At most 10 an hour (`429 rate_limited`). Audit `mail_test`.

### Registration

Whether an account is made is the **registration mode**, `registration.mode`
(site and tenant, `open`, `invite` or `closed`, default `invite`, the site's
`lower` `allow`):

- **Two scopes.** The site's value governs the people who belong to no
  organisation: the default tenant (with tenancy off, everyone). Each
  organisation (a tenant that is not the default) has its own, set by its
  administrators; it is **not bounded by the site's**, and still follows the
  settings machinery (a site administrator may set a rule for one tenant, a
  plan's rule may carry one). An organisation that has set none, and has no
  such rule, is `invite`: **the site's own value never flows into an
  organisation** (a tenant's view of the setting shows `invite` as its default). An organisation's mode governs every way an
  account is made into it: its own SSO, the site's social sign-in that lands in
  it, invites into it, and its domain joins.
- `open`: any way may make an account. Password sign-up still needs mail and
  `--public-url`, and `registration.domains` and `registration.perHour` apply.
- `invite`: only someone holding a valid invite link makes an account, by
  password or by any external sign-in; a password sign-up, or a sign-in without
  an invitation, is refused: `403 invite_required` (`?sso_error=invite_required`).
  For an organisation, a domain's join mode (`auto` or `request`) is its
  administrators' own explicit opt-in and keeps working under `invite`.
- `closed`: nothing makes an account. Invite links stop working too (accepting
  one answers `403 registration_closed`), the domain joins are off (no automatic
  join, no new request; a sign-up of such an address is then an ordinary one
  into the tenant the page names). Requests already pending stay approvable:
  approving is an administrator's act. The first administrator (`POST
  /api/setup`, the setup token) and the command line are exempt. Under `closed`
  the way in is an administrator making the account by hand
  (`POST /api/admin/users`, "Making an account by hand"): that is an
  administrator's act too, so no mode gates it.
- **Every way is held to `quota.people`**: an account that would take a tenant
  over it is refused, `409 people_limit` (`?sso_error=people_limit`), whichever
  way it is made (accepting an invite, confirming a registration, approving a
  request, a sign-in, a person restored from the recycle bin, `user create` on
  the command line). The count is taken in the transaction that makes the
  account, so parallel accounts cannot pass it together. Only the first account
  of a server and the administrator of an organisation that is being made are
  not counted against a limit that is nobody's yet.
- **Organisation creation is another thing**: `features.orgCreation` (below,
  default off) lets a person make an organisation with the registration, whatever
  the mode; the organisation is an ordinary tenant afterwards.
- There is no per-provider switch: whether a sign-in makes an account is only
  the mode of the tenant it would land in (see "Which account an identity signs
  in to"). A tenant that wants its SSO to make accounts and nothing else sets
  `open` together with `signin.ssoOnly`: password sign-up is then refused
  (`409 sso_required`) while the SSO creates.

Password sign-up is possible when the effective mode of the tenant is `open`,
mail is enabled and the server has `--public-url`. Opening the mode is never
refused for want of mail (the accounts a sign-in makes need none); only
`features.orgCreation` is (below). Closing or tightening the mode, switching mail
off, or deleting the tenant deletes the pending registrations the new state does
not allow (all of the tenant's under `closed`; those that are not a domain join
under `invite`), so their links answer `404 not_found`.

#### `GET /api/register?tenant={slug}`

Public. `Cache-Control: no-store`. Answered from memory.

```json
{"open": true, "mode": "open", "tenant": {"slug": "default", "name": "Default"},
 "domains": ["example.com"], "termsUrl": "https://example.com/terms", "privacyUrl": "",
 "password": {"minLength": 8}}
```

`open` says whether a person may sign up here with a password (the mode is
`open` and mail is on); `mode` is the tenant's registration mode, for the words
to a person who cannot (use the link in your invitation, or ask an
administrator). When `open`, `orgCreation.open` and `domainJoin` are all `false`
only those fields are there. `domainJoin` (3.6 d5) is true when some verified domain
joins people to its organisation at sign-up: an address on it registers there
even while `open` is false (see "Joining an organisation"). `GET /api/auth/providers`
gains `register` (this boolean, or `domainJoin`) and `forgot` (see "Password
reset"), and takes the same `?tenant=`, so the sign-in page shows the links.

#### `POST /api/register`

```json
{"tenant": "default", "username": "tang", "name": "Tang", "email": "tang@example.com",
 "password": "…", "acceptTerms": true, "captcha": {"id": "…", "answer": "…"}, "lang": "zh-Hans"}
```

The server checks, in this order, and stops at the first refusal:

1. The address's requests this minute (the shared allowance of 2.8):
   `429 rate_limited`.
2. The mode of the tenant: `403 invite_required` under `invite`,
   `403 registration_closed` under `closed` (and while mail is off). An address
   whose domain joins an organisation (3.6 d5) registers in that organisation
   instead, under `open` and `invite`; under `closed` the join is off; see
   "Joining an organisation".
3. The `register` captcha context: `400 captcha_failed`.
4. The fields: `400 invalid` (username form as for a User, name 1 to 80
   characters, the password rule of "Signing in", an address of at most 254
   bytes, ASCII only, that parses as one plain `local@domain`; see "Email
   addresses are ASCII").
5. `registration.domains` not empty and the address's domain not in it (a
   domain also allows its subdomains): `400 email_domain_not_allowed`.
6. A terms or privacy address set and `acceptTerms` not `true`:
   `400 terms_required`.
7. The username taken by an account or by a pending registration:
   `409 exists`. (Usernames are not secret: they are shown to fellow members.)
8. The tenant's registrations in the last hour at `registration.perHour`, or
   1,000 pending: `429 rate_limited`.

Then `202` with `{"sent": true}`, **always the same** whether or not a message
goes out, so the answer never tells whether an address has an account:

- if an enabled account of the tenant already has this verified address (not
  verified by a tenant administrator alone, see "Password reset"), no account
  is made: the username is held for 24 hours by a registration whose link is
  never sent, as it would be for a free address, so asking for the same
  username again answers `409 exists` either way; and the account's stored
  address (not the one typed) gets "you already have an account" with a link
  to reset the password;
- if the address got 3 messages of this server in the last hour, or the caller's
  address 5, nothing is sent (and nothing is said);
- otherwise a **pending registration** is stored (the password already hashed,
  the username reserved) and the address gets a link
  `<public url>/register/confirm#<token>`, valid 24 hours.

The account is made only when the link is followed: an address nobody controls
never becomes an account.

#### `POST /api/register/confirm`

Public. `{"token": "…"}`. Counts against the address's requests this minute.
`201` with `{"user": {…}}` and a session cookie: a `member` of the tenant with
`email` and `emailVerified: true`, the acceptance of the terms recorded (when and
which addresses), and a personal space, as for an invite. A registration that
joined by its address's domain adds `"joined": {"slug", "name"}`; one whose
domain asks for approval makes no account and answers `202 {"request": …}`
(see "Joining an organisation"). Refusals:
`404 not_found` for a token unknown, used or expired (a failure of the
address); `403 invite_required` or `403 registration_closed` when the mode
tightened since (pending registrations are not honoured after it: the mode is
asked again here); `409 people_limit` when the tenant is full; `409 exists` when the username
was taken in between by an administrator. `registered` goes to the audit log.

#### Organisations people create themselves

`features.orgCreation` (site level only, default **off**) lets a person make an
organisation of their own with the registration; the registration mode does not
govern it. Turning it on is refused with
`409 needs_tenancy` while `features.tenancy` is off, `409 needs_mail` while mail
is not enabled, and `409 sso_only_with_registration` while `ssoOnly` is stored
on; turning off tenancy or mail closes it in effect. The other settings, all
site level (a tenant does not exist yet; creating one takes the site's
resources): `orgCreation.perPerson` (0 to 100, default 3: how many organisations one account
created, counted by `createdBy`; it becomes meaningful with signed-in creation in
3.7, because in 3.6 a registration creates at most one), `orgCreation.perHour`
(for the whole server, default null: no cap) and `orgCreation.plan` (a plan's
slug, `""` for none; `400 invalid` for a plan that does not exist).

**Whether a new tenant's administrators get the audit log.** `features.tenantAudit`
(site, with a rule per tenant; on when no rule says otherwise) is the feature of
the key `tenant.audit.read`: while it is off for a tenant, its administrators
get `403 feature_off` from `GET /api/admin/audit`, `…/kinds` and
`…/download`, and no audit in the overview or the console. It is the site's
alone to set (no tenant layer). `tenants.newAuditDefault` (site only, default
**off**) is the value a tenant is made with: when a tenant is made (by a site
administrator or by a person registering an organisation) a locked manual rule
for `features.tenantAudit` with that value is written in the same transaction,
together with its `setting_changed` row (the tenant as subject, the capacity of the creation).
Changing the default later touches no tenant that exists; a site administrator
sets one tenant with `PUT /api/admin/tenants/{tenant}/settings/features.tenantAudit`
(`siteRule` or `siteLock`). Tenants that had no rule (those before the switch)
keep the audit. The rows are written either way, and site administrators read
every tenant's rows as before.

- `GET /api/register` also has `orgCreation: {"open": true|false}`; `open` is the
  joining of the tenant, as before. When neither is open the answer is
  `{"open": false, "orgCreation": {"open": false}}`. `GET /api/auth/providers` has
  `orgCreation` (the same boolean) next to `register`.
- `POST /api/register` takes `org: {"name": "Acme", "slug": "acme"}` (`tenant` is
  then ignored). The switch is checked where registration openness is
  (`403 registration_closed`); the domain list of a tenant does not apply. The
  slug follows the administrator's rule (2 to 32 of a-z, 0-9 and -, starting with
  a letter or digit; the name 1 to 80 characters): `400 invalid`, and
  `409 slug_taken` when a tenant or a pending registration holds it (slugs are
  public, so this leaks nothing) and `409 slug_reserved` for a reserved address
  (admin, api, support, petalbook, login and the like; the administrator's
  `POST /api/admin/tenants` refuses the same list), before any address is looked at. The caps
  (`perPerson` 0, the server's 1,000 tenants, `perHour`, 1,000 pending) are
  checked before anything is written (`perHour` is counted again at confirm, from the
  organisations made in the last hour: `403 org_limit`); over one the answer is still `202`, no
  username or slug is held, and the address gets a message that says no
  organisation was made.
- `POST /api/register/confirm` makes, **in one transaction**, the tenant
  (`origin: "self"`, `createdBy` the new person, the plan of `orgCreation.plan`)
  and the account as its `admin` (never a site administrator) with its personal
  space. The answer has `tenant: {"slug", "name", "renamed"}`; when the slug was
  taken in the meantime a number is added and `renamed` is `true`.
  `403 org_limit` when the person's cap (re-read in the transaction), the hour's
  cap or the switch no longer allows it; nothing is made and the link still stands.
  `tenant_created` goes to the audit log with `detail.origin` `self` (`admin` for
  a site administrator's).
- `GET /api/admin/tenants` rows have `origin` (`default`, `admin` or `self`) and
  `createdBy` (a user id; an administrator's own for `admin`), and take
  `?origin=` (`400 invalid` for another value).

A new tenant, whoever makes it, copies no settings: it follows the site's
defaults until its administrators say otherwise, and gets no shared space.

### Secrets

One table, `secrets`, holds every secret the server keeps for the site and for
each tenant, sealed with the server's key (`secret.key`) and bound to its scope
and name: the GitHub App's private key, client secret and webhook secret
(`github.appKey`, `github.clientSecret`, `github.webhookSecret`), the mail
password (`mail.password`), the captcha provider's secret (`captcha.secret`),
and what a tenant's own sender, sign-in and App will need. It is in the state
database, so a backup or a move of it carries them. Settings hold policy only: no
read of a setting can return a secret.

**Write-only.** An answer says whether a secret is set and when it changed,
never its value; the value goes only to the code that uses it. Nothing logs it
and no audit row holds it (`secret_changed` has the name and `set` or `cleared`).

A site administrator manages the site's; a tenant administrator only their own
tenant's, and only those the site allows the tenant: `mail.password` needs the
feature `mail.ownSender` for the tenant, else `403 feature_off` (with `feature`);
the others are the site's alone for now (`404 not_found` for a tenant).

#### `GET /api/admin/secrets`, `GET /api/admin/tenants/{tenant}/secrets`

`{"secrets": [{"name": "mail.password", "set": true, "updatedAt": "2026-11-26T08:00:00Z"}]}`
for the names the scope may hold (`updatedAt` absent while not set). The site's
needs `site.settings.manage`, a tenant's `tenant.settings.read` in that tenant
(a site administrator in any; another tenant's is `404`).

#### `PUT /api/admin/secrets/{name}`, `PUT /api/admin/tenants/{tenant}/secrets/{name}`

Takes `{"value": "…"}` (not empty, at most 16 KiB) and answers the list as `GET`.
The GitHub App's secrets answer `409 conflict`: they are changed with the
connection (`/api/admin/providers/github`), which checks them with GitHub. The
mail password and the captcha secret are also written by their own forms
(`PUT /api/admin/mail` `password`, `PUT /api/admin/security`
`captcha.secret`).

#### `DELETE /api/admin/secrets/{name}`, `DELETE /api/admin/tenants/{tenant}/secrets/{name}`

Clears it (`200`, with the list; also when there was none). A tenant may clear
what it left when the site took the feature away.

### Email verification

For any account with an `email`. It is what makes an address count for
password reset and for 2.8's merge by verified address.

#### `POST /api/account/email/verify`

Signed in. `{"lang": "zh-Hans"}`. Sends a link
`<public url>/verify-email#<token>` (valid 24 hours) to the account's current
`email`. `202 {"sent": true}`. `409 mail_not_configured`; `400 invalid` when
the account has no email; `400 email_not_ascii` (from 3.6) for one stored with characters outside ASCII while
`users.emailNonAscii` is off; `409 email_verified` when it is verified already
(not when only a tenant administrator marked it: the person may confirm it,
which clears the mark); `429 rate_limited` after 3 in an hour.

#### `POST /api/email/confirm`

Public, so the link works on another device. `{"token": "…"}`. `204`: the
account's `emailVerified` becomes `true` if its `email` is still the one the
link was sent to. `404 not_found` for a token unknown, used, expired, or made
for an address the account no longer has. Audit `email_verified`.

`PATCH /api/account` with a new `email` (which clears `emailVerified`, as
before) also sends a note to the old address when that one was verified ("the
address of your account was changed"), with no link in it.

Any change of an account's `email` (the person's `PATCH /api/account`, an
administrator's `PATCH /api/users/{id}`) makes every verification and reset
link sent before it unusable; a reset link also answers `404 not_found` once
the account's `email` is no longer the one it was sent to.

#### More than one email address

Behind the layered setting `accounts.multipleEmails` (a feature, off by
default: the site allows, restricts or locks it, and a tenant decides inside).
While it is on, a person may have up to 10 addresses on their account. One is
the **primary**: the UPN, shown as the account's `email`, where notices go.
Every address is unique on the server (the key of "Signing in by email
address"), and the rule of who holds an address applies to each.

- **Add, then confirm.** `POST /api/account/emails` with
  `{"email": "…", "lang": "zh-Hans"}` sends the address a verification link
  (`/verify-email#<token>`, 24 hours) and the primary a note with no link.
  `202 {"sent": true}`. An address nobody has becomes an unverified address of
  the account at once; it counts (signs in, resets, meets a domain
  requirement) only once its link is followed (`POST /api/email/confirm`). An
  address another account holds unverified (or only a tenant administrator
  marked) has no row until it is proved: it shows in `emails` as
  `"pending": true` (not verified, never primary), and its link, a message of
  its own that names the account and says that confirming moves the address
  away from any other account that has it unconfirmed, moves it to this
  account (that account is told, as `email_released`). Whoever proves an
  address first has it; a later link is `409 email_taken`. `409 email_taken`
  for an address another account holds verified, or a deleted account keeps;
  `409 exists` for one already on the account; `409 email_limit` at 10
  addresses; `400 invalid`, `400 email_not_ascii`, `409 mail_not_configured`;
  `429 rate_limited` by the mail caps (3 messages an hour to an address, 5 an
  hour from one caller).
- **Verify again.** `POST /api/account/emails/verify` with `{"email", "lang"}`
  sends a new link (the earlier link of that address stops working; links of
  the account's other addresses keep working). `202`. The primary is
  `POST /api/account/email/verify`'s. `409 email_verified` when the person
  holds the address already; `404 not_found` for an address the account does
  not have; the same caps.
- **Make primary.** `POST /api/account/emails/primary` with `{"email"}`:
  `200 {"user": {…}}`. Only an address the person holds verified (not one only
  a tenant administrator marked): `409 email_unverified` otherwise. The old
  primary stays as another address. The new and the old primary are told.
- **Remove.** `DELETE /api/account/emails/{email}`: `204`, with the links sent
  to it; for a pending claim it withdraws the claim (its link stops working). Never the primary: `409 primary_email` (make another primary first).
  The primary is told, and the removed address when it was verified.
- Every add, confirmation, primary change and removal is audited:
  `email_added`, `email_verified` (with `primary`), `email_primary_changed`
  (with `old`), `email_removed`. The notes have no link and are held to the
  cap of the address they go to, not the caller's; one over the cap is left
  out.
- **Signing in and resetting** take any address the account holds verified,
  the primary or another (`POST /api/login`'s `username` may be an address;
  `POST /api/password/forgot`'s `login`). A reset link sent to an address
  stops working once the account no longer signs in by it.
- **An organisation's domain requirement** (with the organisation part of
  3.6) is met by any address the account
  holds verified in one of its domains (or below one), not only the primary.
- **Off** (the default, or turned off later): the account has its primary
  address only, as before. Addresses added while it was on are kept, but not
  shown (`GET /api/account`, `GET /api/users/{id}`), not used to sign in or
  reset, and their links do not confirm them; the four routes above answer
  `403 feature_off`. Turned on again, they are back as they were.

#### Email addresses are ASCII

An `email` (registration, `PATCH /api/account`, an administrator's
`PATCH /api/users/{id}`, a provider's claim) with any character
outside ASCII is refused with `400 invalid` (from a provider: the account gets
no address). Addresses are compared with A to Z made lower case and nothing
else, in every lookup (registration, password reset, the merge by verified
address of 2.8, `registration.domains`). Unicode case mapping would make
look-alikes equal: `İ` (U+0130) and the Kelvin sign (U+212A) lower to `i` and
`k`. An address stored before 3.3 with such characters is kept as it is, but
matches no lookup and gets no account mail (no reset link, no verification
link): the person or an administrator sets an ASCII address.

### Password reset

Off unless mail is enabled (and `--public-url` set). `GET /api/auth/providers`
says `forgot: true` when it is on.

#### `POST /api/password/forgot`

Public. `{"tenant": "default", "login": "tang", "captcha": {…}, "lang": "zh-Hans"}`.
`login` is a username or an email address. Checks: the address's requests this
minute (`429 rate_limited`), on (`403 password_reset_off`), the `forgot`
captcha context (`400 captcha_failed`). Then `202 {"sent": true}`, always the
same. A message goes out only to an enabled account of the tenant (any tenant
for a username) with a verified address, at most 3 an hour per account and 5
per caller's address. An address only a tenant administrator marked verified
(`email_verified_by` is `tenant_admin`) does not count here, as it does not for
2.8's merge: a tenant administrator could give a member their own address and
reset the password. The person verifies it with the link, or a site
administrator marks it.

- an account with a password gets a link `<public url>/reset-password#<token>`,
  valid 1 hour, usable once; asking again makes the earlier links unusable;
- an account without a password (made through single sign-on), or one whose
  password sign-in `ssoOnly` refuses, gets "sign in with {provider}" and no link:
  a reset never adds a password the person could not already use;
- an address several accounts of the tenant have verified gets one message
  with a link for each (at most 5).

`password_reset_requested` goes to the audit log for an account that got a link
(not for logins that match nothing).

#### `POST /api/password/reset/check`

Public. `{"token": "…"}`. `200` with `{"username": "tang", "expires": "…"}`, or
`404 not_found` (a failure of the address). For the page to say "this link has
expired" before the person types a password.

#### `POST /api/password/reset`

Public. `{"token": "…", "password": "…"}`. `204`. The password is set, every
session of the account ends, its username's and pairs' failures and locks are
cleared, and `password_reset` goes to the audit log. No session is started: the
person signs in, so `ssoOnly`, a required passkey and the rest apply as at any
sign-in. `404 not_found` for a bad token (a failure of the address);
`400 invalid` for a password that breaks the rule.

#### `POST /api/users/{id}/password-reset`

Administrators (in their scope). Sends the same link to the user's verified
address. `202`. `409 email_unverified` (also for an address only a tenant
administrator marked verified, whoever asks), `409 mail_not_configured`,
`409 conflict` for an account without a password.

### Quotas

Measured per person and per tenant: spaces owned, bytes, pages.

- **Whose**: a space counts toward its first owner (the person whose name is
  its default Git author, as in the administration's list: the earliest owner
  membership) and its tenant; a transfer, or any change of who the first owner
  is, moves it. Group members, editors, viewers and later owners count nothing.
- **Spaces** are counted when they are made, deleted or handed over, in the
  transaction that does it: the person's and the tenant's row change by the
  difference.
- **Bytes and pages** come from the background measurement of 1.7, which walks
  one space at a time at most every 6 hours; it now also stores the space's
  page count (one `COUNT` on the space's index: from the open space, or from the
  index file of a closed one without opening it, so a space with a remote is not
  pulled for it; a space whose index is gone keeps its earlier count). The
  measurement stores both in one transaction (`spaces.usage_bytes`,
  `spaces.page_count`) and adds the difference to the first owner's and the
  tenant's row of the `usage` table, so reading anyone's usage is one row.
- **Drift** is corrected from the rows themselves, by one grouped query a
  table: once when the server starts running (a crash may have left a
  difference) and once a day. A re-measure corrects a space's own numbers the
  same way, by the difference to what it held. Nothing on a request path walks
  files, counts pages, or recounts anything.
- **Reserved for 3.1**: a space that is partially cloned or evicted keeps the
  bytes of its last full measurement (3.1's `cloneBytes` and `indexBytes` are
  what the server holds, not what the space holds); 3.1's measuring writes
  `usage_bytes` only from a full working tree (the listing's `sizeBytes` may be
  written on its own), and the quota reads `usage_bytes`. On this branch the
  measurement is the 1.7 one, and `store.RecordMeasurement` is the one place
  that writes `usage_bytes`.
- **What is refused**: only spaces. `POST /api/spaces` over the person's limit
  (their own, else the tenant's `quota.spacesPerPerson`, else the site's) or the
  tenant's `quota.spaces` answers `403 quota_exceeded` with
  `{"quota": "spaces", "limit": 20, "used": 20}`. A limit of `0` is none.
  The check reads the person's usage row (and the tenant's, when it has a total):
  one primary-key lookup each, however many spaces there are. Bytes, pages and
  the tenant's people are **not refused** in 3.3: going over shows a notice to
  the person and the tenant's administrators see it in the list and the tenant,
  and `quota_exceeded` goes to the audit log once per crossing. Uploads keep
  3.1's `507 space_full` for its own limit; a byte quota that refuses later will
  answer `507 quota_exceeded`.
- **A personal space** is never refused for a quota and counts toward the
  person's and the tenant's totals.
- **Over** means more than the limit (at it is not). Whether a row is over is
  looked at after each measurement; after a change of whom a space is counted
  for (a space made or deleted, a transfer, an owner added, promoted, demoted or
  removed: the person it was counted for, the person it is now, and the
  tenant); after a new account (the person and the tenant's people); after a
  change of a person's limits or a tenant's totals, after a quota setting changes (everyone in its scope, one
  read of the usage rows in pages of 500), and after the recount; it is stored
  (`usage.over`), so `over=1` is an index range and a crossing is reported once:
  a row that stays over, and is looked at again, writes nothing.

#### `GET /api/account/usage`

Signed in.

```json
{"spaces": {"used": 3, "limit": 20}, "bytes": {"used": 52428800, "limit": null},
 "pages": {"used": 2100, "limit": null}, "measuredAt": "2026-11-23T02:10:00Z",
 "notices": [{"quota": "pages", "scope": "person", "limit": 2000, "used": 2100}]}
```

`limit` is `null` for none; `measuredAt` is `null` until a space of the person
has been measured. It is the time of the last measurement that found something
new to record, so it may lag the last measurement that found the same as before
(that one only moves the space in the measurement's queue). A **notice** is a
quota that is over: `scope` `person` for the person's own limit, `tenant` for
the tenant's total (`quota` is then also `people`). Three lookups of rows by key; the numbers are the stored ones.

#### `GET /api/admin/usage?tenant={id}&over=1&cursor={cursor}&limit={n}`

Administrators (in their scope: a tenant administrator's own tenant, a site
administrator's every tenant, or `?tenant=`: `404 not_found` for one that is not
theirs or does not exist). People by usage, largest bytes first, then by id:

```json
{"items": [{"user": {"id": "…", "username": "tang", "name": "Tang"}, "tenant": "0000000000",
  "spaces": 3, "bytes": 52428800, "pages": 2100,
  "limits": {"spaces": 20, "bytes": null, "pages": 2000},
  "own": {"spaces": null, "bytes": null, "pages": 2000},
  "ownInEffect": {"spaces": true, "bytes": true, "pages": true},
  "over": ["pages"], "measuredAt": "2026-11-23T02:10:00Z"}],
 "next": "52428800:01JD3K7A2M"}
```

`limits` are the limits in force (`null` none) and `own` the person's own
(what the edit form starts with). `ownInEffect` is `false` for an own limit
that a lock or a restriction put on the setting later forbids (see `quota` on
`PATCH /api/users/{id}`); it is `true` for one that is in force or not set. `over=1` keeps those over a limit (`over=0` or
none keeps all; anything else is `400 invalid`). `limit` is 1 to 200 (default
50). The cursor is what `next` says (`400 invalid` for any other). One query a
page over the usage table's index; the people's names are looked up for the page
only.

#### `quota` on `PATCH /api/users/{id}`

`quota: {"spaces": 50, "bytes": null, "pages": 100000}`: each key given is a
number (`spaces` 0 to 10,000, `0` meaning none; `bytes` and `pages` at least 1)
or `null` to follow the tenant and the site; the keys left out stay. `200` with
the user, whose `quota` is the person's own limits as they are now.

- `400 invalid` for an unknown key, a value that is no whole number in range, or
  a `quota` that is not an object.
- A tenant administrator may lower a person's limit, not raise it above the
  tenant's effective value (no limit is the highest): `403 forbidden`. Only a
  site administrator may. Both answer to a lock or a restriction the layer above
  put on the setting (`409 setting_locked`, `409 setting_restricted`): a tenant
  administrator to the stricter of the site's and the tenant's, a site
  administrator to the site's.
- A lock or a restriction put on the setting **after** the value was saved
  binds it by the same rule, that of the layer that saved it (a site
  administrator's value answers to the site's rule, a tenant administrator's to
  the stricter of the site's and the tenant's). Under a lock the own value is
  not in effect: the tenant's value applies. Under a restriction it counts up
  to the restriction's `max` (no limit is above any `max`). The value is kept,
  shown with `inEffect: false` (`ownInEffect` in `GET /api/admin/usage`), and
  counts again when the rule is lifted.
- A person of another tenant is `404 not_found`, a member `403 forbidden`.
- It is checked before anything else of the request is applied: a refused
  `quota` changes nothing, not the other fields either.
- Saving writes `user_changed` (with the new `quota`) to the audit log and looks
  at the person's numbers again.

### Abuse basics

- **Per-address allowance** (2.8's `limits.perMinute`, 10 by default): shared
  with sign-in by `GET /api/register`, `POST /api/register`,
  `POST /api/register/confirm`, `POST /api/password/forgot`,
  `POST /api/password/reset/check`, `POST /api/password/reset` and
  `POST /api/email/confirm`.
- **Failures**: a wrong token on the four token endpoints counts as a failure of
  the address (2.8's caps and lockout).
- **Captcha**: the `register` and `forgot` contexts of 2.8 are real: they appear
  in `GET /api/admin/security`'s `captcha.contexts` and in `GET /api/captcha`,
  each off by default, `always` or `after_failures` like the others (counted by
  the address).
- **Mail caps**: at most 3 messages an hour to one address, 5 caused by one
  caller's address, `registration.perHour` registrations a tenant an hour, 1,000
  pending registrations a tenant, 1,000 messages waiting on the server. Counts
  are in memory, like 2.8's.
- **Domains**: `registration.domains` limits who may register by address.
- Tokens are 32 random bytes, sent only in the link's fragment (never a query:
  not in logs or `Referer`), stored only as a SHA-256 hash, single use.

### Audit

These rows join those of "Audit log"; every row now has a `tenant` (`""` for
the site's own kinds):

| `kind` | When | `detail` |
|---|---|---|
| `tenant_created`, `tenant_changed`, `tenant_deleted` | a tenant was made, changed or deleted | `tenant`, `slug`, the changed fields |
| `tenant_console_opened` | a site administrator entered a tenant's scope (see "Acting scope"; at most once an hour per session and tenant) | `tenant` |
| `setting_changed` | a site or tenant setting changed | `key`, `layer`, `from`, `to` |
| `mail_changed`, `mail_test` | the mail settings changed; a test was sent | the changed fields (password as `"changed"`); `to`, `ok` |
| `registered` | a registration was confirmed | `email` |
| `user_created` | an administrator made an account by hand (`POST /api/admin/users`) | `role`, `siteAdmin`, `email`, `signIn` (`link`, `sso`), `mail` (`sent`, `none`, `limited`) |
| `email_verified` | an address was verified | `email` |
| `password_set_by_admin` | an administrator gave a person a temporary password | `temporary` |
| `password_reset_requested`, `password_reset` | a reset link was sent; a password was reset | `by` (`user`, `admin`; `welcome` for the first password of an account an administrator made) |
| `quota_exceeded` | a person or a tenant went over a quota (once per crossing) | `quota` (`spaces`, `bytes`, `pages`, `people`), `limit`, `used`, `scope` (`person`, `tenant`); the row's `user` is the person, and for a tenant's total the `tenant` is the one |

`POST /api/register` writes nothing until it is confirmed (made-up addresses do
not fill the log).

### Error codes of this section

| Code | Status | When |
|---|---|---|
| `tenant_limit` | 409 | 1,000 tenants |
| `tenant_not_empty` | 409 | deleting a tenant that has users, spaces, groups or invites (with the counts) |
| `default_tenant` | 409 | disabling or deleting the default tenant |
| `setting_locked` | 409 | setting a value the layer above locks |
| `setting_restricted` | 409 | a value outside the layer above's options or `max`, or a `lower` looser than it |
| `setting_above_max` | 400 | a site value (or the value of the site's rule for a tenant) above the `max` it is held to |
| `setting_until_past` | 400 | a rule for a tenant whose `until` has already passed (it would never count) |
| `setting_rule_needs_max` | 400 | a plan's or billing's rule with a value but no maximum or lock, for a limit the tenant also sets (`quota.*PerPerson`, `files.maxBytes`, `registration.perHour`): it would not cap the tenant |
| `registration_closed` | 403 | making an account while the tenant's `registration.mode` is `closed` (password registration or its confirmation, accepting an invite, an organisation's creation while it is off), or while password registration needs mail and mail is not enabled |
| `invite_required` | 403 | registering (or confirming a registration) while the tenant's mode is `invite` and no invitation or domain join applies; the same word is the `?sso_error=` of a sign-in |
| `email_domain_not_verified` | 400 | an account made by hand for an SSO-only tenant with an address no verified or routed domain of the tenant covers |
| `sso_default_tenant` | 400 | an account made by hand in the default tenant while single sign-on is required of it |
| `site_admin_explicit` | 400 | an administrator of the default tenant made by hand without saying whether it is a site administrator (`siteAdmin`) |
| `people_limit` | 409 | an account that would take its tenant over `quota.people`: accepting an invite, confirming a registration, approving a join request, restoring a person from the recycle bin (the item stays in the bin); `?sso_error=people_limit` for a sign-in |
| `password_reset_off` | 403 | asking for a reset while mail is not enabled |
| `mail_not_configured` | 409 | opening registration, asking for a verification, or sending a reset while mail is not enabled |
| `mail_needs_public_url` | 400 | enabling mail without `--public-url` |
| `mail_insecure` | 400 | `security: "none"` to a host that is not loopback |
| `mail_from_required` | 400 | enabling mail without `from` |
| `sso_only_with_registration` | 409 (opening `features.orgCreation`), 400 (`PUT /api/admin/security`) | `ssoOnly` and organisations people create together (password sign-up makes them) |
| `email_domain_not_allowed` | 400 | the address's domain is not in `registration.domains` |
| `terms_required` | 400 | `acceptTerms` missing while a terms or privacy address is set |
| `email_verified` | 409 | asking to verify an address that is verified |
| `email_unverified` | 409 | an administrator sending a reset to an unverified address |
| `quota_exceeded` | 403 (spaces); 507 reserved for bytes | over a quota that is refused (with `quota`, `limit` and `used`) |
| `captcha_failed` | 400 | as in "Signing in", for the `register` and `forgot` contexts |
| `rate_limited` | 429 | the allowances and caps of "Abuse basics", with `Retry-After` |
| `not_found` | 404 | a token unknown, used or expired; a tenant slug unknown or disabled; another tenant's id |

### Storage

SQLite and PostgreSQL both. The first part of the milestone (tenants, layered
settings, isolation) is below, and the parts that add registration, mail and
quotas follow it.

- `tenants` (id, slug unique, name, name_key, disabled, quota JSON, created,
  updated), with the default row `0000000000` / `default`.
- `users`, `spaces`, `groups`, `invites`, `audit_log` gain
  `tenant_id TEXT NOT NULL DEFAULT '0000000000'`; `users` gains `site_admin`;
  `invites` gains `site_admin` (the invited administrator is one of the server);
  `sign_in_providers` gains `tenant_id` (where the provider's accounts land).
- Indexes `(tenant_id, created)` on users and spaces, `(tenant_id, id)` on
  the audit log, `(tenant_id)` on invites; groups have the unique index below.
- Group names unique per tenant: the unique `name_key` becomes unique
  `(tenant_id, name_key)`.
- New: `layered_settings` (layer, layer id, key, value, lower, updated).
- Added by the second part: `registrations` (pending: id, tenant, username
  unique, name, email, the password as a hash, the SHA-256 of the token, the
  terms accepted as JSON, created, expires), `email_tokens` (the SHA-256 of the
  token, kind `verify` or `reset`, user, address, created, expires, used) and the
  column `users.terms_accepted` (JSON: `at`, `termsUrl`, `privacyUrl`). The mail
  settings are a record of `site_records` (key `mail`, the password sealed).
- Added by the third part: `usage` (kind `user` or `tenant`, id, tenant_id,
  spaces, bytes, pages, people, over, measured; primary key `(kind, id)`; indexes
  `(kind, tenant_id, bytes DESC, id)` and `(kind, bytes DESC, id)` for the list of
  people), and the columns `users.quota` (JSON, `''` for none), `spaces.page_count`,
  `spaces.usage_bytes` (what the last measurement found; the 1.7 `size_bytes` is
  its first value), `spaces.usage_user` (whom the space is counted for now, so a
  hand-over moves exactly what was added; indexed), and `invites.username`,
  `invites.name`, `invites.email` (what the invite fills in).

### What the first part of 3.3 does and does not do

Done: the tenant model, `siteAdmin` and `tenant` on the User, data isolation, the
administrator split, the tenancy switch (`features.tenancy`), the tenants, layered settings with
locks (`/api/admin/settings`, the tenants' settings, `GET /api/settings`), the
daily text's switches and default on that mechanism, and a provider's `tenant`.

The quotas and usage (the `quota.*` settings, the tenant's `usage`,
`/api/account/usage`, `/api/admin/usage`, `quota_exceeded`) and the prefilled
invite of `POST /api/admin/tenants` came with the third part, below.

### What the second part of 3.3 does

Done: mail (`/api/admin/mail`, the test message, the queue), registration,
email verification, password reset, the `register` and `forgot` flags of
`GET /api/auth/providers`, the captcha contexts `register` and `forgot` (in
`captcha.contexts`, off by default, like the others), and `sso_only_with_registration` (since reduced to
organisation creation: see "Registration"). Details the implementation settled, which refine
the text above:

- **Nothing is sent by default.** Mail is off until `PUT /api/admin/mail` has
  `enabled: true`; the tests of this repository talk to an SMTP server in the
  test's own process and to no other.
- `GET /api/admin/mail`'s `status.checked` is `null` until a delivery or a test
  has happened (the status is kept in memory). `POST /api/admin/mail/test`
  takes an optional `lang`, answers `409 mail_not_configured` while no `host`
  is set and `400 mail_from_required` while no `from` is, and the 10 an hour are
  counted by the server (all administrators together).
- `PUT /api/admin/mail` checks in this order: field forms and ranges
  (`invalid`), `mail_insecure`, then, when enabling, a missing host (`invalid`),
  `mail_from_required`, `mail_needs_public_url`. Switching mail off drops what is
  waiting in the queue and cancels the pending registrations.
- The queue tries a message at once, again after 1 minute and again after 5,
  and drops it after the third failure or 10 minutes, whichever comes first; the
  log says why (never the address or the text, which hold a link).
- **Tightening the registration mode deletes the pending registrations** (a
  change of `registration.mode` or of its `lower` at the site or the tenant,
  switching mail off, deleting the tenant, and an hourly check), so their links
  answer `404 not_found`; `403 registration_closed` or `403 invite_required` at
  `POST /api/register/confirm` is what a link of a registration that is still
  stored gets when its tenant's mode does not allow it (for instance the instant
  between the change and the deletion).
- **The tenant's hour** of `registration.perHour` counts every `POST
  /api/register` that passes the field checks, whether or not the address has an
  account, and the pending cap of 1,000 is checked the same way, so those answers
  do not tell either. The password is hashed in every case.
- `GET /api/register` counts against the address's requests this minute.
- A tenant administrator's changes of an address: `emailVerified: true` sent by a
  site administrator always clears the `email_verified_by` mark of a tenant
  administrator, also when the address was already shown as verified; so does the
  person's own `POST /api/email/confirm`.
- `POST /api/users/{id}/password-reset` answers `404 not_found` for a person
  outside the caller's scope, `403 forbidden` for a tenant administrator and a
  site administrator's account, and `409 conflict` also for a disabled account or
  one whose password `ssoOnly` refuses. It counts against the address's 3 an hour
  (`429 rate_limited`) but not against any caller's 5.
- A reset link of an account that can no longer use a password (it is disabled,
  or `ssoOnly` came on since) answers `404 not_found` and changes nothing.
- A message that cannot be queued because 1,000 wait answers `503 unavailable`
  to a signed-in person (verification) and is not said to a public caller.

Details the first part settled:

- An administrator invited by a site administrator into the default tenant is a
  site administrator (what every administrator was before tenants); in any other
  tenant, or invited by a tenant administrator, they are not. `POST
  /api/invites` takes `tenant` and `siteAdmin` (only a site administrator may
  send either to anything but their own tenant, or `true`), and answers both.
  `POST /api/admin/tenants`'s invite is not a site administrator's.
- `PATCH /api/users/{id}` with `role: "admin"` in the default tenant by a site
  administrator also makes a site administrator unless `siteAdmin: false` is
  sent. A demotion ends the flag. A tenant administrator who targets a site
  administrator, or sends `siteAdmin`, is `403 forbidden`; `siteAdmin: true` for
  a member is `400 invalid`.
- `GET /api/admin/spaces` items have `tenant` (an id), as invites have.
- The server's own audit kinds (`security_changed`, `provider_*`, `mail_*`,
  `tenant_*`, `dropped`, `rate_limited`, `lockout`) are hidden from tenant
  administrators; rows of the site layer's settings have no tenant.
- Deleting a tenant puts the providers that named it back on the default tenant.

### What the third part of 3.3 does

Done: the `usage` table and its upkeep, the `quota.*` settings with their person
layer, `GET /api/account/usage`, `GET /api/admin/usage`, `quota` on
`PATCH /api/users/{id}`, the tenant's `usage`, `quota_exceeded` for the number of
spaces, notices and the once-per-crossing audit row, and the prefilled invite of
`POST /api/admin/tenants`. Details the implementation settled, which refine the
text above:

- `serve --profile NAME` (`default` or `cn`) seeds site values on the first
  start only. When the state database is empty (no account, no site value,
  no earlier profile) the profile's values are written as ordinary site rows
  with `set_by` `profile:NAME` and the name is kept in the site record
  `profile`; they are defaults, not locks, so an administrator changes them as
  usual. On any other database the same profile (or none) is ignored with a log
  line, and a different one stops the start with an error (a database from
  before profiles counts as `default`). `cn` is stricter where content leaves
  the organisation: every setting the registry marks outward is off
  (`features.tokens`, `features.mcp`; later public links, publishing and
  external sharing join them) and GitHub stays on, as it is reachable from
  China. The table is `profiles` in `internal/settings/profile.go`, and its test
  fails when a key it names is renamed or an outward setting is missing from `cn`.
- `--max-spaces`, when it is given, is the site's value of
  `quota.spacesPerPerson` (`fromFlag: true`, read only: a `PUT` of the value is
  `409 conflict`); without it the stored setting counts, 20 to begin with. The
  setting's lower rule can be set either way. `0` is no limit.
- The quota settings are `limit` settings with layers site, tenant and person:
  `quota.spacesPerPerson` is 0 to 10,000 (a `null` value sets it back to 3),
  `quota.bytesPerPerson` 1 to 2^50 and `quota.pagesPerPerson` 1 to 2^40, both
  may be `null` (no limit). The person layer is the user's own `quota`: it is
  not among what `GET /api/settings` offers a person, who cannot set it.
- Changing one of these settings, a person's `quota` or a tenant's `quota` looks
  at the numbers of everyone it concerns at once, so `over`, the notices and the
  audit follow the new limit without waiting for a measurement; the answer comes
  after (a site-wide change reads the usage rows once: about 20 ms for 20,000
  people when nothing changes, 0.3 s when half of them cross).
- A tenant's `quota.people` counts people, not accounts that may sign in: a
  disabled person counts. Lowering it under the people a tenant has refuses
  nobody who is there: the tenant, its administrators and its people see it
  (`over`, notices), and the audit has one row. A new account over it is
  refused (`409 people_limit`, see "Registration").
- A space is counted for a tenant from the moment it is made, and for its first
  owner from the moment it has one; a space nobody owns counts for its tenant
  only. A tenant that is deleted takes its usage row with it.
- The audit's `quota_exceeded` for a person has the person as `user` and their
  tenant as `tenant`; for a tenant's total it has the tenant only.
- `POST /api/admin/spaces/{space}/size` measures with the same turn as the
  background one (bytes, pages, usage, a look at the person), so an
  administrator asking for a size also brings the quotas up to date.
- Costs, measured on 20,000 people and 40,000 spaces (SQLite / PostgreSQL): the
  daily recount with nothing to correct
  0.17 s / 0.19 s, correcting a thousand rows 0.20 s / 0.26 s; a person's usage
  row 21 µs / 59 µs; a page of 50 people 0.2 ms / 0.5 ms, 1.3 ms / 2.0 ms ten
  thousand people deep; recording a measurement 0.25 ms / 0.44 ms; making a space
  (with its owner) 0.33 ms / 0.64 ms; handing one over 0.38 ms / 0.83 ms. For a
  space of 20,000 pages the page count is 0.15 ms from the open space and 0.5 ms
  from the index file of a closed one; the walk of its files (1.7's) is the turn's
  cost, about 0.2 to 0.3 s.

## Storage management

Milestone 3.5 (contract, 2026-11-23; written for after 2.7 and 3.1 are merged).
Accounts mode only: single-user mode answers `404` for every endpoint here.

What it adds: a live view of what people and spaces hold on the server, a
recycle bin that every deletion goes through, deleting a user, bulk actions
with a second confirmation, an export before a deletion, and a per-tenant limit
on the size of one file. Site administrators see every tenant (`?tenant=` to
narrow); a tenant administrator sees and acts on their own tenant only, with
the same endpoints, and another tenant's ids answer as ids that do not exist
(the isolation table of 3.3 gains every route of this section).

**Nothing here walks a disk or a repository on a request.** Every number is a
stored measurement (1.7's background measurement, 3.1's `cloneBytes`,
`indexBytes`, `lastUsed`, 3.3's `usage` rows) or an in-memory total (3.1's
attachment cache). Every list is one query a page over an index; the cursor is
keyset (`<sort value>:<id>`), never an offset, so the ten-thousandth page costs
what the first does.

### Measurements this adds

- `users.last_active`: written when a session or token of the person is used,
  at most once an hour per person (the time is held in memory and written only
  when the stored one is more than an hour old, as 3.1's `lastUsed` is).
  `null` until the first use.
- `users.first_write`: when the person first changed anything in a space (a
  page saved or committed, an upload, an import, a page made through MCP).
  Written once (`UPDATE … WHERE first_write IS NULL`), skipped from memory
  afterwards.
- On each person's `usage` row (3.3), kept by difference in the same
  transactions that change what they count, and corrected by the daily recount:
  `held_bytes` (what the server holds for the spaces counted for the person:
  `cloneBytes + indexBytes` of 3.1, summed, wherever the space lives),
  `remote_spaces` (those that follow a remote), `partial_spaces`, `grace_spaces`,
  `overdue_spaces` (3.1's deadline states), `evicted_spaces`, `archived_spaces`,
  `pinned_spaces`, and `connected` (the person has a connection of any provider,
  2.7b). The tenant's row gets the same sums. (`usage.bytes` is, from now on,
  what is counted: see above.)
- **Usage counts where the bytes live** (owner, 2026-11-23; this closes the
  question 3.1 left open about attachment repositories). Every space has a
  `location`, stored with it:
  - `server`: only on this server (a space without a remote). Counted in full.
  - `own`: the remote belongs to the person (their repository, their token, key
    or connection). Measured and shown, **never counted**: a quota limits what
    costs us, and this costs the person. What the server keeps for it (the
    partial clone, the index, the attachment cache) is the pressure controller's
    business, not quota (it is `held_bytes`, below).
  - `tenant` and `site` (3.6): a tenant's own storage target (shown, not
    counted) and the site's storage organisation (counted, with the size the
    provider's API reports for the repository, text and attachments both).
  The size of a remote repository is never found by downloading it. Counted
  means `LocationCounts(location)`; 3.6 changes where a space's location is
  decided (the one function `locationOf`) and where a `site` space's size comes
  from (the provider, a repository at a time, at most every few hours, in the
  background); nothing else here needs to change. A space that has an
  attachment repository counts where that repository lives: with the space on
  the server, or not at all in the person's own.
- A person is **dormant** when `first_write IS NULL`, `remote_spaces = 0` and
  `connected = false`: registered, never connected anything, never used. It is
  a stored flag (`usage.dormant`), so the filter is an index range.

Indexes: `(kind, tenant_id, held_bytes DESC, id)`, `(kind, tenant_id, last_active, id)`,
`(kind, tenant_id, dormant, created, id)` on `usage`; `(tenant_id, size_bytes DESC, id)`,
`(tenant_id, last_used, id)` and `(tenant_id, created, id)` on `spaces`, each also
without the tenant for a site administrator's list across tenants; and the ones the
filters need (see "As built").

### `GET /api/admin/storage`

The 3.1 endpoint keeps its settings (site administrators only for `PUT`, see
"Tenant autonomy" for which of them tenants may tighten from 3.6). `GET` gains a
summary, and a tenant administrator may now `GET` it: they receive the summary
of their tenant and the effective values of the settings, read only.

```json
{"summary": {"tenant": null, "spaces": {"local": 12, "full": 3, "partial": 41, "evicted": 156, "archived": 40, "restoring": 0},
             "people": 340, "dormant": 51, "heldBytes": 3436973876, "usageBytes": 912680550400,
             "assetCacheBytes": 4831838208, "recycle": {"items": 4, "bytes": 52428800},
             "exports": {"items": 1, "bytes": 10485760}, "remotes": {"inGrace": 3, "overdue": 0},
             "recycleDays": 30, "diskFreeBytes": 52428800000, "measuredAt": "2026-11-23T02:10:00Z"}}
```

The sums are the tenant's `usage` row (one primary-key read) or, for the whole
server, one aggregate over the tenants' rows; `assetCacheBytes` comes from
memory (per space, summed for the tenant's spaces from the in-memory map);
`diskFreeBytes` is for site administrators only. `recycleDays` is
`recycle.days` as it is now (how long the recycle bin keeps an item).

What is the server's own is the site's alone: a tenant administrator's answer
has no `hosts.fromFlag`, `cache.minFreeBytes`, `git.version` (`git.partialClone`
stays, it says what the settings can do) and no `summary.spaces.restoring` (the
restores that run on the server, of every tenant); a site administrator's has
them. In the summary of a tenant `assetCacheBytes` is summed over the tenant's
spaces, the tenant of the cached spaces looked up in one query for the ones not
seen before and remembered.

### `GET /api/admin/spaces` (filters and sorting added)

The 1.7 list, with 3.1's `storage` block, gains:

- `sort`: `created` (the default, as before), `name`, `bytes` (`sizeBytes`),
  `lastUsed`; `order`: `asc` or `desc` (default `asc` for `created` and `name`,
  `desc` for the others). Spaces never measured, or never used, sort last.
- Each item gains `location` (`server`, `own`, `tenant` or `site`) and
  `counted` (whether its bytes count toward quotas: see "Measurements this
  adds").
- Filters, combinable: `tenant`, `q` (as before), `owner={user}`, `cache`
  (3.1), `remote=none|ok|offline|error`, `deadline=grace|overdue`, `pinned=1`,
  `exempt=1`, `idleDays={n}` (not used for at least `n` days, or never).
- `400 invalid` for an unknown `sort`, `order` or filter value, and for a
  cursor that was made for another `sort`.

Each filter is a column of `spaces` (the cache state, the remote's state, how
it is followed, the pin and the exemption are stored with the space), so a page
is one query; the owners and member counts are looked up for the page only. The
cursor is opaque (`next` of the page before): a cursor of another sort or order
is `400 invalid`.

### `GET /api/admin/storage/people?tenant=&q=&sort=&order=&cursor=&limit=`

People and what they hold, one page at a time:

```json
{"items": [{"user": {"id": "01JD3K7A2M", "username": "tang", "name": "Tang", "disabled": false},
  "tenant": "0000000000", "created": "2026-01-02T03:04:05Z", "lastActive": "2026-11-22T09:00:00Z",
  "firstWrite": "2026-01-02T03:10:00Z", "dormant": false, "connected": true,
  "spaces": 3, "remoteSpaces": 2, "evictedSpaces": 1, "pinnedSpaces": 0,
  "graceSpaces": 1, "overdueSpaces": 0, "usageBytes": 52428800, "heldBytes": 9437184,
  "measuredAt": "2026-11-23T02:10:00Z"}],
 "next": "held.desc:9437184:01JD3K7A2M"}
```

- `sort`: `held` (the default: what the server holds, largest first), `bytes`
  (the quota's measure), `lastActive`, `created`, `spaces`.
- Filters: `dormant=1`, `remote=none|some`, `deadline=grace|overdue`,
  `inactiveDays={n}` (`lastActive` at least `n` days ago, or never),
  `disabled=0|1`. `q` matches username or name as `GET /api/users` does.
- `limit` 1 to 200 (default 50). One query over the `usage` index; names for
  the page only.

`GET /api/admin/storage/people/{user}` is one item, plus `items` of the person's
spaces in the shape of `GET /api/admin/spaces` (spaces counted for them, at
most 200, by `lastUsed`).

**As built (part 4: the measurements, the lists and the summary).**

- **Where each number is kept.** `spaces` gains `location`, `remote_via` (`''`
  without a remote, the provider of a connection, or `git`) and `remote_state`;
  `users` gains `last_active` and `first_write`; `usage` gains `held_bytes`,
  `remote_spaces`, `partial_spaces`, `grace_spaces`, `overdue_spaces`,
  `evicted_spaces`, `archived_spaces`, `pinned_spaces`, `connected`, `dormant`,
  and a copy of the person's `created` and `last_active` (so a list of people is
  one index).
- **Deltas.** Every change of what a space contributes (a measurement, the
  index size, the cache state, a pin, the remote coming, going or changing
  state, a change of first owner, a deletion, a restore) is made in one
  transaction with the space's lock taken first, and adds the difference to the
  rows of the tenant and of the person the space is counted for. A person's
  `connected` follows their connection rows (a GitHub authorization or one of
  any provider of 3.8: connecting, disconnecting, deleting the provider and
  deleting the person all settle it, and the recount reads both tables), and `dormant` is worked out again
  whenever `remote_spaces`, `connected` or `first_write` changes (the tenant's
  count with it). The daily recount puts every one of these right from the rows,
  and also the `location`, `remote_via` and `remote_state` of spaces, and the
  counted bytes of spaces that do not count; a space that the recount finds back
  on the server (`own` to `server`) counts its last measurement again, as the
  change of its remote does.
- **`grace_spaces` and `overdue_spaces` are not kept by difference.** They change
  with the clock and with the policy, not only with a write, so one grouped
  query over the spaces that lack what is required sets them: every hour, when
  a write of `storage.remotes.*` changes the policy, and with the daily recount. They
  can be an hour behind the clock; the filters `deadline=grace|overdue` of the
  list of spaces compute from the policy and `created` at the time of the
  request and are not behind.
- **Held bytes** are `size_bytes + index_bytes` of 3.1, nothing for the clone of
  an evicted or archived space and nothing at all for an archived one.
- **`last_active`** is written by a session, an API token or an OAuth grant,
  once an hour for a person, from memory. **`first_write`** is written once, the
  first time a change is credited to the person (every write goes through the
  author of the change, which is where it is noted).
- **Spaces** are listed by one query over an index for each sort, with and
  without the tenant in front (site administrators see every tenant). A cursor
  is `<sort>.<order>:<value>:<id>` (for the name, the stored `name_key` of the
  last row, not the name folded again); spaces with no value of the sort (never
  measured, never used) come after the others in id order, as a second range
  (`<sort>.<order>.none::<id>`). Two spaces with the same value are ordered by
  id, ascending in the index's own direction and descending in the other. The
  filters: `cache=local|full|partial|evicted|archived|restoring` (`restoring` is
  in memory only and keeps nothing), `remote=none|ok|offline|error` (`error`
  keeps a rewritten history too), `deadline` and the older `overdue=1` (the
  policy's ranges of `created`, then the lack of a remote, the exemption of the
  space and of its owners), `pinned=1`, `exempt=1` (the space's own exemption,
  not an owner's), `owner` (the first owner), `idleDays`, `q` (a name contains;
  the one filter no index serves: it reads names in the order of the sort until
  the page is full). `0` for `pinned`, `exempt`, `overdue` means not asked.
- **People** are listed from the usage rows joined to their person by key, with
  no one deleted. `sort=lastActive` puts people who were never active last in
  both orders, `q` is a contains match as in `GET /api/users`. The person's
  item counts the spaces as the list does: a tenant administrator is never shown
  a personal space, in a list or in the person's `items`. The person's own row
  (spaces, bytes, held bytes) does count their personal space, as the quota
  does, so for a tenant administrator the total can be more than the spaces they
  are shown add up to. (Aggregates are not split into personal and shared: 3.7
  does that when the organisation's share and the person's home are separate.)
- **Summary.** `GET /api/admin/storage` is open to tenant administrators (the
  route is admin; `PUT` stays the site's). The body is 3.1's settings and
  `summary`; `affected` is 3.1's aggregate for a site administrator and the
  stored counts of grace and overdue spaces for a tenant administrator (who is
  not told about exemptions). The summary is the tenant's usage row or one
  aggregate over the tenants' rows, one count over the recycle bin and one over
  the exports (each an index range of the tenant), the attachment cache from
  memory and, for site administrators, the free room of the disk. Nothing walks
  a disk or a repository. `GET ?tenant=` narrows it for a site administrator and
  is a tenant that does not exist for anyone else. `PUT` answers the settings
  only, as before.
- **Indexes.** On `spaces`: `(tenant_id, created, id)` (replacing 0018's without
  the id), `(created, id)`, `(tenant_id, name_key, id)`, `(name_key, id)`,
  `(tenant_id, size_bytes DESC, id)`, `(size_bytes DESC, id)`,
  `(tenant_id, last_used DESC, id)`, `(last_used DESC, id)`,
  `(tenant_id, remote_via, created, id)` and `(remote_via, created, id)` (the
  deadline filter and `cache=local`), `(tenant_id, remote_state, created, id)`
  and `(remote_state, created, id)`, `(usage_user, created, id)`, and partial
  ones for `pinned` and for `exempt`. On `usage`: for each sort of the people's
  list one index with the tenant and one without, `(kind, tenant_id, dormant,
  created, id)`, and partial ones for people with a grace period or an overdue
  space. `internal/store/storage_explain_test.go` asks the database for the plan
  of every list, sort and main filter, on a server of 16,000 rows, on SQLite and
  on PostgreSQL, and fails on a scan of a whole table or a sort of the rows found.
- Single-user mode answers `404` for all of it, as before.

### The recycle bin

Every deletion of a space or a user goes into the recycle bin; nothing an
administrator deletes is gone at once. An item is kept for `recycle.days`
(a site setting, 30 by default, 1 to 365) and then purged.

- **What a recycled space is.** Its records (the space row, members, space
  groups, remote and attachment repository records with their sealed
  credentials, favourites, drafts, settings records, 3.1's storage records) are
  copied as one JSON document into `recycle_items` and removed from the live
  tables in the same transaction; its directories (`DATA/spaces/<id>`, the
  index cache and journal) are moved to `DATA/recycle/<item>/`; its files in
  3.1's attachment cache are dropped (they are on the attachment repository).
  So no query of the live tables can see a recycled space, and the permission
  caches of its members are invalidated as for a disabled space.
- **The remote repositories are never touched**: not the space's, not its
  attachment repository, on GitHub or anywhere. The preview says so.
- **An evicted space** has only its index on the server; that is what moves.
- **A recycled space does not count** toward anyone's quota (its usage is
  subtracted, as for a deletion in 3.3) and its bytes show as `recycle.bytes`.
- **Restoring** moves the directories back and inserts the records again under
  the same ids, in the original tenant, by `INSERT … SELECT` joined on that
  tenant (3.3's rule, so a restore cannot make a cross-tenant link). A member
  who no longer exists, is deleted or is in the recycle bin is left out (listed
  in the answer); when that leaves no owner the restore needs an `owner`
  (`409 owner_required`). 3.1's deadline of a restored space is computed again
  from the policy in force. Quotas count it again; a restore is never refused
  for a quota (it is an administrator's act), it can only make a notice.
- **Purging** deletes the directory and the item. The purge job runs every
  hour, purges one item at a time and pauses between directories as the 1.7
  measurement does, so its disk load is that of one walk.
- From 3.5 every deletion goes through the recycle bin; 1.7's `DATA/trash/` is
  no longer written or read.

**As built.**

- The states of an item are `moving` (its rows are already out of the live
  tables and its directories are being moved to `DATA/recycle/<item>/space` and
  `/cache`), `ready`, `restoring` and `purging`. Only an item that is `ready` is
  restorable or purged; a crash leaves the others and the next start finishes
  them (a move is completed, a restore is undone, a purge is carried on). A move
  that cannot be made (the disk, a file in the way) leaves the item `moving`,
  with `restorable: false` and a problem, and is tried again by the hourly job.
- `DELETE /api/admin/spaces/{space}` takes an optional `{"reason": "…"}` (at most
  500 characters); the drafts of its people are in the item, not in a file.
- A tenant cannot be deleted while any item of the recycle bin is held for it
  (`409 tenant_not_empty`, with `recycle`), so a restore always finds its tenant.
- A personal space cannot be restored on its own (`409 conflict`): it goes back
  with its person. The restore of a space whose people are gone, and any other,
  puts back what it can and lists `skipped` (`{"user": …}` or `{"group": …}`);
  `owner` must be someone of the tenant who exists, is not deleted and is not
  disabled (`404 user_not_found` otherwise, the same answer as for an id that
  does not exist).
- The deadline of 3.1 is not stored: the policy computes it from the space's
  creation date when it is read, so a restored space has the deadline the policy
  in force gives it.
- The purge job is the manager's (`RunRecycle`); it also deletes exports whose
  days are over. When `recycle.days` cannot be read, the round purges nothing
  (it is never run on the default).
- An action on an item (moving, restoring, purging) holds it while it runs; the
  hourly look at items that are not at rest only takes over what a crash or a
  failure left, never an action that is still running, however long it takes. A
  restore whose item is not `restoring` any more when it ends puts nothing back.
  The hourly look reads each item again once it holds it and leaves it alone
  unless it is still in the state it was listed in (a restore that finished
  meanwhile is not moved back).
- A move between `DATA/spaces` (or `DATA/cache`) and `DATA/recycle` on different
  filesystems is a copy (synced, with the files' modes, written beside its
  destination and renamed into place whole) and then a delete of the source,
  which is first renamed aside (`.moved`, within its own filesystem) so that a
  delete cut short never leaves a partial tree in its place; a copy cut short is
  started again and what is set aside is deleted, so an item is never stuck
  `moving` and a space is never left partial.
- Deleting a space whose clone is being restored from its remote (3.1) stops
  that restore and waits for it, and closes the space's index-only handle (4.1):
  nothing is written at its live place after the move.

#### Recycle item

```json
{"id": "01JDRC1X9A", "kind": "space", "tenant": "0000000000",
 "space": {"id": "01JD3K8Q7Z", "name": "Team", "personal": false},
 "user": null, "with": [],
 "bytes": 18304211, "deletedAt": "2026-11-23T08:30:00Z", "deletedBy": {"id": "…", "username": "wu"},
 "purgeAt": "2026-12-23T08:30:00Z", "reason": "", "state": "ready", "job": null, "restorable": true, "problems": []}
```

`state` is `ready`, `moving`, `restoring` or `purging` (see "As built"), in the
list, in the answers that return an item and in the purge's `202`: a page that
shows an item being purged reads it here and needs no memory of its own. It is
derived from the job that holds the item (`recycle.move`, `recycle.restore` or
`recycle.purge`), whose id is `job` (`null` for an item at rest): follow it with
`GET /api/admin/jobs/{id}`.
`kind` is `space` or `user`. A `user` item has `user` and lists in `with` the
space items recycled with that person (their personal space and the spaces they
owned alone); those spaces are restored with the person, and on their own only
with an `owner`. `restorable` is `false` (with `problems`) when restoring would
fail now (the original tenant was deleted: never, see below; a directory
missing from `DATA/recycle`). `purgeAt` follows `recycle.days` as it is now:
shortening it moves every item's date.

#### `GET /api/admin/recycle?tenant=&kind=&q=&cursor=&limit=`

Items, newest first; one query a page.

#### `POST /api/admin/recycle/{item}/restore`

`{"owner": "01JD3K7A2M"}` (optional). `200` with what was restored and what
was left out (`{"restored": {...}, "skipped": [{"user": "…", "name": "tang", "reason": "deleted"}]}`;
see "What a restore leaves out" for `skipped`).
`409 owner_required`, `409 busy` (a purge of it is running), `409 conflict`
(a space with that id exists again: never in practice, ids are random).

#### `DELETE /api/admin/recycle/{item}?confirm={token}`

Purges now. Site administrators only (it frees the server's disk early; a
tenant administrator waits for the date): `403 forbidden` otherwise. Needs a
confirmation token from `POST /api/admin/confirm` (below). `204`.

**As built:** `202` with the item: the purge is paced and may take long, so it
starts in the background. The item is `purging` until it is gone (a restore of
it answers `409 busy`); `space_purged` or `user_purged` is written when it is
over; one that fails stays `purging` and the hourly job finishes it. The
server's own purge of an item past its days is audited too, with the actor
`system`.

**What a restore leaves out.** `skipped` (in the answer of the restore of a space
and of a person) lists what was left out, each with its `name` as it was when the
thing was deleted (the display name of a person, else their username; the name
of a group or a space) and a stable `reason`:

| Entry | `reason` | Meaning |
|---|---|---|
| `{"user": id, "name"}` | `deleted` | the person of a membership no longer exists, or is deleted |
| `{"group": id, "name"}` | `deleted` | the group no longer exists |
| `{"space": id, "name"}` | `busy` | a space that went with a person is not at rest (a purge or another action holds it); it stays in the bin |
| `{"space": id, "name"}` | `owner_required` | it has no owner left: restore it on its own with an `owner` |
| `{"space": id, "name"}` | `conflict` | a space with that id exists again |
| `{"space": id, "name"}` | `failed` | anything else; it stays in the bin as an item of its own |

`name` is `""` for a thing whose name was not kept. The codes are fixed; the page
words them.

### Deleting a space

`DELETE /api/admin/spaces/{space}` keeps its 1.7 checks (commits what the space
holds first; `409 busy`; `409 personal_space`) and now **recycles** the space.
`200` with the recycle item (1.7's `trashed`, `cacheTrashed` and `warnings`
are gone). A member's next request answers `404 not_found`.

#### `GET /api/admin/spaces/{space}/deletion`

What deleting the space would do; reads stored values only:

```json
{"space": {"id": "01JD3K8Q7Z", "name": "Team"}, "pages": 2100, "bytes": 18304211, "heldBytes": 9437184,
 "attachments": {"inSpace": 312, "repository": true}, "members": 5, "groups": 1,
 "drafts": 2, "remote": {"url": "https://github.com/kazuhahub/tang-notes", "kept": true},
 "assetsRemote": {"url": "https://github.com/kazuhahub/tang-notes-assets", "kept": true},
 "cache": "evicted", "exportable": true, "recycleDays": 30}
```

`exportable` is `false` for an evicted space whose remote cannot be reached now
(an export needs its files; 3.1's restore is what fetches them).
`attachments.inSpace` is `null`: no stored value counts a space's attachments, and
the preview reads stored values only.

### Deleting a user

Users were never deleted before 3.5 (only disabled). From 3.5:

#### `GET /api/admin/users/{user}/deletion?heir={user}`

The plan, computed from the records (memberships, spaces' first owners, tokens,
grants, groups, GitHub connection), never from files:

```json
{"user": {"id": "…", "username": "tang"},
 "recycle": [{"space": {"id": "…", "name": "Tang", "personal": true}, "bytes": 4194304}],
 "transfer": [{"space": {"id": "…", "name": "Team"}, "to": {"id": "…", "username": "li"}, "why": "editor",
               "credential": "user"}],
 "leave": [{"space": {"id": "…", "name": "Shared"}, "role": "owner"}],
 "revoke": {"tokens": 2, "grants": 1, "passkeys": 1, "identities": 1, "github": true, "sessions": 3,
            "connections": [{"provider": "github", "name": "GitHub"}, {"provider": "gitlab-1", "name": "GitLab"}]},
 "groups": 4, "needsHeir": [], "blockers": [], "bytes": 4194304, "recycleDays": 30}
```

`revoke.connections` has one line for each provider the person is connected to
(GitHub and the providers of 3.8); `github` stays as it was. Deleting the person
ends each of them as the person's own disconnect does (the host is asked to
revoke the token where it can be, best effort), and removes any row left of a
provider that is not up; purging removes the rest with the person's data.

The rules:

1. **Personal space**: recycled with the person.
2. **Spaces where the person is the only owner and nobody else is a member**
   (no other direct member, no group): recycled with the person.
3. **Spaces where the person is the only owner and others are members**:
   transferred (3.3's `POST …/transfer`). The heir is, in this order: the one
   named in `transfers` for that space; the earliest direct member with the
   `editor` role; `heir` from the request. Viewers and group members are never
   made owner on their own. With none of these the space is in `needsHeir` and
   the deletion is refused (`409 heir_required` with the spaces).
4. **Spaces with another owner**: the person's membership is removed.
5. `credential: "user"` marks a space that syncs with the person's own token
   (2.7's `via: "user"`, or a remote token they entered): after the transfer it
   stops syncing until the new owner reconnects it; the space's remote state
   says so (the 2.7 "credential stops working" path), nothing is lost.
6. `blockers`: the caller themself; the last enabled site administrator (as
   for disabling); a site administrator, for a tenant administrator (`403`).

#### `DELETE /api/users/{id}`

```json
{"heir": "01JD3K7A2M", "transfers": {"01JD3K8Q7Z": "01JD3K9B4N"}, "reason": "left the company"}
```

All optional. The order, which makes a half-done deletion harmless:

1. The person is **disabled** at once (sessions end, tokens and grants answer
   `401 token_owner_disabled`), in its own transaction.
2. Transfers, then removed memberships, each a transaction of its own (a
   failure leaves a disabled person with fewer spaces, never a space without an
   owner).
3. The spaces of rules 1 and 2 are recycled into items listed under the
   person's item.
4. The person's row is marked deleted (`users.deleted_at`, `users.recycle_item`);
   their API tokens and OAuth grants are revoked (not restored later), their
   GitHub authorisation is revoked at GitHub (best effort, as 2.7 does), their
   group memberships are removed (recorded in the item, restored with them),
   their pending invites are kept (they still let invited people in). Their
   passkeys and identities stay attached, unusable while deleted, so a
   deleted person signing in by SSO is refused instead of getting a new
   account by the registration mode.

`200` with the person's recycle item. The usage of the person moves with the
spaces (3.3's accounting); the tenant's people count goes down by one.

**A deleted person is invisible**: not in user lists, pickers, member lists or
`GET /api/groups`; their id answers `404 user_not_found` to everything but the
recycle bin and the audit log, which show "deleted user" with the username.
Their username stays taken while they are in the recycle bin.

**As built.**

- **The order and its recovery.** Step 1 disables the person (through the same
  call as `PATCH /api/users/{id}`, so the last enabled administrator and
  yourself are refused: `409 conflict`). A person who owns a space that was handed
  on is an editor of it after `POST …/transfer`, so the deletion removes that
  membership too (it is listed in `left`). The spaces of rules 1 and 2 are recycled
  one by one with the marker `u:<user id>` as their parent; step 4, one
  transaction, makes the person's item, marks the row, takes the group memberships
  out and puts the spaces under the item. A failure anywhere leaves a disabled
  person with fewer spaces and some items waiting under the marker, never a space
  without an owner; asking again goes on from there and adopts them. Before step 1
  the deletion records on the person's row whether they were disabled, and then
  each transfer and each membership left, so a deletion asked again after a
  failure records what the first attempt knew: the item says the person was
  enabled if they were before the first attempt, and lists every transfer.
  Enabling the person again ends that record, and so does a deletion that
  finds the person enabled: a later deletion starts afresh.
- A person who is disabled or deleted is never added to a space (the statement
  refuses them), and a deleted person never counts as another owner of a space.
  The last step also leaves any membership that slipped in after the plan (listed
  in `left`); one whose leaving would leave a space without an owner refuses the
  step (`409 conflict`), and asking again plans that space.
- `DELETE /api/users/{id}` answers `409 heir_required` with `spaces` (`id`,
  `name`) before anything is changed; `409 conflict` for the caller themself and
  the last enabled site administrator (the preview lists them as `blockers` with
  a `reason`: `self`, `last_admin`, from 3.6 d4 `last_local_admin`); `403 forbidden` for a site administrator when
  the caller is a tenant administrator; `404 user_not_found` for a person who does
  not exist, is in another tenant or is deleted already. The answer is the person's
  recycle item, `with` the spaces recycled with them.
- **Restoring** is `POST /api/admin/recycle/{item}/restore` on the person's item:
  the row is brought back (enabled or disabled as it was), the group memberships
  whose group is still there, then the spaces that went with them, a personal one
  too. `200` `{"restored": {"item", "user", "spaces"}, "skipped": [...],
  "transfers": [...], "left": [...]}`; a space that cannot be restored is left in
  the bin as an item of its own and listed in `skipped`. A tenant administrator's
  answer leaves personal spaces out of `spaces` and `skipped` (neither their id
  nor their name) and says how many there were in `restored.personal`; a site
  administrator's answer names them, and the audit row (`recycled`) is whole. A personal space whose
  person is there (a deletion cut short, or a restore that left it) is restored on
  its own.
- **Purging** an item of a person purges the spaces recycled with them first, then
  empties their row as described, in that order, so that a purge cut short is
  finished at the next start. A purge takes only the spaces recycled with that
  item (never those waiting under the person's marker, which belong to a
  deletion under way), and empties the person only while that item is the one
  that holds them (`users.recycle_item`): an item that went stale (a restore of
  it was cut short) never touches a person who is back, or whom a later deletion
  holds. An item whose person is not deleted (a restore cut
  short) removes only itself and the spaces recycled with it: it never touches
  the person's data.
- **Usernames.** A deleted person's username stays taken (the row keeps it, so
  the unique key refuses it, and `GET /api/…` lookups by username do not find
  them). `users.reuseUsernames` is read at invites (when one is made with a
  username), registration, accepting an invite and a sign-in that makes an account; with `never` and a
  retired name those answer `409 exists`, and a sign-in picks the next free
  form. A sign-in by an SSO identity of a deleted person is refused (`disabled`),
  never turned into a new account.
- The daily sweep deletes a tombstone when no audit row (as actor or subject), no
  invite (made or used), no recycle item, no export and no space names it.

#### Restoring and purging a user

- Restoring brings back the person as they were before step 1 (enabled or
  disabled), their group memberships, and the spaces recycled with them.
  Transfers and removed memberships are **not** undone (the heirs may have
  changed things since); the answer lists them so an administrator can redo
  them by hand. Revoked tokens and grants stay revoked.
- Purging deletes their personal data: password hash, passkeys, identities,
  email, name, preferences, favourites, drafts, sessions, held sessions,
  usage row, and their spaces' items. **The row itself stays as a tombstone**
  (`id`, `tenant_id`, `deleted_at`, username replaced by `deleted-<id>`) while
  anything refers to it: audit rows, and groups or invites they made. The daily job hard-deletes a tombstone that nothing
  refers to any more (the audit log's 90 days are the usual last reference).
  Git history is never rewritten: commits keep the author name and address
  they were made with.
- **The username after a purge.** Purging frees the email address (it is
  personal data) and, by default, the username. Whether a username may be taken
  again is the site setting `users.reuseUsernames` (site only): `after_purge`
  (default; anyone may take it once the person is purged, never while they are
  in the recycle bin) or `never`. Every purge records the username, case folded,
  in `retired_usernames` whatever the setting, so switching to `never` later
  also protects the names purged before; with `never`, a retired name answers
  `409 exists` wherever a username is chosen (invites, registration, renames,
  a sign-in, which then picks the next free form as 2.8 does). What is
  stored by id (memberships, authors in the records, audit rows) never follows
  a name; a `@mention` or link written as a username in page text points to the
  new holder.

### Confirmations and bulk actions

#### `POST /api/admin/confirm`

```json
{"action": "delete_spaces", "spaces": ["01JD3K8Q7Z", "01JD3K8Q80"]}
```

`action` is one of `delete_spaces`, `delete_users`, `purge`, `restore`,
`evict`, `pin`, `unpin`, `disable_spaces`, `enable_spaces`, `disable_users`,
`enable_users`, `export` (3.6 adds `change_capabilities`, `domain_join_off`
and `move_storage`,
3.7 `manage_accounts`); with `spaces`, `users` or `items` (recycle items), at
most 500, all in the caller's scope (`404` for any that is not: nothing is
confirmed). For `delete_users`, also `heir` and `transfers` as above.

`domain_join_off` (3.6 d5) is the preview of cancelling the requests to join
that wait on a domain: `{"action": "domain_join_off", "tenant": "01JDT3NANT",
"domain": "acme.example"}`, for whoever manages the tenant's domains (`404
not_found` otherwise). `plan` is `[{"tenant", "domain", "requests"}]`, and the
token is bound to the set of waiting requests. It goes as `confirm` with
`PATCH …/domains/{domain}` `{"join": "off"}`, or as `?confirm=` with `DELETE
…/domains/{domain}` (see "Joining an organisation").

`change_capabilities` (3.6 d2) is the preview of switching a feature off while
it is in use: `{"action": "change_capabilities", "key": "features.domains",
"tenant": "01JDT3NANT"}` (`tenant` for a site administrator's rule for one
tenant; without it, the site's value). Only who may make that change gets it
(`404 not_found` otherwise); a feature with no count of its use is `400
invalid` (today `features.domains` and, from 3.6 d3, `features.ownSso` have
one). `plan` has one line per tenant that has the feature now and uses it:
`{"tenant", "slug", "name", "feature", "domains", "verified"}` for
`features.domains` (its claims, how many are verified, and from 3.6 d5
`requests`, its requests to join that wait), `{"tenant",
"slug", "name", "feature", "providers", "enabled"}` for `features.ownSso` (its
own sign-in providers, and how many are on). The token is
not a bulk action's (`POST /api/admin/bulk` refuses it): it goes as `confirm`
with the change itself, `PUT /api/admin/settings/{key}` or
`PUT /api/admin/tenants/{tenant}/settings/{key}` (`{"value": false, "confirm":
"c_…"}`, or `{"siteRule": {"value": false}, "confirm": …}`). What decides is
the value each tenant that uses the feature gets, before and after the change,
now and at every start and end of a rule's window either way (a rule dated to
switch it off later, or a site value of `false` under a rule that holds the
tenant on until a date), whatever words the change uses (a value, a rule, a
lock, a restriction to `[false]`). A change that switches it off for one of
them, now or later, without a token answers `409 confirm_required` with `plan`
(those tenants' lines); a token whose plan changed, `409 confirm_stale`. The
same holds for a plan: `PUT /api/admin/tenants/{tenant}/plan` (`{"plan": …,
"confirm": …}`, with the tenant's token) when the new plan, or the manual rules
the change clears, leave the feature off, and `PATCH /api/admin/plans/{slug}`
(`{"rules": …, "confirm": …}`, with the site's token) for every tenant on the
plan. Plans keep carrying features. Billing (the internal writer, which has
nobody to confirm) is refused (`confirm_required`) a rule that would switch a
feature off for a tenant that uses it. Nothing stored is deleted: the domains
have no effect while the feature is off, and work again as they were when it
is on.

`200`:

```json
{"confirm": "c_6c1…", "expires": "2026-11-23T08:40:00Z", "count": 2,
 "plan": [ /* the deletion previews, or one line per item for the other actions */ ],
 "bytes": 22020096, "needsHeir": [], "blockers": []}
```

The token is bound to the caller, the action, the exact set of ids and a hash
of the plan; it lasts 10 minutes and works once. It is held in memory (at most
1,000; a restart forgets them and the client asks again). The web app shows the
plan and asks the administrator to type the number of items before it sends
the token: that is the second confirmation.

#### `POST /api/admin/bulk`

`{"confirm": "c_6c1…"}`. Needs a confirmation for every action of more than one
item and for `purge` of any. `409 confirm_stale` when what the plan was made
from changed (a member added, a space measured again is not a change: the hash
covers ids, owners, members, heirs and states); `404 not_found` for an unknown,
used or expired token. `202` with a job:

```json
{"job": {"id": "01JDJB…", "kind": "bulk", "subject": "", "action": "delete_spaces", "state": "running",
         "total": 2, "done": 0, "bytesTotal": 0, "bytesDone": 0, "failed": [], "error": "",
         "created": "2026-11-26T08:30:00Z", "updated": "2026-11-26T08:30:00Z", "expires": null,
         "madeBy": {"id": "…", "username": "wu"}}}
```

`GET /api/admin/jobs/{id}` follows it (see "Jobs" below). A job runs the items one at a time,
in the background, each with the rules of its single action (so `409 busy` on
one space fails that item, which is listed in `failed` with its error, and the
rest go on). At most one bulk job per tenant and two on the server run at once;
others wait (`state: "queued"`). A job survives nothing: after a restart, items
not done are not done, and the job reads `state: "interrupted"` with what was
done. A server that stops does not wait for its jobs to finish: each stops before
its next item (the one it was on is cut short as a request whose client left)
and is marked `interrupted` before the server exits. A job that ended is kept 30
days after its last change, then deleted (`404` from then on).

Single actions keep their own endpoints without a token, except the purge of a
recycle item.

**As built.** `POST /api/admin/confirm` takes every action of the list and answers
the plan: the preview of each deletion, or one line per item; `needsHeir` and
`blockers` name the person they are about (`{"user": …}`). The token holds the
request it was asked for. `POST /api/admin/bulk` always needs it (a single
purge's `?confirm=` is the other place it is used), is bound to the caller, spent
at once, and is `409 confirm_stale` when the plan, made again from the records,
has another hash (ids, states, owners, members, heirs, transfers). The actions
`evict`, `pin` and `unpin` are the site's (`403 forbidden` for each item for a
tenant administrator); `export` makes one export of the whole set. A job is a row
of `jobs` (kind `bulk`, run by `internal/jobs`); its `state` is `queued`, `running`, `done` (with `failed` listing
`{"id", "error", "code"}` of each item that did not work, `409 busy` among them)
or `interrupted`, and `failed` when the job broke (a panic, which the server
survives, or progress that could not be written after a few tries; `failed` names
the item it was on); `GET /api/admin/jobs/{id}` is the tenant's, and while the job runs it is answered from memory (progress is written at most once a second or every 100 items). Each item is done
with the rights the administrator who started the job has at that moment (one who
is no longer an administrator gets `403 forbidden` for the items left). A job
waits in the queue of each tenant it works on (a site administrator's job on one
tenant is that tenant's turn, not their own tenant's) and its row is that
tenant's; a job a site administrator started is the site's: a tenant
administrator does not see it. The hash of `delete_users` covers what each
preview decides: the spaces recycled, each transfer and its heir, the spaces left
and with which role, `needsHeir` and the blockers. A job runs on the
server's own time: it does not stop when the caller's request ends.

### Exports

An export is a zip of what a space holds, made before a deletion so that the
content can be kept elsewhere. It is offered by the deletion preview and in the
recycle bin.

#### `POST /api/admin/exports`

`{"spaces": ["01JD3K8Q7Z"], "items": [], "user": null, "history": false}`: spaces,
recycle items, or a user (their personal space and the spaces of rules 1 and 2).
`202` with `{"job": …}`: the export is a job of kind `export` (see "Jobs"),
`queued` at first; `GET /api/admin/exports` lists it with its `bytes` and
`expires`.

- Each space is a folder: its files at the current commit (`git archive` from
  the clone, or from the recycle directory), `drafts.json` (every member's
  unsaved drafts, as 1.7's trash file), and `REMOTE.txt` with the remote
  addresses (never credentials). An attachment repository's files are fetched
  into the zip (an evicted space is restored for it first, from the remote).
  With `history: true` (site administrators only), a `git bundle` of the
  space's repository instead of the files at the current commit.
- **Any space may be exported** by an administrator who may see it (a site
  administrator: every space; a tenant administrator: their tenant's, from 3.6
  only while the tenant has the feature `features.adminView`, see "Viewing a
  space" for personal spaces from 3.7), in the recycle bin or not.
  Every export is audited (`export_created`, `export_downloaded`), and every
  owner of the space gets a notice in their account.
- Exports are written to `DATA/exports/`, kept 7 days (`exports.days`, site
  only) and count against `exports.maxBytes` (site only, 20 GiB by default):
  `507 exports_full` beyond it. One export at a time per tenant, two on the
  server; the rest wait.

**As built.** An export's `state` is `queued` (waiting for its turn), `running`,
`ready` or `failed` (with `error`); a ready export lists in `warnings` what it
could not include (an attachment that could not be fetched), and the zip has the
same list as `NOT-INCLUDED.txt` in the space's folder. The folder of a space is
`<name>-<id>/`. A space in the recycle bin whose directory was never there (it was
evicted when it was deleted) exports its drafts and addresses but no files. The
notice to a space's owners is not sent (there is no account notice to put it in);
the audit rows carry the export and its spaces. `user` in the request names the
spaces of rules 1 and 2 of "Deleting a user" (the personal one only for a site
administrator), or, for a person in the recycle bin, the items recycled with
them. `history: true` is `403 forbidden` for a tenant administrator.

Each entry of an export's `spaces` is `{"space": id, "name", "spaceId", "open"}`
for a space and `{"item": id, "name", "spaceId", "open"}` for a space that was in
the recycle bin when it was exported. `name` is as it was; `spaceId` is the space
either way; `open` says whether that space exists now and the caller may open it
(a personal space is never open to a tenant administrator, and one that was purged
or whose item is still in the bin does not exist), so that the page links to it only
then. One query answers it for a whole page.

`GET /api/admin/exports`, `DELETE /api/admin/exports/{id}`, and
`GET /api/admin/exports/{id}/download` (`Content-Disposition: attachment`,
`Range` supported). `GET /api/admin/exports/{id}` is gone: `GET /api/admin/jobs/{id}`
answers for one export. A row of the list is the export as before (`id`, `tenant`,
`madeBy`, `spaces`, `history`, `state`, `error`, `warnings`, `bytes`, `created`,
`expires`) with its job's progress: `total` (the spaces), `done` and `bytesDone`.
An export made by a tenant administrator is visible to that tenant's
administrators and the site's.

**As built:** an export a site administrator made is the site's: only site
administrators list, read, download or delete it, whatever spaces it holds. So is
any export that holds a history bundle, a personal space or the spaces of more
than one tenant (only a site administrator can make one).

### Jobs

Every background action is a row of `jobs`, run by one runner (`internal/jobs`):
a bulk action (`bulk`), an export (`export`) and the actions of the recycle bin
(`recycle.move`, `recycle.restore`, `recycle.purge`) and a purge of the audit
log by hand (`audit.purge`, site administrators only). One shape answers for each
kind:

```json
{"id": "01JDJB…", "kind": "export", "subject": "", "state": "running",
 "total": 3, "done": 1, "bytesTotal": 0, "bytesDone": 1048576,
 "failed": [], "error": "", "created": "…", "updated": "…", "expires": "…",
 "madeBy": {"id": "…", "username": "wu"}}
```

`state` is `queued`, `running`, `done`, `failed` (it broke: `error` says how),
`interrupted` (the server stopped and the kind does not take it up again) or
`cancelled` (reserved: no endpoint cancels a job yet). `subject` is what the job
works on (a recycle item); `action` is added to a `bulk` job. `total` and `done`
count the items (the spaces of an export), `bytesDone` the bytes written;
`expires` is when an export's file goes (`null` for any other job).

- Both routes admit a caller with any one of `tenant.bulk.run`,
  `tenant.data.export` and `tenant.recycle.read` (`403 forbidden` naming
  `tenant.bulk.run` with none of them); what each answers is narrowed to the
  kinds those keys cover. The move, restore and purge of a personal space's
  item are the site's: a tenant administrator does not see them.
- `GET /api/admin/jobs/{id}`: one job, `404` outside the caller's scope or for a
  kind their keys do not cover. While the job runs on this server the answer
  comes from memory (progress is written at most once a second or every 100
  items), so polling it reads no row.
- `GET /api/admin/jobs?kind=&state=&cursor=&limit=&tenant=`: a page, newest first,
  scoped like every administration list (a site administrator sees all, narrowed
  by `tenant`; a tenant administrator their tenant's, never the site's). Which
  kinds the caller sees follows their keys: `bulk` with `tenant.bulk.run`,
  `export` with `tenant.data.export`, `recycle.*` with `tenant.recycle.read`.
  `next` is the cursor of the following page.
- Slots: a tenant runs at most `jobs.perTenant` (default 1) bulk actions and,
  apart from them, as many exports and moves of attachments; the server runs at
  most `jobs.maxRunning` (default 2) of all of them together. A recycle action
  takes none but holds its item (one job at a time per item).
  After a restart a running `export` or `bulk` job reads `failed` or
  `interrupted`; a move or a purge is taken up again, a restore is undone (the
  item is at rest again).
- A job that ended is kept 30 days after its last change; an export is kept for
  its days (`exports.days`), and its file with it.

### Viewing a space

Before 3.5 administrators could not read people's pages through the API (1.7).
From 3.5 they may read any space they may manage, read only, and **every view
is audited**. From 3.6 a tenant administrator may do so only while the tenant
has the feature `features.adminView` (`403 feature_off` otherwise; turning it
off ends the tenant's open views); site administrators always may.

#### `POST /api/admin/spaces/{space}/view`

`{"reason": "Checking a reported page"}` (1 to 500 characters, required).
`200` with `{"space": "…", "until": "2026-11-23T10:30:00Z"}`.

- For one hour the caller has `read` in the space through the ordinary read
  endpoints (pages, folders, search within the space, files, history, diffs,
  `GET /api/spaces/{space}`), exactly as a `viewer` member would, and nothing
  else: no write, no draft, no member list beyond what a viewer sees, no MCP or
  token access (a view belongs to the session that opened it). Calling it
  again extends the hour and writes a new row.
- The view is held in memory per session (a map from space to expiry, checked
  where the permission cache answers "not a member"), so it adds nothing to
  the request of a member.
- Audit: `space_viewed` when it starts (`reason`), and `page_viewed` the first
  time each page or file is opened in the view (at most one row per
  administrator and page an hour).
- A space in the recycle bin is viewed through its export, not here
  (`404 not_found`).
- `DELETE /api/admin/spaces/{space}/view` ends it early.

- `GET /api/admin/spaces` and `GET /api/admin/spaces/{space}` carry `view` on each
  space: `{"since": "2026-11-23T09:30:00Z", "until": "2026-11-23T10:30:00Z"}` while the
  caller's session has a view of it open (`since` is when it was opened or last
  extended), `null` otherwise. It is the session's alone, so another session of
  the same person, or another administrator, sees `null`. The page needs no memory
  of its own to know whether to show the banner.

The web app opens it from the space's row in the administration, asks for the
reason, and shows a banner "管理员查看，只读，已记录" while it lasts.

**As built.**

- An administrator who is not a member of a space (a site administrator for any
  space, a tenant administrator for their tenant's) has **only `members` and
  `manage`** there, without a view: the names of its people, its
  settings, its rename, its transfer. Not `read`, and never `write`. From 3.6
  not its owners nor its remote either (see "Taking over a space"). This
  replaces 1.7's "an administrator may do everything in every space", which the
  view would otherwise make pointless. A view adds `read` to those, for the
  session that opened it, and nothing else.
- A **personal space** is never reachable by a tenant administrator: not by a
  view, not by an export, not even for managing it; every endpoint answers as for
  a space that does not exist. That is every administration route that takes a
  space or a recycle item (the space, its deletion preview, size, transfer,
  disable, enable, delete, view; restore; the confirmations and exports by the
  ids of their body), and the lists: `GET /api/admin/spaces` and the recycle bin
  leave personal spaces out (the `with` of a person's item too). The person's own
  item is the tenant's: restoring it brings their personal space back with them. A site administrator reaches it as any space,
  with a view each time recorded.
- `page_viewed` is recorded for `GET …/pages/{id}`, its history and versions,
  `GET …/files/{path}` and `GET …/sync/pages` (one row an hour for each
  administrator and page or path), and, as the path `pages?body=1`, for a listing
  that asks for the pages' text.
- A view of a space of a disabled tenant is `403 space_disabled`. The capability
  `features.adminView` is 3.6's; until then every administrator who may see a space
  may view it.

Regional or compliance refinements (telling the owner first, a second
administrator's approval, a longer retention of these rows) are a later item
(docs/design/milestones/phase-5.md, 5.14).

### Viewing as a person

3.6 (docs/design/3-6-c.md section 6). An administrator sees the app as one
person sees it, **read only**, in a separate browser tab, for an hour at most,
and every view is audited. It is a lens over what the administrator could
already open with an audited view of a space, never a new reach.

Key: `tenant.users.viewAs` (tenant scope, feature `features.adminView`),
grantable to custom roles on its own. The built-in tenant and site
administrators hold it. It works in a tenant's acting scope (`PetalBook-Scope`)
too: the view then keeps that scope.

#### `POST /api/admin/users/{id}/view-as`

`{"reason": "Support ticket 12"}` (1 to 500 characters, required). `200` with
`{"id": "va_…", "until": "2026-11-26T10:30:00Z"}`.

- Who can be viewed as: a person in the caller's scope (another tenant's person
  is `404 not_found` for a tenant administrator); never oneself (`400 invalid`);
  never an account that holds site keys unless the caller holds
  `site.admins.manage` (`403 forbidden`); not a disabled account or one of a
  disabled tenant (`400 invalid`). It is opened from a browser session
  only (`403 token_scope`).
- The view is kept in memory, bound to the session that opened it. A session
  has at most one: a new one replaces it. A restart ends it.
- Audit: `view_as_started` (`data`, core), about the person, with `reason`,
  `until` and `viewAs` (the id).
- The person is **not** told. Whether they should be is an open question for
  the owner (3-6-c.md section 11, question 3); the notification kind
  `viewed_as` is registered for it and nothing makes it yet.

#### `DELETE /api/admin/users/{id}/view-as`

Ends the session's view of that person. `204`. Audit: `view_as_ended`.

#### Requests in a view

The tab sends `PetalBook-View-As: <id>` on every request. The server then:

- refuses every route that is not **viewable** with `403 view_as_read_only`,
  before anything else. The viewable routes are `GET` only: `/api/meta`,
  `/api/session`, `/api/settings`, `/api/spaces`, and in a space the pages,
  page groups, views, one page, backlinks, links, the tree, tags, types,
  appearance, files, history and versions. Never the person's account,
  sessions, tokens, grants, connections, notifications, preferences,
  favourites, drafts, sync, members, MCP or the administration;
- refuses a token or an OAuth token (`403 token_scope`);
- answers `403 view_as_ended` for an unknown or expired id, one opened by
  another session, or when the caller no longer holds `tenant.users.viewAs` in
  the person's tenant (the feature switched off, a role changed); the view is
  then gone;
- answers as the person, with no permission keys and no session: their spaces
  that are in the caller's scope, are not personal (of anyone, for anyone) and
  are not in a disabled tenant, each with `permissions: ["read"]`. Any other
  space is `404 not_found`;
- never returns a draft (`GET …/pages/{id}` has no `draft`), and writes nothing
  in the person's name: no recents, prefs, read markers or last-active time.

`GET /api/session` in a view answers the person's public profile (`email` is
`""`), `permissions: []`, and
`viewAs: {"id", "viewer": {"name"}, "until", "hidden": n}`, where `hidden` is
how many of the person's spaces the view leaves out.

Audit rows written in a view have `actor` the administrator, `user` the person,
the administrator's `capacity`, and `detail.viewAs`. Opening a page or file
writes `page_viewed` as in a space view (at most one row an hour for each
administrator and page).

Cost: a request without the header pays one header lookup; one in a view pays
one map lookup, one permission check, and the membership read that opening a
space already does.

### The size of one file

**`files.maxBytes`**: a layered setting (site, tenant; see "Settings by layer")
of kind `limit`, the largest file a write may create or replace, for every kind
of file, images included.

- The **ceiling** is the site's: `--max-upload` (10 MiB by default, at most
  256 MiB) is the site value and its `lower` is `restrict` with `max` equal to
  it, so a tenant can set any value up to it and never above. From 3.6 a
  tenant's ceiling is a rule restricting `files.maxBytes` for it (itself at most
  `--max-upload`). Lowering a ceiling below a tenant's value makes that value
  `inEffect: false` and the ceiling applies.
- **Where it is enforced**, before the file is written anywhere:
  - `POST …/assets` (the `Content-Length` first, then a reader that stops at
    the limit for a body without one);
  - `POST …/import`: each file that would be written (a page, an attachment, a
    file of a zip); the import's own size stays `maxImport` (site only);
  - pages saved by `PUT`/`POST` of pages, drafts (`PUT …/draft`), commits, MCP's
    `page_create` and `page_update`, and 4.2's offline replay: the size of the
    page file as it would be written;
  - a commit to an attachment repository (3.1), which only uploads and imports
    make.
- **Not enforced** on what already exists or arrives from a remote (a pull, a
  restore, 3.1's move of `assets/` into an attachment repository): the server
  cannot refuse what is in Git, and a lower limit never makes a file
  unreadable.
- **Refusal**: `413 file_too_large` with `{"limit": 10485760, "size": 23068672, "name": "street.jpg"}`
  (`size` is absent when the body was cut off at the limit). MCP answers a tool error with `code: "file_too_large"`.
- `GET /api/meta`'s `maxUpload` is the effective value for the caller's tenant
  (the site's for a caller not signed in); 4.1's offline uploads use it.
- The effective value is read from the settings cache (one map lookup), never
  per layer on a request.
- **As built.** The site's value is `--max-upload` and cannot be written
  (`PUT /api/admin/settings/files.maxBytes` with a `value` answers `409
  conflict`, as for any setting a flag sets); its `lower` can be changed, but the
  value in effect is never above `--max-upload` whatever the layers say. A page
  that is over the limit already (the limit was lowered) can still be saved
  again at its size or smaller: only a write that makes an over-limit file
  larger, or a new one, is refused. Templates and types are judged as pages are.
  A draft the server commits on its own (the sweeper) is judged by its person's
  limit; one over it is not written and is kept, held. A draft is judged when it is put and again
  when it is committed. An import is judged whole before anything is written:
  the first page or attachment over the limit is named and nothing is imported
  (`name` is the path in the zip). The upload endpoint's old `413 invalid` is
  now `413 file_too_large`; the `413 invalid` of a request body over `maxBody`
  or `maxImport` stays as it was. 4.2's offline replay does not exist yet; it
  goes through the same page writes, so it is judged by them.

### Audit

| `kind` | When | `detail` |
|---|---|---|
| `space_deleted`, `space_restored`, `space_purged` | a space went into, out of, or was purged from the recycle bin | `item`, `space`, `name`, `bytes`, `reason`, `bulk` (job id) |
| `user_deleted`, `user_restored`, `user_purged` | the same for a person | `item`, `user`, `username`, `transfers`, `recycled` (space ids), `reason`, `bulk` |
| `export_created`, `export_downloaded`, `export_deleted` | an export | `export`, `spaces` |
| `space_viewed` | an administrator started or extended a view | `space`, `reason`, `until` |
| `page_viewed` | an administrator opened a page or file in a view (once an hour per page) | `space`, `page` or `path`; `viewAs` in a view as a person |
| `view_as_started` | an administrator started a view as a person (about the person) | `viewAs`, `reason`, `until` |
| `view_as_ended` | an administrator ended a view as a person | `viewAs` |
| `bulk_started`, `bulk_finished` | a bulk job | `job`, `action`, `count`, `failed` |
| `space_archived`, `space_unarchived` | a space archived or restored from the archive | `space`, `bytes`, `by` (`controller` or a user) |
| `storage_band_changed` | the pressure band changed | `from`, `to`, `pressure` |
| `storage_reclaimed` | a reclaim step finished a batch (at most one row a minute) | `step`, `spaces`, `bytes` |
| `storage_waits_high` | restore waits rose above target and reclamation slowed, or fell back | `restoreShare`, `p95`, `slowed` |
| `storage_settings_changed` | `storage.siteBudget` or `storage.advanced` changed | `from`, `to` |

Rows carry the tenant of the space or person. A tenant administrator sees their
tenant's rows of these kinds.

### Archive

Owner, 2026-11-23. A space's cache state, from most to least on the server:
`full`, `partial`, `evicted` (3.1: the index only), **`archived`**: only the
space's row of the state database and small metadata stay (name, 3.9's small
icon copy, the stored page count and sizes). **The index is deleted too.**
Members, permissions, favourites, the remote and drafts are records of the
state database and are not touched.

- **Only a space with a remote may be archived.** A local-only space is never
  archived nor evicted: `409 archive_needs_remote`. Archiving checks what 3.1's
  eviction checks (remote `ok`, nothing to commit, no rescued commits, a fetch
  that shows the remote branch at the space's `HEAD`); if one fails, the space
  is not archived (`409 busy` for a manual archive, with `reason`).
- `POST /api/admin/spaces/{space}/archive`: administrators (a tenant
  administrator for their tenant's spaces). `202`; the pressure controller
  (below) archives on its own when it runs in `auto`.
- **Restoring** is fetching the text (a partial clone, or a full one where the
  remote cannot filter) and building the index again. It starts when someone
  opens the space or one of its pages, from the login prefetch (below), or by
  `POST /api/admin/spaces/{space}/restore` (`202`).
- **Staged restore.** An archived space, and an evicted one whose text is over
  50 MB, comes back in two stages: (1) the tree and the head of each page, and a
  light index (titles, folders, tags, properties): lists and titles work;
  (2) the bodies and the full-text index, in the background. A page opened
  meanwhile is fetched first. When the restore is done the whole text is on the
  server, as for any cached space (text bodies always stay local).
- **While archived**: the space is left out of global search; a search inside
  it, opening it or one of its pages starts the restore and answers
  `503 restoring` with `phase` (`waiting`, `fetching`, `light`, `indexing`),
  `eta` (seconds, estimated from the stored size and the recent fetch rate) and
  `Retry-After`; the app shows "正在恢复，约 N 秒". It gives no daily text.
  Backlinks that come from it or point into it are not shown (its index is
  gone). The space list shows it from its row, with its icon.
- The Space object's `storage.cache` gains `archived`, and `storage` gains
  `archivedAt` and, while restoring, `restore: {"phase", "eta"}`.

### Restoring when people come back

- When a person signs in, or opens the app with a valid session (counted at
  most once in six hours per person, on `GET /api/session`), the server queues
  the evicted and archived spaces they can reach, in this order: personal,
  favourites, used in the last 30 days, the rest (at most 20 of the rest each
  time). Nothing is queued twice.
- A restore someone is waiting for always goes before a prefetch.
- Prefetch runs only while the pressure band is `normal` or `elevated`, at
  most `prefetch.concurrency` (default 2) spaces at once on the server, and may
  not use the emergency reserve.
- `storage.advanced.prefetch.enabled` (default `true`) turns it off;
  `prefetch.activeDays` (default `null`) limits it to people active in the last
  N days (`users.last_active`). Both have a tenant layer (below).

### Storage pressure

Owner, 2026-11-23. One controller decides what to reclaim, instead of separate
thresholds. **Administrators set one value, the site storage budget**; the
rest derives from it, and the presets and every value sit in an "advanced"
section of the "存储与同步" page that is collapsed and hidden by default.

- **Budget** `storage.siteBudget`: bytes, at least 1 GiB, or `null`. `null`
  means 80% of the total size of the disk of the data directory.
- **Used**: the server's copies of spaces (`cloneBytes`), indexes
  (`indexBytes`), the attachment cache, the recycle bin, exports, and
  temporary files. Kept from stored measurements and incremental accounting
  (every write, fetch, eviction, archive, purge adds or subtracts); never by
  walking a disk on a request.
- **Pressure** = used ÷ min(budget, free disk + used), computed once a minute
  (`free disk` is one `statfs` of the data directory). Whenever the free disk is
  smaller than what is left of the budget, the disk wins.
- **Allocated**: the sum of the tenants' storage totals (`quota.bytes` in
  effect for each, see "Rules for a tenant"). It may exceed the budget (overcommit); the page
  then shows a warning.

**Bands** (balanced preset, the default):

| Band | Pressure | What the controller does |
|---|---|---|
| `normal` | below 70% | painless cleanup only: expired exports, temporary files, the attachment cache above its cap; evicts spaces not opened for 30 days |
| `elevated` | 70–85% | evicts spaces not opened for 14 days; shrinks the attachment cache to half its cap |
| `high` | 85–95% | archives the coldest spaces (not opened for 90 days; 180 days in the lower bands); pauses login prefetch, restores only spaces being opened |
| `critical` | above 95% | pauses new attachments and new spaces (`503 temporarily_unavailable`, see "What people are told"); **saving text always succeeds**; alerts site administrators |

Hysteresis: a band starts when pressure reaches its upper threshold and stops
when pressure is 10 points below it.

**What is reclaimed first** (least harm to people first):

1. expired exports and temporary files;
2. the attachment cache, least recently read first;
3. fetched old-version contents of spaces that follow a remote;
4. evicting idle spaces (the index stays);
5. archiving the coldest spaces (the index goes).

Within a step, candidates are ranked by **score** = bytes freed ÷ (expected
restore cost × probability of being needed soon). The probability comes from
the last open, the opens in the last 30 days, favourites, and the members'
recent sign-ins; the restore cost from the space's stored size. Both come from
stored columns; the ranking is one query per step.

**Hard rules**

- Never touched: spaces that are open or being edited, and spaces restored in
  the last 24 hours (`protectHours`).
- Never evicted nor archived: pinned and exempt spaces, local-only spaces, and
  spaces a tenant keeps (below).
- **Emergency reserve**: default max(5% of the budget, 2 GiB). Only text saves
  and restores someone asked for may use it; prefetch and background jobs may
  not.
- User-opened restores come before prefetch; prefetch runs only in `normal`
  and `elevated`, with bounded concurrency.
- **Per-tenant fairness**: the controller reclaims first from tenants over
  their storage allocation, then by score within each tenant.

**Waits people see.** The server records, per tenant and for the site, the
share of opens in the last 30 days that needed a restore and the restore wait
p95. When either rises above its target (5% and 30 seconds) for an hour, the
controller slows reclamation on its own (evict and archive days doubled, no
archiving below `high`) until it falls back, and tells site administrators to
raise the budget (a banner on the page and an audit row; mail when mail is
set up).

#### Settings: `storage.siteBudget`, `storage.advanced`

Reserved: the storage policy is layered settings now, and these would be too, as the site's settings, where they would take
`storage.siteBudget` (optional) and an optional `storage.advanced` object;
fields left out keep their values, `null` restores a field's default.

```json
{"storage": {"siteBudget": 2199023255552,
             "advanced": {"preset": "balanced", "mode": "auto",
                          "bands": {"elevated": 70, "high": 85, "critical": 95}, "hysteresis": 10,
                          "reserveBytes": null, "attachmentCacheBytes": null,
                          "evictDays": 30, "evictDaysElevated": 14,
                          "archiveDays": 180, "archiveDaysHigh": 90,
                          "protectHours": 24, "minArchiveBytes": 1048576,
                          "prefetch": {"enabled": true, "activeDays": null, "concurrency": 2}}}}
```

| Field | Default (balanced) | Range | Layers |
|---|---|---|---|
| `siteBudget` | `null` = 80% of the disk | ≥ 1 GiB | site |
| `advanced.preset` | `balanced` | `conservative`, `balanced`, `aggressive` | site |
| `advanced.mode` | `auto` | `auto`, `manual` (the controller only lists what it would do; `critical` still refuses) | site |
| `advanced.bands` | 70 / 85 / 95 | increasing, 50–99 | site |
| `advanced.hysteresis` | 10 | 1–30 points | site |
| `advanced.reserveBytes` | `null` = max(5% of budget, 2 GiB) | ≥ 0 | site |
| `advanced.attachmentCacheBytes` | `null` = 10% of budget | ≥ 64 MiB | site |
| `advanced.evictDays`, `.evictDaysElevated` | 30, 14 | 1–365 | site, tenant (`tighten`: fewer days) |
| `advanced.archiveDays`, `.archiveDaysHigh` | 180, 90 | 0 (never by time) or 1–3650 | site, tenant (`tighten`) |
| `advanced.protectHours` | 24 | 1–720 | site |
| `advanced.minArchiveBytes` | 1 MiB | ≥ 0 | site |
| `advanced.prefetch.enabled`, `.activeDays` | `true`, `null` | `activeDays` 1–365 | site, tenant (`tighten`: off, fewer days) |
| `advanced.prefetch.concurrency` | 2 | 0–16 | site |

The presets set the bands and the evict and archive days; a field set by hand
overrides its preset. `conservative`: bands 75 / 90 / 97, evict 60 / 30 days,
archive 365 / 180 days. `aggressive`: 60 / 80 / 92, evict 14 / 7, archive 90 /
45. `balanced` is the table above.

These replace 3.1's `cache.partial` (see below), `cache.evictAfterDays`,
`cache.minIdleHours`, `cache.clonesBytes`, `cache.minFreeBytes` and
`cache.assetsBytes`; `PUT` of those answers `400 invalid`. A tenant's layer is
set with `PUT /api/admin/tenants/{tenant}/settings/storage.advanced.{field}`
(3.3) for the fields marked "tenant"; a tenant may also keep spaces from
archiving: `PUT /api/admin/spaces/{space}/storage` takes `"keep": true` from a
tenant administrator (the space's index then counts toward the tenant's
allocation).

#### `GET /api/admin/storage/pressure`

Site administrators only. Server-level pressure (budget, used, pressure, band,
free disk, reclaims) is never shown to anyone else, in any answer or page. A
tenant administrator's "存储与同步" page reads
`GET /api/admin/tenants/{tenant}/storage/usage` instead: the tenant's used
bytes against its allocation, its waits, and its own next candidates, with no
band and no site figures.

```json
{"band": "elevated", "pressure": 0.78, "since": "2026-11-23T09:12:00Z",
 "budget": {"bytes": 2199023255552, "fromDisk": false}, "allocatedBytes": 2638827906662, "overcommitted": true,
 "usedBytes": 1715238154240,
 "used": {"clones": 1103806595072, "indexes": 214748364800, "attachmentCache": 109951162777,
          "recycle": 52428800000, "exports": 10485760000, "temp": 1073741824},
 "diskFreeBytes": 912680550400, "reserveBytes": 109951162777, "attachmentCacheCapBytes": 219902325555,
 "mode": "auto", "slowed": false,
 "recent": [{"at": "2026-11-23T09:20:00Z", "step": "evict", "space": {"id": "01JD…", "name": "Old notes"}, "bytes": 52428800}],
 "next": [{"step": "evict", "space": {"id": "01JE…", "name": "2024 trip"}, "bytes": 31457280, "score": 4.2}],
 "waits": {"restoreShare": 0.021, "restoreP95Seconds": 12.5, "opens": 18230},
 "partialSync": [{"host": "github.com", "partial": "yes", "checked": "2026-11-20T10:00:00Z"},
                 {"host": "git.example.org", "partial": "no", "checked": "2026-11-22T08:00:00Z"}]}
```

`recent` is the last 50 reclaims, `next` the ten likeliest next candidates.
Everything comes from memory and stored values; one query for `next`.

#### What people are told

Owner, 2026-11-23. **Server-level storage pressure is revealed to site
administrators only.**

- In the `critical` band, ordinary users and tenant administrators get
  `503 temporarily_unavailable` with `Retry-After` (300 seconds) for an upload,
  an import with attachments, or a new space:
  `{"error": "temporarily_unavailable", "action": "upload_attachment" | "create_space"}`.
  The body has no storage, budget or capacity words; nothing in the answer
  shows that the server is full. Text saves are never refused.
- The app shows the message only when the person tries; no persistent banner:

  | `action` | zh | en |
  |---|---|---|
  | `upload_attachment` | 暂时无法上传附件。请稍后再试。你正在写的内容会照常保存，也可以先插入图片链接。 | Can't upload attachments right now. Please try again later. Everything you write is still saved; you can insert an image link instead. |
  | `create_space` | 暂时无法新建空间。请稍后再试。你已有的空间都可以正常使用。 | Can't create a space right now. Please try again later. Your existing spaces work as usual. |

- Tenant administrators get the same answer with `"notified": true`, and the
  app adds 已通知全站管理员 / "The site administrators have been notified."
- Site administrators get `503 temporarily_unavailable` too when they upload,
  with `"cause": "storage"`, and the real cause on the "存储与同步" page: a red
  notice ("storage at N%, new attachments and spaces paused, reclaiming idle
  spaces; raise the site budget or add disk") with the top tenants and spaces
  by bytes; an audit row (`storage_band_changed`); and a mail alert at most once
  per 24 hours per state.
- **Quotas stay explicit**, being the person's or the tenant's own limits:
  - personal quota exhausted: `quota_exceeded` with `level: "person"`, `quota`,
    `used`, `limit`;
  - tenant quota exhausted: `quota_exceeded` with `level: "tenant"` (the app
    says "contact your organisation's admin");
  - file too large: `413 file_too_large` with `limit`.
  `actions` is reserved for a later `upgrade_plan` (billing); it is never sent
  yet.

### Partial sync is automatic

Owner, 2026-11-23. 3.1's `cache.partial` is removed: there is no switch.

- The first time the server connects to a remote host (host and port for a
  self-hosted one), it asks whether the host can filter a fetch (Git protocol
  v2 `fetch` advertising `filter`) and remembers the answer per host
  (`remote_hosts`). A host that can is cloned partially, one that cannot in
  full. When a filtered fetch is refused the host is checked again; every host
  is checked again every 30 days.
- With Git older than 2.44 every space is cloned in full (`git.partialClone`
  stays as 3.1 has it).
- Pinned spaces stay full clones.
- Text bodies always stay on the server (3.1's step 4 is not done).
- The only policy is 3.6's `storage.partial.unsupported`.
- The "存储与同步" page shows "supports partial sync: yes/no" per platform and
  host (`partialSync` above, and `capabilities.partialSync` of each provider).

### Errors of this section

| Code | Status | When |
|---|---|---|
| `file_too_large` | 413 | a file over `files.maxBytes`, with `limit`, `size`, `name` |
| `heir_required` | 409 | deleting a person who owns shared spaces alone and no heir can be chosen, with `spaces` |
| `owner_required` | 409 | restoring a space whose owners are all gone without naming one |
| `confirm_stale` | 409 | a bulk confirmation whose plan no longer holds |
| `exports_full` | 507 | the exports would exceed `exports.maxBytes` |
| `busy` | 409 | as in "Administration", for one space of an action; also archiving a space that fails the eviction checks, with `reason` |
| `archive_needs_remote` | 409 | archiving a local-only space |
| `temporarily_unavailable` | 503 | the pressure band is `critical`: an upload, an import with attachments, a new space; with `Retry-After` and nothing else that names storage (see "What people are told"). Saving text is never refused |
| `quota_exceeded` | 403 (spaces), 507 (bytes) | as 3.3, plus `level` (`person` or `tenant`) beside `quota`, `limit`, `used`; `actions` is reserved for `upgrade_plan` (never sent yet) |
| `file_too_large` | 413 | as above (with `limit`) |
| `restoring` | 503 | as 3.1, also for an archived space; with `phase`, `eta`, `Retry-After` |

### Settings this adds

| Key | Kind | Layers | Site default | Site's `lower` by default |
|---|---|---|---|---|
| `files.maxBytes` | limit in bytes | site, tenant | `--max-upload` (10 MiB) | `restrict`, `max` = the site value |
| `recycle.days` | limit, 1 to 365 | site (server-wide) | 30 | — |
| `exports.days` | limit, 1 to 365 | site (a site administrator may set it for one tenant: an export keeps the days of the tenant that made it) | 7 | — |
| `exports.maxBytes` | limit | site (server-wide: the room of every export) | 20 GiB | — |
| `users.reuseUsernames` | choice `after_purge`, `never` | site (server-wide) | `after_purge` | — |
| `storage.siteBudget` | bytes or `null` (80% of the disk) | site | `null` | — |
| `storage.advanced.*` | see "Settings: `storage.siteBudget`, `storage.advanced`" | site; tenant for the evict and archive days and prefetch | balanced | `tighten` for the tenant fields |

### Storage

SQLite and PostgreSQL.

- `recycle_items` (id, tenant_id, kind, space_id, user_id, parent item, records
  JSON, bytes, deleted_at, deleted_by, reason, purge state); indexes
  `(tenant_id, deleted_at DESC, id)` and `(deleted_at)` for the purge job.
- `users.deleted_at`, `users.recycle_item`, `users.last_active`,
  `users.first_write`; a partial index on users that are not deleted, for the
  lists.
- `usage` gains `held_bytes`, `remote_spaces`, `grace_spaces`, `overdue_spaces`,
  `evicted_spaces`, `pinned_spaces`, `connected`, `dormant`, `last_active`,
  `created` (copied so that the people's list sorts without a join), and the
  indexes above.
- `exports` (id, tenant_id, made_by, spaces JSON, state, bytes, path, created,
  expires).
- `retired_usernames` (the case-folded name, purged at), primary key on the
  name; read only when `users.reuseUsernames` is `never`.
- Every store method that lists users excludes deleted ones; the 3.3 scope test
  gains a check that no listing of `users` forgets it.
- Archive and pressure: `spaces.archived_at`, `spaces.keep`, `spaces.opens_30d`
  (kept by difference, corrected daily), the cache state `archived`; index
  `(tenant_id, cache_state, last_used, id)` for the candidates;
  `remote_hosts` (host, partial `yes`/`no`, checked); `storage_reclaims` (the
  last 1,000 rows, for `recent`); `restore_waits` (one row per day per tenant:
  opens, restores, p95); the settings in `site_records` and
  `layered_settings`.
- **The simulator** (`internal/storagepressure/sim`): deterministic (a fixed
  seed), synthetic people, spaces, sign-in and open patterns sized as 3.1's
  estimates, driving the real controller with a fake clock and disk. Its tests,
  part of the ordinary `go test` run and done in seconds, assert that used
  never exceeds the budget outside the reserve, that prefetch and background
  jobs never use the reserve, and that under the reference load the balanced
  preset keeps the restore share under 5% and the restore p95 under 30
  seconds. It is how the defaults are tuned.

## Tenant autonomy

Milestone 3.6 (contract, 2026-11-23; written for after 2.7, 3.1 and 3.5 are
merged). Accounts mode only. It finishes the administrators' permission model:

- **A tenant administrator may have anything that does not use the server's
  shared resources** (its disk, its outgoing capacity, its identity as a
  service) and does not reach outside their tenant.
- **A site administrator is the highest level**: they see and manage
  everything, in every tenant, and decide for every shared thing whether it is
  forced on all tenants or only a default.
- docs/design/milestones/phase-3-tenants.md lists every administrator-configurable
  thing and its class ("List of admin items"): **site-only** (a), **layered** (b: the site sets a value and
  whether it is forced, a minimum, or a default; tenants configure within it;
  spaces and people below where it makes sense), **tenant-owned** (c: each
  tenant has its own, independent of the others).
- **Capabilities**, set per tenant by site administrators, decide whether a
  tenant has a (c) item or a (b) tenant layer at all; the layered settings
  decide the values.

PetalBook is not launched (v0.x): nothing in this section exists to keep an
earlier deployment behaving as it did. The rules below are the rules; the
tables are described under "Storage of this section".

### Rules for a tenant

What a tenant may have at all, and how much, is not a table of its own: it is
the **site layer's rule for that tenant** in the layered settings (owner,
2026-11-25/26). A rule is a **lock** or a **restriction**, and may carry a
**value** the site gives that tenant in place of its own; it comes from one
**source** and may count only inside a **window**.

- **Features** (`features.*`, `mail.ownSender`) are whole things a tenant may or
  may not have: `features.ownSso`, `features.domains`,
  `features.managedAccounts`, `features.idpAdmins` (default off),
  `features.adminView`, `features.auditDownload`, `features.tenantAudit`,
  `features.ownCaptcha`, `features.ownConnections`, `features.ownDaily`, `mail.ownSender` (default
  **off**), and the setting that already existed, `features.offline`. A feature is on unless a rule says otherwise. "This
  organisation does not have feature X" is X's value `false`, locked, for that
  tenant.
- **A route declares its feature** (see "Features"): while the caller's tenant
  does not have it, the route is `403 feature_off`, naming the feature, before
  its handler runs.
- **Quotas and limits** are settings too: `quota.people`, `quota.spaces`,
  `quota.bytes`, `quota.pages` (the tenant's totals; `null` is no limit; no
  tenant layer, so only a rule sets them and a tenant administrator's write is
  `409 setting_locked`),
  `storage.pinnedBytes`, `storage.mode` (restrict = the modes allowed),
  `rate.apiPerMinute`, `rate.mcpPerMinute`, `rate.mailPerDay`,
  `rate.outboundPerMinute`. A restriction clamps the value in effect: "at most
  10" means at most 10, and no limit is above any maximum. What the limits
  sheet called `storage.maxFileBytes`, `storage.perPerson.*` and
  `sync.assetsRepository` are rules for the tenant over `files.maxBytes`,
  `quota.*PerPerson` and `storage.assets` (a restriction to `["space"]`).

**Sources.** A rule has a source: `manual` (a site administrator), `billing`
(the internal billing module, which writes only through the Go interface
`settings.BillingWriter`) or `plan` (the tenant's plan, below). The rule in
effect is the first **active** one in that order; a whole rule replaces the one
below it (a manual rule's value and lower both win over the plan's). The
precedence is otherwise that of "Settings by layer": the site's lock over
everyone, then the rule in effect for the tenant, then the tenant's own value,
then the rule's value, then the site's. There is **one rule per tenant, key and
source**.

**Windows.** A rule may have `from` and `until` (RFC 3339); it counts from
`from` up to, not including, `until`. A dated manual rule replaces an undated
one (the answer carries the warning `undated_value_replaced`); when its window
ends the value falls to billing, then the plan, then the site's, and does not
return to the old manual rule. `until` must be after `from` (`400 invalid`) and
must not have passed already (`400 setting_until_past`).
Reads are right without a timer: each tenant's cached settings record the next
start or end of a window and are computed again when the clock passes it. One
timer on the active server writes the audit entry for a boundary
(`setting_rule_started`, `setting_rule_ended`, no actor) and looks at the quotas
again; with nothing dated there is no timer.

#### `GET /api/admin/tenants/{tenant}/settings`

Every setting that can differ for the tenant, features and quotas included
(readable by the tenant's administrators, who see the rules but cannot change
them). Each entry's `tenant` has the tenant's own `value` and `lower`, and

```json
"rules": [{"source": "manual", "value": 5, "lower": null,
           "from": "2030-01-01T00:00:00Z", "until": "2030-02-01T00:00:00Z",
           "inEffect": false, "setBy": "6FAVM0992C"},
          {"source": "plan", "value": null, "lower": {"mode": "restrict", "max": 10},
           "inEffect": true}],
"next": "2030-01-01T00:00:00Z"
```

`rules` are at most three, in the order manual, billing, plan. `inEffect` marks
the rule applied (it is false while the site's lock over everyone beats the
rule); `underlying` marks the one that applies when it ends or is removed;
`next` is the next start or end of a window. `setBy` (the id of the site
administrator who set the rule) is shown to site administrators only. The
tenant object's `quota` is read from `quota.*` and is read only.

#### `PUT /api/admin/tenants/{tenant}/settings/{key}`

Besides `value`, `lower` and `siteLock` (see "Settings by layer"), a site
administrator gives `"siteRule": {"value": …, "lower": {…}, "from": …, "until":
…}` (any of them) or `"siteRule": null`. The API always writes the **manual**
rule. `400 setting_above_max` for a value above the rule's own maximum. Every
change is audited as `setting_changed`, with `source` and the window.

#### Plans

A **plan** is a bundle of rules: its rules are rows of the same table, and a
tenant that has the plan has them as the lowest source of its own. Plans are the
internal billing module's unit; the site's administrators define them.

- `GET`, `POST /api/admin/plans`, `GET`, `PATCH`, `DELETE /api/admin/plans/{slug}`
  (`site.tenants.manage`): `{slug, name, description, rules: {key: {value?,
  lower?}}, tenants}`. A plan's rules have no dates and are checked as a rule
  for one tenant is. For a limit the tenant also sets (`quota.*PerPerson`,
  `files.maxBytes`, `registration.perHour`) a plan's or billing's rule with a
  value must also set a maximum (`restrict`) or a `lock`, so a plan always caps
  (`400 setting_rule_needs_max`); a manual rule may still be a default. In `PATCH`, a key with `null` leaves the plan.
  `409 plan_in_use` (with `tenants`) when a tenant has a plan being deleted,
  `409 plan_limit` at 100 plans, `409 exists` for a slug in use. `PATCH`
  answers `affected`, the tenants that have the plan.
- `PUT /api/admin/tenants/{tenant}/plan` `{"plan": "slug"|null}` gives a tenant a
  plan. It clears **every** manual rule of the tenant, undated adjustments and
  dated grants alike (a grant that should outlive the change is set again
  after it), the rule of `features.tenantAudit` included (a site administrator
  sets it again after the change). Billing's rules stay. The answer is `{"plan", "cleared": [...]}`,
  each rule with its `key`.
- Plans are held in memory as one snapshot with its own generation; a plan
  change recomputes the tenants lazily at their next read.

**Cost.** A check is a cached read: one map load, three atomic loads (site,
tenant, plan) and a comparison with the next window boundary; no query and no
lock. A tenant with no rows of its own shares the site's map, or its plan's.
Changes made by another process (the command line, a standby server) are seen
through the one-row table `settings_rev`, read at most once a minute on a
request.

**Removed with the limits sheet**: `GET`, `PUT /api/admin/capabilities`,
`…/tenants/{tenant}/capabilities` and `…/tenants/{tenant}/limits` (404; their
keys are the settings above), the audit kinds `capability_changed`,
`tenant_limit_changed` and `capabilities_default_changed` (now
`setting_changed`), and the error code `capability_off` (now **`feature_off`**,
with `feature`).

#### Events

- A change writes `setting_changed`, and the start or end of a rule's window
  writes `setting_rule_started` or `setting_rule_ended` (no actor), to the audit
  log.
- Going over a limit writes `quota_exceeded` (at most once a day per tenant and
  key).
- A webhook for both is reserved for later; nothing is sent today.

#### `GET /api/admin/tenants/{tenant}/usage/export?from=&to=&format=csv|jsonl`

Site administrators, and the tenant's administrators: one row per day and key
(people, spaces, bytes, pinned bytes, API and MCP requests, mail, outgoing
calls), from `tenant_usage_daily`, written by the daily recount. At most 366
days per request; streamed. Audited (`usage_exported`).

### The billing boundary

Owner, 2026-11-23. **Nothing of billing or payment is built now**; this fixes
where it goes.

- Plans, limits, metering, invoices and renewals belong to an **internal
  billing module**. It writes rules through an internal interface
  (a Go interface in the server, not HTTP), with the source `billing`; plans
  are its data, and 3.6 builds only their storage and the site
  administrators' manual editing ("Plans").
- Only **payment** is external (Stripe, Alipay, WeChat Pay, …), through payment
  adapters and their callbacks. Reserved: the adapter interface and
  `POST /api/billing/callbacks/{adapter}`; no route exists.
- There is **no external billing system's token or API** that changes a tenant:
  an outside system changes a tenant's rules only through a payment callback
  handled by the internal module.
- The site administrators' manual editing of the rules (the tenant detail
  page's "Rules for this tenant", and the plans page) is built, and manual
  rules win over billing's.

### A site administrator's view of a tenant

Site administrators see and change every tenant's configuration, rules
and storage mode; every change is audited (owner, 2026-11-23).

- **The tenant detail page** (`/admin/tenants/{slug}` in the web app) has these
  sections: overview (plan, usage, people, spaces per storage mode, recent audit
  rows), plan, settings (every layered key, with the layer its value
  comes from), rules (features, quotas and limits, each with its source and
  window), storage (mode, storage target, spaces waiting to move), sign-in
  (SSO providers, social choices, domains, joining, managed accounts), mail and
  integrations (SMTP, captcha, daily text, connections), usage (and the usage
  export), members and
  administrators, audit. Each section uses the tenant's endpoints of this
  section, which site administrators may read and write for every tenant.
- `GET /api/admin/tenants/{tenant}/overview` answers the overview from stored
  values and memory (the tenant's `usage` row, the per-mode counts kept on it,
  the in-memory resources, the latest 20 audit rows of the tenant):

```json
{"tenant": {"id": "01JDT3NANT", "slug": "acme", "name": "Acme", "disabled": false},
 "plan": {"slug": "team", "name": "Team"}, "overrides": 2,
 "usage": {"people": 41, "spaces": 77, "bytes": 912680550, "heldBytes": 104857600},
 "storage": {"mode": "tenant", "spaces": {"server": 3, "own": 5, "tenant": 69},
             "target": {"kind": "github", "owner": "acme-notes", "state": "ok"}, "toMove": 8},
 "signIn": {"ssoProviders": 1, "domains": [{"domain": "acme.example", "state": "verified"}], "managed": false},
 "integrations": {"smtp": true, "captcha": false, "daily": 2, "connections": ["github"]},
 "siteSet": 3, "siteLocked": 1, "audit": []}
```

  The tenant's administrators may read their own tenant's overview.
- **Setting a tenant's values as the site.** A site administrator writes the
  tenant's layer with `PUT /api/admin/tenants/{tenant}/settings/{key}` as a
  tenant administrator would, and may add `"siteLock": true`: the tenant's
  administrators then cannot change that value (`409 setting_locked`, `reason.by:
  "site"`) until a site administrator removes the lock. `storage.mode` is such a
  setting; the storage target takes `siteLock` too.
- **What a tenant administrator sees**: in the settings, `setBy` (`site` or
  `tenant`: who wrote the tenant layer's value) and `siteLock`; in the
  rules, `source`; on each tenant-owned resource (providers, domains,
  mail, captcha, daily, GitHub, storage target), `updatedBy`
  (`{"role": "site" | "tenant", "user": {"id", "username"}, "at"}`). The pages
  mark what the site set with "由全站管理员设置" and what it locked with
  "由全站管理员锁定".
- **Audit**: every change to a tenant's rules, plan, settings, storage
  and resources writes a row with the tenant and `by.role` (`site` or
  `tenant`); the tenant's administrators see their tenant's rows, the site
  administrators' changes included.


### Forced, minimum, default

3.3's `lower` (`allow`, `restrict`, `lock`) gains one mode and a reason, and
applies to every layered (b) key of the inventory:

```json
{"mode": "allow"}
{"mode": "tighten", "note": "Company policy: sessions end after 7 days."}
{"mode": "restrict", "options": ["any", "github"]}
{"mode": "restrict", "max": 10485760}
{"mode": "lock", "note": "The server's disk is shared; every space must follow a remote."}
```

| Mode | The administrators' page calls it | Lower layers may |
|---|---|---|
| `allow` | 默认（下面可以改） / default only | set any valid value, looser or stricter than the site's |
| `tighten` | 最低要求 / minimum | set a value at least as strict as the site's **own value** |
| `restrict` | 限定范围 / limited to | pick among `options`, or stay at or under `max` (3.3) |
| `lock` | 强制 / forced | nothing: the site's value applies |

- Every key says what "stricter" means for it (`strictness`, in each entry of
  `GET /api/admin/settings` and of a tenant's settings; left out for a key that
  has none):
  - `higher`: a larger number is stricter, or `true` for a boolean (e.g.
    `signin.ssoOnly`, `signin.lockout.minutes`);
  - `lower`: a smaller number is stricter, `null` (no limit) being the loosest
    (e.g. `registration.perHour`, `sessions.maxDays`);
  - `order`: a choice is stricter the later it is in the key's `choices`, which
    are listed loosest first (`registration.mode`: `open` < `invite` <
    `closed`; `storage.partial.unsupported`: `allow` < `refuse`);
  - `subset`: a list is stricter when it keeps fewer of the entries above it,
    `["*"]` (everything) being the loosest (`signin.social`, `storage.hosts`);
  - `superset`: a list is stricter when it holds more, so a value is looser when
    it lacks an entry the floor has (`signin.passkeyRoles`);
  - `document`: a document, compared field by field by the key's own rule
    (`captcha.contexts`, see below).

  A key without an order, or without a tenant layer below the site, cannot be
  `tighten`ed (`400 invalid`); a floor that is not one of an `order` key's
  choices holds everything open to none (it counts as the strictest). The orders of
  the keys are in `settingStrictness` of the generated
  `web/src/api/generated/registries.json`.
- The site's value the minimum is held to is the site's own, or its rule's
  value for that tenant when a rule for it is in effect (the `floor` of the
  `reason` below). A tenant's value is in effect while it is not looser than
  the floor; a looser one that is already stored (the site became stricter
  after it was written) is kept with `inEffect: false`, the site's value
  applies, and the tenant's page says "overridden by the site's value". It
  counts again when the floor allows it. A write of a looser value is
  `409 setting_restricted`. `affected` of a site change to `tighten` counts the
  tenants whose own value is looser than the site's value now.
- **A key whose site value is for the site's own people.** The site's own
  value of `registration.mode` governs personal accounts only and never
  constrains an organisation (decision of 2026-11-27): an organisation is held
  by a rule written for that tenant alone. So the site's layer of such a key
  (`ownPeopleOnly: true` in `GET /api/admin/settings`) takes no lower mode but
  `allow` (`400 invalid` for `tighten`, `restrict` or `lock`, telling the
  administrator to use a rule for the tenant), and a tenant that has no rule
  sits on the key's tenant default (`invite`), not on the site's value. A rule
  for the tenant with `tighten` holds the tenant to that rule's value.
- One function decides what a tenant's layer is held to and sits on (`Def.heldTo`:
  the site's layer, or the site's rule for the tenant when one is in effect,
  its value and its lower each replacing the site's when it has one). The read
  (`inEffect`), the write check (`409 setting_restricted`), the count of
  `affected` and `reason` all use it, so they cannot differ.
- The default rule of `signin.ssoOnly` and `signin.ssoOnlyAdmins` is
  `tighten` (3.6 d4 made them tenant layers that only tighten; the site may
  now choose `allow` for them too).
- `note` (at most 200 characters, optional, any mode but `allow`) is shown to
  the tenant's administrators next to the setting. It is part of `lower`:
  the site's (`PUT /api/admin/settings/{key}`, `"lower": {"mode": …, "note": …}`),
  the site's rule for one tenant (`siteRule.lower.note`), a plan's rule, and a
  tenant's own rule for the layers below. A write that sends `lower` without
  `note` keeps the note stored; `"note": ""` clears it; a rule that allows
  everything has none (changing to `allow` clears it, and a note with `allow`
  is `400 invalid`), nor does a removed rule; a longer one is `400 invalid`.
  The note is not part of what the layers below are held to. It appears as
  `note` in every `lower` of the answers (`site.lower`, `tenant.rules[].lower`,
  `reason`), and the change is audited: `setting_changed` carries `note` when
  the writer sent one, and its `from` and `to` hold the rules with their notes.
- A lower layer's own `lower` must be at least as strict as the one above it
  (3.3's rule, `tighten` sitting between `allow` and `restrict`).
- Values that a change makes ineffective are kept, `inEffect: false`, as in
  3.3; `affected` counts the tenants.

**Cost.** The effective values are computed once per tenant and cached; a
request does one map lookup and reads two counters, with no lock and no query.
A site write makes every tenant compute again, a tenant's write only that
tenant. A setting a tenant has no row for shares the one entry computed with
the site's layer, so a tenant's map is the map itself (about 1 KB at 20 keys,
about 3 KB at 60) and some 300 bytes for each key it sets: ten thousand tenants
take about 30 MB. There is no cap on the number of tenants cached.

### What a tenant administrator sees of a site rule

`GET /api/admin/tenants/{tenant}/settings` (3.3) gives each setting:

```json
{"key": "storage.mode", "kind": "choice", "layers": ["site", "tenant", "space"],
 "site": {"value": "server", "lower": {"mode": "restrict", "options": ["server", "own", "tenant"], "note": "…"}},
 "tenant": {"value": "tenant", "lower": {"mode": "lock"}, "inEffect": true, "setBy": "site", "siteLock": false},
 "effective": "tenant",
 "editable": true, "forced": false, "allowed": ["own", "tenant"], "reason": {"by": "site", "mode": "restrict", "note": "…"}}
```

- `editable`: whether this caller may change the tenant's value now (`false`
  when the site forces it, when a site administrator locked the tenant's value,
  `siteLock`, or when a rule for the tenant leaves it nothing to choose).
- `setBy`: who wrote the tenant layer's value, `site` or `tenant`.
- `allowed` is what the site's rule and the rule for the tenant both leave.
- `forced`: the site locks it. `reason` is then `{"by": "site", "mode": "lock", "note": "…"}`
  (`by` is `site` when the site's own layer holds the tenant and `rule` when the
  site's rule for this tenant does; a rule that allows everything gives no
  `reason`, and the page shows no badge for it);
  for `tighten` and `restrict` the reason says so as well: it is the rule the
  tenant's layer is held to (the site's, or its rule for this tenant), with
  `options` or `max` for a restriction and, for `tighten`, `floor`, the value
  the tenant's own must be at least as strict as. It is left out while the site
  allows everything. `reason` and `strictness` are in the answers of `GET` and
  `PUT` of `/api/admin/tenants/{tenant}/settings`; `allowed`, `editable` and
  `forced` of the example are not made yet. The page shows a forced setting read only, greyed,
  with "由全站管理员统一设置" and the note.
- `GET /api/settings?space=` (3.3) answers the same `reason` for the space and
  person layers (`by` is `site` or `tenant`).

A tenant administrator never sees the site's value of a secret, a site-only
setting, or anything about another tenant.

### The layered keys

`GET /api/admin/settings` lists every key with its kind and layers (`serverWide`
gives the reason for a key that has no tenant layer); the registry
(`internal/settings/registry.go`) is the list. By prefix, with the layers each
has (S = the site, T = a tenant, Sp = a space, P = a person; "S + rule" is the
site layer with a rule for one tenant, which is how a feature or a limit of a
tenant is set, see "Rules for a tenant"; "S!" is the server's alone):

| Keys | Kind | Layers |
|---|---|---|
| `features.tokens`, `features.mcp`, `features.offline` | boolean | S, T |
| `features.ownSso`, `.domains`, `.managedAccounts`, `.idpAdmins`, `.adminView`, `.auditDownload`, `.tenantAudit`, `.ownCaptcha`, `.ownConnections`, `.ownDaily`, `.githubConnect`; `mail.ownSender` | boolean (features) | S + rule |
| `features.passkeys`, `features.tenancy` | boolean | S! |
| `signin.social` | list: provider slugs, or `["*"]` (default) | S, T (3.6 d3) |
| `domains.joinRole` | choice: `member` (the account roles that are not administrator's) | S, T (3.6 d5) |
| `signin.ssoOnly`, `signin.ssoOnlyAdmins` | boolean (`true` is stricter; tenant layer only tightens) | S, T (3.6 d4) |
| `signin.mode` | choice `dual`, `sso_first`, `sso_redirect`, `local_only`; no order, default rule `allow` | S, T |
| `signin.passkeyRoles` | list of `admin`, `member`; `superset`; default rule `tighten` | S, T |
| `sessions.idleDays`, `sessions.maxDays` | limits; `lower`; default rule `tighten` | S, T |
| `captcha.contexts` | document; `document`; default rule `tighten` | S, T |
| `audit.signins`, `.reads`, `.downloads`, `.api` | boolean | S, T |
| `audit.keepDays`, `.keepRows`, `.optionalKeepDays`, `audit.keepMode` | limits (`keepDays` 0 for no age limit), choice `or`, `and` | S! |
| `audit.keepDays.<category>` (eight) | limit, 0 (forever) to 3,650, or none | S! |
| `quota.people`, `.spaces`, `.bytes`, `.pages` | limit or none | S + rule (no tenant layer) |
| `quota.spacesPerPerson`, `.bytesPerPerson`, `.pagesPerPerson` | limit | S, T, P |
| `rate.apiPerMinute`, `.mcpPerMinute`, `.mailPerDay`, `.outboundPerMinute` | limit or none | S + rule |
| `storage.mode` | choice `server`, `own`, `tenant` | S, T |
| `storage.partial.unsupported` | choice `allow` < `refuse`, default `allow` | S, T |
| `mail.useSite`, `daily.useSiteApis`, `connections.useSiteApps` | boolean, default on | S, T |
| `outbound.perMinute`, `outbound.tenantPerMinute` | limit, 1 or more, default 600 and 120 | S! (the cap counts the requests made for every tenant's configuration together) |
| `storage.assets` | choice `space` < `repository` | S, T, Sp (the owner's request when the space is made) |
| `storage.assets.ownStorageExempt`, `storage.pinnedBytes` | boolean, limit | S + rule |
| `storage.remotes.require`, `.scope`, `.graceDays`, `storage.localSpaces.maxBytes` | text, choice, limits | S, T (the site's lock: a tenant may only tighten) |
| `storage.hosts`, `cache.*` | list, limits | S! |
| `files.maxBytes`, `registration.perHour`, `registration.domains`, `legal.*` | limit, list, text | S, T |
| `daily.own`, `daily.api`, `daily.default` | boolean, boolean, document | S, T, Sp |
| `daily.languages`, `recycle.days`, `exports.maxBytes`, `users.*` | list, limits, choices | S! (`exports.days`: S + rule) |
| `signin.*`, `sessions.*`, `captcha.*` | sign-in policy | S! (the sign-in page comes before any tenant is known) |
| `domains.checksPerMinute` | limit, 1 to 100,000, default 60: the checks of claimed email domains people may ask for in a minute, on the whole server | S! (the server's own resolver asks DNS for every tenant) |

Notes:

- `storage.*` other than `storage.mode` concern `server` spaces and remotes as
  3.1 does; the site may loosen each to `allow` (the site's value is a default)
  or lock it. 3.1's `storage.remotes.require` is derived from `storage.mode`
  (see "Storage modes").
- The keys `mail.useSite`, `daily.useSiteApis`,
  `connections.useSiteApps`, `outbound.perMinute` and `outbound.tenantPerMinute`
  are in the registry, with their defaults, before the features that read them
  are built; until then nothing reads them (their help says "not in effect
  yet"). `storage.mode` and `storage.partial.unsupported` are read (see "Storage
  modes" and the note below). The three `useSite` switches default to on, which is what a tenant
  gets today. A site administrator who wants one off for a tenant has to
  **lock** it off (a rule for the tenant, or the site's lock): an off that is
  not locked is only a default, and the tenant's administrators can turn it
  back on.
- `storage.partial.unsupported` (owner, 2026-11-23) is the only partial-sync
  policy. With `refuse`, connecting a space to a host that 3.5's detection found
  unable to filter (`partialSync: "no"`) answers `409 partial_unsupported` with
  `host` and `provider`. It is read at connect time, for the space's tenant (the
  tenant's value, else the site's), for every way of connecting a space to a
  provider: `PUT …/remote` with a `connection` credential and a new space made
  with `storage.sync` (nothing is made on the host). It applies to every space,
  not only those that need partial sync, and whether or not the server clones
  partially. A remote given as a plain address (`remote` with a token, or `PUT …/remote` without a
  `connection`) whose host is a provider's (compared as host names: lower case, no
  port, no trailing dot, IDNA's ASCII form) is held to what is known of that
  provider, so an address does not get round the policy. A host not found out yet
  (`unknown`), and an address no provider has, is let through; the detection of 3.5 fills it in as the
  provider is used. **Nothing is migrated**: a space that is connected already
  keeps its remote and keeps syncing when the policy turns to `refuse`; only a
  new connection (also reconnecting, or changing the remote) is looked at. With
  `allow` they are cloned in full.
- **Exemptions** (3.1's `remoteExempt` on a user, `exempt` on a space) are
  recorded with the layer that granted them: a tenant administrator's exemption
  releases a space only from what the tenant added on top of the site; the
  site's own value still applies. A site administrator's releases from both.
- **Pinning** (3.1) keeps a space's whole history and attachments on the
  server's disk, so it is the site's: a tenant administrator may pin up to the
  tenant's `storage.pinnedBytes` (`0` by default, so tenant
  administrators cannot pin until the site allows it):
  `409 quota_exceeded` with `quota: "pinnedBytes"`. Evicting is open to tenant
  administrators (it only frees the server's disk).
- `storage.assets` at the space layer is the owner's choice for their space (a
  record of the state database, as 3.1's per-space values, not `space.yml`).
  When the effective rule requires `repository`, it applies to new spaces and to
  a space when it connects a remote; an existing space keeps its attachments
  where they are and is shown the one-step move. Nothing is moved on its own.

#### Where attachments go (`storage.assets`)

Attachments are separate by default: a new space made with a repository on a
provider also gets `<name>-assets`. A space with no remote (a `server` space)
has nowhere to put a second repository and keeps its attachments in the space,
with no error. The layers apply in the owner's order, strongest first:

1. **The site's lock over everyone** (`PUT /api/admin/settings/storage.assets`
   with `lower: {"mode": "lock"}`): the site's value is the value, and what
   tenants and owners set is kept, `inEffect: false`.
2. **The site's lock for one tenant**: a site administrator sets the tenant's
   value and locks it with `PUT /api/admin/tenants/{tenant}/settings/storage.assets`
   `{"value": "repository", "siteLock": true}`. The tenant's administrators get
   `409 setting_locked` for the value, its rule, and `siteLock` itself;
   `siteLock: false` (site administrators only) frees the value, which then
   stays as the tenant's own. A lock needs a value (`400 invalid` otherwise).
   `siteLock` exists only on the tenant's layer. The tenant's row answers
   `siteLocked`.
3. **The site's default** (`value`, with `lower` `allow`).
4. **The tenant's default** (the tenant's `value`, which overrides the site's
   when the site allows it).
5. **The space's owner**, who may ask for either at creation
   (`storage.sync.assets`) inside what is left: `lower` of the tenant's layer
   (and the site's) says whether `space`, `repository` or only one of them
   remains.

A request that says nothing gets the value in effect; a request that asks for
what the rules leave out is refused as above.

Where the second repository cannot be had:

- **A space with no remote** (a `server` space) has no second repository to make
  and keeps its attachments in the space, with no error, under any layer.
- **A space on a provider, under a default** that is not locked: the creation
  does not fail; the attachments stay in the space. This covers an existing
  repository with no `<name>-assets` beside it, a name that is taken, and a
  credential that cannot reach it. A repository made but unusable is
  remembered as a leftover, as other repositories left empty are.
- **A space on a provider, under a lock to `repository`** (the site's, the
  site's for the tenant, or the tenant's): no fallback. The creation fails with
  the provider's own error, and nothing is kept (what was made is named, as for
  any failed creation). The lock means the space is not made without it.
- A request that asked for an attachment repository is an error, as before.

An existing `<name>-assets` repository that the person can push to is adopted
as it is when the space connects to existing repositories (`create: false`);
nothing in it is changed or checked beyond the first fetch. With `create: true`
a name that is already taken is not adopted (`409 name_taken`, or the fallback
above under an unlocked default).

When the tenant's limits table turns `sync.assetsRepository` off, only `space`
is possible, whatever the layers say. That includes attaching a repository to an
existing space (`PUT /api/spaces/{space}/remote/assets`): `409 setting_restricted`
when the space has none; a space that already
has one keeps it and may reconnect it.
`storage.assets.ownStorageExempt` (site only, default `true`): when on, a tenant
that stores on a target of its own (its own organisation or host connection,
`storage.modes` `tenant`, 3.6) is not bound by the site's value or lock for
`storage.assets`, including a lock the site made for that one tenant; it chooses
freely, and its own layers (its value and its rule for owners) still apply. Off:
such a tenant is bound like any other. **Status:** the setting is stored, read
and edited now, and `settings.AssetsRule` honours the exemption, but no tenant
has a target of its own until 3.6, so nothing is exempt yet; 3.6 passes
`OwnStorage` when it makes `tenant` spaces (and `RepositoryOff` from the limits
table). The `GET /api/admin/settings` and tenant settings answers give a choice
setting's `choices` (loosest first).

### What stays the site's

The inventory's (a) items, and the reasons that matter for security:

- **Per-address limits, failure caps and lockout** (`limits.*`, `lockout.*`):
  sign-in does not know the tenant until the account is found, and a tenant's
  stricter cap would show, by when it starts refusing, that a username exists
  and which tenant it is in (2.8 counts failures for names nobody has, so that
  nothing tells them apart).
- **The `login` and `setup` captcha contexts**: the same reason; the sign-in
  form is the server's. A tenant's captcha keys and contexts apply to the forms
  that name the tenant (`register`, `forgot`, `invite`).
- **Passkeys on or off**: their relying party is the server's host.
- **The server itself**: `--listen`, `--public-url`, `--data`, `--database`,
  `--trusted-proxy`, `--allow-origin`, the process limits, the secret key,
  tenancy, tenants' creation, deletion, `disabled` and `slug`, **every
  tenant's rules and plan** (which hold its totals), the server status,
  3.5's site storage budget and pressure settings (which replace 3.1's cache
  and eviction settings; tenants tune only the fields marked tenant), `storage.modes.available`, the
  recycle bin's retention and the exports' limits of 3.5,
  `users.reuseUsernames`, the outbound cap below, the list of public mail
  domains, the manual domain-check cap, `--max-upload` and `--max-import` as
  ceilings.
- **Unique usernames and email addresses**: sign-in names no tenant, so both
  are unique on the server (see "Signing in by email address").

### Tenant-owned resources

Every endpoint below exists as `/api/admin/tenants/{tenant}/…`, for the
tenant's administrators and for site administrators. `{tenant}` is the id; a
tenant administrator who names another answers `404 not_found`. The site's own
counterparts keep their 2.x/3.x addresses (`/api/admin/providers`,
`/api/admin/mail`, …) and stay site administrators only.

Each resource is gated by a feature (see "Rules for a tenant"; `403
feature_off` for writes while it is off) and carries `updatedBy`.

All of them are held in memory per tenant, loaded at start and re-read on
change (one tenant's at a time), so no request reads them from the database:
providers (2.8 already), the SMTP settings, the captcha keys, the daily
interfaces, the providers (2.7b) and the storage targets. A tenant may have at
most: 10 SSO providers, 20 email domains, 1 SMTP sender, 1 captcha
configuration, 20 daily interfaces, 1,000 pasted daily lines, 1 GitHub App and
5 GitLab or Gitea providers (3.8), 1 storage target in use.

#### Outgoing requests made for a tenant

A tenant administrator chooses addresses the server will connect to (an
identity provider's issuer, a SAML metadata address, an SMTP host, a daily
interface, a storage target). The server is the one making those connections,
so:

- **The public-address check of 2.3** applies to every such connection, at
  connect time, on the address the name resolves to: no loopback, private,
  link-local, multicast, unique-local or unspecified address, and no
  `localhost`; redirects are followed only to addresses that pass too (and at
  most 3). An SMTP host must also pass, which leaves out 3.3's `none` (loopback
  relays only) for tenants: a tenant's SMTP is `starttls` or `tls`, on ports
  25, 465, 587 or 2525.
- **A site-wide outbound cap** (`outbound.perMinute`, site only, 600 by
  default) counts every request made for any tenant's configuration, and each
  tenant has at most `outbound.tenantPerMinute` of it (site only, 120 by
  default); over it the request is not made and the feature answers as when the
  remote is unreachable (`unavailable`). The site's own configuration does not
  count against it.
- Answers are bounded (2.3's 64 KB for daily interfaces; 1 MB for discovery
  documents and metadata), with the same timeouts as the site's.
- Secrets are sealed with the server's key with the tenant's id in the
  purpose (so a sealed value cannot be moved to another tenant's record), never
  returned (`hasSecret`), and shown in the audit log as `"changed"`.

#### Sign-in: social providers and SSO

Sign-in providers are of two kinds, as consumer products do it.

- **Social sign-in** (`class: "social"`): consumer accounts (Google, GitHub,
  Microsoft personal accounts, Apple, GitLab.com and the like). **The site
  configures them** (`/api/admin/providers`). Each is a
  "Continue with Google" button on every sign-in page. It signs a person in to
  the account its identity is attached to, in any tenant, as today; with
  registration open for the page's tenant it can register (see below).
  `signin.social` (its tenant layer, 3.6 d3) lets a tenant narrow which social
  providers its people may use;
  an identity from a provider the person's tenant does not allow is refused
  after it is resolved (`sso_error=not_allowed`, the same as for a GitHub
  organisation that does not match).
- **SSO** (`class: "sso"`): enterprise OpenID Connect or SAML, owned by a
  tenant with `signin.sso` (`/api/admin/providers` in the tenant's scope, the same Provider
  object and refusals as 2.8). It signs people in only to accounts of its
  tenant, makes accounts only there (as the tenant's `registration.mode` allows), and maps groups only to the
  tenant's groups. A site administrator may also own SSO providers and offer
  each to all tenants or to a list (`tenants`).

`class` and the owner are chosen when a provider is made and never change (the
page proposes `social` for a GitHub provider on `https://github.com` and for the
issuers of Google, Microsoft's personal accounts, Apple and GitLab.com, `sso`
for everything else).

The Provider object gains:

```json
{"class": "sso", "owner": "01JDT3NANT", "tenants": null, "domains": ["acme.example"], "…": "…"}
```

`owner` is a tenant id, or `"site"`; `tenants` is for a site-owned provider
(`"all"` or a list of ids; `null` for a tenant's); `domains` are the tenant's
verified domains routed to this provider (below).

**Linked accounts.** A linked account is keyed by (the provider's own id,
subject): the provider row's id, never the issuer string an administrator
typed. The provider's `owner` is `site` for the site's and the tenant for a
tenant's SSO. So two tenants may both use the same identity provider (two
companies on Microsoft Entra ID, or one issuer with two applications) without
sharing accounts, and an organisation that copies another's issuer reaches none
of its accounts: issuers may repeat. A person has one linked account per
provider. A tenant's SSO provider:

- finds accounts only by `(its provider id, subject)`, and the account must
  be of its tenant (checked again on the row, so a bad row cannot cross);
- never attaches an identity to an account by its address (3.6, "Signing in
  by email address"), and vouches for an address only on its tenant's
  verified domains;
- **never signs in a site administrator**, even one whose identity is linked:
  a tenant administrator controls that provider and could point it at an
  identity provider of their own (`sso_error=not_allowed`, audited);
- keeps its linked accounts when its issuer, SAML entity id or GitHub base
  address changes: they belong to the provider row, not to the address.

A tenant's own providers are managed at `/api/admin/tenants/{tenant}/providers`
(3.6 d3, below); a site administrator may also make one with
`POST /api/admin/providers` and `"owner": "<tenant id>"`. Slugs stay
unique on the server (they are in the callback addresses registered at the
identity provider), whoever owns them, and the database holds that with one
unique index on `slug` (a GitLab or Gitea host's id is its slug, so they share
it); `409 exists` says only that the slug is taken. The reserved slugs
(`github`, `git`) stay reserved (`409 slug_reserved`). `kind` `github` is
social only, and a tenant's own provider is `sso` only (`400 invalid`).
`class` defaults to `social` for `github` and a template, `sso` otherwise;
a `PATCH` with another `class` is `400 invalid`, and the store never writes
it after the row is made.

**An organisation's own SSO** (3.6 d3, built). The routes need
`tenant.signin.manage` (every tenant administrator holds it) and the feature
`features.ownSso` (`403 feature_off` while it is off for the tenant; a site
administrator is not bound by it). `{tenant}` is one the caller may manage
(`404 not_found` otherwise), and `{id}` a provider that tenant owns (another
tenant's, or the site's, is `404 not_found`):

- `GET /api/admin/tenants/{tenant}/providers`: `{"providers": [Provider]}`,
  the tenant's own, in order;
- `POST …/providers`: the body of `POST /api/admin/providers` for `kind`
  `oidc` or `saml`; `owner` is the tenant (left out, or the same id; another is
  `400 invalid`). Its `slug` starts with the tenant's own slug and a hyphen
  (`acme-okta`; `400 invalid` otherwise, also through `POST /api/admin/providers`
  with an `owner`), so plain names stay the site's to take and no tenant takes
  another's. A template, `kind` `github` or `oauth2`, `class` `social`,
  `mergeVerifiedEmail: true`, a `tenant` (jit) other than its own and
  `tenants` are `400 invalid`. It starts off until the tenant has a verified
  domain (`409 domain_required` when asked on). `201` with the Provider;
- `GET`, `PATCH`, `DELETE …/providers/{id}` (`?force=1` as the site's),
  `POST …/providers/{id}/check`: as the site's;
- `POST …/providers/{id}/test` (`{"return": "/admin/…"}`, optional): a test
  sign-in that comes back to `return` (an address of the console; the
  organisation console's sign-in tab without one) with `?provider_test={id}`,
  read once with `GET …/providers/{id}/test`.

Its audit rows are `tenant_provider_created`, `…_changed`, `…_deleted`, the
tenant's (its administrators read them); the site's own stay `provider_*`,
site-only. At most 50 providers per owner (`409 provider_limit`).

Signing in through a tenant's provider (`/api/auth/{kind}/{slug}/…`, the slug
being unique on the server):

- the provider works only while it is on, the tenant has `features.ownSso`
  and a verified domain; otherwise its start is `404 not_found` and a callback
  `sso_error=provider_unavailable` (nothing is deleted: it works again when they
  are back);
- the account found by `(provider id, subject)` must be of the provider's
  tenant, checked on the account at every sign-in, and must hold no site key
  (the built-in site administrator or a custom site role): otherwise
  `sso_error=not_allowed`, audited as `login_failed` (`reason: not_allowed`)
  in the account's tenant. Linking it (`POST /api/account/links/{id}/start`)
  is offered to the tenant's people only (`404 not_found` for anyone else, and
  `sso_error=not_allowed` for a link started otherwise); it makes no first
  administrator (`setup`), and an invitation it accepts must be of its tenant
  and not a site administrator's;
- its sign-ins make accounts in the provider's tenant only (its `tenant` is
  its owner), as members, when that tenant's `registration.mode` is `open`;
- `groups.map` names the tenant's own groups only (another's is
  `404 group_not_found`, as one that does not exist), and a sign-in never syncs
  anyone into a group that holds a role of the site (the "no site key" check is
  asked again after the sync);
- `groups.allow` (any OIDC or SAML provider, the site's too): when not empty,
  only people whose groups claim (`groups.claim`) names one of these values
  sign in, at every sign-in, whether or not the groups are synced; a missing
  claim lets nobody in (`sso_error=not_allowed`). Who the identity provider
  takes out of the group signs in no more; the account stays.

**A site SSO provider offered to tenants.** `tenants` on a site provider of
class `sso` is `"all"` (the default) or a list of tenant ids (at most 1000;
`400 invalid` on a social provider, which `signin.social` prunes instead). With
a list, only the people of those tenants sign in or link through it, and
no account is made in a tenant not on it (`sso_error=not_allowed`).
A site administrator signs in through the site's own SSO whatever its list
says: the list cannot lock them out.

**`signin.social`** (site and tenant, a list: the slugs of the site's social
providers the tenant's people may use, or `["*"]` for every one, the default).
The site's value is the default and its lower rule says whether a tenant
prunes it; a rule for one tenant may lock it. It is read once a sign-in has
found its account (or the tenant it would make one in): an identity
of a provider the account's tenant does not allow is `sso_error=not_allowed`.

**Outgoing requests.** A tenant's provider reaches public addresses only (its
discovery document, keys, tokens and SAML metadata; the check of "Outgoing
requests made for a tenant", on the address each name resolves to, at most 3
redirects). The per-tenant outbound cap is not applied to sign-in yet.

**Several sign-in methods per account** (owner, 2026-11-23). An account may
have a password and any number of identities (Google, GitHub, Microsoft, …) at
once, and any of them signs in to the same account. Linking stays 2.8's
(`POST /api/account/links/{provider}/start`), unlinking is
`DELETE /api/account/links/{id}` (`409 last_method` when it would leave no
way to sign in), and an account without a password adds one with
`POST /api/account/password` without `current`. 3.6 adds:

- `GET /api/account` gains `methods`, one list for the account page's
  "登录方式":

  ```json
  "methods": [{"type": "password", "lastUsed": "2026-11-23T08:00:00Z"},
              {"type": "identity", "id": "01JD3M2A4C", "providerSlug": "google", "kind": "oidc",
               "class": "social", "label": "kazuha@example.com", "lastUsed": null},
              {"type": "passkey", "id": "01JD3N7X2B", "name": "MacBook", "lastUsed": "2026-11-22T09:10:00Z"}],
  "canUnlink": true
  ```

  `passkeys` and `hasPassword` stay. `canUnlink` is whether
  removing one method would still leave one.
- **Templates.** `GET /api/admin/provider-templates` (site
  administrators): the presets shipped with the server (`google`, `microsoft`,
  `apple`, `github`, `gitlab-com`), each with its `kind`, issuer or addresses,
  scopes, `class: "social"` and icon (3.9):

  ```json
  {"templates": [{"id": "google", "name": "Google", "kind": "oidc", "class": "social",
                  "icon": "preset:google", "issuer": "https://accounts.google.com",
                  "authorizationEndpoint": "https://accounts.google.com/o/oauth2/v2/auth",
                  "tokenEndpoint": "https://oauth2.googleapis.com/token",
                  "jwksUri": "https://www.googleapis.com/oauth2/v3/certs",
                  "scopes": ["openid", "email", "profile"], "secret": "client",
                  "common": true, "region": "global"}]}
  ```

  `secret` is what the administrator gives besides the client id (`client`, or
  `apple`); `responseMode` is `form_post` for Apple; `common` and `region`
  order the page's picker ("More templates" holds the others). `github` is the
  `github` kind on `https://github.com`; the others are `kind: "oidc"`.

  `POST /api/admin/providers` takes `{"template": "google", "slug": "google",
  "clientId": "…", "clientSecret": "…"}` (`slug`, `name` and `icon` default
  to the template's); the template fills the rest. Its issuer, addresses,
  token-endpoint methods and scopes are **copied into the row**, with
  `"template": "google"` and `class: "social"`: a template that changes in a
  later release does not change providers already made, and nothing is
  discovered or fetched (the addresses are the template's). The Provider
  object has `template` (`""` for none), fixed like `class`; the issuer of a
  provider made from a template cannot change (`400 template_fixed`), its
  client and scopes can. An unknown template is `400 template_unknown`; a
  template is site-owned and social only (`400 invalid` otherwise). The
  addresses a provider made from Google's, Apple's or Microsoft's template
  gives follow that template's trust rule (below); GitLab.com's those of a
  configured provider.

  - **Microsoft** is multi-tenant (`/common`): its issuer is
    `https://login.microsoftonline.com/{tenantid}/v2.0`. Each token's `iss`
    must be that with the token's own `tid` (a tenant id) in place of
    `{tenantid}`; it is checked, never skipped, and a token whose `iss` names
    another tenant, or with no `tid`, is refused (`sso_error=idp_error`).
  - **Apple** takes `{"template": "apple", "clientId": "<Services ID>",
    "apple": {"teamId": "…", "keyId": "…", "privateKey": "<AuthKey_….p8>"}}`
    and no client secret (`400 invalid`): the client secret is an ES256 JWT
    the server signs with that key (`iss` the team, `sub` the client, `aud`
    `https://appleid.apple.com`, 180 days), when it needs one, and replaces
    once it has less than 7 days left (nothing renews it on a timer). The key
    is the site's secret `signin.appleKey` (write-only, changed only here, so
    one Apple provider per site: a second is `409 exists`; deleting the
    provider clears it); a key that is not a P-256 PKCS #8 PEM is
    `400 apple_key_invalid`. The Provider object has
    `"apple": {"teamId", "keyId", "hasKey"}`; `PATCH` takes the same `apple`
    block. Apple answers with `response_mode=form_post`: it posts `code`,
    `state` and, at the first sign-in only, `user` (the name) to
    `POST /api/auth/oidc/{slug}/callback` (a form, from Apple's site; only
    for a provider that asked for it, otherwise a redirect to
    `/login?sso_error=provider_unavailable`). That answers a small page of this
    site that goes on to `GET …/callback` with `code`, `state` (and `error`)
    only, so the browser's `SameSite=Lax` cookies (the attempt's binding, the
    session for a link) are sent and the callback checks the attempt as for
    any redirect. The name in `user` is kept on the server with the attempt
    (by its state, taken with it, once) and never put in that address; the
    address in `user` is dropped (the ID token's is the one used).
  - The template ids `google`, `microsoft`, `apple` and `gitlab-com` are
    reserved slugs for the site's own providers: an organisation's provider
    cannot take one (`409 slug_reserved`). The `template` column takes only
    the known templates, in the database too.
- **Custom OAuth 2.0 providers** (`kind: "oauth2"`, 3.6 d6). Besides the
  templates and OpenID Connect providers (by discovery document) a site
  administrator may add a provider that has only plain OAuth 2.0: no ID token
  and no discovery (Discord, a self-hosted GitLab without OpenID Connect). The
  body takes a nested `oauth2` block, like `oidc`, `saml` and `github`:

  ```json
  {"slug": "discord", "kind": "oauth2", "name": "Discord",
   "oauth2": {
     "authorizationEndpoint": "https://discord.com/oauth2/authorize",
     "tokenEndpoint": "https://discord.com/api/oauth2/token",
     "userinfoEndpoint": "https://discord.com/api/users/@me",
     "scopes": ["identify", "email"],
     "authMethod": "client_secret_basic",
     "fields": {"subject": "id", "email": "email", "emailVerified": "verified", "name": "global_name", "username": "username"},
     "clientId": "…", "clientSecret": "…"}}
  ```

  The Provider object has the same block (`clientSecret` never; `hasSecret`
  and `redirectUrl`, `…/api/auth/oauth2/{slug}/callback`, instead). `PATCH`
  takes any part of it.

  - The three addresses are `https` addresses of at most 2048 characters with
    no user-info part, no query and no fragment. `scopes` has at most 32
    entries. `authMethod` is `client_secret_basic` (the client in the
    `Authorization` header, the default) or `client_secret_post` (in the form);
    it is never guessed. PKCE (`S256`) is always used. The site's own providers
    are not held to public addresses, as its OpenID Connect, SAML and GitHub
    Enterprise ones are not; the calls never follow a redirect, read at most
    1 MiB, take 10 seconds each and 20 for the whole callback.
  - After the code is exchanged the server asks `userinfoEndpoint` once, with
    `GET`, `Authorization: Bearer <token>`, `Accept: application/json` and
    `User-Agent: PetalBook`; the answer must be `200` and one JSON object. The
    token is then dropped. Nothing else is asked: no second address request, no
    avatar.
  - `fields` are dotted paths into that object: `id`, `data.user.id`,
    `emails.0.email`. A path is 1 to 8 segments of 1 to 64 characters of
    `A-Z a-z 0-9 _ : -`, at most 256 characters in all. A segment of digits
    indexes an array, and only an array (on an object it is a key). `subject`
    is required; the others may be empty. Map `subject` to a stable id that
    users cannot change (not a name or an address); surrounding whitespace is
    trimmed.
  - **The subject** is a JSON string (trimmed, 1 to 255 bytes without control
    characters) or a whole number as written (`4711`, `9007199254740993`); a
    float (`1.5`, `1.0`), an exponent, a bool, an object, an array, `null` or a
    missing value is `sso_error=idp_error` and nobody is signed in. A number and
    a string of the same digits are the same subject. While anyone is linked to
    the provider, changing `fields.subject` or the host of `userinfoEndpoint`
    is `409 subject_fixed`.
  - **The address** is trusted only when the path in `fields.emailVerified`
    resolves to JSON `true` (or the string `"true"`) **and** the provider's
    `mergeVerifiedEmail` is on, as for OpenID Connect (see "Which sign-ins vouch
    for an address"). Without the path, or without the switch, it is kept as
    given, unverified: it never finds, takes or merges into another account.
  - `oauth2` is site-owned and `social` only: a tenant's own provider route,
    `class: "sso"`, `tenants` and a template are `400 invalid`, and a tenant's
    SSO stays OpenID Connect or SAML. `groups` is `400 invalid` for it (the
    answer is not read for groups). It can never claim an account an
    administrator made for an organisation's SSO (it is social).
  - Accounts are made as the registration mode of the tenant the sign-in lands
    in says, like every sign-in; there is no switch of its own. Sign-ins are
    audited as `login` and `login_failed` with `method: "oauth2"`; changes as
    `provider_created` and `provider_changed`, with the fields as
    `oauth2.authorizationEndpoint`, `oauth2.fields.subject`, … and the secret as
    `oauth2.clientSecret: "changed"`.
  - Its callback is `GET /api/auth/oauth2/{slug}/callback` (a redirect from the
    provider, like GitHub's), and it starts at `/api/auth/oauth2/{slug}/start`.

#### Email domains and the SSO entry

A tenant with the feature `features.domains` claims the email domains its
people use; a **verified** claim routes sign-in for those addresses to the
tenant's SSO.

Built in 3.6 d2 (the claim, the check, the recheck, `restrictOrgs`; the
provider routing is d3, `join` d5). The routes need `tenant.domains.manage`
(every tenant administrator holds it) and the feature `features.domains`
(`403 feature_off` while it is off for the tenant; a site administrator is not
bound by it):

- `GET /api/admin/tenants/{tenant}/domains`: `{"domains": [Domain], "siteHost": "notes.example.org"}`,
  by domain;
- `POST …/domains` (`{"domain": "acme.example"}`): `201` with the Domain, its
  `code` and `records`;
- `POST …/domains/{domain}/verify` (check now): `200` with the Domain;
- `POST …/domains/{domain}/code` (a new code, for a claim that is not
  verified; `409 domain_verified` for one that is): `200` with the Domain, its
  `code` and `records`;
- `PATCH …/domains/{domain}` (`{"restrictOrgs": true}`, `{"provider":
  "01JDP4ACME"}`, or both): `200` with the Domain. `provider` (3.6 d3) routes a
  **verified** domain to one of the tenant's own sign-in providers (its id;
  `null` for none): it needs `tenant.signin.manage` and the feature
  `features.ownSso` too (`403 forbidden`, `403 feature_off`), a provider of
  another tenant, a site social provider or a site SSO not offered to the
  tenant is `404 not_found`, and an unverified domain `409 domain_required`.
  From d4 it may also name a site SSO provider offered to the tenant (then
  `features.ownSso` is not needed), and the sign-in page's discovery reads the
  route (below). Deleting the provider clears it. `{"join": "auto"}` (3.6 d5:
  `off`, `auto` or `request`) is how people with an address on the domain join
  the tenant at sign-up; see "Joining an organisation". Turning it `off` while
  requests wait needs `confirm` (`domain_join_off`);
- `DELETE …/domains/{domain}`: `204`. While requests to join by the domain
  wait, it needs `?confirm=` (`domain_join_off`), and cancels them.

`{domain}` takes any spelling of the domain (Unicode or punycode, any case); a
claim another tenant holds is `404 not_found` as one that does not exist.

```json
{"domain": "xn--bcher-kva.example", "display": "bücher.example", "state": "verified",
 "verifiedAt": "2026-11-23T09:00:00Z", "missingSince": null, "lapsedAt": null,
 "checkedAt": "2026-11-23T09:00:00Z", "nextCheckAt": "2026-11-24T09:00:00Z",
 "codeExpires": "2026-11-30T08:00:00Z", "codeExpired": false,
 "lastCheck": {"at": "2026-11-23T09:00:00Z", "by": "manual", "result": "found", "via": "txt", "seen": [], "error": ""},
 "nextCheckAllowedAt": "2026-11-23T09:01:00Z", "restrictOrgs": false, "provider": null, "join": "off",
 "recordName": "_petalbook.xn--bcher-kva.example", "code": null, "records": null,
 "created": "2026-11-23T08:00:00Z"}
```

- `domain` is the key: the IDNA (UTS #46) form in punycode, lower case, without
  the root's dot; `display` is the same as people read it. Two spellings of one
  domain are one claim (`409 domain_exists`). `mixedScript` is true when a label
  mixes scripts beyond what UTS #39's "highly restrictive" level allows (a
  Cyrillic `а` in `аpple.example`): a warning the pages show, not a refusal.
  It misses a lookalike written in one script only (an all-Cyrillic `аре.com`).
- `state` is `unverified`, `verified`, `missing` (verified, and the daily
  recheck has not found the record since `missingSince`), or `lapsed` (the
  recheck cancelled it at `lapsedAt`: as unverified, it needs a valid code and a
  check).
- `code` and `records` are given only by the answer that made the code; the
  code is stored as a hash, so a lost code is replaced with `POST …/code`.
  `records.cname` is `null` when the server has no `--public-url`, or its host
  is an IP address or a name with one label; a Unicode host is given in punycode.
- `nextCheckAllowedAt` is a minute after the last check by hand (`null` when
  the last check was the daily one).

**Verification by DNS.** A claim is made unverified, with a code (26
characters of `a-z2-7`, 128 random bits, a DNS label, stored as a hash after
it is shown). Either record proves it:

- **TXT** (the default the page shows): `_petalbook.<domain>` with a value
  `petalbook-verify=<code>`;
- **CNAME**: `_petalbook.<domain>` pointing to `<code>.verify.<site domain>`,
  where the site domain is the host of `--public-url`. Only the CNAME's target
  is compared; it is never resolved further, so the server needs no DNS for
  `verify.<site domain>` and nothing is served there.

A lookup asks for the TXT of `_petalbook.<domain>` and, when that does not
hold the code, its CNAME, through the system resolver; one check counts as one
outgoing request of the tenant (`rate.outboundPerMinute`, when it has a limit).
The code is valid for 7 days from when it was made; a check of an unverified
claim whose code has expired answers `409 domain_code_expired` before DNS is
asked, and `POST …/code` makes a new one (the old one stops counting). A
verified claim keeps its code's hash: the daily recheck looks for the same
record. An unverified claim does nothing at all.

**Check now** (`POST …/domains/{domain}/verify`, for an unverified claim and
also to re-check a verified one):

- Limits: once a minute per domain, 20 an hour per tenant, and
  `domains.checksPerMinute` on the server (site only, 60 by default), and the
  tenant's outbound share. Over any of them: `429 rate_limited` with
  `Retry-After` in seconds (the page counts down on the button, from
  `nextCheckAllowedAt` or `Retry-After`). A refused check looks nothing up,
  writes nothing and spends nothing: every limit is asked before any is spent.
  The minute of a domain is the domain's, whichever tenant asks.
- `200` with the claim and `lastCheck`: `result` is `found` (the record is
  there with this code: an unverified claim becomes verified; `via` is `txt` or
  `cname`), `missing` (no TXT or CNAME at that name), or `wrong_value` (a
  `petalbook-verify=` TXT or a CNAME is there but none holds this code; `seen`
  lists up to 5 of them, each cut to 255 bytes, so the administrator sees a
  typo or an old code). A DNS failure is `result: "dns_error"` (with `error`)
  and changes nothing. A `found` for a domain another tenant holds verified
  answers `409 domain_claimed` instead (the check is recorded on the claim).
- Every manual check is audited (`domain_checked`, with `domain`, `result`,
  `via`, `seen`, and `claimed: true` for a refused one); a verification also
  writes `domain_verified`. It does not move the daily check; a `found` of a
  verified claim clears `missingSince`.
- A claim is for exactly that domain (subdomains are claimed one by one), and
  so is everything a verified one does: a restriction, an SSO's trust of an
  address, never a subdomain's address. Public mail domains (a list shipped
  with the server, not editable: `gmail.com`, `outlook.com`, `qq.com`, `163.com`,
  …) cannot be claimed (`400 domain_public`), nor can a public suffix of the
  Public Suffix List, ICANN's or a private one (`co.uk`, `com.cn`, `github.io`:
  `400 domain_suffix`); a name below one (`acme.co.uk`) can.
- **Two tenants and one domain**: any number of tenants may hold an unverified
  claim; at most one claim of a domain is verified. The first to verify has it;
  a later `verify` that finds its own record answers `409 domain_claimed` (it
  does not name the other tenant; only a holder of the DNS learns even that).
  A site administrator may move a verified claim
  (`POST /api/admin/domains/{domain}/assign`), and sees every claim
  (`GET /api/admin/domains`).
- **Verified claims are checked again every day**, each a day after its last
  check (so they are spread as their verifications were), by one timer that
  sleeps until the soonest is due and is woken by a change: nothing polls. It
  takes at most 50 due domains a pass, one at a time, and counts each against
  the tenant's outbound share (over it, that domain waits a minute). A tenant
  whose `features.domains` is off is not looked up; its domains wait a day.
  `missingSince` is set at the first recheck that finds no record with the code
  (or a wrong value), audited once as `domain_missing`; a record found again
  clears it. A recheck that still does not find it **7 days** after
  `missingSince` cancels the claim: it becomes `lapsed` (unverified; a new code
  and a check verify it again), routing, joining and `restrictOrgs` stop at
  once, the tenant's administrators get a `domain_lapsed` notification, and
  `domain_lapsed` is audited, in one transaction. A DNS failure is neither found
  nor missing and does not count; that domain is asked again in an hour. While
  the database fails the timer (the due domains cannot be read, or a failed one
  cannot be moved on), it waits 5 seconds, doubled each time, at most 10
  minutes, instead of coming straight back.
- The map of verified domains is in memory (domain → tenant, `restrictOrgs`,
  provider and `join`), read at start and again after every change (a claim
  verified, removed, lapsed, assigned, `restrictOrgs`, its provider or `join`
  changed, a tenant deleted). `auth`'s `OrgDomains` and the organisation sign-up's
  `domain_managed` read it; a tenant whose `features.domains` is off has no
  domains there (nothing is deleted).

**The site's list and transfer** (`site.tenants.manage`):

- `GET /api/admin/domains?verified=true&cursor=&limit=` (1 to 500, 100 by
  default): every tenant's claims, by domain then tenant, each Domain with
  `tenant: {id, slug, name}`; `next` is the cursor of the next page or `null`.
- `POST /api/admin/domains/{domain}/assign` (`{"tenant": id}`): moves a
  verified domain to the tenant, in one transaction: the holder's claim is
  deleted, the target's becomes the verified one, with the holder's code hash
  and recheck state, so the record in DNS keeps counting. `404 not_found` when
  no tenant holds the domain verified, or the tenant does not exist. The holder's
  audit gets `domain_unassigned` and the target's `domain_assigned`, each with
  `domain` only: neither names the other tenant.

**Organisation sign-up on a restricted domain.** `restrictOrgs: true` on a
verified domain makes `POST /api/register` with an `org` and an address of that
domain (exactly that domain) answer `409 domain_managed` with
`organization: {"slug", "name"}` of the tenant that holds it, from the
in-memory map, before anything is written. It says nothing of whether the
address has an account. Joining the tenant is d5.

**The sign-in page asks for the email or username first** (built in 3.6 d4)
when some verified domain is routed to a provider that can sign in now.
`GET /api/auth/providers` says so with two fields:

- `identifierFirst`: a routed domain exists and the stored `signin.mode` is
  `dual` or `sso_first` (a mode of `sso_redirect` does not ask first). The
  stored mode counts, not the effective one: a server whose only single
  sign-on is an organisation's own has no site provider, so its effective mode
  is `local_only`, and its page still asks for the address first.
- `ssoEntry`: a routed domain exists and the stored mode is not `local_only`.
  The page then links to the "Sign in with SSO" entry (`/login/sso`).

Both are `false` when no domain is routed. They are answered from memory, as
the rest of that answer.

#### `POST /api/auth/discover`

Public. `{"email": "tang@acme.example"}`.

```json
{"sso": {"slug": "acme-entra", "kind": "oidc", "name": "Acme", "icon": "preset:microsoft",
         "start": "/api/auth/oidc/acme-entra/start", "enforced": true}}
```

or `{"sso": null}`.

- It looks only at the **domain** of a value that contains `@`: whether a
  verified claim routes it to a provider that can sign in now. It never looks
  up an account, so the answer is the same, byte for byte, whether one exists.
  A username (no `@`) always gets `null`, as do a subdomain of a claimed domain,
  a domain routed nowhere, and a domain whose tenant lost `features.domains`.
  So it tells only what the DNS of that domain tells anyone: that the domain
  has single sign-on here.
- **Routing.** `PATCH …/domains/{domain}` with `provider` routes a verified
  domain to one of the tenant's own providers (d3; needs `features.ownSso`) or,
  from d4, to a **site SSO provider offered to the tenant** (offered to every
  tenant, or to a list that names it; `features.ownSso` is not needed). A site
  social provider, another tenant's, or a site SSO not offered to the tenant is
  `404 not_found`. The route counts only while the provider can sign in (it is
  enabled; a tenant's own also needs `features.ownSso` and a verified domain; a
  site one must still be offered to the tenant): otherwise the answer is
  `null`. `GET /api/admin/tenants/{tenant}/providers` lists, besides
  `providers`, the site's SSO offered to the tenant as `offered`
  (`[{id, slug, kind, name, icon, enabled}]`, read only), for the route's list.
- `enforced` is whether the routed tenant itself requires single sign-on (its
  `signin.ssoOnly`, below). The page then shows only "Continue with <name>",
  focused, and "Use a password instead", which opens the password step:
  administrators keep the password back door, and the server refuses everyone
  else after the password (`403 local_login_refused`). Without `enforced`, the
  password step shows the button above the password field.
- The start carries the address as the hint: the page adds `login_hint` to
  `start` (`GET …/start?login_hint=`; a hint over 254 bytes or with a control
  character is dropped, for both kinds). An OpenID
  Connect provider gets it as `login_hint`. A SAML provider gets it as the
  AuthnRequest's `<Subject><NameID>` (format `emailAddress`), and only when its
  `nameIdFormat` is `emailAddress`: an identity provider that honours a
  `Subject` authenticates that subject alone, so it is sent only where the
  NameID is the address. The address is only a hint: **the account is decided
  by the identity the provider returns**, with the rules above, so routing can
  never sign anyone into an account outside the provider's tenant.
- It counts against the address's per-minute allowance of 2.8
  (`signin.perMinute`), the same allowance as `POST /api/login`: over it,
  `429 rate_limited` with `Retry-After`. Answered from memory: the verified
  domains' map (with each domain's route), the providers in memory and the
  settings' cache; no query.

The **"Sign in with SSO"** entry (`/login/sso`) asks only for a work address
and sends the browser to `start` with the hint; an address that routes nowhere
is told so ("no single sign-on is set up for this address").

**Usernames still sign in with a password** as before, in every mode.

**With the 2.8 login modes**: `signin.mode` stays the site's (d4 keeps it so;
a tenant layer for it is later work). `local_only` turns off the buttons and the
SSO entry, not the password back door. `GET /api/auth/providers?tenant=` lists
the site's social providers the tenant allows, the site SSO providers offered
to it, and the tenant's own SSO providers; a tenant that requires its single
sign-on is listed without the social ones.

**An organisation's `signin.ssoOnly`** (3.6 d4). The key has a tenant layer
that only tightens (the lower mode `tighten` of "Settings by layer", for this
key): a tenant may turn it on (`PUT /api/admin/tenants/{tenant}/settings/signin.ssoOnly`
with `true`, `tenant.settings.write`), never off under a site that has it on.
Writing `false` while the value above it (the site's, or the site's rule for
the tenant) is `true` is `409 setting_restricted`; a `false` stored before the
site turned it on is kept, `inEffect: false`. The site may also set it for one
tenant with a rule, as for any key.

- **Turning it on** needs a single sign-on the tenant's people can use now (one
  of its own, or a site SSO offered to it): `400 sso_provider_required`
  otherwise; and an enabled administrator of the tenant with a password:
  `400 local_admin_required` otherwise. The same checks hold for the site's
  rule.
- **In effect** while it is on (in the tenant's own layer, or by the site's
  rule for the tenant) and the tenant has such a single sign-on; it is read
  from the cached settings by the account's tenant. It then refuses to the
  tenant's people, as the site's `ssoOnly` does: a password and a passkey
  (`403 local_login_refused`), accepting a member's invite with a password
  (`403 local_login_refused`), a password reset link, and **the site's social
  sign-in** (`?sso_error=sso_required`; the social buttons are left off the
  tenant's page). It is met by the tenant's own SSO and by a site SSO offered to
  the tenant.
- **The back door, and `signin.ssoOnlyAdmins`.** By default (`false`) the
  tenant's administrators and the site's administrators are not refused by it:
  they keep the password. The tenant-layer key `signin.ssoOnlyAdmins` (boolean,
  default `false`, site and tenant layers, tightens only, as `ssoOnly`; set with
  `PUT /api/admin/tenants/{tenant}/settings/signin.ssoOnlyAdmins`) decides
  whether the tenant's administrators are held to the single sign-on like its
  members. It matters only while `ssoOnly` is in effect for the tenant. On: a
  tenant administrator's password and passkey sign-in, a password invite for an
  administrator, and a reset link are refused (`403 local_login_refused`, as for
  a member), and the site's social sign-in is refused to them as to members. A
  site administrator always keeps the password, whatever a tenant sets (owner's
  decision, 2026-11-27).
- Another tenant's people are not touched: the setting is per tenant.
- **Registration.** Registering with a password answers `409 sso_required`
  with `sso` (the answer of `discover` for the address, or absent when the
  address routes nowhere): for an address of a domain whose tenant requires
  its single sign-on (whichever tenant the registration is for, or an
  organisation of one's own), and for any address in a tenant that requires
  it. The link of a registration made before the tenant turned it on answers
  the same when followed. It is about the domain only, never the account.
- **The last administrator with a password stays.** While the tenant's
  `signin.ssoOnly` is on (in its own layer or by the site's rule, whether or not
  a single sign-on meets it now), demoting, disabling or deleting its last
  enabled administrator who has a password answers `409 last_local_admin`: the
  site's 2.8 rule, per tenant. It does not apply while the tenant's
  `signin.ssoOnlyAdmins` is on (no administrator needs a password then), and
  turning `ssoOnly` on then does not need an administrator with a password
  (`local_admin_required`).
- **The sign-in calls of a tenant's provider** (discovery, keys, tokens, user
  information) do **not** spend the tenant's outbound share
  (`rate.outboundPerMinute`; decided in the d4 review): anyone may start
  sign-ins and send callbacks, so charging them would let a stranger drain the
  share, stop the tenant's domain checks and, with `ssoOnly` on, lock its people
  out. The per-address sign-in limits (starts, callbacks) bound them; d3's
  public-address check on every connection stays.

#### Mail: `GET/PUT /api/admin/tenants/{tenant}/mail`, `POST …/mail/test`

The shape of `/api/admin/mail` (3.3), without `security: "none"`, under the
outgoing rules above. A tenant's mail (verification, reset, registration,
notices) is sent with the tenant's SMTP when it has one enabled; otherwise with
the site's if `mail.useSite` is in effect (the site's `from` address, the
tenant's name as the display name), otherwise not at all (the tenant's
registration and reset then answer as with mail off). The 3.3 caps stay
site-wide (1,000 waiting on the server, 3 an hour to one address); each tenant
has at most 200 waiting.

#### Captcha: `GET/PUT /api/admin/tenants/{tenant}/captcha`

`provider`, `siteKey`, `secret` (write-only) as 2.8, and the tenant's values of
the contexts `register`, `forgot`, `invite` (within the site's `tighten`). Used
on the tenant's registration, reset and invite pages; the verification request
goes to the provider's fixed host and counts against the tenant's outbound
share.

#### Daily text: `/api/admin/tenants/{tenant}/daily…`

`GET/PUT …/daily`, `PUT/DELETE …/daily/apis/{id}`, `POST …/daily/try`,
`PUT/DELETE …/daily/lines`: the shapes of 2.3's site endpoints, for the
tenant. The tenant's interfaces have their own sealed header values, their own
`rps` and their own cache (keyed by tenant and interface), so they never use the
site's rate limits; they count against the outbound cap. A tenant's spaces may
choose the tenant's interfaces, and the site's while `daily.useSiteApis` is in
effect. The tenant's `daily.default` (3.3) may name its own interfaces and
lines.

#### Connections: `/api/admin/tenants/{tenant}/connections/{provider}…`

The tenant's own providers (2.7b). For GitHub:

- A tenant may register its **own GitHub App** (`GET/PUT/DELETE
  …/connections/github`, the shape of 2.7b's `/api/admin/providers/github`),
  only on `github.com`. Its installations (`GET …/connections/github/installations`,
  `PUT …/installations/{id}` with `shared`) are shared within the tenant only.
  Needs the feature `features.ownConnections`. From 3.8 a tenant may also
  add its own GitLab and Gitea providers here (see "Connections to GitLab and
  Gitea").
- **The site's App** stays the site's. Its installations are not shared by the
  site any more (2026-11-26): each person and each organisation installs it on
  their own GitHub account ("The GitHub provider", "Installations"); 3.6 adds
  `{"tenants": "all"}` (every tenant, those made later included).
  `GET /api/admin/tenants/{tenant}/connections/github` also lists, read only, the site
  installations shared with the tenant.
- A person connects their GitHub account through their tenant's App when it has
  one, else through the site's while `connections.useSiteApps` is in effect. One
  GitHub account still links to one PetalBook user on the server (2.7). The
  callback address stays one (`/api/account/links/github/callback`);
  the state names the App.
- `internal/githubapp` takes an App as a value (id, key, client), never a
  global; the token cache (10,000) is the server's and is keyed by (App,
  installation, repository).

#### Security: `GET/PUT /api/admin/tenants/{tenant}/security`

The tenant's layer of `signin.ssoOnly`, `auth.loginMode`,
`auth.passkeys.requireRoles`, `auth.social`, `session.*` and the captcha
contexts above, gathered as one page with 2.8's checks scoped to the tenant:
`local_admin_required` counts the tenant's enabled administrators with a
password; `ssoOnly` needs an enabled SSO provider of the tenant or offered to
it. Tenant administrators are not refused a password by `ssoOnly` unless the
tenant's `signin.ssoOnlyAdmins` is on (2.8's back door, kept per tenant by
default). A site administrator is never refused by a tenant's setting.

### Storage modes

Option A (owner, 2026-11-23). A space is stored in one of three modes:

| Mode | The space |
|---|---|
| `server` | is a repository on the server's disk only |
| `own` | follows a remote that a person connects for it (`storage.sync` through a provider, 2.7b; or `remote`, a plain address with a key or token, 3.1) |
| `tenant` | follows its own repository, which the server makes for it at the tenant's storage target |

- **The site decides which modes exist**: `storage.modes.available` (site only;
  default all three).
- **The site decides whether one is forced or only a default**: the layered key
  `storage.mode` (site, tenant, space). `lock` forces the site's value on every
  space; `restrict` with `options` (by default: the available modes) makes the
  site's value a default.
- **A tenant picks only among the modes its rules allow**: what is left
  for the tenant is the site's options that are also in the restriction a rule
  for it puts on `storage.mode`. The tenant sets its default
  (`PUT /api/admin/tenants/{tenant}/settings/storage.mode`) and may lock it or
  narrow it for its spaces with its own `lower`. When a space is made, its
  creator chooses among what is left (`POST /api/spaces` takes
  `"storage": {"mode": "tenant"}`, default the effective value): a mode the
  tenant's rules leave out answers `409 setting_restricted`, one a layer's
  rule leaves out `409 setting_restricted`. What the request brings decides what
  is meant when it names no mode: with `storage.sync` or `remote` it is `own`,
  else the effective value. `server` with a remote is `400 invalid`. Nothing is
  made for a refused request.
- **When a space is made** (the rule is read for the creator's tenant; the
  personal space, which is made at sign-in, is always made local and is not
  asked, so a person whose organisation allows only `own` still has one, and
  its deadline is the requirement's):
  - `server`: a space on the server's disk, no remote.
  - `own`: with `storage.sync` or `remote`, the space follows it from the start;
    without, it is made local and its owner connects a remote afterwards
    (unless `server` is not allowed, below).
  - `tenant`: the tenant's storage target must exist and not be `broken`
    (`409 no_storage_target` otherwise). Until the server makes spaces at a
    target (3.6, later), a tenant that has one answers `501 not_implemented`.
- In `GET /api/settings` a mode that is allowed but cannot be used now is in
  `missing`, so a client leaves it out. The effective value of a tenant is
  cached by the settings; `missing` is read when the list is asked for.
- The allowed modes are, for now, the `options` of the `restrict` rule (a lock
  leaves its value alone); the separate site-only `storage.modes.available` is
  not built, so every mode is available until a rule leaves one out. The mode
  is not stored on the space yet (`spaces.storage_mode` comes with the moves);
  the rule decides where a new space may be made, and `location` and the
  `storage.remote` notice keep describing what exists.
- The mode is stored on the space (`spaces.storage_mode`, with the target for
  `tenant`) and shown in its `storage` block (3.1); the tenant's `usage` row
  keeps the count per mode.

**With 3.1.**

- **The remote requirement.** 3.1's `storage.remotes.require` is no longer set
  on its own: it is derived per space, `null` when `server` is among the space's
  allowed modes, otherwise `any` (`github` when the effective `storage.hosts` is
  only `github.com` and GitHub is set up). 3.1's checks read the derived value.
  Today it is worked out at two points: **for the site** (the modes the site's
  own `storage.mode` rule allows) it is the value 3.1's queries read, with the
  grace period, `scope` and overdue read-only behaviour of 3.1 (below); an
  explicit `storage.remotes.require`, when set, wins; **for an organisation** (the modes its own rules also leave out)
  it is checked when a space is made (`409 remote_required`, exempt people
  excepted). A tenant that drops `server` itself does not put its existing
  spaces under a deadline: the deadline queries are the site's. Such an
  organisation does not let a space go back to the server either: `DELETE
  …/remote` of one of its spaces is `409 remote_required` (the exemptions of 3.1
  excepted), besides the site's own check.
  The grace period counts from `since`, which is when the **requirement in force**
  (what is required, `scope` and `storage.hosts`, whether derived or set) last
  changed, kept with that requirement and settled after a write of
  `storage.mode`, `storage.remotes.require`, `.scope` or `storage.hosts`, and when
  the server starts. It is never the time a rule was written: a rule older than
  the grace period does not make spaces overdue at once, whether it is a later
  widening of `scope`, clearing a set requirement, or a server started with the
  rule in place. A requirement that begins, changes what is required, or widens
  `scope` to `all` starts the period afresh; ending the requirement clears it; a
  change of `storage.hosts` that leaves what is required alone, and narrowing
  `scope` to `new`, keep `since`. A change whose settling failed is settled by
  the next rebuild of the policy. A personal space is not asked about the
  organisation's modes when its remote is removed (the site's check stays).
  `storage.remotes.scope`, `.graceDays`, the exemptions and
  `storage.localSpaces.maxBytes` keep their meaning and concern `server` spaces.
  A `tenant` space follows a remote and so meets the requirement.
- **The attachment repository.** With `storage.assets: repository` (the
  default; see "Where attachments go"), a `tenant`
  space gets a second repository at the same target (`<name>-assets`), made with
  the first; an `own` space as 3.1; a `server` space keeps its attachments in
  the space.
- **Partial caching and eviction.** A `tenant` space is a space with a remote:
  3.1's partial clone, fetching on demand, eviction, restore and pinning (within
  `storage.pinnedBytes`) apply as to an `own` space. A `server` space is never
  evicted.

#### The tenant's storage target

`GET`, `PUT`, `DELETE /api/admin/tenants/{tenant}/storage/target`: the tenant's
administrators (needs `tenant` in the tenant's `storage.modes`, `403
feature_off` otherwise) and site administrators (who may add `"siteLock":
true`).

```json
{"kind": "github", "app": "tenant", "installation": 51234567, "owner": "acme-notes",
 "prefix": "", "state": "ok", "checkedAt": "2026-11-23T09:00:00Z", "spaces": 69,
 "updatedBy": {"role": "tenant", "user": {"id": "…", "username": "wu"}, "at": "…"}}
```

```json
{"kind": "gitlab", "host": "gitlab.acme.example", "namespace": "notes/petalbook",
 "hasSecret": true, "prefix": "pb-", "state": "ok", "spaces": 12}
```

- A target is a place at a provider (2.7b): `kind` is the provider's type and,
  for GitLab and Gitea, `provider` names the provider (3.8; until 3.8 is merged
  only `github` targets exist).
- `kind` `github` (github.com only): `app` is `tenant` (the tenant's own App,
  which needs `integrations.connections`) or `site` (an installation of the site's
  App shared with the tenant); `installation` is on the organisation `owner`
  and needs 2.7's permission to make repositories. `kind` `gitlab` or `gitea`
  (Gitea and Forgejo): `host`, `namespace` (a group, subgroup or organisation)
  and a `token` (write-only, sealed with the tenant's id in the purpose) that
  may make repositories there. A Git host without such an interface cannot be a
  target (its spaces use `own`).
- The host must be in the tenant's effective `storage.hosts` and pass the
  public-address check when connecting. `PUT` checks the target once (the
  organisation or namespace exists and the credential may make repositories
  there): `400 invalid` with `reason` otherwise. The check counts against the
  tenant's outbound share.
- **One repository per space**, never one repository for the tenant. A space
  made in `tenant` mode first gets a private repository at the target (2.7's
  path: `POST /orgs/{owner}/repos` with the installation's token; GitLab's
  `POST /projects` with the namespace; Gitea's `POST /orgs/{org}/repos`), named
  `prefix` + the space's slug (a name taken adds the last 6 characters of the
  space's id), then its attachment repository when needed, and is then made,
  connected with `via: "tenant"`. When the repository cannot be made, no space
  is made: `502 storage_target_error` with what the host answered (`reason`,
  `status`). Those API calls count against the tenant's outbound share; fetch
  and push follow 3.1 as for any remote and do not.
- **Credentials.** A `tenant` space uses the target's credential, not a
  person's: deleting a person (3.5) never leaves it without one, and a new token
  applies to every space at once (they refer to the one sealed record).
- **Changing the target** (another owner, host or namespace) makes a new target
  for new spaces; the old one is kept, read only on the page, for the spaces on
  it until they move. `DELETE` of a target that spaces use answers
  `409 conflict` with the count. A tenant has one target in use, plus old ones
  that still have spaces.
- Deleting a space (3.5) never deletes its repositories at the target.

#### When the mode changes

When the tenant changes its default, the site or a rule for the tenant
removes a mode, or the tenant changes its target:

- New spaces use what is now in effect.
- **Existing spaces stay where they are and keep working** (a `tenant` space
  keeps syncing with its repository, an `own` space with its remote); nothing is
  deleted. Their owners and the tenant's administrators are offered a move.
- Only when `server` stops being allowed for a space does 3.1's rule apply: with
  `scope: all`, the grace period, then read only (never deleted) until the space
  follows a remote.
- `GET /api/admin/tenants/{tenant}/storage/moves?mode=&cursor=&limit=`: the
  spaces whose mode or target is not the effective one; one query over the index
  `(tenant_id, storage_mode, storage_target, id)`.
- `POST /api/spaces/{space}/storage/move` with `{"mode": "tenant" | "server" | "own"}`
  (the space's owners, the tenant's administrators, site administrators). `202`
  with a job (`GET /api/admin/jobs/{id}`, 3.5):
  - to `tenant`: the whole history is fetched (an evicted or partial space is
    restored first, 3.1), the repositories are made at the target, every branch
    and the attachment repository are pushed, then the space's remote is
    switched in one transaction;
  - to `server`: the whole history and the attachments are fetched, then the
    remote is disconnected; it needs room within `storage.localSpaces.maxBytes`
    and the quotas (`409 quota_exceeded`, `507 space_full`);
  - to `own`: answers 3.1's connect flow for the owner; nothing moves until they
    connect.
  - The old remote repositories are never touched. One move per space, one per
    tenant at a time and two on the server; a move survives nothing (after a
    restart it reads `interrupted` and the space is still on its old remote).
    Several spaces at once: 3.5's bulk with `action: "move_storage"` and a
    confirmation.

### Signing in by email address

From 3.6 the email address is the main way in; the username stays.

- **The username** is still the unique handle and id people see: `@mentions`,
  commit authors, links, member lists. Nothing about it changes.
- **The email address is unique on the server**, among all accounts (enabled,
  disabled, and deleted ones in the recycle bin, until they are purged),
  verified or not. Every address of every account is a row of `user_emails`
  (account, address, key, primary, verified, who vouched, created) with a
  unique index on the key, so finding an account by address is one index
  lookup. There are no exceptions: two accounts never share an address. An
  account has one address, its primary one (the UPN), copied to
  `users.email` so that lists and search need no join, or several while its
  tenant's `accounts.multipleEmails` is on (see "More than one email
  address"); **signing in or resetting by address reads a verified row of
  `user_emails`**, never `users.email`.
- **How addresses are compared** (the key): the domain in its ASCII form
  (lower case; a domain outside ASCII, or one with an `xn--` label, through
  IDNA (UTS #46) to punycode, so `例子.公司` and `xn--fsqu00a.xn--55qx5d` are
  one domain); the local part with `A` to `Z` lower case when it is ASCII,
  otherwise NFC, case folded (Unicode full folding) and NFC again. The root's
  dot at the end of a domain is left out of the key (`bob@example.com.` is
  `bob@example.com`), and an address that ends in one is refused where it is
  set (`400 invalid`); a domain with an empty label, as typed or once its
  punycode is decoded (`a@xn--.com`, `a@example..com`), has no key and is
  refused.
- **Addresses outside ASCII** (a Unicode local part, an internationalised
  domain) are refused with `400 email_not_ascii` while the site setting
  `users.emailNonAscii` is off (the default), wherever an address is set:
  registration, `PATCH /api/account`, `PATCH /api/users/{id}`, a new tenant's
  first administrator. Turned on, they are accepted and compared by the key
  above, so they stay unique; mail to them needs a relay that supports
  SMTPUTF8 (RFC 6531), and without one their messages fail at the relay.
  Turning it off again keeps the addresses already stored (and their
  uniqueness) but sends them no account mail, and a lookup by a typed address
  outside ASCII finds nobody. An address an identity provider sends outside
  ASCII while it is off is left out of the new account.
- **`POST /api/login`** takes `{"identifier": "tang@acme.example", "password": "…"}`;
  `identifier` is an email address (it contains `@`) or a username; it
  replaces 2.8's `username` field. The checks and their
  order are those of "Signing in": the per-username failure counts and the
  lockout key on the account when one is found, else on the identifier as
  typed in lower case, so an address and a username of the same account share
  one count. A wrong address answers like a wrong username (a dummy hash, the
  same `401`). An address finds the account that holds it verified.
- **The sign-in page** asks for "email or username" (email first in the label
  and the placeholder), then `POST /api/auth/discover` decides whether it goes
  to SSO (above), then it asks for the password or offers a passkey.
- **Setting an address** that another account holds, verified or not, through
  `PATCH /api/account`, an administrator's `PATCH /api/users/{id}`, or any
  invite whose prefilled address is taken (`POST /api/invites` with an
  `email`, and a new tenant's first administrator, before anything is made):
  `409 email_taken` (signed in or administrator only, so this tells a
  signed-in person no more than the invite and member pages do). Setting one's
  own address in another case is not a change of holder. Registration keeps
  3.3's rule of answering the same `202` either way; what was "an enabled
  account of the tenant with this verified address" is now any account on the
  server that holds it verified (or is deleted and keeps it): the registration
  is then a decoy whose link is never sent, and the holder, when it may get
  account mail, is told at its stored address.
- **`POST /api/password/forgot`** with an address: the account that holds it
  verified, on the whole server (as a username already was), when that address
  may get account mail (verified by the person, a trusted sign-in or a site
  administrator; not by a tenant administrator alone). The link goes to that
  address as stored. An address held unverified finds nobody.
- **An address held only unverified by another account** goes to whoever
  proves it: confirming a registration, or a trusted sign-in (below), for an
  address another live account holds without `emailVerified` moves the
  address: the other account's address is cleared (and its pending links
  dropped), it gets an `email_released` notification
  `{"email": "…"}`, and `email_released` is audited
  (`email`, `to`). It moves only when the proved address is the same mailbox
  (equal without the Unicode case folding: NFC, `A` to `Z` lower case); one that
  is equal only once folded is refused like a verified one. A verified holder,
  and a deleted account until its purge, are never displaced. Verified here
  means by the person, a trusted sign-in, or a site administrator's "mark
  verified" (a site administrator vouches for addresses on the whole server).
  **A tenant administrator's mark alone** (`verified_by` `tenant_admin`) shows
  as `emailVerified` but holds the address no more than an unverified one: it
  moves to whoever proves it, a registration for it is no decoy, and a sign-in
  that vouches for it does not end in `email_in_use`. Otherwise a tenant
  administrator could give a member of theirs anyone's address, verified, and
  keep its owner out of it on the whole server. Setting an
  address (`PATCH`) never moves it: who has not proved it gets
  `409 email_taken`.
- **A sign-in is never attached to an account by its address.** 2.8's merge by
  verified address is gone. An identity nobody has, whose address another
  account holds verified, ends in `sso_error=email_in_use` when the sign-in
  vouches for the address (the person signs in to that account and links the
  provider from their account settings); when it does not vouch for it, a new
  account (made by the registration mode) is made without the address. One held unverified moves
  to the new account when the sign-in vouches for it (above), and is left out
  otherwise.
- **Which sign-ins vouch for an address** (one rule in the server,
  `trustsEmail`). An address a sign-in vouches for is verified at once, with
  `verified_by` `provider:<slug>`; any other is unverified.
  - The built-in social providers (3.6's templates): Google and Apple only when
    the token says `email_verified: true`; GitHub only an address its API lists
    as verified (today's `github` kind), and only github.com itself: a GitHub
    Enterprise server (another `baseUrl`), whose administrators can mark any
    address verified, only with the provider's `mergeVerifiedEmail` switch on;
    Microsoft only with the `xms_edov` claim (the tenant that issued the token
    owns the address's domain). Microsoft's plain `email` claim is never
    trusted, whatever its domain, a domain verified here included: any Entra
    tenant can put any address in it ("nOAuth"). **Recorded decision
    (2026-11-27):** this is the safe rule; the owner may loosen it later (for
    example for personal Microsoft accounts).
  - Apple's private-relay addresses (`@privaterelay.appleid.com`, or
    `is_private_email`) are verified, but never meet another account's: a
    relay address another account holds (verified or not) is neither
    `email_in_use` nor taken from it; the new account is made without it.
    They route to no organisation's domain.
  - An organisation's SSO (OpenID Connect or SAML): only addresses on that
    organisation's verified domains, each matched exactly (a subdomain is
    claimed by itself). Until domain verification exists it vouches for
    nothing; a SAML provider, which has no verified-address claim, is treated
    so.
  - A provider the administrators configured (OpenID Connect, and the
    GitLab.com template): only with its `mergeVerifiedEmail` switch on (shown
    as "trust this provider's verified email addresses", off by default) and
    the `email_verified` claim.
  - A custom OAuth 2.0 provider (`kind: "oauth2"`): as an OpenID Connect one,
    with the value at the path `fields.emailVerified` (JSON `true` or `"true"`)
    in place of the claim; with no such path its addresses are never trusted.

### Joining an organisation

Each tenant chooses how people join it; any combination of these may be on:

| Way | Setting | Default |
|---|---|---|
| (a) invite | always available (`/api/invites`, as 3.3) | — |
| (b) an address of a verified domain joins at once | the domain's `join: "auto"` | off |
| (c) an address of a verified domain asks, an administrator approves | the domain's `join: "request"` | off |
| (d) signing in through the tenant's own SSO joins at once | the tenant's `registration.mode` `open`, with `jit.requireGroups` | `invite` |

(b) and (c) are built in 3.6 d5, **at sign-up only**: 3.6 is one person, one
tenant, so an existing account never joins another organisation (that is 3.7's
membership, which keeps these endpoints). A social provider's sign-in
does not join by domain yet; nor is there a tenant-wide default, a `viewer`
role of accounts, or `defaultRole` (the joined person is a `member`).

**The mode is per verified domain** (`PATCH /api/admin/tenants/{tenant}/domains/{domain}`
with `{"join": "off" | "auto" | "request"}`, `tenant.domains.manage`,
`features.domains`; see "Email domains and the SSO entry"):

- only a **verified** domain takes `auto` or `request` (`409 domain_required`);
  `off` is the default, and any other word `400 invalid`;
- the address must be on the domain **exactly**, as the address's key (the
  domain in IDNA punycode, lower case): `a@eng.acme.example` does not join by
  `acme.example`;
- the domain's tenant must have `features.domains`; off, the domain joins
  nobody (and its requests wait, without effect);
- the mode is read from the in-memory map of verified domains, so a sign-up
  asks no query of it;
- turning it `off` while requests wait, and removing the domain, cancel them:
  without `confirm` the answer is `409 confirm_required` with the plan
  `[{"tenant", "domain", "requests": 2}]`; `POST /api/admin/confirm`
  `{"action": "domain_join_off", "tenant", "domain"}` gives the token, sent as
  `{"join": "off", "confirm": "c_…"}` or `DELETE …/domains/{domain}?confirm=c_…`.
  `auto` from `request` keeps them waiting. The domain lapsing, or a site
  administrator moving it to another tenant, cancels them without asking, and
  turns its join `off` (the new holder turns it on itself).
  Each cancelled request is audited (`join_cancelled`, with `reason`:
  `join_off`, `removed`, `lapsed` or `moved`).
- Turning `features.domains` off for a tenant counts its waiting requests in
  the confirmation's plan (`requests`, beside `domains` and `verified`).

**Signing up.** `POST /api/register` with an address whose domain joins a
tenant registers in **that** tenant, whatever tenant the page named, and even
while that tenant's (or the site's) registration is closed: mail must be on and
the tenant enabled. The tenant's `registration.domains` is not asked (the
domain is its own); its terms, its hour (`registration.perHour`) and its 1,000
pending registrations are. An organisation that requires its single sign-on
(`signin.ssoOnly`) answers `409 sso_required` with the `sso` entry, as for any
password sign-up there. The message names the organisation. `GET /api/register`
says `"domainJoin": true` when some domain joins people now, so the page offers
the form while `open` is false; `/api/auth/providers`' `register` is then true.

Following the link (`POST /api/register/confirm`) asks the domain's mode again:

- **(b) `auto`**: the account is made in the tenant with the role of the
  tenant's `domains.joinRole` (default `member`, see below), with the address verified, and signed in, as any registration: `201` with
  `"joined": {"slug": "acme", "name": "Acme"}`, so the page says which
  organisation it joined. Audited as `registered` and `join_auto`.
- **(c) `request`**: no account: the registration becomes a **join request**,
  `202 {"request": JoinStatus}`, and nobody is signed in. The request holds the
  username (registration answers `409 exists` for it) for **30 days**; then it
  expires and the username is free. One address has one waiting request per
  tenant (`409 join_pending`), and makes at most **3 requests a day**, in all
  tenants, counted without a `+tag` of the address (`bob+x@` is `bob@`;
  `429 rate_limited` with `Retry-After`). A tenant holds at most 1,000 waiting
  requests (`429 join_limit` for the next one). The tenant's enabled
  administrators get a `join_requested` notification; `join_requested` is
  audited.
- the mode `off` now: as a registration of the tenant, with all of its rules:
  `403 registration_closed` unless the tenant's registration is open, and
  `400 email_domain_not_allowed` when the address is not on its
  `registration.domains`.

```json
{"state": "pending", "tenant": {"slug": "acme", "name": "Acme"}, "username": "bob",
 "email": "bob@acme.example", "created": "2026-11-27T09:00:00Z",
 "expires": "2026-12-27T09:00:00Z", "decidedAt": null}
```

`state` is `pending`, `approved`, `denied`, `withdrawn`, `expired` or
`cancelled`.

**The role of a joiner: `domains.joinRole`** (site and tenant layers, a choice,
default `member`; `tenant.settings.write`). The role a person gets who joins by
domain, whether by `auto` (a password sign-up or a social sign-in) or when an
administrator approves the request without naming a role. The choices are the
account roles that exist and are not the administrator's (today only `member`;
a viewer role, when accounts have one, is added to the choices): `400 invalid`
for any other value, `admin` included. It is read when the account is made, so
a change touches no account that exists and no request already approved.

**Social sign-in follows the join mode too.** A new account made by a **site
social provider's sign-in** with an address the provider vouches for (the
rule "Which sign-ins vouch for an address", the one `trustsEmail` applies) on a domain that joins a tenant is made as the domain's mode
says, instead of in the provider's own tenant (an address it does not vouch
for, a subdomain, a domain with mode `off` or a tenant without
`features.domains` is as before):

- `auto`: the account is made in the tenant (role `domains.joinRole`) linked to
  the identity, and signed in; audited as `join_auto`.
- `request`: no account and no session: the identity is kept in a join request
  (as a password sign-up's request, without a password), and the callback
  redirects to `/register/waiting#<token>`, the same waiting page; the token is
  the request's capability. The username comes from the claims, with a number
  added when taken. Approving makes the account **linked to that identity**
  (no password; the person signs in through the provider), with
  `409 provider_unavailable` if the provider is gone or the tenant no longer
  allows it (`signin.social`, or `ssoOnly`). Asking again while a request waits
  redirects to `/login?sso_error=join_pending`; over the day's 3 requests or
  the tenant's 1,000 waiting ones, to `?sso_error=join_limit`.
- A disabled tenant takes nobody (`?sso_error=no_account`); a tenant that
  refuses the provider (`signin.social`, its `ssoOnly`) answers
  `?sso_error=not_allowed`. Mail need not be on: the address is verified by the
  provider, and only the approval's message is sent when it is.

**The waiting page** (`/register/waiting#<token>`, the registration link's
token, which stays the request's capability):

- `POST /api/register/request` `{"token"}`: `200` with the JoinStatus;
  `404 not_found` for a token that is not one. Held to the address's minute and
  failures, as the other links.
- `POST /api/register/request/withdraw` `{"token"}`: withdraws a waiting
  request: `200` with the JoinStatus, the username is free again;
  `409 join_decided` when it is not waiting. Audited as `join_withdrawn`.

**The tenant's administrators** (`tenant.users.manage`, and `features.domains` of the tenant in the path, for
a site administrator outside its scope too; `404 not_found` for another tenant,
as for one that does not exist):

- `GET /api/admin/tenants/{tenant}/join-requests?state=pending|all&cursor=&limit=`
  (1 to 200, 50 by default): `{"items": [JoinRequest], "next": cursor | null}`,
  newest first; `pending` (the default) lists those still waiting.
- `POST …/join-requests/{id}/approve` (`{"role": "member"}`, optional: one of
  the roles `domains.joinRole` offers, the tenant's `domains.joinRole` when
  absent; `400 invalid` for another, `admin` included): makes the account in
  the tenant with the address verified and the password chosen at sign-up (or,
  for a request made with a social sign-in, linked to that identity), and a personal space, in one transaction
  with the approval; the person is mailed a link to sign in, and the new account
  gets a `join_approved` notification. `200` with the JoinRequest (`user` is the
  account). `409 join_decided` when it is not waiting (decided, withdrawn,
  cancelled or expired); `409 tenant_disabled` while the tenant is disabled;
  `409 sso_required` while the tenant requires its
  single sign-on (no password account is made: deny it, and the person signs in
  through the SSO); `409 exists` or `409 email_taken` when the username or the
  address was taken meanwhile. For a request made with a social sign-in:
  `409 conflict` ("That account is linked to another person.") when its identity
  has meanwhile been linked to another account (the request stays waiting; deny
  it), and `409 provider_unavailable` when the provider is gone or the tenant
  no longer allows it. The account's username takes a number when the one asked
  for was taken meanwhile, and the mail names the account made, in the language
  of the sign-in's `Accept-Language`. Audited as `join_approved`.
- `POST …/join-requests/{id}/deny`: the username is free again, and the person
  is mailed that it was not approved. `200` with the JoinRequest. Audited as
  `join_denied`.

```json
{"id": "01JDQ…", "username": "bob", "name": "Bob", "email": "bob@acme.example",
 "domain": "acme.example", "state": "approved", "created": "2026-11-27T09:00:00Z",
 "expires": "2026-12-27T09:00:00Z", "decidedAt": "2026-11-27T10:00:00Z",
 "decidedBy": {"id": "01JDP…", "name": "Ann"}, "user": "01JDQ…"}
```

A request that is no longer waiting keeps no password hash, and is deleted 30
days after it ended (the daily sweep, which also writes the expired ones
`expired`).

Known item (left as it is in 3.6 d5): the hold of the username ends at the
moment the request expires, while its row still reads `pending` (with its
password hash) until the daily sweep writes it `expired`; in that window the
username can be registered again, and the request can no longer be approved
(`409 join_decided`).

**(d) Joining through the tenant's SSO.** A tenant's SSO provider makes the
account in its tenant at first sign-in when the tenant's `registration.mode` is
`open` (the mode replaced a per-provider switch), with `defaultRole`. The Provider gains:

```json
"jit": {"requireGroups": ["5f1c…-petalbook-users"]}
```

- `requireGroups` (empty by default: everyone the provider sends): values of
  the provider's groups claim (the `groups.claim` of 3.2); a person whose claim
  has none of them is refused **at every sign-in through the provider**, not
  only the first (`sso_error=not_allowed`, audited as `login_failed` with
  `reason: "group_required"`). A missing claim is "none of them".
- Groups and roles from the IdP reuse 3.2's `groups` setting:
  - `groups.map` maps values to the tenant's synced groups, as 3.2.
  - `groups.admins` (a list of values) makes a person the tenant's `admin` while
    their claim has one of the values. **Off unless the site allows it** for the
    tenant: the feature `features.idpAdmins` (`false` in the site's default);
    while it is off, `PUT`/`PATCH` with a non-empty `admins` answers
    `403 feature_off`, and turning it off stops promotions at once (people promoted before keep the role
    until their next sign-in, which demotes them). Every promotion and demotion
    through it is audited (`role_from_idp`). It only demotes people it promoted
    (`roleSource: "idp"` on the User, on the membership from 3.7); an
    administrator made by hand is never touched. It never grants `siteAdmin`, and never demotes the last
    enabled administrator of the tenant with a password (`local_admin_required`
    is kept; the row says `skipped`).
- Leaving a group at the IdP takes effect at the person's next sign-in through
  the provider (3.2's rule): their sessions and tokens keep what they had until
  then. SCIM, which would make it immediate, stays in phase 5 (5.10).

### Organisation sign-up

With `tenant.selfService` on at the site (off by default; it needs
`tenancy.enabled` and mail), registration offers a second choice: **create my
own organisation**.

- `GET /api/register` answers `createTenant: true`. `POST /api/register` takes
  `{"organisation": {"name": "Acme", "slug": "acme"}}` beside the 3.3 fields;
  the slug is held while the registration waits for its confirmation (as the
  username is).
- Confirming the address makes the tenant, then the account in it with
  `role: "admin"` (not `siteAdmin`), with a personal space. The new tenant
  starts with the site's defaults, the plan `tenant.selfService.plan` (site
  only; `null` means no plan: the site's defaults),
  and registration closed.
- **Caps** (site only, changeable by site administrators):
  `tenant.selfService.perPerson` tenants per person (default 3; in 3.6 counted
  by the confirmed address, the tenants it made that still exist; from 3.7 by
  account), `tenant.selfService.perHour` on the server (default 20), and 3.3's
  1,000 tenants. Over a cap: the same `202` and an email saying the
  organisation could not be made (the answer stays the same for every
  address, as 3.3 does).
- **A domain may keep its people in one organisation.** A tenant with a verified
  domain may set `restrictOrgs: true` on it (`PATCH …/domains/{domain}`, off by
  default). Then asking to create an organisation with an address of that
  domain answers `409 domain_managed` with
  `{"tenant": {"slug", "name"}, "join": "auto" | "request" | "sso" | "invite", "start": …}`,
  and the page offers to join that organisation instead, by its join rules.
  It answers this before anything is stored (the domain's routing is public
  through `discover` already). It does not stop people of the domain from
  registering as individuals.
- **One address, one account** (3.6, see "Signing in by email address"): an
  address that already has an account cannot make a second one, so asking to
  create an organisation with it gets 3.3's "you already have an account" mail
  and nothing is made. A signed-in person creating further organisations comes
  with 3.7.
- Individuals keep 3.3's flow: an account and a personal space in the tenant of
  the page (or of a verified domain with `join.domain`, as above).
- One account in several tenants (switching organisations) is milestone 3.7
  (see "Several organisations per account").

`tenant_created` rows say `by: "self_service"` and the address.

### Updated administrators' table

| Endpoints | Tenant administrator | Site administrator |
|---|---|---|
| `/api/users…`, `/api/invites…`, `DELETE /api/users/{id}`, `/api/admin/users/{id}/deletion` | their tenant | every tenant |
| `/api/users/{id}/sessions…`, `/api/users/{id}/links…` (3.10) | `404 not_found`, as for no one | every tenant |
| `/api/admin/spaces…` (incl. 3.1's `…/storage` `exempt`, `…/evict`; `pin` within `storage.pinnedBytes`), `/api/admin/groups…`, `/api/admin/tokens…`, `/api/admin/grants…`, `/api/admin/usage`, `/api/admin/storage/people…` | their tenant | every tenant |
| `GET /api/admin/storage` | their tenant's summary, settings read only | all, and `PUT` |
| `/api/admin/recycle…` (list, restore), `/api/admin/confirm`, `/api/admin/bulk`, `/api/admin/jobs…` | their tenant | every tenant |
| `DELETE /api/admin/recycle/{item}` (purge early), exports with `history` | `403` | yes |
| `GET /api/admin/audit`, `…/kinds` | their tenant's rows, without the site's kinds, with `features.tenantAudit` | every row |
| `/api/admin/tenants/{own}`: `GET`, `PATCH name`, `…/overview`, `…/settings`, `…/security`, `…/auth/providers…`, `…/domains…`, `…/join`, `…/join-requests…`, `…/mail…`, `…/captcha`, `…/daily…`, `…/connections…`, `…/storage/target`, `…/storage/moves`, `…/storage/usage`, `…/usage/export` | yes, within the site's rules and the tenant's features (`403 feature_off`), not what a site administrator locked | every tenant, may also set every rule and lock a tenant's value (`siteLock`), never limited by features |
| `GET /api/admin/tenants/{own}/settings` (the rules, with `rules`, `inEffect`, `next`), `GET …/secrets` | read only | every tenant |
| `PUT /api/admin/tenants/{tenant}/settings/{key}` with `siteRule` or `siteLock`, `PUT …/plan`, `/api/admin/plans…`, `/api/admin/secrets…` | `403 forbidden` | yes |
| `POST /api/admin/spaces/{space}/view`, `/api/admin/exports…` (3.5) | their tenant's organisation spaces, with `features.adminView` | every space |
| `GET /api/admin/audit/download` | their tenant's rows, with `features.tenantAudit` and `features.auditDownload` | every row |
| `POST /api/admin/audit/purge` (`site.audit.purge`) | `403 forbidden` | yes |
| `POST /api/spaces/{space}/storage/move` | their tenant's spaces (and owners their own) | every space |
| `/api/admin/security`, `/api/admin/providers…`, `/api/admin/mail…`, `/api/admin/daily…`, `/api/admin/providers…` (the site's providers; a tenant administrator may `GET` the list), `/api/admin/settings…`, `/api/admin/domains…`, `/api/admin/status`, `/api/admin/secrets…`, the tenant list, create, delete, `disabled`, `slug`, `GET /api/admin/storage/pressure` | `403 forbidden` | yes |

The machine-access switches are the settings `features.tokens` and
`features.mcp`: the site's layer is `/api/admin/settings`, the tenant's
`…/tenants/{tenant}/settings`. `/api/admin/security` keeps the captcha's keys
and what the sign-in policy means now.

### Permission keys

3.6 part 1 (built). Every route outside spaces names the **permission key** it
needs in the route table, and one check decides it; no route asks "is this an
administrator" any more. A key is a name, a scope (`site`, `tenant` or
`space`), the tenant capability it needs (if any) and a description. The
registry is in code (`internal/authz`): a key is registered once, and the
names below, `GET /api/admin/permissions` and the web app's `PermissionKey`
are the same list (a test compares them). `site.spaces.personal.content` is
held by the built-in site administrator only: no custom role may hold it.

| Key | Scope | Feature | What it allows |
|---|---|---|---|
| `site.settings.manage` | site | — | Read and change the site's settings |
| `site.security.manage` | site | — | Read and change the sign-in security of the site |
| `site.signin.manage` | site | — | Add, change and remove sign-in providers |
| `site.mail.manage` | site | — | Read and change the mail settings, and send a test message |
| `site.tenants.manage` | site | — | List, create and delete tenants; change a tenant's slug, state and quota, and give a person more than their tenant allows |
| `site.connections.manage` | site | — | Set up the providers people connect to |
| `site.daily.manage` | site | — | Set up the daily text: its interfaces, providers and lines |
| `site.status.read` | site | — | Read the server's status |
| `site.storage.manage` | site | — | Change the server's storage budget and a space's storage, evict and pin spaces, see the disk, and exempt a person from the site's requirement of a remote |
| `site.recycle.purge` | site | — | Purge an item of the recycle bin for good |
| `site.audit.purge` | site | — | Purge rows of the audit log by hand, whatever the retention settings say |
| `site.admins.manage` | site | — | Change administrators of the whole server (their accounts, passwords, passkeys, sessions and tokens), delete them, and make or end one |
| `site.spaces.personal.meta` | site | — | See people's personal spaces in the administration (in lists, the recycle bin and a person's page, with their size), restore them with their person, and manage one without being its member; not their content |
| `site.spaces.personal.content` | site | — | Open an audited view of a person's personal space and export it; held by the built-in site administrator only |
| `tenant.settings.read` | tenant | — | Read the tenant and its settings |
| `tenant.settings.write` | tenant | — | Change the tenant and its settings |
| `tenant.users.read` | tenant | — | List people and read their accounts, memberships, sessions, linked accounts and passkeys |
| `tenant.users.manage` | tenant | — | Change people's accounts, end their sessions, remove linked accounts and passkeys, send password resets |
| `tenant.users.delete` | tenant | — | Delete people |
| `tenant.invites.manage` | tenant | — | Create, list and withdraw invitations |
| `tenant.groups.read` | tenant | — | List groups, their members and their spaces |
| `tenant.groups.manage` | tenant | — | Create, change and delete groups and their members |
| `tenant.spaces.create` | tenant | — | Create spaces |
| `tenant.spaces.read` | tenant | — | List the tenant's spaces |
| `tenant.spaces.manage` | tenant | — | Manage a space without being its member: its settings and members, but not its owners or its remote; measure, disable and enable it |
| `tenant.spaces.takeover` | tenant | — | Take over a space (become its owner) or transfer it; audited, and its owners are told |
| `tenant.spaces.delete` | tenant | — | Delete spaces |
| `tenant.data.view` | tenant | `features.adminView` | Open an audited, read-only view of a space |
| `tenant.users.viewAs` | tenant | `features.adminView` | View the app as a person sees it: read only, for an hour, audited; never personal spaces, drafts or the person's account |
| `tenant.data.export` | tenant | — | Export spaces and download the exports |
| `tenant.storage.read` | tenant | — | Read the storage used by the tenant and its people |
| `tenant.recycle.read` | tenant | — | List the recycle bin |
| `tenant.recycle.restore` | tenant | — | Restore an item of the recycle bin |
| `tenant.bulk.run` | tenant | — | Confirm and run bulk actions, and follow their jobs |
| `tenant.usage.read` | tenant | — | Read the usage of the tenant |
| `tenant.audit.read` | tenant | `features.tenantAudit` | Read the audit log |
| `tenant.tokens.read` | tenant | — | List people's API tokens and app authorizations |
| `tenant.tokens.revoke` | tenant | — | Revoke people's API tokens and app authorizations |
| `tenant.permissions.read` | tenant | — | List the permission keys |
| `tenant.github.manage` | tenant | — | Install the site's GitHub App on the organisation's GitHub account, and see, change or disconnect the organisation's installations |
| `tenant.domains.manage` | tenant | `features.domains` | Claim the organisation's email domains, verify them by DNS, keep others from making organisations on them, and remove them |
| `tenant.signin.manage` | tenant | `features.ownSso` | Add, change, test and remove the organisation's own single sign-on providers (OpenID Connect and SAML), and route its verified domains to them |
| `space.read` | space | — | See pages, history, attachments and search |
| `space.write` | space | — | Create, change, move and delete pages and folders |
| `space.members.manage` | space | — | Add, change and remove the members of the space |
| `space.manage` | space | — | Rename the space; its inbox, remote, sync, types and templates |

- **Built-in roles**, in code: a **site administrator** holds every site and
  tenant key, in every tenant; a **tenant administrator** every tenant key in
  their tenant; a **member** `tenant.spaces.create`; a **viewer** (3.6 part 7)
  none. Custom roles are a later part of 3.6.
- **The check**, in this order: (1) the tenant's feature the key needs
  (`403 feature_off` with `feature` and `permission`; a site
  administrator is not bound by features); (2) the key, among what the caller
  holds in the tenant (`403 forbidden` with `permission`, the key); (3) from
  3.7, the caller's standing in that organisation. A route outside spaces is
  checked in the caller's own tenant.
- **No site or tenant key reads or writes a space's content.** Content goes
  through the space keys only, which a person holds as a member of the space
  (`space.read` … `space.manage` are the four permissions `read`, `write`,
  `members`, `manage`; the API keeps those short names), or, for reading, in
  an administrator's audited view ("Viewing a space").
- **Cost.** What a person holds is built when the session cache loads them
  (with the account row it already reads) and kept with it; a check is one AND
  on two words in memory. No request makes a query it did not make before.

#### `GET /api/admin/permissions`

Needs `tenant.permissions.read`. The registry, in its order:

```json
{"permissions": [{"name": "site.settings.manage", "scope": "site", "feature": "", "description": "Read and change the site's settings"}]}
```

`GET /api/session` lists the keys the caller holds in their own tenant as
`user.permissions` (site keys included; `[]` for a request with an API token,
which no route outside spaces takes).

#### Acting scope: `PetalBook-Scope: tenant:{id}`

3.6 (c3, built). Every request acts in one tenant: by default the caller's own.
An administration request (a route whose key is a site key, or a tenant key
the member and viewer roles do not hold) may send the header
`PetalBook-Scope: tenant:{id}` to act **exactly as that tenant's
administrator**:

- only the tenant keys the caller holds there apply; site keys are set aside
  for the request (`403 forbidden` on a site route), and the tenant's features
  bind the caller (`403 feature_off`) as they bind its administrator;
- lists, `{id}`s and `?tenant=` reach that tenant only (another one answers
  `404`); personal spaces and accounts that hold site keys are out of reach;
- a takeover of a space of another tenant than one's own is refused
  (`403 forbidden`): it makes a member across tenants, which is done in the
  site's scope. Transfers inside the tenant work;
- a bulk job started in the scope keeps to it.

The header only narrows. Who does not hold `tenant.settings.read` in the
tenant, and a tenant that does not exist, get `404 not_found` (never the
caller's own scope instead). A malformed header, or the header on a route that
is not administration (a person's own routes, a space's content), is
`400 invalid`. No reason is asked; a site administrator's entering a tenant's
scope is audited as `tenant_console_opened` (`admin`, the tenant's row), at
most once an hour per session and tenant. The scope is read from the session
and the header, and the tenant from the cached list of tenants: no query.

**`?scope=tenant:{id}`.** What a browser fetches by its address cannot carry a
header (the audit's CSV, `GET /api/admin/audit/download`), so on an
administration route's `GET` the query parameter `scope` is read exactly as the
header is, with the same checks: it narrows only. Sent with the header, sent
twice or malformed it is `400 invalid`; on an administration write it is
`400 invalid` (a write names its scope with the header). A person's own routes
and a space's content do not read it.

**Capacity.** Every audit row written for an administration request records
the capacity used: `site_admin` for a holder of the route's key across the
site (in the site's scope, and in a tenant's scope they hold nothing in
themselves), `tenant_admin` for a key held in the tenant (also for a site
administrator who holds a role in that tenant, who acts there as its
administrator and writes no `tenant_console_opened`), none for anything else (sign-ins, a person's own account). A bulk job's and a
purge's rows record the capacity of the request that started them; a page
opened in a view records the viewer's capacity.

#### `GET /api/admin/scopes`

3.6 (c7). Where the console may act, for its scope selector: `{"scopes":
[{"kind": "site", "capacity": "site_admin", "permissions": […]}, {"kind":
"tenant", "id", "name", "capacity": "tenant_admin", "permissions": […]}]}`.
`permissions` are the keys the person holds in that scope, as a request there
is held to (a tenant's: its tenant keys, which the web console gates its
controls on inside `/admin/t/{id}`). `capacity` is `tenant_admin` when the
tenant's own roles give the person everything they hold there, `site_admin`
when some of it is held only across the site (a site administrator with a
narrow role in the tenant): such acts are a site administrator's, and entering
the scope is recorded as `tenant_console_opened`. `site` comes first, for whoever holds
`tenant.settings.read` across the site; then every tenant the person holds it
in through a role of that tenant (their own tenant's administrator, and a
built-in or custom role held in another tenant), by name. It asks for no key
of the person's own tenant: a member of one tenant who administers another has
the console for that one. `403 forbidden` when the list would be empty (no
console). It reads the session's cached subject and the cached tenants' names:
no query. It is a person's own route, so the scope header on it is `400`. A
site administrator's other tenants are not listed (there may be thousands):
they are entered from the tenant list.

### Taking over a space

3.6 part 1 (built), the owner's decision of 2026-11-25. Until now an
administrator who was not a member of a space managed it with `members` and
`manage`, which let them add themselves, make anyone an owner, or point the
space's remote at a repository of their own: taking the space over without
saying so. That is closed:

- Managing a space without being its member (`tenant.spaces.manage`) still
  covers its settings and its members, **except its owners and its remote**.
  Such an administrator is refused `403 takeover_required` when they would add
  or promote an owner, demote or remove one, give themselves a role (or add or
  change a group they are in), or add, demote or remove an owner group; and on
  `PUT`/`DELETE …/remote`, `POST …/remote/key`, `POST …/remote/accept`,
  `PUT`/`DELETE …/remote/assets` and `POST …/remote/assets/move`, which are for
  members only. `GET …/remote` and `POST …/remote/sync` stay theirs.
- **Taking over is explicit**: `POST /api/admin/spaces/{space}/takeover`,
  `{"reason": "the owner left the team"}` (1 to 500 characters), needs
  `tenant.spaces.takeover`. The caller becomes the space's owner (added, or
  promoted) and every other owner an editor, as `…/transfer` does. `204`.
  Audited as `space_taken_over` with `space`, `to`, `previous` (the owners it
  replaced) and `reason`; each previous owner is told by a notification of kind
  `space_taken_over` with `{"space": "Team", "by": "Ada", "reason": "…"}`
  (`ref` is the space). `409 personal_space` for a personal space; a personal
  space is not found for a tenant administrator, as for every administration
  route. After it the administrator is a member, and the remote and owners are
  theirs to change. A **site administrator** may take over a space of any
  tenant (the owner's decision, 2026-11-25): they become its owner although
  they are of another tenant, the one membership that joins two tenants. A
  **tenant administrator** only within their tenant (another tenant's space is
  `404 not_found`). A **personal space is never taken over**, by anyone:
  `409 personal_space`.
- **Groups that hold the owner role become editors** (3.6 part 2): a
  takeover or a transfer leaves the space one owner, the new one. In the same
  transaction every owner group is made an editor, its people's lists change at
  once, and each of them is told as well; the audit row lists those groups as
  `ownerGroups`.
- **Joining a group is not a way round** (3.6 part 2). An administrator who
  adds themselves to a group (`POST /api/admin/groups/{group}/members`) that
  holds a role in a space where that gives them a permission they do not hold
  already (as a member, or through another group) is refused
  `403 takeover_required`, and nothing is added. Sending a `reason` makes it
  explicit: it needs `tenant.spaces.takeover` in the group's tenant
  (`403 forbidden` naming it otherwise) and is audited as `group_self_added`
  with `group`, `spaces` (those it gave them more in), `roles` (the custom
  roles the group holds outside spaces, which count the same) and `reason`.
  Adding other people is not asked about.
- A built-in tenant administrator holds `tenant.spaces.takeover`, but only this
  endpoint (and `…/transfer`) uses it; it never widens what they may do in a
  space implicitly. The same holds for a site administrator.
- `POST /api/admin/spaces/{space}/transfer` needs `tenant.spaces.takeover`
  too, takes an optional `reason`, is audited as `space_transferred` (same
  detail) and tells the owners it replaced (kind `space_transferred`). A
  transfer to oneself is `403 takeover_required`: becoming the owner oneself is
  the takeover, with its reason.

### Storage of this section

SQLite and PostgreSQL.

- Rules and plans: `plans` (id, slug, name, description) and `tenants.plan_id`;
  the rules are rows of `layered_settings` (see "Rules for a tenant"): primary
  key `(layer, layer_id, key, source)`, `source` `manual`, `billing` or `plan`,
  `starts` and `ends` (the API's `from` and `until`, which SQL reserves),
  `set_by`, `updated`; `settings_rev`, the one-row revision every write counts;
  `tenant_usage_daily` (tenant, day, key, value; primary key
  `(tenant_id, day, key)`). 3.3's `tenants.quota` columns are dropped (the
  totals are `quota.people`, `quota.spaces`, `quota.bytes` and `quota.pages`).
- Settings: `layered_settings.note` (text, at most 200 characters, `''` for
  none), `.set_by`, `.site_lock`.
- Shapes reserved ahead of their features (nothing reads them yet):
  `tenant_integrations` (tenant, kind, id, JSON config, updated by, updated;
  primary key `(tenant_id, kind, id)`, deleted with the tenant; the sealed
  secrets stay in `secrets`); `tenant_usage_daily` with `day` as the UTC date
  `YYYY-MM-DD` and the keys `people`, `spaces`, `bytes`, `pinnedBytes`,
  `apiRequests`, `mcpRequests`, `mail` and `outgoing`, counted by one
  `INSERT … ON CONFLICT DO UPDATE SET value = value + excluded.value`;
  `storage_targets` with `app`, `prefix`, `state` (`checking`, `ok`, `broken`),
  `state_error`, `checked_at`, `in_use` and `updated_by`, and
  `storage_target_shares` (target, tenant) for a site target offered to tenants
  (only a target owned by `site` can be shared; `DeleteTenant` refuses while a
  remote's `target_id` names one of the tenant's targets).
- Tenant-owned: `tenant_mail`, `tenant_captcha`, `tenant_daily` (and its
  interfaces and lines, by tenant), `github_apps` (owner `site` or a tenant;
  the site's App moves into it from `site_records`, where 2.7 keeps it),
  `tenant_domains` (tenant, domain, code hash, code expires, verified, checked,
  missing since, last check JSON, provider, join, restrict orgs; unique on
  verified domain), `storage_targets` (id, owner (`site` or a tenant), provider
  id, installation, namespace and its id, `app`, prefix, state with its error
  and check time, in use, `updated_by`, created, updated; the kind and host are
  the provider's, read through `provider_id`, and a GitLab or Gitea token is a
  sealed secret of the tenant in `secrets`, not a column; a site target is
  offered to tenants through `storage_target_shares`), each with
  `updated_by` and `updated_at`; `site_records` key `publicMailDomains`.
  `github_apps` is the GitHub provider's table; 3.8's providers have their own.
- Sign-in: `providers.owner` and `providers.signin`, one `linked_accounts` row per (provider, person) (unique `(provider_id, subject)` and `(provider_id, user_id)`); `provider_installations` name the App
  they belong to; the sharing stays in 2.7's `github_installation_tenants`, and
  `"all"` is a flag on the installation.
- Spaces: `spaces.storage_mode`, `spaces.storage_target`, index
  `(tenant_id, storage_mode, storage_target, id)`; the per-mode counts on the
  tenant's `usage` row.
- People: `user_emails` (`user_id` with cascade, `email`, `email_key`,
  `is_primary`, `verified`, `verified_by`, `created`; primary key
  `(user_id, email)`; unique `(email_key) WHERE email_key <> ''` and unique
  `(user_id) WHERE is_primary = 1`); `users.email*` stay as the copy of the
  primary row, written with it in one transaction; the index
  `users_email (tenant_id, email_key)` is dropped; the site setting
  `users.emailNonAscii` (bool, site only, default `false`); `users.role` accepts `viewer`;
  `users.role_source` (`manual` or `idp`); `tenants.created_by` (self-service).
- Joining: `tenant_join` (tenant, domain mode, default role); `join_requests`
  (id, tenant, username, name, email, password hash, via, state, created,
  expires; unique pending username, index `(tenant_id, state, created DESC)`);
  `groups.admins` and `jit` live in the provider's settings JSON, as
  `groups.map` does.
- Single-user mode: none of this; the settings resolve site → space → person.

### Audit

| `kind` | When | `detail` |
|---|---|---|
| `setting_changed` | as 3.3, plus `note` | `key`, `layer`, `from`, `to`; for a rule for a tenant also `source` and `window` |
| `tenant_provider_created`, `…_changed`, `…_deleted` | a tenant's SSO provider changed | as `provider_*`, with `tenant`|
| `provider_assigned` | a site SSO provider was handed to a tenant | `slug`, `tenant` |
| `domain_claimed`, `domain_verified`, `domain_lapsed`, `domain_removed`, `domain_assigned` | a domain claim changed | `domain`, `tenant` |
| `tenant_mail_changed`, `tenant_captcha_changed`, `tenant_daily_changed`, `tenant_connection_changed` | a tenant-owned resource changed | the changed fields, secrets as `"changed"`; `provider` for a connection |
| `setting_rule_started`, `setting_rule_ended` | the window of a rule for a tenant started or ended (no actor) | `tenant`, `key`, `source`, `at` |
| `usage_exported` | a tenant's usage was exported | `tenant`, `from`, `to` |
| `sso_refused_site_admin` | a tenant's provider tried to sign in a site administrator | `provider`, `user` |
| `outbound_capped` | a tenant reached its outbound share (once a minute per tenant) | `tenant`, `feature` |
| `domain_checked` | a manual check of a domain | `domain`, `result`, `seen` |
| `join_requested`, `join_approved`, `join_denied`, `join_expired` | a join request | `request`, `username`, `email`, `role` |
| `joined_by_domain`, `joined_by_sso` | an account joined by (b) or (d) | `user`, `domain` or `provider`, `role` |
| `role_from_idp` | `groups.admins` promoted or demoted a person, or skipped a demotion | `user`, `provider`, `from`, `to`, `skipped` |
| `email_released` | an unverified address moved to the account that confirmed it | `user` (the one cleared), `to` |
| `plan_created`, `plan_changed`, `plan_deleted` | a plan | `plan`, `rules`, `affected`; `plan_changed` also `from` and `to` (each key's rule before and after, `null` for none) |
| `tenant_plan_changed` | a tenant's plan changed | `tenant`, `from`, `to`, `cleared` (the manual rules, dated or not, each `{key, value, lower, from, until}`) |
| `storage_target_changed` | a tenant's storage target made, changed or removed | `tenant`, `kind`, `owner` or `host` and `namespace`, secrets as `"changed"` |
| `storage_repository_created` | a repository made at a tenant's target for a space | `space`, `repository` |
| `space_storage_moved` | a space moved between modes or targets | `space`, `from`, `to`, `job` |
| `audit_downloaded` | someone downloaded audit rows as CSV (category `data`) | the filter used (`category`, `kind`, `user`, `actor`, `capacity`, `from`, `to`), `rows`, `truncated` |
| `space_taken_over`, `space_transferred` | an administrator took a space over, or transferred it | `space`, `to`, `previous`, `ownerGroups`, `reason` |
| `email_released` | an account lost an address it held unverified to one that proved it (the row is about the account that lost it; the actor is the one that took it) | `email`, `to` |

Every row about a tenant carries `by.role` (`site` or `tenant`). Tenant
administrators see their tenant's rows of these kinds, the site
administrators' changes to their tenant included.

#### `GET /api/admin/audit/download`

The audit rows of the caller's scope that the filters of `GET /api/admin/audit`
name (the same query, without `cursor` and `limit`), as CSV
(`text/csv; charset=utf-8`, `Content-Disposition: attachment`), newest first,
streamed a page of 1,000 rows at a time, so the server holds one page and never
the whole log. The columns are `id`, `time`, `category`, `kind`, `capacity`,
`tenant`, `actor`, `user`, `username`, `ip`, `detail` (JSON). A cell that starts
with `=`, `+`, `-`, `@`, a tab or a return gets a `'` in front, so a
spreadsheet does not take it for a formula. At most 100,000 rows are sent; when
there were more, the response's trailer `X-Audit-Truncated: true` says so.
Site administrators: every row (`?tenant=` to narrow). A tenant's
administrators: their tenant's rows, never the kinds only the server's
administrators see, while the tenant has `features.auditDownload` (`403
feature_off` otherwise). One download at a time per caller (`429
rate_limited`). A bad filter is `400 invalid` before anything is sent. Every
download is audited as `audit_downloaded`.

#### `POST /api/admin/audit/purge`

Deletes rows of the audit log by hand, whatever the retention settings say.
Site administrators only (`site.audit.purge`); a tenant's administrator and a
site administrator in a tenant's scope get `403 forbidden`. Body, all optional:
`category` (a category id, or `all`, the default) and `before` (an RFC 3339 time
or a day; the default and the most is now). It covers the rows of the category
written before `before`, the rows of kind `audit_purged` never. With the query
`dry_run=true` it deletes nothing, writes nothing and counts: the web asks that
first and says what will be deleted before it asks to confirm. The count is
bounded: it stops at 1,000,000 and says `"more": true` (the web shows "more than
1,000,000"). Answer: `{"dryRun": true, "count": 1234, "more": false,
"category": "all", "before": "2026-11-26T08:00:00Z"}`.

Without `dry_run` it starts a job of kind `audit.purge` and answers `202`
`{"job": {...}}` (the shape of "Jobs"): the delete is not done in the request.
The job deletes in batches of 5,000 rows, each its own short statement, resting
between them, so it never holds the database for long; its `done` is the rows
deleted so far, and `total` is 0 (unknown). One purge runs at a time (`409
busy`). A site administrator follows it with `GET /api/admin/jobs/{id}` and
`GET /api/admin/jobs?kind=audit.purge` (`site.audit.purge`); a tenant's
administrator never sees it. A restart resumes it (the bounds are fixed when the
job is made, and a delete of the same bounds only removes what is left). When
the job ends it writes exactly one `audit_purged` row, with `deleted` and
`partial: true` if it did not finish. A purge that deleted nothing still writes
its row (`deleted: 0`): every purge leaves a trace. A person who holds
`site.audit.purge` through a role, and no tenant key, follows the job with
`GET /api/admin/jobs/{id}` and `GET /api/admin/jobs?kind=audit.purge`. `400 invalid` for a category that does not
exist, a `before` that is not a time, or a `dry_run` that is not a boolean.

### Errors of this section

| Code | Status | When |
|---|---|---|
| `setting_locked`, `setting_restricted` | 409 | as 3.3; `setting_restricted` also for a value looser than a `tighten`; `setting_locked` also for a tenant value a site administrator locked (`reason.by: "site"`) |
| `domain_public` | 400 | claiming a public mail domain |
| `domain_suffix` | 400 | claiming a public suffix (`co.uk`, `github.io`) |
| `domain_code_expired` | 409 | checking an unverified claim whose code is older than 7 days |
| `domain_exists`, `domain_verified` | 409 | claiming a domain the tenant has claimed already; asking a new code for a verified claim |
| `domain_managed` | 409 | creating an organisation with an address of a domain with `restrictOrgs`; with `organization` (`slug`, `name`) |
| `join_pending` | 409 | following a registration link that would ask to join an organisation where the address already has a request waiting |
| `join_limit` | 429 | a tenant holds 1,000 requests to join waiting already |
| `tenant_disabled` | 409 | approving a request to join a disabled tenant |
| `join_decided` | 409 | approving, denying or withdrawing a request to join that is no longer waiting (decided, withdrawn, cancelled or expired) |
| `email_taken` | 409 | setting an address another account holds, verified or not (also a deleted account until its purge) |
| `email_not_ascii` | 400 | an address with characters outside ASCII while `users.emailNonAscii` is off; also asking to verify such a stored address then |
| `feature_off` | 403 | the tenant does not have the feature (`features.*`) what was asked needs; with `feature` (and `permission`, the key, when a route's key needs it); the message is a capitalised sentence naming it ("The API tokens feature is turned off for this organization.") |
| `takeover_required` | 403 | an administrator who is not a member of the space asked for what only taking it over gives (see "Taking over a space") |
| `confirm_required` | 409 | a change of a feature, a plan or its assignment that stops something in use (`features.domains`, `features.ownSso`), now or at a later start or end of a rule, without the confirmation of `change_capabilities`; turning a domain's join off, or removing the domain, while requests to join by it wait, without the confirmation of `domain_join_off`; with `plan` |
| `plan_in_use` | 409 | deleting a plan that tenants use; with `tenants` |
| `partial_unsupported` | 409 | connecting a space to a host that cannot sync partially while `storage.partial.unsupported` is `refuse`; with `host` and `provider` |
| `no_storage_target` | 409 | making a space in `tenant` mode when the organisation has no usable storage target |
| `not_implemented` | 501 | making a space at a storage target, which the server does not do yet |
| `storage_target_error` | 502 | the storage target refused to make a space's repository; with `reason`, `status` |
| `rate_limited` | 429 | as today, also a domain check within its cooldown, with `Retry-After` |
| `domain_claimed` | 409 | another tenant holds the verified claim |
| `domain_limit`, `provider_limit` | 409 | a tenant's 20 domains, 10 providers |
| `sso_required` | 409 | registering with a password for an address of a domain whose tenant requires its single sign-on, or in such a tenant; with `sso` (the `discover` answer, absent when the address routes nowhere) |
| `sso_provider_required` | 400 | a tenant's `signin.ssoOnly` to `true` while it has no single sign-on its people can use (its own, or a site SSO offered to it) |
| `address_not_public` | 400 | a tenant's issuer, metadata, SMTP host or interface resolves to an address that is not public |
| `unavailable` | 503 | as today, also when the outbound cap is reached |
| `quota_exceeded` | 409 | `quota: "pinnedBytes"` |

## Several organisations per account

Milestone 3.7 (contract, 2026-11-23; written for after 3.6 is merged). Accounts
mode only. Moved forward from phase 5 by the owner, modelled on Notion: one
account, a personal home, and membership in any number of organisations
(tenants), with the app switching between them.

With one tenant (tenancy off) no switcher is shown. As in 3.6, nothing here
exists to keep an earlier deployment behaving as it did (v0.x).

### Home, memberships, and who administers an account

- **Memberships.** `tenant_members` (tenant, user, role `admin`/`member`/`viewer`,
  `via`, `roleSource`, joined) replaces the user's single tenant. A person reaches
  a tenant's shared spaces, groups and settings only through a membership.
- **Home.** Every account has a home tenant (`users.tenant_id`, renamed in the
  code to `homeTenant`; the column keeps its name). The person's **personal
  spaces live in their home**, settings and quotas of the account itself
  (session length, passkey rules, per-person quota of personal spaces) resolve
  through it.
- **Held accounts.** `users.held`: when `true`, the home tenant's
  administrators administer the account itself (disable, password reset, email,
  passkeys, delete: everything 3.3 and 3.5 let a tenant administrator do to a
  user). When `false` (a **personal account**), only site administrators and the
  person do. Managed accounts (below) are held.
- **Every account is personal** unless it is managed: its home is the default
  tenant and it is not held. Joining an organisation, however it happens, adds
  a membership; it never moves the home. Only managed accounts are held.
- **Personal spaces are never visible to an organisation's administrators**:
  not in lists, storage views, `view` (3.5) or exports, whatever the account's
  memberships, with no exception (owner, 2026-11-23). Site administrators see
  everything.

The User object gains:

```json
{"…": "…", "tenant": {"id": "0000000000", "slug": "default", "name": "Default"}, "held": false,
 "managedBy": null,
 "tenants": [{"id": "01JDT3NANT", "slug": "acme", "name": "Acme", "role": "member", "via": "domain", "joined": "…"}]}
```

`tenant` is the home. `role` is the role in the home tenant (`member` when the person is not a member
of it). `tenants` is shown to the person and to site administrators; a tenant
administrator sees only the entry of their own tenant.

### The current organisation

- The web app's addresses carry the organisation: `/o/{slug}/…`, and `/` for the
  personal home. Every request it makes sends `X-PetalBook-Tenant: {id}` (the
  home's id for the personal home).
- A request without the header uses the session's last current tenant
  (`PUT /api/session/tenant` with `{"tenant": id}` sets it; the switcher calls
  it), so plain links work.
- The header is checked against the person's memberships held in memory with
  their permission entry (one lookup): a tenant they are not a member of (and
  not their home) answers `404 not_found`, as an unknown id does.
- What it decides: lists (`GET /api/spaces`, search across spaces, recent,
  favourites, the home page's daily text and its settings layer), where a new
  space is made (`POST /api/spaces` makes it in the current tenant; a tenant
  `viewer`, or a managed account in its personal home, gets `403 forbidden` or
  `403 managed_account`), the settings layer of everything that is not a space,
  and the administration pages (`/api/admin/…` act on the current tenant for a
  tenant administrator; site administrators keep `?tenant=`).
- What it does not decide: a route that names a space acts on the space's
  tenant. The person's permission there is their effective permission
  (memberships and groups, as 3.2), capped by their role in that tenant
  (`viewer`); being a member of the space requires being a member of its
  tenant, so no extra check is made.
- Two tabs may show two organisations at once: the session is not bound to one.

`GET /api/session` gains `current` (tenant id), `home` and `tenants` (the
switcher's list: id, slug, name, role, managed), answered from the cache.

### Tokens, OAuth and MCP

- An API token and an OAuth grant are **bound to one tenant** when made:
  `POST /api/tokens` takes `tenant` (default: the current one); the OAuth
  consent page asks which organisation (or the personal home) the client may
  reach. The binding never changes.
- A bound token reaches only spaces of its tenant the person can reach; for any
  other space the answer is `404 not_found`. The check is one comparison of the
  space's tenant with the token's, next to 3.2's.
- MCP's `tools/list`, `spaces_list` and search see only the bound tenant's spaces.
- Leaving or being removed from a tenant revokes the tokens and grants bound to
  it (the person's other tokens stay).
- While the bound tenant lacks `integrations.tokens` (tokens) or
  `integrations.mcp` (OAuth grants and MCP), they answer `403 feature_off`.

### Security rules per organisation

An organisation's `signin.ssoOnly`, `auth.passkeys.requireRoles` and
`session.idleDays`/`maxDays` (3.6) apply to the person **when they act in that
organisation** (a request whose current tenant, or whose space's tenant, is it):

- The session keeps, per tenant, how and when it proved itself there:
  `proofs: {"<tenant>": {"method": "sso", "provider": "acme-entra", "at": "…"}}`.
  Signing in fills the proof of the home and of every tenant whose rules the
  method meets; signing in through an organisation's SSO later adds that
  tenant's proof to the same session.
- A request into an organisation whose rule the session does not meet answers
  `403 tenant_sso_required` (with the provider's `start`), `403 passkey_required`, or
  `401 session_expired_for_tenant` for its session length; the page offers the
  step, and the rest of the session keeps working.
- The account's own sign-in rules stay where 3.6 put them: per-address limits
  and lockout on the site, and the home's settings for the account.
- Site administrators are never refused by an organisation's rule.

### Joining, leaving, and creating organisations

3.6 built these for one tenant per person (new accounts only). From 3.7 they add
a membership to an existing account; the endpoints keep their shapes.

- **Invites** (`POST /api/invites`): when the address belongs to an account,
  accepting the invite (signed in as that account) adds the membership with the
  invite's role; otherwise it makes a personal account with the membership.
  `POST /api/invites` answers the same either way (3.6's `409 email_taken` for
  invites is gone).
- **By domain, automatic** (`join.domain: "auto"`): an account whose verified
  address is on the domain gets the membership at its next sign-in, or when it
  verifies such an address, with `defaultRole`, and a notice. It may leave
  (unless managed).
- **By domain, request**: `GET /api/account/joinable` lists the organisations
  whose verified domains match the person's verified address, with `join`
  (`auto`, `request`, `sso`). `POST /api/join-requests` with `{"tenant": id}`
  (signed in) makes a request for an existing account; administrators handle it
  as in 3.6, and approving adds the membership.
- **Through the organisation's SSO** (the tenant's `open` mode, `jit.requireGroups`): a new
  identity makes a personal account with the membership. For an existing
  account see the owner's question 1 of 3.7 in docs/design/milestones/phase-3-autonomy-and-providers.md; until it is decided, an
  organisation's provider links an identity to an existing personal account
  only when the account's **verified** address is on a **verified** domain of
  that tenant, never for a site administrator, and the session so made carries
  only that tenant's proof: the personal home and other organisations need the
  account's own way in (password, passkey, social sign-in, or an identity the
  person linked themself).
- **Leaving**: `DELETE /api/account/tenants/{tenant}`. Refused for the home of
  a held account, for a managed account (`403 managed_account`), for the last
  enabled administrator of the tenant (`409 last_admin`), and while the person
  solely owns spaces of the tenant that have other members and no heir
  (`409 heir_required`, as 3.5).
- **Removing a member**: `GET /api/admin/tenants/{tenant}/members/{user}/removal`
  (the plan) and `DELETE …/members/{user}` (`{"heir", "transfers", "reason"}`):
  3.5's rules for deleting a person, applied only to the tenant's spaces:
  spaces the person alone owns with no other member go to the tenant's recycle
  bin, those with members are transferred, the rest are left; their group
  memberships in the tenant, their tokens and grants bound to it, and the
  tenant's proofs of their sessions are dropped. The account itself is not
  touched. Restoring the membership from the recycle item restores the spaces
  recycled with it, as 3.5.
- **Members list**: `GET /api/admin/tenants/{tenant}/members?q=&role=&via=&cursor=&limit=`,
  one query over `tenant_members` joined to `users`, keyset cursor;
  `PATCH …/members/{user}` with `{"role"}`. Each row has `held` and `managed`;
  account actions (`PATCH /api/users/{id}` beyond `role`, password reset,
  disable, `DELETE /api/users/{id}`) answer `403 account_not_held` for a member
  the tenant does not hold. `/api/users…` lists the current tenant's members, as
  before.
- **Creating an organisation** when signed in: `POST /api/tenants` with
  `{"name", "slug"}` (needs `tenant.selfService`): the caller becomes its
  `admin`. Over `tenant.selfService.perPerson` (counted by account, the tenants
  it created that still exist): `409 tenant_limit`; the server-wide caps answer
  `429 rate_limited`. An address of a domain with `restrictOrgs` answers `409
  domain_managed` as in 3.6, checked on each of the person's verified
  addresses. Managed accounts: `403 managed_account`.

### Managed accounts

Modelled on Notion's managed users. Per verified domain, off by default:
`PATCH /api/admin/tenants/{tenant}/domains/{domain}` with `{"managed": true}`.

- **Needs**: the tenant's feature `features.managedAccounts` (`403
  feature_off`), the claim verified, and an enabled SSO provider of the
  tenant (or a site provider offered to it) routed to the domain; `409 conflict`
  otherwise. If the feature is turned off, the claim lapses or the provider
  is removed, managing stops (accounts become personal again, below) and the
  administrators are told.
- **Preview and confirmation**: `GET …/domains/{domain}/managed` returns the plan
  (`{"accounts": n, "skipped": [{"reason": "site_admin" | "held_elsewhere"}], "personalSpaces": n}`);
  turning it on takes a 3.5 confirmation token (`POST /api/admin/confirm` with
  `action: "manage_accounts"`).
- **Which accounts**: those whose **verified** address is on the domain, now
  and later (an account that verifies such an address becomes managed). Site
  administrators are never managed (skipped, audited). Accounts held by another
  tenant are skipped.
- **What it does** to such an account: `held: true`, its home becomes the
  managing tenant, `managedBy` names it, and it gets a membership there (by the
  join rules, `defaultRole`) if it had none. Then:
  - it cannot create personal spaces or organisations (`403 managed_account`);
  - it signs in only through the tenant's SSO: a password, passkey or social
    sign-in answers `403 tenant_sso_required` with the provider's `start` (the account's
    sessions made otherwise end at their next request);
  - it joins by the rules above;
  - **its existing personal spaces are not deleted** and stay invisible to the
    organisation's administrators. The person is shown them at sign-in with two
    choices per space: **move into the organisation**
    (`POST /api/account/spaces/{space}/move` with `{"tenant": id}`: the space
    becomes the organisation's, the person stays its owner, its members must be
    members of the organisation or are listed for the person to resolve first,
    `409 conflict`) or **export** (`POST /api/account/exports` with
    `{"spaces": [...]}`: 3.5's export for the person's own personal spaces, kept
    7 days, audited). Until then they stay as they are, usable by the person.
- Each account taken in writes `account_managed`; the person sees a notice at
  their next sign-in saying who manages the account and what changed.
- **Turning it off** makes those accounts personal again (`held: false`, home
  the default tenant), keeps every membership, and writes `account_released`.
- **Without managed accounts**, people of the domain keep their own personal
  home and are also members of the organisation; this is the main case.

### Usage, quotas and audit

- `usage` rows are per (tenant, user). A space counts against the tenant it is
  in and against the per-person allowance of whoever created it, in that tenant;
  personal spaces count against the home. 3.3's totals and their rules are
  unchanged; `GET /api/account/usage` answers per tenant.
- Audit rows carry the tenant they are about: a space's tenant; for a
  membership, that tenant; for the account itself (password, passkeys, email,
  sign-in), the home, visible to the home's administrators only when the
  account is held; a sign-in through an organisation's SSO is also written to
  that organisation.

### Data isolation, updated

3.3's contract and its three tests stay, with these changes:

- The cross-tenant link check (`INSERT … SELECT`) joins the person's
  `tenant_members` row for the space's tenant instead of comparing two
  `tenant_id`s: adding a member, a group member, or transferring a space writes
  nothing unless the person is a member of that tenant. A personal space may be
  shared only with people who share at least one tenant with its owner.
- The fixture gains an account that is a member of both tenants A and B, with a
  personal space: every route is called with A and with B as the current tenant
  and with tokens bound to each, and must answer only that tenant's data; A's
  administrator must not see the personal space.
- The final query also asserts that every space membership's person is a member
  of the space's tenant (or its owner's personal home).

### Storage of this section

SQLite and PostgreSQL.

New: `tenant_members` (primary key `(tenant_id, user_id)`, index `(user_id)`),
`users.held`, `users.managed_by`, a `tenant_id` on API
tokens and OAuth grants, `current_tenant` and `proofs` on sessions,
`tenant_domains.managed`, and `tenant_id` in the key of `usage`.

### Audit

| `kind` | When | `detail` |
|---|---|---|
| `member_added`, `member_removed`, `member_role_changed`, `member_left` | a membership changed | `tenant`, `user`, `via`, `role`, `transfers`, `recycled` |
| `tenant_created` | as 3.6, plus `by: "member"` for a signed-in person | `tenant`, `user` |
| `managed_enabled`, `managed_disabled` | managed accounts turned on or off for a domain | `domain`, `accounts` |
| `account_managed`, `account_released`, `account_manage_skipped` | one account taken in, let go, or skipped | `user`, `domain`, `reason` |
| `personal_space_moved` | a personal space moved into an organisation | `space`, `tenant` |
| `account_export_created`, `account_export_downloaded` | a person exported their own personal spaces | `export`, `spaces` |

### Errors of this section

| Code | Status | When |
|---|---|---|
| `managed_account` | 403 | a managed account creating a personal space or an organisation, or leaving its organisation |
| `account_not_held` | 403 | an account action by a tenant administrator on a member the tenant does not hold |
| `last_admin` | 409 | the last enabled administrator of a tenant leaving it, or being demoted or removed |
| `last_local_admin` | 409 | while a tenant's `signin.ssoOnly` is on: demoting, disabling or deleting its last enabled administrator with a password (3.6 d4) |
| `tenant_limit` | 409 | a signed-in person over `tenant.selfService.perPerson` |
| `tenant_sso_required` | 403 | acting in an organisation that requires its SSO without that proof; a managed account signing in another way; with `start` |
| `session_expired_for_tenant` | 401 | the organisation's session length passed for this session's proof |
| `heir_required`, `domain_managed`, `rate_limited` | as 3.5, 3.6 | |

## Connections to GitLab and Gitea

Milestone 3.8 (contract, 2026-11-23; written for after 2.7b is merged).
Accounts mode only. Two provider types join GitHub behind 2.7b's interface
("Connections"), and pass the same contract tests:

- `gitlab`: gitlab.com and self-hosted GitLab;
- `gitea`: Gitea and Forgejo, Codeberg included (codeberg.org runs Forgejo).

Every route, object and error code of "Connections" applies, with these
differences: there are **no installations** (`capabilities.installations:
false`; those routes answer `404`), and `owners` and `repos` take `q`.

### Configuring a provider

One OAuth application per host, configured by an administrator.

- `POST /api/admin/providers` (site administrators, for the site) or
  `POST /api/admin/tenants/{tenant}/connections` (the tenant's administrators,
  needs `integrations.connections`):

  ```json
  {"id": "gitlab-acme", "type": "gitlab", "host": "gitlab.acme.example", "name": "Acme GitLab",
   "clientId": "…", "clientSecret": "…"}
  ```

  `GET`, `PATCH`, `DELETE …/connections/{provider}` as for 2.7's App. The
  secret is sealed (with the tenant's id in the purpose for a tenant's) and
  never returned (`hasSecret: true`). The answer gives `callbackUrl`
  (`{public URL}/api/account/links/{id}/callback`) and the scopes to
  tick on the host.
- Before saving, the server checks the host once: its version endpoint answers
  as a GitLab or Gitea of a supported version, and the client's credentials are
  accepted by the token endpoint's error path; `400 invalid` with `reason`
  otherwise. The host must be in the effective `storage.hosts`; every
  connection to it passes 3.6's public-address check and, for a tenant's,
  counts against the tenant's outbound share.
- At most 20 such providers on the server and 5 per tenant
  (`409 provider_limit`). `DELETE` of one that connections or spaces use:
  `409 provider_in_use` with the counts.
- **Scopes**: the least that creates and reads and writes repositories,
  written here per platform during implementation, from each platform's
  documentation.

### Connecting a person

- Authorisation code with PKCE (S256), `state` bound to the session, through
  `POST /api/account/links/{provider}/start` (`purpose: "repos"`) and the callback, as
  2.7.
- Access and refresh tokens are sealed separately. Renewal: when an access
  token is within 5 minutes of expiry on use, one renewal per person at a time,
  the new pair stored before use; in the background, one loop for every kind of
  provider (internal/connections) renews refresh tokens at `renew_at`, the time
  the pair was made with: in the last tenth of the refresh token's life and
  within 30 days of its end. The loop sleeps until the soonest `renew_at` or
  until a link is stored, renews a few links at a time, tries a link again
  after 5 minutes, then 10, 20 and so on up to 6 hours when the host cannot be
  asked, and never polls. A revoked or expired refresh token turns the connection `expired`
  and its spaces' remotes `connection_expired`, as 2.7 does: reads work, writes
  answer `503`, drafts are kept. The person gets one `link_reauth` notification
  (the write that marks the link `expired` writes it, in one transaction, so a
  loop and a request that give up together tell once).
- One account on a host (host and account id) links to one PetalBook user on
  the server: `409 account_in_use`.
- Disconnecting revokes at the host when it can, and always deletes the
  tokens.

### Syncing and making repositories

- A space syncs with the token of the person who connected it (`via: "user"`;
  2.7's rules: when that person cannot sign in, loses `manage`, or disconnects,
  the remote stops with `connection_missing` and nothing is lost).
- A tenant's storage target (3.6) on GitLab or Gitea names the provider and a
  group or organisation, with a token of the tenant's (a GitLab group access
  token; a Gitea bot account's token), and syncs `via: "tenant"`.
- `POST /api/spaces` with `storage.sync.provider` set to a GitLab or Gitea
  provider makes a private repository in the person's namespace or a group or
  organisation they may create in, then follows 2.7's order (made on disk, made
  at the host, pushed, recorded in one transaction; a repository left behind is
  kept and remembered as a leftover).
- Partial sync is detected per host as for any remote (3.5).

### Storage, audit, errors

- Storage: SQLite and PostgreSQL. Since consolidation 5b: a
  GitLab or Gitea host is a row of `providers` (`kind` `gitlab` or `gitea`,
  `repos = 1`), a person's connection a row of `linked_accounts` (`subject` is the
  account id on the host, sealed tokens, expiries, `renew_at`, `state`, `version`;
  unique `(provider_id, subject)` and `(provider_id, user_id)`), the reminders
  of empty repositories rows of `repo_leftovers`.
- Audit: 2.7b's `connection_provider_changed`, `account_connected`,
  `account_disconnected`.
- Errors: 2.7b's generic codes; `provider_limit` (409).
- Tests talk to fake GitLab and Gitea servers in the process, never to a real
  host.

### As built (3.8, 2026-11-24)

What the server does where the text above leaves room, and where it differs.

- **Setting up.** `POST /api/admin/providers` (`201`, with `Location`) takes
  `{id, type, host, name?, clientId, clientSecret}`; `GET`, `PATCH` (and `PUT`,
  the same) and `DELETE` `/api/admin/providers/{provider}`;
  `POST …/{provider}/check` reads the host again. Site administrators only (a
  tenant administrator gets `403`; the tenant routes and
  `integrations.connections` come with 3.6). `host` is a name with an optional
  port (`gitlab.example.com`, `git.example.org:8443`), always https; the
  server's `--remote-unsafe` also takes `http://…` and private addresses (tests
  and labs). `id` is 1 to 40 of `a-z`, `0-9` and `-`, not `git` or `github`, not
  starting with `t-`. The answer (and `GET`):

  ```json
  {"id": "gitlab-acme", "type": "gitlab", "host": "gitlab.acme.example", "name": "Acme GitLab", "owner": "site",
   "configured": true, "clientId": "…", "hasSecret": true,
   "callbackUrl": "https://notes.example.com/api/account/links/gitlab-acme/callback",
   "scopes": ["api"], "userAuthorization": true, "version": "16.7.2", "checked": "2026-11-24T08:00:00Z",
   "partialSync": "unknown", "connections": 0, "spaces": 0, "updated": "2026-11-24T08:00:00Z"}
  ```

  `PATCH` changes `name`, `clientId` and `clientSecret` (the host is checked with
  them before they are kept). Giving the provider **another client id** ends every
  connection to it (`state` becomes `expired`, the stored tokens are deleted, and
  people connect again): a token of a replaced application is never used. A new
  secret for the same client id keeps the connections. A check never replaces a
  version the server already knows with an empty one (GitLab does not say its
  version to anyone signed out);
  `id`, `type` and `host` cannot change (`400 invalid`). `configured` is `false`
  after a check that failed (version, platform, client), and then every call of
  the provider answers `409 provider_not_configured` until a check passes; a
  host that cannot be reached is not a verdict, and leaves the state alone.
- **Refusals** are `400 invalid` with a `reason`: `host_invalid`,
  `host_not_public`, `host_unreachable`, `not_this_platform`,
  `unsupported_version` (GitLab before 15, Gitea before 1.19; a Forgejo is
  judged by the Gitea it follows), `client_rejected`, `type_invalid`. A host the
  storage settings do not allow is the same error a remote on it gets. `409
  exists` is a taken id or host, `409 provider_limit` the 20th, `409
  provider_in_use` (with `connections` and `spaces`) a removal of one in use.
- **Checking a host.** The version endpoint (GitLab answers a signed-out caller
  `401 {"message": "401 Unauthorized"}`, which is how it is recognised; its
  version is read with the first person's token and kept), then the token
  endpoint is called with a code that cannot be valid: `invalid_grant` means the
  host knows the client, `invalid_client` that it does not.
- **Scopes.** GitLab: `api` (creating a project needs it; `read_repository` and
  `write_repository` only cover Git). Gitea and Forgejo: none are asked for; the
  application's token is as wide as the person's own rights.
- **Tokens.** Git is given the user name `oauth2` and the access token. An
  OAuth token is the person's, not narrowed to one repository as a GitHub
  installation token is, so a space syncs with the connected person's token and
  nobody else's; `via` is always `user` (`"app"` is `400 invalid`). GitLab's
  refresh tokens do not end; Gitea's end at the `exp` they carry (read, not
  verified), so `refreshExpires` is there for Gitea only. A refresh token is
  renewed in the last tenth of its life (and at the latest 30 days before it
  ends), not on a fixed window: a Gitea token that lives 730 hours is renewed
  once, about three days before it ends, not every day. A `401` from the host to
  a call made with a person's token does not end the connection by itself: the
  server renews the pair once (one renewal at a time per person; if another
  request has already replaced the token, that one is used) and makes the call
  again once; the connection becomes `expired` only when the host refuses the
  refresh token itself. GitLab is asked to revoke on disconnect (`/oauth/revoke`); Gitea has no such endpoint, and the
  tokens are deleted here either way.
- **Owners and repositories.** `GET /api/account/links/{provider}/owners?q=&cursor=`
  lists the person's own namespace first (`type: "User"`, first page only), then
  groups (GitLab: those with at least Developer access, subgroups by full path)
  or organisations (Gitea: those where one of their teams may create
  repositories); `via` is `user`, `installationId` is `null`, `needsInstall` is
  `false`. `GET …/repos?q=&cursor=` lists the repositories the person's
  connection can push to (`mine=true` may be said, and is not needed;
  `installation=`, with any value, is `400 invalid`). Cursors are page numbers made by the server
  (`400 invalid` otherwise); `q` is at most 100 characters. A GitLab group that
  lists the person but does not let them create projects passes `PlanCreate`
  and is refused when the project is made (`403 provider_refused`); a Gitea
  organisation is checked first (`403 owner_not_allowed`).
- **Administrators of a Gitea.** When the person who connects is an
  administrator of the Gitea or Forgejo instance (its user answer says
  `is_admin`), `connection` has `"admin": true` (the field is absent otherwise,
  and always for GitLab). The token the server then holds carries those
  administrator rights, which a Gitea cannot narrow. The web app shows a
  warning on that connection (in English and Chinese) and suggests an ordinary
  account. It is read when the person connects and kept until they connect
  again; it is not asked of the host at each renewal.
- **Names.** A repository name is 1 to 100 of letters, digits, `.`, `_` and `-`,
  not `.`, `..` or ending in `.git` (GitLab: also not starting with `.` or `-`
  and not ending in `.atom`; Gitea: not ending in `.wiki`). A name that is taken
  is `409 name_taken`, as on GitHub; the automatic suffix is 3.6's.
- **Partial sync.** The first time a repository is made or connected, and every
  30 days after, the server asks the repository for its Git protocol v2
  advertisement with the person's token and records whether `fetch` lists
  `filter` on the provider (`partialSync` is then `yes` or `no`; `unknown`
  before). 3.5's table of hosts takes over this column when it is built.
- **Limits of the checks.** Every request the server makes to a host (OAuth,
  REST, the partial-sync probe) is refused unless the address it resolved to is
  public, follows no redirect and uses no proxy of the environment. The `git`
  programs connect by themselves and are held only to the allowed hosts.
- **Tenants.** A provider has an owner, `site` or a tenant, shown as `"site"` or
  `{"tenant": "<id>"}`; people of other tenants do not see a tenant's provider in
  the list and get `404 not_found` from its routes. Nothing makes a tenant's
  provider yet (3.6).
- **Audit.** `connection_provider_changed` with `change` `created`, `saved` or
  `removed` (never a secret); `account_connected` and `account_disconnected` as
  for GitHub.
- **Process.** Providers live in the memory of the process that serves them;
  one set up by another process of the same database is picked up by the daily
  renewal pass or a restart. Saving a provider, checking it and removing it
  never write a stale copy of the row: a save reads the provider after it has
  the lock; a check writes the state, the version and the time only, and not at
  all if the client was replaced while it ran; a reload reads the rows with the
  lock held, so it cannot bring back a provider that was just removed.
- **Error text.** What a host says in an error (its message, an OAuth
  description) reaches errors and logs with line breaks and control characters
  replaced by spaces, at most 300 bytes, and anything that looks like a token
  (a GitLab or GitHub token, a private key, a JWT) replaced by `[redacted]`.

## Avatars and brand icons

Milestone 3.9 (contract, 2026-11-23; written for after 3.5 is merged; the brand
icons do not depend on anything).

### Brand icons

- GitHub's icon is already drawn since 2.7b (Simple Icons' path inlined in the
  web app); 3.9 adds the others the same way.
- GitHub, GitLab, Gitea, Forgejo, Codeberg, Google, Microsoft and Apple use
  their official brand icons from the Simple Icons set (CC0, monochrome SVG,
  drawn with `currentColor` so they follow the theme). For a brand that Simple
  Icons does not carry (it withdraws icons at a brand's request), the brand's
  own published mark is used the same way. The trademarks stay the brands';
  `NOTICE` says so.
- They appear on sign-in buttons, in "已连接的账号", on a space's remote, on the
  "存储与同步" page and in a new space's "存在哪里".
- "Sign in with X" buttons follow each brand's guidelines (wording, colours,
  spacing, minimum size).
- The icons are bundled with the web app (only these); nothing is fetched from
  elsewhere. The client picks the icon by the provider's `type` (and by the
  sign-in provider's kind for Google, Microsoft and Apple).

### Images

Every image people give is handled the same way:

- Accepted: PNG, JPEG, WebP, GIF (the first frame), at most 1 MB. **Never SVG.**
  The size in the header is read before decoding; more than 40 million pixels is
  refused.
- Re-encoded on the server to a 256 × 256 square (centre crop) PNG (when it has
  transparency) or WebP, metadata dropped. Only the re-encoded image is kept.
- Served from `/api/avatars/…?v={version}` with `Cache-Control: public,
  max-age=31536000, immutable`, `X-Content-Type-Options: nosniff` and
  `Content-Security-Policy: default-src 'none'`. A person who may not see the
  thing (a space they cannot open, a user of another tenant) gets `404`.
- **No Gravatar** by default (it would hand a hash of the email to a third
  party).

| Code | Status | When |
|---|---|---|
| `image_too_large` | 413 | over 1 MB, or over 40 million pixels; with `limit` |
| `image_type` | 415 | not PNG, JPEG, WebP or GIF (SVG included) |
| `image_invalid` | 422 | the image does not decode |

### A person's avatar

- `PUT /api/account/avatar`: the image as the body (`Content-Type` of the
  image), or `{"from": {"provider": "github"}}` (a connection, 2.7b and 3.8) or
  `{"from": {"identity": "<id>"}}` (a sign-in identity, 2.8): the server fetches
  that avatar once (3.6's public-address check), handles it as above and keeps
  it; it never fetches it again. `200` with the User's `avatar`.
- `DELETE /api/account/avatar`: back to the default. `204`.
- The default is the initials of the name and a colour chosen by the user's id
  from twelve that keep their contrast in both themes. The User object gains
  `avatar`: `{"url": "/api/avatars/users/{id}?v=3"}` or
  `{"initials": "TW", "colour": 7}`.
- Stored on the server (`DATA/avatars/users/{id}.{png|webp}`); the state
  database keeps `users.avatar_version`.

### A space's icon

- Set by the space's managers (`manage`): `PUT /api/spaces/{space}/icon` with
  `{"emoji": "🌸"}`, `{"icon": "book"}` (a name from the app's icon set), or the
  image as the body; `DELETE` removes it. Each is a commit to the space by the
  caller.
- **Stored in the space's own repository**, so it travels with export, sync and
  moves: an image is `.petalbook/icon.png` or `.petalbook/icon.webp`; an emoji or
  icon name is `icon` in `space.yml`.
- A 64 px copy (at most 16 KB) is kept on the space's row of the state database
  (`spaces.icon_small`), so lists show it while the space is evicted or
  archived. It is updated when the icon changes, through the API or from the
  remote.
- **An image found in the repository** (pushed by anyone) is checked and
  re-encoded as above before it is shown; one that fails is treated as no icon
  and reported on the checks page (`kind: "icon_invalid"`).
- The Space object gains `icon`: `{"emoji": "🌸"}`, `{"icon": "book"}`,
  `{"url": "/api/avatars/spaces/{id}?v=…"}` or `null`.

### An organisation's logo

- `PUT /api/admin/tenants/{tenant}/logo` (the tenant's administrators and site
  administrators), the image as the body; `DELETE` removes it. Stored on the
  server as a person's avatar. The Tenant object gains `logo`.

### Audit

`avatar_changed` (`user`), `space_icon_changed` (`space`, `kind`),
`tenant_logo_changed` (`tenant`).

## Administration

Accounts mode, administrators only; single-user mode answers `404`, anyone else
`403 forbidden`. From 3.3 an administrator is a site administrator or a tenant
administrator, and a tenant administrator sees only their tenant and none of the
server's own settings (see "Administrators" under "Tenants, registration and
mail"). These endpoints work on the server's records, not on a space's
pages: none opens a space (so they work on a disabled one), and nothing they do
deletes a file. They are separate from "Users and invites", which is unchanged.

### `GET /api/admin/spaces?q={text}&cursor={cursor}&limit={n}`

Every space on the server, in order of creation, one page at a time:

```json
{"items": [{"id": "01JD3K8Q7Z", "name": "Team", "personal": false, "inbox": "Inbox", "created": "2026-11-21T08:30:00Z",
  "disabled": false, "owner": {"id": "01JD3K7A2M", "username": "tang", "name": "Tang"}, "members": 3,
  "sizeBytes": 18304211, "sizeAt": "2026-11-22T02:10:00Z", "sync": "local"}],
 "next": "01JD3K8Q7Z"}
```

`q` keeps the spaces whose name contains it, ignoring case the same way on SQLite and
PostgreSQL: the server compares Unicode case foldings of both (a column of the
state database, `name_key`, written with every name), so `STRASSE` finds
`Straße`, `唐诗` finds `唐诗 Tang`, and `İ` is the letter `i` with a dot above it
(it is found by `i`, not by `I`). `limit` is 1 to
200 (default 50). `next` is the `cursor` of the following page, `""` after the
last. The list is one database query for the whole page, however many spaces
there are.

`inbox` is the folder quick notes go to (see "Space"): an administrator who opens a
space they are not a member of captures into it. `owner` is the first owner (the one whose name is the Git author of a space's
default commits), `null` if there is none. `members` counts every member.
`sizeBytes` and `sizeAt` are the last stored measurement and are both `null`
until the space has been measured (also when its first measurement failed). Sizes are not measured when the list is asked for:
a space can hold tens of thousands of pages. The server measures them in the
background, one space at a time: it takes the space measured longest ago (or
never), if that was more than 6 hours ago, and adds up the sizes of the files
under its directory, `.git` included (the index cache is not counted); then it
goes straight on to the next stale space, so a first round over thousands of
spaces takes as long as the walks do (a few hundred milliseconds for a space of
tens of thousands of pages), not days. Walks never overlap and pause
briefly after each directory, so the load stays that of one walk. When no space
is stale it looks again every minute. So the number can be several hours old.
`POST .../size` measures one space now.

`sync` is `local` or the state of the remote the space follows (see "Remote
repository"), as the server knows it without opening the space. A space that
has never been opened since the server started has the state it was last left
in; nothing is opened to fill this in.

Milestone 3.1 adds `storage` to each item, and the filters `cache` and
`overdue` (see "Server storage", "Administrators: per space").

### `GET /api/admin/spaces/{space}`

One space, the same object as an item of the list (same first owner, same
member count), found by its id with one query; this is how a client finds a
space it was sent to by address without reading the list. `404 not_found` for
an id that is no space.

### `POST /api/admin/spaces/{space}/size`

Measures the space's directory now (waiting its turn if a measurement is
running: they never run in parallel) and stores the result. `200` with
`{"sizeBytes": 18304211, "sizeAt": "2026-11-22T02:10:00Z"}`.

### `POST /api/admin/spaces/{space}/transfer`

`{"userId": "01JD3K7A2M", "reason": "…"}` (`reason` optional, at most 500
characters). Needs `tenant.spaces.takeover` (3.6); audited and the replaced
owners are told, as for "Taking over a space". The user becomes the owner of the space (a member
is promoted, anyone else added) and every other owner becomes an editor. The
Git author of later commits is the new owner, also in a space that is open
now. `204`. `404 user_not_found` when there is no such user, `409 user_disabled`
when the user cannot sign in, `409 personal_space` for a personal space.

### `POST /api/admin/spaces/{space}/disable`

`204`. A disabled space stays in its members' lists (`disabled: true`) but
cannot be opened by anyone, administrators included: every request on it
answers `403 space_disabled`, and the drafts waiting in it are kept and
committed once it is enabled. If it is open it is closed after committing what
it holds; requests that are using it at that moment finish first (the server
waits up to 5 seconds, then leaves it to close as soon as the last one is done).

### `POST /api/admin/spaces/{space}/enable`

`204`. Undoes `disable`.

### `DELETE /api/admin/spaces/{space}`

From 3.5 the space goes into the recycle bin instead (see "Storage management"); what follows is the 1.7 behaviour.

Closes the space (committing what it holds), deletes its records (members,
drafts, remote and the space itself) and moves its directories into the trash
inside the data directory; no file is deleted. `200`:

```json
{"trashed": "trash/01JD3K8Q7Z-20261122T021000Z", "cacheTrashed": "trash/01JD3K8Q7Z-20261122T021000Z.cache", "warnings": []}
```

`trashed` is the repository, `cacheTrashed` the index and journal (moved too:
the journal can hold a change that never reached Git), both relative to the
data directory. To get a space back, move the directory back and add the
records by hand; to get the room back, delete the trash. Members' drafts, held
ones too, are written first to `trash/<id>-<stamp>.drafts.json` (space, page,
user, base rev, header, body, times) so that the text typed and not saved is not
lost with the records; if that file cannot be written nothing is deleted. If a move fails after
the records are gone, the files are left where they are, `trashed` is `""` and
`warnings` says what happened (the server also logs it as an error). Members'
very next request answers `404 not_found`. `409 busy` when requests still use
the space or its pending changes cannot be committed: nothing was changed, try
again. `409 personal_space` for a personal space, which goes with its person.

### `GET /api/admin/status`

```json
{"version": "0.9.0", "startedAt": "2026-11-22T01:00:00Z", "uptimeSeconds": 4210, "database": "sqlite",
 "spaces": 212, "users": 40, "openSpaces": 7, "diskFreeBytes": 52428800000, "goVersion": "go1.27.4",
 "build": {"commit": "3f9c2ab", "time": "2026-11-25T22:48:00Z", "timeKind": "commit", "modified": false}}
```

`version` is the release tag the binary was built with, `dev` otherwise. `build`
says which build it is, read from what the Go toolchain stamps into the binary
(`debug.ReadBuildInfo`): `commit` is the first 7 characters of the commit,
`time` the time of that commit (RFC 3339, UTC), `modified` whether the working
tree had changes the commit does not have. A build with `-ldflags "-X
main.buildTime=<RFC 3339>"` reports that time instead, and `timeKind` is then
`build`; otherwise it is `commit`. `commit` and `time` are `""` (and `timeKind`
`""`) for a binary built outside a Git checkout. The detail is only here, for
site administrators; the public `GET /api/meta` keeps its short `version`.

`database` is `sqlite` or `postgres`. `openSpaces` is how many spaces are open
in memory now (idle ones are closed, see `--space-idle`). `diskFreeBytes` is
the room left for the server on the disk of the data directory; it is absent
on platforms that cannot say (anything but Linux, macOS and FreeBSD).
Milestone 3.1 adds `storage` (see "Server storage").

### Security settings

The 安全 page of `/admin`. The sign-in policy is layered settings of the site;
the keys that take effect after the person is known, or on a page opened for
an organisation, have a tenant layer too (see "The sign-in policy of a
tenant"), and the rest are the same for every tenant (the sign-in page comes
before any tenant is known). Written with `PUT /api/admin/settings/{key}` and read with
`GET /api/admin/settings`:

| Key | Kind | Default | |
|---|---|---|---|
| `signin.mode` | choice | `dual` | `dual`, `sso_first`, `sso_redirect` or `local_only` |
| `signin.ssoOnly` | boolean | `false` | refuse password and passkey sign-in for everyone who is not an administrator; from 3.6 d4 a tenant may also turn it on for its own people (a tenant layer that only tightens, see "Email domains and the SSO entry") |
| `features.passkeys` | boolean | `true` | passkeys on for the whole server |
| `signin.passkeyRoles` | list | `[]` | the roles a passkey is required of: `admin`, `member` |
| `captcha.provider` | choice | `image` | `image`, `turnstile`, `recaptcha` or `hcaptcha` |
| `captcha.contexts` | document | all off | `{"login", "invite", "setup", "forgot", "register": {"on", "when": "always"\|"after_failures", "failures": 1–100}}`; a partial document is merged over the current one, and the answer is the whole |
| `signin.lockout.enabled`, `.minutes`, `.scope` | boolean, 1–10080, choice | `false`, 15, `ip_account` | `ip_account`, `ip` or `account` |
| `signin.perMinute`, `signin.maxFailures`, `signin.windowMinutes` | limits | 10, 10, 15 | as in "Rate limit, failure caps and lockout" |
| `sessions.idleDays`, `sessions.maxDays` | limits | 30, 90 (the `--session-*` flags) | 1–365 and 1–3650, `maxDays` at least `idleDays` |

The server works the policy out from them once for each set of their values (a
request costs one atomic load and the settings' cache), so a write applies to
the next request. A change by another process (`petalbook security reset`, a
standby server) is seen through the settings' revision, read at most once every
60 seconds on a request.

The checks that look at more than one value are the settings' own, and refuse
the write that breaks them (`400` unless stated):

| `error` | When |
|---|---|
| `local_admin_required` | `signin.ssoOnly` to `true`, or `signin.mode` other than `local_only` while `ssoOnly` is on and the stored mode is `local_only`, while no enabled administrator has a password. Creating or enabling a provider while `ssoOnly` is on is refused the same way, and while it is stored (in effect or not) the last enabled administrator with a password cannot be made a member or disabled (`409 conflict`) |
| `sso_only_with_registration` | `signin.ssoOnly` to `true` while `features.orgCreation` is open (see "Registration") |
| `captcha_site_key_required` | a context switched on with a third-party provider and no site key, or a change of provider while a context is on (the keys belong to the provider: a new provider has none) |
| `captcha_secret_required` | a context switched on with a third-party provider and no secret |
| `captcha_needs_public_url` | a third-party provider without `--public-url` (its host name is what the answer is checked against) |
| `passkey_unavailable` | `signin.passkeyRoles` not empty, or `features.passkeys` on again with requirements, while passkeys cannot work (no suitable public URL) |
| `passkey_requirement_unsatisfiable` | `features.passkeys` off while passkeys are required of someone (lift the requirements first) |
| `invalid` | a role, a mode, a context or a number that is none, or out of range (never clamped) |

Every write is one `setting_changed` row of the audit log. (`security_changed`
is gone.)

#### `GET /api/admin/security`

What the policy means now, and the captcha's keys; nothing of the policy itself:

```json
{
  "login": {"effectiveMode": "dual", "ssoOnlyInEffect": false, "ssoAvailable": true},
  "passkeys": {"available": true, "required": 2, "requiredWithout": 1},
  "captcha": {"siteKey": "", "hasSecret": false},
  "passwordAdmins": 1,
  "notices": []
}
```

- `effectiveMode` is what the sign-in page uses (`local_only` with no enabled
  provider: the page says so, and the choice is kept for when one is added).
  `ssoAvailable`: some provider is enabled. `ssoOnlyInEffect` is `false` while
  the page offers no provider: refusing passwords then would leave people a form
  that always fails and nothing else. Administrators are never refused by
  `ssoOnly` (`403 local_login_refused` for everyone else, checked after the
  password): when the identity provider breaks, they can still sign in.
- `passkeys.available` is whether passkeys can work (see "Passkeys"); `required`
  counts the people a passkey is required of (by role or by themselves),
  `requiredWithout` those of them who have none yet and will be held at their
  next sign-in.
- `captcha.siteKey` is public (it goes into the page). The secret is never
  returned: `hasSecret` says whether one is stored.
- `passwordAdmins` counts the enabled administrators with a password: the ones
  who can still sign in if every provider fails.
- `notices`: things the administrators should know, each `{"code", "message"}`:
  `secret_unreadable` (a stored secret cannot be opened with this server's key,
  with `slug` or `field`), `passkey_host_changed`.

#### `PUT /api/admin/security`

Takes `{"captcha": {"siteKey": "…", "secret": "…"}}` and nothing else (`400
invalid`): the part of the sign-in policy that is not a setting. The site key is
a record of the server; the secret is the secret `captcha.secret` (see
"Secrets"), absent keeps the stored one and `""` clears it. `400
invalid` while the provider is `image`, which has no keys. `captcha_secret_required`
and `captcha_site_key_required` when a context is on and the change would leave
none. A change of `captcha.provider` clears both. `200` with the view as `GET`.

### Sign-in providers

At most 50, in the table `providers` (an `id`, an `owner`, flags `signin` and
`repos` for what a row may be used for; this section is the sign-in rows).
Each has a `slug`: 1 to 32 characters of `a-z`, `0-9` and `-`, starting with a
letter or digit, unique per owner and fixed once made (it is in the addresses
registered at the identity provider). The `id` of a site provider is its slug;
a tenant's provider has a generated id, so its slug never meets another
organisation's. Accounts are linked by the provider's `id`. All providers are read into
memory once; a change re-reads them and rebuilds only the changed provider's
client. OpenID Connect discovery documents and SAML metadata are kept in the
state database and in memory, so a sign-in does not fetch them; SAML metadata
given by address is fetched again every 24 hours in the background and on
`check`.

Secrets (an OpenID Connect or GitHub client secret, the SAML signing key) are
sealed with the server's key (`secret.key`, as remote credentials are) and are
never returned: `hasSecret` says whether one is stored.

#### Provider

```json
{
  "id": "company", "owner": "site",
  "slug": "company", "kind": "oidc", "name": "Company SSO", "icon": "preset:microsoft",
  "enabled": true, "order": 0,
  "mergeVerifiedEmail": false,
  "oidc": {"issuer": "https://login.microsoftonline.com/<tenant>/v2.0", "clientId": "…", "hasSecret": true,
           "scopes": ["profile", "email"],
           "redirectUrl": "https://notes.example.com/api/auth/oidc/company/callback"},
  "status": {"ok": true, "checked": "2026-11-23T08:00:00Z", "message": ""},
  "people": 12,
  "created": "…", "updated": "…"
}
```

- `owner` is `site`, or the id of the tenant that owns the provider (an
  organisation's own SSO); it is given when the provider is made (`owner` in the
  body, site administrators only for now) and cannot change. A tenant's
  provider puts new accounts in that tenant, is made `enabled: false`, and can
  be turned on only once the tenant has a verified domain (`409
  domain_required`): domain verification arrives with 3.6 (d), until then no
  tenant has one, so none can be turned on. It is not reachable by a slug, and
  its addresses are trusted only on the tenant's verified domains.
- `kind` is fixed once made. Exactly one of `oidc`, `saml`, `github` is present,
  matching it.
- `name` is 1 to 60 characters; `icon` as in `GET /api/auth/providers`.
- `order` sorts the buttons (then `name`).
- There is no switch for making accounts at the first sign-in: that is the
  registration mode of the tenant the sign-in lands in (see "Registration"). The
  provider's `tenant` is where its accounts land (a tenant's own provider: that
  tenant).
- `mergeVerifiedEmail`: from 3.6, "trust this provider's verified email
  addresses": an OpenID Connect provider's address with `email_verified: true`
  counts as verified (see "Which sign-ins vouch for an address"). For
  SAML (no such claim) it is the administrator's word that the address is
  verified. It never signs anyone in to an existing account. It does nothing
  for GitHub (trusted by its API), and only the site's own providers may have
  it (`400 invalid` for a tenant's). Off by default. The name stays for the
  contract.
- `status` is the last discovery or metadata fetch; `people` how many accounts
  are linked to this provider.

`oidc`: `issuer` (an `https` address), `clientId`, the secret (write-only
`clientSecret`; may be empty for a public client: PKCE is always used),
`scopes` besides `openid` (default `profile`, `email`), and the read-only
`redirectUrl` to register.

`saml`:

```json
{"metadataUrl": "https://idp.example/metadata", "metadataXml": "", "idpEntityId": "https://idp.example",
 "entityId": "https://notes.example.com/api/auth/saml/campus/metadata",
 "acsUrl": "https://notes.example.com/api/auth/saml/campus/acs",
 "metadataPath": "/api/auth/saml/campus/metadata",
 "attributes": {"email": "mail", "name": "displayName", "username": "uid"},
 "nameIdFormat": "", "signRequests": false, "allowIdpInitiated": false, "certificate": "-----BEGIN CERTIFICATE-----…", "certificateExpires": "2036-11-23T00:00:00Z"}
```

One of `metadataUrl` (`https`) or `metadataXml` is given; `idpEntityId` is read
from the metadata. `entityId` defaults to the metadata address shown. The
signing key and certificate are made by the server when the provider is
created (RSA 2048, 10 years); the key is never shown. `nameIdFormat` `""` lets
the identity provider choose (transient is refused at sign-in). `attributes`
name where the email, display name and username come from. The subject of the
linked account is the `NameID`, which need not be an address (`persistent` is
recommended: an address can change). The address is the account's UPN and is
required: it is read from `attributes.email` (`mail` when empty), else taken
from the `NameID` only when its format is `emailAddress`; with neither the
sign-in is refused with `sso_error=saml_no_email`, which tells the administrator
to map an email attribute. Use "Test sign-in" to see what the provider sends.

Behaviour change: a site SAML provider with `mergeVerifiedEmail` on now vouches
for the address it sends (it counts as verified, as an OpenID Connect
provider's `email_verified: true` does), and a SAML sign-in without an address
is refused with `saml_no_email` instead of going on with none.

`github`:

```json
{"baseUrl": "https://github.com", "clientId": "…", "hasSecret": true,
 "redirectUrl": "https://notes.example.com/api/auth/github/github/callback"}
```

`baseUrl` is `https://github.com` or a GitHub Enterprise Server address.
Which organisations a person belongs to is read only as the groups claim (see
"Groups from sign-in"): `groups.allow` holds sign-in to the organisations it
names, and `groups.map` maps them to groups.

`groups` (any kind, from 3.2; absent or `{"enabled": false}` by default): how
the provider's groups claim maps to synced groups; see "Groups from sign-in".

#### `GET /api/admin/providers`, `GET /api/admin/providers/{id}`

`{"providers": [{…}]}` in `order`, and one provider (by its `id`).

#### `POST /api/admin/providers`

A provider as above, with `slug`, `kind`, `name` and the kind's required
fields; for `oidc` and `github` the secret as `clientSecret`. `201` with the
provider. The ids of the server's own providers are reserved: `github` is for
`kind: "github"` only, and `git` for none (`409 slug_reserved`). The server fetches the discovery document or the metadata before it
answers; a provider that cannot be reached is still stored, `enabled: false`
unless asked otherwise, with `status.ok: false`.

#### `PATCH /api/admin/providers/{id}`

Any fields but `slug`, `kind` and `owner`. `clientSecret` absent keeps the stored one,
`""` clears it. `200` with the provider. A `kind: "github"` provider's `github.baseUrl` moves to
another host only while no account is linked to it (links are GitHub's numeric
ids, unique only on one host): `409 provider_host_mismatch` otherwise; delete
the provider, or remove its linked accounts, first.

#### `DELETE /api/admin/providers/{id}?force=1`

`204`. The accounts its people linked to it go with it (a provider added again
with the same issuer brings none of them back). Without `force=1`, refused with
`409 provider_in_use` (and `{"accounts": n}`) when some account has no other way
to sign in: no password, no passkey, no linked account at another provider.
The sessions that started through the provider (`sso:<slug>`) end with it, and
answer `401 signed_out_elsewhere` afterwards. `409 provider_in_use` (with
`{"spaces": n}`) while a space syncs through it, counted again in the deleting
transaction. A provider people sign in with (the GitHub row once it is turned
on for sign-in too) is removed only by who manages sign-in
(`site.signin.manage`): an administrator who manages only connections gets
`403 forbidden`, and nothing changes.

#### `POST /api/admin/providers/{id}/check`

Fetches the discovery document or the metadata now. `200` with the provider
(its `status` says how it went).

#### `POST /api/admin/providers/{id}/test`, `GET /api/admin/providers/{id}/test`

A test sign-in, for a site provider on or off. `POST` answers `{"url": "…"}`:
send the browser there; the administrator signs in at the provider, and
nothing is made, linked or signed in. The browser comes back to
`/admin/security/providers?provider_test={id}`, or to
`/admin/security/providers?sso_error=<code>` when the sign-in failed (for
instance `saml_no_email`). `GET` answers what the provider said, once, to the
administrator who ran it and for five minutes:
`{"result": {"subject", "email", "emailTrusted", "name", "username", "unresolved", "at"}}`, or
`{"result": null}`. `emailTrusted` is whether the trust rules would verify the
address. `unresolved` is the list of configured paths of a custom OAuth 2.0
provider that found nothing in the userinfo answer (`[]` for any other kind).

#### Provider refusals

| `error` | Status | When |
|---|---|---|
| `invalid` | 400 | a field is missing, too long or not of its form (the message names it) |
| `exists` | 409 | the slug is taken |
| `domain_required` | 409 | turning on a provider owned by a tenant that has no verified domain |
| `provider_limit` | 409 | there are 50 providers |
| `subject_fixed` | 409 | changing `oauth2.fields.subject` or the host of `oauth2.userinfoEndpoint` while accounts are linked to the provider |
| `oidc_issuer_invalid` | 400 | the issuer is not an `https` address |
| `saml_needs_https` | 400 | SAML without an `https` `--public-url` (the cookie that crosses from the identity provider must be `Secure`) |
| `saml_metadata_invalid` | 400 | the metadata cannot be parsed, has no signing certificate or no redirect binding for sign-in |
| `public_url_required` | 400 | any provider without `--public-url` |
| `provider_in_use` | 409 | `DELETE` without `force=1`, see above |

### Groups

Milestone 3.2; see "Groups" for what a group is and the codes. Every list is one
query a page.

#### `GET /api/admin/groups?q={text}&source={source}&user={id}&cursor={cursor}&limit={n}`

`{"items": [Group], "next": "…"}`, by name. `q` keeps names containing it
(ignoring case, as above); `source` is `local` or `synced`; `user` keeps the
groups that user is in. `limit` 1 to 200 (default 50).

#### `GET /api/admin/groups/{group}`

One group; `404 not_found`.

#### `POST /api/admin/groups`

`{"name": "Engineering", "description": "", "source": "local"}`; `source` is
required. `201` with the group. `409 exists` for a name taken, `409 group_limit`
at 10,000 groups in the tenant (each tenant has its own 10,000; the message
says how many the tenant has). A synced group starts empty; map it from a provider's
`groups` setting.

#### `PATCH /api/admin/groups/{group}`

`{"name": "…", "description": "…"}`, each optional. `200` with the group.
`source` cannot change (`400 invalid`).

#### `DELETE /api/admin/groups/{group}?force=1`

`204`. Without `force=1`, refused with `409 group_in_use` while the group is a
member of a space or a provider maps to it. With it, the group leaves every
space, its mapping entries are removed from the providers, and its people's
cached lists are dropped. Nothing else is deleted: people, spaces and drafts
stay.

#### `GET /api/admin/groups/{group}/members?q={text}&cursor={cursor}&limit={n}`

`{"items": [{"userId", "username", "name", "disabled", "from": [{"provider": "company", "value": "…"}], "added": "…"}], "next": "…"}`,
by username. `from` is `[]` in a local group; in a synced group it is the
provider and claim value each membership came from.

#### `POST /api/admin/groups/{group}/members`

Local groups only (`409 group_synced`). `{"userIds": ["01JD3K8Q7Z", …]}`, 1 to
500, and an optional `reason` (1 to 500 characters). `200` with
`{"added": 3, "already": 1}`. All or nothing: an unknown id is
`404 user_not_found` with `{"missing": […]}`; `409 group_limit` when the group
or a person would pass their limit. Disabled users may be added (they still
cannot sign in). Adding **oneself** to a group that would give one more in a
space than one holds is `403 takeover_required` without a `reason`; with one it
needs `tenant.spaces.takeover` and is audited as `group_self_added` (see
"Taking over a space").

#### `DELETE /api/admin/groups/{group}/members/{user}`

Local groups only (`409 group_synced`). `204`; `404 not_found` when they are not
in it.

#### `GET /api/admin/groups/{group}/spaces?cursor={cursor}&limit={n}`

The spaces the group is a member of: `{"items": [{"id", "name", "role"}], "next": "…"}`,
by name. What an administrator looks at before deleting a group.

### `GET /api/admin/audit?category=&kind=&user=&actor=&capacity=&from=&to=&tenant=&cursor=&limit=`

The audit log (see "Audit log" under "Signing in"), newest first:

```json
{"items": [{"id": "01JD3P...", "at": "2026-11-23T08:30:00Z", "kind": "lockout", "category": "security", "capacity": null,
            "actor": null, "user": null, "username": "kazuha", "ip": "203.0.113.7", "detail": {"scope": "ip_account", "minutes": 15}}],
 "next": "01JD3O..."}
```

The filters combine, and each is answered by an index that ends in the row's
id:

- `category`: one or more (repeat it, or separate with commas) of `security`,
  `admin`, `data`, `system`, `signins`, `reads`, `downloads`, `api`; another
  word is `400 invalid`.
- `kind`: one kind.
- `user`: the id of the person a row is about; `actor`: the one who acted.
- `capacity`: `site_admin` or `tenant_admin`.
- `from`, `to`: an RFC 3339 time, or a day (`2026-11-23`; a day in `to`
  includes that day). Turned into a range of ids, since an id starts with its
  time.
- `tenant`: a site administrator narrows to one tenant; a tenant's
  administrators get their own only (another is `404`).

`limit` is 1 to 200 (default 50). One query per page (one per category when
several are asked, merged by id).

### `GET /api/admin/audit/kinds`

The registry of kinds, for the filters of the web:

```json
{"categories": [{"id": "security", "core": true, "setting": null, "on": true,
                 "kinds": [{"name": "login_failed", "site": false}]},
                {"id": "reads", "core": false, "setting": "audit.reads", "on": false, "kinds": []}]}
```

`on` says whether an optional category is written in the caller's scope (the
tenant's value, or the site's for a site administrator without `?tenant=`).
`site` marks a kind only the server's administrators see; a tenant's
administrators are not given those.

### The sign-in policy of a tenant

`signin.mode`, `signin.passkeyRoles`, `sessions.idleDays`, `sessions.maxDays`
and `captcha.contexts` are site and tenant settings (decided 2026-11-28): the
organisation decides inside what the site allows, with the lower modes of
"Forced, minimum, default". Written for an organisation with
`PUT /api/admin/tenants/{tenant}/settings/{key}` (its administrators, or a site
administrator), and listed in `GET /api/settings`. What takes the server-wide
decisions before any tenant is known stays the site's alone: `signin.lockout.*`,
`signin.perMinute`, `.maxFailures`, `.windowMinutes` (a stricter value of a
tenant would tell which tenant a username is in), `captcha.provider` and its
keys, and `features.passkeys`.

| Key | Order of strictness | Default rule of the site |
|---|---|---|
| `sessions.idleDays`, `sessions.maxDays` | `lower`: fewer days is stricter | `tighten` |
| `signin.passkeyRoles` | `superset`: more roles is stricter | `tighten` |
| `captcha.contexts` | `document`: see below | `tighten` |
| `signin.mode` | none: `local_only` is different from `sso_redirect`, not more or less strict | `allow` |

The site may switch any of them to `allow`, `restrict` (a mode has `options`),
`lock` or, where there is an order, `tighten`; `signin.mode` cannot be
`tighten`ed (`400 invalid`). A tenant's value looser than the site's under
`tighten` is `409 setting_restricted` when written, and kept but not in effect
when the site became stricter after it.

- **Sessions.** The lifetimes that apply to a session are those of its person's
  own tenant (`users.tenant_id`), when it is created (the cookie's `Max-Age`)
  and on every request. They are read from the settings' cache, two lookups
  and no query, so the check costs what it did. A tenant's idle time is never
  more than its lifetime when read (a tenant that shortens only `maxDays` is
  not refused; the site's own two are held to each other, `400 invalid`). A
  site administrator, in whatever organisation, follows the site's lifetimes
  and passkey roles, as for `ssoOnly`. A session older than
  its tenant's `maxDays` or unused for longer than its `idleDays` ends with
  `reason: "expired"`, the first time it is used. The daily sweep deletes
  expired sessions by their tenant's cutoffs.
- **`signin.passkeyRoles`.** A person must hold a passkey when their role is in
  their tenant's list. A tenant's list holds every role of the site's (looser is
  `409`) and may add the others; without a value it is the site's. A non-empty
  list needs passkeys to work at all (`features.passkeys` on and an `https`
  `--public-url`), else `400 passkey_unavailable`. The person's
  `passkeyRequired` (`GET /api/users/{id}`) says the tenant's requirement too.
- **`captcha.contexts`.** A document of the three forms an organisation may
  ask a captcha of, `login`, `forgot` and `register`, each `{"on", "when",
  "failures"}` as at the site; a partial document is merged over the
  organisation's own and written whole. `invite` and `setup` are the site's
  (the invitation is found only after the captcha is asked, and there is no
  tenant at setup): naming one is `400 invalid`. A form is stricter when it is
  on, asks `always` (rather than `after_failures`), or asks after fewer
  failures; a value that is looser than the site's in any form is looser as a
  whole. A third-party provider needs the site's keys (`400
  captcha_site_key_required`, `captcha_needs_public_url`).
  **Which tenant.** The forms are answered before the person is known, so the
  tenant is the one the request is scoped to, never one worked out from what
  the person typed: `tenant` (a slug) in the body of `POST /api/login`,
  `POST /api/register` and `POST /api/password/forgot`, and `?tenant=` of
  `GET /api/captcha`. The default tenant is a tenant like any other (its
  value is the site's unless it has one): `register` and `forgot` without a
  `tenant` are its, as everywhere. **A request that names a tenant is asked
  the stricter of the site's own value and that tenant's, never less than the
  site's, even where the site's rule is `allow`** (the tenant named is not
  tied to the account signing in). **When the tenant is unknown, the site's
  value is used:** a `login` or `GET /api/captcha` that names no tenant, the
  invitation and the first-run setup. So a sign-in without `tenant` is no
  stricter than the site's, and the web app's page for an organisation
  passes it.
- **`signin.mode`.** The mode of the sign-in page opened for the organisation
  (`loginMode` of `GET /api/auth/providers?tenant=`): `local_only` while no
  provider is enabled at all, else the organisation's. It shapes the page and
  nothing else: `signin.ssoOnly` is enforced as before, and an organisation's
  `local_only` never lifts the site's `ssoOnly` for it (the page then shows the
  site's mode). Without `?tenant=` the page is the site's.

`petalbook security reset` writes the site's rows only: an organisation's own
values are kept.

### `petalbook security reset --data DIR`

The command-line rescue for settings that keep everyone out: sets the login
mode to `dual`, `ssoOnly` off, lockout off, every captcha context off and
`passkeys.requireRoles` to `[]`: it writes the rows of `signin.ssoOnly`,
`signin.mode`, `signin.lockout.enabled`, `captcha.contexts` and
`signin.passkeyRoles`, and nothing else. Providers, people and per-person
requirements are untouched (`petalbook user passwd` sets a password). A
running server sees it within 60 seconds (the settings' revision); it is
recorded in the audit log as `setting_changed` with `actor` `null` and
`detail.by` `"cli"`.

### Machine access

#### The settings `features.tokens` and `features.mcp`

Two booleans, on by default, with a site layer and a tenant layer
(`PUT /api/admin/settings/{key}` for the site's,
`PUT /api/admin/tenants/{tenant}/settings/{key}` for a tenant's, which a site
rule or lock can force: "Settings by layer"). `GET /api/admin/access` and `PUT
/api/admin/access` are gone (`404`).

`features.tokens`: API tokens of the people of the tenant are accepted (off:
every API token of the tenant answers `403 tokens_off`; nothing is deleted, and
turning it on again brings them back). It is known once the token's owner is,
so an unknown token is `401 token_invalid` whatever the settings say. OAuth
grants are not API tokens: they keep working on `/mcp` until `features.mcp` is
switched off for the tenant.

`features.mcp`: `/mcp` is served to the tenant's people (off: `404 not_found`
after the credential is read; grants are kept). The authorization server's own
endpoints (`/.well-known/…`, `/oauth/…`) answer `404` while the site's own value
is off. Both are read from the settings' cache, so no request costs a query for
them.

#### `GET /api/admin/tokens?user={id}&cursor={cursor}&limit={n}`

Every token on the server, newest first, revoked and expired ones still kept
included: `{"items": [Token + "owner": {"id", "username", "name"}], "next": "…"}`.
`user` keeps one user's. `limit` 1–200, default 50; one query a page.

#### `DELETE /api/admin/tokens/{id}`

Revokes any token. `204`, also when it was revoked already; `404 not_found`.

#### `GET /api/admin/grants?user={id}&cursor={cursor}&limit={n}`, `DELETE /api/admin/grants/{id}`

The same for OAuth grants (items as in `GET /api/account/grants`, with
`owner`).

### Connections

#### `GET /api/admin/providers`

Site administrators. `{"providers": [{"id": "github", "type": "github", "name": "GitHub", "host": "github.com", "configured": true, "partialSync": "yes", "connections": 3, "spaces": 4}, {"id": "git", …}]}`:
every provider the server has, with its configuration state and counts (people
connected, spaces syncing through it). A tenant administrator gets
`403 forbidden`. `linkCallback` is where a host's application sends people back
to, with `{id}` standing for the provider's id
(`https://notes.example.com/api/account/links/{id}/callback`, `""` without
`--public-url` or for who does not manage connections): the form that adds a
host shows it before the host has an id.

#### The GitHub App

The server's one GitHub App (see "The GitHub provider"). It is in the state database, not
in a flag or a file, and a change takes effect at once, without a restart. The
private key and the client secret are sealed with the server's key
(`DATA/secret.key`, or `PETALBOOK_SECRET_KEY`), are **never returned**, and
never reach a log or an error message. The App's webhook (see "The GitHub
provider", "Webhook") tells the server who installed each installation, and
when one is uninstalled; its secret is sealed like the others. The
administrator either makes the App on GitHub from here in one click (the
manifest flow below) or makes it by hand and copies its details here.

The App needs these permissions on GitHub: Contents read and write (Git),
Metadata read, and Administration read and write (creating repositories;
without it, spaces can only be connected to repositories that exist).
"Request user authorization (OAuth) during installation" should be ticked, so
that installing on one's own account and authorising it are one step, and the
App must be installable on **any account** (public), or a person cannot install
it on their own account: `anyAccount` below says which it is. Its callback URL
is the `callbackUrl` below. Its webhook is active, at `webhookUrl` below, with a
secret entered here too (`webhookSecret`), and subscribes to nothing more than
GitHub sends every App (the installation events); without it an organisation's
installation cannot be connected.

#### `GET /api/admin/providers/github`

```json
{"configured": true, "appId": "1234567", "clientId": "Iv23liAbCdEfGhIjKlMn",
 "slug": "petalbook-notes", "name": "PetalBook Notes", "owner": "kazuhahub",
 "htmlUrl": "https://github.com/apps/petalbook-notes",
 "installUrl": "https://github.com/apps/petalbook-notes/installations/new",
 "privateKey": {"set": true, "fingerprint": "SHA256:…"}, "clientSecret": {"set": true},
 "webhookSecret": {"set": true}, "webhookUrl": "https://notes.example.com/api/github/webhook",
 "callbackUrl": "https://notes.example.com/api/account/links/github/callback",
 "permissions": {"administration": "write", "contents": "write", "metadata": "read"},
 "missing": [], "gaps": [], "anyAccount": true,
 "userAuthorization": true, "checked": "2026-11-23T08:30:00Z",
 "spaces": 4, "authorizations": 3}
```

Before anything is saved: `{"configured": false, "callbackUrl": "…", "webhookUrl": "…"}`.

- `slug`, `name`, `owner` and `permissions` are what GitHub said about the App
  (`GET /app`) when it was last saved, at `checked`. `htmlUrl` is the App's
  public page; `installUrl` is where an organisation or a user installs it.
- `privateKey.fingerprint` is the SHA-256 fingerprint of the public half, to
  compare with what GitHub shows for the App's keys.
- `missing` lists the permissions above that the App lacks (`"administration"`,
  `"contents"`, `"metadata"`). It is a warning: saving is not refused.
- `gaps` lists, from the last check, the installations that lack one of them
  (an App whose permissions grew, installed where nobody has accepted that yet):
  `[{"installationId": 51234567, "account": "kazuhahub", "missing": ["administration"]}]`,
  at most 20. The owner accepts the new permissions in the installation's
  settings on GitHub.
- `anyAccount` says whether the App can be installed on any account, which a
  person connecting their own account needs (the App must be public: "Any
  account" in its settings). It is what GitHub says about the App when it does,
  else `true` once an installation on an account that is not the App owner's has
  been seen, else `null` (not known). A client warns on `false`.
- Everything above is as of `checked`: it is read from GitHub when the App is
  saved, by `POST /api/admin/providers/github/check`, and, at most once a minute, when a
  token GitHub refuses (for a permission the App or an installation lacks)
  shows the stored view is old. A plain `GET` never asks GitHub.
- `callbackUrl` is `<public-url>/api/account/links/github/callback` and
  `webhookUrl` `<public-url>/api/github/webhook`, `""` without
  `--public-url`. `webhookSecret.set` says whether the webhook's secret is kept. `userAuthorization` is as in `GET /api/account/links`.
- `spaces` counts the spaces with a `github` credential; `authorizations` the
  users connected.

#### `PUT /api/admin/providers/github`

```json
{"appId": "1234567", "privateKey": "-----BEGIN RSA PRIVATE KEY-----\n…",
 "clientId": "Iv23liAbCdEfGhIjKlMn", "clientSecret": "…", "webhookSecret": "…"}
```

The first save needs `appId` and `privateKey`. Later, a `privateKey`, a
`clientSecret` or a `webhookSecret` that is left out keeps the one stored (the
webhook secret is not checked with GitHub: a wrong one makes every delivery
`401`). `clientId` and
`clientSecret` go together; without them users cannot connect their GitHub
accounts or install the App here. `"clientId": ""` removes
both (`409 provider_in_use` while a user is connected; spaces that sync
through an installation do not stand in the way).

The server checks with GitHub before it saves anything, and saves nothing when
a check fails:

- the key is an RSA key in PEM (PKCS#1 or PKCS#8): else `400 key_invalid`;
- GitHub accepts a JWT signed with it for `appId` (`GET /app`): else
  `400 app_rejected`;
- `clientId` is that App's client id: else `400 client_rejected`;
- GitHub accepts the client id and secret (it is asked to check a made-up
  token with them, `POST /applications/{client_id}/token`: `404` means the
  pair is right, `401` that it is not): else `400 client_rejected`;
- GitHub cannot be reached: `503 unavailable`.

A new key or secret for the same App replaces the old one and drops the
cached tokens. Another App (`appId` different from the saved one) is refused
with `409 provider_in_use` while any space has a `github` credential or any
user is connected; another or no `clientId` only while any user is connected.
A GitHub sign-in provider (the `github` row) set up for another GitHub host is
not taken over while accounts are linked to it: their links name accounts of
that host by GitHub's numeric ids, which an account on the App's host could
share. `409 provider_host_mismatch` then (`message` names both hosts): delete
that sign-in provider, or remove its linked accounts, first.
`200` as `GET`.

#### `POST /api/admin/providers/github/check`

No body. Reads the App (`GET /app`) and up to five pages of its installations
(`GET /app/installations`) from GitHub again, stores what the permissions are
now (`permissions`, `missing`, `gaps`, `anyAccount`, `checked`, and the App's
name and page) and answers `200` as `GET /api/admin/providers/github`. `409
provider_not_configured` without an App, or when GitHub no longer accepts its key;
`503 unavailable` when GitHub cannot be reached (nothing is changed).

#### `POST /api/admin/providers/github/manifest`

Makes the App on GitHub in one click (GitHub's App Manifest flow); entering an
App by hand with `PUT` stays possible. `{"organization": "kazuhahub", "name":
"PetalBook Notes"}`, both optional: with `organization` the App is made in that
organisation (else in the administrator's own account), `name` is at most 34
characters (default `PetalBook <host of --public-url>`; GitHub's names are
unique, so the administrator may need another). `200`:

```json
{"url": "https://github.com/organizations/kazuhahub/settings/apps/new?state=…",
 "manifest": {"name": "PetalBook Notes", "url": "https://notes.example.com",
   "hook_attributes": {"url": "https://notes.example.com/api/github/webhook", "active": true},
   "redirect_url": "https://notes.example.com/api/providers/github/manifest/callback",
   "callback_urls": ["https://notes.example.com/api/account/links/github/callback"],
   "setup_url": "https://notes.example.com/api/providers/github/manifest/callback",
   "public": true, "request_oauth_on_install": true,
   "default_permissions": {"administration": "write", "contents": "write", "metadata": "read"}}}
```

and a short-lived cookie that ties the attempt to this browser. The client
posts the manifest to `url` from the administrator's browser, as a form with one
field, `manifest`, holding the JSON (`/settings/apps/new` without
`organization`). The App is public ("Any account"), asks for user authorisation
during installation, and has its webhook on, at `/api/github/webhook`.

The `state` in `url` is 32 random bytes, held in memory for ten minutes, used
once, bound to the cookie and to the signed-in site administrator. `409
public_url_needed` without `--public-url`; `409 provider_in_use` while
any space has a `github` credential or any user is connected (an App in use is
not replaced); `400 invalid` for a bad `organization` or `name`; `403
forbidden` for anyone but a site administrator.

#### `GET /api/providers/github/manifest/callback`

Where GitHub sends the administrator's browser after the App is made, with
`code` and `state`. Like the callback of a user's connection it answers a
redirect (`303`), never JSON, needs the cookie and the session of the
administrator who started the attempt, and is a cross-site navigation by nature.
The server trades the code (`POST /app-manifests/{code}/conversions`, which needs
no other credential and works once) for the App's id, slug, client id, client
secret, private key and webhook secret, then saves them as `PUT
/api/admin/providers/github` does (checked with GitHub, secrets sealed with the server's
key; the webhook secret too, which signs the webhook's deliveries), and
sends the browser to `/admin/storage/connections?connection_app=created`. A failure goes to
`/admin/storage/connections?connection_app_error=<code>`: `expired` (no such attempt, used, older
than ten minutes, another browser or user), `provider_error` (GitHub did not turn
the code into an App, or the App could not be saved), `app_in_use`,
`unavailable` (GitHub made the App but could not be asked to check it, so it is
not saved yet; see the next route). A visit with `installation_id` or `setup_action` and no `code`
(the manifest names this address as the post-installation page too) goes to
`/settings/connections`.

#### `POST /api/admin/providers/github/manifest/retry`

When GitHub made the App but the server could not save it (`unavailable`: GitHub
could not be asked to check it), the redirect is
`/admin/storage/connections?connection_app_error=unavailable&pending=1&app=<slug>`: the App
exists on GitHub and its credentials are kept in the server's memory, sealed,
for ten minutes, for that administrator only (one pending App per administrator,
the newest). The retry takes no body, saves them as the callback would, and
answers `200` as `GET /api/admin/providers/github`. `404 not_found` when
nothing is pending (none was made, it was used or ten minutes passed: the
administrator then finds the App at `https://github.com/apps/<slug>` (or in the
organisation's settings), makes a private key there and enters the details with
`PUT`; `app` in the redirect names it); `503 unavailable` when GitHub still
cannot be reached (the credentials stay pending); `409 provider_in_use` as
elsewhere. The redirect has no `pending` when nothing could be kept.

#### `DELETE /api/admin/providers/github`

`204`. The App's details are deleted, the users' connections with them
(GitHub is asked to revoke each grant first, best effort), and what was shared of
the installations. `409 provider_in_use` while any space has a `github`
credential (`message` says how many): switch or disconnect those first. Also
`409 provider_in_use` while `remotes.require` of the storage settings names the
provider (`message` says to change the requirement first), because nobody could
meet the requirement without it.

#### Installations

The site's administrators do not list or share the App's installations (the
routes `GET /api/admin/providers/github/installations` and `PUT …/{id}` are gone,
2026-11-26): each person and each organisation installs the App on their own
GitHub account and manages it (see "The GitHub provider", "Installations"). The
site keeps the App and the switch `features.githubConnect`.

