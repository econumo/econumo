export const COMMENT_ANCHOR_ATTR = 'data-comment-anchor'

// A thread popover anchors to the whole cell, not to the control that opened it
// (a 20px marker or an amount button), so it lines up the same way from every
// entry point.
export function commentAnchorOf(el: Element): HTMLElement {
  return (el.closest(`[${COMMENT_ANCHOR_ATTR}]`) as HTMLElement | null) ?? (el as HTMLElement)
}

export function openLimitEditorIn(anchor: HTMLElement): void {
  anchor.querySelector<HTMLButtonElement>('[data-limit-trigger]')?.click()
}
