// One walkthrough per iOS version, newest first: the Shortcuts app looks
// different enough between versions that one recording cannot serve both.
// Hosted outside the binary (every self-hosted download would carry them
// otherwise) and only ever linked, so a page that is not clicked makes no
// request to the host. An empty list hides the link.
export interface SetupVideo {
  /** shown as is, e.g. "iOS 26" */
  label: string
  url: string
}

export const SETUP_VIDEOS: SetupVideo[] = []
