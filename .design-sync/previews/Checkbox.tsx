import { Checkbox, Label } from 'web'

export const Default = () => (
  <div className="flex flex-col gap-3">
    <div className="flex items-center gap-2.5">
      <Checkbox id="col-date" defaultChecked />
      <Label htmlFor="col-date" className="font-normal">
        Date
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <Checkbox id="col-category" defaultChecked />
      <Label htmlFor="col-category" className="font-normal">
        Category
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <Checkbox id="col-payee" />
      <Label htmlFor="col-payee" className="font-normal">
        Payee
      </Label>
    </div>
  </div>
)

export const States = () => (
  <div className="flex flex-col gap-3">
    <div className="flex items-center gap-2.5">
      <Checkbox id="st-unchecked" />
      <Label htmlFor="st-unchecked" className="font-normal">
        Unchecked
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <Checkbox id="st-checked" defaultChecked />
      <Label htmlFor="st-checked" className="font-normal">
        Checked
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <Checkbox id="st-indeterminate" defaultChecked="indeterminate" />
      <Label htmlFor="st-indeterminate" className="font-normal">
        Indeterminate
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <Checkbox id="st-disabled" disabled />
      <Label htmlFor="st-disabled" className="font-normal text-muted-foreground">
        Disabled
      </Label>
    </div>
    <div className="flex items-center gap-2.5">
      <Checkbox id="st-disabled-checked" disabled defaultChecked />
      <Label htmlFor="st-disabled-checked" className="font-normal text-muted-foreground">
        Disabled checked
      </Label>
    </div>
  </div>
)

export const CategoryList = () => (
  <ul className="flex w-72 flex-col">
    {[
      { id: 'groceries', name: 'Groceries', checked: true },
      { id: 'restaurants', name: 'Restaurants', checked: true },
      { id: 'transport', name: 'Transport', checked: false },
    ].map((category) => (
      <li key={category.id}>
        <Label
          htmlFor={`env-cat-${category.id}`}
          className="flex items-center gap-2.5 rounded-md py-2 font-normal"
        >
          <span className="min-w-0 flex-1 truncate text-sm">{category.name}</span>
          <Checkbox
            id={`env-cat-${category.id}`}
            className="bg-background"
            defaultChecked={category.checked}
          />
        </Label>
      </li>
    ))}
  </ul>
)
