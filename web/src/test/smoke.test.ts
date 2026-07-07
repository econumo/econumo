describe('harness', () => {
  it('runs tests with jsdom and localStorage', () => {
    localStorage.setItem('k', 'v')
    expect(localStorage.getItem('k')).toBe('v')
  })
})
