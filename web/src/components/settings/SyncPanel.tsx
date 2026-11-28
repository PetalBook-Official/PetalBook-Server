// Settings of one space that live in the settings dialog: the inbox folder, the
// way into the import page, and where the space keeps its Git repository. A
// space either lives on the server only, or follows a remote repository; the
// sync section connects it, shows how the connection is doing and disconnects it.

import { useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import {
  AlertTriangle,
  Check,
  CircleAlert,
  CirclePause,
  Cloud,
  CloudOff,
  Copy,
  ExternalLink,
  KeyRound,
  RefreshCw,
  Save,
  Unlink,
} from 'lucide-react'
import { toast } from 'sonner'
import { api, ApiError, spaceApi } from '@/api/client'
import {
  useAcceptRemote,
  useConnectRemote,
  useDisconnectRemote,
  useActiveSpace,
  useProvider,
  useConnectionOwners,
  useConnectionRepos,
  useRemote,
  useSession,
  useSyncRemote,
  useUpdateSpace,
} from '@/api/hooks'
import type { ConnectRemoteInput, ConnectionVia, Remote, RemoteKey, RemoteState, SpaceInfo } from '@/api/types'
import { Field, FormMessage } from '@/components/auth/Field'
import { ErrorState, PageFallback, Spinner } from '@/components/common'
import { can } from '@/lib/permissions'
import { SettingsNote, SettingsSection } from './SettingsParts'
import { AssetsSection } from './AssetsSection'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { NativeSelect } from '@/components/ui/native-select'
import { Input } from '@/components/ui/input'
import { checkFolderPath } from '@/lib/folders'
import { absoluteTime, relativeTime } from '@/lib/time'
import { spaceUrls } from '@/lib/urls'
import { settingsPath } from '@/components/settings/sections'
import { t } from '@/i18n'
import { tg } from '@/i18n/github'

// What a space owner sees under Sync: the remote repository, connected,
// followed and disconnected as before. Moved from the old space settings page.
export function SyncPanel({ space }: { space: string }) {
  const { spaces, resolving } = useActiveSpace()
  const current = spaces.find((s) => s.id === space)
  if (resolving) return <PageFallback />
  if (!current) return <SettingsNote>{t.editor.spaceMissing}</SettingsNote>
  if (current.disabled) return <SettingsNote>{t.spaces.disabledTitle}</SettingsNote>
  if (!can(current, 'manage')) return <SettingsNote>{t.remote.ownerOnly}</SettingsNote>
  return (
    <div className="space-y-8">
      <RemoteSection space={space} />
      <AssetsSection space={space} />
    </div>
  )
}

// A way into the import page; the import itself has a page of its own.
export function ImportSection({ space }: { space: string }) {
  return (
    <SettingsSection title={t.import.settingsTitle} id="import-title">
      <p className="text-sm text-muted-foreground">{t.import.settingsHelp}</p>
      <div>
        <Button variant="outline" render={<Link to={spaceUrls(space).import()} />}>
          <ExternalLink aria-hidden="true" />
          {t.import.settingsLink}
        </Button>
      </div>
    </SettingsSection>
  )
}

// Where quick notes go. Single-user mode sets it with `--inbox`, so it is only
// shown there; in accounts mode the owner can change it, and nothing is moved.
export function InboxSection({ space, readOnly = false, why }: { space: SpaceInfo; readOnly?: boolean; why?: string }) {
  const update = useUpdateSpace()
  const [value, setValue] = useState(space.inbox)
  const [error, setError] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (update.isPending) return
    const checked = checkFolderPath(value)
    if ('error' in checked)
      return setError(checked.error === t.folderRules.empty ? t.inboxSetting.invalid : checked.error)
    setError('')
    if (checked.path === space.inbox) return setValue(checked.path)
    try {
      const saved = await update.mutateAsync({ id: space.id, inbox: checked.path })
      setValue(saved.inbox)
      toast.success(t.inboxSetting.saved)
    } catch (err) {
      setError(
        err instanceof ApiError && err.code === 'invalid'
          ? err.message
          : t.fmt.labelled(t.inboxSetting.failed, err instanceof Error ? err.message : ''),
      )
    }
  }

  return (
    <SettingsSection title={t.inboxSetting.title} id="inbox-title">
      <form className="grid gap-3" onSubmit={(e) => void submit(e)} noValidate>
        <Field
          label={t.inboxSetting.label}
          help={readOnly ? (why ?? t.inboxSetting.readOnly) : t.inboxSetting.help}
          value={readOnly ? space.inbox : value}
          onChange={(e) => setValue(e.target.value)}
          readOnly={readOnly}
          error={error}
          autoComplete="off"
          spellCheck={false}
        />
        {!readOnly && (
          <div>
            <Button type="submit" disabled={update.isPending || value === space.inbox}>
              {update.isPending ? <Spinner /> : <Save aria-hidden="true" />}
              {t.inboxSetting.save}
            </Button>
          </div>
        )}
      </form>
    </SettingsSection>
  )
}

function RemoteSection({ space }: { space: string }) {
  const remote = useRemote(space)
  return (
    <SettingsSection title={t.remote.title} id="remote-title">
      {remote.isPending ? (
        <PageFallback />
      ) : remote.isError ? (
        <ErrorState
          message={t.fmt.labelled(t.remote.loadFailed, remote.error.message)}
          onRetry={() => void remote.refetch()}
        />
      ) : remote.data === null ? (
        <>
          <p className="text-sm text-muted-foreground">{t.remote.intro}</p>
          <ConnectForm space={space} existing={null} />
        </>
      ) : (
        <Following space={space} remote={remote.data} />
      )}
    </SettingsSection>
  )
}

// --- the state of the connection ----------------------------------------------------

const STATE_ICON: Record<RemoteState, typeof Cloud> = {
  ok: Cloud,
  offline: CloudOff,
  rewritten: CirclePause,
  error: CircleAlert,
}

function StateBadge({ state }: { state: RemoteState }) {
  const Icon = STATE_ICON[state]
  return (
    <Badge
      variant={state === 'ok' ? 'secondary' : state === 'error' ? 'destructive' : 'outline'}
      className={state === 'offline' || state === 'rewritten' ? 'border-warning text-warning' : ''}
    >
      <Icon />
      {t.remote.state[state]}
    </Badge>
  )
}

function Following({ space, remote }: { space: string; remote: Remote }) {
  const sync = useSyncRemote(space)
  const disconnect = useDisconnectRemote(space)
  const [editing, setEditing] = useState(false)
  const [confirmDisconnect, setConfirmDisconnect] = useState(false)

  const syncNow = async () => {
    try {
      const r = await sync.mutateAsync()
      if (r.state === 'ok') toast.success(t.remote.synced)
      else toast.warning(t.fmt.labelled(t.remote.syncFailed, r.error || t.remote.state[r.state]))
    } catch (e) {
      toast.error(t.fmt.labelled(t.remote.syncFailed, e instanceof Error ? e.message : ''))
    }
  }
  const doDisconnect = async () => {
    try {
      await disconnect.mutateAsync()
      setConfirmDisconnect(false)
      toast.success(t.remote.disconnected)
    } catch (e) {
      setConfirmDisconnect(false)
      toast.error(t.fmt.labelled(t.remote.disconnectFailed, e instanceof Error ? e.message : ''))
    }
  }

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <StateBadge state={remote.state} />
        <span className="text-sm text-muted-foreground" role="status">
          {remote.lastSync
            ? t.fmt.labelled(
                t.remote.lastSync,
                t.fmt.note(relativeTime(remote.lastSync), absoluteTime(remote.lastSync)),
              )
            : t.remote.never}
        </span>
      </div>

      {remote.state === 'offline' && (
        <Notice icon={CloudOff} title={t.remote.offlineTitle} body={t.remote.offlineBody} error={remote.error} />
      )}
      {remote.state === 'error' && <ErrorNotice remote={remote} />}
      {remote.state === 'rewritten' && <Rewritten space={space} remote={remote} />}

      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
        <dt className="text-muted-foreground">{t.remote.address}</dt>
        <dd className="font-mono text-xs break-all">{remote.url}</dd>
        <dt className="text-muted-foreground">{t.remote.branch}</dt>
        <dd>{remote.branch}</dd>
        <dt className="text-muted-foreground">{t.remote.credentialLabel}</dt>
        <dd className="break-words">{credentialText(remote)}</dd>
        {remote.hostKey && (
          <>
            <dt className="text-muted-foreground">{t.remote.hostKey}</dt>
            <dd>
              <code className="font-mono text-xs break-all">{remote.hostKey}</code>
              <p className="mt-0.5 text-xs text-muted-foreground">{t.remote.hostKeyHelp}</p>
            </dd>
          </>
        )}
      </dl>

      {remote.credential.type === 'ssh' && (
        <PublicKey
          publicKey={remote.credential.publicKey}
          fingerprint={remote.credential.fingerprint}
          title={t.remote.sshKeyOf}
        />
      )}

      <div className="flex flex-wrap gap-2">
        <Button onClick={() => void syncNow()} disabled={sync.isPending}>
          {sync.isPending ? <Spinner /> : <RefreshCw />}
          {sync.isPending ? t.remote.syncing : t.remote.syncNow}
        </Button>
        <Button variant="outline" onClick={() => setEditing((v) => !v)} aria-expanded={editing}>
          {editing ? t.remote.cancelEdit : t.remote.editConnection}
        </Button>
        <Button variant="outline" onClick={() => setConfirmDisconnect(true)}>
          <Unlink aria-hidden="true" />
          {t.remote.disconnect}
        </Button>
      </div>

      {editing && (
        <div className="rounded-lg border bg-muted/20 p-4">
          <ConnectForm space={space} existing={remote} onDone={() => setEditing(false)} />
        </div>
      )}

      <AlertDialog open={confirmDisconnect} onOpenChange={setConfirmDisconnect}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t.remote.disconnectTitle}</AlertDialogTitle>
            <AlertDialogDescription>{t.remote.disconnectDescription}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t.remote.disconnectCancel}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={disconnect.isPending}
              onClick={() => void doDisconnect()}
            >
              {t.remote.disconnectConfirm}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// What the credential is, in words; a GitHub one names what it syncs with and holds no secret.
function credentialText(remote: Remote): string {
  const c = remote.credential
  if (c.type === 'ssh') return t.remote.ssh
  if (c.type === 'token') return t.remote.tokenOf(c.username)
  return c.via === 'app' ? tg.sync.viaAppOf(c.account) : tg.sync.viaUserOf(c.user.username, c.login)
}

// An error state that has a code of its own says what to do about it: the GitHub ones are fixed by
// connecting again, by installing the App again, or by an administrator.
function ErrorNotice({ remote }: { remote: Remote }) {
  const session = useSession()
  const github = useProvider('github', remote.credential.type === 'connection')
  const [busy, setBusy] = useState(false)
  const me = session.data?.user?.id
  const c = remote.credential
  const owner = c.type === 'connection' && c.via === 'user' ? c.user : null
  const mine = owner !== null && owner.id === me
  const reconnect = async () => {
    setBusy(true)
    try {
      const { url } = await api.connectionStart('github', window.location.pathname)
      window.location.assign(url)
    } catch (e) {
      setBusy(false)
      toast.error(t.fmt.labelled(tg.sync.reconnectFailed, e instanceof Error ? e.message : ''))
    }
  }
  const code = remote.errorCode
  const known = (tg.sync.codes as Record<string, { title: string; body: (who: string) => string }>)[code]
  if (!known || !Object.hasOwn(tg.sync.codes, code))
    return (
      <Notice
        icon={AlertTriangle}
        title={t.remote.errorTitle}
        body={t.remote.errorBody}
        error={remote.error}
        tone="error"
      />
    )
  return (
    <div
      role="alert"
      className="grid grid-cols-1 gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm"
    >
      <p className="flex items-center gap-1.5 font-medium">
        <AlertTriangle className="size-4 shrink-0" />
        {known.title}
      </p>
      {remote.error && <p className="font-mono text-xs break-words">{remote.error}</p>}
      <p className="text-muted-foreground">{known.body(owner?.username ?? '')}</p>
      <p className="text-muted-foreground">{tg.sync.keptSafe}</p>
      <div className="flex flex-wrap items-center gap-2">
        {(code === 'connection_expired' || code === 'connection_missing') && mine && (
          <Button type="button" size="sm" onClick={() => void reconnect()} disabled={busy}>
            {busy ? <Spinner /> : <RefreshCw aria-hidden="true" />}
            {tg.sync.reconnect}
          </Button>
        )}
        {code === 'access_lost' && mine && (
          <Button size="sm" render={<Link to={settingsPath('connections')} />}>
            <RefreshCw aria-hidden="true" />
            {tg.sync.accountLinks}
          </Button>
        )}
        {code === 'installation_gone' && github.data?.app && (
          <a
            className="inline-flex items-center gap-1 text-xs break-all underline underline-offset-4"
            href={github.data.app.installUrl}
            target="_blank"
            rel="noreferrer noopener"
          >
            {tg.sync.openInstall}
            <ExternalLink className="size-3 shrink-0" />
          </a>
        )}
      </div>
    </div>
  )
}

function Notice({
  icon: Icon,
  title,
  body,
  error,
  tone = 'warning',
}: {
  icon: typeof Cloud
  title: string
  body: string
  error?: string
  tone?: 'warning' | 'error'
}) {
  return (
    <div
      role="alert"
      className={`grid gap-1 rounded-lg border p-3 text-sm ${tone === 'error' ? 'border-destructive/40 bg-destructive/5' : 'border-warning/50 bg-warning/5'}`}
    >
      <p className="flex items-center gap-1.5 font-medium">
        <Icon className="size-4 shrink-0" />
        {title}
      </p>
      {error && <p className="font-mono text-xs break-words">{error}</p>}
      <p className="text-muted-foreground">{body}</p>
    </div>
  )
}

// The remote's history no longer contains what the server last saw: writes are
// paused until the owner accepts the new history.
function Rewritten({ space, remote }: { space: string; remote: Remote }) {
  const accept = useAcceptRemote(space)
  const sync = useSyncRemote(space)
  const [open, setOpen] = useState(false)
  const r = remote.rewritten

  const doAccept = async () => {
    if (!r) return
    try {
      await accept.mutateAsync(r.remote)
      setOpen(false)
      toast.success(t.remote.accepted)
    } catch (e) {
      setOpen(false)
      // The remote moved on again: look at it afresh before deciding.
      if (e instanceof ApiError && e.code === 'conflict') {
        toast.warning(t.remote.acceptStale)
        void sync.mutateAsync().catch(() => undefined)
      } else toast.error(t.fmt.labelled(t.remote.acceptFailed, e instanceof Error ? e.message : ''))
    }
  }

  return (
    <div role="alert" className="grid gap-3 rounded-lg border border-warning/50 bg-warning/5 p-3 text-sm">
      <p className="flex items-center gap-1.5 font-medium">
        <CirclePause className="size-4 shrink-0" />
        {t.remote.rewrittenTitle}
      </p>
      <p className="text-muted-foreground">{t.remote.rewrittenBody}</p>
      {r && (
        <dl className="grid gap-2">
          <Commit label={t.remote.rewrittenLastGood} commit={r.lastGood} />
          <Commit label={t.remote.rewrittenRemote} commit={r.remote} />
        </dl>
      )}
      <p className="text-muted-foreground">{t.remote.rewrittenChoices}</p>
      <div>
        <Button variant="outline" onClick={() => setOpen(true)} disabled={!r}>
          <Check aria-hidden="true" />
          {t.remote.accept}
        </Button>
      </div>
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t.remote.acceptTitle}</AlertDialogTitle>
            <AlertDialogDescription>{t.remote.acceptDescription}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t.remote.acceptCancel}</AlertDialogCancel>
            <AlertDialogAction disabled={accept.isPending} onClick={() => void doAccept()}>
              {t.remote.acceptConfirm}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function Commit({ label, commit }: { label: string; commit: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd>
        <code className="font-mono text-xs break-all select-all">{commit}</code>
      </dd>
    </div>
  )
}

// --- the public key ----------------------------------------------------------------

function PublicKey({ publicKey, fingerprint, title }: { publicKey: string; fingerprint: string; title: string }) {
  const area = useRef<HTMLTextAreaElement>(null)
  const [copied, setCopied] = useState(false)
  const [failed, setFailed] = useState(false)
  const copy = async () => {
    setFailed(false)
    try {
      await navigator.clipboard.writeText(publicKey)
      setCopied(true)
    } catch {
      // No clipboard permission: select it so Ctrl+C works.
      area.current?.select()
      setFailed(true)
    }
  }
  return (
    <div className="grid gap-2 rounded-lg border p-3" role="group" aria-label={title}>
      <h3 className="text-sm font-medium">{t.remote.addKeyTitle}</h3>
      <Textarea
        ref={area}
        readOnly
        rows={3}
        value={publicKey}
        aria-label={t.remote.publicKey}
        className="font-mono text-xs break-all"
        onFocus={(e) => e.currentTarget.select()}
      />
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" variant="outline" size="sm" onClick={() => void copy()}>
          {copied ? <Check /> : <Copy />}
          {copied ? t.remote.copied : t.remote.copyKey}
        </Button>
        <span role="status" className="text-xs text-destructive">
          {failed ? t.remote.copyFailed : ''}
        </span>
      </div>
      <p className="text-xs text-muted-foreground">{t.remote.addKeyHow}</p>
      <p className="text-xs text-muted-foreground">
        {t.remote.fingerprint}
        {t.fmt.colon}
        <code className="font-mono break-all">{fingerprint}</code>
      </p>
    </div>
  )
}

// --- connecting ------------------------------------------------------------------------

const looksSsh = (url: string) => /^(ssh:\/\/|[\w.-]+@[\w.-]+:)/.test(url.trim())
const looksHttps = (url: string) => /^https:\/\//i.test(url.trim())
const looksGitHub = (url: string) =>
  /^(https:\/\/github\.com\/|git@github\.com:|ssh:\/\/git@github\.com\/)[^/\s]+\/[^/\s]+/i.test(url.trim())

// What the server said to a connect that did not go through, in plain words.
function Problem({ error }: { error: unknown }) {
  if (!error) return null
  const e = error instanceof ApiError ? error : null
  const gh = e && Object.hasOwn(tg.sync.problems, e.code) ? (tg.sync.problems as Record<string, string>)[e.code] : null
  if (e && gh)
    return (
      <div role="alert" className="grid gap-1 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm">
        <p className="font-medium">{gh}</p>
        {(e.serverMessage ?? e.message) && (
          <p className="font-mono text-xs break-words text-muted-foreground">{e.serverMessage ?? e.message}</p>
        )}
      </div>
    )
  if (e?.code === 'partial_unsupported')
    return (
      <div role="alert" className="grid gap-1 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm">
        <p className="font-medium">{t.remote.partialTitle}</p>
        <p className="break-words">{e.message}</p>
        <p className="text-muted-foreground">{t.remote.partialAdvice}</p>
      </div>
    )
  if (e?.status === 409)
    return (
      <div role="alert" className="grid gap-1 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm">
        <p className="font-medium">{t.remote.conflictTitle}</p>
        <p>{t.remote.conflictBody}</p>
        <p className="text-muted-foreground">{t.remote.conflictAdvice}</p>
        {(e.serverMessage ?? e.message) && (
          <p className="font-mono text-xs break-words text-muted-foreground">{e.serverMessage ?? e.message}</p>
        )}
      </div>
    )
  if (e?.code === 'unavailable')
    return (
      <div role="alert" className="grid gap-1 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm">
        <p className="font-medium">{t.remote.unreachableTitle}</p>
        <p className="break-words">{e.serverMessage ?? e.message}</p>
        <p className="text-muted-foreground">{t.remote.unreachableAdvice}</p>
      </div>
    )
  if (e?.code === 'forbidden')
    return (
      <div role="alert" className="grid gap-1 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm">
        <p className="font-medium">{t.remote.forbiddenTitle}</p>
        <p className="break-words">{e.message}</p>
      </div>
    )
  return <FormMessage>{t.fmt.labelled(t.remote.failed, error instanceof Error ? error.message : '')}</FormMessage>
}

function ConnectForm({ space, existing, onDone }: { space: string; existing: Remote | null; onDone?: () => void }) {
  const connect = useConnectRemote(space)
  const [url, setUrl] = useState(existing?.url ?? '')
  const [branch, setBranch] = useState(existing?.branch ?? '')
  const [kind, setKind] = useState<'ssh' | 'token' | 'github'>(
    existing?.credential.type === 'connection' ? 'github' : (existing?.credential.type ?? 'ssh'),
  )
  const github = useProvider('github')
  const githubOffered = !!github.data?.app || existing?.credential.type === 'connection'
  const [via, setVia] = useState<ConnectionVia>(
    existing?.credential.type === 'connection' ? existing.credential.via : 'app',
  )
  // Until the person picks one, the address decides.
  const [chose, setChose] = useState(existing !== null)
  const [username, setUsername] = useState(existing?.credential.type === 'token' ? existing.credential.username : '')
  const [token, setToken] = useState('')
  const [key, setKey] = useState<RemoteKey | null>(
    existing?.credential.type === 'ssh'
      ? { publicKey: existing.credential.publicKey, fingerprint: existing.credential.fingerprint }
      : null,
  )
  const [keyBusy, setKeyBusy] = useState(false)
  const [keyError, setKeyError] = useState('')
  const [rotateOpen, setRotateOpen] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [fieldError, setFieldError] = useState<{ url?: string; token?: string; key?: string }>({})

  const hadToken = existing?.credential.type === 'token'
  const pickByAddress = (next: string) => {
    setUrl(next)
    if (!chose) {
      if (looksSsh(next)) setKind('ssh')
      else if (looksHttps(next)) setKind('token')
    }
  }

  const makeKey = async (rotate: boolean) => {
    setKeyBusy(true)
    setKeyError('')
    try {
      setKey(await spaceApi(space).remoteKey(rotate))
      setFieldError((f) => ({ ...f, key: undefined }))
    } catch (e) {
      setKeyError(t.fmt.labelled(t.remote.keyFailed, e instanceof Error ? e.message : ''))
    }
    setKeyBusy(false)
    setRotateOpen(false)
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (connect.isPending) return
    setError(null)
    const errs: typeof fieldError = {}
    // owner/name is enough for GitHub.
    const address =
      kind === 'github' && /^[\w.-]+\/[\w.-]+$/.test(url.trim())
        ? `https://github.com/${url.trim().replace(/\.git$/, '')}.git`
        : url.trim()
    if (address === '') errs.url = t.remote.urlRequired
    else if (kind === 'ssh' && !looksSsh(address)) errs.url = t.remote.mismatchSsh
    else if (kind === 'token' && !looksHttps(address)) errs.url = t.remote.mismatchToken
    else if (kind === 'github' && !looksGitHub(address)) errs.url = tg.sync.mismatchGitHub
    if (kind === 'ssh' && !key) errs.key = t.remote.keyNeeded
    if (kind === 'token' && token === '' && !hadToken) errs.token = t.remote.tokenRequired
    setFieldError(errs)
    if (Object.keys(errs).length > 0) return
    const input: ConnectRemoteInput = {
      url: address,
      ...(branch.trim() ? { branch: branch.trim() } : {}),
      credential:
        kind === 'ssh'
          ? { type: 'ssh' }
          : kind === 'github'
            ? { type: 'connection', provider: 'github', via }
            : { type: 'token', ...(username.trim() ? { username: username.trim() } : {}), ...(token ? { token } : {}) },
    }
    try {
      await connect.mutateAsync(input)
      setToken('')
      toast.success(existing ? t.remote.updated : t.remote.connected)
      onDone?.()
    } catch (err) {
      setError(err)
    } finally {
      // The mutation holds the request, token included: let go of it at once.
      connect.reset()
    }
  }

  return (
    <form className="grid gap-4" onSubmit={(e) => void submit(e)} noValidate>
      <Field
        label={t.remote.url}
        help={t.remote.urlHelp}
        value={url}
        onChange={(e) => pickByAddress(e.target.value)}
        placeholder={t.remote.urlPlaceholder}
        error={fieldError.url}
        autoComplete="off"
        spellCheck={false}
        inputMode="url"
      />
      <Field
        label={t.remote.branch}
        help={t.remote.branchHelp}
        value={branch}
        onChange={(e) => setBranch(e.target.value)}
        placeholder="main"
        autoComplete="off"
        spellCheck={false}
      />

      <fieldset className="grid gap-2">
        <legend className="mb-1 text-sm font-medium">{t.remote.credential}</legend>
        <div role="radiogroup" aria-label={t.remote.credential} className="flex flex-wrap gap-2">
          {(githubOffered ? (['ssh', 'token', 'github'] as const) : (['ssh', 'token'] as const)).map((k) => (
            <label
              key={k}
              className={`flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-1.5 text-sm has-[:focus-visible]:ring-3 has-[:focus-visible]:ring-ring/50 ${kind === k ? 'border-brand bg-brand/5' : ''}`}
            >
              <input
                type="radio"
                name={`credential-${space}`}
                className="accent-[var(--brand)]"
                checked={kind === k}
                onChange={() => {
                  setKind(k)
                  setChose(true)
                }}
              />
              {k === 'ssh' ? t.remote.ssh : k === 'token' ? t.remote.token : tg.sync.github}
            </label>
          ))}
        </div>
      </fieldset>

      {kind === 'ssh' ? (
        <div className="grid gap-2">
          <p className="text-xs text-muted-foreground">{t.remote.sshHelp}</p>
          {key ? (
            <>
              <PublicKey publicKey={key.publicKey} fingerprint={key.fingerprint} title={t.remote.ssh} />
              {/* A new key replaces the old one, which only a space not yet connected allows. */}
              {!existing && (
                <div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setRotateOpen(true)}
                    disabled={keyBusy}
                  >
                    <KeyRound aria-hidden="true" />
                    {t.remote.rotateKey}
                  </Button>
                </div>
              )}
            </>
          ) : (
            <div>
              <Button type="button" variant="outline" onClick={() => void makeKey(false)} disabled={keyBusy}>
                {keyBusy ? <Spinner /> : <KeyRound />}
                {keyBusy ? t.remote.makingKey : t.remote.makeKey}
              </Button>
            </div>
          )}
          {fieldError.key && <p className="text-xs text-destructive">{fieldError.key}</p>}
          {keyError && (
            <p role="alert" className="text-xs text-destructive">
              {keyError}
            </p>
          )}
        </div>
      ) : kind === 'github' ? (
        <div className="grid grid-cols-1 gap-3">
          <p className="text-xs text-muted-foreground">{tg.sync.help}</p>
          <div role="radiogroup" aria-label={tg.sync.viaLabel} className="grid grid-cols-1 gap-2">
            {(['app', 'user'] as const).map((v) => (
              <label
                key={v}
                className={`flex cursor-pointer items-start gap-2 rounded-lg border px-3 py-2 text-sm has-[:focus-visible]:ring-3 has-[:focus-visible]:ring-ring/50 ${via === v ? 'border-brand bg-brand/5' : ''}`}
              >
                <input
                  type="radio"
                  name={`via-${space}`}
                  className="mt-1 accent-[var(--brand)]"
                  checked={via === v}
                  onChange={() => setVia(v)}
                />
                <span className="min-w-0">
                  <span className="block font-medium">{v === 'app' ? tg.sync.viaApp : tg.sync.viaUser}</span>
                  <span className="block text-xs text-muted-foreground">
                    {v === 'app' ? tg.sync.viaAppHelp : tg.sync.viaUserHelp}
                  </span>
                </span>
              </label>
            ))}
          </div>
          {via === 'user' && github.data?.connection?.state !== 'ok' && (
            <p className="text-xs text-warning">{tg.sync.needConnection}</p>
          )}
          <RepoPicker onPick={(fullName) => setUrl(fullName)} />
        </div>
      ) : (
        <div className="grid gap-4">
          <Field
            label={t.remote.username}
            help={t.remote.usernameHelp}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="x-access-token"
            autoComplete="off"
            spellCheck={false}
          />
          <Field
            label={t.remote.tokenField}
            help={hadToken ? t.remote.tokenKeep : t.remote.tokenHelp}
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            // A saved token is never shown, so the box starts empty: leaving it so keeps it.
            placeholder={hadToken ? '••••••••' : ''}
            error={fieldError.token}
            autoComplete="new-password"
          />
        </div>
      )}

      <Problem error={error} />
      <div className="flex items-center gap-3">
        <Button type="submit" disabled={connect.isPending}>
          {connect.isPending ? <Spinner /> : <Save aria-hidden="true" />}
          {connect.isPending ? t.remote.connecting : existing ? t.remote.saveChanges : t.remote.connect}
        </Button>
      </div>

      <AlertDialog open={rotateOpen} onOpenChange={setRotateOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t.remote.rotateTitle}</AlertDialogTitle>
            <AlertDialogDescription>{t.remote.rotateDescription}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t.remote.disconnectCancel}</AlertDialogCancel>
            <AlertDialogAction disabled={keyBusy} onClick={() => void makeKey(true)}>
              {t.remote.rotateConfirm}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </form>
  )
}

// --- choosing a repository from what GitHub lists -----------------------------------------

// An account to look in, then its repositories a page (one call to GitHub) at a
// time, filtered by what has been loaded: GitHub cannot filter by name. Choosing
// one fills in the address; typing owner/name there works as well.
function RepoPicker({ onPick }: { onPick: (fullName: string) => void }) {
  const [open, setOpen] = useState(false)
  const owners = useConnectionOwners('github', open)
  const list = owners.data?.pages.flatMap((p) => p.items) ?? []
  const [login, setLogin] = useState('')
  const chosen = list.find((o) => o.login === login) ?? list[0]
  const which = chosen
    ? chosen.installationId !== null
      ? { installation: chosen.installationId }
      : { mine: true as const }
    : null
  const repos = useConnectionRepos('github', open ? which : null)
  const [filter, setFilter] = useState('')
  const items = (repos.data?.pages.flatMap((p) => p.items) ?? []).filter((r) =>
    r.fullName.toLowerCase().includes(filter.trim().toLowerCase()),
  )
  return (
    <div className="grid grid-cols-1 gap-2">
      <div>
        <Button type="button" variant="ghost" size="sm" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
          {open ? tg.sync.hidePicker : tg.sync.showPicker}
        </Button>
      </div>
      {open && (
        <div className="grid min-w-0 grid-cols-1 gap-2 rounded-lg border p-3">
          {owners.isPending ? (
            <PageFallback />
          ) : list.length === 0 ? (
            <p className="text-sm text-muted-foreground">{tg.sync.noOwners}</p>
          ) : (
            <>
              <NativeSelect
                aria-label={tg.sync.pickOwner}
                value={chosen?.login ?? ''}
                onChange={(e) => setLogin(e.target.value)}
              >
                {list.map((o) => (
                  <option key={o.login} value={o.login}>
                    {o.login}
                  </option>
                ))}
              </NativeSelect>
              {owners.hasNextPage && (
                <div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={owners.isFetchingNextPage}
                    onClick={() => void owners.fetchNextPage()}
                  >
                    {tg.newSpace.moreOwners}
                  </Button>
                </div>
              )}
              <Input
                aria-label={tg.sync.filter}
                placeholder={tg.sync.filter}
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                className="w-full min-w-0"
                autoComplete="off"
              />
              {repos.isPending ? (
                <PageFallback />
              ) : repos.isError ? (
                <p role="alert" className="text-sm text-destructive break-words">
                  {t.fmt.labelled(tg.sync.reposFailed, repos.error.message)}
                </p>
              ) : items.length === 0 ? (
                <p className="text-sm text-muted-foreground">{tg.sync.noRepos}</p>
              ) : (
                <ul className="max-h-56 divide-y overflow-y-auto rounded-md border">
                  {items.map((r) => (
                    <li key={r.id}>
                      <button
                        type="button"
                        className="flex w-full min-w-0 flex-wrap items-center gap-x-2 px-3 py-1.5 text-left text-sm break-all hover:bg-muted/60 focus-visible:bg-muted/60 focus-visible:outline-none"
                        onClick={() => onPick(r.fullName)}
                      >
                        {r.fullName}
                        <span className="text-xs text-muted-foreground">
                          {r.private ? tg.sync.private : tg.sync.public}
                        </span>
                        {!r.canPush && <span className="text-xs text-warning">{tg.sync.readOnly}</span>}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              {repos.hasNextPage && (
                <div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={repos.isFetchingNextPage}
                    onClick={() => void repos.fetchNextPage()}
                  >
                    {repos.isFetchingNextPage && <Spinner />}
                    {tg.sync.moreRepos}
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
