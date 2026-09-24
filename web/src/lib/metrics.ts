import { capture, capturePageView, setAnalyticsContext, setAnalyticsGroup } from './analytics'
import { analyticsAllowed } from './analyticsPreference'
import { authMethods } from './analyticsAuthMethods'
import { profileAttributes } from './analyticsProfile'
import { backendHost, getInstanceId, getVersion, locale } from './config'
import { isNativeApp } from './platform'
import { hasToken } from './storage'

// Collector event names derive from these via analyticsEventName(), so a
// renamed value is a new event in the dashboards, cut off from its history.
export const METRICS = {
  PAGE_VIEW: 'appPageView',
  USER_LOGIN: 'appUserLogin',
  USER_LOGOUT: 'appUserLogout',
  USER_REGISTRATION: 'appUserRegistration',
  USER_UPDATE_NAME: 'appUserUpdateName',
  USER_UPDATE_AVATAR: 'appUserUpdateAvatar',
  USER_UPDATE_PASSWORD: 'appUserUpdatePassword',
  USER_CHANGE_EMAIL: 'appUserChangeEmail',
  USER_UPDATE_CURRENCY: 'appUserUpdateCurrency',
  USER_UPDATE_ANALYTICS: 'appUserUpdateAnalytics',
  CURRENCY_CREATE: 'appCurrencyCreate',
  CURRENCY_UPDATE: 'appCurrencyUpdate',
  CURRENCY_DELETE: 'appCurrencyDelete',
  CURRENCY_DISABLE: 'appCurrencyDisable',
  CURRENCY_ENABLE: 'appCurrencyEnable',
  CURRENCY_DISABLE_ALL: 'appCurrencyDisableAll',
  CURRENCY_ENABLE_ALL: 'appCurrencyEnableAll',
  USER_COMPLETE_ONBOARDING: 'appUserCompleteOnboarding',
  USER_UPDATE_DEFAULT_BUDGET: 'appUserUpdateDefaultBudget',
  USER_UPDATE_LANGUAGE: 'appUserUpdateLanguage',
  USER_REMIND_PASSWORD: 'appUserRemindPassword',
  USER_RESET_PASSWORD: 'appUserResetPassword',
  EMAIL_VERIFICATION_COMPLETED: 'appEmailVerificationCompleted',
  EMAIL_VERIFICATION_RESENT: 'appEmailVerificationResent',
  OAUTH_LOGIN_COMPLETED: 'appOauthLoginCompleted',
  OAUTH_ACCOUNT_CREATED: 'appOauthAccountCreated',
  IDENTITY_LINKED: 'appIdentityLinked',
  IDENTITY_UNLINKED: 'appIdentityUnlinked',
  SESSION_REVOKE: 'appSessionRevoke',
  SESSION_REVOKE_OTHERS: 'appSessionRevokeOthers',
  PERSONAL_TOKEN_CREATE: 'appPersonalTokenCreate',
  PERSONAL_TOKEN_REVOKE: 'appPersonalTokenRevoke',
  ACCOUNT_CREATE: 'appAccountCreate',
  ACCOUNT_UPDATE: 'appAccountUpdate',
  ACCOUNT_DELETE: 'appAccountDelete',
  ACCOUNT_DECLINE_ACCESS: 'appAccountDeclineAccess',
  ACCOUNT_ORDER_LIST: 'appApiAccountOrderList',
  ACCOUNT_FOLDER_EXPAND: 'appAccountFolderExpand',
  ACCOUNT_FOLDER_COLLAPSE: 'appAccountFolderCollapse',
  ACCOUNT_FOLDER_CREATE: 'appAccountFolderCreate',
  ACCOUNT_FOLDER_UPDATE: 'appAccountFolderUpdate',
  ACCOUNT_FOLDER_REPLACE: 'appAccountFolderReplace',
  ACCOUNT_FOLDER_ORDER_LIST: 'appAccountFolderOrderList',
  ACCOUNT_FOLDER_HIDE: 'appAccountFolderHide',
  ACCOUNT_FOLDER_SHOW: 'appAccountFolderShow',
  CATEGORY_CREATE: 'appCategoryCreate',
  CATEGORY_UPDATE: 'appCategoryUpdate',
  CATEGORY_ORDER_LIST: 'appCategoryOrderList',
  CATEGORY_DELETE: 'appCategoryDelete',
  CATEGORY_ARCHIVE: 'appCategoryArchive',
  CATEGORY_UNARCHIVE: 'appCategoryUnarchive',
  PAYEE_CREATE: 'appPayeeCreate',
  PAYEE_UPDATE: 'appPayeeUpdate',
  PAYEE_ORDER_LIST: 'appPayeeOrderList',
  PAYEE_DELETE: 'appPayeeDelete',
  PAYEE_ARCHIVE: 'appPayeeArchive',
  PAYEE_UNARCHIVE: 'appPayeeUnarchive',
  CLASSIFICATION_SEARCH: 'appClassificationSearch',
  // one key for all four kinds, discriminated by the `type` property — same
  // shape as CLASSIFICATION_SEARCH above
  CLASSIFICATION_MERGE: 'appClassificationMerge',
  BUDGET_CREATE: 'appBudgetCreate',
  BUDGET_UPDATE: 'appBudgetUpdate',
  BUDGET_DELETE: 'appBudgetDelete',
  BUDGET_CLONE: 'appBudgetCloned',
  BUDGET_COMPLETE: 'appBudgetCompleted',
  BUDGET_ARCHIVE: 'appBudgetArchived',
  BUDGET_UNARCHIVE: 'appBudgetUnarchived',
  BUDGET_SET_END_DATE: 'appBudgetEndDateSet',
  BUDGET_GRANT_ACCESS: 'appBudgetGrantAccess',
  BUDGET_REVOKE_ACCESS: 'appBudgetRevokeAccess',
  BUDGET_ACCEPT_ACCESS: 'appBudgetAcceptAccess',
  BUDGET_DECLINE_ACCESS: 'appBudgetDeclineAccess',
  BUDGET_FOLDER_CREATE: 'appBudgetFolderCreate',
  BUDGET_FOLDER_DELETE: 'appBudgetFolderDelete',
  BUDGET_FOLDER_UPDATE: 'appBudgetFolderUpdate',
  BUDGET_FOLDER_CHANGE_ORDER: 'appBudgetFolderChangeOrder',
  BUDGET_CHANGE_DATE: 'appBudgetChangeDate',
  BUDGET_UPDATE_ELEMENT_LIMIT: 'appBudgetUpdateElementLimit',
  BUDGET_CHANGE_ORDER_ELEMENT: 'appBudgetChangeOrderElement',
  BUDGET_ELEMENT_CHANGE_CURRENCY: 'appBudgetElementChangeCurrency',
  BUDGET_ENVELOPE_DELETE: 'appBudgetEnvelopeDelete',
  BUDGET_ENVELOPE_UPDATE: 'appBudgetEnvelopeUpdate',
  BUDGET_ENVELOPE_CREATE: 'appBudgetEnvelopeCreate',
  BUDGET_PLAN_OPEN: 'appBudgetPlanOpen',
  BUDGET_PLAN_CHANGE_WINDOW: 'appBudgetPlanChangeWindow',
  BUDGET_PLAN_HIDE_EMPTY_TOGGLE: 'appBudgetPlanHideEmptyToggle',
  BUDGET_PLAN_FILL_RIGHT: 'appBudgetPlanFillRight',
  BUDGET_PLAN_PASTE_CELL: 'appBudgetPlanPasteCell',
  TAG_CREATE: 'appTagCreate',
  TAG_UPDATE: 'appTagUpdate',
  TAG_ORDER_LIST: 'appTagOrderList',
  TAG_DELETE: 'appTagDelete',
  TAG_ARCHIVE: 'appTagArchive',
  TAG_UNARCHIVE: 'appTagUnarchive',
  LABEL_CREATE: 'appLabelCreate',
  LABEL_UPDATE: 'appLabelUpdate',
  LABEL_ORDER_LIST: 'appLabelOrderList',
  LABEL_DELETE: 'appLabelDelete',
  LABEL_ARCHIVE: 'appLabelArchive',
  LABEL_UNARCHIVE: 'appLabelUnarchive',
  TRANSACTION_CREATE: 'appTransactionCreate',
  TRANSACTION_UPDATE: 'appTransactionUpdate',
  TRANSACTION_DELETE: 'appTransactionDelete',
  TRANSACTION_IMPORT: 'appTransactionImport',
  TRANSACTION_EXPORT: 'appTransactionExport',
  CONNECTION_GENERATE_INVITE: 'appConnectionGenerateInvite',
  CONNECTION_ACCEPT_INVITE: 'appConnectionAcceptInvite',
  CONNECTION_DELETE: 'appConnectionDelete',
  CONNECTION_UPDATE_ACCOUNT_ACCESS: 'appConnectionUpdateAccountAccess',
  CONNECTION_REVOKE_ACCOUNT_ACCESS: 'appConnectionRevokeAccountAccess',
  CONNECTION_ACCEPT_ACCOUNT_ACCESS: 'appConnectionAcceptAccountAccess',
  CONNECTION_DECLINE_ACCOUNT_ACCESS: 'appConnectionDeclineAccountAccess',
  SUBSCRIPTION_CTA_CLICK: 'appSubscriptionCtaClick',
  SUBSCRIPTION_BANNER_SHOW: 'appSubscriptionBannerShow',
} as const
export type Metric = (typeof METRICS)[keyof typeof METRICS]

const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi

// Route with UUID segments templated to ":id": no instance data may ride
// along on an analytics event.
export function scrubbedPage(pathname: string): string {
  return pathname.substring(1).replace(UUID_RE, ':id')
}

// Same cutoffs as the layout hooks: useIsMobile switches the shell below 768px
// and useIsCompact goes single-pane below 1024px, so the reported mode matches
// the layout the user actually saw.
export function viewMode(width: number = window.innerWidth): 'mobile' | 'tablet' | 'desktop' {
  if (width < 768) {
    return 'mobile'
  }
  if (width < 1024) {
    return 'tablet'
  }
  return 'desktop'
}

// Collector names: the app-prefixed camelCase becomes snake_case,
// e.g. appBudgetPlanFillRight -> budget_plan_fill_right.
export function analyticsEventName(metric: string): string {
  return metric
    .replace(/^app/, '')
    .replace(/([A-Z]+)(?=[A-Z][a-z])/g, '$1_')
    .replace(/([a-z0-9])(?=[A-Z])/g, '$1_')
    .toLowerCase()
}

// There is no persisted profile to hang a default attribute on (see
// lib/analytics.ts) — per-event accuracy comes from stamping the state at
// capture time from this module-level value, refreshed wherever user data
// lands (login, get-user-data).
let currentAccessState: string | null = null

export function setAnalyticsAccessState(state: string | null): void {
  currentAccessState = state
}

// In a Capacitor WebView window.location.hostname is always 'localhost', so
// the app must derive the effective host from the configured backend instead
// (mirrors the web path exactly when not running natively).
function currentHostname(): string {
  if (!isNativeApp()) {
    return window.location.hostname
  }
  try {
    return new URL(backendHost()).hostname
  } catch {
    return window.location.hostname
  }
}

export function isCloudHost(hostname: string = currentHostname()): boolean {
  return hostname === 'econumo.com' || hostname.endsWith('.econumo.com')
}

// The real hostname never appears in an event PAYLOAD — self-hosted deployments
// are identified by the server-derived instance digest instead. The browser's
// request itself still discloses the origin via the mandatory Origin header
// (and Referer, absent an explicit no-referrer policy); see analytics.ts.
export function analyticsHost(hostname: string = currentHostname()): string {
  if (isCloudHost(hostname)) {
    return hostname
  }
  return `selfhosted_${getInstanceId() || 'unknown'}`
}

export function deploymentKind(hostname: string = currentHostname()): 'cloud' | 'self-hosted' {
  return isCloudHost(hostname) ? 'cloud' : 'self-hosted'
}

export function analyticsPlatform(): 'web' | 'ios' | 'android' {
  if (!isNativeApp()) {
    return 'web'
  }
  return /android/i.test(navigator.userAgent) ? 'android' : 'ios'
}

export function trackEvent(metric: Metric, eventData: Record<string, unknown> = {}) {
  if (!metric) {
    return
  }
  if (!analyticsAllowed()) {
    return
  }
  // Nothing outside an authenticated session (login/register page views, the
  // pre-login auth events): the collector project is identified-only, and a
  // visitor who never signs in would otherwise show up as a one-day person
  // who can never return, dragging retention down for no product signal.
  if (!hasToken()) {
    return
  }
  // Resolved on every call rather than once at module load: in the mobile app
  // this module evaluates before the async fetchServerConfig() merges the
  // real INSTANCE_ID, so a fixed-at-import read would leave the group unset
  // for the whole session. The cost is two cheap string reads per event.
  const instanceId = getInstanceId()
  if (instanceId) {
    setAnalyticsGroup(instanceId, analyticsHost())
  }
  // Session-wide attributes: recomputed on every call rather than fixed at
  // module load, since the profile counts change as the query cache fills in
  // behind the boot loader.
  //
  // Everything here describes the SESSION, not the action — where the user is
  // signed in, how they signed in, what their data looks like. Only $path
  // describes the event itself. The system keys go on views and product
  // events alike. $host is the synthetic host, so a self-hosted deployment's
  // real hostname never appears; $device is the layout the user actually saw,
  // which wins over the form factor the SDK detects.
  setAnalyticsContext({
    $app_version: getVersion(),
    $platform: analyticsPlatform(),
    $host: analyticsHost(),
    $app_locale: locale(),
    $device: viewMode(),
    deployment: deploymentKind(),
    // Omitted while unknown, so neither reads as a measured "none".
    ...(currentAccessState ? { access_state: currentAccessState } : {}),
    ...(authMethods() ?? {}),
    ...profileAttributes(),
  })
  const pathname = window.location.pathname
  // Every UUID templated to :id: no instance data may ride along.
  const $path = `/${scrubbedPage(pathname)}`
  if (metric === METRICS.PAGE_VIEW) {
    // A real view, not a product event, so it lands in the views dashboards.
    // No $referrer: a self-hosted instance's own domain would be stored as a
    // referral source, since it never matches the synthetic $host.
    capturePageView(pathname, { $path, $referrer: null })
    return
  }
  capture(analyticsEventName(metric), { ...eventData, $path })
}
