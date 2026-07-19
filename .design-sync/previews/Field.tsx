import {
  Checkbox,
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSeparator,
  FieldSet,
  Input,
} from 'web'

export const AmountField = () => (
  <Field className="w-72">
    <FieldLabel htmlFor="tx-amount">Amount</FieldLabel>
    <Input id="tx-amount" defaultValue="$42.50" />
    <FieldDescription>Charged to Main account in USD.</FieldDescription>
  </Field>
)

export const FieldWithError = () => (
  <Field className="w-72" data-invalid="true">
    <FieldLabel htmlFor="category-name">Category name</FieldLabel>
    <Input id="category-name" defaultValue="Gr" aria-invalid />
    <FieldError>Category name must be 3-64 characters</FieldError>
  </Field>
)

export const HorizontalCheckboxField = () => (
  <Field orientation="horizontal" className="w-72">
    <Checkbox id="archived-accounts" defaultChecked />
    <FieldContent>
      <FieldLabel htmlFor="archived-accounts">Show archived accounts</FieldLabel>
      <FieldDescription>Savings and closed accounts stay visible in reports.</FieldDescription>
    </FieldContent>
  </Field>
)

export const TransferFieldGroup = () => (
  <FieldGroup className="w-80">
    <FieldSet>
      <FieldLegend>New transfer</FieldLegend>
      <Field>
        <FieldLabel htmlFor="from-account">From account</FieldLabel>
        <Input id="from-account" defaultValue="Main account" />
      </Field>
      <Field>
        <FieldLabel htmlFor="transfer-amount">Amount</FieldLabel>
        <Input id="transfer-amount" defaultValue="$385.20" />
      </Field>
    </FieldSet>
    <FieldSeparator>or</FieldSeparator>
    <Field>
      <FieldLabel htmlFor="to-account">To account</FieldLabel>
      <Input id="to-account" defaultValue="Savings" />
    </Field>
  </FieldGroup>
)
