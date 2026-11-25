import { useMemo } from 'react'
import { motion, type Variants } from 'framer-motion'

import { cn } from '@/lib/utils'
import { fortuneChipOffsetPercent, sectorCenterDeg } from '@/features/fortune/fortuneWheelGeometry'
import { FORTUNE_SECTOR_ICONS } from '@/features/fortune/fortuneSectorIcons'
import { sectorIconClassWheel, type FortuneDesignVariant } from '@/features/fortune/fortunePrizeVisuals'

type WinTier = 'normal' | 'good' | 'jackpot'

type WheelWinAnimationProps = {
  active: boolean
  prize: string | null
  amount: number
  winnerIndex: number
  sectorCount: number
  designVariant: FortuneDesignVariant
}

type Particle = {
  x: number
  y: number
  size: number
  delay: number
  duration: number
  color: string
  rotate: number
}

const FLASH_VARIANTS: Variants = {
  hidden: { opacity: 0 },
  show: {
    opacity: [0, 1, 0.84, 0],
    transition: { duration: 0.9, times: [0, 0.2, 0.45, 1], ease: 'easeOut' },
  },
}

function tierFromPrize(prize: string | null, amount: number): WinTier {
  if (!prize) return 'normal'
  if (prize === 'days_180' || prize === 'days_30' || prize === 'days_15') return 'jackpot'
  if (prize === 'discount_5' || amount >= 30) return 'good'
  return 'normal'
}

function buildConfetti(tier: WinTier): Particle[] {
  const base = tier === 'jackpot' ? 82 : tier === 'good' ? 58 : 38
  const out: Particle[] = []
  const colors =
    tier === 'jackpot'
      ? ['#facc15', '#f59e0b', '#fb7185', '#c084fc', '#22d3ee', '#34d399']
      : ['#22d3ee', '#a78bfa', '#f59e0b', '#34d399', '#f472b6']
  for (let i = 0; i < base; i++) {
    const t = i / Math.max(1, base - 1)
    const angle = Math.PI * 2 * t + (i % 3) * 0.18
    const radius = 72 + ((i * 37) % 140)
    out.push({
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius * 0.92 + 32,
      size: 5 + (i % 6),
      delay: (i % 14) * 0.018,
      duration: 0.9 + (i % 4) * 0.12,
      color: colors[i % colors.length],
      rotate: (i % 2 === 0 ? 1 : -1) * (180 + (i % 6) * 45),
    })
  }
  return out
}

export function WheelWinAnimation({
  active,
  prize,
  amount,
  winnerIndex,
  sectorCount,
  designVariant,
}: WheelWinAnimationProps) {
  const tier = tierFromPrize(prize, amount)
  const confetti = useMemo(() => buildConfetti(tier), [tier])
  const angle = sectorCenterDeg(winnerIndex, Math.max(1, sectorCount))
  const glow = fortuneChipOffsetPercent(angle, 37)
  const launch = fortuneChipOffsetPercent(angle, 40)
  const PrizeIcon = prize ? (FORTUNE_SECTOR_ICONS[prize] ?? FORTUNE_SECTOR_ICONS.micro) : FORTUNE_SECTOR_ICONS.micro

  if (!active) return null

  return (
    <div className="pointer-events-none absolute inset-0 z-[35] overflow-hidden rounded-full motion-reduce:hidden" aria-hidden>
      <motion.div
        variants={FLASH_VARIANTS}
        initial="hidden"
        animate="show"
        className="absolute inset-[-10%] bg-[radial-gradient(circle_at_50%_50%,rgba(255,255,255,0.97)_0%,rgba(250,204,21,0.62)_24%,rgba(251,146,60,0.22)_48%,transparent_72%)]"
      />

      <motion.div
        initial={{ opacity: 0, scale: 0.7 }}
        animate={{
          opacity: [0, tier === 'jackpot' ? 1 : 0.82, 0.45, 0],
          scale: [0.7, 1.04, 1, 1.06],
        }}
        transition={{ duration: 1.25, times: [0, 0.22, 0.58, 1], ease: 'easeOut' }}
        className={cn(
          'absolute left-1/2 top-1/2 size-[46%] rounded-full',
          tier === 'jackpot'
            ? 'shadow-[0_0_52px_18px_rgba(250,204,21,0.6),0_0_86px_34px_rgba(255,255,255,0.28)]'
            : 'shadow-[0_0_32px_10px_rgba(250,204,21,0.45),0_0_58px_20px_rgba(255,255,255,0.18)]',
        )}
        style={{
          transform: `translate(calc(-50% + ${glow.x}%), calc(-50% + ${glow.y}%))`,
          background:
            'radial-gradient(circle, rgba(255,255,255,0.96) 0%, rgba(250,204,21,0.66) 32%, rgba(250,204,21,0.18) 58%, transparent 74%)',
        }}
      />

      <motion.div
        initial={{ opacity: 0, scale: 0.9 }}
        animate={{ opacity: [0, 1, 0], scale: [0.9, 1.02, 1] }}
        transition={{ duration: 1.4, times: [0, 0.38, 1], repeat: tier === 'jackpot' ? 1 : 0, repeatDelay: 0.1 }}
        className="absolute inset-[8%] rounded-full border border-white/50"
      />

      <motion.div
        initial={{ opacity: 0, x: `${launch.x}%`, y: `${launch.y}%`, scale: 0.7 }}
        animate={{ opacity: [0, 1, 1, 0], y: [`${launch.y}%`, `${launch.y - 20}%`, `${launch.y - 39}%`, `${launch.y - 45}%`], scale: [0.7, 1.04, 1, 0.84] }}
        transition={{ duration: 1.1, times: [0, 0.22, 0.7, 1], ease: 'easeOut' }}
        className="absolute left-1/2 top-1/2"
      >
        <div className="rounded-full bg-white/20 p-2 backdrop-blur-sm">
          <PrizeIcon className={cn('size-8', sectorIconClassWheel(prize ?? 'micro', designVariant))} />
        </div>
      </motion.div>

      {confetti.map((p, i) => (
        <motion.span
          key={i}
          initial={{ opacity: 0, x: 0, y: 0, rotate: 0, scale: 0.6 }}
          animate={{ opacity: [0, 1, 1, 0], x: p.x, y: p.y, rotate: p.rotate, scale: [0.6, 1, 1, 0.4] }}
          transition={{ delay: p.delay, duration: p.duration, ease: [0.22, 1, 0.36, 1] }}
          className="absolute left-1/2 top-1/2"
          style={{
            width: p.size,
            height: p.size * 1.36,
            borderRadius: i % 2 ? '999px' : '2px',
            backgroundColor: p.color,
            boxShadow: `0 0 ${4 + (i % 5)}px ${p.color}`,
          }}
        />
      ))}
    </div>
  )
}
