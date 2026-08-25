import { useCallback, useState } from 'react'

// Field errors for the hand-written dialog forms. They are raised on submit, so
// each one has to be retracted as soon as its own field changes — otherwise a
// message the user has already acted on stays on screen until the next submit.
// `clear` is per-key on purpose: fixing one field must not blank the others.
export function useFormErrors<E extends object>(initial: E = {} as E) {
  const [errors, setErrors] = useState<E>(initial)

  const clear = useCallback((key: keyof E) => {
    setErrors((prev) => (prev[key] === undefined ? prev : { ...prev, [key]: undefined }))
  }, [])

  const reset = useCallback(() => setErrors({} as E), [])

  return { errors, setErrors, clear, reset }
}
