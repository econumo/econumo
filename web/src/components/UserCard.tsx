import type { ReactNode } from 'react'
import { UserAvatar } from '@/components/UserAvatar'

interface UserCardProps {
  user: { name: string; email: string; avatar: string }
  /** md = sidebar (48px avatar), lg = profile page (96px avatar) */
  size?: 'md' | 'lg'
  /** extra content under the email (e.g. a logout link) */
  children?: ReactNode
  /** when set, the avatar renders as a button (e.g. opens the avatar picker) */
  onAvatarClick?: () => void
  /** accessible label for the avatar button; only used when onAvatarClick is set */
  avatarLabel?: string
}

// The avatar + name + email block, styled like the Vue sidebar user card:
// rounded-square avatar, 18px name, 14px muted email.
export function UserCard({ user, size = 'md', children, onAvatarClick, avatarLabel }: UserCardProps) {
  const avatar = <UserAvatar avatar={user.avatar} size={size === 'lg' ? 'xl' : 'card'} />
  return (
    <span className="flex min-w-0 items-center gap-4">
      {onAvatarClick ? (
        <button
          type="button"
          aria-label={avatarLabel}
          onClick={onAvatarClick}
          className="shrink-0 rounded-3xl transition-opacity hover:opacity-80 focus-visible:ring-2 focus-visible:ring-ring"
        >
          {avatar}
        </button>
      ) : (
        avatar
      )}
      <span className="flex min-w-0 flex-col gap-1">
        <span className="truncate text-lg leading-5">{user.name}</span>
        <span className="truncate text-sm leading-4 text-muted-foreground">{user.email}</span>
        {children}
      </span>
    </span>
  )
}
