// Shapes of docs/api.md.

// What a caller may do in a space (docs/api.md, "Space"). The app shows or hides
// controls by these and never by the name of a role.
export type Permission = 'read' | 'write' | 'members' | 'manage'
// The caller's role in a space. Empty for an administrator who is not a member.
export type SpaceRole = 'owner' | 'editor' | 'viewer'

export interface SpaceInfo {
  id: string
  name: string
  // For showing ("Owner", "Viewer"); never for deciding what is allowed.
  role: SpaceRole | ''
  // What the caller may do here.
  permissions: Permission[]
  // True while an administrator has switched the space off: it stays in the
  // list, but nothing in it can be opened.
  disabled?: boolean
  // Made together with the account.
  personal: boolean
  // `local` (the repository lives on the server only) or the state of the
  // remote it follows. A server that does not say is taken to be local.
  sync?: SyncState
  // The folder quick notes go to: a folder path, never empty. Only a name; the
  // folder exists once a page has been made in it.
  inbox: string
  // On a space an administrator is not a member of: the audited view this session has open, null when there is none.
  view?: AdminViewState | null
  // From 3.2 `role` and `permissions` are the effective ones (the person's own
  // role and their groups' together). The person's own role, "" when they reach
  // the space only through groups, and whether a group of theirs is a member.
  directRole?: SpaceRole | ''
  viaGroups?: boolean
  // What the space's copy on the server is and what the storage policy asks of
  // it (docs/api.md, "Server storage"). Absent in single-user mode.
  storage?: SpaceStorage
}

// What a space's copy on the server is.
export type CacheState = 'local' | 'full' | 'partial' | 'evicted' | 'archived' | 'restoring'
// A requirement of the storage policy that the space does not meet yet: any
// remote, or one through the provider of that id (2.7b's connections).
export interface RemoteNotice {
  required: string
  deadline: string
  overdue: boolean
}
export interface SpaceStorage {
  cache: CacheState
  assets: 'space' | 'repository'
  // null unless the policy asks something of this space that it does not have yet.
  remote: RemoteNotice | null
}
export interface MetaResponse {
  version: string
  // The largest file, in bytes, the server accepts as an upload.
  maxUpload: number
  // The largest body, in bytes, the server accepts for an import.
  maxImport: number
  spaces: SpaceInfo[]
}

// What an import did (docs/api.md, "Import"). `skipped` is at most 200 entries;
// `skippedMore` counts the ones beyond that and is absent when there are none.
export interface ImportReport {
  pages: number
  files: number
  skipped: { path: string; reason: string }[]
  skippedMore?: number
}

// The answer to storing a file for a page: `path` is what goes into the page.
export interface UploadedAsset {
  path: string
  name: string
  size: number
}

export interface PageSummary {
  id: string
  path: string
  folder: string
  title?: string
  subtitle?: string
  // Where the text comes from: a list like the tags (a person, a book, a film).
  source: string[]
  label: string
  untitled: boolean
  tags?: string[]
  type?: string
  // The type the page has: its header's, else its folder's (see "Page types").
  effectiveType?: string
  created?: string
  updated: string
  excerpt?: string
  // The page's own icon (a Lucide name or an emoji) and colour, from its header.
  icon?: string
  color?: string
  // Only with `props: true` in the listing (the table view): the header's keys
  // that are not built in, as plain values, and the rev a change of them needs.
  props?: Record<string, unknown>
  rev?: string
  // With `props`: the keys whose value there is not the whole of what the header
  // holds (cut text, a list that lost elements, an object). Absent when none. A
  // cell must not write such a copy back.
  partialProps?: string[]
  // Only with `body: true` in the listing (continuous reading): the Markdown.
  body?: string
}

// The page header as the server serves it. Fields the app does not know about
// are sent back exactly as received; the server decides the key order.
export type PageMeta = Record<string, unknown>

// The caller's draft of a page (docs/api.md, "Drafts"). `stale` means the file
// moved on since `baseRev`, so the draft cannot be committed as it is.
export interface PageDraft {
  // Opaque; new each time the draft is stored. What `prev` and commit's
  // `version` refer to, since `updated` only has one-second resolution.
  version: string
  baseRev: string
  meta: PageMeta
  body: string
  updated: string
  stale: boolean
}

export interface Page {
  id: string
  path: string
  folder: string
  rev: string
  // Only the answer to a save or a commit carries it: the rev the text that was sent stands for, before the
  // server rewrote links in it (the save was based on a rev before a page it links to moved). Equal to `rev`
  // when nothing was rewritten. Text typed while the save was out is based on it, not on `rev` (docs/api.md,
  // "Typing during a save").
  textRev?: string
  updated: string
  meta: PageMeta
  // The header exactly as the file has it (the text between the `---` lines),
  // for showing the file as written. What a save writes is `meta` alone.
  header?: string
  body: string
  // Only `GET .../pages/{id}` carries it, and only when the caller has one.
  draft?: PageDraft
  // The type the page has: its header's `type`, else its folder's, else "".
  effectiveType?: string
  // The keys of `meta` that a listing holds only in part (the summary's list, judged by the server from
  // the file); absent when none. A cell must not write such a key back.
  partialProps?: string[]
}

export interface DraftReceipt {
  version: string
  baseRev: string
  updated: string
  stale: boolean
}

export type SortKey = 'updated' | 'created' | 'title' | '-updated' | '-created' | '-title'
// Sorting by a property of the header: `prop:status`, `-prop:status`.
export type PropSort = `prop:${string}` | `-prop:${string}`

// One condition of a table's filter (docs/api.md, "Filtering by properties").
export type FilterOp = 'in' | 'range' | 'empty' | 'notEmpty' | 'contains'
export interface PropFilter {
  key: string
  // `notIn` is the server's too (has a value and none of the given ones); the table's own form never makes it,
  // a board's "other values" column and a saved view do.
  op: FilterOp | 'notIn'
  values?: (string | number | boolean | null)[]
}

export interface ListParams {
  folder?: string
  recursive?: boolean
  tag?: string[]
  q?: string
  // With `q`: `title` searches titles only.
  in?: 'title'
  // Days (`2026-11-01`, UTC), both ends included. `updated*` is when the file
  // was last written; `created*` is the header's own date.
  updatedFrom?: string
  updatedTo?: string
  createdFrom?: string
  createdTo?: string
  sort?: SortKey | PropSort
  // Conditions on properties; all must hold.
  filter?: PropFilter[]
  // Put `props` and `rev` in the summaries (the table view does).
  props?: boolean
  // Put the Markdown `body` in the summaries; the limit is then at most 20.
  body?: boolean
  limit?: number
  cursor?: string
}
export interface PageList {
  items: PageSummary[]
  next: string
}

// A page that links to this one (`GET …/pages/{id}/backlinks`).
export interface Backlink {
  id: string
  path: string
  title: string
  untitled: boolean
  // The paragraph the link is in, as plain text; "" when the file no longer has the link.
  excerpt: string
}
export interface BacklinkList {
  items: Backlink[]
  next: string
  total: number
}
// One link of a page, in the order of the text (`GET …/pages/{id}/links`).
export interface PageLink {
  text: string
  // The file the link names, from the root of the space.
  path: string
  id?: string
  anchor?: string
  broken: boolean
  target?: { id: string; path: string; title: string; untitled: boolean }
}
export interface PageLinkList {
  items: PageLink[]
}

export interface CreatePageInput {
  folder?: string
  meta?: PageMeta
  body?: string
  // The template to start from: left out for the folder's (or the quick-note) default, "" for none.
  template?: string
  // The browser's offset in minutes east of UTC, for {{date}}, {{time}} and {{datetime}} (UTC when left out).
  tz?: number
}
export interface SavePageInput {
  baseRev: string
  meta: PageMeta
  body: string
}
// Storing a draft says which version it replaces ("" when none is known).
export interface DraftInput extends SavePageInput {
  prev: string
}

// --- page types (docs/api.md, "Page types") ----------------------------------------

export type FieldKind =
  'text' | 'longtext' | 'select' | 'multiselect' | 'date' | 'number' | 'checkbox' | 'url' | 'other'
export interface TypeField {
  key: string
  // "" when the file gives none: show the key.
  title: string
  kind: FieldKind
  // For select and multiselect.
  options?: string[]
  // The field's schema as the file has it.
  schema?: unknown
}
export interface PageType {
  // The file name; what a header's `type` says. Never changes.
  name: string
  // "" when the file gives none: show the name.
  title: string
  // The Git blob id of the type's file: a change or a delete is made from it (`baseRev`).
  rev: string
  fields: TypeField[]
  // The default icon and colour of the pages of the type (`x-icon`, `x-color` of its file).
  icon?: string
  color?: string
  // Set when the file cannot be used; the fields are then empty.
  error?: string
}
// A template page of .petalbook/templates (docs/api.md, "Templates"). The list leaves `body` out.
export interface PageTemplate {
  // The file name without .md; the template's id.
  name: string
  path: string
  rev: string
  meta: PageMeta
  body?: string
}
export interface TemplateDefaults {
  folders: Record<string, string>
  quickNote: string
}
export interface TemplatesResponse {
  templates: PageTemplate[]
  defaults: TemplateDefaults
}
export interface PutTemplateInput {
  // "" makes the template; otherwise the rev it was read at.
  baseRev: string
  meta: PageMeta
  body: string
}
// What the check page lists (docs/api.md, "Checks"), in this order.
export type ProblemKind =
  | 'no_id'
  | 'duplicate_id'
  | 'bad_header'
  | 'type_mismatch'
  | 'not_in_type'
  | 'broken_link'
  | 'missing_asset'
  | 'duplicate_title'
export type ProblemFix = 'add_id' | 'remove_fields'
export interface Problem {
  kind: ProblemKind
  path: string
  // The page, when the file is one.
  id?: string
  message: string
  // The current rev of the file the problem is in: the baseRev of POST .../checks/fix
  // (a file without an id has no page to read it from).
  rev: string
  // bad_header: {line}; duplicate_id and duplicate_title: {paths}; type_mismatch and
  // not_in_type: {fields}; broken_link: {target, line}; missing_asset: {target}.
  detail?: Record<string, unknown>
  fix: ProblemFix | null
}
export interface ChecksParams {
  kind?: ProblemKind
  cursor?: string
}
export interface ChecksResponse {
  // Every kind, listed or not.
  counts: Record<ProblemKind, number>
  items: Problem[]
  next: string
}
export interface FixCheckInput {
  kind: 'no_id' | 'not_in_type'
  path: string
  // The rev of the file as it was listed.
  baseRev: string
}
export interface FolderBinding {
  folder: string
  type: string
}
export interface TypesResponse {
  types: PageType[]
  bindings: FolderBinding[]
  // What the server did about a settings file it could not use (docs/api.md, "Settings files that cannot be used").
  // Only for those who manage the space; absent for anyone else, and when there is nothing to tell.
  settingsNotices?: SettingsNotice[]
}
// A version of a settings file: a commit, or (without one) a change in the folder that is in no commit yet.
export interface SettingsVersion {
  commit?: string
  time: string
}
export interface SettingsNotice {
  // The settings file or type file, as a path in the space.
  file: string
  // One entry of it (`Tang`, `Tang: views`), or "not a folder" for a link or a file where a folder belongs.
  entry?: string
  // The file in the space is still the one that cannot be used; false once it was put right.
  unreadable: boolean
  since?: SettingsVersion
  restoredFrom?: SettingsVersion
  // Where the file as it was is kept.
  copy?: string
  repairedAt?: string
  // The entries that could not be read even one by one ("*": none of it).
  lost?: string[]
}
// What `PUT .../types/{name}` takes: the whole of what the type is to be.
export interface TypeInput {
  // The `rev` of the type this is made from; "" or absent to make one.
  baseRev?: string
  title: string
  fields: { key: string; title?: string; kind?: FieldKind; options?: string[] }[]
}
export interface PatchPropsInput {
  baseRev: string
  set?: Record<string, unknown>
  unset?: string[]
}

// --- saved views (docs/api.md, "Saved views") -------------------------------------------

export type ViewLayout = 'list' | 'cards' | 'table' | 'board' | 'read'
// One saved view. The server always answers with every key; keys it does not know come back as they were
// sent (the app keeps `cardFields` there: the fields a board's card shows).
export interface SavedView {
  id: string
  name: string
  layout: ViewLayout
  // "" is the listing's default order.
  sort: string
  // The field a board is grouped by.
  group: string
  filter: PropFilter[]
  // The keys of the columns a table hides.
  hidden: string[]
  [extra: string]: unknown
}
export interface ViewList {
  views: SavedView[]
  rev: string
}
// A view belongs to a folder ("" is the root) or to a tag.
export type ViewScope = { folder: string } | { tag: string }
export interface PutViewsInput {
  baseRev: string
  // New views have no `id`; the server makes one.
  views: (Omit<SavedView, 'id'> & { id?: string })[]
}

// The answer of `GET .../pages/groups`: how many pages hold each value of a property, for a board's headers.
export interface GroupsResponse {
  groups: { value: string; count: number }[]
  empty: number
  truncated: boolean
}
export interface GroupsParams {
  key: string
  folder?: string
  recursive?: boolean
  tag?: string[]
  filter?: PropFilter[]
}

export interface TreeResponse {
  // `updated`: the newest page in the folder or below it; absent when there is none.
  folders: { path: string; pages: number; updated?: string }[]
}
// The ten names the server accepts for a tag's colour; how each looks is the
// client's business (src/lib/tagColors.ts).
export type TagColorName = 'gray' | 'red' | 'orange' | 'amber' | 'green' | 'teal' | 'blue' | 'purple' | 'pink' | 'rose'
// A name, or a custom `#rrggbb` (the server stores it in lower case).
export type TagColor = TagColorName | `#${string}`
// The eight colours of an icon (the server keeps only the name).
export type IconColor = 'red' | 'orange' | 'amber' | 'green' | 'teal' | 'blue' | 'purple' | 'pink'
// The icon and colour of a folder, kept in space.yml.
export interface FolderStyle {
  folder: string
  icon?: string
  color?: IconColor
}
// What renaming a field of a type did (docs/api.md): the type now, how many pages were rewritten, and the pages of
// the type left as they were: `has_key` when one already has a value under the new key, `unreadable` when its header
// cannot be read (such a file has no id).
export interface FieldRename {
  type: PageType
  pages: number
  skipped: { id?: string; path: string; reason: 'has_key' | 'unreadable' }[]
}

export interface Appearance {
  folders: FolderStyle[]
  // The sources the space offers; null when it chose none (the client's defaults apply).
  sourcePresets: string[] | null
  iconColors: IconColor[]
}
export interface SourceCount {
  source: string
  pages: number
}

export interface TagsResponse {
  // `color` is absent when the tag has none.
  tags: { tag: string; pages: number; color?: TagColor }[]
}

// --- history (docs/api.md, "History") -------------------------------------------

export type VersionChange = 'add' | 'edit' | 'move' | 'delete'
// One commit that changed the page's file, newest first in a listing.
export interface HistoryItem {
  commit: string
  time: string
  author: string
  path: string
  // Absent for the version in which the page was deleted.
  rev?: string
  change: VersionChange
  // How the change was made, when it did not come from the web app.
  via?: 'mcp' | 'token'
}
export interface HistoryList {
  items: HistoryItem[]
  next: string
}
// The page as it was in one version.
export interface PageVersion {
  commit: string
  time: string
  author: string
  path: string
  rev?: string
  meta: PageMeta
  body: string
}

// --- remote repository (docs/api.md, "Remote repository") -------------------------

export type RemoteState = 'ok' | 'offline' | 'rewritten' | 'error'
export type SyncState = 'local' | RemoteState

export type RemoteCredential =
  | { type: 'ssh'; publicKey: string; fingerprint: string }
  | { type: 'token'; username: string }
  // A provider's connection (docs/api.md, "Connections"): no secret is stored for the space or returned.
  | { type: 'connection'; provider: string; via: 'app'; installationId: number; account: string; repoId: number }
  | {
      type: 'connection'
      provider: string
      via: 'user'
      user: { id: string; username: string }
      login: string
      account?: string
      repoId: number
    }

export interface Remote {
  url: string
  branch: string
  // The provider the remote goes through (`git` for a plain address).
  provider: string
  credential: RemoteCredential
  state: RemoteState
  // Absent before the first time the server saw the remote.
  lastSync?: string
  // In words fit to show; empty when all is well.
  error: string
  // What `error` state needs, for choosing what to offer: `auth`, `hostkey`, `clone`, `rescued` or a
  // connection code (see ConnectionErrorCode). A code this client does not know is treated like `auth`.
  errorCode: string
  // SSH only: the host key trusted on first connection.
  hostKey?: string
  // Set when `state` is `rewritten`.
  rewritten: { lastGood: string; remote: string } | null
}

// How far moving the attachments of a space into its attachment repository has come.
export type AssetMoveState = 'copying' | 'switching' | 'done' | 'failed'
export interface AssetMove {
  state: AssetMoveState
  files: number
  filesDone: number
  bytes: number
  bytesDone: number
  error: string
}
// The attachment repository of a space: a Remote, and the move.
export interface AssetsRemote extends Remote {
  move: AssetMove | null
}
// What `PUT .../remote/assets` takes: a Remote's body, or a repository to make or find on a provider.
export type ConnectAssetsInput =
  | ConnectRemoteInput
  | {
      sync: {
        provider: string
        owner: string
        name: string
        private?: boolean
        description?: string
        create?: boolean
        via?: ConnectionVia
      }
    }

export interface RemoteKey {
  publicKey: string
  fingerprint: string
}

export interface ConnectRemoteInput {
  url: string
  branch?: string
  // A token left out while the space already has one keeps it.
  credential:
    | { type: 'ssh' }
    | { type: 'token'; username?: string; token?: string }
    | { type: 'connection'; provider: string; via: ConnectionVia }
}

// --- connections (docs/api.md, "Connections") ----------------------------------------------

export type ConnectionVia = 'app' | 'user'
export type ConnectionErrorCode =
  | 'connection_expired'
  | 'connection_missing'
  | 'installation_gone'
  | 'access_lost'
  | 'provider_not_configured'
  | 'app_missing_permission'

export interface Connection {
  login: string
  accountId: string
  state: 'ok' | 'expired'
  connected: string
  // Absent when the platform does not expire tokens.
  refreshExpires?: string
  // How many spaces sync through this connection.
  spaces: number
  // The host said the account is an administrator of the host (Gitea, Forgejo): the token carries those rights.
  admin?: boolean
}

export interface ProviderCapabilities {
  createRepo: boolean
  installations: boolean
  connectAccount: boolean
  storageTarget: boolean
  partialSync: 'yes' | 'no' | 'unknown'
}

// A platform the server syncs spaces with, and the caller's connection to it.
export interface Provider {
  id: string
  type: string
  host: string
  name: string
  // 'site', or the tenant that owns a provider of its own.
  owner: 'site' | { tenant: string }
  capabilities: ProviderCapabilities
  configured: boolean
  app: { name: string; slug: string; htmlUrl: string; installUrl: string } | null
  // Whether people can connect their account.
  userAuthorization: boolean
  connection: Connection | null
  // Empty repositories that failed attempts to make a space left on the platform, newest first.
  leftovers: ConnectionLeftover[]
}

// A reminder: the repository was made and kept; `spaceName` is what the space was to be called.
// One entry of GET /api/admin/providers: a sign-in provider (with `repo` when it also holds repositories), or a provider
// that only holds repositories, with its repository side flat.
export interface AdminProviderItem {
  id: string
  kind: string
  name: string
  signin: boolean
  repos: boolean
  // A provider that only holds repositories.
  type?: string
  host?: string
  configured?: boolean
  partialSync?: 'yes' | 'no' | 'unknown'
  connections?: number
  spaces?: number
  // The repository side of a sign-in provider.
  repo?: {
    host: string
    configured: boolean
    partialSync: 'yes' | 'no' | 'unknown'
    connections: number
    spaces: number
  }
}

// What the administrators' list says of each repository provider (docs/api.md, "Providers").
export interface AdminProviderRow {
  id: string
  type: string
  name: string
  host: string
  configured: boolean
  partialSync: 'yes' | 'no' | 'unknown'
  connections: number
  spaces: number
}

// A host set up with an OAuth application (GitLab, Gitea, Forgejo, Codeberg); the secret is never returned.
export interface AdminForge {
  id: string
  type: string
  host: string
  name: string
  owner: 'site' | { tenant: string }
  configured: boolean
  clientId: string
  hasSecret: boolean
  // Where the host sends people back to: to be set as the application's redirect URI there ('' without a public address).
  callbackUrl: string
  // What the application is asked for (none for a host whose tokens are as wide as the person).
  scopes: string[]
  userAuthorization: boolean
  version: string
  checked?: string
  partialSync: 'yes' | 'no' | 'unknown'
  connections: number
  spaces: number
  updated: string
}

export interface CreateAdminForgeInput {
  id: string
  type: string
  host: string
  name?: string
  clientId: string
  clientSecret: string
}

export interface PatchAdminForgeInput {
  name?: string
  clientId?: string
  clientSecret?: string
}

export interface ConnectionLeftover {
  repoId: number
  fullName: string
  htmlUrl: string
  spaceName: string
  created: string
}

export interface ConnectionOwner {
  login: string
  type: 'User' | 'Organization'
  // Whose token creates the repository: the caller's own, or the installation's.
  via: ConnectionVia
  installationId: number | null
  // The caller's own account, where the App is not installed: no repository can be made there until it is.
  needsInstall: boolean
}

export interface ConnectionRepo {
  id: number
  fullName: string
  private: boolean
  defaultBranch: string
  htmlUrl: string
  canPush: boolean
}

export interface CursorPage<T> {
  items: T[]
  // Opaque; empty after the last page.
  next: string
}

// A new space stored on a provider (`POST /api/spaces`, `storage.sync`).
export interface NewSync {
  provider: string
  owner: string
  name: string
  private?: boolean
  description?: string
  // False connects the space to the one that exists already.
  create?: boolean
  // Also make an attachment repository, on the same owner and with the same credential (docs/api.md,
  // "Server storage"): `true` names it `<name>-assets`. Left out, the administrator's default applies.
  assets?: boolean | { name: string }
}

export interface CreateSpaceInput {
  name: string
  inbox?: string
  sync?: NewSync
  // Where the space is stored (`storage.mode`); left out, the default in effect (or "own" for a space with a remote).
  mode?: 'server' | 'own' | 'tenant'
}

// The settings of the server's one GitHub App as an administrator sees them: never a secret.
export type AdminGitHub =
  | { configured: false; callbackUrl: string; signinCallbackUrl?: string; webhookUrl?: string; provider?: AuthProvider }
  | {
      configured: true
      // Where GitHub sends people who sign in with the App's client: the App's callback URLs list it too.
      signinCallbackUrl?: string
      // The sign-in side of the same provider (turned on there, by `enabled`).
      provider?: AuthProvider
      appId: string
      clientId: string
      slug: string
      name: string
      owner: string
      htmlUrl: string
      installUrl: string
      privateKey: { set: boolean; fingerprint: string }
      clientSecret: { set: boolean }
      // The secret GitHub signs the App's webhook deliveries with: without it an organisation cannot connect its
      // installation. webhookUrl is where GitHub sends them ("" without the server's public address).
      webhookSecret: { set: boolean }
      webhookUrl: string
      callbackUrl: string
      permissions: Record<string, string>
      // Of administration, contents and metadata: what the App lacks. A warning only.
      missing: string[]
      userAuthorization: boolean
      checked: string
      spaces: number
      authorizations: number
      // Installations that lack a permission, from the last check.
      gaps: GitHubGap[]
      // Whether the App can be installed on any account (a person can only install it on their own account when it can); null: not known.
      anyAccount: boolean | null
    }

export interface GitHubGap {
  installationId: number
  account: string
  missing: string[]
}

// What the administrator's browser posts to GitHub to make the App (docs/api.md, "POST /api/admin/connections/{provider}/manifest").
export interface GitHubManifestStart {
  url: string
  manifest: Record<string, unknown>
}

// An installation of the site's GitHub App on a GitHub account, connected to its one owner on this server: a person
// (their own account) or an organisation (docs/api.md, "The GitHub provider", "Installations"). Each one's spaces sync
// only through what is their own.
export interface GitHubInstallation {
  id: number
  account: string
  accountType: 'User' | 'Organization'
  owner: { kind: 'person' | 'organization'; id: string }
  // The user who connected it (for an organisation, one of its administrators; the installation is the tenant's).
  claimedBy: string
  connected: string
  spaces: number
  // The installation's settings on GitHub: its repositories are chosen there, and it is uninstalled there.
  manageUrl: string
}
// An organisation's claim of an installation that waits for GitHub's signed event to say who installed it (error
// ""), or that failed when it came (not_installer, installation_taken), until it expires (10 minutes).
export interface GitHubPendingClaim {
  id: number
  account: string
  claimedBy: string
  started: string
  expires: string
  error: string
}
// The owner's installations, and their claims still waiting for GitHub.
export interface GitHubInstallationList {
  items: GitHubInstallation[]
  pending: GitHubPendingClaim[]
}
// The spaces that sync through an installation (at most 200 by name), and how many there are in all.
export interface GitHubInstallationSpaces {
  items: { id: string; name: string }[]
  total: number
}
// Whose installations a list is: the person's own, or the organisation's of the console.
export type GitHubInstallationsOf = 'mine' | 'org'

export interface PutAdminGitHubInput {
  appId?: string
  privateKey?: string
  // "" removes the client id and the secret.
  clientId?: string
  clientSecret?: string
  webhookSecret?: string
}

// A refusal of the daily-text endpoints says why with a stable code in `error`, next to `message`, and the
// thing it is about: header_reenter / header_missing (header), not_json, too_large, path_error /
// path_reveals_secret (field: text|source|author|link), link_uses_header, skip_too_large, bad_language (a language
// tag that is not one), api_not_found (an interface of the default that is not set up), builtin (what a built-in
// interface fixes; field).
export interface ReasonCode {
  code: string
  header?: string
  field?: string
}

export type ErrorCode =
  | 'template_not_found'
  | 'template_limit'
  | 'template_too_large'
  | 'file_too_large'
  | 'heir_required'
  | 'owner_required'
  | 'confirm_stale'
  | 'exports_full'
  | 'feature_off'
  | 'changes_expired'
  // The page is in the catalogue on this device but its text was not kept (offline).
  | 'not_offline'
  | 'token_limit'
  | 'quota_exceeded'
  | 'tenant_limit'
  | 'tenant_not_empty'
  | 'plan_in_use'
  | 'plan_limit'
  | 'default_tenant'
  | 'setting_locked'
  | 'setting_restricted'
  | 'setting_above_max'
  | 'mail_not_configured'
  | 'needs_mail'
  | 'needs_tenancy'
  | 'slug_taken'
  | 'slug_reserved'
  | 'org_limit'
  | 'mail_insecure'
  | 'mail_from_required'
  | 'mail_needs_public_url'
  | 'registration_closed'
  | 'invite_required'
  | 'people_limit'
  | 'password_reset_off'
  | 'email_domain_not_allowed'
  | 'email_domain_not_verified'
  | 'sso_default_tenant'
  | 'site_admin_explicit'
  | 'email_limit'
  | 'primary_email'
  | 'terms_required'
  | 'email_verified'
  | 'email_unverified'
  | 'email_taken'
  | 'email_not_ascii'
  | 'sso_only_with_registration'
  | 'sso_provider_required'
  | 'sso_required'
  | 'bad_language'
  | 'api_not_found'
  | 'builtin'
  | 'header_reenter'
  | 'header_missing'
  | 'not_json'
  | 'too_large'
  | 'path_error'
  | 'path_reveals_secret'
  | 'link_uses_header'
  | 'skip_too_large'
  | 'not_found'
  | 'invalid'
  | 'conflict'
  | 'exists'
  | 'unauthenticated'
  | 'forbidden'
  // A view as a person (docs/api.md, "Viewing as a person"): a route it does not reach, and a view that is over.
  | 'view_as_read_only'
  | 'view_as_ended'
  | 'no_permission'
  | 'space_disabled'
  | 'user_not_found'
  | 'user_disabled'
  | 'last_owner'
  | 'personal_space'
  | 'busy'
  | 'remote_required'
  | 'partial_unsupported'
  | 'no_storage_target'
  | 'not_implemented'
  | 'not_cached'
  | 'restoring'
  | 'space_full'
  | 'provider_not_configured'
  | 'not_connected'
  | 'owner_not_allowed'
  | 'name_taken'
  | 'provider_refused'
  | 'repo_not_in_installation'
  | 'installation_taken'
  | 'not_installer'
  | 'push_failed'
  | 'key_invalid'
  | 'app_rejected'
  | 'client_rejected'
  | 'provider_in_use'
  | 'provider_host_mismatch'
  | 'app_missing_permission'
  | 'app_not_installed'
  | 'public_url_needed'
  | 'rate_limited'
  | 'captcha_failed'
  | 'local_login_refused'
  | 'last_local_admin'
  | 'passkey_required'
  | 'passkey_failed'
  | 'recent_login_required'
  | 'too_many_passkeys'
  | 'passkey_unavailable'
  | 'last_method'
  | 'already_linked'
  | 'github_app_client'
  | 'domain_required'
  // The email domains (3.6 d2).
  | 'domain_public'
  | 'domain_suffix'
  | 'domain_exists'
  | 'domain_limit'
  | 'domain_claimed'
  | 'domain_code_expired'
  | 'domain_verified'
  | 'domain_managed'
  // Joining by email domain at sign-up (3.6 d5).
  | 'join_pending'
  | 'join_decided'
  | 'join_limit'
  | 'tenant_disabled'
  | 'confirm_required'
  | 'provider_in_use'
  | 'group_not_found'
  | 'group_synced'
  | 'group_not_synced'
  | 'group_limit'
  | 'group_in_use'
  | 'member_via_group'
  | 'unavailable'
  | 'internal'
  | 'network'
  // No answer in the time given (the request was called off; what it did on the server is not known).
  | 'timeout'
  | 'unknown'

// --- accounts -----------------------------------------------------------------

export type Role = 'admin' | 'member'

export interface User {
  id: string
  username: string
  name: string
  email: string
  emailVerified: boolean
  role: Role
  disabled: boolean
  // Whether this person must sign in with a passkey.
  passkeyRequired: boolean
  // An administrator of the whole server (3.3); false for one of a tenant. Absent in single-user mode.
  siteAdmin?: boolean
  // The tenant the person belongs to; absent in single-user mode, which has none.
  tenant?: TenantRef
  // The person's own limits (null follows the tenant); only the administrators' list and PATCH answer have it.
  quota?: OwnQuota
  created: string
  // The users list only: how many passkeys and single sign-on identities the person has.
  passkeys?: number
  sso?: number
  // The users list only: when the person was last active (absent: never).
  lastActive?: string
  // The storage policy does not ask the spaces this user owns for a remote (absent when false).
  remoteExempt?: boolean
  // One person's page only (GET /api/users/{id}): their addresses, as GET /api/account shows them.
  emails?: AccountEmail[]
}

// Why a session ended without the person signing out (docs/api.md, "Why a session ended").
export type EndedReason = 'expired' | 'signed_out_elsewhere' | 'password_changed' | 'disabled'
export const ENDED_REASONS: readonly EndedReason[] = ['expired', 'signed_out_elsewhere', 'password_changed', 'disabled']

// A permission key (docs/api.md, "Permission keys"): what a route outside spaces needs. The type is generated from the
// server's registry, so a renamed or removed key fails the typecheck.
import type { PermissionKey } from './generated/permissionKeys.ts'
export type { PermissionKey }

// One key of the registry (GET /api/admin/permissions).
export interface PermissionInfo {
  name: PermissionKey
  scope: 'site' | 'tenant' | 'space'
  // The tenant feature (a features.* setting) the key needs ("" for none).
  feature: string
  description: string
}

// The signed-in user as GET /api/session has them: with the keys they hold in their own tenant. Absent on a user the
// app put into the session itself (after signing in) until the session is read again.
export interface SessionUser extends User {
  permissions?: PermissionKey[]
}

// What every JSON sign-in answers: GET /api/session's `user` (with the keys the person holds) and `hold`.
export interface SignInAnswer {
  user: SessionUser
  hold: 'passkey' | 'password' | null
}

export interface SessionInfo {
  mode: 'single' | 'accounts'
  user: SessionUser | null
  // True while there is no user at all: the app shows the first-run page.
  setup: boolean
  // Why the session of this browser's cookie ended, when that is why nobody is signed in (absent otherwise).
  endedReason?: EndedReason
  // `passkey`: the signed-in person must register a passkey before anything else.
  hold?: 'passkey' | 'password' | null
  // Absent in single-user mode.
  methods?: { password: boolean }
  // The API token the request was made with; the app's own requests have none.
  token?: { id: string; name: string; access: TokenAccess; spaces: string[] } | null
  // In a tab that views the app as a person: the view (`user` is then the person, with no permissions).
  viewAs?: ViewAsInfo
}

// A view as a person (docs/api.md, "Viewing as a person"): who is looking, until when, and how many of the person's
// spaces it leaves out (personal ones, and those outside the administrator's organisation).
export interface ViewAsInfo {
  id: string
  viewer: { name: string }
  until: string
  hidden: number
}

// An API token (docs/api.md, "API tokens"). The secret is never in one of these.
export type TokenAccess = 'read' | 'write'
export type TokenState = 'active' | 'expired' | 'revoked'
export interface ApiTokenSpace {
  id: string
  // "" for a space that is gone or that the owner has left.
  name: string
}
export interface ApiToken {
  id: string
  name: string
  prefix: string
  access: TokenAccess
  spaces: ApiTokenSpace[]
  created: string
  expires: string | null
  lastUsed: string | null
  revoked: string | null
  state: TokenState
}
// The secret is in the response of the creating request only.
export interface CreatedToken {
  token: ApiToken
  secret: string
}
export interface AdminToken extends ApiToken {
  owner: { id: string; username: string; name: string }
}
export interface AdminTokenList {
  items: AdminToken[]
  next: string
}
// An OAuth grant: one approval of one client (an MCP client, say) by one person
// (docs/api.md, "OAuth for MCP clients"). It works like a token of that access
// and those spaces.
export interface Grant {
  id: string
  client: { name: string; id: string }
  access: TokenAccess
  spaces: ApiTokenSpace[]
  created: string
  // The last refresh; the creation until the first.
  lastUsed: string | null
}
export interface AdminGrant extends Grant {
  owner: { id: string; username: string; name: string }
}
export interface AdminGrantList {
  items: AdminGrant[]
  next: string
}
// What the consent page shows of an authorization request.
export interface ConsentInfo {
  client: { name: string; id: string; host: string }
  redirectHost: string
  // The host may show as letters that look like others.
  punycode: boolean
  scopes: TokenAccess[]
  spaces: { id: string; name: string; permissions: Permission[]; personal: boolean }[]
  user: { id: string; name: string }
}
export interface ConsentDecision {
  req: string
  approve: boolean
  spaces?: string[]
  access?: TokenAccess
}

export type ProviderKind = 'oidc' | 'saml' | 'github' | 'oauth2'

// One account the person has at a provider (GET /api/account/links, GET /api/users/{id}/links): one per provider. It
// signs them in (`signin`), holds their repositories (`repos`), or both: the GitHub link is one row for the two.
export interface Link {
  id: string
  providerId: string
  // The provider's display name.
  provider: string
  slug: string
  kind: ProviderKind | 'gitlab' | 'gitea' | ''
  label: string
  signin: boolean
  created: string
  // The account's name on the platform (the person's own list says it; an administrator's does not).
  login?: string
  // Repositories are connected through this link, or were and the person must authorise again (state 'expired').
  repos?: boolean
  state?: 'ok' | 'expired'
  refreshExpires?: string
  spaces?: number
  admin?: boolean
}
export interface Passkey {
  id: string
  name: string
  created: string
  lastUsed?: string | null
  synced: boolean
}
export interface AccountInfo {
  user: User
  hasPassword: boolean
  passkeys: Passkey[]
  // The groups the person is in, by name.
  groups: GroupRef[]
  // The account's addresses, the primary first: the primary alone while accounts.multipleEmails is off.
  emails?: AccountEmail[]
}

// One address of an account (docs/api.md, "More than one email address"). markedByAdmin: verified by a tenant
// administrator's mark alone, which the person may still confirm.
export interface AccountEmail {
  email: string
  primary: boolean
  verified: boolean
  markedByAdmin?: boolean
  // A claim of an address another account holds unverified: it has no row until its link is followed, and can be
  // removed (withdrawn).
  pending?: boolean
  created: string
}

// --- signing in (docs/api.md, "Signing in") -------------------------------------------

export type LoginMode = 'dual' | 'sso_first' | 'sso_redirect' | 'local_only'
export interface PublicProvider {
  slug: string
  kind: ProviderKind
  name: string
  // `''` or `preset:<name>`.
  icon: string
}
export interface AuthProviders {
  providers: PublicProvider[]
  loginMode: LoginMode
  local: boolean
  sso: boolean
  passkey: boolean
  // 3.3: whether the page links to registration and to password reset.
  register?: boolean
  forgot?: boolean
  // A person may register an organisation of their own (features.orgCreation).
  orgCreation?: boolean
  // 3.6 d4: a verified domain routes sign-in to a single sign-on, so the page asks for the address (or username)
  // first, and offers the "Sign in with SSO" entry (/login/sso).
  identifierFirst?: boolean
  ssoEntry?: boolean
}

// A site SSO provider offered to a tenant, as its administrators see it (read only): a domain of the tenant may be
// routed to it (3.6 d4).
export interface OfferedSSO {
  id: string
  slug: string
  kind: ProviderKind
  name: string
  icon: string
  enabled: boolean
}

// Where an address signs in (POST /api/auth/discover): the provider its domain is routed to, and whether the
// domain's organisation requires it.
export interface DiscoveredSSO {
  slug: string
  kind: ProviderKind
  name: string
  icon: string
  start: string
  enforced: boolean
}

// --- mail, registration, email verification and password reset (3.3) ---
export type MailSecurity = 'starttls' | 'tls' | 'none'
export interface MailSettings {
  enabled: boolean
  host: string
  port: number
  security: MailSecurity
  username: string
  hasPassword: boolean
  from: string
  fromName: string
  replyTo: string
  status: { ok: boolean; checked: string | null; message: string }
  queue: { waiting: number; sentLastHour: number; failedLastHour: number }
}
export interface MailPatch {
  enabled?: boolean
  host?: string
  port?: number
  security?: MailSecurity
  username?: string
  // Write only: absent keeps the stored one, '' clears it.
  password?: string
  from?: string
  fromName?: string
  replyTo?: string
}
export interface MailTestResult {
  ok: boolean
  message?: string
}
// Who may make an account (registration.mode): `open` any way, `invite` only a holder of an invitation (an
// organisation's own email-domain join keeps working), `closed` nobody, invitations included.
export type RegistrationMode = 'open' | 'invite' | 'closed'
// What registration offers: a place in the tenant (`open`: its mode is open and mail is on) and, when the site allows
// it, an organisation of the person's own (`orgCreation.open`). `mode` is the tenant's registration mode, for the words
// to a person who cannot sign up here (use the link of an invitation, or ask an administrator).
export interface RegisterInfo {
  open: boolean
  mode: RegistrationMode
  orgCreation: { open: boolean }
  // Some verified domain joins people to its organisation at sign-up (3.6 d5): the form is offered while open is false.
  domainJoin: boolean
  tenant?: { slug: string; name: string }
  domains?: string[]
  termsUrl?: string
  privacyUrl?: string
  password?: { minLength: number }
}
// The organisation a registration makes, and the registration answer that made one (renamed: the slug was taken in
// the meantime and a number was added).
export interface RegisterOrg {
  name: string
  slug: string
}
export interface RegisteredOrg extends RegisterOrg {
  renamed: boolean
}
// What following a registration link made: an account (signed in; `tenant` the organisation it made, `joined` the one
// it joined by its address's domain), or a request to join that waits for approval (nobody is signed in).
export type RegisterConfirmAnswer =
  (SignInAnswer & { tenant?: RegisteredOrg; joined?: RegisterOrg; request?: undefined }) | { request: JoinStatus }
export interface RegisterInput {
  tenant?: string
  org?: RegisterOrg
  username: string
  name: string
  email: string
  password: string
  acceptTerms?: boolean
  captcha?: CaptchaAnswer
  lang?: string
}

export type CaptchaProvider = 'image' | 'turnstile' | 'recaptcha' | 'hcaptcha'
export type CaptchaContext = 'login' | 'invite' | 'setup' | 'forgot' | 'register'
export interface CaptchaChallenge {
  required: boolean
  provider?: CaptchaProvider
  siteKey?: string
  id?: string
  image?: string
}
// What a form sends back: the typed answer of an image, or a widget's token.
export type CaptchaAnswer = { id: string; answer: string } | { token: string }

export interface PasskeyCeremony {
  ceremony: string
  options: Record<string, unknown>
}

export interface Invite {
  id: string
  role: Role
  created: string
  expires: string
  createdBy: string
}
// The token is in the response of the creating request only.
export interface CreatedInvite {
  id: string
  token: string
  role: Role
  created: string
  expires: string
  // What a new tenant's first administrator's invite fills in (see InviteCheck).
  username?: string
  name?: string
  email?: string
}
// An account an administrator made (POST /api/admin/users). The link is in this response only: `signIn` is `link` when
// the person sets a password with it, `sso` when their organisation requires its single sign-on (no link then).
export interface CreatedAccount {
  user: User
  signIn: 'link' | 'sso'
  link: string | null
  expires: string | null
  mail: 'sent' | 'none' | 'limited'
}
export interface InviteCheck {
  role: Role
  expires: string
  // What the invite starts the form with ("" when it says nothing); the address becomes the account's, not verified.
  username?: string
  name?: string
  email?: string
}

export type OidcIntent = 'login' | 'setup' | 'invite'
export type ProviderIntent = OidcIntent

export type BatchPagesInput = { op: 'move'; ids: string[]; folder: string } | { op: 'delete'; ids: string[] }

// --- members (docs/api.md, "Members") -----------------------------------------------

export interface Member {
  userId: string
  username: string
  name: string
  role: SpaceRole
  // True for the caller.
  you: boolean
  // A group of theirs is a member of the space too; the role the person's own
  // role and those groups give together.
  viaGroups?: boolean
  effectiveRole?: SpaceRole
}

// --- groups (docs/api.md, "Groups") --------------------------------------------------

export type GroupSource = 'local' | 'synced'
// What everyone may see of a group.
export interface GroupRef {
  id: string
  name: string
  source: GroupSource
}
// A group as an administrator sees it.
export interface Group extends GroupRef {
  description: string
  people: number
  spaces: number
  mappedFrom: { provider: string; value: string }[]
  created: string
  updated: string
}
export interface GroupList {
  items: Group[]
  next: string
}
export interface GroupPerson {
  userId: string
  username: string
  name: string
  disabled: boolean
  from: { provider: string; value: string }[]
  added: string
}
export interface GroupPersonList {
  items: GroupPerson[]
  next: string
}
export interface GroupSpaceRef {
  id: string
  name: string
  role: SpaceRole
}
export interface GroupSpaceList {
  items: GroupSpaceRef[]
  next: string
}
// A group that is a member of a space.
export interface GroupMember {
  groupId: string
  name: string
  source: GroupSource
  role: SpaceRole
  people: number
  // The caller is in the group.
  yours: boolean
}
export interface MembersResponse {
  members: Member[]
  groups: GroupMember[]
}
export interface EffectiveMember {
  userId: string
  username: string
  name: string
  role: SpaceRole | ''
  directRole: SpaceRole | ''
  groups: { groupId: string; name: string; role: SpaceRole }[]
  you: boolean
}
export interface EffectiveMemberList {
  items: EffectiveMember[]
  next: string
}
// What a provider does with its groups claim.
export interface GroupsMapping {
  enabled: boolean
  claim: string
  missing: 'keep' | 'clear'
  map: { value: string; groupId: string }[]
  // Only people whose claim names one of these sign in, at every sign-in (3.6 d3); [] lets everyone through.
  allow: string[]
}

// --- administration (docs/api.md, "Administration") -----------------------------------

export interface AdminSpace {
  id: string
  name: string
  personal: boolean
  // The folder quick notes go to.
  inbox: string
  created: string
  disabled: boolean
  // The first owner; null when there is none.
  owner: { id: string; username: string; name: string } | null
  members: number
  // The last stored measurement; null until the space has been measured.
  sizeBytes: number | null
  sizeAt: string | null
  sync: SyncState
  // What the copy on the server is, per space (docs/api.md, "Server storage").
  storage: AdminSpaceStorage
  // Where the bytes live, and whether they count toward quotas (docs/api.md, "Measurements this adds").
  location: SpaceLocation
  counted: boolean
  // The audited view of the space this session has open; null when there is none (docs/api.md, "Viewing a space").
  view: AdminViewState | null
}
export interface AdminViewState {
  since: string
  until: string
}
// `server`: only on this server; `own`: the person's own remote; `tenant`: the organisation's own storage target;
// `site`: the site's storage organisation. The first and the last count toward quotas, the other two do not.
export type SpaceLocation = 'server' | 'own' | 'tenant' | 'site'
export interface AdminSpaceStorage {
  cache: CacheState
  pinned: boolean
  exempt: boolean
  lastUsed: string | null
  cloneBytes: number
  indexBytes: number
  assetCacheBytes: number
  assets: 'space' | 'repository'
  remote: RemoteNotice | null
}
// What GET /api/admin/storage answers: only what is measured or derived. The policy is the storage.* and cache.*
// settings (AdminStorage puts them together).
// `git.version` and the summary's `spaces.restoring` are the site's: a tenant administrator is not told them.
export interface AdminStorageBody {
  remotes: { since: string | null }
  git: { version?: string; partialClone: boolean }
  // `exempt` is the site's: a tenant administrator is not told it.
  affected: { inGrace: number; overdue: number; exempt?: number }
  // What the server holds (docs/api.md, "GET /api/admin/storage"); `diskFreeBytes` is for site administrators only.
  summary?: StorageSummary
}
// The storage policy as the page shows it: the settings read from /api/admin/settings, with what is derived.
export interface AdminStorage {
  // require: null, 'any', or the id of a provider (2.7b's connections, such as 'github'); the setting's "" is null here.
  remotes: { require: string | null; scope: 'new' | 'all'; graceDays: number; since: string | null }
  // `fromFlag` is the site's: read only here.
  hosts: { allowed: string[]; fromFlag?: boolean }
  localSpaces: { maxBytes: number | null }
  cache: {
    partial: boolean
    evictAfterDays: number | null
    minIdleHours: number
    clonesBytes: number | null
    minFreeBytes: number | null
    assetsBytes: number
  }
  git: AdminStorageBody['git']
  affected: AdminStorageBody['affected']
}
export interface StorageSummary {
  // null for the whole server.
  tenant: string | null
  spaces: { local: number; full: number; partial: number; evicted: number; archived: number; restoring?: number }
  people: number
  dormant: number
  heldBytes: number
  usageBytes: number
  assetCacheBytes: number
  recycle: { items: number; bytes: number }
  exports: { items: number; bytes: number }
  remotes: { inGrace: number; overdue: number }
  // How long the recycle bin keeps an item.
  recycleDays: number
  diskFreeBytes?: number
  measuredAt: string | null
}
// What the page changes: any of the sections, each partly; each field is one setting.
export interface PutAdminStorage {
  remotes?: Partial<Omit<AdminStorage['remotes'], 'since'>>
  hosts?: { allowed?: string[] }
  localSpaces?: { maxBytes?: number | null }
  cache?: Partial<AdminStorage['cache']>
}
export interface StorageStatus {
  spaces: { local: number; full: number; partial: number; evicted: number; restoring: number }
  cloneBytes: number
  indexBytes: number
  assetCacheBytes: number
  clonesBudget: number | null
  assetsBudget: number | null
  restoresWaiting: number
  remotes: { inGrace: number; overdue: number }
}
export interface AdminSpaceList {
  items: AdminSpace[]
  // The cursor of the next page, '' after the last.
  next: string
}
export interface AdminSpacesParams {
  q?: string
  // Keeps the spaces in one cache state, or (overdue) those past their deadline for a remote.
  cache?: CacheState
  overdue?: boolean
  cursor?: string
  limit?: number
  // 3.5: sorting and the other filters of the spaces' list (docs/api.md, "GET /api/admin/spaces").
  sort?: SpaceSort
  order?: 'asc' | 'desc'
  tenant?: string
  owner?: string
  remote?: 'none' | 'ok' | 'offline' | 'error'
  deadline?: 'grace' | 'overdue'
  pinned?: boolean
  exempt?: boolean
  idleDays?: number
}
export type SpaceSort = 'created' | 'name' | 'bytes' | 'lastUsed'
export interface SpaceSize {
  sizeBytes: number
  sizeAt: string
}
export interface DeletedSpace {
  // Where the repository went, relative to the data directory; '' if it could not be moved.
  trashed: string
  cacheTrashed: string
  warnings: string[]
}
export interface ServerStatus {
  version: string
  startedAt: string
  uptimeSeconds: number
  database: 'sqlite' | 'postgres'
  spaces: number
  users: number
  openSpaces: number
  // Absent where the platform cannot say.
  diskFreeBytes?: number
  goVersion: string
  build: BuildInfo
  storage: StorageStatus
}

// Which build the server is (site administrators only). `commit` and `time` are empty when the binary was built outside a
// Git checkout. `timeKind` is `build` when the build injected its own time, else `commit`: the time of the commit.
export interface BuildInfo {
  commit: string
  time: string
  timeKind: 'build' | 'commit' | ''
  modified: boolean
}

// --- home: the daily text and favourites (docs/api.md, "Home") ---------------------

// Where a space takes its daily text from: a folder (and those below it), a tag or a type, with filters.
export interface DailySource {
  folder?: string
  tag?: string
  type?: string
  filter?: PropFilter[]
}
// A source for every language beside sources by language (the language codes are lower-case keys).
export type DailySettings = DailySource & { [lang: string]: DailySource | string | PropFilter[] | undefined }

export interface DailyQuote {
  text: string
  source?: string
  author?: string
  link?: string
  fetched?: string
  stale?: boolean
}
export type DailyReason = 'no_source' | 'no_match' | 'unavailable' | 'out_of_range'
// What `GET …/daily` says: a page of the space, a text from the administrators, or nothing to show.
export interface DailyAnswer {
  page: (PageSummary & { body: string; bodyTruncated?: boolean; author?: string }) | null
  count?: number
  quote?: DailyQuote
  origin?: 'space' | 'external' | 'site'
  reason?: DailyReason
}

// What a reader may choose of a built-in service (docs/api.md, "Built-in interfaces"): each value is sent as a
// repeated query parameter; choosing none is choosing all.
export interface DailyOption {
  key: string
  param: string
  // multi: any number of the values; path: one of them (the default when none is chosen).
  kind: 'multi' | 'path'
  default?: string
  values: { value: string; label: { zh: string; en: string } }[]
}
// The values chosen, by interface id and option key; absent is all.
export type DailyOptionChoice = Record<string, Record<string, string[]>>

export interface DailyPrefs {
  inherit: boolean
  // In order: the first for the day, the next ones when it does not answer and for "another".
  apis: string[]
  options?: DailyOptionChoice
  // The space's own list of sources for each language, when it has one: api, the administrators' lines (`list`) or
  // pages of this space (`own`). Read in place of `apis` for a language that has a list.
  sources?: Record<string, DailySpaceSource[]>
  // What is the matter with an interface the space chose: `disabled` (switched off; kept, marked and skipped) or `gone`
  // (it no longer exists; left out of the answer, and nothing else is shown in its place).
  unavailable?: Record<string, 'disabled' | 'gone'>
  // A built-in interface that answers in more than one language lists them in `languages`.
  available: { id: string; name: string; lang: string; languages?: string[]; options?: DailyOption[] }[]
  // The languages the settings pages have a tab for (besides "all languages").
  languages?: string[]
  default: { kind: string } | null
  // What the administrators let spaces do (absent: everything). A field that is `false` is set for the whole
  // server: the space's page shows it disabled and says so.
  allowed?: { own?: boolean; api?: boolean }
}
export interface DailyAllow {
  own: boolean
  api: boolean
}

export type FavoriteKind = 'page' | 'folder' | 'view'
export interface Favorite {
  kind: FavoriteKind
  target: string
  name: string
  path?: string
  // A view: the folder or tag it belongs to.
  of?: string
  ofKind?: 'folder' | 'tag'
  missing: boolean
  created: string
}
export interface FavoriteList {
  items: Favorite[]
  limit: number
}
export interface FavoriteRef {
  kind: FavoriteKind
  target: string
}

export interface DailyApiHeader {
  name: string
  set?: boolean
  value?: string
}
export interface DailyApi {
  id: string
  name: string
  lang: string
  url: string
  headers: DailyApiHeader[]
  text: string
  source: string
  author: string
  link: string
  refresh: 'daily' | 'hourly' | 'minutes'
  refreshMinutes: number
  skipNew: boolean
  rps: number
  enabled: boolean
  // How a language is written in {lang} for this service: {"zh-Hans": "zh"}.
  langMap?: Record<string, string>
  // A built-in interface: its address, paths, link and language mapping are fixed.
  builtin?: boolean
  // The languages a built-in interface answers in (lang is the first).
  languages?: string[]
}
// A service the server knows how to ask: switched on and off, no address to fill in.
export interface DailyProvider {
  id: string
  name: string
  languages: string[]
  site: string
  note: string
  enabled: boolean
  refresh: 'daily' | 'hourly' | 'minutes'
  minutes: number
  rps: number
  maxRps?: number
  skipNew: boolean
  options?: DailyOption[]
}
export interface KnownLanguage {
  tag: string
  name: string
  english: string
}
export type DailySpaceSource =
  // An interface the administrators switched off stays in the list, marked `available: false`, and is skipped.
  | { kind: 'api'; id: string; options?: Record<string, string[]>; available?: boolean; reason?: string }
  | { kind: 'list' }
  | { kind: 'own'; source: DailySource }
export type DailySourceItem =
  | { kind: 'api'; id: string; options?: Record<string, string[]> }
  | { kind: 'list' }
  | { kind: 'space'; space: string; source: DailySettings }
export type DailyDefault =
  | null
  | { kind: 'list' }
  | { kind: 'space'; space: string; source: DailySettings }
  // Each language lists its interfaces in order.
  | { kind: 'api'; apis: Record<string, string[]>; options?: DailyOptionChoice }
  // Each language lists sources of any kind, in order: tried in turn by day, the next when one has nothing.
  | { kind: 'sources'; sources: Record<string, DailySourceItem[]> }
// What GET /api/admin/daily answers: the interfaces, providers and lines. The default, the languages and what spaces
// may do are the daily.* settings (AdminDaily puts them together).
export interface AdminDailyBody {
  // The common languages to pick from; any valid tag may still be typed.
  knownLanguages: KnownLanguage[]
  apis: DailyApi[]
  providers: DailyProvider[]
  lines: { total: number; byLang: Record<string, number>; max: number }
}
export interface AdminDaily extends AdminDailyBody {
  default: DailyDefault
  languages: string[]
  // Whether spaces may use a source of their own and pick an external interface.
  allow: DailyAllow
}
export interface DailyTryResult {
  ok: boolean
  status?: number
  url?: string
  message?: string
  // Why it did not work, as a code (see ReasonCode), with what it is about.
  error?: string
  header?: string
  field?: string
  raw?: string
  rawTruncated?: boolean
  quote?: DailyQuote
  pathErrors?: { field: string; code: string; message: string }[]
}
export type LinesFormat = 'lines' | 'markdown' | 'csv'
// A line of the administrators' own text: the text, and who said it and where it is from, each split off.
export interface DailyLine {
  text: string
  author?: string
  source?: string
}

// --- security settings, providers and the audit log (docs/api.md, "Administration") ------------

export type LockoutScope = 'ip_account' | 'ip' | 'account'
export type CaptchaWhen = 'always' | 'after_failures'
export interface CaptchaContextSettings {
  on: boolean
  when: CaptchaWhen
  failures: number
}
export interface SecurityNotice {
  code: 'secret_unreadable' | 'passkey_host_changed' | (string & {})
  message: string
  slug?: string
  field?: string
}
// What GET /api/admin/security answers: what the sign-in policy means now, and the captcha's keys (the secret only as
// set or not). The policy itself is the signin.*, sessions.*, captcha.* and features.passkeys settings.
export interface SecurityBody {
  login: { effectiveMode: LoginMode; ssoOnlyInEffect: boolean; ssoAvailable: boolean }
  passkeys: { available: boolean; required: number; requiredWithout: number }
  captcha: { siteKey: string; hasSecret: boolean }
  passwordAdmins: number
  notices: SecurityNotice[]
}
// The security page's view: the settings and what is derived, put together (api/settingsFold.ts).
export interface SecuritySettings {
  login: {
    mode: LoginMode
    effectiveMode: LoginMode
    ssoOnly: boolean
    ssoOnlyInEffect: boolean
    ssoAvailable: boolean
  }
  passkeys: { enabled: boolean; available: boolean; requireRoles: Role[]; required: number; requiredWithout: number }
  captcha: {
    provider: CaptchaProvider
    siteKey: string
    hasSecret: boolean
    contexts: Record<CaptchaContext, CaptchaContextSettings>
  }
  lockout: { enabled: boolean; minutes: number; scope: LockoutScope }
  limits: {
    perMinute: number
    maxFailures: number
    windowMinutes: number
    sessionIdleDays: number
    sessionMaxDays: number
  }
  passwordAdmins: number
  notices: SecurityNotice[]
}
// What `PUT /api/admin/security` takes: the captcha's credentials only; the secret is write-only ("" removes it).
export interface CaptchaCredentials {
  siteKey?: string
  secret?: string
}

export interface OidcSettings {
  issuer: string
  clientId: string
  hasSecret: boolean
  scopes: string[]
  redirectUrl: string
}
export interface SamlSettings {
  metadataUrl: string
  metadataXml: string
  idpEntityId: string
  entityId: string
  acsUrl: string
  metadataPath: string
  attributes: { email: string; name: string; username: string }
  nameIdFormat: string
  signRequests: boolean
  allowIdpInitiated: boolean
  certificate: string
  certificateExpires: string
}
// The paths into the userinfo answer of a plain OAuth 2.0 provider: dotted, e.g. `data.user.id` or `emails.0.email`.
export interface OAuth2Fields {
  subject: string
  email: string
  emailVerified: string
  name: string
  username: string
}
export type OAuth2AuthMethod = 'client_secret_basic' | 'client_secret_post'
export interface OAuth2Settings {
  authorizationEndpoint: string
  tokenEndpoint: string
  userinfoEndpoint: string
  scopes: string[]
  authMethod: OAuth2AuthMethod
  fields: OAuth2Fields
  clientId: string
  hasSecret: boolean
  redirectUrl: string
}
export interface GithubSettings {
  baseUrl: string
  clientId: string
  hasSecret: boolean
  redirectUrl: string
  // The client is the GitHub App's: it is set with the App, not here.
  app?: boolean
}
// Social sign-in a site offers on every sign-in page, or an organisation's single sign-on: chosen when a provider is
// made, never changed.
export type ProviderClass = 'social' | 'sso'
// The presets of social sign-in that ship with the server (GET /api/admin/provider-templates).
export type ProviderTemplateId = 'google' | 'microsoft' | 'apple' | 'github' | 'gitlab-com'
export interface ProviderTemplate {
  id: ProviderTemplateId
  name: string
  kind: ProviderKind
  class: ProviderClass
  icon: string
  // The OpenID Connect issuer ({tenantid} in Microsoft's), or GitHub's base address.
  issuer: string
  authorizationEndpoint?: string
  tokenEndpoint?: string
  jwksUri?: string
  scopes: string[]
  responseMode?: 'form_post'
  // What the administrator gives besides the client id: a client secret, or Apple's team id, key id and key.
  secret: 'client' | 'apple'
  common: boolean
  region: string
}
// What Sign in with Apple's client secret is signed with: the key itself is never shown.
export interface AppleSettings {
  teamId: string
  keyId: string
  hasKey: boolean
}
export interface AuthProvider {
  // A site provider's id is its slug.
  id: string
  // "site" or a tenant id.
  owner: string
  slug: string
  kind: ProviderKind
  class: ProviderClass
  // The template it was made from ('' for none): its issuer and addresses are the template's.
  template: ProviderTemplateId | ''
  apple?: AppleSettings
  name: string
  icon: string
  enabled: boolean
  order: number
  mergeVerifiedEmail: boolean
  // The tenant the provider is aimed at: its new accounts land there, as that tenant's registration mode allows (a tenant's own provider: that tenant).
  tenant: string
  // A site SSO provider is offered to every tenant ('all') or a list of tenant ids; null for any other (3.6 d3).
  tenants: 'all' | string[] | null
  // The verified domains of a tenant's own provider routed to it ([] for any other).
  domains: string[]
  groups: GroupsMapping
  oidc?: OidcSettings
  saml?: SamlSettings
  github?: GithubSettings
  oauth2?: OAuth2Settings
  status: { ok: boolean; checked: string; message: string }
  // A provider people sign in with always has `signin`; `repos` says it also holds repositories (the GitHub row once its App is set up).
  signin?: boolean
  repos?: boolean
  people: number
  created: string
  updated: string
}
// What a create or a patch sends: the common fields and the kind's own block, with `clientSecret` where there is one.
export interface ProviderInput {
  slug?: string
  owner?: string
  kind?: ProviderKind
  // A create from a template: it fills the kind, the class, the addresses, the scopes, the name and the icon.
  template?: ProviderTemplateId
  class?: ProviderClass
  clientId?: string
  clientSecret?: string
  apple?: { teamId?: string; keyId?: string; privateKey?: string }
  name?: string
  icon?: string
  enabled?: boolean
  order?: number
  mergeVerifiedEmail?: boolean
  tenants?: 'all' | string[]
  groups?: Partial<GroupsMapping>
  oidc?: Partial<Pick<OidcSettings, 'issuer' | 'clientId' | 'scopes'>> & { clientSecret?: string }
  saml?: Partial<
    Pick<
      SamlSettings,
      'metadataUrl' | 'metadataXml' | 'entityId' | 'attributes' | 'nameIdFormat' | 'signRequests' | 'allowIdpInitiated'
    >
  >
  github?: Partial<Pick<GithubSettings, 'baseUrl' | 'clientId'>> & { clientSecret?: string }
  oauth2?: Partial<
    Pick<
      OAuth2Settings,
      'authorizationEndpoint' | 'tokenEndpoint' | 'userinfoEndpoint' | 'scopes' | 'authMethod' | 'clientId'
    >
  > & { fields?: Partial<OAuth2Fields>; clientSecret?: string }
}

// What a row of the audit log is about (docs/api.md, "Audit log"): the core categories are always written, the optional
// ones (signins, reads, downloads, api) are switched by the settings audit.*.
export type AuditCategory = 'security' | 'admin' | 'data' | 'system' | 'signins' | 'reads' | 'downloads' | 'api'
export type AuditCapacity = 'site_admin' | 'tenant_admin'
// The registry of kinds is the server's (GET /api/admin/audit/kinds), so a kind is a word here.
export type AuditKind = string
export interface AuditItem {
  id: string
  at: string
  kind: AuditKind
  category: AuditCategory
  capacity: AuditCapacity | null
  actor: string | null
  user: string | null
  username: string | null
  ip: string
  detail: Record<string, unknown>
}
// The filters of GET /api/admin/audit and of its CSV download; the audit page keeps them in its address.
export interface AuditFilters {
  category: AuditCategory[]
  kind: string
  user: string
  actor: string
  capacity: AuditCapacity | ''
  // A day, YYYY-MM-DD (both ends included).
  from: string
  to: string
  // Site administrators: one tenant's id.
  tenant: string
}
// GET /api/admin/audit/kinds: the registry of kinds, for the filters.
export interface AuditCategoryInfo {
  id: AuditCategory
  core: boolean
  // The setting that switches an optional category; null for a core one.
  setting: string | null
  // Whether an optional category is written in the caller's scope.
  on: boolean
  kinds: { name: string; site: boolean }[]
}
export interface AuditKinds {
  categories: AuditCategoryInfo[]
}
export interface AuditList {
  items: AuditItem[]
  next: string
}
// POST /api/admin/audit/purge (site administrators): `category` is a category or "all", `before` a time or a day, both
// optional. With dry_run it only counts (`AuditPurgeCount`); otherwise it starts a job of kind `audit.purge` (202 `{job}`)
// whose `done` is the rows deleted so far and `total` 0.
export interface AuditPurgeInput {
  category?: AuditCategory | 'all'
  before?: string
}
export interface AuditPurgeCount {
  dryRun: boolean
  // How many rows would be deleted, at most 1,000,000: `more` says the count stopped there.
  count: number
  more: boolean
  category: AuditCategory | 'all'
  before: string
}
export interface AuditPurgeStarted {
  job: Job
}

// --- offline sync (docs/api.md, "Offline sync") -------------------------------------
//
// The light summary of a page that a catalogue and a change list hold.
export interface CatalogEntry {
  id: string
  path: string
  folder: string
  title?: string
  label: string
  untitled: boolean
  tags: string[]
  type: string
  created?: string
  updated: string
  rev: string
}
// A page that was removed, in a change list.
export interface RemovedEntry {
  id: string
  deleted: true
}
export type ChangeItem = CatalogEntry | RemovedEntry
export const isRemoved = (e: ChangeItem): e is RemovedEntry => 'deleted' in e && e.deleted === true

export interface CatalogBatch {
  items: CatalogEntry[]
  // "" after the last batch.
  next: string
  // The change cursor as it was before the walk began; in the first batch only.
  changes?: string
  tree: string
  settings: string
}
export interface ChangeList {
  items: ChangeItem[]
  next: string
  more: boolean
  tree: string
  settings: string
}
export interface SyncPages {
  pages: Page[]
  missing?: string[]
  // What did not fit in the answer, in the order asked.
  rest?: string[]
}
export interface SyncHeads {
  heads: Record<string, string>
  missing?: string[]
}

// --- tenants and layered settings (3.3, docs/api.md "Tenants, registration and mail") ---

export interface TenantRef {
  id: string
  slug: string
  name: string
}
export type TenantQuotaKey = 'people' | 'spaces' | 'bytes' | 'pages'
// What a tenant uses, as measured (docs/api.md, "Quotas"); `over` lists the totals it is over.
export interface TenantUsage {
  people: number
  spaces: number
  bytes: number
  pages: number
  measuredAt: string | null
  over: TenantQuotaKey[]
}
export interface Tenant extends TenantRef {
  default: boolean
  disabled: boolean
  // Who may make an account in the tenant: its own registration mode (the site's, for the default tenant).
  registration: { mode: RegistrationMode }
  quota: Record<TenantQuotaKey, number | null>
  // The plan the tenant has, null for none.
  plan: PlanRef | null
  // How the tenant came to be: with the database, by a site administrator, or by a person who registered it (then
  // `createdBy` is that person's id).
  origin: TenantOrigin
  createdBy: string
  usage: TenantUsage
  created: string
  updated: string
}
export type TenantOrigin = 'default' | 'admin' | 'self'
// GET /api/admin/tenants/{tenant}/overview: a tenant's page in one call.
export interface TenantOverview {
  tenant: Tenant
  createdByName: string
  groups: number
  admins: { id: string; username: string; name: string; siteAdmin: boolean; disabled: boolean }[]
  adminCount: number
  featuresOff: string[]
  recentAdminActivity: {
    id: string
    at: string
    kind: string
    category: string
    capacity: 'site_admin' | 'tenant_admin' | null
    actor: string | null
    username: string
  }[]
}
// One place the console may act (GET /api/admin/scopes): the whole site, or a tenant the person administers, with what
// they act as there.
export interface ConsoleScope {
  kind: 'site' | 'tenant'
  // The tenant's id and name; absent for the site.
  id?: string
  name?: string
  // tenant_admin when the tenant's own roles give them everything they hold there, site_admin when some of it is held
  // only across the site.
  capacity: 'site_admin' | 'tenant_admin'
  // The keys they hold in that scope, as a request in it is held to (a tenant's: its tenant keys).
  permissions: PermissionKey[]
}
export interface TenantList {
  items: Tenant[]
  next: string
  // How many tenants there are in all.
  count: number
}
// Whether tenants besides the default one may exist (features.tenancy), and how many there are.
export interface Tenancy {
  enabled: boolean
  tenants: number
}
export type CreatedTenant = Tenant & { invite?: CreatedInvite }

// --- plans (docs/api.md, "Plans") ---

export interface PlanRef {
  slug: string
  name: string
}
// One rule of a plan: a value, a lock or a restriction, with no dates. A limit the tenant also sets must cap
// (restrict with a max, or lock) when it has a value.
export interface PlanRule {
  value?: unknown
  lower?: SettingLower | null
}
export interface Plan extends PlanRef {
  description: string
  rules: Record<string, PlanRule>
  tenants: number
}
export interface PlanWrite {
  name?: string
  description?: string
  // A key with null leaves the plan.
  rules?: Record<string, PlanRule | null>
}
// What a change of plan cleared: the manual rules the tenant had.
export interface ClearedRule {
  key: string
  value: unknown
  lower: SettingLower | null
  from?: string
  until?: string
}

// --- quotas and usage (3.3) ---

export type PersonQuotaKey = 'spaces' | 'bytes' | 'pages'
// A person's own limits: a number, or null to follow the tenant and the site.
export type OwnQuota = Record<PersonQuotaKey, number | null>
// A quota that is over: the person's own (scope person) or the tenant's total (scope tenant).
export interface UsageNotice {
  quota: TenantQuotaKey
  scope: 'person' | 'tenant'
  limit: number
  used: number
}
// GET /api/account/usage. limit null is none.
export interface AccountUsage {
  spaces: { used: number; limit: number | null }
  bytes: { used: number; limit: number | null }
  pages: { used: number; limit: number | null }
  measuredAt: string | null
  notices: UsageNotice[]
}
export interface PersonUsage {
  user: { id: string; username: string; name: string }
  tenant: string
  spaces: number
  bytes: number
  pages: number
  limits: Record<PersonQuotaKey, number | null>
  // The person's own limits (null follows the tenant), what the edit form starts with.
  own: OwnQuota
  // false for an own limit a lock or restriction put on the setting later forbids (kept, in force again when lifted).
  ownInEffect: Record<PersonQuotaKey, boolean>
  over: PersonQuotaKey[]
  measuredAt: string | null
}
export interface AdminUsageList {
  items: PersonUsage[]
  next: string
}

export type SettingLayer = 'site' | 'tenant' | 'space' | 'person'
export type SettingKind = 'boolean' | 'choice' | 'limit' | 'list' | 'text' | 'doc'
// What a layer lets the layers below it do.
export interface SettingLower {
  mode: 'allow' | 'tighten' | 'restrict' | 'lock'
  options?: (string | boolean)[]
  max?: number
  // Why the rule is there, at most 200 characters, shown to the layers below beside the setting. On a write, a rule
  // without one keeps the stored note; an empty one clears it.
  note?: string
}
// What stricter means for a key, which the lower mode tighten needs; a key without one cannot be tightened.
// higher/lower: a larger/smaller number (or true/false); order: later in the choices; subset/superset: fewer/more
// entries; document: field by field, by the setting's own rule (captcha.contexts).
export type Strictness = 'higher' | 'lower' | 'order' | 'subset' | 'superset' | 'document'
// The rule a tenant's layer is held to, as the tenant is shown it. floor: under tighten, the value the tenant's own
// must be at least as strict as.
// by: the site's own layer (site) or its rule for this tenant (rule).
export type SettingReason = SettingLower & { by: 'site' | 'rule'; floor?: unknown }
export interface SiteSetting {
  key: string
  kind: SettingKind
  layers: SettingLayer[]
  // The values of a choice setting, loosest first.
  choices?: string[]
  site: { value: unknown; lower: SettingLower; fromFlag?: boolean }
  strictness?: Strictness
  // The site's own value governs its own people only and holds no organisation: the site's layer takes no rule, only
  // a rule for one tenant holds it.
  ownPeopleOnly?: boolean
  // The server's own, with no tenant layer: the reason, in words.
  serverWide?: string
  // Set by PUT when the new rule makes this many tenants' values ineffective.
  affected?: number
  // Set by PUT of storage.remotes.*: how many spaces the requirement asks something of.
  remotes?: { inGrace: number; overdue: number; exempt: number }
}
export type RuleSource = 'manual' | 'billing' | 'plan'
// One rule for a tenant. inEffect: it is the one applied (its window holds, no stronger source covers it, and the
// site's lock over everyone does not beat it); underlying: it applies when the one in effect ends or is removed.
export interface SettingRule {
  source: RuleSource
  value: unknown
  lower: SettingLower | null
  from?: string
  until?: string
  inEffect: boolean
  underlying?: boolean
  setBy?: string
}
// What a site administrator writes as the manual rule for one tenant: any of the fields; from and until in RFC 3339.
export interface RuleWrite {
  value?: unknown
  lower?: SettingLower | null
  from?: string
  until?: string
}
export interface TenantSetting {
  key: string
  kind: SettingKind
  layers: SettingLayer[]
  site: { value: unknown; lower: SettingLower }
  choices?: string[]
  strictness?: Strictness
  // The rule the tenant's layer is held to (the site's, or its rule for this tenant) with its note; absent when the
  // site allows everything.
  reason?: SettingReason
  // value and lower are the tenant's own. rules are the rules for this one tenant, strongest first (at most one per
  // source): a value in place of the site's, and the rule the tenant's layer is held to (a lock forces the value, a
  // restriction bounds the tenant's), each with the window it counts in. siteLocked says the rule in effect is a lock.
  // next is the next time a window starts or ends.
  tenant: {
    value: unknown
    lower: SettingLower | null
    inEffect: boolean
    siteLocked?: boolean
    rules?: SettingRule[]
    next?: string
  } | null
  effective: unknown
}
// What PUT answers for a tenant's setting: with a warning when a dated manual rule replaced an undated one.
export type TenantSettingWritten = TenantSetting & { warning?: { code: string; keys: string[]; message: string } }
// The features of a tenant (docs/api.md, "Features"): whole things a tenant may or may not have. A route that belongs to
// one answers 403 feature_off while it is off; the app hides what needs it (useFeature).
export type FeatureKey =
  | 'features.tokens'
  | 'features.mcp'
  | 'features.offline'
  | 'features.tenancy'
  | 'features.passkeys'
  | 'features.adminView'
  | 'features.auditDownload'
  | 'features.tenantAudit'
  | 'features.ownSso'
  | 'features.domains'
  | 'features.managedAccounts'
  | 'features.idpAdmins'
  | 'features.ownCaptcha'
  | 'features.ownConnections'
  | 'features.ownDaily'
  | 'features.githubConnect'
  | 'mail.ownSender'
  | 'accounts.multipleEmails'
  | 'features.orgCreation'
// What GET /api/settings answers: the features as the caller's tenant has them, and the settings a space or a person can
// change.
export interface MySettings {
  features: Record<string, boolean>
  settings: MySetting[]
}
// What GET /api/settings says of a setting that a space or a person can change.
export interface MySetting {
  key: string
  effective: unknown
  space?: { canChange: boolean; inEffect: boolean; source?: 'site' | 'tenant' }
  person?: { canChange: boolean; inEffect: boolean; source?: 'site' | 'tenant' }
  // storage.assets only: what a person may ask for when they make a space, and whether a lock leaves no choice.
  allowed?: string[]
  locked?: boolean
  // storage.mode only: the allowed modes that cannot be used now ("tenant" while the organisation has no storage target).
  missing?: string[]
}

// --- storage management (docs/api.md, "Storage management") --------------------------------

export type PeopleSort = 'held' | 'bytes' | 'lastActive' | 'created' | 'spaces'
export interface PeopleParams {
  tenant?: string
  q?: string
  sort?: PeopleSort
  order?: 'asc' | 'desc'
  cursor?: string
  limit?: number
  dormant?: boolean
  remote?: 'none' | 'some'
  deadline?: 'grace' | 'overdue'
  inactiveDays?: number
  disabled?: boolean
}
// One person and what they hold. `usageBytes` is what counts toward their quota, `heldBytes` what the server keeps for them.
export interface StoragePerson {
  user: { id: string; username: string; name: string; disabled: boolean }
  tenant: string
  created: string
  lastActive: string | null
  firstWrite: string | null
  dormant: boolean
  connected: boolean
  spaces: number
  remoteSpaces: number
  evictedSpaces: number
  pinnedSpaces: number
  graceSpaces: number
  overdueSpaces: number
  usageBytes: number
  heldBytes: number
  measuredAt: string | null
}
export interface StoragePeopleList {
  items: StoragePerson[]
  next: string
}
// A person's item, with the spaces counted for them (never a personal one for a tenant administrator).
export type StoragePersonDetail = StoragePerson & { items: AdminSpace[] }

export type RecycleKind = 'space' | 'user'
// `ready` is the only state in which an item can be restored or purged; the others are an action in progress.
export type RecycleState = 'ready' | 'moving' | 'restoring' | 'purging'
// An item recycled with a person.
export interface RecycleChild {
  id: string
  space: { id: string; name: string; personal: boolean }
  bytes: number
}
export interface RecycleItem {
  id: string
  kind: RecycleKind
  tenant: string
  space: { id: string; name: string; personal: boolean } | null
  user: { id: string; username: string } | null
  with: RecycleChild[]
  bytes: number
  deletedAt: string
  deletedBy: { id: string; username: string }
  purgeAt: string
  reason: string
  state: RecycleState
  // The job that holds the item (follow it with GET /admin/jobs/{id}); null at rest.
  job: string | null
  // false (with `problems`) while the item is moving, being restored or purged, or when a directory is missing.
  restorable: boolean
  problems: string[]
}
export interface RecycleList {
  items: RecycleItem[]
  next: string
}
// `name` is as it was when the thing was deleted ('' when it was not kept); `reason` is a code (docs/api.md, "What a
// restore leaves out"): `deleted` for a person or group, and for a space `busy`, `owner_required`, `conflict` or `failed`.
export interface RestoreSkipped {
  user?: string
  group?: string
  space?: string
  name: string
  reason: string
}
export interface RestoreResult {
  restored: { item: string; space?: { id: string; name: string }; user?: User; spaces?: string[] }
  // What was left out, by name: a person or a group that is gone, or a space that stayed in the bin.
  skipped: RestoreSkipped[]
  transfers?: { space: string; name: string; to: string; toUsername: string }[]
  left?: { space: string; name: string; role: string }[]
}

export interface SpaceDeletionPreview {
  space: { id: string; name: string }
  pages: number
  bytes: number
  heldBytes: number
  attachments: { inSpace: number | null; repository: boolean }
  members: number
  groups: number
  drafts: number
  // `url` is '' when the space has none.
  remote: { url: string; kept: boolean }
  assetsRemote: { url: string; kept: boolean }
  cache: CacheState
  exportable: boolean
  recycleDays: number
}
export interface PlanSpace {
  id: string
  name: string
  personal?: boolean
}
export interface UserDeletionPlan {
  user: { id: string; username: string }
  recycle: { space: PlanSpace; bytes: number }[]
  // `why`: named (you chose), editor (the earliest editor) or heir (the one asked for).
  transfer: { space: PlanSpace; to: { id: string; username: string }; why: string; credential?: string }[]
  leave: { space: PlanSpace; role: string }[]
  revoke: {
    tokens: number
    grants: number
    passkeys: number
    identities: number
    github: boolean
    sessions: number
    connections: { provider: string; name: string }[]
  }
  groups: number
  needsHeir: PlanSpace[]
  blockers: { reason: 'self' | 'last_admin' | 'last_local_admin' | string; message?: string }[]
  bytes: number
  recycleDays: number
}
export type ConfirmAction =
  | 'delete_spaces'
  | 'delete_users'
  | 'purge'
  | 'restore'
  | 'evict'
  | 'pin'
  | 'unpin'
  | 'disable_spaces'
  | 'enable_spaces'
  | 'disable_users'
  | 'enable_users'
  | 'export'
export interface ConfirmRequest {
  // change_capabilities is not a bulk action: switching a feature off that is in use, `key` the feature, `tenant` a
  // site rule's tenant (none: the site's value); its token goes with the setting's change.
  action: ConfirmAction | 'change_capabilities' | 'domain_join_off'
  spaces?: string[]
  users?: string[]
  items?: string[]
  heir?: string
  transfers?: Record<string, string>
  key?: string
  tenant?: string
  // domain_join_off: the domain whose waiting requests to join are cancelled (with tenant).
  domain?: string
}
// One line of change_capabilities' plan: a tenant that uses the feature, and how much of it.
// One tenant of the preview: its claims of features.domains, or its own sign-in providers of features.ownSso.
export interface CapabilityPlanLine {
  tenant: string
  slug: string
  name: string
  feature: string
  domains?: number
  verified?: number
  // features.domains: the requests to join by a domain that wait (3.6 d5).
  requests?: number
  providers?: number
  enabled?: number
}
// One line of a confirmation's plan: the preview of a deletion, or what an action does to one thing.
export type ConfirmPlanLine = (
  | SpaceDeletionPreview
  | UserDeletionPlan
  | { space: { id: string; name: string }; action: string }
  | { user: { id: string; username: string }; action: string }
  | { item: string; kind: RecycleKind; name: string; bytes: number; action: string; deletedAt: string }
  | CapabilityPlanLine
) &
  Record<string, unknown>
export interface ConfirmResult {
  confirm: string
  expires: string
  count: number
  plan: ConfirmPlanLine[]
  bytes: number
  // The people (ids) whose shared spaces have nobody to inherit them, with those spaces.
  needsHeir: { user: string; spaces: PlanSpace[] }[]
  // Why something cannot be done: `user` or `space` is an id.
  blockers: { user?: string; space?: string; reason: string }[]
}
// A background action: one shape for every kind (docs/api.md, "Jobs"). `total` and `done` count the items (the spaces of
// an export), `bytesDone` the bytes written; `subject` is what a recycle job works on; `action` is a bulk job's;
// `expires` is when an export's file goes.
export type JobState = 'queued' | 'running' | 'done' | 'interrupted' | 'failed' | 'cancelled'
export type JobKind = 'bulk' | 'export' | 'recycle.move' | 'recycle.restore' | 'recycle.purge' | 'audit.purge'
export interface Job {
  id: string
  kind: JobKind
  subject: string
  action?: ConfirmAction
  state: JobState
  total: number
  done: number
  bytesTotal: number
  bytesDone: number
  failed: { id: string; error: string; code?: string }[]
  error: string
  created: string
  updated: string
  expires: string | null
  madeBy: { id: string; username: string }
}
export interface JobList {
  items: Job[]
  next: string
}
export interface JobsParams {
  kind?: JobKind
  state?: JobState
  tenant?: string
  cursor?: string
  limit?: number
}
export type ExportState = 'queued' | 'running' | 'ready' | 'failed'
export interface ExportRec {
  id: string
  tenant: string
  madeBy: { id: string; username: string }
  // `space` for a live space, `item` for one in the recycle bin; `name` as it was, `spaceId` the space either way and
  // `open` whether it exists now and the caller may open it.
  spaces: { space?: string; item?: string; name: string; spaceId: string; open: boolean }[]
  history: boolean
  state: ExportState
  error?: string
  warnings: string[]
  bytes: number
  created: string
  expires: string
  // The job's progress: the spaces made into the zip so far, and the bytes written.
  total: number
  done: number
  bytesDone: number
}
export interface ExportList {
  items: ExportRec[]
  next: string
}
export interface ExportRequest {
  spaces?: string[]
  items?: string[]
  user?: string | null
  history?: boolean
}
// What starting a view answers (docs/api.md, "Viewing a space").
export interface SpaceView {
  space: string
  until: string
}

// --- the users list, paged by the server, and a person's page (3.10) ---
export type UserSort = 'created' | 'name' | 'lastActive'
export interface UsersParams {
  q?: string
  limit?: number
  cursor?: string
  sort?: UserSort
  order?: 'asc' | 'desc'
  role?: 'member' | 'admin' | 'site'
  state?: 'enabled' | 'disabled'
  inactiveDays?: number
  activeDays?: number
  tenant?: string
}
export interface UsersPage {
  users: User[]
  next: string
}
// One sign-in of a person, for a site administrator. `method` is password, passkey, invite, registration, setup or sso:<slug>.
export interface PersonSession {
  id: string
  userAgent: string
  method: string
  created: string
  lastUsed: string
  held?: boolean
  // The session of this very request: the signed-in administrator's own.
  current?: boolean
}
// What a provider's test sign-in found (read once).
export interface ProviderTestResult {
  subject: string
  email: string
  emailTrusted: boolean
  name: string
  username: string
  // The paths of a plain OAuth 2.0 provider that found nothing in the userinfo answer ([] for any other kind).
  unresolved: string[]
  at: string
}
export interface PersonMemberships {
  spaces: { id: string; name: string; role: string; personal: boolean }[]
  groups: { id: string; name: string; source: string }[]
}

// --- the notification centre and the person's preferences (docs/api.md, "Notifications", "Preferences") ---

// The registry of kinds (store.NotifyKinds; i18n/notify.test.ts keeps this list equal to the server's, and every kind
// with its texts). `problems` is derived (read from the index, not a row); `mention` and `todo` are reserved: nothing
// makes them yet.
export const NOTIFY_KINDS = [
  'problems',
  'invite',
  'mention',
  'sync',
  'todo',
  'space_taken_over',
  'space_transferred',
  'account_changed',
  'password_set',
  'email_released',
  'viewed_as',
  'link_reauth',
  'domain_lapsed',
  'join_requested',
  'join_approved',
] as const
export type NotifyKind = (typeof NOTIFY_KINDS)[number]

export interface NotificationItem {
  // `problems:<space id>` for a derived one.
  id: string
  // A kind this client does not know is shown as a plain notice, so the type stays open.
  kind: string
  // What it is about: a space id.
  ref: string
  payload: Record<string, unknown>
  // When it happened; null for a derived one, which is a state that holds now.
  at: string | null
  read: boolean
  derived?: boolean
}
export interface NotificationList {
  items: NotificationItem[]
  next: string
  // Counted up to 100.
  unread: number
  // How many more spaces have problems than the list names (at most 50 are); absent when none.
  moreProblems?: number
}
export interface NotificationsParams {
  kind?: string
  unread?: boolean
  cursor?: string
  limit?: number
}
export interface PrefsAnswer {
  // false: the person has no personal space to keep them in, so this browser does.
  personal: boolean
  prefs: Record<string, unknown>
}

// --- secrets (write-only: the API says whether one is set and when it changed, never its value) ---
export interface SecretMeta {
  name: string
  set: boolean
  updatedAt?: string
}

// An email domain a tenant claimed (3.6 d2, docs/api.md "Email domains and the SSO entry"). `domain` is the key (IDNA,
// punycode, lower case), `display` the same as people read it. `code` and `records` come only with the answer that
// made the code: it is kept as a hash, so it is shown that once. `missing` is verified, but the daily recheck has not
// found the record since `missingSince`; `lapsed` the recheck cancelled it.
export type DomainState = 'unverified' | 'verified' | 'missing' | 'lapsed'
export type DomainCheckResult = 'found' | 'missing' | 'wrong_value' | 'dns_error'
export interface DomainCheck {
  at: string
  by: 'manual' | 'daily'
  result: DomainCheckResult
  via: '' | 'txt' | 'cname'
  seen: string[]
  error: string
}
export interface DomainRecords {
  txt: { name: string; value: string }
  cname: { name: string; target: string } | null
}
export interface TenantDomain {
  domain: string
  display: string
  // A label mixes scripts (a Cyrillic а in a Latin name): a lookalike to warn of, not a refusal.
  mixedScript: boolean
  state: DomainState
  verifiedAt: string | null
  missingSince: string | null
  lapsedAt: string | null
  checkedAt: string | null
  nextCheckAt: string | null
  codeExpires: string | null
  codeExpired: boolean
  lastCheck: DomainCheck | null
  nextCheckAllowedAt: string | null
  restrictOrgs: boolean
  // The provider sign-in is routed to (3.6 d3), and how people of the domain join the organisation at sign-up (d5).
  provider: string | null
  join: DomainJoinMode
  recordName: string
  code: string | null
  records: DomainRecords | null
  created: string
  // The claim's tenant, in the site's list only.
  tenant?: { id: string; slug: string; name: string }
}
export interface TenantDomainList {
  domains: TenantDomain[]
  siteHost: string
}

// Joining an organisation by email domain at sign-up (3.6 d5, docs/api.md "Joining an organisation"): per verified
// domain, off, auto (the confirmed address makes an account in the organisation) or request (an administrator approves).
export type DomainJoinMode = 'off' | 'auto' | 'request'
export type JoinRequestState = 'pending' | 'approved' | 'denied' | 'withdrawn' | 'expired' | 'cancelled'
// A request as the organisation's administrators list it. `user` is the account an approval made.
export interface JoinRequest {
  id: string
  username: string
  name: string
  email: string
  domain: string
  state: JoinRequestState
  created: string
  expires: string
  decidedAt: string | null
  decidedBy: { id: string; name: string } | null
  user: string | null
}
export interface JoinRequestList {
  items: JoinRequest[]
  next: string | null
}
// A request as the person who made it sees it (the waiting page).
export interface JoinStatus {
  state: JoinRequestState
  tenant: { slug: string; name: string }
  username: string
  email: string
  created: string
  expires: string
  decidedAt: string | null
}
// One line of domain_join_off's plan: the requests that wait on the domain, which turning its join off or removing it
// cancels.
export interface JoinOffPlanLine {
  tenant: string
  domain: string
  requests: number
}
