import type { ReactNode } from 'react'

import { RING_THICKNESS_PX, RingArc } from '@/components/RingArc'
import '@/lib/splitColor'
import type { ToneLevel } from '@/lib/subscriptionTone'

/**
 * Два вложенных кольца для плитки «Трафик» (раздельный трафик): снаружи обычные
 * серверы, внутри обход. Цвет внутреннего кольца — фиолетовый, пока всё спокойно;
 * жёлтый и красный — те же уровни тревоги, что у остальных колец главной.
 *
 * Толщина и дуги со скруглёнными концами и тенью у кончика — общие с кольцами «Дней» и
 * «Устройств» (RingArc): ряд плиток выглядит ровно. У безлимитных обычных серверов внешнее
 * кольцо приглушённое, без кончика и тени, а знак «∞» вписан прямо в него (в верхней точке,
 * по центру полосы), без отдельной плашки.
 */

const OUTER_COLOR: Record<ToneLevel, string> = {
  calm: 'hsl(var(--primary))',
  warn: 'hsl(38 92% 50%)',
  danger: 'hsl(var(--destructive))',
}
const INNER_COLOR: Record<ToneLevel, string> = {
  calm: 'hsl(var(--split-bypass))',
  warn: 'hsl(38 92% 50%)',
  danger: 'hsl(var(--destructive))',
}

/** Цвет метки-точки в подписи под кольцом. */
export const SPLIT_DOT_COLOR = { main: OUTER_COLOR, bypass: INNER_COLOR }

type Layer = { value: number | null; tone: ToneLevel }

/** Зазор между кольцами в пикселях. */
const GAP_PX = 2.5

/** Безлимитное кольцо: полное, приглушённое, со знаком «∞» по центру полосы в верхней точке. */
function UnlimitedArc({ r, w }: { r: number; w: number }) {
  // Знак нарисован контуром вокруг своего центра (0, 0): так он стоит ровно посередине полосы,
  // а не «сидит» на базовой линии шрифта. Масштаб привязан к толщине кольца.
  const k = (w / 11) * 0.84
  return (
    <>
      <circle cx="50" cy="50" r={r} fill="none" stroke={OUTER_COLOR.calm} strokeOpacity={0.3} strokeWidth={w} />
      <path
        className="nested-ring-infinity"
        transform={`translate(50 ${50 - r}) scale(${k})`}
        d="M0 0 C2 -4.6 8.4 -4.6 8.4 0 C8.4 4.6 2 4.6 0 0 C-2 -4.6 -8.4 -4.6 -8.4 0 C-8.4 4.6 -2 4.6 0 0Z"
        fill="none"
        stroke="currentColor"
        strokeWidth={2.3}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </>
  )
}

export function NestedStatRing({
  outer,
  inner,
  size = 64,
  children,
}: {
  /** Обычные серверы. value = null — безлимит: кольцо полное, приглушённое, со знаком «∞». */
  outer: Layer
  /** Обход. */
  inner: Layer & { value: number }
  size?: number
  children: ReactNode
}) {
  // Единицы viewBox (100) на пиксель: толщина в пикселях одна и та же при любом размере плитки.
  const unit = 100 / size
  const w = RING_THICKNESS_PX * unit
  const outerR = 50 - w / 2
  const innerR = outerR - w - GAP_PX * unit
  return (
    <div className="relative shrink-0" style={{ width: size, height: size }}>
      <svg viewBox="0 0 100 100" width={size} height={size} aria-hidden>
        {outer.value === null ? (
          <UnlimitedArc r={outerR} w={w} />
        ) : (
          <RingArc r={outerR} w={w} value={outer.value} color={OUTER_COLOR[outer.tone]} />
        )}
        <RingArc r={innerR} w={w} value={inner.value} color={INNER_COLOR[inner.tone]} />
      </svg>
      <div className="absolute inset-0 flex items-center justify-center text-center leading-none">{children}</div>
    </div>
  )
}
