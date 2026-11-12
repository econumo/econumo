// iPadOS 13+ reports itself as a Mac; the touch-point probe tells them apart.
// Used for iPhone-specific setup hints.
export function isIOS(): boolean {
  const ua = navigator.userAgent
  return /iPhone|iPad|iPod/.test(ua) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
}

// Apple Wallet's card-tap automation exists on iPhone only, so iPad is out.
export function isIPhone(): boolean {
  return /iPhone|iPod/.test(navigator.userAgent)
}

// iPadOS also reports Macintosh, so isIOS() has the final word.
export function isMac(): boolean {
  return /Macintosh/.test(navigator.userAgent) && !isIOS()
}

// Hands a custom-scheme URL (shortcuts://…) to the OS by navigating the page.
// A thin wrapper so callers can be tested; jsdom cannot navigate.
export function openAppUrl(url: string): void {
  window.location.href = url
}
