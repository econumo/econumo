import { encodePattern, extendPattern, isValidPattern, nodeBetween, patternLength } from './pattern'

describe('nodeBetween', () => {
  it('finds the dot a straight line passes over', () => {
    expect(nodeBetween(0, 2)).toBe(1)
    expect(nodeBetween(0, 8)).toBe(4)
    expect(nodeBetween(6, 2)).toBe(4)
    expect(nodeBetween(1, 7)).toBe(4)
    expect(nodeBetween(3, 5)).toBe(4)
    expect(nodeBetween(2, 8)).toBe(5)
  })

  it('finds none between neighbours or on a knight move', () => {
    expect(nodeBetween(0, 1)).toBeNull()
    expect(nodeBetween(0, 4)).toBeNull()
    expect(nodeBetween(0, 5)).toBeNull()
    expect(nodeBetween(0, 7)).toBeNull()
  })
})

describe('extendPattern', () => {
  it('adds a skipped middle dot first', () => {
    expect(extendPattern([0], 2)).toEqual([0, 1, 2])
    expect(extendPattern([0], 8)).toEqual([0, 4, 8])
  })

  it('crosses a dot already used without adding it again', () => {
    expect(extendPattern([4, 0], 8)).toEqual([4, 0, 8])
  })

  it('never takes a dot twice', () => {
    expect(extendPattern([0, 1, 2], 1)).toEqual([0, 1, 2])
  })

  it('ignores anything off the grid', () => {
    expect(extendPattern([0], 9)).toEqual([0])
    expect(extendPattern([0], -1)).toEqual([0])
  })
})

describe('encoding', () => {
  it('encodes in drawing order', () => {
    expect(encodePattern([0, 4, 8, 5])).toBe('p:0-4-8-5')
    expect(patternLength('p:0-4-8-5')).toBe(4)
  })

  it('accepts 4 to 9 distinct dots only', () => {
    expect(isValidPattern('p:0-4-8-5')).toBe(true)
    expect(isValidPattern('p:0-1-2-3-4-5-6-7-8')).toBe(true)
    expect(isValidPattern('p:0-4-8')).toBe(false)
    expect(isValidPattern('p:0-4-8-4')).toBe(false)
    expect(isValidPattern('p:0-4-8-9')).toBe(false)
    expect(isValidPattern('0-4-8-5')).toBe(false)
    expect(isValidPattern('1234')).toBe(false)
  })
})
