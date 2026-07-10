// The bundle inlines sonner, so toast() must come from the SAME module —
// ds-extras re-exports it through 'web'. The real shipped <Toaster/> host
// receives these toasts directly.
import { useEffect } from 'react'
import { Toaster, toast } from 'web'

// transform creates a containing block so the fixed-position toast viewport
// stays inside the cell instead of escaping to the page corner.
const Frame = ({ children }: { children: React.ReactNode }) => (
  <div className="relative h-64 w-full overflow-hidden" style={{ transform: 'translateZ(0)' }}>
    {children}
  </div>
)

export const SuccessToast = () => {
  useEffect(() => {
    toast.success('Transaction added', {
      description: '−$42.50 Groceries · Main account',
      duration: Infinity,
    })
  }, [])
  return (
    <Frame>
      <Toaster position="bottom-right" />
    </Frame>
  )
}

export const ErrorAndInfo = () => {
  useEffect(() => {
    toast.info('Rates updated', { description: 'Currency rates as of today', duration: Infinity })
    toast.error('Import failed', {
      description: 'Row 14: unknown account "Wallet2"',
      duration: Infinity,
    })
  }, [])
  return (
    <Frame>
      <Toaster position="bottom-right" />
    </Frame>
  )
}
