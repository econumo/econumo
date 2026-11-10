import { isAxiosError } from 'axios'

export type WriteOutcome = 'done' | 'refused' | 'retry' | 'wait' | 'pause'

export function classifyWriteError(err: unknown): WriteOutcome {
  // Not a request failure at all: retried up to the limit, so a bug cannot hold the queue for ever.
  if (!isAxiosError(err)) return 'retry'
  if (!err.response) return 'wait'
  const { status, data } = err.response
  if (status === 401) return 'pause'
  // A proxy's error page during a deploy says nothing about the write.
  if (typeof data?.success !== 'boolean') return 'wait'
  if (status === 409) return 'done'
  if (status === 429) return 'wait'
  if (status >= 500) return 'retry'
  return status >= 400 ? 'refused' : 'wait'
}
