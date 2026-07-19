import { Label, Switch } from 'web'

export const Default = () => (
  <div className="flex w-72 flex-col gap-3">
    <div className="flex items-center justify-between gap-2.5">
      <Label htmlFor="sw-archived" className="font-normal">
        Show archived accounts
      </Label>
      <Switch id="sw-archived" defaultChecked />
    </div>
    <div className="flex items-center justify-between gap-2.5">
      <Label htmlFor="sw-hidden" className="font-normal">
        Include hidden folders
      </Label>
      <Switch id="sw-hidden" />
    </div>
  </div>
)

export const AccountRows = () => (
  <ul className="flex w-72 flex-col">
    {[
      { id: 'main', name: 'Main account', included: true },
      { id: 'cash', name: 'Cash', included: true },
      { id: 'savings', name: 'Savings', included: false },
    ].map((account) => (
      <li key={account.id} className="flex items-center gap-2.5 py-2">
        <span className="min-w-0 flex-1 truncate text-sm">{account.name}</span>
        <Switch aria-label={`include ${account.name}`} defaultChecked={account.included} />
      </li>
    ))}
  </ul>
)

export const Sizes = () => (
  <div className="flex flex-col gap-3">
    <div className="flex items-center gap-2.5">
      <Switch id="sw-size-default" defaultChecked />
      <Label htmlFor="sw-size-default" className="font-normal">
        Default
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <Switch id="sw-size-sm" size="sm" defaultChecked />
      <Label htmlFor="sw-size-sm" className="font-normal">
        Small
      </Label>
    </div>
  </div>
)

export const Disabled = () => (
  <div className="flex flex-col gap-3">
    <div className="flex items-center gap-2.5">
      <Switch id="sw-dis-on" disabled defaultChecked />
      <Label htmlFor="sw-dis-on" className="font-normal text-muted-foreground">
        Disabled on
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <Switch id="sw-dis-off" disabled />
      <Label htmlFor="sw-dis-off" className="font-normal text-muted-foreground">
        Disabled off
      </Label>
    </div>
  </div>
)
