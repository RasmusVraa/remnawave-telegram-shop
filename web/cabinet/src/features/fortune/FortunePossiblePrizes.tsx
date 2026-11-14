import { useEffect, useMemo, useRef } from 'react'
import type { TFunction } from 'i18next'
import { Sparkles } from 'lucide-react'

import type { FortuneSectorDTO } from '@/lib/api'
import { cn } from '@/lib/utils'
import { FORTUNE_SECTOR_ICONS } from '@/features/fortune/fortuneSectorIcons'
import { sectorAccentBarClass, sectorIconClass, sortSectorsByPrizeDesc } from '@/features/fortune/fortunePrizeVisuals'

const AUTO_SCROLL_MS = 3000
const AUTO_SCROLL_ANIM_MS = 880

function easeInOutCubic(t: number): number {
  const x = Math.min(1, Math.max(0, t))
  return x < 0.5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2
}

function prizeTitle(t: TFunction, s: FortuneSectorDTO): string {
  const rt = s.reward_type
  if (rt === 'micro') return t('fortune.prizeCard.micro')
  if (rt === 'xp') return t('fortune.prizeCard.xp')
  if (rt === 'free_spin') return t('fortune.prizeCard.freeSpin')
  if (rt === 'discount_3' || rt === 'discount_5') {
    const pct = s.display_percent ?? (rt === 'discount_3' ? 3 : 5)
    return t('fortune.prizeCard.discount', { pct })
  }
  if (rt.startsWith('days_')) {
    const d = s.display_days ?? 0
    return t('fortune.prizeCard.plusDays', { d })
  }
  return t(`fortune.sector.${rt}`, { defaultValue: rt })
}

function prizeSubtitle(t: TFunction, s: FortuneSectorDTO): string {
  if (s.reward_type.startsWith('days_')) return t('fortune.prizeCard.hintDays')
  if (s.reward_type === 'discount_3' || s.reward_type === 'discount_5') return t('fortune.prizeCard.hintDiscount')
  if (s.reward_type === 'micro' || s.reward_type === 'xp') return t('fortune.prizeCard.hintXp')
  if (s.reward_type === 'free_spin') return t('fortune.prizeCard.hintFreeSpin')
  return ''
}

type FortunePossiblePrizesProps = {
  sectors: FortuneSectorDTO[]
  t: TFunction
}

/** Горизонтальная витрина: от лучшего приза к меньшему, автоскролл каждые 3 с. */
export function FortunePossiblePrizes({ sectors, t }: FortunePossiblePrizesProps) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const sorted = useMemo(() => sortSectorsByPrizeDesc(sectors), [sectors])

  useEffect(() => {
    const el = scrollRef.current
    if (!el || sorted.length === 0) return undefined
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return undefined

    let raf = 0
    let cancelled = false

    const animateScrollTo = (targetLeft: number) => {
      const from = el.scrollLeft
      const t0 = performance.now()
      const tickAnim = (now: number) => {
        if (cancelled) return
        const u = Math.min(1, (now - t0) / AUTO_SCROLL_ANIM_MS)
        el.scrollLeft = from + (targetLeft - from) * easeInOutCubic(u)
        if (u < 1) {
          raf = requestAnimationFrame(tickAnim)
        } else {
          raf = 0
        }
      }
      if (raf) cancelAnimationFrame(raf)
      raf = requestAnimationFrame(tickAnim)
    }

    const stepScroll = () => {
      const max = el.scrollWidth - el.clientWidth
      if (max <= 4) return
      const step = Math.min(240, Math.max(160, el.clientWidth * 0.45))
      const atEnd = el.scrollLeft + 6 >= max
      const target = atEnd ? 0 : Math.min(max, el.scrollLeft + step)
      animateScrollTo(target)
    }

    const id = window.setInterval(stepScroll, AUTO_SCROLL_MS)
    return () => {
      cancelled = true
      window.clearInterval(id)
      cancelAnimationFrame(raf)
    }
  }, [sorted])

  if (sorted.length === 0) return null

  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4 shadow-sm">
      <p className="mb-3 text-[14px] font-semibold leading-snug tracking-tight text-foreground sm:text-[16px]">
        {t('fortune.youCanWinTitle')}
      </p>
      <div
        ref={scrollRef}
        className={cn(
          'fortune-prizes-scroll -mx-1 flex gap-2.5 overflow-x-auto pb-2 pt-0.5',
          'motion-reduce:scroll-auto',
        )}
      >
        {sorted.map((s) => {
          const Icon = FORTUNE_SECTOR_ICONS[s.reward_type] ?? Sparkles
          const bar = sectorAccentBarClass(s.reward_type)
          const iconCls = sectorIconClass(s.reward_type)
          const sub = prizeSubtitle(t, s)
          return (
            <div
              key={s.reward_type}
              className="flex min-w-[112px] max-w-[132px] shrink-0 flex-col overflow-hidden rounded-xl border border-border/80 bg-muted/30 shadow-sm sm:min-w-[140px] sm:max-w-[168px]"
            >
              <div className={cn('h-1 w-full shrink-0', bar)} />
              <div className="flex flex-1 flex-col items-center gap-1.5 px-2 py-2.5 sm:gap-2 sm:px-2.5 sm:py-3.5">
                <Icon className={cn('size-7 shrink-0 sm:size-10', iconCls)} strokeWidth={2} aria-hidden />
                <p className="text-center text-[12px] font-semibold leading-snug text-foreground sm:text-[14px]">
                  {prizeTitle(t, s)}
                </p>
                {sub ? (
                  <p className="text-center text-[10px] leading-snug text-muted-foreground sm:text-[12px]">{sub}</p>
                ) : null}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
