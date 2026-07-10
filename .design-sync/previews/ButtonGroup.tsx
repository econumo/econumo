import {
  Button,
  ButtonGroup,
  ButtonGroupSeparator,
  ButtonGroupText,
  Input,
} from 'web'
import { ChevronDown, ChevronLeft, ChevronRight, Minus, Plus, Wallet } from 'lucide-react'

export const Attached = () => (
  <div className="flex flex-wrap items-center gap-4">
    <ButtonGroup>
      <Button variant="outline" size="sm">
        <ChevronLeft />
        Previous
      </Button>
      <Button variant="outline" size="sm">
        Next
        <ChevronRight />
      </Button>
    </ButtonGroup>
    <ButtonGroup>
      <Button variant="outline" size="sm">Accounts</Button>
      <Button variant="outline" size="sm">Budgets</Button>
      <Button variant="outline" size="sm">Reports</Button>
    </ButtonGroup>
  </div>
)

export const SplitButton = () => (
  <ButtonGroup>
    <Button>Add transaction</Button>
    <ButtonGroupSeparator />
    <Button size="icon" aria-label="More options">
      <ChevronDown />
    </Button>
  </ButtonGroup>
)

export const WithInputAndText = () => (
  <div className="flex w-80 flex-col gap-4">
    <ButtonGroup>
      <ButtonGroupText>
        <Wallet />
        USD
      </ButtonGroupText>
      <Input placeholder="0.00" aria-label="Amount" />
      <Button variant="outline">Convert</Button>
    </ButtonGroup>
    <ButtonGroup>
      <Input placeholder="Invite code" aria-label="Invite code" />
      <Button variant="secondary">Apply</Button>
    </ButtonGroup>
  </div>
)

export const Vertical = () => (
  <ButtonGroup orientation="vertical" aria-label="Adjust limit">
    <Button variant="outline" size="icon" aria-label="Increase limit">
      <Plus />
    </Button>
    <Button variant="outline" size="icon" aria-label="Decrease limit">
      <Minus />
    </Button>
  </ButtonGroup>
)
