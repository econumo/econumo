import { analyticsDomain, capture } from './analytics'
import { analyticsEnabled, getVersion, locale, selfHosted } from './config'

declare global {
  interface Window {
    dataLayer: unknown[]
  }
}

// prefix "app" is required! These are the frozen dataLayer (GTM/liltag) names;
// PostHog event names derive from them via posthogEventName().
export const METRICS = {
  PAGE_VIEW: 'appPageView',
  USER_LOGIN: 'appUserLogin',
  USER_LOGOUT: 'appUserLogout',
  USER_REGISTRATION: 'appUserRegistration',
  USER_UPDATE_NAME: 'appUserUpdateName',
  USER_UPDATE_AVATAR: 'appUserUpdateAvatar',
  USER_UPDATE_PASSWORD: 'appUserUpdatePassword',
  USER_UPDATE_CURRENCY: 'appUserUpdateCurrency',
  USER_COMPLETE_ONBOARDING: 'appUserCompleteOnboarding',
  USER_UPDATE_DEFAULT_BUDGET: 'appUserUpdateDefaultBudget',
  USER_UPDATE_LANGUAGE: 'appUserUpdateLanguage',
  USER_REMIND_PASSWORD: 'appUserRemindPassword',
  USER_RESET_PASSWORD: 'appUserResetPassword',
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
  BUDGET_CREATE: 'appBudgetCreate',
  BUDGET_UPDATE: 'appBudgetUpdate',
  BUDGET_DELETE: 'appBudgetDelete',
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
  TAG_CREATE: 'appTagCreate',
  TAG_UPDATE: 'appTagUpdate',
  TAG_ORDER_LIST: 'appTagOrderList',
  TAG_DELETE: 'appTagDelete',
  TAG_ARCHIVE: 'appTagArchive',
  TAG_UNARCHIVE: 'appTagUnarchive',
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
  SUBSCRIPTION_PARTNER_CTA_CLICK: 'appSubscriptionPartnerCtaClick',
  SUBSCRIPTION_READONLY_BLOCKED: 'appSubscriptionReadonlyBlocked',
  SUBSCRIPTION_BANNER_SHOW: 'appSubscriptionBannerShow',
  UI_MODAL_ACCOUNT_OPEN: 'appUIModalAccountOpen',
  UI_MODAL_ACCOUNT_CLOSE: 'appUIModalAccountClose',
  UI_MODAL_TRANSACTION_OPEN: 'appUIModalTransactionOpen',
  UI_MODAL_TRANSACTION_CLOSE: 'appUIModalTransactionClose',
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

// PostHog names: the frozen dataLayer prefix+camelCase becomes snake_case,
// e.g. appUIModalTransactionOpen -> ui_modal_transaction_open.
export function posthogEventName(metric: string): string {
  return metric
    .replace(/^app/, '')
    .replace(/([A-Z]+)(?=[A-Z][a-z])/g, '$1_')
    .replace(/([a-z0-9])(?=[A-Z])/g, '$1_')
    .toLowerCase()
}

// There is no PostHog SDK and no person profile to hang a super property on
// (see lib/analytics.ts) — per-event accuracy comes from stamping the state
// at capture time from this module-level value, refreshed wherever user data
// lands (login, get-user-data).
let currentAccessState: string | null = null

export function setAnalyticsAccessState(state: string | null): void {
  currentAccessState = state
}

export function trackEvent(metric: Metric, eventData: Record<string, unknown> = {}) {
  if (!metric) {
    return
  }
  window.dataLayer = window.dataLayer || []
  window.dataLayer.push({
    event: metric,
    eventData,
    eventContext: {
      selfHosted: selfHosted(),
      locale: locale(),
      // the Vue app ran on hash routing; the path is the equivalent here
      page: window.location.pathname.substring(1),
    },
    eventTimestamp: Date.now(),
  })
  // Per-field/modal micro-interactions stay dataLayer-only: they dominate
  // event volume without informing any product decision.
  if (analyticsEnabled() && !metric.startsWith('appUIModal')) {
    const host = analyticsDomain()
    // The synthetic "self-hosted" host keeps real hostnames out of the URL;
    // only econumo.com domains appear verbatim.
    capture(posthogEventName(metric), {
      host,
      self_hosted: host === 'self-hosted',
      locale: locale(),
      version: getVersion(),
      mode: viewMode(),
      current_url: `https://${host}/${scrubbedPage(window.location.pathname)}`,
      ...(currentAccessState ? { access_state: currentAccessState } : {}),
    })
  }
}
