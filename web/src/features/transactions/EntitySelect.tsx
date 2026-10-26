import { useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { ChevronsUpDown, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Combobox as ComboboxPrimitive } from '@base-ui/react'
import { Combobox, ComboboxContent, ComboboxItem, ComboboxList } from '@/components/ui/combobox'
import { EntityIcon } from '@/components/EntityIcon'
import { rankByName } from '@/lib/search'
import { CommandItem } from '@/components/ui/command'
import { PickerSheet } from '@/components/PickerSheet'
import { useIsPhone } from '@/hooks/useIsPhone'

export interface EntityOption {
  value: string
  label: string
  icon?: string
  disabled?: boolean
  /** what typing matches against when the label carries decoration (e.g. an account's balance) */
  searchText?: string
}

interface Row extends EntityOption {
  create?: boolean
  clear?: boolean
}

interface EntitySelectProps {
  value: string | null
  onChange: (value: string | null) => void
  options: EntityOption[]
  id?: string
  'aria-label'?: string
  /** the phone picker sheet's heading; the aria-label when absent */
  title?: string
  placeholder?: string
  clearable?: boolean
  disabled?: boolean
  /** offered when the typed text matches no option and passes validation */
  onCreate?: (name: string) => void
  createValidator?: (name: string) => boolean
}

// Filter-as-you-type entity picker (category/payee) with optional create-on-the-fly,
// mirroring the Vue q-select use-input + @new-value behavior. The field is a real
// <input> (not a button-triggered popover): macOS Safari's plain Tab only reaches
// text fields, so a button trigger would be unreachable by keyboard there.
function buildRows({ options, value, search, clearable, onCreate, createValidator }: Pick<EntitySelectProps, 'options' | 'value' | 'clearable' | 'onCreate' | 'createValidator'> & { search: string }) {
  const filtered = rankByName(options, (o) => o.searchText ?? o.label, search)
  // the created name is trimmed, so it is also what must be new and valid
  const name = search.trim()
  const exactMatch = options.some((o) => o.label.toLowerCase() === name.toLowerCase())
  const canCreate = !!onCreate && name !== '' && !exactMatch && (createValidator ? createValidator(name) : true)
  const rows: Row[] = [
    // hidden while filtering so the first row is a real match, not the clear row
    ...(clearable && value && !search ? [{ value: '__clear__', label: '—', clear: true }] : []),
    ...filtered,
    ...(canCreate ? [{ value: '__create__', label: name, create: true }] : []),
  ]
  return { rows, canCreate }
}

function RowContent({ row }: { row: Row }) {
  const { t } = useTranslation()
  return (
    <>
      {row.create ? (
        <Plus className="size-4 text-econumo-link" />
      ) : row.icon ? (
        <EntityIcon name={row.icon} className="text-base text-muted-foreground" />
      ) : null}
      {row.create ? `${t('common.button.add.label')} «${row.label}»` : row.label}
    </>
  )
}

function CreateHint() {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-1.5 border-t px-3 py-2 text-xs text-muted-foreground">
      <Plus className="size-3.5" />
      {t('common.select.create_hint')}
    </div>
  )
}

// px a touch may travel and still count as a tap on the phone picker
const TAP_SLOP = 10

const FORM_FIELDS = 'input:not([type=hidden]):not(:disabled), textarea:not(:disabled), select:not(:disabled)'

function fieldBeside(from: HTMLElement, step: 1 | -1): HTMLElement | null {
  const scope = from.closest('[data-slot="dialog-content"], form') ?? document.body
  const fields = Array.from(scope.querySelectorAll<HTMLElement>(FORM_FIELDS))
  const at = fields.indexOf(from)
  return at < 0 ? null : (fields[at + step] ?? null)
}

const fieldClass =
  'flex h-8 w-full items-center gap-2 rounded-lg border border-input bg-transparent px-2.5 text-sm transition-colors focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50 has-disabled:opacity-50 max-md:h-11 max-md:text-base'

export function EntitySelect(props: EntitySelectProps) {
  // a text field inside a phone form makes iOS scroll the form to centre it
  // on every focus, and back for the next one; the phone picker keeps focus
  // out of the form and searches in a sheet of its own instead
  return useIsPhone() ? <PhoneEntitySelect {...props} /> : <InlineEntitySelect {...props} />
}

function PhoneEntitySelect({
  value,
  onChange,
  options,
  id,
  'aria-label': ariaLabel,
  title,
  placeholder,
  clearable,
  disabled,
  onCreate,
  createValidator,
}: EntitySelectProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const triggerRef = useRef<HTMLInputElement>(null)
  const touchStart = useRef<{ x: number; y: number } | null>(null)
  const selected = options.find((o) => o.value === value) ?? null
  const rowsFor = (search: string) => buildRows({ options, value, search, clearable, onCreate, createValidator })

  const close = () => setOpen(false)
  const pick = (row: Row | undefined) => {
    if (!row || row.disabled) {
      return
    }
    if (row.create) {
      onCreate?.(row.label)
    } else {
      onChange(row.clear ? null : row.value)
    }
    close()
  }
  // iOS's ^ / v keys inside the sheet carry on through the form: the sheet
  // closes and the field before / after the picker takes focus (a picker there
  // opens its own sheet)
  const moveTo = (step: 1 | -1) => {
    const trigger = triggerRef.current
    const target = trigger ? fieldBeside(trigger, step) : null
    flushSync(close)
    if (target) {
      target.focus()
      // a programmatic focus gets no reveal from iOS: Notes would stay behind
      // the footer until typing scrolled it up
      requestAnimationFrame(() => target.scrollIntoView({ block: 'nearest' }))
    } else if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur()
    }
  }

  return (
    <>
      {/* a text field that never raises a keyboard, so iOS's ^ / v keys stop
          on it; a tap opens the sheet without focusing it (the focus would make
          iOS scroll the form to it), arriving by those keys opens it on focus */}
      <div data-slot="entity-select" className={`${fieldClass} has-disabled:cursor-not-allowed`} onClick={() => !disabled && setOpen(true)}>
        {selected?.icon ? <EntityIcon name={selected.icon} className="shrink-0 text-base text-muted-foreground" /> : null}
        <input
          ref={triggerRef}
          id={id}
          role="combobox"
          aria-label={ariaLabel}
          aria-expanded={open}
          aria-haspopup="dialog"
          inputMode="none"
          autoComplete="off"
          value={selected?.label ?? ''}
          placeholder={placeholder ?? ''}
          onChange={() => {}}
          disabled={disabled}
          onFocus={() => setOpen(true)}
          onTouchStart={(e) => {
            touchStart.current = { x: e.touches[0].clientX, y: e.touches[0].clientY }
          }}
          onTouchMove={(e) => {
            // a touch that ever travels past the slop is a scroll, even if it
            // comes back to where it began
            const start = touchStart.current
            const at = e.touches[0]
            if (start && (Math.abs(at.clientX - start.x) > TAP_SLOP || Math.abs(at.clientY - start.y) > TAP_SLOP)) {
              touchStart.current = null
            }
          }}
          onTouchCancel={() => {
            touchStart.current = null
          }}
          onTouchEnd={(e) => {
            // a scroll of the form that began on the picker is no tap: iOS
            // neither clicks nor focuses after it, so leave it alone
            const start = touchStart.current
            touchStart.current = null
            const end = e.changedTouches[0]
            if (!start || Math.abs(end.clientX - start.x) > TAP_SLOP || Math.abs(end.clientY - start.y) > TAP_SLOP) {
              return
            }
            e.preventDefault()
            if (!disabled) {
              setOpen(true)
            }
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              setOpen(true)
            }
          }}
          className="h-full w-full min-w-0 flex-1 truncate bg-transparent caret-transparent outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed"
        />
        <ChevronsUpDown className="pointer-events-none size-4 shrink-0 opacity-50" />
      </div>
      <PickerSheet
        open={open}
        onClose={close}
        title={title ?? ariaLabel ?? ''}
        searchLabel={ariaLabel ?? ''}
        searchPlaceholder={onCreate ? t('common.select.create_placeholder') : t('common.select.search_placeholder')}
        // a typed name means its best match, never the clear row
        onSearchEnter={(search) => pick(rowsFor(search).rows.find((r) => !r.clear && !r.disabled))}
        onNavigate={moveTo}
        // focus never returns to the picker: arriving there would reopen it
        onCloseAutoFocus={(e) => e.preventDefault()}
        listLabel={ariaLabel}
        after={(search) => (onCreate && !rowsFor(search).canCreate ? <CreateHint /> : null)}
      >
        {(search) =>
          rowsFor(search).rows.map((row) => (
            <CommandItem
              key={row.create ? '__create__' : row.value}
              value={row.create ? '__create__' : row.value}
              disabled={row.disabled}
              data-checked={!row.clear && !row.create && row.value === value}
              className={row.create ? 'text-econumo-link' : undefined}
              onSelect={() => pick(row)}
            >
              <RowContent row={row} />
            </CommandItem>
          ))
        }
      </PickerSheet>
    </>
  )
}

function InlineEntitySelect({
  value,
  onChange,
  options,
  id,
  'aria-label': ariaLabel,
  placeholder,
  clearable,
  disabled,
  onCreate,
  createValidator,
}: EntitySelectProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const rootRef = useRef<HTMLDivElement | null>(null)
  // portal the popup INTO the containing dialog/drawer: the dialog's scroll lock
  // swallows wheel events on anything portaled to body (and vaul's touch lock
  // does the same to touchmove), so the list would not scroll otherwise
  const portalContainer =
    (rootRef.current?.closest('[data-slot="drawer-content"], [data-slot="dialog-content"]') as HTMLElement | null) ?? undefined

  const selected = options.find((o) => o.value === value) ?? null
  const { rows, canCreate } = buildRows({ options, value, search, clearable, onCreate, createValidator })

  return (
    <Combobox
      items={rows}
      filter={null}
      value={selected}
      onValueChange={(row: Row | null) => {
        if (!row) {
          return
        }
        if (row.create) {
          onCreate?.(row.label)
          return
        }
        onChange(row.clear ? null : row.value)
      }}
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        // closing without a pick abandons the search; the field falls back to
        // displaying the selected label
        if (!next) {
          setSearch('')
        }
      }}
      inputValue={open ? search : (selected?.label ?? '')}
      onInputValueChange={setSearch}
      itemToStringLabel={(row: Row | null) => row?.label ?? ''}
      isItemEqualToValue={(a: Row | null, b: Row | null) => a?.value === b?.value}
      autoHighlight
      openOnInputClick
      disabled={disabled}
    >
      <div
        ref={rootRef}
        data-slot="entity-select"
        className={fieldClass}
        onClick={(e) => {
          // clicks on the icon/chevron/padding (and clicks forwarded by
          // SelectCard) open the picker, not just clicks on the input itself
          if (!disabled && !open) {
            e.currentTarget.querySelector('input')?.focus()
            setOpen(true)
          }
        }}
      >
        {selected?.icon && !open ? <EntityIcon name={selected.icon} className="shrink-0 text-base text-muted-foreground" /> : null}
        <ComboboxPrimitive.Input
          id={id}
          aria-label={ariaLabel}
          placeholder={
            open ? (onCreate ? t('common.select.create_placeholder') : t('common.select.search_placeholder')) : (placeholder ?? '')
          }
          className="h-full w-full min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !open) {
              // the picker owns Enter: open the list instead of submitting the form
              e.preventDefault()
              setOpen(true)
            }
            // the hosting dialog leaves Escape to the picker while it is open
            // (ResponsiveDialog's onEscapeKeyDown); close it here since the
            // open state is controlled
            if (e.key === 'Escape' && open) {
              setOpen(false)
              setSearch('')
            }
          }}
        />
        <ChevronsUpDown className="pointer-events-none size-4 shrink-0 opacity-50" />
      </div>
      <ComboboxContent container={portalContainer}>
        <ComboboxList>
          {(row: Row) => (
            <ComboboxItem
              key={row.create ? '__create__' : row.value}
              value={row}
              disabled={row.disabled}
              className={row.create ? 'text-econumo-link' : undefined}
            >
              <RowContent row={row} />
            </ComboboxItem>
          )}
        </ComboboxList>
        {/* creation happens by typing a new name — say so instead of hiding it */}
        {onCreate && !canCreate ? <CreateHint /> : null}
      </ComboboxContent>
    </Combobox>
  )
}
