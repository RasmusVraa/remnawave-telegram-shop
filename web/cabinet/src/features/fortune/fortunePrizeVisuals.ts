import type { FortuneSectorDTO } from '@/lib/api'

/** Варианты дизайна колеса — разные визуальные концепции, не только оттенок заливки. */
export type FortuneDesignVariant = 'classic' | 'porcelain' | 'void' | 'ledger' | 'bento'

/** Сектора в порядке `index` из API. Порядок слотов на диске задаёт `arrangeFortuneSlots`. */
export function sortFortuneSectorsByIndex(sectors: FortuneSectorDTO[]): FortuneSectorDTO[] {
  return [...sectors].sort((a, b) => a.index - b.index)
}

/** От лучшего к худшему для витрины «Вы можете получить» (не связано с весами RNG). */
export const FORTUNE_PRIZE_SORT_ORDER: readonly string[] = [
  'days_180',
  'days_30',
  'days_15',
  'days_7',
  'days_5',
  'days_3',
  'discount_5',
  'discount_3',
  'free_spin',
  'xp',
  'micro',
]

export function sortSectorsByPrizeDesc(sectors: FortuneSectorDTO[]): FortuneSectorDTO[] {
  return [...sectors].sort((a, b) => {
    const ia = FORTUNE_PRIZE_SORT_ORDER.indexOf(a.reward_type)
    const ib = FORTUNE_PRIZE_SORT_ORDER.indexOf(b.reward_type)
    const va = ia === -1 ? 999 : ia
    const vb = ib === -1 ? 999 : ib
    return va - vb
  })
}

/** Заливка сектора (яркая, классическое колесо). */
export function wheelSectorFill(rewardType: string, index: number): string {
  const map: Record<string, string> = {
    micro: 'hsl(285 88% 62% / 0.94)',
    xp: 'hsl(46 100% 54% / 0.94)',
    discount_3: 'hsl(204 95% 58% / 0.94)',
    days_3: 'hsl(168 78% 48% / 0.94)',
    discount_5: 'hsl(248 82% 65% / 0.94)',
    days_5: 'hsl(152 76% 46% / 0.94)',
    days_7: 'hsl(130 70% 46% / 0.94)',
    days_15: 'hsl(24 95% 58% / 0.94)',
    days_30: 'hsl(350 85% 58% / 0.94)',
    days_180: 'hsl(318 80% 58% / 0.94)',
    free_spin: 'hsl(188 85% 44% / 0.94)',
  }
  if (map[rewardType]) return map[rewardType]
  const h = (index * 47 + 180) % 360
  return `hsl(${h} 78% 56% / 0.92)`
}

/** Непрозрачный цвет редкости для кольца на ободе / акцентов. */
export function wheelSectorRimSolid(rewardType: string, index: number): string {
  const map: Record<string, string> = {
    micro: 'hsl(285 72% 48%)',
    xp: 'hsl(42 96% 48%)',
    discount_3: 'hsl(204 88% 48%)',
    days_3: 'hsl(168 72% 40%)',
    discount_5: 'hsl(258 78% 58%)',
    days_5: 'hsl(152 68% 40%)',
    days_7: 'hsl(130 62% 40%)',
    days_15: 'hsl(24 90% 50%)',
    days_30: 'hsl(350 78% 52%)',
    days_180: 'hsl(318 72% 55%)',
    free_spin: 'hsl(188 80% 38%)',
  }
  if (map[rewardType]) return map[rewardType]
  const h = (index * 47 + 180) % 360
  return `hsl(${h} 65% 50%)`
}

/** Пастельная заливка «бенто» (низкая насыщенность, разные оттенки). */
function wheelSectorBentoFill(rewardType: string, index: number): string {
  const map: Record<string, string> = {
    micro: 'hsl(285 35% 92%)',
    xp: 'hsl(42 40% 91%)',
    discount_3: 'hsl(204 38% 91%)',
    days_3: 'hsl(168 32% 90%)',
    discount_5: 'hsl(258 36% 91%)',
    days_5: 'hsl(152 32% 90%)',
    days_7: 'hsl(130 36% 90%)',
    days_15: 'hsl(28 42% 91%)',
    days_30: 'hsl(350 38% 91%)',
    days_180: 'hsl(318 40% 91%)',
    free_spin: 'hsl(188 36% 90%)',
  }
  if (map[rewardType]) return map[rewardType]
  const h = (index * 47 + 180) % 360
  return `hsl(${h} 28% 90%)`
}

/** Заливка диска. `slots` — уже в порядке слотов (см. `arrangeFortuneSlots`), не в порядке `index`. */
export function buildWheelConicGradient(slots: FortuneSectorDTO[], variant: FortuneDesignVariant = 'classic'): string {
  const ordered = slots
  if (ordered.length === 0) return 'conic-gradient(from 0deg, hsl(var(--muted)) 0deg 360deg)'
  const n = ordered.length
  const step = 360 / n

  if (variant === 'porcelain') {
    const a = 'hsl(var(--card))'
    const b = 'hsl(var(--muted) / 0.55)'
    const parts = ordered.map((_, i) => {
      const fill = i % 2 === 0 ? a : b
      return `${fill} ${i * step}deg ${(i + 1) * step}deg`
    })
    return `conic-gradient(from 0deg, ${parts.join(', ')})`
  }

  if (variant === 'void') {
    const c = 'hsl(225 42% 9%)'
    return `conic-gradient(from 0deg, ${c} 0deg 360deg)`
  }

  if (variant === 'ledger') {
    const parts = ordered.map((_, i) => {
      const fill = i % 2 === 0 ? 'hsl(42 28% 96%)' : 'hsl(38 22% 91%)'
      return `${fill} ${i * step}deg ${(i + 1) * step}deg`
    })
    return `conic-gradient(from 0deg, ${parts.join(', ')})`
  }

  if (variant === 'bento') {
    const parts = ordered.map((s, i) => {
      const fill = wheelSectorBentoFill(s.reward_type, s.index)
      return `${fill} ${i * step}deg ${(i + 1) * step}deg`
    })
    return `conic-gradient(from 0deg, ${parts.join(', ')})`
  }

  const parts = ordered.map((s, i) => {
    const c = wheelSectorFill(s.reward_type, s.index)
    return `${c} ${i * step}deg ${(i + 1) * step}deg`
  })
  return `conic-gradient(from 0deg, ${parts.join(', ')})`
}

/** Кольцо редкости на ободе (полный круг conic — маска задаётся в компоненте). `slots` — в порядке слотов. */
export function buildOuterRarityRingGradient(slots: FortuneSectorDTO[]): string {
  const ordered = slots
  if (ordered.length === 0) return ''
  const step = 360 / ordered.length
  const parts = ordered.map((s, i) => {
    const c = wheelSectorRimSolid(s.reward_type, s.index)
    return `${c} ${i * step}deg ${(i + 1) * step}deg`
  })
  return `conic-gradient(from 0deg, ${parts.join(', ')})`
}

/** Иконки на колесе: яркие (тёмный диск / классика) или приглушённые (светлый диск). */
export function sectorIconClassWheel(rewardType: string, variant: FortuneDesignVariant): string {
  if (variant === 'classic') {
    return 'text-white drop-shadow-[0_1px_6px_rgb(0_0_0_/_0.45)]'
  }
  if (variant === 'porcelain' || variant === 'ledger' || variant === 'bento') {
    const map: Record<string, string> = {
      micro: 'text-fuchsia-700 dark:text-fuchsia-300',
      xp: 'text-amber-800 dark:text-amber-300',
      discount_3: 'text-sky-700 dark:text-sky-300',
      days_3: 'text-teal-800 dark:text-teal-300',
      discount_5: 'text-violet-800 dark:text-violet-300',
      days_5: 'text-emerald-800 dark:text-emerald-300',
      days_7: 'text-lime-800 dark:text-lime-300',
      days_15: 'text-orange-800 dark:text-orange-300',
      days_30: 'text-rose-800 dark:text-rose-300',
      days_180: 'text-pink-800 dark:text-pink-300',
      free_spin: 'text-cyan-800 dark:text-cyan-300',
    }
    return map[rewardType] ?? 'text-primary'
  }
  return sectorIconClass(rewardType)
}

/** Цвет иконки (Tailwind-классы) — классическое колесо и «пустота». */
export function sectorIconClass(rewardType: string): string {
  const map: Record<string, string> = {
    micro: 'text-fuchsia-400 drop-shadow-[0_0_6px_rgb(232_121_249_/_0.45)]',
    xp: 'text-amber-300 drop-shadow-[0_0_6px_rgb(252_211_77_/_0.45)]',
    discount_3: 'text-sky-300 drop-shadow-[0_0_6px_rgb(125_211_252_/_0.45)]',
    days_3: 'text-teal-300 drop-shadow-[0_0_6px_rgb(94_234_212_/_0.4)]',
    discount_5: 'text-violet-300 drop-shadow-[0_0_6px_rgb(196_181_253_/_0.45)]',
    days_5: 'text-emerald-300 drop-shadow-[0_0_6px_rgb(110_231_183_/_0.4)]',
    days_7: 'text-lime-300 drop-shadow-[0_0_6px_rgb(190_242_100_/_0.4)]',
    days_15: 'text-orange-300 drop-shadow-[0_0_6px_rgb(253_186_116_/_0.45)]',
    days_30: 'text-rose-300 drop-shadow-[0_0_6px_rgb(253_164_175_/_0.45)]',
    days_180: 'text-pink-300 drop-shadow-[0_0_6px_rgb(249_168_212_/_0.45)]',
    free_spin: 'text-cyan-300 drop-shadow-[0_0_6px_rgb(103_232_249_/_0.45)]',
  }
  return map[rewardType] ?? 'text-primary drop-shadow-sm'
}

/** Акцентная полоска для карточки «можно выиграть». */
export function sectorAccentBarClass(rewardType: string): string {
  const map: Record<string, string> = {
    micro: 'bg-fuchsia-500',
    xp: 'bg-amber-400',
    discount_3: 'bg-sky-500',
    days_3: 'bg-teal-500',
    discount_5: 'bg-violet-500',
    days_5: 'bg-emerald-500',
    days_7: 'bg-lime-500',
    days_15: 'bg-orange-500',
    days_30: 'bg-rose-500',
    days_180: 'bg-pink-500',
    free_spin: 'bg-cyan-500',
  }
  return map[rewardType] ?? 'bg-primary'
}
