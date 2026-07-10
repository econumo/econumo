import {
  Button,
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemSeparator,
  ItemTitle,
} from 'web'
import { ChevronRight, CreditCard, Landmark, PiggyBank, Wallet } from 'lucide-react'

export const AccountRow = () => (
  <Item variant="outline" className="w-96">
    <ItemMedia variant="icon">
      <Wallet />
    </ItemMedia>
    <ItemContent>
      <ItemTitle>Main account</ItemTitle>
      <ItemDescription>Personal · USD</ItemDescription>
    </ItemContent>
    <ItemActions>
      <span className="text-sm font-medium">$2,450.80</span>
      <ChevronRight className="size-4 text-muted-foreground" />
    </ItemActions>
  </Item>
)

export const VariantSweep = () => (
  <div className="flex w-96 flex-col gap-3">
    <Item variant="default">
      <ItemMedia variant="icon">
        <Wallet />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Cash</ItemTitle>
        <ItemDescription>Default variant</ItemDescription>
      </ItemContent>
      <ItemActions>
        <span className="text-sm">$180.00</span>
      </ItemActions>
    </Item>
    <Item variant="outline">
      <ItemMedia variant="icon">
        <Landmark />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Savings</ItemTitle>
        <ItemDescription>Outline variant</ItemDescription>
      </ItemContent>
      <ItemActions>
        <span className="text-sm">$12,300.00</span>
      </ItemActions>
    </Item>
    <Item variant="muted">
      <ItemMedia variant="icon">
        <CreditCard />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Credit card</ItemTitle>
        <ItemDescription>Muted variant</ItemDescription>
      </ItemContent>
      <ItemActions>
        <span className="text-sm text-expense">−$385.20</span>
      </ItemActions>
    </Item>
  </div>
)

export const AccountList = () => (
  <ItemGroup className="w-96 rounded-lg border p-2">
    <Item>
      <ItemMedia variant="icon">
        <Wallet />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Main account</ItemTitle>
        <ItemDescription>Personal · USD</ItemDescription>
      </ItemContent>
      <ItemActions>
        <span className="text-sm font-medium">$2,450.80</span>
      </ItemActions>
    </Item>
    <ItemSeparator />
    <Item>
      <ItemMedia variant="icon">
        <PiggyBank />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Savings</ItemTitle>
        <ItemDescription>Shared with Anna · EUR</ItemDescription>
      </ItemContent>
      <ItemActions>
        <span className="text-sm font-medium">€8,120.00</span>
      </ItemActions>
    </Item>
    <ItemSeparator />
    <Item>
      <ItemMedia variant="icon">
        <CreditCard />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Credit card</ItemTitle>
        <ItemDescription>Personal · USD</ItemDescription>
      </ItemContent>
      <ItemActions>
        <span className="text-sm font-medium text-expense">−$385.20</span>
      </ItemActions>
    </Item>
  </ItemGroup>
)

export const SizeSweep = () => (
  <div className="flex w-96 flex-col gap-3">
    <Item variant="outline" size="default">
      <ItemMedia variant="icon">
        <Wallet />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Groceries · default size</ItemTitle>
        <ItemDescription>Whole Foods Market</ItemDescription>
      </ItemContent>
      <ItemActions>
        <Button variant="ghost" size="sm">
          Edit
        </Button>
      </ItemActions>
    </Item>
    <Item variant="outline" size="sm">
      <ItemMedia variant="icon">
        <Wallet />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Transport · sm size</ItemTitle>
      </ItemContent>
      <ItemActions>
        <span className="text-sm text-expense">−$42.50</span>
      </ItemActions>
    </Item>
    <Item variant="outline" size="xs">
      <ItemMedia variant="icon">
        <Wallet />
      </ItemMedia>
      <ItemContent>
        <ItemTitle>Restaurants · xs size</ItemTitle>
      </ItemContent>
      <ItemActions>
        <span className="text-sm text-expense">−$18.90</span>
      </ItemActions>
    </Item>
  </div>
)
