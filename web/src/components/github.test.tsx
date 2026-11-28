// @vitest-environment jsdom
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { Route, Routes } from 'react-router'
import { useState } from 'react'
import type { SpaceInfo } from '@/api/types'
import type { Remote } from '@/api/types'
import { click, installDomShims, locationText, mount, settle, type, type FxMounted } from '@/testing/harness'
import { githubApp, githubProvider, leaveRepository, type GitHubState } from '@/api/fixtures/data/connections'
import type { FixtureServer } from '@/api/fixtures/server'
import { t } from '@/i18n'
import { tc } from '@/i18n/console'
import { tg } from '@/i18n/github'
import { ts } from '@/i18n/storage'
import SettingsRoute from '@/components/settings/SettingsRoute'
import { AdminGate } from '@/components/admin/AdminGate'
import { SidebarProvider } from '@/components/ui/sidebar'
import { NewSpaceDialog } from '@/components/layout/NewSpaceDialog'
import { BrandIcon } from '@/components/brand/BrandIcon'

// What the app shows of GitHub, against the fixture server: the new-space dialog, the account's
// connection, a space's remote settings and the administrators' App.

beforeAll(installDomShims)
beforeEach(() => localStorage.clear())

let m: FxMounted | undefined
afterEach(async () => {
  await m?.unmount()
  m = undefined
})

const text = () => document.body.textContent ?? ''
const dialog = () => document.querySelector<HTMLElement>('[role="dialog"]')
const button = (label: string, root: ParentNode = document) =>
  [...root.querySelectorAll<HTMLButtonElement>('button')].find((b) => b.textContent?.trim() === label)
const posts = () => m!.requests.filter((r) => r.method === 'POST' && r.url === '/api/spaces')
// What the site's settings say of a new space's attachments (the one setting the dialog reads).
const assetsRule = (fx: FixtureServer, rule: { effective: string; allowed: string[]; locked?: boolean }) =>
  fx.reply('GET', '/api/settings', {
    features: fx.data.features,
    settings: [{ key: 'storage.assets', locked: false, ...rule }],
  })
// The answer to making a space on a platform: the space, stored there.
const madeOnPlatform = (fx: FixtureServer, name: string) =>
  fx.reply('POST', '/api/spaces', { ...fx.data.spaces[0].info, id: '01FZMADE', name, sync: 'ok' }, { status: 201 })
const repoName = () => dialog()!.querySelector<HTMLInputElement>('input[spellcheck="false"]')!

function Harness({ onCreated }: { onCreated: (s: SpaceInfo) => void }) {
  const [open, setOpen] = useState(true)
  return <NewSpaceDialog open={open} onOpenChange={setOpen} onCreated={onCreated} />
}

async function openNewSpace(
  github: 'off' | 'app' | 'connected',
  rule?: Parameters<typeof assetsRule>[1],
  scene: (d: FixtureServer['data']) => void = () => {},
  // What storage.mode says to this person (the places a new space may be stored).
  mode?: Record<string, unknown>,
) {
  const created: SpaceInfo[] = []
  m = await mount(<Harness onCreated={(s) => created.push(s)} />, {
    signIn: true,
    fixtures: {
      mode: 'accounts',
      data: (d) => {
        githubProvider(d, github)
        scene(d)
      },
    },
    entries: (fx) => {
      if (rule) assetsRule(fx, rule)
      if (mode)
        fx.reply('GET', '/api/settings', {
          features: fx.data.features,
          settings: [{ key: 'storage.mode', locked: false, ...mode }],
        })
      return ['/']
    },
  })
  await settle(() => dialog() !== null, 'the dialog')
  return created
}
const SEPARATE = { effective: 'repository', allowed: ['space', 'repository'] }
const nameField = () => dialog()!.querySelector<HTMLInputElement>('input[autocomplete="off"]')!
const checkbox = (label: string) =>
  [...dialog()!.querySelectorAll('label')]
    .find((l) => l.textContent?.includes(label))!
    .querySelector<HTMLInputElement>('input[type="checkbox"], input[type="radio"]')!

describe('the new-space dialog', () => {
  it('offers no repository while the server has no GitHub App', async () => {
    await openNewSpace('off')
    expect(text()).not.toContain(tg.newSpace.whereSync('GitHub'))
    await type(nameField(), 'Plain')
    await click(button(t.spaces.create))
    await settle(() => posts().length === 1, 'the request')
    expect(JSON.parse(posts()[0].body!)).toEqual({ name: 'Plain', inbox: t.inbox.folderName })
  })

  it('asks a person who has not connected GitHub to do so first', async () => {
    await openNewSpace('app')
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => text().includes(tg.newSpace.ownerNone), 'the hint')
    expect(button(tg.newSpace.connect)).toBeTruthy()
  })

  it('makes a space with a repository: the owner, a name taken from the space, private by default', async () => {
    const created = await openNewSpace('connected')
    madeOnPlatform(m!.fx, 'Tang Notes')
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Tang Notes')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    const owners = [...dialog()!.querySelectorAll('select option')].map((o) => o.getAttribute('value'))
    expect(owners).toEqual(['tang-gh', 'kazuhahub', 'tang-poetry'])
    // The repository name follows the space name until it is edited.
    const repo = () => dialog()!.querySelector<HTMLInputElement>('input[spellcheck="false"]')!
    expect(repo().value).toBe('Tang-Notes')
    await type(dialog()!.querySelector('select')!, 'kazuhahub')
    await type(repo(), 'tang-notes')
    expect(checkbox(tg.newSpace.private).checked).toBe(true)
    await type(dialog()!.querySelector('textarea')!, 'Notes on Tang poetry')
    await click(button(t.spaces.create))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[0].body!)).toEqual({
      name: 'Tang Notes',
      inbox: t.inbox.folderName,
      storage: {
        sync: {
          provider: 'github',
          owner: 'kazuhahub',
          name: 'tang-notes',
          private: true,
          description: 'Notes on Tang poetry',
          create: true,
        },
      },
    })
    expect(created[0].sync).toBe('ok')
    await settle(() => dialog() === null, 'the dialog closing')
  })

  it('starts the attachment box from the default (separate), and sends nothing about it while it is left alone', async () => {
    const created = await openNewSpace('connected', SEPARATE)
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Tang Notes')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await settle(() => checkbox(ts.assets.newSpace).checked, 'the default')
    expect(checkbox(ts.assets.newSpace).disabled).toBe(false)
    await click(button(t.spaces.create))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[0].body!).storage.sync).not.toHaveProperty('assets')
  })

  it('sends what the owner chose when they change the box, and only then', async () => {
    const created = await openNewSpace('connected', SEPARATE)
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Tang Notes')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await settle(() => checkbox(ts.assets.newSpace).checked, 'the default')
    await click(checkbox(ts.assets.newSpace))
    await click(checkbox(ts.assets.newSpace))
    await click(checkbox(ts.assets.newSpace))
    await click(button(t.spaces.create))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[0].body!).storage.sync).toMatchObject({ provider: 'github', create: true, assets: false })
  })

  it('follows a default of the same repository, and asks for a separate one only when ticked', async () => {
    const created = await openNewSpace('connected', { effective: 'space', allowed: ['space', 'repository'] })
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Tang Notes')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await settle(() => checkbox(ts.assets.newSpace) !== undefined, 'the box')
    expect(checkbox(ts.assets.newSpace).checked).toBe(false)
    await click(checkbox(ts.assets.newSpace))
    await click(button(t.spaces.create))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[0].body!).storage.sync).toMatchObject({ assets: true })
  })

  it('shows the box ticked and disabled, with the reason, under a lock to the repository', async () => {
    const created = await openNewSpace('connected', { ...SEPARATE, locked: true })
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Tang Notes')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await settle(() => text().includes(ts.assets.locked), 'the reason')
    expect(checkbox(ts.assets.newSpace).checked).toBe(true)
    expect(checkbox(ts.assets.newSpace).disabled).toBe(true)
    await click(button(t.spaces.create))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[0].body!).storage.sync).not.toHaveProperty('assets')
  })

  it('shows the box ticked and disabled when the site allows only a repository of their own, without a lock', async () => {
    const created = await openNewSpace('connected', { effective: 'repository', allowed: ['repository'] })
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Tang Notes')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await settle(() => text().includes(ts.assets.locked), 'the reason')
    expect(checkbox(ts.assets.newSpace).checked).toBe(true)
    expect(checkbox(ts.assets.newSpace).disabled).toBe(true)
    await click(button(t.spaces.create))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[0].body!).storage.sync).not.toHaveProperty('assets')
  })

  it('hides the box when no second repository is possible (a lock to the same repository)', async () => {
    const created = await openNewSpace('connected', { effective: 'space', allowed: ['space'], locked: true })
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Tang Notes')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await new Promise((r) => setTimeout(r, 50))
    expect(text()).not.toContain(ts.assets.newSpace)
    await click(button(t.spaces.create))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[0].body!).storage.sync).not.toHaveProperty('assets')
  })

  it('checks the repository name before asking', async () => {
    await openNewSpace('connected')
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'X')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await type(repoName(), 'bad name.git')
    await click(button(t.spaces.create))
    await settle(() => text().includes(tg.newSpace.repoInvalid), 'the message')
    expect(posts()).toHaveLength(0)
  })

  it('offers the existing repository when the name is taken, and uses it without creating', async () => {
    const created = await openNewSpace('connected')
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Wiki')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await type(dialog()!.querySelector('select')!, 'kazuhahub')
    await type(repoName(), 'wiki')
    m!.fx.fail('POST', '/api/spaces', 409, { code: 'name_taken', message: 'kazuhahub/wiki exists' }, { times: 1 })
    await click(button(t.spaces.create))
    await settle(() => text().includes(tg.newSpace.errors.nameTaken), 'the refusal')
    expect(created).toHaveLength(0)
    await click(button(tg.newSpace.useExisting))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[1].body!).storage.sync).toMatchObject({ owner: 'kazuhahub', name: 'wiki', create: false })
  })

  it('names the empty repository a failed push left, and connects to it on a second try', async () => {
    const created = await openNewSpace('connected')
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Slow')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await type(dialog()!.querySelector('select')!, 'kazuhahub')
    await type(repoName(), 'slow')
    m!.fx.fail(
      'POST',
      '/api/spaces',
      502,
      {
        code: 'push_failed',
        message: 'kazuhahub/slow exists but the first push failed',
        repo: { id: 880_000_077, fullName: 'kazuhahub/slow', htmlUrl: 'https://github.com/kazuhahub/slow' },
      },
      { times: 1 },
    )
    await click(button(t.spaces.create))
    await settle(() => text().includes(tg.newSpace.leftBehind('kazuhahub/slow')), 'the empty repository')
    expect(text()).toContain(tg.newSpace.errors.pushFailed)
    await click(button(tg.newSpace.useLeft))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[1].body!).storage.sync).toMatchObject({ owner: 'kazuhahub', name: 'slow', create: false })
  })

  it("shows GitHub's refusal in its own words and keeps the form", async () => {
    const created = await openNewSpace('connected')
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    m!.fx.fail('POST', '/api/spaces', 403, { code: 'provider_refused', message: 'GitHub 拒绝建仓库' })
    await type(nameField(), 'No')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await click(button(t.spaces.create))
    await settle(() => text().includes('GitHub 拒绝建仓库'), 'the refusal')
    expect(created).toHaveLength(0)
    expect(nameField().value).toBe('No')
  })

  it('marks the own account that needs the App, lets the server refuse creating there, asks again and installs in one step', async () => {
    await openNewSpace('connected', undefined, (d) => {
      d.area.connections.owners['github'][0].needsInstall = true
    })
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Mine')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await settle(() => text().includes(tg.newSpace.needsInstall), 'the mark')
    expect(text()).toContain(tg.newSpace.needsInstallHelp('tang-gh'))
    // The other accounts are not marked.
    const marked = [...dialog()!.querySelectorAll('select option')].filter((o) =>
      o.textContent!.includes(tg.newSpace.needsInstall),
    )
    expect(marked).toHaveLength(1)
    m!.fx.fail(
      'POST',
      '/api/spaces',
      409,
      { code: 'app_not_installed', installUrl: 'https://github.com/apps/petalbook-notes/installations/new' },
      { times: 1 },
    )
    await click(button(t.spaces.create))
    await settle(() => text().includes(tg.newSpace.errors.notInstalled('tang-gh')), 'the refusal')
    // The server answered; the client did not refuse on its own.
    expect(posts()).toHaveLength(1)
    // "Check again" asks for the owners with the server's memory dropped.
    await click(button(tg.newSpace.checkAgain)!)
    await settle(
      () => m!.requests.some((r) => r.url === '/api/account/links/github/owners?recheck=true'),
      'the recheck',
    )
    await click(button(tg.newSpace.installOnMine))
    await settle(() => m!.requests.some((r) => r.url === '/api/account/links/github/start'), 'the request')
    expect(JSON.parse(m!.requests.find((r) => r.url === '/api/account/links/github/start')!.body!)).toEqual({
      purpose: 'repos',
      return: '/',
      install: true,
    })
  })

  it('offers other (by address): the space is made on the server and its sync settings open', async () => {
    const created: { space: SpaceInfo; openSync: boolean }[] = []
    m = await mount(
      <NewSpaceDialog
        open
        onOpenChange={() => {}}
        onCreated={(space, then) => created.push({ space, openSync: then?.openSync === true })}
      />,
      {
        signIn: true,
        fixtures: { mode: 'accounts', data: (d) => void githubProvider(d, 'app') },
        entries: ['/'],
      },
    )
    await settle(() => dialog() !== null && text().includes(tg.newSpace.whereOther), 'the option')
    await type(nameField(), 'By address')
    await click(checkbox(tg.newSpace.whereOther))
    await click(button(t.spaces.create)!)
    await settle(() => created.length === 1, 'the space')
    expect(created[0]!.openSync).toBe(true)
    // No place was made on a platform for it.
    expect(JSON.parse(posts()[0]!.body!).sync).toBeUndefined()
  })

  it("names the permission the App lacks instead of GitHub's refusal", async () => {
    await openNewSpace('connected')
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    m!.fx.fail('POST', '/api/spaces', 403, { code: 'app_missing_permission', missing: ['administration'] })
    await type(nameField(), 'Perm')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await click(button(t.spaces.create))
    await settle(() => text().includes(tg.newSpace.errors.missingPermission(['administration'])), 'the message')
  })

  it('offers only the places the settings allow, and starts at the default', async () => {
    // Only a remote of their own: no choice of this server, and the platform is where it starts.
    await openNewSpace('connected', undefined, () => {}, { effective: 'own', allowed: ['own'] })
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    expect(text()).not.toContain(tg.newSpace.whereServerHelp)
    // A plain address is given after the space is made, which a remote that must be there from the start cannot wait for.
    expect(text()).not.toContain(tg.newSpace.whereOtherHelp)
    expect(text()).toContain(tg.newSpace.addressNeedsServer)
    expect(checkbox(tg.newSpace.whereSync('GitHub')).checked).toBe(true)
  })

  it('offers the organisation storage when it can be used, and says nothing when it is the default', async () => {
    const created = await openNewSpace('off', undefined, () => {}, {
      effective: 'tenant',
      allowed: ['server', 'tenant'],
    })
    m!.fx.reply(
      'POST',
      '/api/spaces',
      { ...m!.fx.data.spaces[0].info, id: '01FZTENANT', name: 'Team' },
      { status: 201 },
    )
    await settle(() => text().includes(tg.newSpace.whereTenant), 'the organisation option')
    expect(checkbox(tg.newSpace.whereTenant).checked).toBe(true)
    await type(nameField(), 'Team')
    await click(button(t.spaces.create))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[0]!.body!)).toEqual({ name: 'Team', inbox: t.inbox.folderName })
    await settle(() => dialog() === null, 'the dialog closing')
  })

  it('says the mode when the person picks one other than the default', async () => {
    const created = await openNewSpace('off', undefined, () => {}, {
      effective: 'tenant',
      allowed: ['server', 'own', 'tenant'],
    })
    m!.fx.reply(
      'POST',
      '/api/spaces',
      { ...m!.fx.data.spaces[0].info, id: '01FZSERVER', name: 'Local' },
      { status: 201 },
    )
    await settle(() => text().includes(tg.newSpace.whereTenant), 'the organisation option')
    await click(checkbox(tg.newSpace.whereServer))
    await type(nameField(), 'Local')
    await click(button(t.spaces.create))
    await settle(() => created.length === 1, 'the space')
    expect(JSON.parse(posts()[0]!.body!).storage).toEqual({ mode: 'server' })
  })

  it('says what is available, not "pick a platform", when no platform is set up', async () => {
    await openNewSpace('off', undefined, () => {}, { effective: 'tenant', allowed: ['own', 'tenant'] })
    await settle(() => text().includes(tg.newSpace.whereTenant), 'the organisation option')
    expect(text()).toContain(tg.newSpace.noPlatform)
    expect(text()).not.toContain(tg.newSpace.addressNeedsServer)
  })

  it('hides a mode that cannot be used now, and says so when nothing is left', async () => {
    await openNewSpace('off', undefined, () => {}, {
      effective: 'tenant',
      allowed: ['tenant'],
      locked: true,
      missing: ['tenant'],
    })
    await settle(() => text().includes(tg.newSpace.whereTenantMissing), 'the notice')
    expect(text()).not.toContain(tg.newSpace.whereServerHelp)
    expect(button(t.spaces.create)!.disabled).toBe(true)
  })

  it('shows why a refused place was refused, in words', async () => {
    const created = await openNewSpace('connected')
    m!.fx.fail('POST', '/api/spaces', 409, { code: 'partial_unsupported', host: 'git.example.test' })
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await type(nameField(), 'Refused')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    await click(button(t.spaces.create))
    await settle(() => text().includes('git.example.test'), 'the reason')
    expect(created).toHaveLength(0)
    expect(dialog()!.querySelector('[role="alert"]')!.textContent).toContain('git.example.test')
  })

  it('fits a phone: one column, fields that may shrink, long names that break', async () => {
    await openNewSpace('connected')
    await settle(() => text().includes(tg.newSpace.whereSync('GitHub')), 'the option')
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => dialog()!.querySelector('select') !== null, 'the owners')
    const form = dialog()!.querySelector('form')!
    expect(form.className).toContain('grid-cols-1')
    expect(dialog()!.querySelector('fieldset')!.className).toContain('min-w-0')
    expect(dialog()!.querySelector<HTMLInputElement>('input[spellcheck="false"]')!.className).toContain('min-w-0')
    expect(dialog()!.querySelector('textarea')!.className).toMatch(/w-full min-w-0/)
  })
})

// --- the account's connection ---------------------------------------------------------

const settingsRoutes = (
  <Routes>
    <Route path="/settings/:section?" element={<SettingsRoute />} />
  </Routes>
)
const panel = () => document.querySelector<HTMLElement>('[data-slot="settings-dialog"]')

async function openSettings(github: GitHubState, entry = '/settings/connections') {
  m = await mount(settingsRoutes, {
    signIn: true,
    fixtures: { mode: 'accounts', data: (d) => void githubProvider(d, github) },
    entries: [entry],
  })
  await settle(() => panel() !== null, 'the settings')
}
const github = () => m!.fx.data.area.connections.providers.find((p) => p.id === 'github')!

describe('the GitHub section of the account', () => {
  it('says so when the server has no App, and where an administrator sets it up', async () => {
    await openSettings('off')
    await settle(() => panel()!.textContent!.includes(tg.account.notConfigured), 'the note')
    expect(panel()!.textContent).toContain(tg.account.notConfiguredAdmin)
  })

  it('starts connecting: asks the server for the address and says where to come back to', async () => {
    await openSettings('app')
    await settle(() => button(tg.account.connect, panel()!) !== undefined, 'the button')
    await click(button(tg.account.connect, panel()!))
    await settle(() => m!.requests.some((r) => r.url === '/api/account/links/github/start'), 'the request')
    const req = m!.requests.find((r) => r.url === '/api/account/links/github/start')!
    // Connecting goes through the App's installation page, which authorises in the same flow.
    expect(JSON.parse(req.body!)).toEqual({ purpose: 'repos', return: '/settings/connections', install: true })
  })

  it('can authorize alone when the App is installed already', async () => {
    await openSettings('app')
    await settle(() => button(tg.account.authorizeOnly, panel()!) !== undefined, 'the button')
    await click(button(tg.account.authorizeOnly, panel()!))
    await settle(() => m!.requests.some((r) => r.url === '/api/account/links/github/start'), 'the request')
    expect(JSON.parse(m!.requests.find((r) => r.url === '/api/account/links/github/start')!.body!)).toEqual({
      purpose: 'repos',
      return: '/settings/connections',
    })
  })

  it("offers to install the App on the person's own account when it is not there, and connects in the same step", async () => {
    await openSettings('connected')
    m!.fx.data.area.connections.owners['github'][0].needsInstall = true
    await m!.qc.invalidateQueries()
    await settle(() => panel()!.textContent!.includes(tg.account.needsInstallTitle('tang-gh')), 'the notice')
    expect(panel()!.textContent).toContain(tg.account.needsInstallBody)
    await click(button(tg.account.installOnMine, panel()!))
    await settle(() => m!.requests.some((r) => r.url === '/api/account/links/github/start'), 'the request')
    expect(JSON.parse(m!.requests.find((r) => r.url === '/api/account/links/github/start')!.body!)).toEqual({
      purpose: 'repos',
      return: '/settings/connections',
      install: true,
    })
  })

  it('says nothing about installing when the App is on the account', async () => {
    await openSettings('connected')
    await settle(() => panel()!.textContent!.includes('tang-gh'), 'the account')
    expect(panel()!.textContent).not.toContain(tg.account.needsInstallBody)
  })

  it('explains an installation that sent no code, and one that waits for approval', async () => {
    await openSettings('app', '/settings?provider=github&connection_error=installed')
    await settle(() => panel()!.textContent!.includes(tg.account.errors.installed), 'the explanation')
    await m!.unmount()
    m = undefined
    await openSettings('app', '/settings?provider=github&connection_error=requested')
    await settle(() => panel()!.textContent!.includes(tg.account.errors.requested), 'the explanation')
  })

  it('shows a connection, and warns how many spaces use it before disconnecting', async () => {
    await openSettings('connected')
    await settle(() => panel()!.textContent!.includes('tang-gh'), 'the account')
    expect(panel()!.textContent).toContain(tg.account.stateOk)
    // A space of the person's syncs through the connection.
    github().connection!.spaces = 1
    await m!.qc.invalidateQueries()
    await settle(() => panel()!.textContent!.includes(tg.account.spaces(1)), 'the count')
    await click(button(tg.account.disconnect, panel()!))
    await settle(() => document.querySelector('[role="alertdialog"]') !== null, 'the question')
    expect(document.querySelector('[role="alertdialog"]')!.textContent).toContain(tg.account.disconnectWarn(1))
    await click(
      [...document.querySelectorAll('[role="alertdialog"] button')].find(
        (b) => b.textContent === tg.account.disconnectConfirm,
      ),
    )
    await settle(
      () => m!.requests.some((r) => r.method === 'DELETE' && r.url === '/api/account/links/link-github/repos'),
      'the request',
    )
    await settle(() => button(tg.account.connect, panel()!) !== undefined, 'not connected any more')
  })

  it('shows an expired connection and offers to connect again', async () => {
    await openSettings('expired')
    await settle(() => panel()!.textContent!.includes(tg.account.expiredTitle), 'the notice')
    expect(button(tg.account.reconnect, panel()!)).toBeTruthy()
  })

  it('explains a failed connection once, from the address GitHub sent the browser back to', async () => {
    await openSettings('app', '/settings?provider=github&connection_error=account_in_use')
    await settle(() => panel()!.textContent!.includes(tg.account.errors.account_in_use), 'the explanation')
    expect([...panel()!.querySelectorAll('h2')].map((h) => h.textContent)).toContain(t.settingsUi.sections.connections)
    await settle(() => !locationText().includes('connection_error'), 'the address cleaned')
    expect(locationText()).toBe('/settings/connections')
    // A code nobody made is not a message.
    await m!.unmount()
    m = undefined
    await openSettings('app', '/settings/connections?provider=github&connection_error=constructor')
    await settle(() => panel()!.textContent!.includes(tg.account.errors.unknown), 'the generic message')
  })
})

// --- a space's remote ------------------------------------------------------------------

describe('the reminder of an empty repository a failed space left on GitHub', () => {
  it('lists it with its name and link, and connects a space to it when asked to try again', async () => {
    await openSettings('connected')
    await settle(() => panel()!.textContent!.includes('tang-gh'), 'the account')
    expect(panel()!.textContent).not.toContain(tg.account.leftoverTitle)
    leaveRepository(m!.fx.data, 'tang-gh/poems-repo')
    await m!.qc.invalidateQueries()
    await settle(() => panel()!.textContent!.includes(tg.account.leftoverTitle), 'the reminder')
    expect(panel()!.textContent).toContain('tang-gh/poems-repo')
    expect(panel()!.textContent).toContain('Poems')
    const link = [...panel()!.querySelectorAll<HTMLAnchorElement>('a')].find((x) =>
      x.textContent?.includes('tang-gh/poems-repo'),
    )!
    expect(link.getAttribute('href')).toBe('https://github.com/tang-gh/poems-repo')
    expect(link.rel).toContain('noopener')
    await click(button(tg.account.leftoverRetry, panel()!))
    await settle(() => m!.requests.some((r) => r.method === 'POST' && r.url === '/api/spaces'), 'the try')
    // The server forgets the reminder once the space follows the repository.
    github().leftovers = []
    await m!.qc.invalidateQueries()
    await settle(() => !panel()!.textContent!.includes(tg.account.leftoverTitle), 'the reminder going')
    const post = m!.requests.filter((r) => r.method === 'POST' && r.url === '/api/spaces').at(-1)!
    expect(JSON.parse(post.body!)).toEqual({
      name: 'Poems',
      storage: { sync: { provider: 'github', owner: 'tang-gh', name: 'poems-repo', create: false } },
    })
    expect(m!.fx.data.spaces.filter((x) => x.info.name === 'Poems')).toHaveLength(1)
  })

  it('keeps the reminder and says why when trying again fails', async () => {
    await openSettings('connected')
    await settle(() => panel()!.textContent!.includes('tang-gh'), 'the account')
    leaveRepository(m!.fx.data, 'tang-gh/stuck-repo')
    await m!.qc.invalidateQueries()
    await settle(() => button(tg.account.leftoverRetry, panel()!) !== undefined, 'the reminder')
    m!.fx.fail('POST', '/api/spaces', 409, { code: 'not_connected', message: 'connect your account first' })
    await click(button(tg.account.leftoverRetry, panel()!))
    await settle(
      () => m!.requests.filter((r) => r.method === 'POST' && r.url === '/api/spaces').length === 1,
      'the try',
    )
    await settle(() => document.body.textContent!.includes(tg.account.leftoverFailed), 'the failure')
    expect(panel()!.textContent).toContain('tang-gh/stuck-repo')
  })

  it('forgets it when the person says they deleted the repository', async () => {
    await openSettings('connected')
    await settle(() => panel()!.textContent!.includes('tang-gh'), 'the account')
    leaveRepository(m!.fx.data, 'tang-gh/gone-repo')
    await m!.qc.invalidateQueries()
    await settle(() => button(tg.account.leftoverDismiss, panel()!) !== undefined, 'the reminder')
    await click(button(tg.account.leftoverDismiss, panel()!))
    await settle(() => !panel()!.textContent!.includes(tg.account.leftoverTitle), 'the reminder going')
    expect(
      m!.requests.some((r) => r.method === 'DELETE' && /^\/api\/account\/leftovers\/github\/\d+$/.test(r.url)),
    ).toBe(true)
    expect(m!.fx.data.spaces.some((x) => x.info.name === 'Poems')).toBe(false)
  })
})

// The remote the server says a space has once it follows a repository on GitHub.
const appRemote = (over: Partial<Remote> = {}): Remote => ({
  url: 'https://github.com/kazuhahub/wiki.git',
  branch: 'main',
  provider: 'github',
  credential: {
    type: 'connection',
    provider: 'github',
    via: 'app',
    installationId: 5001,
    account: 'kazuhahub',
    repoId: 880_000_002,
  },
  state: 'ok',
  lastSync: '2026-11-25T10:00:00Z',
  error: '',
  errorCode: '',
  rewritten: null,
  ...over,
})

describe('the sync settings of a space', () => {
  const remotePath = '/api/spaces/:space/remote'
  async function openSync() {
    m = await mount(settingsRoutes, {
      signIn: true,
      fixtures: { mode: 'accounts', data: (d) => void githubProvider(d, 'connected') },
      entries: (fx) => {
        // The space follows no repository yet.
        fx.fail('GET', remotePath, 404, { code: 'not_found', message: 'this space follows no repository' })
        return ['/settings/sync']
      },
    })
    await settle(() => panel()?.textContent?.includes(t.remote.intro) === true, 'the sync settings')
    await settle(
      () => [...panel()!.querySelectorAll('label')].some((l) => l.textContent?.includes(tg.sync.github)),
      'the GitHub choice',
    )
  }
  // The server answers the next reads of the remote with `remote`.
  const follow = async (remote: Remote) => {
    m!.fx.clear()
    m!.fx.reply('GET', remotePath, remote)
    await m!.qc.invalidateQueries()
  }
  const radio = (label: string) =>
    [...panel()!.querySelectorAll('label')]
      .find((l) => l.textContent?.includes(label))!
      .querySelector<HTMLInputElement>('input[type="radio"]')!
  const puts = () => m!.requests.filter((r) => r.method === 'PUT' && r.url.endsWith('/remote'))

  it('can switch to the App: only the address and which access, and the PUT carries no secret', async () => {
    await openSync()
    m!.fx.reply('PUT', remotePath, appRemote())
    await click(radio(tg.sync.github))
    await settle(() => panel()!.textContent!.includes(tg.sync.viaApp), 'the choices')
    const url = panel()!.querySelector<HTMLInputElement>('input[inputmode="url"]')!
    await type(url, 'kazuhahub/wiki')
    await click(button(t.remote.connect, panel()!))
    await settle(() => puts().length === 1, 'the request')
    expect(JSON.parse(puts()[0].body!)).toEqual({
      url: 'https://github.com/kazuhahub/wiki.git',
      credential: { type: 'connection', provider: 'github', via: 'app' },
    })
    await follow(appRemote())
    await settle(() => panel()!.textContent!.includes(tg.sync.viaAppOf('kazuhahub')), 'the connected remote')
    expect(panel()!.textContent).not.toMatch(/ghs_|ghu_|token/i)
  })

  it('says why a host that cannot sync partially was refused, and what to do', async () => {
    await openSync()
    m!.fx.fail('PUT', remotePath, 409, { code: 'partial_unsupported', host: 'git.example.test' })
    await click(radio(tg.sync.github))
    await settle(() => panel()!.textContent!.includes(tg.sync.viaApp), 'the choices')
    await type(panel()!.querySelector<HTMLInputElement>('input[inputmode="url"]')!, 'kazuhahub/wiki')
    await click(button(t.remote.connect, panel()!))
    await settle(() => panel()!.textContent!.includes(t.remote.partialTitle), 'the reason')
    expect(panel()!.textContent).toContain('git.example.test')
    expect(panel()!.textContent).toContain(t.remote.partialAdvice)
    expect(panel()!.textContent).not.toContain(t.remote.conflictTitle)
  })

  it("can switch to the user's own connection, and chooses a repository from a list", async () => {
    await openSync()
    m!.fx.reply('PUT', remotePath, appRemote())
    await click(radio(tg.sync.github))
    await settle(() => panel()!.textContent!.includes(tg.sync.viaUser), 'the choices')
    await click(radio(tg.sync.viaUser))
    await click(button(tg.sync.showPicker, panel()!))
    await settle(() => panel()!.querySelectorAll('ul button').length > 0, 'the repositories')
    const first = panel()!.querySelector<HTMLButtonElement>('ul button')!
    const full = first.firstChild!.textContent!
    await click(first)
    expect(panel()!.querySelector<HTMLInputElement>('input[inputmode="url"]')!.value).toBe(full)
    await click(button(t.remote.connect, panel()!))
    await settle(() => puts().length === 1, 'the request')
    expect(JSON.parse(puts()[0].body!).credential).toEqual({ type: 'connection', provider: 'github', via: 'user' })
  })

  it('names the GitHub problem of a space in error and offers what fixes it', async () => {
    await openSync()
    await follow(
      appRemote({
        url: 'https://github.com/tang-gh/journal.git',
        credential: {
          type: 'connection',
          provider: 'github',
          via: 'user',
          user: { id: m!.fx.data.auth.signedIn!, username: 'admin' },
          login: 'tang-gh',
          repoId: 1,
        },
        state: 'error',
        error: 'GitHub says the authorization ran out',
        errorCode: 'connection_expired',
      }),
    )
    await settle(() => panel()!.textContent!.includes(tg.sync.codes.connection_expired.title), 'the notice')
    expect(panel()!.textContent).toContain(tg.sync.keptSafe)
    expect(button(tg.sync.reconnect, panel()!)).toBeTruthy()
  })

  it('sends the person whose account a space synced with to their connected accounts when access is lost', async () => {
    await openSync()
    const own = (id: string) =>
      appRemote({
        url: 'https://gitlab.example.com/tang/journal.git',
        provider: 'gitlab',
        credential: {
          type: 'connection',
          provider: 'gitlab',
          via: 'user',
          user: { id, username: 'someone' },
          login: 'tang',
          repoId: 1,
        },
        state: 'error',
        error: 'this space lost access',
        errorCode: 'access_lost',
      })
    await follow(own(m!.fx.data.auth.signedIn!))
    await settle(() => panel()!.textContent!.includes(tg.sync.codes.access_lost.title), 'the notice')
    // The text names no one provider, and the action goes to the account links.
    expect(tg.sync.codes.access_lost.title).not.toMatch(/GitHub/)
    expect(tg.sync.codes.access_lost.body()).not.toMatch(/GitHub/)
    expect(panel()!.querySelector('[role="alert"] a[href="/settings/connections"]')?.textContent).toContain(
      tg.sync.accountLinks,
    )
    // Someone else's connection: nothing for this person to reconnect.
    await follow(own('someone-else'))
    await settle(
      () => panel()!.querySelector('[role="alert"] a[href="/settings/connections"]') === null,
      'the notice of another person',
    )
  })
})

// --- the administrators' App -------------------------------------------------------------

describe('the GitHub section of the administration', () => {
  const routes = (
    <SidebarProvider>
      <Routes>
        <Route path="/admin/*" element={<AdminGate />} />
      </Routes>
    </SidebarProvider>
  )
  const field = (label: string) =>
    document.querySelector<HTMLInputElement | HTMLTextAreaElement>(
      `#${[...document.querySelectorAll('label')].find((l) => l.textContent === label)!.getAttribute('for')}`,
    )!

  it('saves the App, shows only what is safe to show, and keeps the key when it is left empty', async () => {
    m = await mount(routes, { entries: ['/admin/storage/connections'], signIn: true, fixtures: { mode: 'accounts' } })
    await settle(
      () =>
        document.querySelector('#github-private-key') !== null &&
        [...document.querySelectorAll('label')].some((l) => l.textContent === tg.admin.appId),
      'the form',
    )
    await type(field(tg.admin.appId), '1234567')
    await type(field(tg.admin.privateKey), '-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----')
    await type(field(tg.admin.clientId), 'Iv23liMockClientId')
    await type(field(tg.admin.clientSecret), 'a-secret')
    await click(button(tg.admin.save))
    await settle(() => text().includes(tg.admin.fingerprint), 'the saved App')
    expect(text()).not.toContain('a-secret')
    expect(text()).not.toContain('BEGIN RSA')
    expect(field(tg.admin.privateKey).value).toBe('')
    expect(text()).toContain(tg.admin.privateKeyKept)
    // The second save sends no key and no secret.
    await click(button(tg.admin.save))
    await settle(
      () => m!.requests.filter((r) => r.method === 'PUT' && r.url === '/api/admin/providers/github').length === 2,
      'the second save',
    )
    const second = JSON.parse(
      m!.requests.filter((r) => r.method === 'PUT' && r.url === '/api/admin/providers/github')[1].body!,
    )
    expect(second).toEqual({ appId: '1234567', clientId: 'Iv23liMockClientId' })
    expect(text()).toContain(tc.sections.storage)
  })

  it('says why GitHub refused the key or the secret', async () => {
    m = await mount(routes, { entries: ['/admin/storage/connections'], signIn: true, fixtures: { mode: 'accounts' } })
    await settle(
      () =>
        document.querySelector('#github-private-key') !== null &&
        [...document.querySelectorAll('label')].some((l) => l.textContent === tg.admin.appId),
      'the form',
    )
    await type(field(tg.admin.appId), '1234567')
    await type(field(tg.admin.privateKey), 'nonsense')
    m.fx.fail('PUT', '/api/admin/providers/github', 400, { code: 'key_invalid' }, { times: 1 })
    await click(button(tg.admin.save))
    await settle(() => text().includes(tg.admin.errors.codes.key_invalid), 'the key refusal')
    await type(field(tg.admin.privateKey), '-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----')
    await type(field(tg.admin.clientId), 'Iv23liMockClientId')
    await type(field(tg.admin.clientSecret), 'wrong')
    m.fx.fail('PUT', '/api/admin/providers/github', 400, { code: 'client_rejected' }, { times: 1 })
    await click(button(tg.admin.save))
    await settle(() => text().includes(tg.admin.errors.codes.client_rejected), 'the secret refusal')
  })

  it('refuses to remove the App while a space syncs through it', async () => {
    m = await mount(routes, {
      entries: ['/admin/storage/connections'],
      signIn: true,
      fixtures: {
        mode: 'accounts',
        data: (d) => {
          githubApp(d)
          // A person is connected, and a space syncs through the App.
          Object.assign(d.area.connections.github!, { spaces: 1, authorizations: 1 })
        },
      },
    })
    await settle(() => text().includes(tg.admin.usageValue(1, 1)), 'the usage')
    await click(button(tg.admin.delete))
    await settle(() => document.querySelector('[role="alertdialog"]') !== null, 'the question')
    expect(document.querySelector('[role="alertdialog"]')!.textContent).toContain(tg.admin.deleteBlocked(1))
    const confirm = [...document.querySelectorAll<HTMLButtonElement>('[role="alertdialog"] button')].find(
      (b) => b.textContent === tg.admin.deleteConfirm,
    )!
    expect(confirm.disabled).toBe(true)
  })
})

describe('the App in the administration: re-checking, warnings and making it on GitHub', () => {
  const routes = (
    <SidebarProvider>
      <Routes>
        <Route path="/admin/*" element={<AdminGate />} />
      </Routes>
    </SidebarProvider>
  )
  const configured = { mode: 'accounts' as const, data: (d: FixtureServer['data']) => githubApp(d) }

  it('shows the permissions of the last check until Re-check reads them from GitHub again', async () => {
    m = await mount(routes, { entries: ['/admin/storage/connections'], signIn: true, fixtures: configured })
    await settle(() => text().includes(tg.admin.recheck), 'the card')
    expect(text()).not.toContain(tg.admin.missing(['contents']))
    // GitHub's side changed: the card does not know until the server asks again.
    m.fx.data.area.connections.githubSide = {
      permissions: { administration: 'write', metadata: 'read' },
      missing: ['contents'],
      gaps: [{ installationId: 5001, account: 'kazuhahub', missing: ['contents'] }],
    }
    await m.qc.invalidateQueries()
    expect(text()).not.toContain(tg.admin.missing(['contents']))
    await click(button(tg.admin.recheck))
    await settle(
      () => m!.requests.some((r) => r.method === 'POST' && r.url === '/api/admin/providers/github/check'),
      'the request',
    )
    await settle(() => text().includes(tg.admin.missing(['contents'])), 'the missing permission')
    expect(text()).toContain(tg.admin.gaps(['kazuhahub (contents)']))
  })

  it('warns when the App can only be installed on one account, and says when GitHub did not say', async () => {
    m = await mount(routes, { entries: ['/admin/storage/connections'], signIn: true, fixtures: configured })
    await settle(() => button(tg.admin.recheck) !== undefined, 'the card')
    expect(text()).not.toContain(tg.admin.anyAccountNo)
    m.fx.data.area.connections.githubSide = { anyAccount: false }
    await click(button(tg.admin.recheck))
    await settle(() => text().includes(tg.admin.anyAccountNo), 'the warning')
    await settle(() => button(tg.admin.recheck) !== undefined, 'the button again')
    m.fx.data.area.connections.githubSide = { anyAccount: null }
    await click(button(tg.admin.recheck))
    await settle(
      () => text().includes(tg.admin.anyAccountUnknown) && !text().includes(tg.admin.anyAccountNo),
      'the note',
    )
  })

  it('makes the App on GitHub: asks the server for the manifest and posts it from the browser', async () => {
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {})
    try {
      m = await mount(routes, { entries: ['/admin/storage/connections'], signIn: true, fixtures: { mode: 'accounts' } })
      const start = {
        url: 'https://github.com/organizations/kazuhahub/settings/apps/new?state=s1',
        manifest: { name: 'PetalBook', public: true, redirect_url: 'https://notes.example.test/manifest/callback' },
      }
      m.fx.reply('POST', '/api/admin/providers/github/manifest', start)
      await settle(() => button(tg.admin.createButton) !== undefined, 'the button')
      await type(
        [...document.querySelectorAll<HTMLInputElement>('input')].find(
          (i) => i.closest('div')?.textContent?.includes(tg.admin.createOrg) && i.id !== '',
        )!,
        'kazuhahub',
      )
      await click(button(tg.admin.createButton))
      await settle(() => submit.mock.calls.length === 1, 'the form posted')
      const req = m.requests.find((r) => r.method === 'POST' && r.url === '/api/admin/providers/github/manifest')!
      expect(JSON.parse(req.body!)).toEqual({ organization: 'kazuhahub' })
      // What the server gave is what the browser posts, to the address the server gave.
      const form = submit.mock.contexts[0] as HTMLFormElement
      expect(form.method).toBe('post')
      expect(form.action).toBe(start.url)
      expect(JSON.parse((form.elements.namedItem('manifest') as HTMLInputElement).value)).toEqual(start.manifest)
    } finally {
      submit.mockRestore()
    }
  })

  it('does not offer to replace an App that is in use', async () => {
    m = await mount(routes, {
      entries: ['/admin/storage/connections'],
      signIn: true,
      fixtures: { mode: 'accounts', data: (d) => (githubApp(d), (d.area.connections.github!.authorizations = 1)) },
    })
    await settle(() => text().includes(tg.admin.createInUse), 'the note')
    expect(button(tg.admin.createButton)!.disabled).toBe(true)
  })

  it('says how the manifest flow ended, once, from the address GitHub sent the browser back to', async () => {
    m = await mount(routes, {
      entries: ['/admin/storage/connections?connection_app=created'],
      signIn: true,
      fixtures: configured,
    })
    await settle(() => text().includes(tg.admin.created), 'the notice')
    await settle(() => !locationText().includes('github_app'), 'the address cleaned')
    expect(locationText()).toBe('/admin/storage/connections')
    await m.unmount()
    m = undefined
    m = await mount(routes, {
      entries: ['/admin/storage/connections?connection_app_error=app_in_use'],
      signIn: true,
      fixtures: configured,
    })
    await settle(() => text().includes(tg.admin.returned.app_in_use), 'the explanation')
    await m.unmount()
    m = undefined
    m = await mount(routes, {
      entries: ['/admin/storage/connections?connection_app_error=constructor'],
      signIn: true,
      fixtures: configured,
    })
    await settle(() => text().includes(tg.admin.returned.unknown), 'the generic message')
  })
})

describe('Storage & sync', () => {
  const routes = (
    <SidebarProvider>
      <Routes>
        <Route path="/admin/*" element={<AdminGate />} />
      </Routes>
    </SidebarProvider>
  )
  it("shows the connections as cards on the administrators' page, under the name of the page", async () => {
    m = await mount(routes, {
      entries: ['/admin/storage/connections'],
      signIn: true,
      fixtures: { mode: 'accounts', data: (d) => githubApp(d) },
    })
    await settle(() => text().includes(tg.admin.connectionsTitle), 'the connections part')
    await settle(() => document.querySelector('[data-brand="github"]') !== null, 'the GitHub card with its mark')
  })

  it('does not say nothing was saved when GitHub made the App but it could not be saved: it says the App exists, and offers a retry', async () => {
    m = await mount(routes, {
      entries: ['/admin/storage/connections?connection_app_error=unavailable&pending=1&app=petalbook-notes'],
      signIn: true,
      fixtures: { mode: 'accounts' },
    })
    await settle(() => text().includes(tg.admin.createdOnGitHub('petalbook-notes')), 'the App that exists')
    expect(text()).toContain(tg.admin.pendingBody)
    expect(text()).not.toContain('什么也没有保存')
    // The server kept nothing (ten minutes are over): the page says how to finish by hand.
    m.fx.fail('POST', '/api/admin/providers/github/manifest/retry', 404, { code: 'not_found' })
    await click(button(tg.admin.pendingRetry))
    await settle(() => text().includes(tg.admin.pendingGone), 'the way to finish by hand')
    expect(m!.requests.some((r) => r.method === 'POST' && r.url === '/api/admin/providers/github/manifest/retry')).toBe(
      true,
    )
  })

  it('draws the GitHub mark from the page, and a plain mark for a platform without one', async () => {
    m = await mount(
      <>
        <BrandIcon type="github" />
        <BrandIcon type="gitlab" />
      </>,
      { entries: ['/'], fixtures: true },
    )
    expect(document.querySelectorAll('svg[data-brand="github"] path')).toHaveLength(1)
    expect(document.querySelectorAll('svg[data-brand]')).toHaveLength(1)
  })

  it('says where a new space is stored, this server or a platform, and never says Git or repository to a person', async () => {
    await openNewSpace('connected')
    await settle(() => text().includes(tg.newSpace.legend), 'the place choice')
    expect(text()).toContain(tg.newSpace.whereServer)
    expect(text()).toContain(tg.newSpace.whereSync('GitHub'))
    await click(checkbox(tg.newSpace.whereSync('GitHub')))
    await settle(() => text().includes(tg.newSpace.repoName), 'the name on GitHub')
    for (const word of [/\bGit\b/, /repositor/i, /仓库/]) expect(dialog()!.textContent).not.toMatch(word)
  })
})
