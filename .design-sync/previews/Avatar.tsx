import {
  Avatar,
  AvatarBadge,
  AvatarFallback,
  AvatarGroup,
  AvatarGroupCount,
} from 'web'
import { Check } from 'lucide-react'

export const InitialsSizes = () => (
  <div className="flex items-end gap-4">
    <Avatar size="sm">
      <AvatarFallback>DK</AvatarFallback>
    </Avatar>
    <Avatar>
      <AvatarFallback>DK</AvatarFallback>
    </Avatar>
    <Avatar size="lg">
      <AvatarFallback>DK</AvatarFallback>
    </Avatar>
  </div>
)

export const WithBadge = () => (
  <div className="flex items-center gap-3">
    <Avatar size="lg">
      <AvatarFallback>MK</AvatarFallback>
      <AvatarBadge>
        <Check />
      </AvatarBadge>
    </Avatar>
    <div className="text-sm">
      <p className="font-medium">Maria K.</p>
      <p className="text-muted-foreground">Shared budget · admin</p>
    </div>
  </div>
)

export const SharedAccountGroup = () => (
  <div className="flex items-center gap-3">
    <AvatarGroup>
      <Avatar>
        <AvatarFallback>DK</AvatarFallback>
      </Avatar>
      <Avatar>
        <AvatarFallback>MK</AvatarFallback>
      </Avatar>
      <Avatar>
        <AvatarFallback>AP</AvatarFallback>
      </Avatar>
      <AvatarGroupCount>+2</AvatarGroupCount>
    </AvatarGroup>
    <span className="text-sm text-muted-foreground">
      5 people share Main account
    </span>
  </div>
)
