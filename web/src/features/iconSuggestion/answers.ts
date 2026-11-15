// The suggestions answered for this page's lifetime, per user, so retyping a name asks nothing
// twice. Kept apart from the hook so the test setup can reset it without loading the hook's
// imports ahead of a test file's mocks.
export const answers = new Map<string, string>()

export function resetIconSuggestionsForTests(): void {
  answers.clear()
}
