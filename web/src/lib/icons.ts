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

// Neutral on purpose: a new item shows it until the user or an icon suggestion
// picks a real one, so it must not read as a wrong guess (the catalogue's
// first entry, a house, did).
export const defaultAccountIcon = 'credit_card'

export const defaultCategoryIcon = 'star'

export const defaultEnvelopeIcon = 'star'
