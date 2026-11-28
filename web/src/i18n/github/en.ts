export const githubEn = {
  newSpace: {
    legend: 'Where it is stored',
    option: 'Sync with GitHub',
    optionHelp:
      'The space stays in step with a place on GitHub: every change is synced there, and what changes there comes back.',
    loadingOwners: 'Looking up your GitHub accounts…',
    ownersFailed: 'Could not load the accounts',
    ownerNone: 'There is no GitHub account to sync to. Connect yours first.',
    connect: 'Connect GitHub',
    needsInstall: 'needs install',
    needsInstallHelp: (login: string) =>
      `GitHub lets nothing be created in ${login} until the App is installed on that account. Installing connects it in the same step.`,
    installOnMine: 'Install on my account',
    checkAgain: 'Check again',
    checkingAgain: 'Checking…',
    checkFailed: 'Could not check',
    owner: 'Owner',
    organisation: 'organisation',
    moreOwners: 'More accounts',
    viaUser: 'Made with your own GitHub account, so GitHub’s rules for you apply.',
    viaApp: 'Made through your organisation’s own GitHub installation.',
    repoName: 'Name on GitHub',
    repoNameHelp: 'Letters, digits, “.”, “_” and “-”, up to 100 characters.',
    repoInvalid: 'Use 1 to 100 letters, digits, “.”, “_” or “-”, not ending in .git.',
    private: 'Private',
    privateHelp: 'Only people you give access to on GitHub can see it.',
    description: 'Description (optional)',
    descriptionInvalid: 'At most 350 characters, on one line.',
    creating: 'Creating…',
    takenAsk: (full: string) => `${full} exists already. Sync this space to it?`,
    useExisting: 'Use the existing one',
    leftBehind: (full: string) =>
      `${full} was made on GitHub and is still empty. Nothing was deleted there. Try again to sync the new space to it; if you close this, a reminder stays in Settings, Connected accounts.`,
    useLeft: 'Sync to it',
    openOnGitHub: 'Open it on GitHub',
    whereServer: 'This server',
    whereServerHelp: 'The space exists on this server only.',
    whereSync: (name: string) => `Sync to ${name}`,
    whereOther: 'Other (by address)',
    whereTenant: 'Your organisation’s storage',
    whereTenantHelp: 'The server makes a repository for the space at the place your organisation has set up.',
    addressNeedsServer:
      'Your administrators require a space to follow a remote from the start, so a plain address cannot be given afterwards: pick a platform.',
    noPlatform:
      'Your administrators require a space to follow a remote from the start, and no platform is set up on this server to follow one.',
    noPlace: 'The administrators allow no place to store a new space for you right now.',
    whereTenantMissing: 'Your organisation has no storage target yet.',
    whereOtherHelp:
      'The space is made on this server first; you then give the address to sync with in its sync settings.',
    errors: {
      nameTaken: 'That owner already has one with this name.',
      notInInstallation:
        'The App’s installation there does not reach the new place, and it could not be added. Add it in the installation’s settings on GitHub, then try again.',
      pushFailed: 'It was made on GitHub, but the space could not be synced to it.',
      noPermission: (reason: string) => `GitHub refused: ${reason}`,
      ownerNotAllowed: 'You cannot create anything for that owner.',
      notAuthorized: 'Connect your GitHub account first (Settings, Connected accounts).',
      notConfigured: 'This server has no GitHub App set up.',
      rateLimited: 'GitHub is limiting this server for a moment. Try again shortly.',
      unavailable: 'GitHub could not be reached. Nothing was created; try again.',
      notInstalled: (login: string) =>
        `The App is not installed on your account ${login}, and GitHub lets nothing be made there until it is. Use “Install on my account” first.`,
      missingPermission: (names: readonly string[]) =>
        `The App or its installation lacks the ${[...names].join(', ')} permission. An administrator must give it to the App on GitHub and accept the change in the installation’s settings.`,
    },
  },
  account: {
    title: 'Connected accounts',
    intro: 'Connect your GitHub account to sync new spaces to GitHub, or to sync spaces you already have.',
    notConfigured: 'This server has no GitHub set up, so there is nothing to connect.',
    notConfiguredAdmin: 'You can set it up in the administration, under Storage & sync.',
    userAuthOff: 'People cannot connect their GitHub accounts on this server yet.',
    userAuthOffAdmin:
      'It needs the App’s client id and secret, and the server’s public address (--public-url); see the administration, Storage & sync.',
    install: (app: string) => `Install ${app} on a GitHub account or organisation`,
    notConnected: 'Your GitHub account is not connected.',
    connect: 'Connect GitHub',
    connectHelp:
      'Opens the App’s page on GitHub. Install it on your own account there; that authorises it too, in one step.',
    authorizeOnly: 'Already installed? Authorize only',
    installOnMine: 'Install on my account',
    needsInstallTitle: (login: string) => `The App is not installed on your account ${login}`,
    needsInstallBody:
      'GitHub lets nothing be made in your own account until the App is installed there. Organisations are not affected.',
    reconnect: 'Connect again',
    startFailed: 'Could not start connecting to GitHub.',
    failed: 'Could not connect to GitHub',
    errors: {
      denied: 'You did not allow it on GitHub.',
      expired: 'The attempt took too long, or was not started from this browser. Try again.',
      provider_error: 'GitHub refused the request or could not be reached. Try again.',
      installed:
        'The App was installed, but GitHub did not ask you to authorize it. Use “Already installed? Authorize only”.',
      requested:
        'The organisation’s owners must approve the installation on GitHub first. Connect again once they have.',
      account_in_use: 'That GitHub account is connected to another user of this server.',
      different_account:
        'Spaces here sync through the GitHub account you connected before. Disconnect it first, or switch those spaces to another credential.',
      last_method:
        'You sign in with the GitHub account linked to you, and it is your only way in. Set a password or link another sign-in method first, or authorize with that same account.',
      installation_not_yours:
        'GitHub does not list that installation for your GitHub account, so it was not connected. Install it again from here.',
      wrong_account:
        'The App was installed on an organisation, not on your own account. An administrator of your organisation on this server connects an organisation’s installation.',
      installation_taken: 'That installation is already connected to another organisation or person on this server.',
      not_installer:
        'GitHub does not say that you installed the App there. The person who installed it on GitHub connects it.',
      forbidden: 'Connecting GitHub is turned off for you.',
      not_connected:
        'GitHub did not authorize you in the same step. Use “Already installed? Authorize only”, then install again.',
      unknown: 'Something went wrong.',
    },
    account: 'Account',
    accountHelp: 'The GitHub account connected to you.',
    stateOk: 'Connected',
    stateExpired: 'Expired',
    connectedAt: 'Connected',
    renews: 'Renewed before',
    renewsHelp:
      'The server renews the connection by itself, so it lasts as long as it is used or the server is running.',
    usedBy: 'Spaces using it',
    usedByHelp: 'Spaces that sync through your connection instead of the App.',
    spaces: (n: number) => (n === 1 ? '1 space' : `${n} spaces`),
    expiredTitle: 'The connection has expired',
    expiredBody:
      'GitHub no longer accepts it (it expired or was revoked). Spaces that sync through it can still be read; changes wait, and drafts are kept, until you connect again.',
    disconnect: 'Disconnect',
    disconnectTitle: 'Disconnect GitHub?',
    disconnectWarn: (n: number) =>
      `${n === 1 ? '1 space syncs' : `${n} spaces sync`} through this connection. It will stop syncing and show an error until it is switched to another credential or you connect again. Nothing in it is lost.`,
    disconnectNone: 'No space syncs through this connection. The server also asks GitHub to revoke it.',
    cancel: 'Cancel',
    disconnectConfirm: 'Disconnect',
    disconnected: 'GitHub disconnected',
    disconnectFailed: 'Could not disconnect',
    leftoverTitle: 'Spaces that did not finish syncing',
    leftoverHelp:
      'Making these spaces failed after their place on GitHub was made. It was kept, empty; nothing was deleted there. Sync a space to one and try again, or say you deleted it.',
    leftoverFor: (space: string, when: string) => `For the space “${space}”, made ${when}`,
    leftoverRetry: 'Sync to it and try again',
    leftoverDismiss: 'I deleted it',
    leftoverRetried: (space: string, repo: string) => `The space “${space}” now syncs to ${repo}`,
    leftoverFailed: 'Could not sync to it',
    leftoverDismissFailed: 'Could not forget it',
  },
  sync: {
    github: 'GitHub',
    help: 'Sync through the server’s GitHub App or through your own GitHub connection. No token is stored for the space.',
    viaLabel: 'Which GitHub access',
    viaApp: 'The GitHub App',
    viaAppHelp:
      'Uses the App’s installation on this sync location, so it does not depend on anyone being connected. A personal space takes your own installation on your account; an organisation’s space only the organisation’s own.',
    viaUser: 'My GitHub connection',
    viaUserHelp:
      'Syncs as you. It stops if you disconnect, lose the right to manage this space, or your connection expires.',
    needConnection: 'Connect your GitHub account first (Settings, Connected accounts).',
    mismatchGitHub: 'Use the sync location’s address on GitHub (https://github.com/owner/name), or owner/name.',
    viaAppOf: (account: string) => `GitHub App (${account})`,
    viaUserOf: (username: string, login: string) => `GitHub connection of ${username}${login ? ` (${login})` : ''}`,
    reconnect: 'Connect GitHub again',
    accountLinks: 'Open connected accounts',
    reconnectFailed: 'Could not start connecting',
    keptSafe: 'The space can still be read. Changes are refused until this is fixed, and drafts are kept.',
    openInstall: 'Open the App’s installation page',
    codes: {
      connection_expired: {
        title: 'The GitHub authorisation has expired',
        body: (who: string) =>
          `${who ? `${who}` : 'The person this space syncs through'} must connect GitHub again. It is tried again at once when they do.`,
      },
      connection_missing: {
        title: 'No GitHub connection to sync with',
        body: (who: string) =>
          `${who ? `${who} has` : 'The person this space syncs through has'} disconnected GitHub, can no longer sign in, or can no longer manage this space. Connect yourself and switch the credential, or switch to the App.`,
      },
      installation_gone: {
        title: 'The App can no longer reach this sync location',
        body: () =>
          'The App was uninstalled or suspended there, or the location was taken out of its selection. Install it again or add it back.',
      },
      app_missing_permission: {
        title: 'The App lacks a permission',
        body: () =>
          'GitHub refuses a token because the App or its installation lacks the Contents permission. An administrator gives it to the App on GitHub, accepts the change in the installation’s settings and presses Re-check in the administration.',
      },
      access_lost: {
        title: 'This space lost access to its repository',
        body: () =>
          'What it synced with is no longer connected here: the person whose account it used disconnected it, or it expired, or the installation it went through was disconnected from the space’s owner. The notes are kept. Connect the account again, or connect the space to another repository.',
      },
      provider_not_configured: {
        title: 'The server’s GitHub App does not work',
        body: () => 'The App is not set up, or GitHub no longer accepts its key. An administrator must fix it.',
      },
    },
    problems: {
      provider_not_configured: 'This server has no GitHub App set up.',
      not_connected: 'Connect your GitHub account first (Settings, Connected accounts).',
      owner_not_allowed:
        'You cannot use this sync location: the installation that reaches it is not this space’s owner’s own, and an organisation’s space never syncs through a person’s own account.',
      repo_not_in_installation:
        'The App is not installed on this sync location, or its installation does not reach it.',
      provider_refused: 'GitHub refused.',
      app_missing_permission: 'The App or its installation lacks a permission. An administrator must fix it.',
    },
    showPicker: 'Choose from a list…',
    hidePicker: 'Hide the list',
    noOwners: 'There are no accounts to list sync locations of. You can type owner/name above.',
    pickOwner: 'Account',
    filter: 'Filter the locations shown',
    reposFailed: 'Could not load the sync locations',
    noRepos: 'No locations here (or none match). GitHub cannot search by name: load more, or type owner/name above.',
    private: 'private',
    public: 'public',
    readOnly: 'read only',
    moreRepos: 'More locations',
  },
  installs: {
    titleMine: 'GitHub on your own account',
    introMine:
      'Install the site’s GitHub App on your own GitHub account. Your own spaces can then sync through it, without depending on your connection staying fresh; nobody else’s spaces can use it.',
    titleOrg: 'GitHub for the organisation',
    introOrg:
      'Install the site’s GitHub App on the organisation’s GitHub organisation. The organisation’s spaces sync only through it, never through a member’s own GitHub account. It belongs to the organisation: any of its administrators can manage or disconnect it.',
    install: 'Install on GitHub',
    installing: 'Opening GitHub…',
    account: 'GitHub organisation',
    accountHelp:
      'Its name on GitHub, as in github.com/name. GitHub then asks you to confirm, and this server checks that you can reach it.',
    accountInvalid: 'Give the organisation’s name on GitHub: letters, digits and “-”.',
    off: 'Connecting GitHub is turned off on this server.',
    notConfigured: 'This server has no GitHub App set up yet.',
    loadFailed: 'Could not load the installations',
    none: 'Nothing is installed yet.',
    ownerPerson: 'Yours',
    ownerOrg: 'The organisation’s',
    connectedAt: (when: string) => `Connected ${when}`,
    pending: 'Connecting…',
    pendingFailed: 'Not connected',
    pendingHelp: (until: string) =>
      `Waiting for GitHub to confirm who installed it. This usually takes a few seconds: refresh to see it; if nothing comes by ${until}, uninstall the App on GitHub, then install it again from here.`,
    refresh: 'Refresh',
    refreshFor: (account: string) => `Refresh ${account}`,
    spaces: (n: number) => (n === 1 ? '1 space syncs through it' : `${n} spaces sync through it`),
    manage: 'Manage on GitHub',
    repos: 'Repositories',
    hideRepos: 'Hide repositories',
    reposFailed: 'Could not load the repositories',
    noRepos: 'It reaches no repositories yet. Choose them in its settings on GitHub.',
    moreRepos: 'More repositories',
    private: 'private',
    disconnect: 'Disconnect',
    disconnectFor: (account: string) => `Disconnect ${account}`,
    disconnectTitle: (account: string) => `Disconnect ${account}?`,
    disconnectBody:
      'It stays installed on GitHub (uninstall it there). The spaces below lose GitHub access: their notes are kept, and they stop syncing until GitHub is connected again or they are connected to another repository. To connect an organisation’s installation again later, uninstall the App on GitHub, then install it again from here.',
    disconnectGrace:
      'Where this server requires every space to sync to a repository, they count as having none, and each gets the usual time to choose a new repository, counted from the moment you disconnect, even if its earlier time had run out.',
    disconnectSpaces: (n: number) => (n === 1 ? '1 space syncs through it:' : `${n} spaces sync through it:`),
    disconnectMore: (n: number) => `and ${n} more`,
    disconnectNone: 'No space syncs through it.',
    loadingSpaces: 'Looking up the spaces…',
    spacesFailed: 'Could not look up the spaces that sync through it',
    cancel: 'Cancel',
    disconnectConfirm: 'Disconnect',
    disconnected: (account: string) => `${account} is disconnected`,
    disconnectFailed: 'Could not disconnect',
    startFailed: 'Could not start installing',
    returned: {
      denied: 'You did not allow it on GitHub.',
      expired: 'The attempt took too long, or was not started from this browser. Try again.',
      provider_error: 'GitHub refused the request or could not be reached. Try again.',
      requested: 'The organisation’s owners must approve the installation on GitHub first. Try again once they have.',
      forbidden:
        'You may not connect an installation here: connecting GitHub is off, or you are not the organisation’s administrator.',
      installation_not_yours:
        'GitHub does not list that installation for your GitHub account, so it was not connected. Install it again from here.',
      wrong_account: 'The App was installed on another account than the one asked for, so it was not connected.',
      installation_taken: 'That installation is already connected to another organisation or person on this server.',
      not_installer:
        'GitHub does not say that you installed the App on that organisation, so it was not connected. The person who installed it there (an owner of the organisation) connects it from here, with their own GitHub account.',
      not_connected:
        'GitHub did not authorize you in the same step. Connect your GitHub account first, then install again.',
      unknown: 'Something went wrong.',
    },
  },
  admin: {
    connectionsTitle: 'Connections',
    connectionsIntro:
      'One card per platform: whether it is set up, who is connected, and which spaces use it. GitHub is below.',
    loadFailed: 'Could not load the GitHub App',
    title: 'GitHub App',
    intro:
      'One GitHub App for the whole server. The easiest way is the button below, which makes it on GitHub for you. Or make it yourself on GitHub (Settings, Developer settings), installable on any account, with “Request user authorization (OAuth) during installation” ticked and with the Contents (read and write), Metadata (read) and Administration (read and write) permissions and its webhook active at the address below, with a secret, then copy its details here. The key and the secrets are kept encrypted and never shown again.',
    openApp: 'Open on GitHub',
    fingerprint: 'Key fingerprint',
    permissions: 'Permissions',
    usage: 'In use',
    usageValue: (spaces: number, users: number) =>
      `${spaces} space${spaces === 1 ? '' : 's'} sync through it, ${users} user${users === 1 ? ' is' : 's are'} connected`,
    missing: (list: readonly string[]) =>
      `The App lacks these permissions: ${[...list].join(', ')}. Saving works, but spaces can only be connected to repositories that exist while Administration is missing, and syncing needs Contents. After changing them on GitHub, press Re-check.`,
    userAuthOff:
      'Users cannot connect their GitHub accounts yet: that needs the client id and secret below and the server’s public address (--public-url).',
    recheck: 'Re-check',
    rechecking: 'Checking…',
    rechecked: 'Checked with GitHub',
    checkedAt: 'Last checked',
    gaps: (list: readonly string[]) =>
      `These installations have not accepted the App’s permissions yet: ${[...list].join('; ')}. Their owners accept the change in the installation’s settings on GitHub.`,
    anyAccountNo:
      'The App can only be installed on one account. A person cannot install it on their own account, so repositories in personal accounts will not work. In the App’s settings on GitHub, under “Install App”, choose “Any account”.',
    anyAccountUnknown:
      'GitHub did not say whether the App can be installed on any account. Personal accounts need “Any account” in the App’s settings.',
    anyAccountYes: 'The App can be installed on any account.',
    createTitle: 'Create the App on GitHub',
    createHelp:
      'One click makes the App on GitHub with the right settings: installable on any account, asking for authorisation during installation, with the Administration, Contents and Metadata permissions and no webhook. GitHub shows what will be made and asks you to confirm; it then sends you back here and the details are saved.',
    createOrg: 'Organisation (optional)',
    createOrgHelp: 'Leave empty to make it in your own GitHub account.',
    createName: 'App name (optional)',
    createNameHelp: 'GitHub’s App names are unique; change it if GitHub says it is taken.',
    createButton: 'Create the App on GitHub',
    createStarting: 'Opening GitHub…',
    createInUse: 'Spaces still sync through the App, or users are connected, so it cannot be replaced.',
    createManual: 'Or enter an App you made yourself below.',
    created: 'The GitHub App was created and saved.',
    createFailed: 'The App could not be created',
    createdOnGitHub: (app: string) => `The App “${app}” exists on GitHub.`,
    pendingBody: 'Its key is kept, encrypted, in the server for ten minutes: try saving it again.',
    pendingRetry: 'Try saving again',
    pendingRetrying: 'Saving…',
    pendingSaved: 'GitHub App saved',
    pendingGone:
      'What was kept is gone (more than ten minutes, or already used). Open the App on GitHub, make a private key for it, and enter its details below by hand.',
    pendingLost:
      'It could not be kept. Open the App on GitHub, make a private key for it, and enter its details below by hand.',
    returned: {
      expired: 'The attempt took too long, or was not started from this browser. Try again.',
      provider_error:
        'GitHub did not hand over the App, or it could not be saved. If it was created on GitHub, delete it there and try again, or enter its details below.',
      app_in_use: 'Spaces still sync through the App, or users are connected, so it was not replaced.',
      unavailable: 'GitHub made the App, but this server could not ask GitHub about it, so it is not saved yet.',
      unknown: 'Something went wrong.',
    },
    appId: 'App ID',
    appIdHelp: 'The number on the App’s settings page.',
    privateKey: 'Private key',
    keepKey: 'Leave empty to keep the saved key',
    privateKeyKept: 'A key is saved. Paste a new one (PEM) only to replace it.',
    privateKeyHelp: 'The contents of the .pem file GitHub gave you (an RSA key).',
    clientId: 'Client ID',
    clientIdHelp: 'Needed for people to connect their GitHub accounts. Empty removes the client id and secret.',
    clientSecret: 'Client secret',
    secretKept: 'A secret is saved. Enter one only to replace it.',
    secretHelp: 'Made on the App’s settings page, with the client id.',
    callbackHelp: 'Set this as the App’s callback URL on GitHub:',
    webhookSecret: 'Webhook secret',
    webhookSecretKept: 'A webhook secret is saved. Enter one only to replace it.',
    webhookSecretHelp:
      'The secret of the App’s webhook on GitHub. GitHub signs its events with it; without it an organisation cannot connect its installation.',
    webhookHelp: 'Set this as the App’s webhook URL on GitHub (webhook active):',
    signinCallbackHelp: 'And this one too, for signing in with GitHub (turn it on under Sign-in providers):',
    noPublicUrl: 'The server has no public address (--public-url).',
    needIds: 'Give the App ID and, the first time, the private key.',
    needSecret: 'The client id and the client secret go together.',
    checking: 'Checking with GitHub…',
    save: 'Save',
    saved: 'The GitHub App is saved',
    delete: 'Remove the App',
    deleteTitle: 'Remove the GitHub App?',
    deleteDescription: (users: number) =>
      `The App’s details are deleted from the server${users > 0 ? `, and so are the ${users} user connection${users === 1 ? '' : 's'} (GitHub is asked to revoke them)` : ''}. Nothing on GitHub is removed.`,
    deleteBlocked: (spaces: number) =>
      `${spaces} space${spaces === 1 ? ' syncs' : 's sync'} through the App. Switch them or disconnect them first.`,
    cancel: 'Cancel',
    deleteConfirm: 'Remove',
    deleted: 'The GitHub App is removed',
    errors: {
      failed: 'Something went wrong.',
      codes: {
        key_invalid: 'That is not an RSA private key in PEM form.',
        app_rejected: 'GitHub does not accept this key for this App ID.',
        client_rejected: 'GitHub does not accept this client id and secret for the App.',
        provider_in_use: 'Spaces still sync through the App, or users are connected. Switch or disconnect those first.',
        provider_host_mismatch:
          'Signing in with GitHub is set up for another GitHub host, and people have linked accounts there. Delete that sign-in provider, or remove its linked accounts, first.',
        unavailable: 'GitHub could not be reached. Nothing was saved.',
        provider_not_configured: 'GitHub no longer accepts the saved key. Save the App again.',
        public_url_needed: 'The server has no public address (--public-url), so GitHub cannot send you back.',
      },
    },
  },
}
