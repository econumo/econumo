// The icon catalogue (Material Symbols ligature names) shared with the Go
// server lives in icons/icons.json. Check a new name renders in the bundled
// Material Symbols Rounded font before adding it (a missing ligature falls
// back to raw text).
import list from '../../../icons/icons.json'

export const icons: string[] = list

const unique = Array.from(new Set(icons))

// The Vue app capped the picker at 45 icons to keep its paginator small; the
// carousel picker shows the whole catalogue.
export const availableIcons = unique

export const defaultAccountIcon = availableIcons[0]

export const defaultCategoryIcon = availableIcons[0]

export const defaultEnvelopeIcon = availableIcons[0]
