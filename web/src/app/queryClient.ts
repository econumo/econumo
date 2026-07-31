import { createAppQueryClient } from '@/lib/queryPersist'

// The app-wide singleton — importable outside the React tree (the 402
// interceptor invalidates the user query through it).
export const queryClient = createAppQueryClient()
