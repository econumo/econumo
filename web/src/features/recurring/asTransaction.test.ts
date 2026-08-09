import { expect, it } from 'vitest'
import type { RecurringDto } from '@/api/dto/recurring'
import { recurringAsTransaction } from './asTransaction'

const template = {
  id: 'rt1',
  type: 'expense',
  accountId: 'a1',
  accountRecipientId: null,
  amount: '12.00',
  categoryId: null,
  payeeId: null,
  tagId: 'tg1',
  labelIds: ['lb2', 'lb-gone'],
  description: 'gym',
  nextPaymentAt: '2026-08-01 09:00:00',
} as unknown as RecurringDto

const lookups = {
  tags: [{ id: 'tg1', name: 'Italy 2026', icon: 'tag' }],
  labels: [
    { id: 'lb1', name: 'Kitty', icon: 'label' },
    { id: 'lb2', name: 'Doggo', icon: 'label' },
  ],
} as never

it('resolves a template\'s reporting tags, ignoring ids it cannot match', () => {
  const tx = recurringAsTransaction(template, lookups)
  expect(tx.labels?.map((l) => l.name)).toEqual(['Doggo'])
})

it('leaves reporting tags empty when the template carries none', () => {
  const tx = recurringAsTransaction({ ...template, labelIds: [] } as unknown as RecurringDto, lookups)
  expect(tx.labels).toEqual([])
})
