import { useEffect, useRef, useState, type FormEvent } from 'react'
import { ExternalLink } from 'lucide-react'
import { ApiError, api } from '@/api/client'
import { useQuery } from '@tanstack/react-query'
import { useConnectionOwners, useConnections, useCreateSpace, useRecheckOwners } from '@/api/hooks'
import type { ConnectionOwner, NewSync, SpaceInfo } from '@/api/types'
import { Field } from '@/components/auth/Field'
import { Spinner } from '@/components/common'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { t } from '@/i18n'
import { BrandIcon } from '@/components/brand/BrandIcon'
import { tg } from '@/i18n/github'
import { ts } from '@/i18n/storage'

// A name for a repository from the name of a space: letters, digits and the few
// marks GitHub keeps; everything else becomes a hyphen. Only a suggestion.
export function repoNameFrom(name: string): string {
  const s = name
    .trim()
    .normalize('NFKD')
    .replace(/[^A-Za-z0-9._-]+/g, '-')
    .replace(/^[-.]+|[-.]+$/g, '')
    .slice(0, 100)
  return s.toLowerCase().endsWith('.git') ? s.slice(0, -4) : s
}

const repoNameOk = (n: string) =>
  /^[A-Za-z0-9._-]{1,100}$/.test(n) && n !== '.' && n !== '..' && !n.toLowerCase().endsWith('.git')

// What follows a new space: "by address" has no place made yet, so the sync settings open for the address to be given.
export interface NewSpaceThen {
  openSync: boolean
}

// The "where" of the option "other (by address)" and of the organisation's own storage target.
const OTHER = '*address'
const TENANT = '*tenant'

interface Left {
  fullName: string
  htmlUrl: string
}

// Makes a space: its name and where it is stored: on this server, or synced with a platform the server connects
// to (the owner, name, privacy and description of the place made there for it).
// Mounted only while open, so the form starts empty each time.
export function NewSpaceDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (space: SpaceInfo, then?: NewSpaceThen) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open && <NewSpaceForm onClose={() => onOpenChange(false)} onCreated={onCreated} />}
    </Dialog>
  )
}

function NewSpaceForm({
  onClose,
  onCreated,
}: {
  onClose: () => void
  onCreated: (space: SpaceInfo, then?: NewSpaceThen) => void
}) {
  const create = useCreateSpace()
  const connections = useConnections()
  // The platforms a space can be synced to: set up, and able to make a place for it.
  const providers = (connections.data?.providers ?? []).filter((p) => p.configured && p.capabilities.createRepo)
  const [name, setName] = useState('')
  // Where the space is stored, once the person picks: '' is this server, else the id of a provider, OTHER for a plain
  // address given afterwards, or TENANT for the organisation's storage target. Until then the default decides.
  const [picked, setWhere] = useState<string | null>(null)
  // What the settings say a new space's attachments do for this person: the default, what they may ask for, and
  // whether a lock (or a rule allowing only one answer) leaves no choice. null until it is read; the box is then hidden.
  const rule = useQuery({
    queryKey: ['settings', 'new-space-assets'],
    staleTime: 30_000,
    queryFn: async ({ signal }) => {
      const all = (await api.settings(undefined, signal)).settings
      const found = all.find((x) => x.key === 'storage.assets')
      // Where the settings let a new space be stored (storage.mode): the modes allowed, the default, and the ones that
      // cannot be used now. Absent: the server says nothing, and the choice is as before.
      const mode = all.find((x) => x.key === 'storage.mode')
      const modes = mode
        ? {
            default: String(mode.effective),
            allowed: mode.allowed ?? [],
            missing: mode.missing ?? [],
          }
        : undefined
      return found
        ? {
            on: found.effective === 'repository',
            canRepo: !!found.allowed?.includes('repository'),
            // No choice either when the site allows only a repository of their own without locking it.
            locked: !!found.locked || !found.allowed?.includes('space'),
            modes,
          }
        : { on: false, canRepo: true, locked: false, modes }
    },
  }).data
  // What the modes allow: the server's disk, a remote of their own (a platform, or an address), the organisation's target.
  const modes = rule?.modes
  const canServer = !modes || modes.allowed.includes('server')
  const canOwn = !modes || modes.allowed.includes('own')
  const canTenant = !!modes?.allowed.includes('tenant')
  const tenantMissing = !!modes?.missing.includes('tenant')
  const shownProviders = canOwn ? providers : []
  // The default's place, then the first place left when it is not one.
  const places = [
    ...(canServer ? [''] : []),
    ...shownProviders.map((p) => p.id),
    // A plain address is given after the space exists, so it is no way in where the server is not allowed.
    ...(canOwn && canServer && providers.length > 0 ? [OTHER] : []),
    ...(canTenant && !tenantMissing ? [TENANT] : []),
  ]
  const defaultWhere = (() => {
    const want = modes?.default === 'own' ? (shownProviders[0]?.id ?? OTHER) : modes?.default === 'tenant' ? TENANT : ''
    return places.includes(want) ? want : (places[0] ?? '')
  })()
  const where = picked !== null && places.includes(picked) ? picked : defaultWhere
  const provider = shownProviders.find((p) => p.id === where) ?? null
  const useRepo = provider !== null
  // The owner's own choice, once they touch the box; until then nothing is sent and the default decides.
  const [assetsChoice, setAssetsChoice] = useState<boolean | null>(null)
  const withAssets = assetsChoice ?? rule?.on ?? false
  const [owner, setOwner] = useState('')
  const [repo, setRepo] = useState('')
  const [repoTouched, setRepoTouched] = useState(false)
  const [priv, setPriv] = useState(true)
  const [description, setDescription] = useState('')
  const [error, setError] = useState('')
  const [fieldError, setFieldError] = useState<{ name?: string; repo?: string; description?: string }>({})
  // A repository the server made and could not use yet, or one that was there already: the retry connects to it.
  const [left, setLeft] = useState<Left | null>(null)
  const [taken, setTaken] = useState(false)
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => input.current?.select(), [])

  const owners = useConnectionOwners(where, useRepo)
  const recheck = useRecheckOwners(where)
  const list: ConnectionOwner[] = owners.data?.pages.flatMap((p) => p.items) ?? []
  // The first account until the person picks one.
  const chosen = list.find((o) => o.login === owner) ?? list[0]
  const connected = provider?.connection?.state === 'ok'

  const submit = async (e: FormEvent, existing?: { owner: string; name: string }) => {
    e.preventDefault()
    if (busy) return
    const trimmed = name.trim()
    const problems: typeof fieldError = {}
    if (trimmed === '' || Array.from(trimmed).length > 80) problems.name = t.spaces.nameInvalid
    let gh: NewSync | undefined
    if (useRepo) {
      const repoName = existing?.name ?? repo.trim()
      const ownerLogin = existing?.owner ?? chosen?.login ?? ''
      if (!repoNameOk(repoName)) problems.repo = tg.newSpace.repoInvalid
      if (
        Array.from(description).length > 350 ||
        Array.from(description).some((c) => c.charCodeAt(0) < 0x20 || c.charCodeAt(0) === 0x7f)
      )
        problems.description = tg.newSpace.descriptionInvalid
      if (ownerLogin === '') problems.repo ??= tg.newSpace.ownerNone
      gh = {
        provider: where,
        owner: ownerLogin,
        name: repoName,
        private: priv,
        ...(description.trim() ? { description: description.trim() } : {}),
        create: existing === undefined,
        ...(assetsChoice !== null && assetsChoice !== rule?.on ? { assets: assetsChoice } : {}),
      }
    }
    setFieldError(problems)
    if (Object.keys(problems).length > 0) return
    setBusy(true)
    setError('')
    setLeft(null)
    setTaken(false)
    try {
      const mode = where === '' ? ('server' as const) : where === TENANT ? ('tenant' as const) : ('own' as const)
      // A Chinese-language app names the inbox folder in Chinese.
      const space = await create.mutateAsync({
        name: trimmed,
        inbox: t.inbox.folderName,
        ...(gh ? { sync: gh } : {}),
        // Said only where the server could not tell from the request: a space with a repository is "own", and the
        // default is what a request that does not say gets.
        ...(modes && !gh && mode !== modes.default ? { mode } : {}),
      })
      onCreated(space, where === OTHER ? { openSync: true } : undefined)
      onClose()
    } catch (err) {
      setBusy(false)
      explain(err)
    }
  }

  const explain = (err: unknown) => {
    if (!(err instanceof ApiError)) return setError(t.spaces.failed)
    const body = err.body as { repo?: { fullName?: string; htmlUrl?: string } } | undefined
    const code = err.code as string
    switch (code) {
      case 'name_taken':
        setTaken(true)
        return setFieldError({ repo: tg.newSpace.errors.nameTaken })
      case 'push_failed':
      case 'repo_not_in_installation':
      case 'internal':
        if (body?.repo?.fullName) setLeft({ fullName: body.repo.fullName, htmlUrl: body.repo.htmlUrl ?? '' })
        return setError(
          code === 'repo_not_in_installation'
            ? tg.newSpace.errors.notInInstallation
            : code === 'push_failed'
              ? tg.newSpace.errors.pushFailed
              : t.spaces.failed,
        )
      case 'provider_refused':
        return setError(tg.newSpace.errors.noPermission(err.serverMessage ?? err.message))
      case 'app_not_installed':
        return setError(tg.newSpace.errors.notInstalled(chosen?.login ?? ''))
      case 'app_missing_permission':
        if (body?.repo?.fullName) setLeft({ fullName: body.repo.fullName, htmlUrl: body.repo.htmlUrl ?? '' })
        return setError(
          tg.newSpace.errors.missingPermission((err.body as { missing?: string[] } | undefined)?.missing ?? []),
        )
      case 'owner_not_allowed':
        return setError(tg.newSpace.errors.ownerNotAllowed)
      case 'not_connected':
        return setError(tg.newSpace.errors.notAuthorized)
      case 'provider_not_configured':
        return setError(tg.newSpace.errors.notConfigured)
      case 'rate_limited':
        return setError(tg.newSpace.errors.rateLimited)
      case 'unavailable':
        return setError(tg.newSpace.errors.unavailable)
      default:
        return setError(err.message || t.spaces.failed)
    }
  }

  const connect = async () => {
    try {
      // Installing the App on one's own account connects it in the same step.
      const { url } = await api.connectionStart(where, window.location.pathname, true)
      window.location.assign(url)
    } catch (err) {
      explain(err)
    }
  }

  const [leftOwner, leftName] = left ? left.fullName.split('/') : ['', '']

  return (
    <DialogContent className="max-h-[calc(100dvh-2rem)] grid-cols-[minmax(0,1fr)] overflow-y-auto">
      <form className="grid min-w-0 grid-cols-1 gap-4" onSubmit={(e) => void submit(e)} noValidate>
        <DialogHeader>
          <DialogTitle>{t.spaces.newTitle}</DialogTitle>
          <DialogDescription>{t.spaces.nameHelp}</DialogDescription>
        </DialogHeader>
        <Field
          label={t.spaces.nameLabel}
          inputRef={input}
          value={name}
          onChange={(e) => {
            setName(e.target.value)
            if (!repoTouched) setRepo(repoNameFrom(e.target.value))
          }}
          error={fieldError.name}
          autoFocus
          autoComplete="off"
        />

        {(places.length > 1 || (!canServer && canOwn && places.length > 0)) && (
          <fieldset className="grid min-w-0 grid-cols-1 gap-3 rounded-lg border p-3">
            <legend className="px-1 text-xs text-muted-foreground">{tg.newSpace.legend}</legend>
            {canServer && (
              <label className="flex cursor-pointer items-start gap-2 text-sm">
                <input
                  type="radio"
                  name="new-space-where"
                  className="mt-1 accent-[var(--brand)]"
                  checked={where === ''}
                  onChange={() => setWhere('')}
                />
                <span className="min-w-0">
                  <span className="block font-medium">{tg.newSpace.whereServer}</span>
                  <span className="block text-xs text-muted-foreground">{tg.newSpace.whereServerHelp}</span>
                </span>
              </label>
            )}
            {shownProviders.map((p) => (
              <label key={p.id} className="flex cursor-pointer items-start gap-2 text-sm">
                <input
                  type="radio"
                  name="new-space-where"
                  className="mt-1 accent-[var(--brand)]"
                  checked={where === p.id}
                  onChange={() => setWhere(p.id)}
                />
                <span className="min-w-0">
                  <span className="flex items-center gap-1.5 font-medium">
                    <BrandIcon type={p.type} className="size-4" />
                    {tg.newSpace.whereSync(p.name)}
                  </span>
                  <span className="block text-xs text-muted-foreground">{tg.newSpace.optionHelp}</span>
                </span>
              </label>
            ))}
            {places.includes(OTHER) && (
              <label className="flex cursor-pointer items-start gap-2 text-sm">
                <input
                  type="radio"
                  name="new-space-where"
                  className="mt-1 accent-[var(--brand)]"
                  checked={where === OTHER}
                  onChange={() => setWhere(OTHER)}
                />
                <span className="min-w-0">
                  <span className="block font-medium">{tg.newSpace.whereOther}</span>
                  <span className="block text-xs text-muted-foreground">{tg.newSpace.whereOtherHelp}</span>
                </span>
              </label>
            )}
            {!canServer && canOwn && (
              <p className="text-xs break-words text-muted-foreground">
                {shownProviders.length > 0 ? tg.newSpace.addressNeedsServer : tg.newSpace.noPlatform}
              </p>
            )}
            {places.includes(TENANT) && (
              <label className="flex cursor-pointer items-start gap-2 text-sm">
                <input
                  type="radio"
                  name="new-space-where"
                  className="mt-1 accent-[var(--brand)]"
                  checked={where === TENANT}
                  onChange={() => setWhere(TENANT)}
                />
                <span className="min-w-0">
                  <span className="block font-medium">{tg.newSpace.whereTenant}</span>
                  <span className="block text-xs text-muted-foreground">{tg.newSpace.whereTenantHelp}</span>
                </span>
              </label>
            )}

            {useRepo && (
              <div className="grid min-w-0 grid-cols-1 gap-3">
                {rule?.canRepo && (
                  <label
                    className={`flex min-h-10 items-start gap-2 text-sm ${rule.locked ? 'cursor-not-allowed opacity-80' : 'cursor-pointer'}`}
                  >
                    <input
                      type="checkbox"
                      className="mt-1 size-4 accent-[var(--brand)]"
                      checked={withAssets}
                      disabled={rule.locked}
                      onChange={(e) => setAssetsChoice(e.target.checked)}
                    />
                    <span className="min-w-0">
                      <span className="block font-medium">{ts.assets.newSpace}</span>
                      <span className="block text-xs text-muted-foreground">
                        {rule.locked ? ts.assets.locked : ts.assets.newSpaceHelp}
                      </span>
                    </span>
                  </label>
                )}
                {owners.isPending ? (
                  <p className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Spinner />
                    {tg.newSpace.loadingOwners}
                  </p>
                ) : owners.isError ? (
                  <p role="alert" className="text-sm text-destructive break-words">
                    {t.fmt.labelled(tg.newSpace.ownersFailed, owners.error.message)}
                  </p>
                ) : list.length === 0 ? (
                  <div className="grid grid-cols-1 gap-2 text-sm">
                    <p className="text-muted-foreground">{tg.newSpace.ownerNone}</p>
                    {provider?.userAuthorization && !connected && (
                      <div>
                        <Button type="button" variant="outline" onClick={() => void connect()}>
                          {tg.newSpace.connect}
                        </Button>
                      </div>
                    )}
                  </div>
                ) : (
                  <div className="grid min-w-0 grid-cols-1 gap-1.5">
                    <Label htmlFor="new-space-owner">{tg.newSpace.owner}</Label>
                    <NativeSelect
                      id="new-space-owner"
                      value={chosen?.login ?? ''}
                      onChange={(e) => setOwner(e.target.value)}
                    >
                      {list.map((o) => (
                        <option key={o.login} value={o.login}>
                          {o.login}
                          {o.type === 'Organization' ? ` · ${tg.newSpace.organisation}` : ''}
                          {o.needsInstall ? ` · ${tg.newSpace.needsInstall}` : ''}
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
                          {owners.isFetchingNextPage ? <Spinner /> : null}
                          {tg.newSpace.moreOwners}
                        </Button>
                      </div>
                    )}
                    <p className="text-xs text-muted-foreground">
                      {chosen?.via === 'app' ? tg.newSpace.viaApp : tg.newSpace.viaUser}
                    </p>
                    {chosen?.needsInstall && (
                      <div className="grid grid-cols-1 gap-2 rounded-lg border border-warning/50 bg-warning/5 p-3 text-sm">
                        <p className="break-words">{tg.newSpace.needsInstallHelp(chosen.login)}</p>
                        <div>
                          <Button type="button" variant="outline" size="sm" onClick={() => void connect()}>
                            {tg.newSpace.installOnMine}
                          </Button>{' '}
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            disabled={recheck.isPending}
                            onClick={() => recheck.mutate()}
                          >
                            {recheck.isPending ? <Spinner /> : null}
                            {recheck.isPending ? tg.newSpace.checkingAgain : tg.newSpace.checkAgain}
                          </Button>
                          {recheck.isError && (
                            <span role="alert" className="ml-2 text-xs text-destructive">
                              {tg.newSpace.checkFailed}
                            </span>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                )}

                <Field
                  label={tg.newSpace.repoName}
                  help={tg.newSpace.repoNameHelp}
                  value={repo}
                  onChange={(e) => {
                    setRepo(e.target.value)
                    setRepoTouched(true)
                    setTaken(false)
                  }}
                  error={fieldError.repo}
                  autoComplete="off"
                  spellCheck={false}
                  className="w-full min-w-0"
                />
                <label className="flex cursor-pointer items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-1 accent-[var(--brand)]"
                    checked={priv}
                    onChange={(e) => setPriv(e.target.checked)}
                  />
                  <span className="min-w-0">
                    <span className="block font-medium">{tg.newSpace.private}</span>
                    <span className="block text-xs text-muted-foreground">{tg.newSpace.privateHelp}</span>
                  </span>
                </label>
                <div className="grid min-w-0 grid-cols-1 gap-1.5">
                  <Label htmlFor="new-space-description">{tg.newSpace.description}</Label>
                  <Textarea
                    id="new-space-description"
                    rows={2}
                    maxLength={350}
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    className="w-full min-w-0"
                    aria-invalid={fieldError.description ? true : undefined}
                  />
                  {fieldError.description && <p className="text-xs text-destructive">{fieldError.description}</p>}
                </div>
                {taken && chosen && (
                  <div className="grid grid-cols-1 gap-2 rounded-lg border bg-muted/30 p-3 text-sm">
                    <p className="break-words">{tg.newSpace.takenAsk(`${chosen.login}/${repo.trim()}`)}</p>
                    <div>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={busy}
                        onClick={(e) => void submit(e, { owner: chosen.login, name: repo.trim() })}
                      >
                        {tg.newSpace.useExisting}
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            )}
          </fieldset>
        )}

        {places.length === 0 && (
          <p role="alert" className="rounded-md border border-warning/50 bg-warning/5 px-3 py-2 text-sm break-words">
            {tenantMissing ? tg.newSpace.whereTenantMissing : tg.newSpace.noPlace}
          </p>
        )}
        {left && (
          <div
            role="alert"
            className="grid grid-cols-1 gap-2 rounded-lg border border-warning/50 bg-warning/5 p-3 text-sm"
          >
            <p className="break-words">{tg.newSpace.leftBehind(left.fullName)}</p>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                size="sm"
                disabled={busy}
                onClick={(e) => void submit(e, { owner: leftOwner, name: leftName })}
              >
                {busy && <Spinner />}
                {tg.newSpace.useLeft}
              </Button>
              {left.htmlUrl && /^https?:\/\//.test(left.htmlUrl) && (
                <a
                  className="inline-flex items-center gap-1 text-xs break-all underline underline-offset-4"
                  href={left.htmlUrl}
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  {tg.newSpace.openOnGitHub}
                  <ExternalLink className="size-3 shrink-0" />
                </a>
              )}
            </div>
          </div>
        )}
        {error && (
          <p
            role="alert"
            className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm break-words text-destructive"
          >
            {error}
          </p>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t.spaces.cancel}
          </Button>
          <Button type="submit" disabled={busy || places.length === 0}>
            {busy && <Spinner />}
            {busy && useRepo ? tg.newSpace.creating : t.spaces.create}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
