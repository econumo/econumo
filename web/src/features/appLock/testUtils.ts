import type { AppLockConfig } from './config'

export function testConfig(overrides: Partial<AppLockConfig> = {}): AppLockConfig {
  return {
    v: 1,
    userId: 'u1',
    secret: { kind: 'pin', salt: 'c2FsdA==', hash: 'aGFzaA==', iterations: 1000, length: 4 },
    lockAfterMs: 60_000,
    ...overrides,
  }
}

export function signIn(userId: string | null = 'u1'): void {
  localStorage.setItem('token', 'tok')
  if (userId) {
    localStorage.setItem('sessionUserId', userId)
  }
}

const GRID_PX = 300

// jsdom lays nothing out: give the grid a 300px box so pointer positions map to dots.
export function layOutGrid(grid: HTMLElement): void {
  grid.getBoundingClientRect = () =>
    ({ left: 0, top: 0, right: GRID_PX, bottom: GRID_PX, width: GRID_PX, height: GRID_PX, x: 0, y: 0, toJSON: () => ({}) }) as DOMRect
}

export function dotCentre(node: number): { clientX: number; clientY: number } {
  const cell = GRID_PX / 3
  return { clientX: ((node % 3) + 0.5) * cell, clientY: (Math.floor(node / 3) + 0.5) * cell }
}

// One continuous stroke through the given dots, as a finger would draw it.
export function drawPattern(
  fire: { pointerDown: (el: Element, init: object) => unknown; pointerMove: (el: Element, init: object) => unknown; pointerUp: (el: Element, init: object) => unknown },
  grid: HTMLElement,
  nodes: number[],
): void {
  layOutGrid(grid)
  const init = { pointerId: 1, pointerType: 'touch', isPrimary: true }
  fire.pointerDown(grid, { ...init, ...dotCentre(nodes[0]) })
  for (const node of nodes.slice(1)) {
    fire.pointerMove(grid, { ...init, ...dotCentre(node) })
  }
  fire.pointerUp(grid, { ...init, ...dotCentre(nodes.at(-1)!) })
}
