import type { ComponentProps } from 'react'
import { Input } from '@/components/ui/input'
import { SearchClearButton } from '@/components/SearchClearButton'
import { cn } from '@/lib/utils'

type SearchInputProps = Omit<ComponentProps<typeof Input>, 'value'> & {
  value: string
  onClear: () => void
  /** classes for the wrapper, which takes the field's place in the layout */
  wrapperClassName?: string
}

export function SearchInput({ value, onClear, wrapperClassName, className, ...props }: SearchInputProps) {
  return (
    <div className={cn('relative', wrapperClassName)}>
      <Input value={value} className={cn(value !== '' && 'pr-9', className)} {...props} />
      {value !== '' ? (
        <SearchClearButton
          className="absolute top-1/2 right-0.5 -translate-y-1/2"
          onClear={onClear}
        />
      ) : null}
    </div>
  )
}
