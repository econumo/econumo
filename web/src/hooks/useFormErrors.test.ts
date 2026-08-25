import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useFormErrors } from './useFormErrors'

describe('useFormErrors', () => {
  it('clears one key without disturbing the others', () => {
    const { result } = renderHook(() => useFormErrors<{ name?: string; amount?: string }>())
    act(() => result.current.setErrors({ name: 'Required field', amount: 'Invalid' }))
    act(() => result.current.clear('name'))
    expect(result.current.errors.name).toBeUndefined()
    expect(result.current.errors.amount).toBe('Invalid')
  })

  it('keeps the same object when the key is already clear, so no needless re-render', () => {
    const { result } = renderHook(() => useFormErrors<{ name?: string }>())
    act(() => result.current.setErrors({ name: 'Required field' }))
    act(() => result.current.clear('name'))
    const afterFirst = result.current.errors
    act(() => result.current.clear('name'))
    expect(result.current.errors).toBe(afterFirst)
  })

  it('reset drops every error', () => {
    const { result } = renderHook(() => useFormErrors<{ name?: string; amount?: string }>())
    act(() => result.current.setErrors({ name: 'a', amount: 'b' }))
    act(() => result.current.reset())
    expect(result.current.errors).toEqual({})
  })
})
