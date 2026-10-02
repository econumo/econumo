/** marks an amount that is not in the budget currency (those carry no symbol) */
export function CurrencyTag({ code }: { code: string }) {
  return (
    <span data-testid="currency-tag" className="shrink-0 rounded bg-muted px-1 text-[10px] font-medium text-muted-foreground">
      {code}
    </span>
  )
}
