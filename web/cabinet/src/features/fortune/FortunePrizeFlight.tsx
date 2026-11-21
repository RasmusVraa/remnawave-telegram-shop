import { useEffect, useMemo } from 'react'
import { createPortal } from 'react-dom'
import { motion } from 'framer-motion'
import { Sparkles } from 'lucide-react'

import { cn } from '@/lib/utils'
import { FORTUNE_SECTOR_ICONS } from '@/features/fortune/fortuneSectorIcons'
import { sectorIconClass } from '@/features/fortune/fortunePrizeVisuals'

const FLIGHT_MS = 720
const FLIGHT_Z = 2700

/** Куда летит приз: счётчик в шапке страницы. У скидки и XP счётчика на странице нет, они просто закрываются. */
export function fortuneFlightTargetId(rewardType: string): string | null {
  if (rewardType.startsWith('days_')) return 'fortune-stat-days'
  if (rewardType === 'free_spin') return 'fortune-stat-spins'
  return null
}

type FortunePrizeFlightProps = {
  rewardType: string
  /** Где была плитка приза в окне, когда нажали «Забрать». */
  from: DOMRect
  targetId: string
  onDone: () => void
}

/** Иконка приза вылетает из окна «Ваш приз» и садится в счётчик, который подпрыгивает при попадании. */
export function FortunePrizeFlight({ rewardType, from, targetId, onDone }: FortunePrizeFlightProps) {
  const target = useMemo(() => document.getElementById(targetId), [targetId])
  const Icon = FORTUNE_SECTOR_ICONS[rewardType] ?? Sparkles
  const size = 56

  const path = useMemo(() => {
    const t = target?.getBoundingClientRect()
    if (!t) return null
    const sx = from.left + from.width / 2
    const sy = from.top + from.height / 2
    const dx = t.left + t.width / 2 - sx
    const dy = t.top + t.height / 2 - sy
    return {
      x: [0, dx * 0.35, dx],
      y: [0, dy * 0.2 - 60, dy],
      scale: [1, 1.25, 0.35],
      sx,
      sy,
    }
  }, [target, from])

  useEffect(() => {
    if (!path) onDone()
  }, [path, onDone])

  if (!path || typeof document === 'undefined') return null

  return createPortal(
    <motion.div
      className={cn('pointer-events-none fixed flex items-center justify-center')}
      style={{ left: path.sx - size / 2, top: path.sy - size / 2, width: size, height: size, zIndex: FLIGHT_Z }}
      initial={{ x: 0, y: 0, scale: 1, opacity: 1 }}
      animate={{ x: path.x, y: path.y, scale: path.scale, opacity: [1, 1, 0.9] }}
      transition={{ duration: FLIGHT_MS / 1000, times: [0, 0.35, 1], ease: [0.5, 0, 0.3, 1] }}
      onAnimationComplete={() => {
        // Счётчик подпрыгивает в момент попадания.
        target?.classList.remove('fortune-stat-bump')
        void target?.offsetWidth
        target?.classList.add('fortune-stat-bump')
        window.setTimeout(() => target?.classList.remove('fortune-stat-bump'), 700)
        onDone()
      }}
      aria-hidden
    >
      <Icon className={cn('size-full', sectorIconClass(rewardType))} strokeWidth={2} />
    </motion.div>,
    document.body,
  )
}
