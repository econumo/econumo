const NO_KEYBOARD_INPUTS = new Set(['button', 'checkbox', 'radio', 'range', 'color', 'file', 'submit', 'reset', 'image', 'hidden'])

/** whether focusing the element raises the on-screen keyboard */
export function ownsKeyboard(target: EventTarget | null): boolean {
  if (target instanceof HTMLTextAreaElement) {
    return target.inputMode !== 'none'
  }
  if (target instanceof HTMLInputElement) {
    return target.inputMode !== 'none' && !NO_KEYBOARD_INPUTS.has(target.type)
  }
  return target instanceof HTMLElement && target.isContentEditable
}
