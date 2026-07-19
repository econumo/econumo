import { CardField, cardFieldControlClass, amountCardInputClass, Input } from 'web'

// Gray-card form field used by every create/edit dialog (AccountDialog,
// BudgetDialog, EnvelopeDialog, SetLimitDialog).

export const AccountName = () => (
  <div className="w-80">
    <CardField label="Name" htmlFor="cf-account-name">
      <Input id="cf-account-name" className={cardFieldControlClass} defaultValue="Main account" maxLength={64} />
    </CardField>
  </div>
)

export const WithError = () => (
  <div className="w-80">
    <CardField label="Name" htmlFor="cf-name-err" error="This value should not be blank.">
      <Input id="cf-name-err" className={cardFieldControlClass} defaultValue="" placeholder="e.g. Cash" />
    </CardField>
  </div>
)

export const AmountField = () => (
  <div className="w-80">
    <CardField label="Limit" htmlFor="cf-limit">
      <div className={amountCardInputClass}>
        <Input id="cf-limit" inputMode="decimal" defaultValue="1,250.00" />
      </div>
    </CardField>
  </div>
)

export const DialogForm = () => (
  <div className="flex w-80 flex-col gap-4">
    <CardField label="Name" htmlFor="cf-form-name">
      <Input id="cf-form-name" className={cardFieldControlClass} defaultValue="Savings" />
    </CardField>
    <CardField label="Balance" htmlFor="cf-form-balance">
      <Input id="cf-form-balance" className={cardFieldControlClass} inputMode="decimal" defaultValue="4,200.00" />
    </CardField>
  </div>
)
