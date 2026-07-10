import {
  Bubble,
  BubbleContent,
  Message,
  MessageAvatar,
  MessageContent,
  MessageHeader,
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from 'web'

const chat: Array<{ from: 'anna' | 'me'; text: string }> = [
  { from: 'anna', text: 'I accepted your invite to the Family budget.' },
  { from: 'me', text: 'Welcome! June limits are already set.' },
  { from: 'anna', text: 'Groceries is at 92% of the $600 limit.' },
  { from: 'me', text: 'I moved $50 from Restaurants to cover it.' },
  { from: 'anna', text: 'Added receipts: −$85.20 Groceries, −$42.50 Transport.' },
  { from: 'me', text: 'Imported the bank CSV — 214 transactions matched.' },
  { from: 'anna', text: 'Savings goal is at $3,150 of $5,000 now.' },
  { from: 'me', text: 'Nice. Vacation envelope set to $600 for July.' },
  { from: 'anna', text: 'Flights booked — flagged them as Vacation.' },
  { from: 'me', text: 'Perfect, budget still balances after that.' },
]

const ChatItem = ({ from, text }: { from: 'anna' | 'me'; text: string }) =>
  from === 'anna' ? (
    <Message>
      <MessageAvatar>
        <span className="flex size-8 items-center justify-center text-xs font-medium">
          AK
        </span>
      </MessageAvatar>
      <MessageContent>
        <MessageHeader>Anna</MessageHeader>
        <Bubble variant="muted">
          <BubbleContent>{text}</BubbleContent>
        </Bubble>
      </MessageContent>
    </Message>
  ) : (
    <Message align="end">
      <MessageContent>
        <Bubble align="end">
          <BubbleContent>{text}</BubbleContent>
        </Bubble>
      </MessageContent>
    </Message>
  )

export const SharedBudgetChat = () => (
  <MessageScrollerProvider>
    <MessageScroller className="h-72 w-96 rounded-xl border bg-background p-2">
      <MessageScrollerViewport>
        <MessageScrollerContent className="gap-3 p-2">
          {chat.map((m, i) => (
            <MessageScrollerItem key={i}>
              <ChatItem from={m.from} text={m.text} />
            </MessageScrollerItem>
          ))}
        </MessageScrollerContent>
      </MessageScrollerViewport>
      <MessageScrollerButton />
    </MessageScroller>
  </MessageScrollerProvider>
)

export const ScrolledToTop = () => (
  <MessageScrollerProvider defaultScrollPosition="start" autoScroll={false}>
    <MessageScroller className="h-72 w-96 rounded-xl border bg-background p-2">
      <MessageScrollerViewport>
        <MessageScrollerContent className="gap-3 p-2">
          {chat.map((m, i) => (
            <MessageScrollerItem key={i}>
              <ChatItem from={m.from} text={m.text} />
            </MessageScrollerItem>
          ))}
        </MessageScrollerContent>
      </MessageScrollerViewport>
      <MessageScrollerButton />
    </MessageScroller>
  </MessageScrollerProvider>
)
