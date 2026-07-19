import {
  Bubble,
  BubbleContent,
  Message,
  MessageAvatar,
  MessageContent,
  MessageFooter,
  MessageGroup,
  MessageHeader,
  Attachment,
  AttachmentContent,
  AttachmentDescription,
  AttachmentMedia,
  AttachmentTitle,
} from 'web'
import { FileSpreadsheet } from 'lucide-react'

const Initials = ({ children }: { children: string }) => (
  <span className="flex size-8 items-center justify-center text-xs font-medium">
    {children}
  </span>
)

export const Conversation = () => (
  <MessageGroup className="w-96">
    <Message>
      <MessageAvatar>
        <Initials>AK</Initials>
      </MessageAvatar>
      <MessageContent>
        <MessageHeader>Anna</MessageHeader>
        <Bubble variant="muted">
          <BubbleContent>
            I accepted your invite — I can see the Family budget now.
          </BubbleContent>
        </Bubble>
      </MessageContent>
    </Message>
    <Message align="end">
      <MessageContent>
        <Bubble align="end">
          <BubbleContent>
            Great! You have editor access to Groceries and Transport.
          </BubbleContent>
        </Bubble>
      </MessageContent>
    </Message>
    <Message>
      <MessageAvatar>
        <Initials>AK</Initials>
      </MessageAvatar>
      <MessageContent>
        <Bubble variant="muted">
          <BubbleContent>Perfect, adding this week&apos;s receipts.</BubbleContent>
        </Bubble>
      </MessageContent>
    </Message>
  </MessageGroup>
)

export const WithFooter = () => (
  <MessageGroup className="w-96">
    <Message>
      <MessageAvatar>
        <Initials>DM</Initials>
      </MessageAvatar>
      <MessageContent>
        <MessageHeader>Dmitry</MessageHeader>
        <Bubble variant="muted">
          <BubbleContent>
            Salary +$4,200.00 landed on Main account.
          </BubbleContent>
        </Bubble>
        <MessageFooter>Today, 09:12</MessageFooter>
      </MessageContent>
    </Message>
    <Message align="end">
      <MessageContent>
        <Bubble align="end">
          <BubbleContent>Moving $800.00 to Savings then.</BubbleContent>
        </Bubble>
        <MessageFooter>Today, 09:15 · Read</MessageFooter>
      </MessageContent>
    </Message>
  </MessageGroup>
)

export const WithAttachment = () => (
  <MessageGroup className="w-96">
    <Message>
      <MessageAvatar>
        <Initials>AK</Initials>
      </MessageAvatar>
      <MessageContent>
        <MessageHeader>Anna</MessageHeader>
        <Bubble variant="muted">
          <BubbleContent>Here is the June export from the bank.</BubbleContent>
        </Bubble>
        <Attachment>
          <AttachmentMedia>
            <FileSpreadsheet />
          </AttachmentMedia>
          <AttachmentContent>
            <AttachmentTitle>transactions-2026-06.csv</AttachmentTitle>
            <AttachmentDescription>512 KB · 214 transactions</AttachmentDescription>
          </AttachmentContent>
        </Attachment>
        <MessageFooter>Yesterday, 18:47</MessageFooter>
      </MessageContent>
    </Message>
  </MessageGroup>
)
