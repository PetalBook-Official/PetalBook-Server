// Who is signed in, and the pages around signing in: sessions, login, logout, setup, invites, providers, captcha,
// passkeys, registration, password reset, OAuth consent; and the few reads every page makes at start (meta, settings). A login signs in the person named, whatever password came; a refusal is for `fx.fail`.

import type { SessionInfo, SessionUser, SignInAnswer } from '../../types.ts'
import type { AuthData } from '../data/model.ts'
import { DEMO_ADMIN, DEMO_MEMBER, type UserRec } from '../data/people.ts'
import { add, newId, Refusal, reply, rfc3339, type Ctx, type Route } from '../kit.ts'

export const view = ({ password: _p, passkeys: _k, ...user }: UserRec): SessionUser => (void _p, void _k, user)
export const current = (a: AuthData) => a.users.find((u) => u.id === a.signedIn)

// What every sign-in answers: the person with their permissions, and the hold on the session.
function signIn(a: AuthData, u: UserRec): SignInAnswer {
  a.signedIn = u.id
  a.ended = undefined
  return { user: view(u), hold: a.hold }
}

// A new account takes after the demo person of its role: what that role holds is declared in data/people.ts.
const BASE = { admin: DEMO_ADMIN, member: DEMO_MEMBER }
function accountFrom(c: Ctx, role: keyof typeof BASE): UserRec {
  const { username, name, email = '', password = '' } = c.body
  return {
    ...BASE[role],
    id: newId(c.data, '01FU'),
    username,
    name: name || username,
    email,
    password,
    siteAdmin: false,
  }
}

export const authRoutes: Route[] = [
  [
    'GET',
    '/api/session',
    (c): SessionInfo => {
      const a = c.data.auth
      const user = current(a)
      return {
        mode: a.mode,
        user: user ? view(user) : null,
        setup: a.setup,
        hold: user ? a.hold : null,
        ...(a.ended && !user ? { endedReason: a.ended } : {}),
        ...(a.mode === 'accounts' ? { methods: { password: true } } : {}),
        token: null,
      }
    },
  ],
  [
    'POST',
    '/api/login',
    (c) => {
      const a = c.data.auth
      const u = a.users.find((x) => x.username === c.body.username)
      if (!u) throw new Refusal(401, 'unauthenticated', 'wrong username or password')
      return signIn(a, u)
    },
  ],
  [
    'POST',
    '/api/logout',
    (c) => {
      c.data.auth.signedIn = null
    },
  ],
  [
    'POST',
    '/api/setup',
    (c) => {
      const a = c.data.auth
      const u = add(a.users, accountFrom(c, 'admin'))
      a.setup = false
      return signIn(a, u)
    },
  ],
  [
    'POST',
    '/api/invites/check',
    (c) => {
      const inv = c.data.auth.invite
      if (!inv || inv.token !== c.body.token) throw new Refusal(404, 'not_found', 'the invite is not valid')
      return { role: inv.role, expires: '2099-01-01T00:00:00Z', username: '', name: '', email: '', ...inv.fill }
    },
  ],
  [
    'POST',
    '/api/invites/accept',
    (c) => {
      const a = c.data.auth
      return reply(201, signIn(a, add(a.users, accountFrom(c, c.data.auth.invite?.role ?? 'member'))))
    },
  ],
  [
    'GET',
    '/api/auth/providers',
    (c) => {
      const a = c.data.auth
      const routed = Object.keys(a.ssoDomains).length > 0
      return {
        providers: a.providers.map((p) => ({ slug: p.slug, kind: p.kind, name: p.name, icon: p.icon ?? '' })),
        loginMode: a.loginMode,
        local: true,
        sso: a.providers.length > 0,
        passkey: a.passkeys,
        register: a.register,
        forgot: a.forgot,
        orgCreation: a.orgCreation,
        identifierFirst: routed && (a.loginMode === 'dual' || a.loginMode === 'sso_first'),
        ssoEntry: routed && a.loginMode !== 'local_only',
      }
    },
  ],
  [
    'POST',
    '/api/auth/discover',
    (c) => {
      const email = String(c.body.email ?? '').trim()
      const at = email.lastIndexOf('@')
      return { sso: at < 0 ? null : (c.data.auth.ssoDomains[email.slice(at + 1).toLowerCase()] ?? null) }
    },
  ],
  ['POST', '/api/auth/:kind/:slug/start', (c) => ({ url: `/api/auth/${c.params.kind}/${c.params.slug}/callback` })],
  [
    'GET',
    '/api/captcha',
    (c) => (c.data.auth.captcha ? { required: true, ...c.data.auth.captcha } : { required: false }),
  ],
  [
    'POST',
    '/api/auth/passkey/begin',
    () => ({
      ceremony: 'ceremony-login',
      options: { challenge: 'Y2hhbGxlbmdl', rpId: 'localhost', timeout: 300000, userVerification: 'required' },
    }),
  ],
  [
    'POST',
    '/api/auth/passkey/finish',
    (c) => {
      const a = c.data.auth
      const u = a.users.find((x) => x.passkeys) ?? a.users[0]
      return { ...signIn(a, u), return: c.query.get('return') ?? '/' }
    },
  ],
  // registration, password reset, email links
  [
    'GET',
    '/api/register',
    (c) => ({
      open: c.data.auth.register,
      mode: c.data.auth.registerMode,
      orgCreation: { open: c.data.auth.orgCreation },
      domainJoin: Object.keys(c.data.auth.joinDomains).length > 0,
      ...(c.data.auth.termsUrl ? { termsUrl: c.data.auth.termsUrl } : {}),
    }),
  ],
  [
    'POST',
    '/api/register',
    (c) => {
      // An address of a domain whose organisation requires its single sign-on signs up there (3.6 d4).
      const email = String(c.body.email ?? '')
      const sso = c.data.auth.ssoDomains[email.slice(email.lastIndexOf('@') + 1).toLowerCase()]
      if (sso?.enforced)
        throw new Refusal(409, 'sso_required', 'this organization signs its people in with single sign-on', { sso })
      // An address of a domain that joins its organisation registers there, open or not (3.6 d5).
      const join = c.body.org
        ? undefined
        : c.data.auth.joinDomains[email.slice(email.lastIndexOf('@') + 1).toLowerCase()]
      c.data.sent.mail.push({
        to: c.body.email,
        kind: c.body.org
          ? 'register_org'
          : join
            ? join.mode === 'request'
              ? 'register_join_request'
              : 'register_join'
            : 'register',
      })
      return reply(202, { sent: true })
    },
  ],
  [
    'POST',
    '/api/register/confirm',
    (c) => {
      const a = c.data.auth
      // A sign-up that asked to join waits for approval (3.6 d5): its token shows the request.
      const waiting = a.joinWaiting[String(c.body.token)]
      if (waiting) {
        if (waiting.state !== 'pending' || waiting.decidedAt)
          throw new Refusal(404, 'not_found', 'That link is not valid.')
        return reply(202, { request: waiting })
      }
      return signIn(a, add(a.users, accountFrom(c, 'member')))
    },
  ],
  [
    'POST',
    '/api/register/request',
    (c) => {
      const s = c.data.auth.joinWaiting[String(c.body.token)]
      if (!s) throw new Refusal(404, 'not_found', 'That link is not valid, or has expired.')
      return s
    },
  ],
  [
    'POST',
    '/api/register/request/withdraw',
    (c) => {
      const s = c.data.auth.joinWaiting[String(c.body.token)]
      if (!s) throw new Refusal(404, 'not_found', 'That link is not valid, or has expired.')
      if (s.state !== 'pending') throw new Refusal(409, 'join_decided', 'This request is no longer waiting.')
      s.state = 'withdrawn'
      s.decidedAt = rfc3339(c.now())
      return s
    },
  ],
  [
    'POST',
    '/api/password/forgot',
    (c) => {
      c.data.sent.mail.push({ to: String(c.body.login), kind: 'reset' })
      return reply(202, { sent: true })
    },
  ],
  ['POST', '/api/password/reset/check', () => ({ username: 'admin', expires: '2099-01-01T00:00:00Z' })],
  ['POST', '/api/password/reset', () => undefined],
  ['POST', '/api/email/confirm', () => undefined],

  // OAuth consent
  [
    'GET',
    '/api/oauth/consent',
    (c) => {
      const info = c.data.auth.consents[c.query.get('req') ?? '']
      if (!info) throw new Refusal(404, 'not_found', 'no such request')
      return info
    },
  ],
  [
    'POST',
    '/api/oauth/consent',
    (c) => ({
      redirect: c.body.approve ? 'https://client.example/cb?code=ok' : 'https://client.example/cb?error=access_denied',
    }),
  ],

  // what every page reads at start
  [
    'GET',
    '/api/meta',
    (c) => ({
      version: 'fixture',
      maxUpload: 10 << 20,
      maxImport: 100 << 20,
      spaces: c.data.spaces.map((s) => s.info),
    }),
  ],
  [
    'GET',
    '/api/settings',
    (c) => ({
      features: c.data.features,
      // Where a new space may be stored: everything is allowed, the server is the default, and the organisation has no
      // storage target yet (docs/api.md, "Storage modes").
      settings: [
        {
          key: 'storage.mode',
          effective: 'server',
          allowed: ['server', 'own', 'tenant'],
          locked: false,
          missing: ['tenant'],
        },
      ],
    }),
  ],
]
