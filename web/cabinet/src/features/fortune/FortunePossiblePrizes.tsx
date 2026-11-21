import { useEffect, useMemo, useRef } from 'react'
import type { TFunction } from 'i18next'
import { Sparkles } from 'lucide-react'

import type { FortuneSectorDTO } from '@/lib/api'
import { cn } from '@/lib/utils'
import { FORTUNE_SECTOR_ICONS } from '@/features/fortune/fortuneSectorIcons'
import {
  sectorAccentBarClass,
  sectorIconClass,
  sectorIconClassWheel,
  sortSectorsByPrizeDesc,
} from '@/features/fortune/fortunePrizeVisuals'

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
    <div className="rounded-[20px] border border-border bg-card/80 p-3.5 shadow-sm">
      <p className="mb-3 text-base font-semibold leading-snug tracking-tight text-foreground">
        {t('fortune.youCanWinTitle')}
      </p>
      <div
        ref={scrollRef}
        className={cn(
          'fortune-prizes-scroll flex gap-2.5 overflow-x-auto pb-1.5',
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
              className="flex w-24 shrink-0 flex-col overflow-hidden rounded-[14px] border border-border/80 bg-muted/30 shadow-sm sm:w-[108px]"
            >
              <div className={cn('h-[3px] w-full shrink-0', bar)} />
              <div className="flex flex-1 flex-col items-center gap-1 px-1.5 py-3">
                <Icon className={cn('size-6 shrink-0', iconCls)} strokeWidth={2} aria-hidden />
                <p className="mt-0.5 text-center text-[13px] font-semibold leading-tight text-foreground sm:text-sm">
                  {prizeTitle(t, s)}
                </p>
                {sub ? (
                  <p className="text-center text-[11px] leading-snug text-muted-foreground sm:text-xs">{sub}</p>
                ) : null}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}


/** ПК: те же призы столбиком справа от колеса — от лучшего к меньшему, без автопрокрутки. */
export function FortunePrizeList({
  sectors,
  t,
  highlightType = null,
}: FortunePossiblePrizesProps & {
  /** Тип приза, который только что выпал: его строка вспыхивает. */
  highlightType?: string | null
}) {
  const sorted = useMemo(() => sortSectorsByPrizeDesc(sectors), [sectors])
  if (sorted.length === 0) return null

  return (
    <div>
      <p className="mb-2.5 text-base font-semibold tracking-tight text-foreground">{t('fortune.youCanWinTitle')}</p>
      <ul className="grid gap-1">
        {sorted.map((s) => {
          const Icon = FORTUNE_SECTOR_ICONS[s.reward_type] ?? Sparkles
          const sub = prizeSubtitle(t, s)
          return (
            <li
              key={s.reward_type}
              className={cn(
                'flex items-center gap-3 rounded-xl border border-border/60 bg-muted/20 px-3 py-1.5 text-sm dark:border-white/[0.08] dark:bg-white/[0.03] font-semibold text-foreground transition-[background-color,border-color,transform] duration-300',
                highlightType === s.reward_type &&
                  'scale-[1.02] border-primary/70 bg-primary/15 dark:border-primary/70 dark:bg-primary/15',
              )}
            >
              {/* Светлая тема — насыщенные -700, тёмная — светлые -300: пастель на белом не читается. */}
              <Icon className={cn('size-5 shrink-0', sectorIconClassWheel(s.reward_type, 'bento'))} strokeWidth={2} aria-hidden />
              <span className="min-w-0 truncate">{prizeTitle(t, s)}</span>
              {sub ? <span className="ml-auto shrink-0 text-xs font-normal text-muted-foreground">{sub}</span> : null}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
