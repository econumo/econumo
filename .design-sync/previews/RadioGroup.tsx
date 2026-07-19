import { Label, RadioGroup, RadioGroupItem } from 'web'

export const Default = () => (
  <RadioGroup defaultValue="expense" className="w-64">
    <div className="flex items-center gap-2.5">
      <RadioGroupItem value="expense" id="tx-expense" />
      <Label htmlFor="tx-expense" className="font-normal">
        Expense
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <RadioGroupItem value="income" id="tx-income" />
      <Label htmlFor="tx-income" className="font-normal">
        Income
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <RadioGroupItem value="transfer" id="tx-transfer" />
      <Label htmlFor="tx-transfer" className="font-normal">
        Transfer
      </Label>
    </div>
  </RadioGroup>
)

export const Horizontal = () => (
  <RadioGroup defaultValue="monthly" orientation="horizontal" className="flex w-auto gap-6">
    <div className="flex items-center gap-2.5">
      <RadioGroupItem value="weekly" id="period-weekly" />
      <Label htmlFor="period-weekly" className="font-normal">
        Weekly
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <RadioGroupItem value="monthly" id="period-monthly" />
      <Label htmlFor="period-monthly" className="font-normal">
        Monthly
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <RadioGroupItem value="yearly" id="period-yearly" />
      <Label htmlFor="period-yearly" className="font-normal">
        Yearly
      </Label>
    </div>
  </RadioGroup>
)

export const Disabled = () => (
  <RadioGroup defaultValue="usd" className="w-64">
    <div className="flex items-center gap-2.5">
      <RadioGroupItem value="usd" id="cur-usd" />
      <Label htmlFor="cur-usd" className="font-normal">
        USD — US Dollar
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <RadioGroupItem value="eur" id="cur-eur" />
      <Label htmlFor="cur-eur" className="font-normal">
        EUR — Euro
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <RadioGroupItem value="gbp" id="cur-gbp" disabled />
      <Label htmlFor="cur-gbp" className="font-normal text-muted-foreground">
        GBP — not enabled
      </Label>
    </div>
  </RadioGroup>
)
