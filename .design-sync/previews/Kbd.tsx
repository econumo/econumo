import { Kbd, KbdGroup } from 'web'

export const SingleKeys = () => (
  <div className="flex items-center gap-3">
    <Kbd>⌘</Kbd>
    <Kbd>⇧</Kbd>
    <Kbd>Esc</Kbd>
    <Kbd>Enter</Kbd>
    <Kbd>Tab</Kbd>
  </div>
)

export const ShortcutCombos = () => (
  <div className="flex flex-col gap-2">
    <KbdGroup>
      <Kbd>⌘</Kbd>
      <Kbd>K</Kbd>
    </KbdGroup>
    <KbdGroup>
      <Kbd>Ctrl</Kbd>
      <span className="text-xs text-muted-foreground">+</span>
      <Kbd>N</Kbd>
    </KbdGroup>
    <KbdGroup>
      <Kbd>⌘</Kbd>
      <Kbd>⇧</Kbd>
      <Kbd>E</Kbd>
    </KbdGroup>
  </div>
)

export const InlineHints = () => (
  <div className="flex w-80 flex-col gap-2 text-sm">
    <div className="flex items-center justify-between rounded-md border px-3 py-2">
      <span>New transaction</span>
      <KbdGroup>
        <Kbd>⌘</Kbd>
        <Kbd>N</Kbd>
      </KbdGroup>
    </div>
    <div className="flex items-center justify-between rounded-md border px-3 py-2">
      <span>Search payees</span>
      <KbdGroup>
        <Kbd>⌘</Kbd>
        <Kbd>K</Kbd>
      </KbdGroup>
    </div>
    <p className="text-muted-foreground">
      Press <Kbd>Esc</Kbd> to close the budget editor without saving.
    </p>
  </div>
)
