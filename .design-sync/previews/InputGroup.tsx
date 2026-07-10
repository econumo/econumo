import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
  InputGroupText,
  InputGroupTextarea,
} from 'web'
import { Copy, Search, Wallet, X } from 'lucide-react'

export const WithIconAndText = () => (
  <div className="flex w-80 flex-col gap-3">
    <InputGroup>
      <InputGroupAddon>
        <Search />
      </InputGroupAddon>
      <InputGroupInput placeholder="Search transactions" />
    </InputGroup>
    <InputGroup>
      <InputGroupInput placeholder="0.00" aria-label="Budget limit" />
      <InputGroupAddon align="inline-end">
        <InputGroupText>USD</InputGroupText>
      </InputGroupAddon>
    </InputGroup>
  </div>
)

export const WithButtons = () => (
  <div className="flex w-80 flex-col gap-3">
    <InputGroup>
      <InputGroupInput readOnly defaultValue="ECON-4F2K-9QRX" aria-label="Invite code" />
      <InputGroupAddon align="inline-end">
        <InputGroupButton aria-label="Copy invite code" size="icon-xs">
          <Copy />
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
    <InputGroup>
      <InputGroupAddon>
        <Wallet />
      </InputGroupAddon>
      <InputGroupInput defaultValue="Savings" aria-label="Account name" />
      <InputGroupAddon align="inline-end">
        <InputGroupButton aria-label="Clear" size="icon-xs">
          <X />
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
  </div>
)

export const WithTextarea = () => (
  <div className="w-80">
    <InputGroup>
      <InputGroupTextarea placeholder="Comment, e.g. split dinner with Alex" rows={3} />
      <InputGroupAddon align="block-end" className="border-t">
        <InputGroupText>120 characters left</InputGroupText>
      </InputGroupAddon>
    </InputGroup>
  </div>
)

export const InvalidAndDisabled = () => (
  <div className="flex w-80 flex-col gap-3">
    <InputGroup>
      <InputGroupInput aria-invalid="true" defaultValue="-42" aria-label="Amount" />
      <InputGroupAddon align="inline-end">
        <InputGroupText>EUR</InputGroupText>
      </InputGroupAddon>
    </InputGroup>
    <InputGroup>
      <InputGroupAddon>
        <Search />
      </InputGroupAddon>
      <InputGroupInput disabled placeholder="Search payees" />
    </InputGroup>
  </div>
)
