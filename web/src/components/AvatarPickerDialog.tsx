import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { IconPicker } from '@/components/IconPicker'
import { UserAvatar } from '@/components/UserAvatar'
import { avatarColors, avatarColorSwatches, avatarIcons, joinAvatar, splitAvatar } from '@/lib/avatars'
import { cn } from '@/lib/utils'
import { useUpdateAvatar, useUserData } from '@/features/user/queries'

interface AvatarPickerDialogProps {
  open: boolean
  onClose: () => void
}

// Icon + color picker for the user's avatar, seeded from the current value.
export function AvatarPickerDialog({ open, onClose }: AvatarPickerDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const updateAvatar = useUpdateAvatar()
  const [icon, setIcon] = useState(() => splitAvatar(user?.avatar ?? '').icon)
  const [color, setColor] = useState(() => splitAvatar(user?.avatar ?? '').color)

  // Seed from the saved avatar once per open (first render with user data
  // present), NOT on every user-cache rewrite — a background refetch while the
  // dialog is open must not discard the in-progress selection.
  const seeded = useRef(false)
  useEffect(() => {
    if (!open) {
      seeded.current = false
      return
    }
    if (user && !seeded.current) {
      seeded.current = true
      const v = splitAvatar(user.avatar)
      setIcon(v.icon)
      setColor(v.color)
    }
  }, [open, user])

  const save = () => {
    updateAvatar.mutate(
      { icon, color },
      {
        onSuccess: () => onClose(),
      },
    )
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('modals.avatar_picker.title')}>
      <div className="flex flex-col gap-4">
        <div className="flex justify-center">
          <UserAvatar avatar={joinAvatar(icon, color)} size="xl" />
        </div>
        <div role="radiogroup" aria-label={t('modals.avatar_picker.colors')} className="flex flex-wrap justify-center gap-2">
          {avatarColors.map((c) => (
            <button
              key={c}
              type="button"
              role="radio"
              aria-checked={c === color}
              aria-label={c}
              title={c}
              className={cn(
                'size-7 rounded-full',
                avatarColorSwatches[c],
                c === color ? 'ring-2 ring-ring ring-offset-2 ring-offset-background' : '',
              )}
              onClick={() => setColor(c)}
            />
          ))}
        </div>
        <IconPicker value={icon} onChange={setIcon} icons={avatarIcons} aria-label={t('modals.avatar_picker.icons')} />
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="button" onClick={save} disabled={updateAvatar.isPending}>
            {t('elements.button.save.label')}
          </Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
