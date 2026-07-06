import type { CSSProperties } from 'react'

const COIN_STAGGER_S = 0.16

export function CoinLoader({ label }: { label?: string }) {
  return (
    <div className="flex justify-center" role="status" aria-label={label}>
      <div className="coin-loader" aria-hidden="true">
        {[0, 1, 2].map((i) => (
          <span key={i} className="coin-loader-unit" style={{ '--coin-delay': `${i * COIN_STAGGER_S}s` } as CSSProperties}>
            <span className="coin-loader-coin">e</span>
            <span className="coin-loader-shadow" />
          </span>
        ))}
      </div>
    </div>
  )
}
