import type { TFunction } from 'i18next'
import { Sparkles } from 'lucide-react'
import { useMemo } from 'react'

import type { FortuneSectorDTO } from '@/lib/api'
import { cn } from '@/lib/utils'
import { fortuneChipOffsetPercent, sectorCenterDeg } from '@/features/fortune/fortuneWheelGeometry'
import { FORTUNE_SECTOR_ICONS } from '@/features/fortune/fortuneSectorIcons'
import {
  type FortuneDesignVariant,
  buildOuterRarityRingGradient,
  sectorIconClassWheel,
} from '@/features/fortune/fortunePrizeVisuals'

/** Радиус до центра чипа (% от ширины квадрата колеса). */
const CHIP_RADIUS_PCT = 37

function sectorShortLabel(t: TFunction, s: FortuneSectorDTO): string {
  const rt = s.reward_type
  if (rt === 'micro') return t('fortune.wheelShort.micro')
  if (rt === 'xp') return t('fortune.wheelShort.xp')
  if (rt === 'free_spin') return t('fortune.wheelShort.freeSpin', { n: s.display_spins ?? 1 })
  if (rt === 'discount_3' || rt === 'discount_5') {
    const pct = s.display_percent ?? (rt === 'discount_3' ? 3 : 5)
    return t('fortune.wheelShort.discount', { pct })
  }
  if (rt.startsWith('days_')) {
    const d = s.display_days ?? 0
    return t('fortune.wheelShort.plusDays', { d })
  }
  return t(`fortune.sector.${rt}`, { defaultValue: rt })
}

function wheelLabelClass(variant: FortuneDesignVariant): string {
  if (variant === 'porcelain' || variant === 'ledger' || variant === 'bento') {
    return 'text-zinc-800 dark:text-zinc-100 [text-shadow:none] sm:text-[9px] md:text-[10px]'
  }
  return 'text-white [text-shadow:0_1px_3px_rgb(0_0_0/_0.92),0_0_10px_rgb(0_0_0/_0.35)] sm:text-[9px] md:text-[10px]'
}

function SectorChip({
  sector,
  slotIndex,
  n,
  t,
  designVariant,
}: {
  sector: FortuneSectorDTO
  slotIndex: number
  n: number
  t: TFunction
  designVariant: FortuneDesignVariant
}) {
  const ang = sectorCenterDeg(slotIndex, n)
  const SectorIcon = FORTUNE_SECTOR_ICONS[sector.reward_type] ?? Sparkles
  const label = sectorShortLabel(t, sector)
  const iconCls = sectorIconClassWheel(sector.reward_type, designVariant)
  const { x, y } = fortuneChipOffsetPercent(ang, CHIP_RADIUS_PCT)

  return (
    <div
      className="pointer-events-none absolute flex flex-col items-center justify-center"
      style={{
        left: `calc(50% + ${x}%)`,
        top: `calc(50% + ${y}%)`,
        transform: `translate(-50%, -50%) rotate(${-ang}deg)`,
      }}
    >
      <div className="flex min-h-[42px] w-[56px] max-w-[56px] flex-col items-center justify-center gap-0.5 px-0.5 text-center sm:min-h-[52px] sm:w-[72px] sm:max-w-[72px] md:min-h-[58px] md:w-[80px] md:max-w-[80px]">
        <SectorIcon
          className={cn(
            'size-6 shrink-0 sm:size-7 md:size-9',
            designVariant === 'classic' || designVariant === 'void' ? 'drop-shadow-md' : 'drop-shadow-sm',
            iconCls,
          )}
          strokeWidth={2.5}
          aria-hidden
        />
        <span
          className={cn(
            'line-clamp-2 w-full text-[8px] font-bold leading-[1.12]',
            wheelLabelClass(designVariant),
          )}
        >
          {label}
        </span>
      </div>
    </div>
  )
}

function SectorSpokes({ n, variant }: { n: number; variant: FortuneDesignVariant }) {
  if (variant !== 'porcelain' && variant !== 'ledger' && variant !== 'bento') return null
  const step = 360 / n
  const stroke =
    variant === 'ledger'
      ? 'rgba(60, 40, 20, 0.22)'
      : variant === 'porcelain'
        ? 'hsl(var(--border) / 0.85)'
        : 'hsl(var(--foreground) / 0.12)'

  return (
    <svg
      className="pointer-events-none absolute inset-0 z-[6] h-full w-full rounded-full"
      viewBox="0 0 100 100"
      aria-hidden
    >
      {Array.from({ length: n }, (_, i) => (
        <line
          key={i}
          x1="50"
          y1="50"
          x2="50"
          y2="4"
          stroke={stroke}
          strokeWidth={variant === 'ledger' ? 0.55 : 0.45}
          vectorEffect="non-scaling-stroke"
          transform={`rotate(${i * step} 50 50)`}
        />
      ))}
    </svg>
  )
}

type FortuneWheelFaceProps = {
  /** В порядке слотов на диске (`arrangeFortuneSlots`), а не `index` из API. */
  sectors: FortuneSectorDTO[]
  rotationDeg: number
  spinning: boolean
  spinMs: number
  gradient: string
  designVariant: FortuneDesignVariant
  t: TFunction
  hubRewardType: string | null
  className?: string
}

function outerRingMask(variant: FortuneDesignVariant): string {
  if (variant === 'porcelain') return 'radial-gradient(circle, transparent 72%, #000 73.5%, #000 100%)'
  if (variant === 'void') return 'radial-gradient(circle, transparent 76%, #000 77.5%, #000 100%)'
  if (variant === 'ledger') return 'radial-gradient(circle, transparent 74%, #000 75.2%, #000 100%)'
  if (variant === 'bento') return 'radial-gradient(circle, transparent 73%, #000 74.5%, #000 100%)'
  return 'radial-gradient(circle, transparent 63%, #000 66%, #000 100%)'
}

/** Круг колеса: conic-gradient + подписи/иконки призов (вращается вместе с колесом). */
export function FortuneWheelFace({
  sectors,
  rotationDeg,
  spinning,
  spinMs,
  gradient,
  designVariant,
  t,
  hubRewardType,
  className,
}: FortuneWheelFaceProps) {
  const ordered = sectors
  const n = ordered.length
  const HubIcon = hubRewardType ? (FORTUNE_SECTOR_ICONS[hubRewardType] ?? Sparkles) : Sparkles
  const hubIconCls = hubRewardType
    ? sectorIconClassWheel(hubRewardType, designVariant)
    : designVariant === 'void'
      ? 'text-emerald-400/90'
      : 'text-primary'
  const rarityRing = useMemo(() => buildOuterRarityRingGradient(ordered), [ordered])

  const showRarityRing =
    designVariant === 'porcelain' ||
    designVariant === 'void' ||
    designVariant === 'ledger' ||
    designVariant === 'bento' ||
    designVariant === 'classic'

  const ringMask = outerRingMask(designVariant)

  const diskShadow =
    designVariant === 'porcelain'
      ? '0 1px 0 rgb(255 255 255 / 0.65) inset, 0 12px 40px -18px rgb(0 0 0 / 0.18), 0 0 0 1px hsl(var(--border) / 0.9)'
      : designVariant === 'void'
        ? '0 0 0 1px rgb(16 185 129 / 0.25), 0 0 48px -8px rgb(16 185 129 / 0.2), inset 0 0 40px rgb(0 0 0 / 0.45)'
        : designVariant === 'ledger'
          ? '0 2px 0 rgb(255 255 255 / 0.5) inset, 0 10px 28px -14px rgb(60 40 20 / 0.15), 0 0 0 1px rgb(60 40 20 / 0.12)'
          : designVariant === 'bento'
            ? '0 8px 32px -14px rgb(0 0 0 / 0.12), inset 0 1px 0 rgb(255 255 255 / 0.7), 0 0 0 1px hsl(var(--border) / 0.6)'
            : '0 0 48px -10px rgb(2 132 199 / 0.35), inset 0 0 28px rgb(0 0 0 / 0.14), inset 0 0 0 1px rgb(255 255 255 / 0.06)'

  return (
    <div
      className={cn(
        'relative aspect-square w-full max-w-full rounded-full border-4 border-border shadow-xl',
        designVariant === 'porcelain' && 'border-zinc-200/90 dark:border-zinc-600/90',
        designVariant === 'void' && 'border-emerald-500/35 dark:border-emerald-400/30',
        designVariant === 'ledger' && 'border-amber-900/25 dark:border-amber-200/20',
        designVariant === 'bento' && 'border-border',
        'transition-transform [transition-timing-function:cubic-bezier(0.22,0.61,0.36,1)]',
        className,
      )}
      style={{
        transform: `rotate(${rotationDeg}deg)`,
        background: gradient,
        boxShadow: diskShadow,
        transitionDuration: spinning ? `${spinMs}ms` : '0ms',
      }}
    >
      {showRarityRing && rarityRing && (
        <div
          className={cn(
            'pointer-events-none absolute inset-0 z-[4] rounded-full',
            designVariant === 'void' && 'opacity-95',
          )}
          style={{
            background: rarityRing,
            WebkitMaskImage: ringMask,
            maskImage: ringMask,
          }}
          aria-hidden
        />
      )}

      {designVariant === 'void' && (
        <div
          className="pointer-events-none absolute inset-[3%] z-[3] rounded-full opacity-40"
          style={{
            boxShadow: 'inset 0 0 0 1px rgba(52, 211, 153, 0.35)',
          }}
          aria-hidden
        />
      )}

      <SectorSpokes n={n} variant={designVariant} />

      <div className="pointer-events-none absolute inset-0 z-10 rounded-full" aria-hidden>
        {n > 0 &&
          ordered.map((s, i) => (
            <SectorChip
              key={`fortune-slot-${s.index}-${s.reward_type}`}
              sector={s}
              slotIndex={i}
              n={n}
              t={t}
              designVariant={designVariant}
            />
          ))}
      </div>

      <div
        className={cn(
          'absolute inset-[27%] z-20 flex items-center justify-center rounded-full border shadow-inner',
          designVariant === 'porcelain' &&
            'border-border bg-card/98 backdrop-blur-sm dark:bg-card/95',
          designVariant === 'void' &&
            'border-emerald-500/40 bg-slate-950/90 backdrop-blur-md dark:bg-slate-950/95',
          designVariant === 'ledger' &&
            'border-amber-900/20 bg-[hsl(42_30%_98%)] dark:border-amber-200/15 dark:bg-[hsl(222_22%_14%)]',
          designVariant === 'bento' &&
            'border-border/80 bg-card/95 backdrop-blur-sm',
          designVariant === 'classic' && 'border-border/80 bg-card/95 backdrop-blur-sm',
        )}
      >
        <HubIcon className={cn('size-8 transition-colors duration-150 sm:size-10', hubIconCls)} strokeWidth={2} />
      </div>
    </div>
  )
}
