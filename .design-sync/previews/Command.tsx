import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from 'web'
import { ArrowLeftRight, Banknote, PieChart, PiggyBank, Plus, Wallet } from 'lucide-react'

export const CommandPalette = () => (
  <Command className="w-80 border shadow-md">
    <CommandInput placeholder="Search actions, accounts, categories…" />
    <CommandList>
      <CommandEmpty>No results found.</CommandEmpty>
      <CommandGroup heading="Actions">
        <CommandItem>
          <Plus />
          Add transaction
          <CommandShortcut>⌘N</CommandShortcut>
        </CommandItem>
        <CommandItem>
          <ArrowLeftRight />
          Transfer between accounts
        </CommandItem>
        <CommandItem>
          <PieChart />
          Go to budgets
          <CommandShortcut>⌘B</CommandShortcut>
        </CommandItem>
      </CommandGroup>
      <CommandSeparator />
      <CommandGroup heading="Accounts">
        <CommandItem>
          <Wallet />
          Main account
          <span className="ml-auto text-xs text-muted-foreground">$1,240.50</span>
        </CommandItem>
        <CommandItem>
          <Banknote />
          Cash
          <span className="ml-auto text-xs text-muted-foreground">$86.00</span>
        </CommandItem>
        <CommandItem>
          <PiggyBank />
          Savings
          <span className="ml-auto text-xs text-muted-foreground">$4,200.00</span>
        </CommandItem>
      </CommandGroup>
    </CommandList>
  </Command>
)

export const CommandNoResults = () => (
  <Command className="w-80 border shadow-md">
    <CommandInput value="Yacht mooring" />
    <CommandList>
      <CommandEmpty>No results found.</CommandEmpty>
      <CommandGroup heading="Categories">
        <CommandItem>Groceries</CommandItem>
        <CommandItem>Restaurants</CommandItem>
        <CommandItem>Transport</CommandItem>
      </CommandGroup>
    </CommandList>
  </Command>
)
