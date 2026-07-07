interface EntityIconProps {
  name?: string | null
  className?: string
}

export function EntityIcon({ name, className }: EntityIconProps) {
  return (
    <span aria-hidden="true" className={`material-icon select-none ${className ?? ''}`}>
      {name || 'question_mark'}
    </span>
  )
}
