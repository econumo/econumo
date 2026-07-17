import { useRef, useState } from 'react'
import { ChevronsUpDown, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Combobox as ComboboxPrimitive } from '@base-ui/react'
import { Combobox, ComboboxContent, ComboboxItem, ComboboxList } from '@/components/ui/combobox'
import { EntityIcon } from '@/components/EntityIcon'

export interface EntityOption {
  value: string
  label: string
  icon?: string
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
export function EntitySelect({
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
  const filtered = options.filter((o) => !search || o.label.toLowerCase().includes(search.toLowerCase()))
  const exactMatch = options.some((o) => o.label.toLowerCase() === search.toLowerCase())
  const canCreate = !!onCreate && search !== '' && !exactMatch && (createValidator ? createValidator(search) : true)

  const rows: Row[] = [
    // hidden while filtering so autoHighlight lands on a real match, not the clear row
    ...(clearable && value && !search ? [{ value: '__clear__', label: '—', clear: true }] : []),
    ...filtered,
    ...(canCreate ? [{ value: '__create__', label: search, create: true }] : []),
  ]

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
        className="flex h-8 w-full items-center gap-2 rounded-lg border border-input bg-transparent px-2.5 text-sm transition-colors focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50 has-disabled:opacity-50 dark:bg-input/30"
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
              className={row.create ? 'text-econumo-magenta' : undefined}
            >
              {row.create ? (
                <Plus className="size-4 text-econumo-magenta" />
              ) : row.icon ? (
                <EntityIcon name={row.icon} className="text-base text-muted-foreground" />
              ) : null}
              {row.create ? `${t('common.button.add.label')} «${row.label}»` : row.label}
            </ComboboxItem>
          )}
        </ComboboxList>
        {/* creation happens by typing a new name — say so instead of hiding it */}
        {onCreate && !canCreate ? (
          <div className="flex items-center gap-1.5 border-t px-3 py-2 text-xs text-muted-foreground">
            <Plus className="size-3.5" />
            {t('common.select.create_hint')}
          </div>
        ) : null}
      </ComboboxContent>
    </Combobox>
  )
}
