import type { OAuthProviderId } from '@/api/dto/oauth'
import { KeyRound } from 'lucide-react'

// Google's mark is multi-colour, so it carries its own fills; Apple's uses
// currentColor. Both follow the vendors' branding rules (App Review checks Apple's).
export function GoogleMark() {
  return (
    <svg viewBox="0 0 24 24" className="size-5" aria-hidden="true">
      <path fill="#4285F4" d="M23.49 12.27c0-.79-.07-1.54-.19-2.27H12v4.51h6.47a5.53 5.53 0 0 1-2.4 3.63v3h3.87c2.27-2.09 3.55-5.17 3.55-8.87z" />
      <path fill="#34A853" d="M12 24c3.24 0 5.95-1.08 7.94-2.91l-3.87-3c-1.08.72-2.45 1.16-4.07 1.16-3.13 0-5.78-2.11-6.73-4.96H1.29v3.09A12 12 0 0 0 12 24z" />
      <path fill="#FBBC05" d="M5.27 14.29A7.2 7.2 0 0 1 4.89 12c0-.8.14-1.57.38-2.29V6.62H1.29A12 12 0 0 0 0 12c0 1.94.46 3.77 1.29 5.38l3.98-3.09z" />
      <path fill="#EA4335" d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.42-3.42C17.95 1.19 15.24 0 12 0 7.31 0 3.26 2.69 1.29 6.62l3.98 3.09C6.22 6.86 8.87 4.75 12 4.75z" />
    </svg>
  )
}

export function AppleMark() {
  return (
    <svg viewBox="0 0 24 24" className="size-5" fill="currentColor" aria-hidden="true">
      <path d="M16.37 12.64c.03 3.02 2.65 4.02 2.68 4.03-.02.07-.42 1.43-1.38 2.84-.83 1.22-1.7 2.43-3.06 2.46-1.34.02-1.77-.8-3.3-.8-1.53 0-2.01.77-3.28.82-1.31.05-2.31-1.32-3.15-2.53-1.72-2.48-3.03-7.02-1.27-10.08.88-1.52 2.44-2.48 4.14-2.51 1.29-.02 2.51.87 3.3.87.79 0 2.27-1.08 3.83-.92.65.03 2.48.26 3.65 1.98-.09.06-2.18 1.27-2.16 3.84zM13.84 4.65c.7-.85 1.17-2.03 1.04-3.2-1.01.04-2.23.67-2.95 1.52-.65.75-1.22 1.95-1.06 3.1 1.12.09 2.27-.57 2.97-1.42z" />
    </svg>
  )
}

export function ProviderMark({ id }: { id: OAuthProviderId }) {
  if (id === 'google') return <GoogleMark />
  if (id === 'apple') return <AppleMark />
  return <KeyRound className="size-5" aria-hidden="true" />
}
