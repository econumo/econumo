// KNOWN GAP (see learnings/wave2-b4.md): the bundle's i18next instance is
// uninitialized and unreachable from preview code, so every label in this
// dialog (title + three buttons, all t() calls) renders as a raw key until
// the bundle gains an i18n bootstrap. Composition and layout are real.
import { SortDialog } from 'web'

// As composed in ClassificationList: opened from the list toolbar to reorder
// categories/tags/payees alphabetically. Props: open/onClose/onPick only.
export const SortOptions = () => <SortDialog open onClose={() => {}} onPick={() => {}} />
