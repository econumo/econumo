import { analyticsDomain, capture } from './analytics'
import { analyticsEnabled, getVersion, locale, selfHosted } from './config'

declare global {
  interface Window {
    dataLayer: unknown[]
  }
}

// prefix "app" is required!
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
  USER_REMIND_PASSWORD: 'appUserRemindPassword',
  USER_RESET_PASSWORD: 'appUserResetPassword',
  ACCOUNT_SELECT: 'appAccountSelect',
  ACCOUNT_CREATE: 'appAccountCreate',
  ACCOUNT_UPDATE: 'appAccountUpdate',
  ACCOUNT_DELETE: 'appAccountDelete',
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
  CATEGORY_CHANGE_ORDER: 'appAccountChangeOrder',
  CATEGORY_DELETE: 'appCategoryDelete',
  CATEGORY_REPLACE: 'appCategoryReplace',
  CATEGORY_ARCHIVE: 'appCategoryArchive',
  CATEGORY_UNARCHIVE: 'appCategoryUnarchive',
  PAYEE_CREATE: 'appPayeeCreate',
  PAYEE_UPDATE: 'appPayeeUpdate',
  PAYEE_ORDER_LIST: 'appPayeeOrderList',
  PAYEE_CHANGE_ORDER: 'appPayeeChangeOrder',
  PAYEE_DELETE: 'appPayeeDelete',
  PAYEE_ARCHIVE: 'appPayeeArchive',
  PAYEE_UNARCHIVE: 'appPayeeUnarchive',
  BUDGET_CREATE: 'appBudgetCreate',
  BUDGET_UPDATE: 'appBudgetUpdate',
  BUDGET_ORDER_LIST: 'appBudgetOrderList',
  BUDGET_CHANGE_ORDER: 'appBudgetChangeOrder',
  BUDGET_DELETE: 'appBudgetDelete',
  BUDGET_RESET: 'appBudgetReset',
  BUDGET_GRANT_ACCESS: 'appBudgetGrantAccess',
  BUDGET_REVOKE_ACCESS: 'appBudgetRevokeAccess',
  BUDGET_ACCEPT_ACCESS: 'appBudgetAcceptAccess',
  BUDGET_DECLINE_ACCESS: 'appBudgetAcceptAccess',
  BUDGET_FOLDER_CREATE: 'appBudgetFolderCreate',
  BUDGET_FOLDER_DELETE: 'appBudgetFolderDelete',
  BUDGET_FOLDER_UPDATE: 'appBudgetFolderUpdate',
  BUDGET_FOLDER_CHANGE_ORDER: 'appBudgetFolderChangeOrder',
  BUDGET_CHANGE_DATE: 'appBudgetChangeDate',
  BUDGET_UPDATE_ELEMENT_LIMIT: 'appBudgetUpdateElementLimit',
  BUDGET_CHANGE_ORDER_ELEMENT: 'appBudgetChangeOrderElement',
  BUDGET_TRANSFER_ENVELOPE_BUDGET: 'appBudgetTransferEnvelopeBudget',
  BUDGET_ELEMENT_CHANGE_CURRENCY: 'appBudgetElementChangeCurrency',
  BUDGET_ENVELOPE_DELETE: 'appBudgetEnvelopeDelete',
  BUDGET_ENVELOPE_UPDATE: 'appBudgetEnvelopeUpdate',
  BUDGET_ENVELOPE_CREATE: 'appBudgetEnvelopeCreate',
  BUDGET_ENVELOPE_COPY_BUDGET: 'appBudgetEnvelopeCopyBudget',
  TAG_CREATE: 'appTagCreate',
  TAG_UPDATE: 'appTagUpdate',
  TAG_ORDER_LIST: 'appTagOrderList',
  TAG_CHANGE_ORDER: 'appTagChangeOrder',
  TAG_DELETE: 'appTagDelete',
  TAG_ARCHIVE: 'appTagArchive',
  TAG_UNARCHIVE: 'appTagUnarchive',
  TRANSACTION_CREATE: 'appTransactionCreate',
  TRANSACTION_UPDATE: 'appTransactionUpdate',
  TRANSACTION_DELETE: 'appTransactionDelete',
  CONNECTION_GENERATE_INVITE: 'appConnectionGenerateInvite',
  CONNECTION_DELETE_INVITE: 'appConnectionDeleteInvite',
  CONNECTION_ACCEPT_INVITE: 'appConnectionAcceptInvite',
  CONNECTION_DELETE: 'appConnectionDelete',
  CONNECTION_UPDATE_ACCOUNT_ACCESS: 'appConnectionUpdateAccountAccess',
  CONNECTION_REVOKE_ACCOUNT_ACCESS: 'appConnectionRevokeAccountAccess',
  CONNECTION_UPDATE_BUDGET_ACCESS: 'appConnectionUpdateBudgetAccess',
  CONNECTION_REVOKE_BUDGET_ACCESS: 'appConnectionRevokeBudgetAccess',
  UI_MODAL_ACCOUNT_OPEN: 'appUIModalAccountOpen',
  UI_MODAL_ACCOUNT_CLOSE: 'appUIModalAccountClose',
  UI_MODAL_ACCOUNT_CHANGE_NAME: 'appUIModalAccountChangeName',
  UI_MODAL_ACCOUNT_CHANGE_BALANCE: 'appUIModalAccountChangeBalance',
  UI_MODAL_ACCOUNT_CHANGE_CURRENCY: 'appUIModalAccountChangeCurrency',
  UI_MODAL_ACCOUNT_CHANGE_ICON: 'appUIModalAccountChangeIcon',
  UI_MODAL_TRANSACTION_OPEN: 'appUIModalTransactionOpen',
  UI_MODAL_TRANSACTION_CLOSE: 'appUIModalTransactionClose',
  UI_MODAL_TRANSACTION_CHANGE_TYPE: 'appUIModalTransactionChangeType',
  UI_MODAL_TRANSACTION_CHANGE_ACCOUNT: 'appUIModalTransactionChangeAccount',
  UI_MODAL_TRANSACTION_CHANGE_ACCOUNT_RECIPIENT: 'appUIModalTransactionChangeAccountRecipient',
  UI_MODAL_TRANSACTION_CHANGE_AMOUNT: 'appUIModalTransactionChangeAmount',
  UI_MODAL_TRANSACTION_CHANGE_AMOUNT_RECIPIENT: 'appUIModalTransactionChangeAmountRecipient',
  UI_MODAL_TRANSACTION_CHANGE_CATEGORY: 'appUIModalTransactionChangeCategory',
  UI_MODAL_TRANSACTION_CHANGE_DESCRIPTION: 'appUIModalTransactionChangeDescription',
  UI_MODAL_TRANSACTION_CHANGE_PAYEE: 'appUIModalTransactionChangePayee',
  UI_MODAL_TRANSACTION_CHANGE_TAG: 'appUIModalTransactionChangeTag',
  UI_MODAL_TRANSACTION_CHANGE_DATE: 'appUIModalTransactionChangeDate',
} as const
export type Metric = (typeof METRICS)[keyof typeof METRICS]

const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi

// Route with UUID segments templated to ":id": no instance data may ride
// along on an analytics event.
export function scrubbedPage(pathname: string): string {
  return pathname.substring(1).replace(UUID_RE, ':id')
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
  if (analyticsEnabled()) {
    const domain = analyticsDomain()
    capture(metric, {
      domain,
      selfHosted: domain === 'self-hosted',
      locale: locale(),
      version: getVersion(),
      page: scrubbedPage(window.location.pathname),
    })
  }
}
