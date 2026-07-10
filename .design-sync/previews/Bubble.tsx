import { Bubble, BubbleContent, BubbleGroup, BubbleReactions } from 'web'
import { Heart, ThumbsUp } from 'lucide-react'

export const Conversation = () => (
  <BubbleGroup className="w-80">
    <Bubble variant="muted">
      <BubbleContent>
        Hi! I just shared the Family budget with you.
      </BubbleContent>
    </Bubble>
    <Bubble variant="muted">
      <BubbleContent>
        Groceries is already at 92% of the limit for June.
      </BubbleContent>
    </Bubble>
    <Bubble align="end">
      <BubbleContent>
        Thanks! I&apos;ll move $50 from Restaurants to cover it.
      </BubbleContent>
    </Bubble>
    <Bubble variant="muted">
      <BubbleContent>Perfect, that should do it.</BubbleContent>
    </Bubble>
  </BubbleGroup>
)

export const Variants = () => (
  <BubbleGroup className="w-80">
    <Bubble variant="default">
      <BubbleContent>Default — Salary +$4,200.00 received</BubbleContent>
    </Bubble>
    <Bubble variant="secondary">
      <BubbleContent>Secondary — Savings goal reached</BubbleContent>
    </Bubble>
    <Bubble variant="muted">
      <BubbleContent>Muted — Rates updated for USD/EUR</BubbleContent>
    </Bubble>
    <Bubble variant="tinted">
      <BubbleContent>Tinted — Budget shared with Anna</BubbleContent>
    </Bubble>
    <Bubble variant="outline">
      <BubbleContent>Outline — CSV import finished</BubbleContent>
    </Bubble>
    <Bubble variant="destructive">
      <BubbleContent>Destructive — Groceries over limit by $38.40</BubbleContent>
    </Bubble>
    <Bubble variant="ghost">
      <BubbleContent>Ghost — plain text without a bubble surface</BubbleContent>
    </Bubble>
  </BubbleGroup>
)

export const WithReactions = () => (
  <BubbleGroup className="w-80 gap-4 pb-4">
    <Bubble variant="muted" className="pb-1">
      <BubbleContent>
        I set the Vacation envelope to $600 for July.
      </BubbleContent>
      <BubbleReactions>
        <ThumbsUp className="size-3.5 text-primary" />
        <span className="text-xs text-muted-foreground">2</span>
      </BubbleReactions>
    </Bubble>
    <Bubble align="end" className="pb-1">
      <BubbleContent>Great — flights are already booked!</BubbleContent>
      <BubbleReactions align="start">
        <Heart className="size-3.5 fill-expense text-expense" />
        <span className="text-xs text-muted-foreground">1</span>
      </BubbleReactions>
    </Bubble>
  </BubbleGroup>
)
