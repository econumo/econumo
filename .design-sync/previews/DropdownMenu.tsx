import {
  Button,
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from 'web'
import { Coins, Pencil, Trash2 } from 'lucide-react'

export const EnvelopeActionsMenu = () => (
  <div className="flex justify-center pt-2">
    <DropdownMenu open>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">Envelope actions</Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        <DropdownMenuLabel>Groceries envelope</DropdownMenuLabel>
        <DropdownMenuItem>
          <Pencil />
          Edit envelope
          <DropdownMenuShortcut>⌘E</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem>
          <Coins />
          Change currency
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive">
          <Trash2 />
          Delete envelope
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </div>
)

export const TransactionsViewMenu = () => (
  <div className="flex justify-center pt-2">
    <DropdownMenu open>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">View options</Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        <DropdownMenuLabel>Transactions view</DropdownMenuLabel>
        <DropdownMenuCheckboxItem checked>
          Show archived accounts
        </DropdownMenuCheckboxItem>
        <DropdownMenuCheckboxItem>Group by payee</DropdownMenuCheckboxItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel>Sort by</DropdownMenuLabel>
        <DropdownMenuRadioGroup value="date">
          <DropdownMenuRadioItem value="date">Date</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="amount">Amount</DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  </div>
)
