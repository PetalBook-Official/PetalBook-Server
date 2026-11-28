import type { ApiErrorTexts } from '../apiErrorsTypes.ts'

export const apiErrorsEn: ApiErrorTexts = {
  // Any request.
  invalid: 'The request is not right. Check it and try again.',
  not_found: 'That could not be found. It may have been deleted or moved.',
  forbidden: 'You are not allowed to do that.',
  conflict: 'It was just changed elsewhere. Refresh and try again.',
  exists: 'One with that name already exists. Choose another name.',
  internal: 'The server ran into a problem. Try again in a moment.',
  unauthenticated: 'The username or password is wrong, or your sign-in has ended.',
  unavailable: (p) =>
    p.wait
      ? `Cannot be reached right now. Try again in ${p.wait}. Nothing was lost.`
      : 'Cannot be reached right now. Nothing was lost; try again later.',
  rate_limited: (p) => (p.wait ? `Too many attempts. Try again in ${p.wait}.` : 'Too many attempts. Try again later.'),
  busy: 'This is busy right now; try again in a moment.',
  request_failed: 'The request did not succeed.',
  status: 'The other side answered with a status that cannot be used.',
  no_text: 'The answer has no text to show.',
  already_invited: 'This address has an open invitation already.',
  answer_reveals_secret: 'The answer carried a secret, so it was refused.',
  not_cached: (p) =>
    p.wait
      ? `This space is not on the server yet. Try again in ${p.wait}.`
      : 'This space is not on the server yet. Try again in a moment.',
  restoring: 'This space is being restored. Try again in a moment.',

  // Files and sizes.
  file_too_large: (p) =>
    p.name && p.size
      ? `"${p.name}" (${p.size}) is over the limit for one file${p.limit ? ` of ${p.limit}` : ''}.`
      : `The file is over the limit for one file${p.limit ? ` of ${p.limit}` : ''}.`,
  space_full: 'This space is full and cannot hold more.',
  exports_full: 'The exports already take all the room they may. Delete some, or wait for them to expire.',
  changes_expired: 'The offline position is too old. Read the catalogue again.',

  // Spaces, members and groups.
  space_disabled: 'This space has been disabled by an administrator.',
  no_permission: 'Your role in this space does not allow that.',
  prefs_full: 'Your preferences are full (at most 100 of them, 256 KiB in all). This one is kept in this browser only.',
  personal_space: 'A personal space belongs to one person and cannot be shared, transferred or deleted.',
  last_owner: 'A space needs at least one owner. Make someone else an owner first.',
  owner_required: 'There is nobody to put back as owner. Name one.',
  owner_not_allowed: 'That owner cannot be used.',
  user_not_found: 'There is no such user, or the user is disabled. Check the username.',
  user_disabled: 'That user is disabled and cannot own a space.',
  view_as_read_only: 'Viewing as someone is read only.',
  view_as_ended: 'This view has ended.',
  heir_required: 'This person still owns spaces. Name who takes them over first.',
  quota_exceeded: (p) =>
    p.quota === 'spaces' && p.limit
      ? `The limit on spaces is reached (at most ${p.limit}${p.used ? `, ${p.used} used` : ''}).`
      : 'A limit is reached, so no more can be created.',
  group_not_found: 'There is no such group.',
  group_in_use: (p) =>
    `This group is still in use${p.spaces ? ` (${p.spaces} spaces)` : ''}. Remove where it is used first.`,
  group_limit: 'The limit on groups is reached.',
  group_synced: 'The members of this group come from the sign-in service and cannot be changed by hand.',
  group_not_synced: 'This group is not synced from the sign-in service, so that cannot be set.',
  member_via_group: 'This person is a member through a group. Remove them from the group.',
  default_tenant: 'The default organisation cannot be changed that way.',
  template_limit: 'A space has at most 200 templates. Delete one you do not use first.',
  template_too_large: 'The template is too large: 256 KiB at most.',
  template_not_found: 'That template does not exist. It may have just been deleted.',
  tenant_limit: 'The limit on organisations is reached.',
  feature_off: (p) =>
    p.feature
      ? `The ${p.feature} feature is turned off for your organisation.`
      : 'Your organisation does not have this feature.',
  takeover_required:
    'Only a member of the space can change its owners or its remote. An administrator takes the space over first.',
  tenant_not_empty:
    'The organisation still has users, spaces, groups, invites or items in the recycle bin, so it cannot be moved as it is.',
  plan_in_use: 'Organisations still use this plan. Give them another first.',
  plan_limit: 'The limit on plans is reached.',
  not_fixable: 'This kind of problem cannot be fixed here.',
  remote_required: 'This space needs a remote repository first.',
  partial_unsupported: (p) =>
    `${p.host ?? 'This host'} cannot sync a part of a repository, and the administrators do not allow a space on a host that cannot.`,
  no_storage_target:
    'Your organisation has no storage target yet. Ask its administrators to set one up, or pick another place.',
  not_implemented: 'This is not available on this server yet.',
  confirm_stale: 'Things changed after the confirmation. Confirm again.',
  confirm_required: 'This feature is in use. Look at what would stop, then confirm.',

  // Settings.
  setting_locked: 'This setting is locked by the level above and cannot be changed.',
  setting_restricted: 'This value is outside what the level above allows.',
  setting_above_max: 'The value is above the maximum; raise the maximum first.',
  setting_until_past: 'The rule ends at a time that has already passed: give a later end, or none.',
  setting_rule_needs_max: 'A plan or billing rule for this setting must set a maximum or a lock.',
  mail_not_configured: 'Mail is not set up, so that cannot be done.',
  needs_mail: 'Mail is not set up, so that cannot be turned on yet.',
  needs_tenancy: 'Multiple organisations are not turned on, so that cannot be turned on yet.',
  slug_reserved: 'That name is reserved. Choose another.',
  slug_taken: 'That organisation address is taken. Choose another.',
  org_limit: 'No more organisations can be made.',
  mail_insecure: 'The mail connection is not encrypted. Use an encrypted one.',
  mail_from_required: 'Give the address mail is sent from.',
  mail_needs_public_url: 'The links in the messages point to the public address. Start the server with --public-url.',
  public_url_required: 'Sign-in providers need the public address. Start the server with --public-url.',
  public_url_needed: 'The public address of the server (--public-url) has to be set first.',
  oidc_issuer_invalid: 'The address of the sign-in service must be https.',
  template_unknown: 'There is no such sign-in template.',
  template_fixed: "A provider made from a template keeps the template's issuer and addresses.",
  apple_key_invalid: 'The key must be the .p8 file Apple gave.',
  saml_metadata_invalid: "The sign-in service's metadata cannot be used.",
  saml_needs_https: 'SAML needs an https public address.',
  domain_required: 'Verify the domain first: this needs a verified domain.',
  domain_public:
    'That is a public mail service’s domain: anyone can have an address there, so no organization can claim it.',
  domain_suffix:
    'That is a public suffix (such as co.uk or github.io), under which anyone can register a name. Claim your own domain below it.',
  domain_exists: 'This organization has claimed that domain already.',
  domain_limit: 'An organization can claim at most 20 domains.',
  domain_claimed: 'Another organization has verified this domain. A site administrator can transfer it.',
  domain_code_expired: 'The verification code has expired. Make a new one and publish it.',
  domain_verified: 'The domain is verified: its record stays as it is.',
  domain_managed:
    'An organization manages the addresses of this email domain. Ask it to add you instead of making another.',
  join_pending:
    'You have asked to join this organization already. Wait for its answer, or withdraw that request first.',
  join_decided: 'This request is no longer waiting: it was decided, withdrawn or cancelled, or it expired.',
  join_limit: 'This organization has too many requests to join waiting. Try again later.',
  tenant_disabled: 'This organization is disabled: nobody joins it now.',
  already_linked: 'You already have an account linked at this provider. Unlink it first to link another.',
  last_method: 'This is the last way to sign in. Set a password or link another sign-in method first.',
  github_app_client:
    "This provider's client is the GitHub App's: change it with the App's settings in the repository providers.",
  local_admin_required: 'An administrator who can sign in with a password has to exist first.',
  last_local_admin:
    'This organisation requires single sign-on: keep at least one enabled administrator of it with a password.',
  sso_only_with_registration:
    'With password sign-in off nobody could finish registering an organisation. Turn organisation creation off first.',
  sso_provider_required:
    'Turn on a single sign-on for this organisation first (its own, or one the site offers it): otherwise its people could not sign in.',
  sso_required: 'This organisation signs its people in with single sign-on, not with a password.',
  provider_unavailable: 'The sign-in this request was made with is not available to this organisation any more.',
  captcha_failed: 'The human check did not pass. Try again.',
  captcha_needs_public_url:
    'A third-party captcha is checked against the public address. Start the server with --public-url.',
  captcha_secret_required: 'Give the secret of the captcha service.',
  captcha_site_key_required: 'Give the site key of the captcha service.',

  // Signing in and accounts.
  local_login_refused: 'This account cannot sign in with a password. Use the sign-in service.',
  registration_closed: 'Registration is closed here: no account can be made now, not even from an invitation.',
  invite_required:
    'Accounts here are made from an invitation. Open the link in yours, or ask an administrator to invite you.',
  people_limit: 'This organisation has reached the number of people it may have.',
  email_domain_not_verified:
    'This address is not on a verified domain of the organisation, so its single sign-on could never match it. Verify the domain first.',
  sso_default_tenant:
    'People of the default tenant cannot be made by hand while single sign-on is required: they sign in with it themselves.',
  site_admin_explicit:
    'An administrator of the default tenant is an administrator of the whole server: say so to make one.',
  password_reset_off: 'Password reset is not available.',
  email_domain_not_allowed: 'The domain of this email address is not allowed.',
  email_unverified: 'This email address has not been verified yet.',
  email_verified: 'This email address is already verified.',
  email_limit: 'An account can have at most 10 email addresses.',
  primary_email: 'The primary email address cannot be removed; make another one primary first.',
  email_taken: 'Another account already has this email address.',
  email_not_ascii: 'Email addresses with characters outside ASCII are not accepted on this server.',
  terms_required: 'Accept the terms first.',
  passkey_required: 'This account has to sign in with a passkey first.',
  password_change_required: 'Choose a new password of your own first.',
  passkey_failed: 'The passkey check did not pass.',
  passkey_unavailable: 'This account has no passkey it can use, so one cannot be required.',
  passkey_requirement_unsatisfiable: 'Some people have no passkey yet, so everyone cannot be required to use one now.',
  recent_login_required: 'Sign in again and retry: this step needs a sign-in from the last 10 minutes.',
  too_many_passkeys: 'The limit on passkeys is reached.',
  token_limit: 'You have too many tokens in use. Revoke one first.',
  token_expired: 'This token has expired.',
  token_invalid: 'This is not a token this server issued.',
  token_revoked: 'This authorisation was revoked.',
  token_owner_disabled: 'The account this authorisation belongs to is disabled.',
  token_scope: 'The scope of this token does not include this.',
  tokens_off: 'An administrator has switched API tokens off.',

  // Connections and repositories.
  provider_not_configured: 'The server has not set this platform up.',
  not_connected: 'You have no account connected on this platform. Connect one in the settings first.',
  provider_refused: (p) => `${p.provider ?? 'The platform'} refused the request. Check the permissions of the account.`,
  provider_error: (p) => `${p.provider ?? 'The platform'} ran into a problem. Try again later.`,
  provider_in_use: (p) =>
    `Accounts still sign in or connect through this platform${p.accounts ? ` (${p.accounts} accounts)` : ''}. Deal with them first.`,
  provider_limit: 'The limit on platforms of this kind is reached.',
  provider_host_mismatch:
    'People have linked accounts to this GitHub provider on its current host, so it cannot move to another GitHub host. Delete the provider, or remove its linked accounts, first.',
  subject_fixed:
    'Accounts are linked to this provider, so the path of the subject and the host of the user info address cannot change. Delete the provider, or remove its linked accounts, first.',
  name_taken: 'That name is already taken. Choose another.',
  push_failed: 'The content could not be pushed to the repository, so the space was not created.',
  repo_not_in_installation: 'The repository is not among those the installed app reaches. Add it on the platform.',
  app_not_installed: 'The app is not installed on that account yet. Install it first.',
  app_missing_permission: (p) =>
    `The app lacks a permission${p.permission ? ` ("${p.permission}")` : ''}. Update its permissions on the platform.`,
  app_rejected: 'The platform does not accept what was given for the app. Check it and try again.',
  client_rejected: 'The platform refused the authorisation.',
  key_invalid: 'That is not a valid private key.',
  connection_expired: 'Your connection to this platform has ended. Connect again.',
  connection_missing: 'This connection no longer exists.',
  installation_gone: 'The app has been uninstalled from that account.',
  installation_taken: 'That GitHub installation is already connected to another organisation or person on this server.',
  not_installer: 'Only the person who installed the App on the GitHub organisation can connect its installation.',

  // The daily text.
  bad_language: 'That is not a valid language tag.',
  api_not_found: 'There is no such interface.',
  builtin: 'This part of a built-in interface is fixed and cannot be changed.',
  header_reenter: (p) =>
    `Enter the value of the header${p.header ? ` ${p.header}` : ''} again: the address changed, so the old value is not sent.`,
  header_missing: (p) => `The header${p.header ? ` ${p.header}` : ''} has no value yet.`,
  not_json: 'The answer is not JSON.',
  too_large: 'The answer is too large.',
  path_error: (p) => `The path${p.field ? ` of "${p.field}"` : ''} is not right.`,
  path_reveals_secret: (p) => `The path${p.field ? ` of "${p.field}"` : ''} would show a secret, so it was refused.`,
  link_uses_header: 'A link cannot use an interface with headers.',
  skip_too_large: 'There is too much to skip.',
}
