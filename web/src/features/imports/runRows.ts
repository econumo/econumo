import type { ImportRunLinkDto } from '@/api/dto/imports'

// A linked row whose transaction is gone is a tombstone: the import happened,
// the user deleted the result afterwards. It stays listed so the history is
// honest, and so a future "allow re-import" action has something to act on.
export function rowStatus(link: ImportRunLinkDto): 'imported' | 'deleted' | 'queued' | 'skipped' {
  if (link.status === 'linked') {
    return link.transactionId ? 'imported' : 'deleted'
  }
  return link.status === 'queued' ? 'queued' : 'skipped'
}
