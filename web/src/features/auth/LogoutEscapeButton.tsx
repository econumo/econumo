import { createPortal } from 'react-dom'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { RouterPage } from '@/app/router-pages'

// Floats over the loader it escapes from so its appearance never shifts
// layout.
//
// placement="screen" (default) portals to <body>, pinned to the viewport
// bottom — for modal loaders. It must stay OUTSIDE dialog content (the
// centered card is CSS-transformed, which would re-anchor position:fixed to
// the card) and outside the layout tree (an open modal aria-hides layout
// siblings and swallows their clicks via body pointer-events:none); mounting
// after the dialog's hide pass with own pointer-events keeps it usable.
//
// placement="container" anchors to the nearest positioned ancestor — for
// in-page loaders that are centered in a sub-area (e.g. the workspace next
// to the sidebar), so the caption stays on the spinner's own axis.
export function LogoutEscapeButton({ placement = 'screen' }: { placement?: 'screen' | 'container' }) {
  const { t } = useTranslation()
  const caption = (
    <span className="text-xs text-muted-foreground">
      {t('common.loader.having_trouble')}{' '}
      <Link to={RouterPage.LOGOUT} className="underline underline-offset-2 hover:text-foreground">
        {t('settings.page.logout')}
      </Link>
    </span>
  )
  if (placement === 'container') {
    return (
      <div className="absolute inset-x-0 bottom-[max(env(safe-area-inset-bottom),1rem)] flex justify-center animate-in fade-in duration-500">
        {caption}
      </div>
    )
  }
  return createPortal(
    <div className="pointer-events-auto fixed inset-x-0 bottom-[max(env(safe-area-inset-bottom),1rem)] z-[60] flex justify-center animate-in fade-in duration-500">
      {caption}
    </div>,
    document.body,
  )
}
