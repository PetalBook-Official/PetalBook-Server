// Typed functions for every endpoint of docs/api.md.

import { t } from '@/i18n'
import { apiErrorText, type ErrorParams } from '@/i18n/apiErrors'
import { featureName } from '@/i18n/featureNames'
import { formatBytes } from '@/lib/bytes'
import { noteNetworkOk } from '@/lib/offline/status'
import { forgetLastAccount } from '@/lib/lastAccount'
import { scopeHeaderFor, scopeOfRequest } from '@/lib/adminScope'
import { viewAsHeaders, viewAsId } from '@/lib/viewAs'

import { ENDED_REASONS } from './types'
import type {
  SignInAnswer,
  RegisterInfo,
  JoinStatus,
  RegisterConfirmAnswer,
  RegisterInput,
  MySettings,
  EffectiveMemberList,
  GroupMember,
  GroupRef,
  MembersResponse,
  AccountInfo,
  AccountUsage,
  AdminSpace,
  ApiToken,
  ConsentDecision,
  ConsentInfo,
  CreatedToken,
  Grant,
  TokenAccess,
  BacklinkList,
  DailyAnswer,
  DailyOptionChoice,
  DailyPrefs,
  DailySpaceSource,
  DailySettings,
  FavoriteList,
  FavoriteRef,
  CatalogBatch,
  ChangeList,
  CreatePageInput,
  SyncHeads,
  SyncPages,
  DraftInput,
  DraftReceipt,
  BatchPagesInput,
  AssetsRemote,
  ConnectAssetsInput,
  ConnectRemoteInput,
  AuthProviders,
  DiscoveredSSO,
  Link,
  CaptchaAnswer,
  CaptchaChallenge,
  CaptchaContext,
  ConnectionOwner,
  ConnectionRepo,
  CursorPage,
  NewSync,
  Provider,
  EndedReason,
  ErrorCode,
  Passkey,
  PasskeyCeremony,
  ProviderIntent,
  ProviderKind,
  ReasonCode,
  HistoryList,
  ImportReport,
  InviteCheck,
  ListParams,
  Member,
  MetaResponse,
  Page,
  PageDraft,
  PageLinkList,
  PageList,
  ChecksParams,
  ChecksResponse,
  FixCheckInput,
  NotificationList,
  NotificationsParams,
  PrefsAnswer,
  PageTemplate,
  PutTemplateInput,
  TemplateDefaults,
  TemplatesResponse,
  PatchPropsInput,
  PageVersion,
  Remote,
  RemoteKey,
  SavePageInput,
  SessionInfo,
  SpaceInfo,
  SpaceRole,
  Appearance,
  FolderStyle,
  IconColor,
  SourceCount,
  TagColor,
  TagsResponse,
  TypeInput,
  PageType,
  FieldRename,
  TypesResponse,
  FolderBinding,
  GroupsParams,
  GroupsResponse,
  PutViewsInput,
  ViewList,
  ViewScope,
  TreeResponse,
  UploadedAsset,
  User,
} from './types'

export const API_BASE = '/api'
// Single-user mode has exactly one space, with this id.
export const DEFAULT_SPACE = 'default'

// The codes the daily-text endpoints put in `error` (with what they are about next to it).
const DAILY_CODES = new Set([
  'bad_language',
  'api_not_found',
  'builtin',
  'header_reenter',
  'header_missing',
  'not_json',
  'too_large',
  'path_error',
  'path_reveals_secret',
  'link_uses_header',
  'skip_too_large',
])
const reasonOf = (b: { error: string; header?: unknown; field?: unknown }): ReasonCode => ({
  code: b.error,
  ...(typeof b.header === 'string' ? { header: b.header } : {}),
  ...(typeof b.field === 'string' ? { field: b.field } : {}),
})

// The `reason` of a 401: one of the four, or nothing.
const endedReasonOf = (v: unknown): EndedReason | undefined => ENDED_REASONS.find((r) => r === v)

export class ApiError extends Error {
  readonly status: number
  readonly code: ErrorCode
  // Present on a 409 conflict: the page as the server has it now.
  readonly current?: Page
  // On a draft 409: the caller's draft as the server has it, or null when there
  // is none. Undefined when the answer did not carry one.
  readonly draft?: PageDraft | null
  // Present on 429: seconds the server asks us to wait.
  readonly retryAfter?: number
  // The stable reason a daily-text refusal carries next to `error`, with what it is about; absent otherwise.
  readonly reason?: ReasonCode
  // The parsed answer, when it had a body: some refusals carry more than a code (the daily trial's `result`).
  readonly body?: unknown
  // What the server said, in its own words (English, for API users). `message` is the text for people, in the language
  // of the page, chosen by `code`; it is this only when the client has no text for the code.
  readonly serverMessage?: string
  constructor(
    status: number,
    code: ErrorCode,
    message: string,
    current?: Page,
    retryAfter?: number,
    draft?: PageDraft | null,
    reason?: ReasonCode,
    body?: unknown,
    serverMessage?: string,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.current = current
    this.retryAfter = retryAfter
    this.draft = draft
    this.reason = reason
    this.body = body
    this.serverMessage = serverMessage
  }
}

const str = (v: unknown): string | undefined => (typeof v === 'string' && v ? v : undefined)
const count = (v: unknown): string | undefined => (typeof v === 'number' && Number.isFinite(v) ? String(v) : undefined)

// What an error answer is about, written for people: the fields the server puts next to `error` (docs/api.md), and
// Retry-After as a wait.
export function errorParams(code: string, body: unknown, retryAfter: number | undefined): ErrorParams {
  const b = (body ?? {}) as Record<string, unknown>
  const bytes = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? formatBytes(v) : undefined)
  const size = code === 'file_too_large'
  return {
    limit: size ? bytes(b.limit) : count(b.limit),
    used: count(b.used),
    size: bytes(b.size),
    name: str(b.name),
    provider: str(b.provider),
    host: str(b.host),
    permission: str(b.permission),
    header: str(b.header),
    field: str(b.field),
    quota: str(b.quota),
    feature: featureName(str(b.feature)),
    spaces: count(b.spaces),
    accounts: count(b.accounts),
    wait: retryAfter ? t.auth.wait(retryAfter) : undefined,
  }
}

// --- the copy on this device (milestone 4.1) ---------------------------------------------
//
// When a read cannot reach the server, the copy of the space on this device answers it, if there is one
// (src/lib/offline). The module that does so registers itself here, so this one needs nothing from it; and when the
// person signs out, it is told whose copy to delete.

export interface OfflineHooks {
  // Answers a GET (`path` is without the /api prefix) from the device; undefined when it does not. May throw an
  // ApiError (a page that was not kept).
  read?: (user: string | null, path: string) => Promise<unknown>
  // Called after a sign-out with the id of the person who signed out, to delete their copies.
  signedOut?: (user: string) => Promise<void>
  // Called when a request was answered from the device, not by the server.
  answered?: () => void
}
let offlineHooks: OfflineHooks = {}
export function setOfflineHooks(h: OfflineHooks) {
  offlineHooks = h
}
// Who is signed in, as far as this tab has seen: the copies on the device are theirs.
let knownUser: string | null = null
export const currentUserId = () => knownUser
export function rememberUser(id: string | null) {
  knownUser = id
}

// 403 `forbidden` normally means the server will not talk to this page's
// address (its allowed origins do not include it). The generic message would
// send people looking for a save bug, so say what is wrong and show where we are.
export function forbiddenMessage(origin: string): string {
  return t.errors.forbidden(origin)
}

type ForbiddenListener = (message: string) => void
const forbiddenListeners = new Set<ForbiddenListener>()
// The app registers one to show a persistent notice, whichever request hit it.
export function onForbidden(fn: ForbiddenListener): () => void {
  forbiddenListeners.add(fn)
  return () => void forbiddenListeners.delete(fn)
}

// --- being signed out --------------------------------------------------------
//
// The first 401 on any request other than the sign-in ones tells the app once
// that the session is gone. From then until someone signs in again, requests
// that need a session fail on the spot with the same error instead of going to
// the server: several queries failing together make one redirect, not a storm,
// and an editor that leaves in the meantime does not hammer the server with
// saves that cannot work. Nothing here touches local drafts.

// `reason` is why, as the 401 said it; none when it said nothing (see `markSignedOut`).
type SignedOutListener = (reason: EndedReason | undefined) => void
const signedOutListeners = new Set<SignedOutListener>()
export function onSignedOut(fn: SignedOutListener): () => void {
  signedOutListeners.add(fn)
  return () => void signedOutListeners.delete(fn)
}
let signedOut = false
export const isSignedOut = () => signedOut
// Called when a sign-in succeeded or the session shows a user.
export function markSignedIn() {
  signedOut = false
}
// `reason` is the one the 401 carried. None given means the server keeps no record of the cookie (signed out on
// purpose elsewhere, a deleted account, a record past its 30 days): no reason is made up, and the sign-in page is the
// plain one.
function markSignedOut(reason: EndedReason | undefined) {
  if (signedOut) return
  signedOut = true
  signedOutListeners.forEach((fn) => fn(reason))
}

// --- a session held until a passkey is registered ---------------------------------------
//
// While `GET /api/session` has `hold: "passkey"` every other request answers 403
// `passkey_required`. The app learns of it once, from whichever request hit it, and
// shows the passkey page (a hold that began after the session was read).

type HoldListener = (hold: 'passkey' | 'password') => void
const holdListeners = new Set<HoldListener>()
export function onHold(fn: HoldListener): () => void {
  holdListeners.add(fn)
  return () => void holdListeners.delete(fn)
}

// Paths where a 401 is an answer to the request itself (wrong password), not
// a lost session.
const SIGN_IN_PATHS =
  /^\/(login|logout|setup|session|meta|captcha|invites\/(check|accept)|auth\/(providers|passkey\/(begin|finish)|oidc\/start|(oidc|saml|github)\/[^/]+\/start))$/
// Paths where 403 is an answer about the request itself (a wrong setup token,
// a wrong current password, a member asking for admin things), not a refused
// origin. Page and space endpoints have no such 403: a space of someone else
// is a 404.
// The remote endpoints answer 403 for a host the administrator did not allow.
// The administration and the members of a space answer the same way.
// A member whose role lacks the permission a route needs gets 403 `no_permission`
// from any space endpoint: its own code, so it is never taken for a refused origin
// and no path has to be listed for it. What is listed here answers `forbidden` for
// reasons of its own (the limit on spaces, a host that is not allowed, favourites of a space the
// administrator is not a member of).
const USER_LEVEL_403 =
  /^\/(setup|account(\/.*)?|users(\/.*)?|invites(\/.*)?|admin(\/.*)?)$|^\/spaces(\/[^/]+\/(remote|favorites)(\/.*)?)?$/

// --- a remote that cannot be reached ---------------------------------------------
//
// A space that follows a remote answers every write with 503 `unavailable`
// while the remote is out of reach or paused. The app learns of it once, from
// whichever request hit it, so the sync indicator can look again.

type UnavailableListener = () => void
const unavailableListeners = new Set<UnavailableListener>()
export function onUnavailable(fn: UnavailableListener): () => void {
  unavailableListeners.add(fn)
  return () => void unavailableListeners.delete(fn)
}

// --- a view as a person ended ---------------------------------------------------------
//
// In a tab that views the app as a person, `view_as_ended` means the view is over (its hour, a restart, the key lost):
// the tab says so in place of the app.
type ViewAsEndedListener = () => void
const viewAsEndedListeners = new Set<ViewAsEndedListener>()
export function onViewAsEnded(fn: ViewAsEndedListener): () => void {
  viewAsEndedListeners.add(fn)
  return () => void viewAsEndedListeners.delete(fn)
}

// --- access to a space changed -------------------------------------------------------
//
// A request refused because an administrator switched the space off, or because
// the person's role no longer allows it, says that what the app knows about the
// space (its list entry: permissions, the disabled mark) is out of date. So does a
// 404 on a space's own address (the person was removed, the space deleted), though
// it can as well be a page that is gone: `notFound` says which kind it was, and the
// app decides how much to look again. The app is told once per refusal.

type AccessListener = (space: string | null, notFound: boolean) => void
const accessListeners = new Set<AccessListener>()
export function onSpaceAccess(fn: AccessListener): () => void {
  accessListeners.add(fn)
  return () => void accessListeners.delete(fn)
}

export const isConflict = (e: unknown): e is ApiError => e instanceof ApiError && e.code === 'conflict'
export const isUnavailable = (e: unknown): e is ApiError => e instanceof ApiError && e.code === 'unavailable'
export const isUnauthenticated = (e: unknown): e is ApiError => e instanceof ApiError && e.status === 401

// Browsers refuse keepalive requests whose body is over 64 KiB; stay under it
// with room for headers, and send larger bodies as ordinary requests.
const KEEPALIVE_LIMIT = 60_000
const byteLength = (s: string) => new TextEncoder().encode(s).length

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  keepalive?: boolean
  signal?: AbortSignal
  // A file's bytes, sent as they are (the one request that is not JSON).
  blob?: Blob
}

// Called off for taking too long (AbortSignal.timeout), as opposed to called off on purpose.
const isTimeout = (e: unknown) =>
  typeof e === 'object' && e !== null && (e as { name?: unknown }).name === 'TimeoutError'
const timedOut = (e: unknown, signal: AbortSignal | undefined) =>
  isTimeout(e) || (signal?.aborted === true && isTimeout(signal.reason))

export async function request<T>(
  path: string,
  { method = 'GET', body, keepalive, signal, blob }: RequestOptions = {},
): Promise<T> {
  // The scope is the caller's, decided now, before the first await (lib/adminScope).
  const scope = scopeOfRequest(signal)
  const barePath = path.split('?')[0]
  const isSignIn = SIGN_IN_PATHS.test(barePath)
  if (signedOut && !isSignIn) throw new ApiError(401, 'unauthenticated', t.errors.signedOut)
  // Every write carries a JSON content type, even with nothing to say: the
  // server refuses writes without one (it is what keeps other sites from
  // posting to it from a browser form).
  const isWrite = method !== 'GET'
  // A file goes out as its bytes: every fetch implementation sends those the
  // same way, which cannot be said of a Blob made by another library.
  const payload: string | ArrayBuffer | undefined = blob
    ? await blob.arrayBuffer()
    : body === undefined
      ? isWrite
        ? '{}'
        : undefined
      : JSON.stringify(body)
  let res: Response
  try {
    res = await fetch(API_BASE + path, {
      method,
      signal,
      // The session is a same-origin cookie.
      credentials: 'same-origin',
      headers: {
        Accept: 'application/json',
        // The console in a tenant's scope names it on every administration call (the one place that does), and a tab
        // that views the app as a person names its view on every call (lib/viewAs).
        ...scopeHeaderFor(path, scope),
        ...viewAsHeaders(),
        ...(payload === undefined ? {} : { 'Content-Type': blob ? 'application/octet-stream' : 'application/json' }),
      },
      body: payload,
      keepalive: keepalive && typeof payload === 'string' && byteLength(payload) < KEEPALIVE_LIMIT ? true : undefined,
    })
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError' && !timedOut(e, signal)) throw e
    // A read that cannot reach the server may be answered by the copy on this device.
    // Never in a view as a person: the copy on this device is the administrator's own.
    if (method === 'GET' && offlineHooks.read && viewAsId() === null) {
      const local = await offlineHooks.read(knownUser, path).catch((err) => {
        if (err instanceof ApiError) throw err
        return undefined
      })
      if (local !== undefined) {
        offlineHooks.answered?.()
        return local as T
      }
    }
    if (timedOut(e, signal)) throw new ApiError(0, 'timeout', t.errors.timeout)
    throw new ApiError(0, 'network', t.errors.network)
  }
  noteNetworkOk()
  if (res.status === 204) return undefined as T
  let text: string
  try {
    text = await res.text()
  } catch (e) {
    // The time ran out while the body was on its way.
    if (timedOut(e, signal)) throw new ApiError(0, 'timeout', t.errors.timeout)
    throw e
  }
  let json: unknown = null
  if (text) {
    try {
      json = JSON.parse(text)
    } catch {
      // A proxy answering with HTML when the server is down.
      throw new ApiError(res.status, 'network', res.ok ? t.errors.badResponse : t.errors.network)
    }
  }
  if (!res.ok) {
    const err = (json ?? {}) as {
      error?: string
      message?: string
      current?: Page
      draft?: PageDraft | null
      host?: unknown
      header?: unknown
      field?: unknown
      reason?: unknown
    }
    if (err.error === 'forbidden' && !USER_LEVEL_403.test(barePath)) {
      const message = forbiddenMessage(globalThis.location?.origin ?? '')
      forbiddenListeners.forEach((fn) => fn(message))
      throw new ApiError(res.status, 'forbidden', message)
    }
    if (res.status === 401 && !isSignIn) markSignedOut(endedReasonOf(err.reason))
    if (res.status === 403 && err.error === 'passkey_required' && !isSignIn)
      holdListeners.forEach((fn) => fn('passkey'))
    if (res.status === 403 && err.error === 'password_change_required' && !isSignIn)
      holdListeners.forEach((fn) => fn('password'))
    if (err.error === 'unavailable') unavailableListeners.forEach((fn) => fn())
    if (err.error === 'view_as_ended' && viewAsId() !== null) viewAsEndedListeners.forEach((fn) => fn())
    if (err.error === 'space_disabled' || err.error === 'no_permission' || err.error === 'not_found') {
      const space = /^\/spaces\/([^/]+)/.exec(barePath)?.[1]
      // Only requests inside a space say anything about the space.
      if (space || err.error !== 'not_found')
        accessListeners.forEach((fn) => fn(space ? decodeURIComponent(space) : null, err.error === 'not_found'))
    }
    const retry = Number(res.headers.get('Retry-After'))
    const wait = Number.isFinite(retry) && retry > 0 ? retry : undefined
    // The text for people is by code; only a code the client has no text for shows what the server said.
    const text = err.error ? apiErrorText(err.error, errorParams(err.error, json, wait), err.message) : undefined
    throw new ApiError(
      res.status,
      (err.error as ErrorCode) || 'unknown',
      text || err.message || t.errors.requestFailed(res.status),
      err.current,
      res.status === 429 && wait ? wait : undefined,
      err.draft,
      typeof err.error === 'string' && DAILY_CODES.has(err.error)
        ? reasonOf(err as { error: string; header?: unknown; field?: unknown })
        : undefined,
      json ?? undefined,
      err.message,
    )
  }
  return json as T
}

export function listParamsToQuery(p: ListParams): string {
  const q = new URLSearchParams()
  if (p.folder !== undefined) q.set('folder', p.folder)
  if (p.recursive) q.set('recursive', 'true')
  for (const tag of p.tag ?? []) q.append('tag', tag)
  if (p.q) q.set('q', p.q)
  if (p.in) q.set('in', p.in)
  if (p.updatedFrom) q.set('updatedFrom', p.updatedFrom)
  if (p.updatedTo) q.set('updatedTo', p.updatedTo)
  if (p.createdFrom) q.set('createdFrom', p.createdFrom)
  if (p.createdTo) q.set('createdTo', p.createdTo)
  if (p.sort) q.set('sort', p.sort)
  if (p.filter?.length) q.set('filter', JSON.stringify(p.filter))
  if (p.props) q.set('props', '1')
  if (p.body) q.set('body', '1')
  if (p.limit) q.set('limit', String(p.limit))
  if (p.cursor) q.set('cursor', p.cursor)
  const s = q.toString()
  return s ? `?${s}` : ''
}

export const enc = encodeURIComponent

export function groupsParamsToQuery(p: GroupsParams): string {
  const q = new URLSearchParams()
  q.set('key', p.key)
  if (p.folder !== undefined) q.set('folder', p.folder)
  if (p.recursive) q.set('recursive', 'true')
  for (const tag of p.tag ?? []) q.append('tag', tag)
  if (p.filter?.length) q.set('filter', JSON.stringify(p.filter))
  return `?${q.toString()}`
}

const scopeQuery = (s: ViewScope) => ('folder' in s ? `folder=${enc(s.folder)}` : `tag=${enc(s.tag)}`)

// Where the bytes of a file of a space are served (docs/api.md, "Files"): `path`
// is from the root of the space, and each segment is percent-encoded.
export function fileUrl(space: string, path: string): string {
  const segments = path.split('/').filter((s) => s !== '')
  return `${API_BASE}/spaces/${enc(space)}/files/${segments.map(enc).join('/')}`
}

// Everything under one space. A space id is a path segment of the URL, so each
// caller says which space it means; there is no "current" space in this module.
export function spaceApi(space: string) {
  const base = (rest = '') => `/spaces/${enc(space)}${rest}`
  const pageBase = (id: string, rest = '') => base(`/pages/${enc(id)}${rest}`)
  return {
    listPages: (params: ListParams = {}, signal?: AbortSignal) =>
      request<PageList>(base(`/pages${listParamsToQuery(params)}`), { signal }),
    // How many pages have each value of a property (the heads of a board's columns).
    groups: (params: GroupsParams, signal?: AbortSignal) =>
      request<GroupsResponse>(base(`/pages/groups${groupsParamsToQuery(params)}`), { signal }),
    // The saved views of a folder or a tag; a save replaces the whole list and 409s (with `current`) on a stale `baseRev`.
    views: (scope: ViewScope, signal?: AbortSignal) =>
      request<ViewList>(base(`/views?${scopeQuery(scope)}`), { signal }),
    putViews: (scope: ViewScope, input: PutViewsInput, signal?: AbortSignal) =>
      request<ViewList>(base('/views'), { method: 'PUT', body: { ...scope, ...input }, signal }),
    getPage: (id: string, signal?: AbortSignal) => request<Page>(pageBase(id), { signal }),
    createPage: (input: CreatePageInput = {}) => request<Page>(base('/pages'), { method: 'POST', body: input }),
    savePage: (id: string, input: SavePageInput, opts: { keepalive?: boolean } = {}) =>
      request<Page>(pageBase(id), { method: 'PUT', body: input, keepalive: opts.keepalive }),
    // Changes properties of the header only; the body does not travel. 409 with the page on a stale rev.
    patchProps: (id: string, input: PatchPropsInput) =>
      request<Page>(pageBase(id, '/props'), { method: 'PATCH', body: input }),
    movePage: (id: string, folder: string) =>
      request<Page>(pageBase(id, '/move'), { method: 'POST', body: { folder } }),
    deletePage: (id: string) => request<void>(pageBase(id), { method: 'DELETE' }),
    // Several pages as one change. Resolves to how many were moved or deleted.
    batchPages: (input: BatchPagesInput) =>
      request<{ pages: number }>(base('/pages/batch'), { method: 'POST', body: input }),

    // Drafts: what autosave writes. The file changes only on commit.
    putDraft: (id: string, input: DraftInput, opts: { keepalive?: boolean } = {}) =>
      request<DraftReceipt>(pageBase(id, '/draft'), { method: 'PUT', body: input, keepalive: opts.keepalive }),
    // With a version, only that draft is deleted (409 when it is another).
    deleteDraft: (id: string, version?: string, rev?: string) =>
      request<void>(
        pageBase(id, `/draft${version ? `?version=${enc(version)}${rev ? `&rev=${enc(rev)}` : ''}` : ''}`),
        { method: 'DELETE' },
      ),
    // Only the draft of that version is committed (409 with the page and the draft otherwise).
    commitPage: (id: string, version: string) =>
      request<Page>(pageBase(id, '/commit'), { method: 'POST', body: { version } }),

    // Who links to a page (20 at a time), and where its own links lead.
    backlinks: (id: string, cursor?: string, signal?: AbortSignal) =>
      request<BacklinkList>(pageBase(id, `/backlinks${cursor ? `?cursor=${enc(cursor)}` : ''}`), { signal }),
    links: (id: string, signal?: AbortSignal) => request<PageLinkList>(pageBase(id, '/links'), { signal }),

    // Stores a file for a page; the answer's `path` is what the page refers to.
    uploadAsset: (id: string, file: Blob, name: string, signal?: AbortSignal) =>
      request<UploadedAsset>(pageBase(id, `/assets?name=${enc(name)}`), { method: 'POST', blob: file, signal }),

    // One Markdown file (`name` is required) or a .zip of them; the server tells
    // which from the bytes. Everything lands in `folder`, made if missing.
    importMarkdown: (file: Blob, opts: { folder?: string; name?: string } = {}, signal?: AbortSignal) => {
      const q = new URLSearchParams()
      if (opts.folder) q.set('folder', opts.folder)
      if (opts.name) q.set('name', opts.name)
      const qs = q.toString()
      return request<ImportReport>(base(`/import${qs ? `?${qs}` : ''}`), { method: 'POST', blob: file, signal })
    },

    tree: (signal?: AbortSignal) => request<TreeResponse>(base('/tree'), { signal }),
    createFolder: (path: string) => request<{ path: string }>(base('/folders'), { method: 'POST', body: { path } }),
    // Moves a folder with its contents; renaming is a move to a sibling path.
    moveFolder: (from: string, to: string) =>
      request<{ path: string }>(base('/folders/move'), { method: 'POST', body: { from, to } }),
    // Without `recursive` the folder must hold no pages.
    deleteFolder: (path: string, recursive = false) =>
      request<void>(base(`/folders?path=${enc(path)}${recursive ? '&recursive=true' : ''}`), { method: 'DELETE' }),
    tags: (signal?: AbortSignal) => request<TagsResponse>(base('/tags'), { signal }),
    // Sets a tag's colour, or removes it with null. The answer leaves `color` out when removed.
    setTagColor: (tag: string, color: TagColor | null) =>
      request<{ tag: string; color?: TagColor }>(base(`/tags/${enc(tag)}`), { method: 'PUT', body: { color } }),

    // Page types and the folders bound to them. Writing needs `manage`.
    types: (signal?: AbortSignal) => request<TypesResponse>(base('/types'), { signal }),
    putType: (name: string, input: TypeInput) =>
      request<PageType>(base(`/types/${enc(name)}`), { method: 'PUT', body: input }),
    deleteType: (name: string, baseRev: string) =>
      request<void>(base(`/types/${enc(name)}?baseRev=${enc(baseRev)}`), { method: 'DELETE' }),
    // The check page (docs/api.md, "Checks"). Fixing needs `write`.
    checks: (params: ChecksParams = {}, signal?: AbortSignal) => {
      const q = new URLSearchParams()
      if (params.kind) q.set('kind', params.kind)
      if (params.cursor) q.set('cursor', params.cursor)
      const s = q.toString()
      return request<ChecksResponse>(base(`/checks${s ? `?${s}` : ''}`), { signal })
    },
    fixCheck: (input: FixCheckInput) => request<Page>(base('/checks/fix'), { method: 'POST', body: input }),
    // Templates (docs/api.md, "Templates"). Writing needs `manage`.
    templates: (signal?: AbortSignal) => request<TemplatesResponse>(base('/templates'), { signal }),
    template: (name: string, signal?: AbortSignal) =>
      request<PageTemplate>(base(`/templates/${enc(name)}`), { signal }),
    putTemplate: (name: string, input: PutTemplateInput) =>
      request<PageTemplate>(base(`/templates/${enc(name)}`), { method: 'PUT', body: input }),
    deleteTemplate: (name: string, baseRev: string) =>
      request<void>(base(`/templates/${enc(name)}?baseRev=${enc(baseRev)}`), { method: 'DELETE' }),
    // `template` "" removes the default.
    setFolderTemplate: (folder: string, template: string) =>
      request<TemplateDefaults>(base('/templates/defaults'), { method: 'PUT', body: { folder, template } }),
    setQuickNoteTemplate: (quickNote: string) =>
      request<TemplateDefaults>(base('/templates/defaults'), { method: 'PUT', body: { quickNote } }),
    // The icons and colours of folders, and the presets of the sources (docs/api.md, "Icons, colours...").
    appearance: (signal?: AbortSignal) => request<Appearance>(base('/appearance'), { signal }),
    // "" for the icon and the colour takes them away. Needs `write`.
    setFolderStyle: (folder: string, icon: string, color: IconColor | '') =>
      request<FolderStyle>(base('/folders/style'), { method: 'PUT', body: { folder, icon, color } }),
    sources: (signal?: AbortSignal) => request<{ sources: SourceCount[] }>(base('/sources'), { signal }),
    // null goes back to the defaults. Needs `manage`.
    setSourcePresets: (presets: string[] | null) =>
      request<{ presets: string[] | null }>(base('/sources/presets'), { method: 'PUT', body: { presets } }),
    // Gives a field of a type another key, in the type and in every page of it. Needs `manage`.
    renameTypeField: (name: string, input: { baseRev: string; from: string; to: string }) =>
      request<FieldRename>(base(`/types/${enc(name)}/rename-field`), {
        method: 'POST',
        body: input,
      }),
    // `type` "" removes the binding.
    bindFolder: (folder: string, type: string) =>
      request<FolderBinding>(base('/folders/type'), { method: 'PUT', body: { folder, type } }),

    // History: nothing here is asked for until the history panel is open.
    history: (id: string, params: { limit?: number; cursor?: string } = {}, signal?: AbortSignal) => {
      const q = new URLSearchParams()
      if (params.limit) q.set('limit', String(params.limit))
      if (params.cursor) q.set('cursor', params.cursor)
      const qs = q.toString()
      return request<HistoryList>(pageBase(id, `/history${qs ? `?${qs}` : ''}`), { signal })
    },
    version: (id: string, commit: string, signal?: AbortSignal) =>
      request<PageVersion>(pageBase(id, `/versions/${enc(commit)}`), { signal }),

    // The remote repository of the space (accounts mode, owner only).
    getRemote: (signal?: AbortSignal) => request<Remote>(base('/remote'), { signal }),
    remoteKey: (rotate = false) =>
      request<RemoteKey>(base('/remote/key'), { method: 'POST', body: rotate ? { rotate: true } : {} }),
    connectRemote: (input: ConnectRemoteInput) => request<Remote>(base('/remote'), { method: 'PUT', body: input }),
    disconnectRemote: () => request<void>(base('/remote'), { method: 'DELETE' }),
    syncRemote: () => request<Remote>(base('/remote/sync'), { method: 'POST' }),
    // The attachment repository (docs/api.md, "Server storage"); 404 when the space has none.
    getAssetsRemote: (signal?: AbortSignal) => request<AssetsRemote>(base('/remote/assets'), { signal }),
    connectAssets: (input: ConnectAssetsInput) =>
      request<AssetsRemote>(base('/remote/assets'), { method: 'PUT', body: input }),
    disconnectAssets: () => request<void>(base('/remote/assets'), { method: 'DELETE' }),
    moveAssets: () => request<AssetsRemote>(base('/remote/assets/move'), { method: 'POST' }),
    acceptRemote: (commit: string) => request<Remote>(base('/remote/accept'), { method: 'POST', body: { commit } }),

    // The day's text. `date` is the day where the browser is and `tz` its offset in minutes east of UTC; `skip` asks for another.
    daily: (q: { date: string; skip?: number; lang?: string; tz?: number }, signal?: AbortSignal) => {
      const p = new URLSearchParams({ date: q.date })
      if (q.skip) p.set('skip', String(q.skip))
      if (q.lang) p.set('lang', q.lang)
      if (q.tz !== undefined) p.set('tz', String(q.tz))
      return request<DailyAnswer>(base(`/daily?${p.toString()}`), { signal })
    },
    dailySettings: (signal?: AbortSignal) =>
      request<{ daily: DailySettings | null; inEffect?: boolean }>(base('/daily/settings'), { signal }),
    putDailySettings: (daily: DailySettings | null) =>
      request<{ daily: DailySettings | null; inEffect?: boolean }>(base('/daily/settings'), {
        method: 'PUT',
        body: { daily },
      }),
    // Accounts mode only (the single-user server answers 404).
    dailyPrefs: (signal?: AbortSignal) => request<DailyPrefs>(base('/daily/prefs'), { signal }),
    putDailyPrefs: (input: {
      inherit: boolean
      apis: string[]
      options?: DailyOptionChoice
      sources?: Record<string, DailySpaceSource[]>
    }) => request<DailyPrefs>(base('/daily/prefs'), { method: 'PUT', body: input }),

    // A person's own favourites of the space.
    favorites: (signal?: AbortSignal) => request<FavoriteList>(base('/favorites'), { signal }),
    addFavorite: (ref: FavoriteRef) => request<FavoriteList>(base('/favorites'), { method: 'POST', body: ref }),
    removeFavorite: (ref: FavoriteRef) =>
      request<FavoriteList>(base(`/favorites?kind=${enc(ref.kind)}&target=${enc(ref.target)}`), { method: 'DELETE' }),
    orderFavorites: (items: FavoriteRef[]) =>
      request<FavoriteList>(base('/favorites/order'), { method: 'PUT', body: { items } }),

    reindex: () => request<{ pages: number; ms: number }>(base('/reindex'), { method: 'POST' }),

    // Offline sync (docs/api.md, "Offline sync"): the catalogue a batch at a time, what changed after a change
    // cursor (410 `changes_expired` when it is too old), and several pages at once.
    syncCatalog: (params: { folders?: string[]; limit?: number; cursor?: string } = {}, signal?: AbortSignal) => {
      const q = new URLSearchParams()
      for (const f of params.folders ?? []) q.append('folder', f)
      if (params.limit) q.set('limit', String(params.limit))
      if (params.cursor) q.set('cursor', params.cursor)
      const qs = q.toString()
      return request<CatalogBatch>(base(`/sync/catalog${qs ? `?${qs}` : ''}`), { signal })
    },
    changes: (since: string, limit?: number, signal?: AbortSignal) =>
      request<ChangeList>(base(`/changes?since=${enc(since)}${limit ? `&limit=${limit}` : ''}`), { signal }),
    syncPages: (ids: string[], signal?: AbortSignal) =>
      request<SyncPages>(base(`/sync/pages?ids=${ids.map(enc).join(',')}`), { signal }),

    // The people of the space (accounts mode). `user` is a member's `userId`.
    members: (signal?: AbortSignal) => request<MembersResponse>(base('/members'), { signal }),
    // Everyone who can reach the space, through their own role or a group, a cursor page at a time.
    effectiveMembers: (params: { q?: string; cursor?: string; limit?: number } = {}, signal?: AbortSignal) => {
      const q = new URLSearchParams()
      if (params.q) q.set('q', params.q)
      if (params.cursor) q.set('cursor', params.cursor)
      if (params.limit) q.set('limit', String(params.limit))
      const qs = q.toString()
      return request<EffectiveMemberList>(base(`/members/effective${qs ? `?${qs}` : ''}`), { signal })
    },
    // Groups as members of the space (shared spaces only).
    addGroup: (groupId: string, role: SpaceRole) =>
      request<GroupMember>(base('/groups'), { method: 'POST', body: { groupId, role } }),
    setGroupRole: (groupId: string, role: SpaceRole) =>
      request<GroupMember>(base(`/groups/${enc(groupId)}`), { method: 'PATCH', body: { role } }),
    removeGroup: (groupId: string) => request<void>(base(`/groups/${enc(groupId)}`), { method: 'DELETE' }),
    addMember: (username: string, role: SpaceRole) =>
      request<Member>(base('/members'), { method: 'POST', body: { username, role } }),
    setMemberRole: (user: string, role: SpaceRole) =>
      request<Member>(base(`/members/${enc(user)}`), { method: 'PATCH', body: { role } }),
    // Removing yourself is leaving the space.
    removeMember: (user: string) => request<void>(base(`/members/${enc(user)}`), { method: 'DELETE' }),
  }
}
export type SpaceApi = ReturnType<typeof spaceApi>

export const api = {
  meta: (signal?: AbortSignal) => request<MetaResponse>('/meta', { signal }),

  // --- session and signing in ---
  session: async (signal?: AbortSignal) => {
    const s = await request<SessionInfo>('/session', { signal })
    if (s.user) {
      markSignedIn()
      rememberUser(s.user.id)
    }
    return s
  },
  setup: async (input: {
    token: string
    username: string
    name?: string
    password: string
    captcha?: CaptchaAnswer
  }) => {
    const r = await request<SignInAnswer>('/setup', { method: 'POST', body: input })
    markSignedIn()
    rememberUser(r.user.id)
    return r
  },
  // Where an address signs in: about its domain only, never an account; a username gets null.
  discover: (email: string) =>
    request<{ sso: DiscoveredSSO | null }>('/auth/discover', { method: 'POST', body: { email } }),
  login: async (input: { username: string; password: string; captcha?: CaptchaAnswer; tenant?: string }) => {
    const r = await request<SignInAnswer>('/login', { method: 'POST', body: input })
    markSignedIn()
    rememberUser(r.user.id)
    return r
  },
  // Signing out deletes this person's copies on the device (they are not encrypted: see the offline settings).
  logout: async () => {
    const who = knownUser
    await request<void>('/logout', { method: 'POST' })
    // On purpose: the browser forgets who it was too (a session that ended by itself keeps it).
    forgetLastAccount()
    knownUser = null
    if (who) await offlineHooks.signedOut?.(who).catch(() => undefined)
  },
  // What the sign-in page draws: the enabled providers, the mode in effect and whether passkeys work.
  providers: (signal?: AbortSignal, tenant?: string) =>
    request<AuthProviders>(`/auth/providers${tenant ? `?tenant=${enc(tenant)}` : ''}`, { signal }),
  // Whether a captcha is needed now, and the challenge when it is. `account` matters for `login` only; `tenant` (a
  // slug) is the organisation the form is opened for, whose captcha settings apply (absent: the site's).
  captcha: (ctx: CaptchaContext, account?: string, tenant?: string, signal?: AbortSignal) =>
    request<CaptchaChallenge>(
      `/captcha?ctx=${enc(ctx)}${account ? `&account=${enc(account)}` : ''}${tenant ? `&tenant=${enc(tenant)}` : ''}`,
      { signal },
    ),
  // Setup, invite and link carry a token or a session, so they ask for the address with a POST; a login is a plain navigation (see `providerStartUrl`).
  providerStart: (
    kind: ProviderKind,
    slug: string,
    input: { intent: ProviderIntent; token?: string; return?: string },
  ) => request<{ url: string }>(`/auth/${enc(kind)}/${enc(slug)}/start`, { method: 'POST', body: input }),
  passkeyBegin: (username?: string) =>
    request<PasskeyCeremony>('/auth/passkey/begin', { method: 'POST', body: username ? { username } : {} }),
  passkeyFinish: async (ceremony: string, ret: string, credential: unknown) => {
    const r = await request<SignInAnswer & { return: string }>(
      `/auth/passkey/finish?ceremony=${enc(ceremony)}&return=${enc(ret)}`,
      { method: 'POST', body: credential },
    )
    markSignedIn()
    rememberUser(r.user.id)
    return r
  },

  // --- account ---
  account: (signal?: AbortSignal) => request<AccountInfo>('/account', { signal }),
  // The caller's own spaces, bytes and pages against their limits (3.3): stored numbers, nothing is measured for the request.
  accountUsage: (signal?: AbortSignal) => request<AccountUsage>('/account/usage', { signal }),
  // Finding a group by part of its name, to add it to a space: names only.
  findGroups: (q: string, signal?: AbortSignal) =>
    request<{ groups: GroupRef[] }>(`/groups?q=${enc(q)}&limit=20`, { signal }),
  updateAccount: (input: { name?: string; username?: string; email?: string }) =>
    request<{ user: User }>('/account', { method: 'PATCH', body: input }),
  // `current` is left out when the account has no password yet.
  changePassword: (input: { current?: string; new: string }) =>
    request<void>('/account/password', { method: 'POST', body: input }),
  // The person's accounts at providers (one per provider: a sign-in, repositories, or both), and the providers they may
  // connect repositories at. `provider` is the provider's id (its slug, for a site provider).
  accountLinks: (signal?: AbortSignal) =>
    request<{ links: Link[]; providers: Provider[] }>('/account/links', { signal }),
  startLink: (provider: string, ret?: string) =>
    request<{ url: string }>(`/account/links/${enc(provider)}/start`, {
      method: 'POST',
      body: { purpose: 'signin', ...(ret ? { return: ret } : {}) },
    }),
  removeLink: (id: string) => request<void>(`/account/links/${enc(id)}`, { method: 'DELETE' }),
  // Registering a passkey: a ceremony, the browser's credential, then the list as it is.
  passkeyRegisterBegin: (name: string) =>
    request<PasskeyCeremony>('/account/passkeys/begin', { method: 'POST', body: { name } }),
  passkeyRegisterFinish: (ceremony: string, credential: unknown) =>
    request<{ passkeys: Passkey[] }>(`/account/passkeys/finish?ceremony=${enc(ceremony)}`, {
      method: 'POST',
      body: credential,
    }),
  renamePasskey: (id: string, name: string) =>
    request<Passkey>(`/account/passkeys/${enc(id)}`, { method: 'PATCH', body: { name } }),
  removePasskey: (id: string) => request<void>(`/account/passkeys/${enc(id)}`, { method: 'DELETE' }),

  // --- API tokens ---
  // What each setting that a space or a person can change comes to for the caller, and who keeps it from changing (3.3).
  settings: (space?: string, signal?: AbortSignal) =>
    request<MySettings>(`/settings${space ? `?space=${enc(space)}` : ''}`, { signal }),
  tokens: (signal?: AbortSignal) => request<{ tokens: ApiToken[] }>('/account/tokens', { signal }),
  // `expiresInDays` 0 is a token that never expires; left out, 90 days.
  createToken: (input: { name: string; access: TokenAccess; spaces: string[]; expiresInDays?: number }) =>
    request<CreatedToken>('/account/tokens', { method: 'POST', body: input }),
  revokeToken: (id: string) => request<void>(`/account/tokens/${enc(id)}`, { method: 'DELETE' }),

  // --- the notification centre and the person's own preferences ---
  notifications: (params: NotificationsParams = {}, signal?: AbortSignal) => {
    const q = new URLSearchParams()
    if (params.kind) q.set('kind', params.kind)
    if (params.unread) q.set('unread', '1')
    if (params.cursor) q.set('cursor', params.cursor)
    if (params.limit) q.set('limit', String(params.limit))
    const qs = q.toString()
    return request<NotificationList>(`/notifications${qs ? `?${qs}` : ''}`, { signal })
  },
  unreadNotifications: (signal?: AbortSignal) => request<{ count: number }>('/notifications/unread', { signal }),
  markNotificationsRead: (ids: string[]) =>
    request<{ unread: number }>('/notifications/read', { method: 'POST', body: { ids } }),
  markAllNotificationsRead: (kind?: string) =>
    request<{ unread: number }>('/notifications/read-all', { method: 'POST', body: kind ? { kind } : {} }),
  // The person's own preferences, kept in their personal space (never the database); null values remove a key.
  prefs: (signal?: AbortSignal) => request<PrefsAnswer>('/prefs', { signal }),
  patchPrefs: (set: Record<string, unknown>) =>
    request<{ personal: boolean }>('/prefs', { method: 'PATCH', body: { set } }),

  // --- connected apps (OAuth grants) and the consent page ---
  // 404 when the server has no authorization server (no public address, or MCP is off).
  grants: (signal?: AbortSignal) => request<{ grants: Grant[] }>('/account/grants', { signal }),
  revokeGrant: (id: string) => request<void>(`/account/grants/${enc(id)}`, { method: 'DELETE' }),
  consent: (req: string, signal?: AbortSignal) => request<ConsentInfo>(`/oauth/consent?req=${enc(req)}`, { signal }),
  decideConsent: (input: ConsentDecision) =>
    request<{ redirect: string }>('/oauth/consent', { method: 'POST', body: input }),

  // --- users and invites (administrators) ---
  // --- registration, email verification and password reset (3.3) ---
  registerInfo: (tenant?: string, signal?: AbortSignal) =>
    request<RegisterInfo>(`/register${tenant ? `?tenant=${enc(tenant)}` : ''}`, { signal }),
  register: (input: RegisterInput) => request<{ sent: boolean }>('/register', { method: 'POST', body: input }),
  registerConfirm: async (token: string, lang?: string) => {
    const r = await request<RegisterConfirmAnswer>('/register/confirm', {
      method: 'POST',
      body: { token },
      ...(lang ? { headers: { 'Accept-Language': lang } } : {}),
    })
    // A request to join signs nobody in.
    if (!r.request) markSignedIn()
    return r
  },
  // The waiting page of a sign-up that asked to join an organisation (3.6 d5): the registration link's token shows the
  // request, and withdraws it.
  joinStatus: (token: string, signal?: AbortSignal) =>
    request<JoinStatus>('/register/request', { method: 'POST', body: { token }, signal }),
  withdrawJoin: (token: string) =>
    request<JoinStatus>('/register/request/withdraw', { method: 'POST', body: { token } }),
  // More than one address (accounts.multipleEmails): add one (a link is sent to it), send its link again, make a
  // verified one primary, remove one that is not.
  addEmail: (email: string, lang?: string) =>
    request<{ sent: boolean }>('/account/emails', { method: 'POST', body: lang ? { email, lang } : { email } }),
  resendEmailVerify: (email: string, lang?: string) =>
    request<{ sent: boolean }>('/account/emails/verify', { method: 'POST', body: lang ? { email, lang } : { email } }),
  makePrimaryEmail: (email: string) =>
    request<{ user: User }>('/account/emails/primary', { method: 'POST', body: { email } }),
  removeEmail: (email: string) => request<void>(`/account/emails/${enc(email)}`, { method: 'DELETE' }),
  requestEmailVerify: (lang?: string) =>
    request<{ sent: boolean }>('/account/email/verify', { method: 'POST', body: lang ? { lang } : {} }),
  confirmEmail: (token: string) => request<void>('/email/confirm', { method: 'POST', body: { token } }),
  forgotPassword: (input: { tenant?: string; login: string; captcha?: CaptchaAnswer; lang?: string }) =>
    request<{ sent: boolean }>('/password/forgot', { method: 'POST', body: input }),
  checkReset: (token: string, signal?: AbortSignal) =>
    request<{ username: string; expires: string }>('/password/reset/check', {
      method: 'POST',
      body: { token },
      signal,
    }),
  resetPassword: (token: string, password: string) =>
    request<void>('/password/reset', { method: 'POST', body: { token, password } }),
  checkInvite: (token: string, captcha?: CaptchaAnswer, signal?: AbortSignal) =>
    request<InviteCheck>('/invites/check', {
      method: 'POST',
      body: { token, ...(captcha ? { captcha } : {}) },
      signal,
    }),
  acceptInvite: async (input: {
    token: string
    username: string
    name?: string
    password: string
    captcha?: CaptchaAnswer
  }) => {
    const r = await request<SignInAnswer>('/invites/accept', { method: 'POST', body: input })
    markSignedIn()
    rememberUser(r.user.id)
    return r
  },

  // --- spaces ---
  spaces: (signal?: AbortSignal) => request<{ spaces: SpaceInfo[] }>('/spaces', { signal }),
  // `sync` makes the space's repository on a provider too (docs/api.md, "Spaces and remotes").
  createSpace: (name: string, inbox?: string, sync?: NewSync, mode?: 'server' | 'own' | 'tenant') =>
    request<SpaceInfo>('/spaces', {
      method: 'POST',
      body: {
        name,
        ...(inbox === undefined ? {} : { inbox }),
        ...(sync === undefined && mode === undefined
          ? {}
          : { storage: { ...(mode === undefined ? {} : { mode }), ...(sync === undefined ? {} : { sync }) } }),
      },
    }),

  // --- repositories at a provider (accounts mode; docs/api.md, "Linked accounts") ---
  connections: (signal?: AbortSignal) => request<{ providers: Provider[] }>('/account/links', { signal }),
  // The address to send the browser to; the platform sends it back to `return` (a path of this site).
  // `install` sends them to the App's installation page, which authorises them in the same flow; without it, to the authorisation page alone.
  connectionStart: (provider: string, ret?: string, install?: boolean) =>
    request<{ url: string }>(`/account/links/${enc(provider)}/start`, {
      method: 'POST',
      body: { purpose: 'repos', ...(ret ? { return: ret } : {}), ...(install ? { install: true } : {}) },
    }),
  // Drops the repository tokens of the person's link at the provider (a sign-in at it stays).
  connectionDisconnect: async (provider: string) => {
    const { links } = await api.accountLinks()
    const link = links.find((l) => l.providerId === provider && l.repos)
    if (link) await request<void>(`/account/links/${enc(link.id)}/repos`, { method: 'DELETE' })
  },
  connectionDismissLeftover: (provider: string, repoId: number) =>
    request<void>(`/account/leftovers/${enc(provider)}/${repoId}`, { method: 'DELETE' }),
  // recheck: the first page, after the server forgets what it remembered about this person ("the App is not installed there").
  connectionOwners: (provider: string, cursor?: string, signal?: AbortSignal, recheck = false) =>
    request<CursorPage<ConnectionOwner>>(
      `/account/links/${enc(provider)}/owners${cursor ? `?cursor=${enc(cursor)}` : recheck ? '?recheck=true' : ''}`,
      { signal },
    ),
  connectionRepos: (
    provider: string,
    which: { installation: number } | { mine: true },
    cursor?: string,
    signal?: AbortSignal,
  ) => {
    const q = new URLSearchParams(
      'installation' in which ? { installation: String(which.installation) } : { mine: 'true' },
    )
    if (cursor) q.set('cursor', cursor)
    return request<CursorPage<ConnectionRepo>>(`/account/links/${enc(provider)}/repos?${q.toString()}`, { signal })
  },
  renameSpace: (id: string, name: string) =>
    request<SpaceInfo>(`/spaces/${enc(id)}`, { method: 'PATCH', body: { name } }),
  // Either field or both; the server refuses a body with neither.
  updateSpace: (id: string, patch: { name?: string; inbox?: string }) =>
    request<SpaceInfo>(`/spaces/${enc(id)}`, { method: 'PATCH', body: patch }),

  // The change cursor each of the spaces is at (1 to 100), from the server's memory.
  syncHeads: (spaces: string[], signal?: AbortSignal) =>
    request<SyncHeads>(`/sync/heads?${spaces.map((s) => `space=${enc(s)}`).join('&')}`, { signal }),

  // --- administration (administrators, accounts mode) ---
  // One space as the list has it; 404 for an id that is no space.
  adminSpace: (id: string, signal?: AbortSignal) => request<AdminSpace>(`/admin/spaces/${enc(id)}`, { signal }),
}

// Where a sign-in button goes: a full-page navigation (docs/api.md, "Starting").
// `hint` is the address of the person coming back, for the provider's `login_hint`.
export const providerStartUrl = (kind: ProviderKind, slug: string, ret: string, hint?: string) =>
  `${API_BASE}/auth/${enc(kind)}/${enc(slug)}/start?return=${enc(ret)}${hint ? `&login_hint=${enc(hint)}` : ''}`
