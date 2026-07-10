import {
  Button,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from 'web'
import { PiggyBank, ReceiptText, SearchX } from 'lucide-react'

export const NoTransactions = () => (
  <Empty className="w-96 border border-dashed">
    <EmptyHeader>
      <EmptyMedia variant="icon">
        <ReceiptText />
      </EmptyMedia>
      <EmptyTitle>No transactions yet</EmptyTitle>
      <EmptyDescription>
        Transactions you add to Main account will appear here.
      </EmptyDescription>
    </EmptyHeader>
    <EmptyContent>
      <Button size="sm">Add transaction</Button>
    </EmptyContent>
  </Empty>
)

export const NoSearchResults = () => (
  <Empty className="w-96">
    <EmptyHeader>
      <EmptyMedia variant="icon">
        <SearchX />
      </EmptyMedia>
      <EmptyTitle>No payees found</EmptyTitle>
      <EmptyDescription>
        No payees match “whole foods”. Try a different search or{' '}
        <a href="#">create a new payee</a>.
      </EmptyDescription>
    </EmptyHeader>
  </Empty>
)

export const NoBudget = () => (
  <Empty className="w-96 border border-dashed">
    <EmptyHeader>
      <EmptyMedia variant="default">
        <PiggyBank className="size-10 text-muted-foreground" />
      </EmptyMedia>
      <EmptyTitle>No budget for June</EmptyTitle>
      <EmptyDescription>
        Set limits for your envelopes to start tracking spending against a
        plan.
      </EmptyDescription>
    </EmptyHeader>
    <EmptyContent>
      <div className="flex gap-2">
        <Button size="sm">Create budget</Button>
        <Button size="sm" variant="outline">
          Copy from May
        </Button>
      </div>
    </EmptyContent>
  </Empty>
)
