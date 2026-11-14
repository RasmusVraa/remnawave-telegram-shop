import { Fragment, useMemo } from 'react'
import type { TFunction } from 'i18next'
import { Sparkles } from 'lucide-react'

import { cn } from '@/lib/utils'

import type { FortuneRecentWinItem } from '@/lib/api'
import { FORTUNE_SECTOR_ICONS } from '@/features/fortune/fortuneSectorIcons'
import { sectorIconClass, wheelSectorRimSolid } from '@/features/fortune/fortunePrizeVisuals'

/** Относительное время: после 24 ч — только дни (2 дн. назад), не «48 ч». */
export function formatFortuneWinRelative(iso: string, t: TFunction): string {
  const ts = Date.parse(iso)
  if (Number.isNaN(ts)) return ''
  const sec = Math.max(0, Math.floor((Date.now() - ts) / 1000))
  if (sec < 45) return t('fortune.relative.justNow')
  const min = Math.floor(sec / 60)
  if (min < 60) return t('fortune.relative.minutes', { n: min })
  const h = Math.floor(sec / 3600)
  if (h < 24) return t('fortune.relative.hours', { n: h })
  const d = Math.floor(sec / 86400)
  return t('fortune.relative.days', { n: Math.min(99, Math.max(1, d)) })
}

function sectorIdxForType(rt: string): number {
  const order = [
    'micro',
    'xp',
    'discount_3',
    'days_3',
    'discount_5',
    'days_5',
    'days_7',
    'days_15',
    'days_30',
    'days_180',
    'free_spin',
  ]
  const i = order.indexOf(rt)
  return i === -1 ? 0 : i
}

function buildMarqueeStrip(items: FortuneRecentWinItem[]): FortuneRecentWinItem[] {
  if (items.length === 0) return []
  const minLen = 12
  let acc: FortuneRecentWinItem[] = []
  while (acc.length < minLen) {
    acc = [...acc, ...items]
  }
  return acc
}

/** Свечение текста призов «в днях» — зелёное, в духе drop-shadow иконок в «Вы можете получить». */
function tickerDaysPrizeClass(): string {
  return 'text-emerald-600 drop-shadow-[0_0_10px_rgba(16,185,129,0.55)] dark:text-emerald-300 dark:drop-shadow-[0_0_14px_rgba(52,211,153,0.5)]'
}

function TickerEntryText({ w, t }: { w: FortuneRecentWinItem; t: TFunction }) {
  const rt = w.reward_type
  const v = w.reward_value
  const name = w.masked_name

  if (rt.startsWith('days_')) {
    return (
      <>
        <span className="font-semibold text-[#0c1222] dark:text-white">{name}</span>
        <span className="text-slate-600 dark:text-slate-400"> {t('fortune.ticker.won')} </span>
        <span className={cn('font-semibold', tickerDaysPrizeClass())}>{t('fortune.ticker.daysPrize', { d: v })}</span>
      </>
    )
  }
  if (rt.startsWith('discount_')) {
    return (
      <>
        <span className="font-semibold text-[#0c1222] dark:text-white">{name}</span>
        <span className="font-semibold text-[#0c1222] dark:text-white"> {t('fortune.ticker.discount', { pct: v })}</span>
      </>
    )
  }
  const rim = wheelSectorRimSolid(rt, sectorIdxForType(rt))
  if (rt === 'free_spin') {
    return (
      <>
        <span className="font-semibold text-[#0c1222] dark:text-white">{name}</span>
        <span className="text-slate-600 dark:text-slate-400"> {t('fortune.ticker.won')} </span>
        <span className="font-semibold" style={{ color: rim }}>
          {t('fortune.ticker.freeSpin')}
        </span>
      </>
    )
  }
  return (
    <>
      <span className="font-semibold text-[#0c1222] dark:text-white">{name}</span>
      <span className="font-semibold" style={{ color: rim }}>
        {' '}
        {t('fortune.ticker.xpPrize', { value: v })}
      </span>
    </>
  )
}

function MarqueeStrip({
  strip,
  t,
  stripKey,
}: {
  strip: FortuneRecentWinItem[]
  t: TFunction
  stripKey: string
}) {
  return (
    <>
      {strip.map((w, i) => {
        const Icon = FORTUNE_SECTOR_ICONS[w.reward_type] ?? Sparkles
        const iconCls = sectorIconClass(w.reward_type)
        const rel = formatFortuneWinRelative(w.spin_at, t)
        return (
          <Fragment key={`${stripKey}-${w.spin_at}-${w.masked_name}-${w.reward_type}-${i}`}>
            {i > 0 ? (
              <span
                className="inline-flex h-7 w-7 shrink-0 items-center justify-center self-center text-[10px] leading-none text-slate-400 dark:text-slate-500"
                aria-hidden
              >
                ●
              </span>
            ) : null}
            <span className="inline-flex items-center gap-1.5 text-[13px] leading-tight sm:text-sm">
              <span className="inline-flex size-7 shrink-0 items-center justify-center rounded-full border border-slate-300/80 bg-white/90 shadow-inner dark:border-white/10 dark:bg-white/5">
                <Icon
                  className={cn('size-3.5 shrink-0', iconCls, w.reward_type.startsWith('days_') && 'scale-105')}
                  strokeWidth={2.4}
                  aria-hidden
                />
              </span>
              <TickerEntryText w={w} t={t} />
              <span className="shrink-0 whitespace-nowrap text-slate-500 dark:text-slate-400"> · {rel}</span>
            </span>
          </Fragment>
        )
      })}
    </>
  )
}

type FeedProps = {
  items: FortuneRecentWinItem[]
  t: TFunction
}

/** Бегущая строка победителей (единственный вариант UI). */
export function FortuneWinnersMarquee({ items, t }: FeedProps) {
  const strip = useMemo(() => buildMarqueeStrip(items), [items])
  if (strip.length === 0) {
    return (
      <div className="rounded-xl border border-slate-300/80 bg-[hsl(228_56%_97%)] px-4 py-2.5 text-center text-xs text-slate-600 dark:border-white/10 dark:bg-[#0a0e27] dark:text-slate-400">
        {t('fortune.winnerFeedEmpty')}
      </div>
    )
  }

  return (
    <div className="relative overflow-hidden rounded-xl border border-slate-300/90 bg-[hsl(228_56%_97%)] shadow-sm dark:border-white/[0.08] dark:bg-[#0a0e27] dark:shadow-none">
      <div className="pointer-events-none absolute inset-y-0 left-0 z-10 w-8 bg-gradient-to-r from-[hsl(228_56%_97%)] to-transparent dark:from-[#0a0e27]" />
      <div className="pointer-events-none absolute inset-y-0 right-0 z-10 w-8 bg-gradient-to-l from-[hsl(228_56%_97%)] to-transparent dark:from-[#0a0e27]" />
      <div className="overflow-hidden py-2.5 px-4 sm:px-5">
        <div className="flex w-max min-w-full animate-fortune-winners-marquee items-center motion-reduce:animate-none">
          <div className="inline-flex shrink-0 items-center whitespace-nowrap">
            <MarqueeStrip strip={strip} t={t} stripKey="a" />
          </div>
          <div className="inline-flex shrink-0 items-center whitespace-nowrap" aria-hidden>
            <MarqueeStrip strip={strip} t={t} stripKey="b" />
          </div>
        </div>
      </div>
    </div>
  )
}
