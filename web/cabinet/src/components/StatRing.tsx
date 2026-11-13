import { Infinity as InfinityIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { RING_THICKNESS_PX, RingArc } from '@/components/RingArc'
import { cn } from '@/lib/utils'
import type { ToneLevel } from '@/lib/subscriptionTone'

/**
 * Кольцевой индикатор показателя подписки.
 *
 * Цвет задаётся уровнем тревоги, поэтому кольцо, полосы и подписи красятся
 * из одного места и не расходятся между собой. Дуга — SVG со скруглёнными
 * концами и тенью у кончика (RingArc).
 */

const TONE_COLOR: Record<ToneLevel, string> = {
  calm: 'var(--primary)',
  warn: '38 92% 50%',
  danger: 'var(--destructive)',
}

export function StatRing({
  value,
  tone = 'calm',
  size = 72,
  label,
  children,
  className,
}: {
  /** 0–100. */
  value: number
  tone?: ToneLevel
  size?: number
  label?: string
  children: ReactNode
  className?: string
}) {
  // Единицы viewBox (100) на пиксель: толщина в пикселях одна и та же при любом размере плитки.
  const w = RING_THICKNESS_PX * (100 / size)
  return (
    <div className={cn('flex flex-col items-center gap-2', className)}>
      <div className="cabinet-ring shrink-0" style={{ width: size }}>
        <svg viewBox="0 0 100 100" className="absolute inset-0 h-full w-full" aria-hidden>
          <RingArc
            r={50 - w / 2}
            w={w}
            value={value}
            color={`hsl(${TONE_COLOR[tone]})`}
            trackColor="hsl(var(--muted))"
            trackOpacity={1}
          />
        </svg>
        <div className="text-center leading-none">{children}</div>
      </div>
      {label && (
        <span className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</span>
      )}
    </div>
  )
}

/**
 * Кольцо для безлимитного трафика: статичное, со знаком бесконечности.
 * Прогресса без потолка не существует, но место в ряду показателей занять надо.
 */
export function UnboundedRing({
  size = 72,
  label,
  className,
}: {
  size?: number
  label?: string
  className?: string
}) {
  return (
    <div className={cn('flex flex-col items-center gap-2', className)}>
      <div
        className="cabinet-ring cabinet-ring--unbounded shrink-0"
        style={{ width: size, height: size }}
      >
        <InfinityIcon size={size * 0.32} className="text-primary" aria-hidden />
      </div>
      {label && (
        <span className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</span>
      )}
    </div>
  )
}
