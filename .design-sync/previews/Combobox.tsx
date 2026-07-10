import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxChipsInput,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxValue,
  useComboboxAnchor,
} from 'web'

const categories = ['Groceries', 'Restaurants', 'Salary', 'Transport']
const payees = ['REWE Supermarket', 'Shell Station', 'Amazon', 'Landlord']

export const CategoryCombobox = () => (
  <Combobox items={categories} defaultValue="Groceries">
    <ComboboxInput placeholder="Select category" className="w-56" />
    <ComboboxContent>
      <ComboboxEmpty>No categories found.</ComboboxEmpty>
      <ComboboxList>
        {(item: string) => (
          <ComboboxItem key={item} value={item}>
            {item}
          </ComboboxItem>
        )}
      </ComboboxList>
    </ComboboxContent>
  </Combobox>
)

export const PayeeComboboxOpen = () => (
  <Combobox items={payees} defaultValue="Shell Station" defaultOpen>
    <ComboboxInput placeholder="Select payee" className="w-56" />
    <ComboboxContent>
      <ComboboxEmpty>No payees found.</ComboboxEmpty>
      <ComboboxList>
        {(item: string) => (
          <ComboboxItem key={item} value={item}>
            {item}
          </ComboboxItem>
        )}
      </ComboboxList>
    </ComboboxContent>
  </Combobox>
)

export const MultiCategoryChips = () => {
  const anchorRef = useComboboxAnchor()
  return (
    <Combobox multiple items={categories} defaultValue={['Groceries', 'Transport']}>
      <ComboboxChips ref={anchorRef} className="w-72">
        <ComboboxValue>
          {(value: string[]) => (
            <>
              {value.map((category) => (
                <ComboboxChip key={category}>{category}</ComboboxChip>
              ))}
              <ComboboxChipsInput placeholder="Add category" />
            </>
          )}
        </ComboboxValue>
      </ComboboxChips>
      <ComboboxContent anchor={anchorRef}>
        <ComboboxEmpty>No categories found.</ComboboxEmpty>
        <ComboboxList>
          {(item: string) => (
            <ComboboxItem key={item} value={item}>
              {item}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}
