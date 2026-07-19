import {
  Menubar,
  MenubarCheckboxItem,
  MenubarContent,
  MenubarItem,
  MenubarMenu,
  MenubarRadioGroup,
  MenubarRadioItem,
  MenubarSeparator,
  MenubarShortcut,
  MenubarTrigger,
} from 'web'

export const BudgetMenubar = () => (
  <Menubar className="w-fit">
    <MenubarMenu value="transactions">
      <MenubarTrigger>Transactions</MenubarTrigger>
      <MenubarContent>
        <MenubarItem>
          Add transaction
          <MenubarShortcut>⌘N</MenubarShortcut>
        </MenubarItem>
        <MenubarItem>Import CSV</MenubarItem>
        <MenubarItem>Export CSV</MenubarItem>
      </MenubarContent>
    </MenubarMenu>
    <MenubarMenu value="budget">
      <MenubarTrigger>Budget</MenubarTrigger>
      <MenubarContent>
        <MenubarItem>Set limit</MenubarItem>
        <MenubarItem>New envelope</MenubarItem>
      </MenubarContent>
    </MenubarMenu>
    <MenubarMenu value="reports">
      <MenubarTrigger>Reports</MenubarTrigger>
      <MenubarContent>
        <MenubarItem>Spending by category</MenubarItem>
        <MenubarItem>Income vs expenses</MenubarItem>
      </MenubarContent>
    </MenubarMenu>
  </Menubar>
)

export const MenubarOpenTransactions = () => (
  <div className="flex min-h-72 flex-col items-start">
    <Menubar value="transactions">
      <MenubarMenu value="transactions">
        <MenubarTrigger>Transactions</MenubarTrigger>
        <MenubarContent align="start">
          <MenubarItem>
            Add transaction
            <MenubarShortcut>⌘N</MenubarShortcut>
          </MenubarItem>
          <MenubarItem>Import CSV</MenubarItem>
          <MenubarItem disabled>Export CSV</MenubarItem>
          <MenubarSeparator />
          <MenubarCheckboxItem checked>Show archived</MenubarCheckboxItem>
          <MenubarSeparator />
          <MenubarRadioGroup value="month">
            <MenubarRadioItem value="month">Group by month</MenubarRadioItem>
            <MenubarRadioItem value="payee">Group by payee</MenubarRadioItem>
          </MenubarRadioGroup>
        </MenubarContent>
      </MenubarMenu>
      <MenubarMenu value="budget">
        <MenubarTrigger>Budget</MenubarTrigger>
        <MenubarContent>
          <MenubarItem>Set limit</MenubarItem>
        </MenubarContent>
      </MenubarMenu>
    </Menubar>
  </div>
)
