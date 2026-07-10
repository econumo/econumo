import type { ReactNode } from 'react'
import { UserAvatar } from '@/components/UserAvatar'

interface UserCardProps {
  user: { name: string; email: string; avatar: string }
  /** md = sidebar (48px avatar), lg = profile page (96px avatar) */
  size?: 'md' | 'lg'
  /** extra content under the email (e.g. a logout link) */
  children?: ReactNode
}

// The avatar + name + email block, styled like the Vue sidebar user card:
// rounded-square avatar, 18px name, 14px muted email.
export function UserCard({ user, size = 'md', children }: UserCardProps) {
  return (
    <span className="flex min-w-0 items-center gap-4">
      <UserAvatar avatar={user.avatar} size={size === 'lg' ? 'xl' : 'card'} />
      <span className="flex min-w-0 flex-col gap-1">
        <span className="truncate text-lg leading-5">{user.name}</span>
        <span className="truncate text-sm leading-4 text-muted-foreground">{user.email}</span>
        {children}
      </span>
    </span>
  )
}
