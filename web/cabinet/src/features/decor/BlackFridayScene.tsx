/**
 * Сцена темы «Чёрная пятница»: золотые лучи сверху, пирамида слитков и стопки
 * монет по нижним углам, ценники на нитках в боковых полях. Это фон, а не
 * главный герой: всё приглушено, лежит под контентом и не ловит клики.
 *
 * Слитки, стопки и ценники — только от 1400px (на узких экранах им нет места,
 * кабинет важнее). В админке (`compact`) их нет совсем: там контент во всю
 * ширину, остаются только лучи и падающие монеты.
 *
 * Тема только тёмная (decorThemes.ts → DARK_ONLY_DECOR_THEMES).
 */
import { useTranslation } from 'react-i18next'

import './blackFriday.css'

function BlackFridayDefs() {
  return (
    <svg width="0" height="0" className="bf-art__defs" aria-hidden focusable="false">
      <defs>
        <radialGradient id="bfa-cg" cx="35%" cy="30%" r="80%">
          <stop offset="0" stopColor="#fff6c0" />
          <stop offset=".45" stopColor="#f2c230" />
          <stop offset="1" stopColor="#a86d00" />
        </radialGradient>
        <linearGradient id="bfa-igf" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#f5c93a" />
          <stop offset=".55" stopColor="#cf9208" />
          <stop offset="1" stopColor="#8a5600" />
        </linearGradient>
        <linearGradient id="bfa-igt" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#fff6bd" />
          <stop offset="1" stopColor="#f3c840" />
        </linearGradient>
        <linearGradient id="bfa-igr" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#e9b32a" />
          <stop offset="1" stopColor="#b57c06" />
        </linearGradient>
        <linearGradient id="bfa-edge" x1="0" y1="0" x2="1" y2="0">
          <stop offset="0" stopColor="#8a5600" />
          <stop offset=".3" stopColor="#f0bd30" />
          <stop offset=".55" stopColor="#ffe27a" />
          <stop offset="1" stopColor="#7a4a00" />
        </linearGradient>
        <linearGradient id="bfa-ctop" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#fff2a8" />
          <stop offset=".6" stopColor="#f2c230" />
          <stop offset="1" stopColor="#c58a05" />
        </linearGradient>
        <radialGradient id="bfa-glow" cx="50%" cy="60%" r="55%">
          <stop offset="0" stopColor="#f2c230" stopOpacity=".35" />
          <stop offset="1" stopColor="#f2c230" stopOpacity="0" />
        </radialGradient>
        <pattern id="bfa-ridge" width="3.4" height="4" patternUnits="userSpaceOnUse">
          <rect width="1.2" height="4" fill="#5a3600" opacity=".55" />
        </pattern>
        <symbol id="bfa-bar" viewBox="0 0 140 74">
          <ellipse cx="70" cy="72" rx="68" ry="3" fill="#000" opacity=".4" />
          <path d="M12 36H128L140 66Q141 71 136 71H4Q-1 71 0 66Z" fill="url(#bfa-igf)" />
          <path d="M12 36H128L140 66Q141 71 136 71" fill="none" stroke="#ffe9a0" strokeOpacity=".35" />
          <path d="M30 7Q32 2 38 2H102Q108 2 110 7L128 36H12Z" fill="url(#bfa-igt)" />
          <path d="M40 11H100L114 31H26Z" fill="url(#bfa-igr)" stroke="#a87208" strokeOpacity=".7" />
          <path d="M44 14H96L106 27H34Z" fill="none" stroke="#fff3b0" strokeOpacity=".5" strokeWidth=".8" />
          <path d="M14 40L50 40L42 68H6Z" fill="#fff" opacity=".16" />
          <path d="M58 40L66 40L58 68H50Z" fill="#fff" opacity=".1" />
          <text x="70" y="61" textAnchor="middle" fontSize="10.5" fontWeight="800" letterSpacing="2.4" fill="#5a3600" fillOpacity=".75" fontFamily="Inter,Arial,sans-serif">
            Au 999.9
          </text>
          <text x="70" y="25" textAnchor="middle" fontSize="9" fontWeight="800" letterSpacing="2" fill="#7a4d00" fillOpacity=".7" fontFamily="Inter,Arial,sans-serif">
            1000 g
          </text>
        </symbol>
      </defs>
    </svg>
  )
}

const BARS: ReadonlyArray<{ x: number; y: number; tilt?: boolean }> = [
  { x: 0, y: 124 },
  { x: 155, y: 124 },
  { x: 310, y: 124 },
  { x: 78, y: 68 },
  { x: 232, y: 68 },
  { x: 155, y: 12, tilt: true },
]

function IngotsPile() {
  return (
    <svg className="bf-art__ingots" viewBox="0 0 450 200" overflow="visible" aria-hidden focusable="false">
      <ellipse cx="225" cy="150" rx="260" ry="90" fill="url(#bfa-glow)" />
      {BARS.map((b, i) => (
        <use
          key={i}
          href="#bfa-bar"
          x={b.x}
          y={b.y}
          width="140"
          height="74"
          transform={b.tilt ? 'rotate(-2 225 50)' : undefined}
        />
      ))}
    </svg>
  )
}

const JITTER = [0, 1.5, -1, 2, -1.5, 1, -2] as const

function CoinStack({ n, mark, x, y }: { n: number; mark: string; x: number; y: number }) {
  const topY = 80 - (n - 1) * 9 - 9
  return (
    <g transform={`translate(${x} ${y})`}>
      {Array.from({ length: n }, (_, i) => {
        const yb = 80 - i * 9
        return (
          <g key={i} transform={`translate(${JITTER[i % JITTER.length]} 0)`}>
            <ellipse cx="32" cy={yb} rx="30" ry="8" fill="#6a4000" />
            <rect x="2" y={yb - 9} width="60" height="9" fill="url(#bfa-edge)" />
            <rect x="2" y={yb - 9} width="60" height="9" fill="url(#bfa-ridge)" />
            <ellipse cx="32" cy={yb - 9} rx="30" ry="8" fill="url(#bfa-ctop)" stroke="#8a5600" strokeOpacity=".6" />
            <ellipse cx="32" cy={yb - 9} rx="23" ry="5.6" fill="none" stroke="#8a5600" strokeOpacity=".45" />
          </g>
        )
      })}
      <text
        transform={`translate(32 ${topY + 3.2}) scale(1 .5)`}
        textAnchor="middle"
        fontSize="17"
        fontWeight="800"
        fill="#7a4d00"
        fillOpacity=".8"
        fontFamily="Inter,Arial,sans-serif"
      >
        {mark}
      </text>
    </g>
  )
}

function CoinStacks() {
  return (
    <svg className="bf-art__stacks" viewBox="0 0 250 100" overflow="visible" aria-hidden focusable="false">
      <ellipse cx="125" cy="92" rx="125" ry="9" fill="#000" opacity=".35" />
      <CoinStack n={5} mark="₽" x={0} y={6} />
      <CoinStack n={8} mark="%" x={76} y={-2} />
      <CoinStack n={4} mark="₽" x={160} y={10} />
      <g transform="translate(222 84) rotate(-8)">
        <ellipse cx="0" cy="0" rx="18" ry="5" fill="#6a4000" />
        <ellipse cx="0" cy="-3" rx="18" ry="5" fill="url(#bfa-ctop)" stroke="#8a5600" strokeOpacity=".6" />
      </g>
    </svg>
  )
}

function PriceTag({ className, glyph, label }: { className: string; glyph: string; label: string }) {
  return (
    <div className={`bf-tag ${className}`}>
      <div className="bf-tag__body">
        <span className="bf-tag__glyph">{glyph}</span>
        <small>{label}</small>
      </div>
    </div>
  )
}

export function BlackFridayScene({ compact = false }: { compact?: boolean }) {
  const { t } = useTranslation()
  return (
    <>
      <div className="bf-rays" />
      {!compact && (
        <div className="bf-art">
          <BlackFridayDefs />
          <IngotsPile />
          <CoinStacks />
          <PriceTag className="bf-tag--l1" glyph="SALE" label={t('decorBlackFriday.tagSale')} />
          <PriceTag className="bf-tag--r1" glyph="%" label={t('decorBlackFriday.tagDeal')} />
          <PriceTag className="bf-tag--r2" glyph="★" label={t('decorBlackFriday.tagHit')} />
        </div>
      )}
    </>
  )
}
