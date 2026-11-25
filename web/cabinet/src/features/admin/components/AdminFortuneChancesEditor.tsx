import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Gift, Lock, LockOpen, RotateCcw } from 'lucide-react'

import { cn } from '@/lib/utils'
import { sectorIconClassWheel, wheelSectorRimSolid } from '@/features/fortune/fortunePrizeVisuals'
import { FORTUNE_SECTOR_ICONS } from '@/features/fortune/fortuneSectorIcons'
import { surface } from './Surface'

/** Весь пирог: 10 000 долей = 100%, одна доля = 0,01%. В настройках лежат именно доли. */
const TOTAL = 10000
/** Защита симуляции от бесконечной цепочки бесплатных спинов. */
const SIM_SPIN_CAP = 5000

type SizeKind = 'one' | 'range'
type SizeUnit = 'xp' | 'discount' | 'days' | 'spin'

interface PrizeDef {
  rewardType: string
  weightKey: string
  /** Для 'one' — один ключ размера; для 'range' — [от, до]. */
  sizeKeys: string[]
  sizeKind: SizeKind
  unit: SizeUnit
  nameKey: 'micro' | 'xp' | 'discount' | 'days' | 'free_spin'
  /** Стандартный вес (шкала 1000, сумма 1000) — для кнопки «Сбросить» и пустых настроек. */
  defaultWeight: number
  /** Стандартный размер приза, в порядке sizeKeys. */
  defaultSizes: number[]
}

/** Приз, который выдавать нельзя: сектор «+180 дней» на колесе только для вида. Замок неснимаемый. */
const NEVER_DROPS = 'days_180'

const DEFAULT_SIZES: Record<string, number[]> = {
  micro: [5, 15],
  xp: [35],
  discount_3: [3],
  discount_5: [5],
  days_3: [3],
  days_5: [5],
  days_7: [7],
  days_15: [15],
  days_30: [30],
  days_180: [180],
  free_spin: [1],
}

const PRIZES: PrizeDef[] = [
  { rewardType: 'micro', weightKey: 'FORTUNE_WEIGHT_MICRO', sizeKeys: ['FORTUNE_REWARD_MICRO_XP_MIN', 'FORTUNE_REWARD_MICRO_XP_MAX'], sizeKind: 'range', unit: 'xp', nameKey: 'micro', defaultWeight: 478, defaultSizes: DEFAULT_SIZES['micro'] },
  { rewardType: 'xp', weightKey: 'FORTUNE_WEIGHT_XP', sizeKeys: ['FORTUNE_REWARD_XP_AMOUNT'], sizeKind: 'one', unit: 'xp', nameKey: 'xp', defaultWeight: 354, defaultSizes: DEFAULT_SIZES['xp'] },
  { rewardType: 'discount_3', weightKey: 'FORTUNE_WEIGHT_DISCOUNT_3', sizeKeys: ['FORTUNE_REWARD_DISCOUNT_3_PERCENT'], sizeKind: 'one', unit: 'discount', nameKey: 'discount', defaultWeight: 60, defaultSizes: DEFAULT_SIZES['discount_3'] },
  { rewardType: 'discount_5', weightKey: 'FORTUNE_WEIGHT_DISCOUNT_5', sizeKeys: ['FORTUNE_REWARD_DISCOUNT_5_PERCENT'], sizeKind: 'one', unit: 'discount', nameKey: 'discount', defaultWeight: 40, defaultSizes: DEFAULT_SIZES['discount_5'] },
  { rewardType: 'days_3', weightKey: 'FORTUNE_WEIGHT_DAYS_3', sizeKeys: ['FORTUNE_REWARD_DAYS_3'], sizeKind: 'one', unit: 'days', nameKey: 'days', defaultWeight: 25, defaultSizes: DEFAULT_SIZES['days_3'] },
  { rewardType: 'days_5', weightKey: 'FORTUNE_WEIGHT_DAYS_5', sizeKeys: ['FORTUNE_REWARD_DAYS_5'], sizeKind: 'one', unit: 'days', nameKey: 'days', defaultWeight: 15, defaultSizes: DEFAULT_SIZES['days_5'] },
  { rewardType: 'days_7', weightKey: 'FORTUNE_WEIGHT_DAYS_7', sizeKeys: ['FORTUNE_REWARD_DAYS_7'], sizeKind: 'one', unit: 'days', nameKey: 'days', defaultWeight: 10, defaultSizes: DEFAULT_SIZES['days_7'] },
  { rewardType: 'days_15', weightKey: 'FORTUNE_WEIGHT_DAYS_15', sizeKeys: ['FORTUNE_REWARD_DAYS_15'], sizeKind: 'one', unit: 'days', nameKey: 'days', defaultWeight: 6, defaultSizes: DEFAULT_SIZES['days_15'] },
  { rewardType: 'days_30', weightKey: 'FORTUNE_WEIGHT_DAYS_30', sizeKeys: ['FORTUNE_REWARD_DAYS_30'], sizeKind: 'one', unit: 'days', nameKey: 'days', defaultWeight: 2, defaultSizes: DEFAULT_SIZES['days_30'] },
  { rewardType: 'days_180', weightKey: 'FORTUNE_WEIGHT_DAYS_180', sizeKeys: ['FORTUNE_REWARD_DAYS_180'], sizeKind: 'one', unit: 'days', nameKey: 'days', defaultWeight: 0, defaultSizes: DEFAULT_SIZES['days_180'] },
  { rewardType: 'free_spin', weightKey: 'FORTUNE_WEIGHT_FREE_SPIN', sizeKeys: ['FORTUNE_REWARD_FREE_SPINS'], sizeKind: 'one', unit: 'spin', nameKey: 'free_spin', defaultWeight: 10, defaultSizes: DEFAULT_SIZES['free_spin'] },
]

/** Ключи, которыми управляет редактор: отдельными полями на странице они больше не показываются. */
export const FORTUNE_CHANCES_KEYS: string[] = [
  ...PRIZES.map((p) => p.weightKey),
  ...PRIZES.flatMap((p) => p.sizeKeys),
]

/** Целые доли с суммой ровно TOTAL (метод наибольших остатков). */
function roundToTotal(raw: number[]): number[] {
  const floors = raw.map((v) => Math.floor(v))
  let left = TOTAL - floors.reduce((s, v) => s + v, 0)
  raw
    .map((v, i) => [i, v - Math.floor(v)] as const)
    .sort((a, b) => b[1] - a[1])
    .forEach(([i]) => {
      if (left > 0) {
        floors[i] += 1
        left -= 1
      }
    })
  return floors
}

/** Доли из сохранённых весов: любая шкала (1000, 10000, 999…) приводится к 10 000. */
function weightsToUnits(values: Record<string, string>): number[] {
  let w = PRIZES.map((p) => {
    const n = Number.parseInt(values[p.weightKey] ?? '', 10)
    return Number.isFinite(n) && n > 0 ? n : 0
  })
  if (w.every((v) => v === 0)) w = PRIZES.map((p) => p.defaultWeight)
  const sum = w.reduce((s, v) => s + v, 0)
  return roundToTotal(w.map((v) => (v / sum) * TOTAL))
}

/** Двигаем один приз: остальные (кроме запертых) сжимаются или растут пропорционально. */
function moveUnits(cur: number[], idx: number, target: number, locked: Set<number>): number[] {
  const free = cur.map((_, i) => i).filter((i) => i !== idx && !locked.has(i))
  const lockedSum = cur.reduce((s, v, i) => (i !== idx && locked.has(i) ? s + v : s), 0)
  let v = Math.max(0, Math.min(Math.round(target), TOTAL - lockedSum))
  if (free.length === 0) v = TOTAL - lockedSum
  const rest = TOTAL - lockedSum - v
  const next = [...cur]
  next[idx] = v
  if (free.length > 0) {
    const sumFree = free.reduce((s, i) => s + cur[i], 0)
    const raw = free.map((i) => (sumFree > 0 ? (rest * cur[i]) / sumFree : rest / free.length))
    const floors = raw.map((x) => Math.floor(x))
    let left = rest - floors.reduce((s, x) => s + x, 0)
    raw
      .map((x, k) => [k, x - Math.floor(x)] as const)
      .sort((a, b) => b[1] - a[1])
      .forEach(([k]) => {
        if (left > 0) {
          floors[k] += 1
          left -= 1
        }
      })
    free.forEach((i, k) => {
      next[i] = floors[k]
    })
  }
  return next
}

function fmtPct(units: number, decimal: string): string {
  return (units / 100).toFixed(2).replace(/0+$/, '').replace(/\.$/, '').replace('.', decimal)
}

function numVal(values: Record<string, string>, key: string, fallback: number): number {
  const n = Number.parseFloat(values[key] ?? '')
  return Number.isFinite(n) ? n : fallback
}

type VerdictKind = 'ok' | 'warn' | 'bad'

const VERDICT_STYLE: Record<VerdictKind, { badge: string; bar: string }> = {
  ok: { badge: 'bg-emerald-500/15 text-emerald-400', bar: 'bg-emerald-500' },
  warn: { badge: 'bg-amber-500/15 text-amber-400', bar: 'bg-amber-500' },
  bad: { badge: 'bg-red-500/15 text-red-400', bar: 'bg-red-500' },
}

/** Шкала «сколько вернулось» растянута до 150%, чтобы риска перебора было видно. */
const BAR_MAX = 1.5

export interface AdminFortuneChancesEditorProps {
  /** Значения полей группы (черновик поверх сохранённого). */
  values: Record<string, string>
  disabled?: boolean
  onChange: (key: string, value: string) => void
}

/**
 * Шансы призов колеса одним «пирогом» на 100%: ползунок одного приза двигает остальные,
 * замочек фиксирует приз. Размер приза лежит в той же карточке.
 * В настройках остаются прежние ключи FORTUNE_WEIGHT_*, но теперь это доли из 10 000.
 */
export function AdminFortuneChancesEditor({ values, disabled, onChange }: AdminFortuneChancesEditorProps) {
  const { t, i18n } = useTranslation()
  const decimal = i18n.language?.startsWith('en') ? '.' : ','
  // «+180 дней» не выпадает по задумке: замок на нём неснимаемый.
  const [locks, setLocks] = useState<Set<number>>(
    () => new Set([PRIZES.findIndex((p) => p.rewardType === NEVER_DROPS)]),
  )
  const [typing, setTyping] = useState<{ idx: number; text: string } | null>(null)
  const [simResult, setSimResult] = useState<{
    paidSpins: number
    totalSpins: number
    days: number
    counts: number[]
  } | null>(null)

  const units = useMemo(() => weightsToUnits(values), [values])

  const writeUnits = (next: number[]) => {
    next.forEach((u, i) => {
      const key = PRIZES[i].weightKey
      if ((values[key] ?? '') !== String(u)) onChange(key, String(u))
    })
  }

  const setPrize = (idx: number, target: number) => {
    writeUnits(moveUnits(units, idx, target, locks))
  }

  const toggleLock = (idx: number) => {
    if (PRIZES[idx].rewardType === NEVER_DROPS) return
    setLocks((prev) => {
      const next = new Set(prev)
      if (next.has(idx)) next.delete(idx)
      else next.add(idx)
      return next
    })
  }

  /** Стандартные шансы и размеры призов. Только в черновик — в силу вступит после «Сохранить». */
  const resetToDefaults = () => {
    PRIZES.forEach((p) => {
      onChange(p.weightKey, String(p.defaultWeight * (TOTAL / 1000)))
      p.sizeKeys.forEach((k, j) => onChange(k, String(p.defaultSizes[j])))
    })
    setLocks(new Set([PRIZES.findIndex((p) => p.rewardType === NEVER_DROPS)]))
    setTyping(null)
  }

  const commitTyping = () => {
    if (!typing) return
    const n = Number.parseFloat(typing.text.replace(',', '.'))
    if (Number.isFinite(n)) setPrize(typing.idx, n * 100)
    setTyping(null)
  }

  const sizeOf = (p: PrizeDef, k = 0): number => numVal(values, p.sizeKeys[k], 0)

  const prizeName = (p: PrizeDef): string =>
    t(`admin.settings.fortune.chances.name.${p.nameKey}`, { n: sizeOf(p) })

  const oneIn = (u: number): string => {
    if (u <= 0) return t('admin.settings.fortune.chances.never')
    if (u >= TOTAL / 2) return t('admin.settings.fortune.chances.half')
    return t('admin.settings.fortune.chances.oneIn', { n: Math.round(TOTAL / u) })
  }

  const unitLabel = (u: SizeUnit): string => t(`admin.settings.fortune.chances.unit.${u}`)
  const fmtNum = (x: number) => (Math.round(x * 100) / 100).toString().replace('.', decimal)

  // Выгодность. Приз «бесплатный спин» даёт ещё спины без оплаты: каждый платный спин
  // в среднем превращается в 1 / (1 − f) спинов, где f — ожидаемое число бесплатных за спин.
  const cost = Math.max(1, numVal(values, 'FORTUNE_SPIN_COST_DAYS', 1))
  let daysPerSpin = 0
  let freePerSpin = 0
  let chanceDaysOrSpin = 0
  PRIZES.forEach((p, i) => {
    const share = units[i] / TOTAL
    if (p.unit === 'days') {
      daysPerSpin += share * sizeOf(p)
      chanceDaysOrSpin += share
    } else if (p.unit === 'spin') {
      freePerSpin += share * Math.max(1, sizeOf(p))
      chanceDaysOrSpin += share
    }
  })
  const back = freePerSpin < 1 ? daysPerSpin / (1 - freePerSpin) : Number.POSITIVE_INFINITY
  const backPct = Math.round((back / cost) * 100)
  const verdictKind: VerdictKind = back >= cost ? 'bad' : back >= cost * 0.6 ? 'warn' : 'ok'
  const vs = VERDICT_STYLE[verdictKind]
  const barFill = Math.min(back / cost, BAR_MAX) / BAR_MAX

  const runSim = () => {
    const paidSpins = 1000
    const counts = PRIZES.map(() => 0)
    let left = paidSpins
    let played = 0
    let days = 0
    while (left > 0 && played < SIM_SPIN_CAP) {
      left -= 1
      played += 1
      let r = Math.random() * TOTAL
      for (let i = 0; i < PRIZES.length; i++) {
        r -= units[i]
        if (r < 0) {
          counts[i] += 1
          if (PRIZES[i].unit === 'days') days += sizeOf(PRIZES[i])
          if (PRIZES[i].unit === 'spin') left += Math.max(1, sizeOf(PRIZES[i]))
          break
        }
      }
    }
    setSimResult({ paidSpins, totalSpins: played, days, counts })
  }

  const numberInput = (key: string, label: string) => (
    <input
      type="number"
      min={0}
      step={1}
      aria-label={label}
      disabled={disabled}
      value={values[key] ?? ''}
      onChange={(e) => onChange(key, e.target.value)}
      className={surface('well', 'h-8 w-16 rounded-lg px-2 text-center text-sm font-semibold tabular-nums text-foreground outline-none focus:border-primary')}
    />
  )

  const net = simResult ? simResult.paidSpins * cost - simResult.days : 0

  return (
    <div className="space-y-3 py-3">
      <div className="grid grid-cols-1 gap-2.5 lg:grid-cols-2">
        {PRIZES.map((p, i) => {
          const u = units[i]
          const color = wheelSectorRimSolid(p.rewardType, i)
          const isLocked = locks.has(i)
          const lockFixed = p.rewardType === NEVER_DROPS
          const shown = typing?.idx === i ? typing.text : fmtPct(u, decimal)
          const name = prizeName(p)
          const PrizeIcon = FORTUNE_SECTOR_ICONS[p.rewardType] ?? Gift
          const lockLabel = t(
            lockFixed
              ? 'admin.settings.fortune.chances.lockFixed'
              : isLocked
                ? 'admin.settings.fortune.chances.unlock'
                : 'admin.settings.fortune.chances.lock',
          )
          return (
            <div
              key={p.weightKey}
              className={cn(
                surface('raised', 'rounded-xl p-3.5 transition-opacity'),
                u <= 0 && 'opacity-70',
                isLocked && 'border-primary/50',
              )}
              style={{ ['--chance-color' as string]: color }}
            >
              <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
                <div className="flex min-w-[10rem] flex-1 items-center gap-3">
                  <span
                    className="flex size-10 shrink-0 items-center justify-center rounded-xl"
                    style={{ background: 'color-mix(in srgb, var(--chance-color) 20%, transparent)' }}
                    aria-hidden
                  >
                    <PrizeIcon className={cn('size-5', sectorIconClassWheel(p.rewardType, 'porcelain'))} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold leading-tight text-foreground">{name}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">{oneIn(u)}</p>
                  </div>
                </div>
                <div className="ml-auto flex items-center gap-3">
                  <button
                    type="button"
                    disabled={disabled || lockFixed}
                    onClick={() => toggleLock(i)}
                    aria-pressed={isLocked}
                    aria-label={lockLabel}
                    title={lockLabel}
                    className={cn(
                      'flex size-9 shrink-0 items-center justify-center rounded-lg border transition-colors',
                      isLocked
                        ? 'border-primary bg-primary text-primary-foreground'
                        : 'border-border bg-background/40 text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {isLocked ? <Lock className="size-4" aria-hidden /> : <LockOpen className="size-4" aria-hidden />}
                  </button>
                  <div className="relative w-[104px] shrink-0">
                    <input
                      type="text"
                      inputMode="decimal"
                      aria-label={name}
                      disabled={disabled || isLocked}
                      value={shown}
                      onFocus={(e) => e.currentTarget.select()}
                      onChange={(e) => setTyping({ idx: i, text: e.target.value })}
                      onBlur={commitTyping}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') e.currentTarget.blur()
                      }}
                      className={surface('well', 'h-10 w-full rounded-lg pl-2 pr-7 text-right text-base font-bold tabular-nums text-foreground outline-none focus:border-primary')}
                    />
                    <span className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 font-semibold text-muted-foreground">
                      %
                    </span>
                  </div>
                </div>
              </div>

              <input
                type="range"
                min={0}
                max={TOTAL}
                step={1}
                value={u}
                disabled={disabled || isLocked}
                aria-label={name}
                onChange={(e) => setPrize(i, Number(e.target.value))}
                className="fortune-chance-range mt-3 w-full"
                style={{ ['--p' as string]: `${(u / TOTAL) * 100}%` }}
              />

              <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-dashed border-border/60 pt-2.5 text-xs text-muted-foreground">
                <span>{t('admin.settings.fortune.chances.prize')}</span>
                {p.sizeKind === 'range' ? (
                  <>
                    <span>{t('admin.settings.fortune.chances.from')}</span>
                    {numberInput(p.sizeKeys[0], `${name}: ${t('admin.settings.fortune.chances.from')}`)}
                    <span>{t('admin.settings.fortune.chances.to')}</span>
                    {numberInput(p.sizeKeys[1], `${name}: ${t('admin.settings.fortune.chances.to')}`)}
                  </>
                ) : (
                  numberInput(p.sizeKeys[0], name)
                )}
                <span>{unitLabel(p.unit)}</span>
              </div>
            </div>
          )
        })}
        <button
          type="button"
          disabled={disabled}
          onClick={resetToDefaults}
          className="flex min-h-12 items-center justify-center gap-2 rounded-xl border border-dashed border-border px-4 py-3 text-sm font-medium text-muted-foreground transition-colors hover:border-primary/60 hover:text-foreground disabled:opacity-50"
        >
          <RotateCcw className="size-4" aria-hidden />
          {t('admin.settings.fortune.chances.reset')}
        </button>
      </div>

      <div className={surface('raised', 'rounded-xl p-3.5')}>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-sm font-semibold text-foreground">{t('admin.settings.fortune.chances.profit.title')}</p>
          <span className={cn('rounded-full px-2.5 py-1 text-xs font-semibold', vs.badge)}>
            {t(`admin.settings.fortune.chances.profit.badge.${verdictKind}`)}
          </span>
        </div>
        <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
          {t('admin.settings.fortune.chances.profit.intro', { cost })}
        </p>

        <div className="mt-3 grid grid-cols-3 gap-2">
          <div className={surface('well', 'rounded-lg px-3 py-2.5')}>
            <p className="text-[11px] leading-tight text-muted-foreground">
              {t('admin.settings.fortune.chances.profit.spend')}
            </p>
            <p className="mt-1 text-base font-bold tabular-nums text-foreground sm:text-lg">
              {fmtNum(cost)} <span className="text-xs font-medium text-muted-foreground">{unitLabel('days')}</span>
            </p>
          </div>
          <div className={surface('well', 'rounded-lg px-3 py-2.5')}>
            <p className="text-[11px] leading-tight text-muted-foreground">
              {t('admin.settings.fortune.chances.profit.back')}
            </p>
            <p className="mt-1 text-base font-bold tabular-nums text-foreground sm:text-lg">
              {Number.isFinite(back) ? fmtNum(back) : '∞'}{' '}
              <span className="text-xs font-medium text-muted-foreground">{unitLabel('days')}</span>
            </p>
          </div>
          <div className={surface('well', 'rounded-lg px-3 py-2.5')}>
            <p className="text-[11px] leading-tight text-muted-foreground">
              {t('admin.settings.fortune.chances.profit.chance')}
            </p>
            <p className="mt-1 text-base font-bold tabular-nums text-foreground sm:text-lg">
              {fmtNum(chanceDaysOrSpin * 100)}%
            </p>
          </div>
        </div>

        <div className="mt-3.5">
          <div className="relative h-2 rounded-full bg-border">
            <div
              className={cn('h-full rounded-full transition-all duration-300', vs.bar)}
              style={{ width: `${barFill * 100}%` }}
            />
            <div
              className="absolute -top-1 h-4 w-0.5 rounded bg-foreground/70"
              style={{ left: `${(1 / BAR_MAX) * 100}%` }}
              aria-hidden
            />
          </div>
          <div className="mt-1.5 flex items-center justify-between gap-3 text-xs text-muted-foreground">
            <span>
              {Number.isFinite(back)
                ? t('admin.settings.fortune.chances.profit.returns', { pct: backPct })
                : t('admin.settings.fortune.chances.profit.returnsInf')}
            </span>
            <span>{t('admin.settings.fortune.chances.profit.breakEven')}</span>
          </div>
        </div>

        <div className="mt-3.5 border-t border-border/60 pt-3">
          <button
            type="button"
            onClick={runSim}
            disabled={disabled}
            className={surface('well', 'rounded-lg px-3 py-2 text-sm font-medium text-foreground transition-colors hover:border-primary/60')}
          >
            {t('admin.settings.fortune.chances.profit.sim')}
          </button>
          {simResult && (
            <div className="mt-3 space-y-2.5">
              <div className="grid grid-cols-3 gap-2">
                <div className={surface('well', 'rounded-lg px-3 py-2')}>
                  <p className="text-[11px] leading-tight text-muted-foreground">
                    {t('admin.settings.fortune.chances.profit.simSpins')}
                  </p>
                  <p className="mt-1 text-base font-bold tabular-nums text-foreground">{simResult.totalSpins}</p>
                  {simResult.totalSpins > simResult.paidSpins && (
                    <p className="text-[11px] text-muted-foreground">
                      {t('admin.settings.fortune.chances.profit.simFree', {
                        n: simResult.totalSpins - simResult.paidSpins,
                      })}
                    </p>
                  )}
                </div>
                <div className={surface('well', 'rounded-lg px-3 py-2')}>
                  <p className="text-[11px] leading-tight text-muted-foreground">
                    {t('admin.settings.fortune.chances.profit.simWon')}
                  </p>
                  <p className="mt-1 text-base font-bold tabular-nums text-foreground">{simResult.days}</p>
                  <p className="text-[11px] text-muted-foreground">
                    {t('admin.settings.fortune.chances.profit.simPaid', { n: simResult.paidSpins * cost })}
                  </p>
                </div>
                <div className={surface('well', 'rounded-lg px-3 py-2')}>
                  <p className="text-[11px] leading-tight text-muted-foreground">
                    {t('admin.settings.fortune.chances.profit.simNet')}
                  </p>
                  <p
                    className={cn(
                      'mt-1 text-base font-bold tabular-nums',
                      net >= 0 ? 'text-emerald-400' : 'text-red-400',
                    )}
                  >
                    {net >= 0 ? '+' : '−'}
                    {Math.abs(net)}
                  </p>
                  <p className="text-[11px] text-muted-foreground">
                    {t(
                      net >= 0
                        ? 'admin.settings.fortune.chances.profit.simNetGood'
                        : 'admin.settings.fortune.chances.profit.simNetBad',
                    )}
                  </p>
                </div>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {PRIZES.map((p, i) => ({ p, i, c: simResult.counts[i] }))
                  .filter((x) => x.c > 0)
                  .map((x) => (
                    <span
                      key={x.p.weightKey}
                      className={surface('well', 'inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs text-foreground')}
                    >
                      <span
                        className="size-2 rounded-full"
                        style={{ background: wheelSectorRimSolid(x.p.rewardType, x.i) }}
                        aria-hidden
                      />
                      {prizeName(x.p)}
                      <span className="font-bold tabular-nums">{x.c}</span>
                    </span>
                  ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
