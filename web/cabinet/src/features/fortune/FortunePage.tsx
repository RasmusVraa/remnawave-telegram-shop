import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { motion, useAnimation } from 'framer-motion'
import { useTranslation } from 'react-i18next'
import { CalendarDays, CalendarMinus, Ticket, Volume2, VolumeX } from 'lucide-react'

import { AppLayout } from '@/components/AppLayout'
import { PageReveal, RevealItem } from '@/components/PageReveal'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { api, ApiError, type FortuneStatusResponse } from '@/lib/api'
import { hapticImpact, hapticSelection, hapticSuccess, useErrorHaptic } from '@/lib/haptic'
import {
  ensureFortuneAudio,
  playSpinAnticipation,
  playSpinError,
  playWinFanfare,
  scheduleSpinWheelSounds,
  winTier,
} from '@/features/fortune/fortuneWheelAudio'
import { FortunePossiblePrizes, FortunePrizeList } from '@/features/fortune/FortunePossiblePrizes'
import { FortunePrizeModal } from '@/features/fortune/FortunePrizeModal'
import { FortuneWinnersMarquee } from '@/features/fortune/fortuneWinnersFeed'
import {
  buildWheelConicGradient,
  type FortuneDesignVariant,
} from '@/features/fortune/fortunePrizeVisuals'
import { arrangeFortuneSlots, slotIndexOfSector } from '@/features/fortune/fortuneWheelLayout'
import {
  nextSpinRotation,
  sectorIndexUnderPointer,
  sectorCenterDeg,
  spinVisualEase,
} from '@/features/fortune/fortuneWheelGeometry'
import { FortuneWheelFace } from '@/features/fortune/FortuneWheelFace'
import { WheelWinAnimation } from '@/features/fortune/WheelWinAnimation'

const FORTUNE_SOUND_MUTED_KEY = 'cabinet-fortune-sound-muted'
/** Длительность прокрута колеса (CSS transition и звук тиков). */
const FORTUNE_SPIN_MS = 4200
/** Вспышка/конфетти и задержка модалки «что выиграли». */
const FORTUNE_WIN_CELEBRATION_MS = 1500

type SpinTweenSnapshot = {
  from: number
  to: number
  n: number
  t0: number
  sectorTypes: string[]
}

function readFortuneSoundMuted(): boolean {
  try {
    return window.localStorage.getItem(FORTUNE_SOUND_MUTED_KEY) === '1'
  } catch {
    return false
  }
}

/** Момент следующей полуночи UTC (мс с epoch). */
function nextUtcMidnightMs(): number {
  const n = new Date()
  return Date.UTC(n.getUTCFullYear(), n.getUTCMonth(), n.getUTCDate() + 1, 0, 0, 0, 0)
}

function useUtcMidnightCountdown(active: boolean): number {
  const [ms, setMs] = useState(0)
  useEffect(() => {
    if (!active) {
      setMs(0)
      return undefined
    }
    const tick = () => setMs(Math.max(0, nextUtcMidnightMs() - Date.now()))
    tick()
    const id = window.setInterval(tick, 1000)
    return () => window.clearInterval(id)
  }, [active])
  return ms
}

/** Заглушка колеса и списка призов, пока не пришёл status. */
function FortuneSkeleton() {
  return (
    <div className="space-y-4 md:space-y-6" aria-hidden>
      <Skeleton className="mx-auto aspect-square w-full max-w-[22rem] rounded-full" />
      <Skeleton className="mx-auto h-11 w-full max-w-[18rem] rounded-full" />
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <Skeleton key={i} className="h-16 w-full rounded-xl" />
        ))}
      </div>
    </div>
  )
}

function formatCountdownHms(
  ms: number,
  t: (key: string, opts: Record<string, string | number>) => string,
): string {
  const totalSec = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  return t('fortune.countdownHms', {
    h,
    m: String(m).padStart(2, '0'),
    s: String(s).padStart(2, '0'),
  })
}

export default function FortunePage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const spinSoundRef = useRef<{ clear: () => void } | null>(null)
  const [soundMuted, setSoundMuted] = useState(readFortuneSoundMuted)
  const soundMutedRef = useRef(soundMuted)
  soundMutedRef.current = soundMuted
  const [rotation, setRotation] = useState(0)
  const [spinning, setSpinning] = useState(false)
  const [lastResult, setLastResult] = useState<{ type: string; value: number; free: boolean } | null>(null)
  const [showPrizeModal, setShowPrizeModal] = useState(false)
  const [spinError, setSpinError] = useState<string | null>(null)
  useErrorHaptic(spinError)
  /** Иконка в центре колеса: приз под стрелкой во время/после спина; `null` — дефолтная звезда. */
  const [hubRewardType, setHubRewardType] = useState<string | null>(null)
  const spinTweenRef = useRef<SpinTweenSnapshot | null>(null)
  const lastWinnerRewardRef = useRef<string | null>(null)
  const [winnerIndex, setWinnerIndex] = useState(0)
  const [winCelebration, setWinCelebration] = useState(false)
  const designVariant: FortuneDesignVariant = 'classic'
  const wheelFxControls = useAnimation()
  /** Подсказка по метрикам в шапке: платные спины / дни подписки */
  const [statsHint, setStatsHint] = useState<null | 'paid' | 'subscription'>(null)
  const statsHintClusterRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    return () => {
      spinSoundRef.current?.clear()
      spinSoundRef.current = null
    }
  }, [])

  useEffect(() => {
    if (!spinning) return undefined
    const snap = spinTweenRef.current
    if (!snap || snap.n <= 0 || snap.sectorTypes.length === 0) return undefined

    let raf = 0
    let lastIdx: number | null = null
    let lastHapticAt = 0
    const tick = (now: number) => {
      const elapsed = now - snap.t0
      const u = Math.min(1, elapsed / FORTUNE_SPIN_MS)
      const eased = spinVisualEase(u)
      const currentR = snap.from + (snap.to - snap.from) * eased
      const idx = sectorIndexUnderPointer(currentR, snap.n)
      const rt = snap.sectorTypes[idx]
      if (rt) setHubRewardType(rt)
      // Трещотка: тик на каждый сектор под стрелкой. На разгоне секторы мелькают
      // десятками в секунду — не чаще раза в 70 мс, иначе вместо тиков сплошное жужжание.
      if (lastIdx !== null && idx !== lastIdx && now - lastHapticAt >= 70) {
        hapticSelection()
        lastHapticAt = now
      }
      lastIdx = idx
      if (u < 1) raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [spinning])

  const setMutedPersist = (muted: boolean) => {
    setSoundMuted(muted)
    try {
      window.localStorage.setItem(FORTUNE_SOUND_MUTED_KEY, muted ? '1' : '0')
    } catch {
      /* ignore */
    }
  }

  const { data: status, isLoading, error } = useQuery({
    queryKey: ['fortune', 'status'],
    queryFn: () => api.fortuneStatus(),
  })

  const winnerFeedEnabled = Boolean(status?.winner_feed?.enabled)

  const { data: recentWins } = useQuery({
    queryKey: ['fortune', 'recent-wins'],
    queryFn: () => api.fortuneRecentWins(),
    enabled: Boolean(status?.enabled && status?.panel_ready && winnerFeedEnabled),
    staleTime: 30_000,
  })

  const spinMut = useMutation({
    mutationFn: () => api.fortuneSpin(),
    onSuccess: async (res) => {
      setLastResult({ type: res.reward_type, value: res.reward_value, free: res.is_free_spin })
      setShowPrizeModal(false)
      const fresh = qc.getQueryData<FortuneStatusResponse>(['fortune', 'status'])
      const slots = arrangeFortuneSlots(fresh?.sectors ?? status?.sectors ?? [])
      const n = slots.length || 10
      // Приз определён сервером; сектор стоит не на своём `index`, поэтому целимся в его слот.
      const winnerSlot = slotIndexOfSector(slots, res.sector_index)
      const target = nextSpinRotation({
        currentRotation: rotation,
        sectorIndex: winnerSlot,
        sectorCount: n,
        fullSpins: 5,
      })
      const sectorTypes = slots.map((s) => s.reward_type)
      spinTweenRef.current = {
        from: rotation,
        to: target,
        n,
        t0: performance.now(),
        sectorTypes,
      }
      lastWinnerRewardRef.current = res.reward_type
      setWinnerIndex(winnerSlot)
      setSpinning(true)
      setRotation(target)

      spinSoundRef.current?.clear()
      if (!soundMutedRef.current) {
        void ensureFortuneAudio().then((ctx) => {
          if (!ctx) return
          spinSoundRef.current = scheduleSpinWheelSounds(
            ctx,
            FORTUNE_SPIN_MS,
            rotation,
            target,
            n,
            () => {
              playWinFanfare(ctx, res.reward_type)
            },
          )
        })
      }

      window.setTimeout(() => {
        setSpinning(false)
        setHubRewardType(lastWinnerRewardRef.current)
        setWinCelebration(true)
        // Вибрация выигрыша не зависит от звука: крупный приз — ещё и сильный толчок.
        if (winTier(res.reward_type) === 'large') {
          hapticImpact('heavy')
          window.setTimeout(hapticSuccess, 140)
        } else {
          hapticSuccess()
        }
        const nSafe = Math.max(1, n)
        const c = sectorCenterDeg(winnerSlot, nSafe)
        const finalPointer = -90 + (rotation + (target - rotation)) * -1 + 90
        const overshootDeg = Math.max(2.2, Math.min(6.5, Math.abs(((finalPointer - c) % 360) * 0.03)))
        void wheelFxControls.start({
          rotate: [0, overshootDeg, -overshootDeg * 0.44, 0],
          transition: { duration: 0.72, times: [0, 0.34, 0.7, 1], ease: [0.22, 1, 0.36, 1] },
        })
        void wheelFxControls.start({
          x: [0, -2, 2, -1, 1, 0],
          y: [0, 1, -1, 1, 0],
          transition: { duration: 0.5, ease: 'easeInOut' },
        })
        window.setTimeout(() => setWinCelebration(false), FORTUNE_WIN_CELEBRATION_MS)
        window.setTimeout(() => setShowPrizeModal(true), FORTUNE_WIN_CELEBRATION_MS)
        void qc.invalidateQueries({ queryKey: ['fortune', 'status'] })
        void qc.invalidateQueries({ queryKey: ['fortune', 'recent-wins'] })
        void qc.invalidateQueries({ queryKey: ['subscription'] })
        void qc.invalidateQueries({ queryKey: ['loyalty-dashboard'] })
        void qc.invalidateQueries({ queryKey: ['promocodes-state'] })
      }, FORTUNE_SPIN_MS)
    },
    onError: (e: unknown) => {
      if (!soundMutedRef.current) {
        void ensureFortuneAudio().then((ctx) => {
          if (ctx) playSpinError(ctx)
        })
      }
      if (e instanceof ApiError && e.status === 400) {
        try {
          const j = JSON.parse(e.body) as { code?: string; error?: string }
          const code = j.code ?? 'unknown'
          setSpinError(t(`fortune.reason.${code}`, { defaultValue: j.error ?? e.message }))
          return
        } catch {
          setSpinError(e.message)
          return
        }
      }
      setSpinError(t('common.error'))
    },
  })

  /** Сектора в порядке слотов на диске — вперемешку, а не лесенкой по ценности из API. */
  const slotSectors = useMemo(() => arrangeFortuneSlots(status?.sectors ?? []), [status?.sectors])
  const gradient = useMemo(
    () => buildWheelConicGradient(slotSectors, designVariant),
    [slotSectors, designVariant],
  )

  const reasonText = useMemo(() => {
    if (!status || status.can_spin) return null
    const c = status.reason_code ?? 'unknown'
    return t(`fortune.reason.${c}`, { defaultValue: c })
  }, [status, t])

  const canPress =
    Boolean(status?.enabled && status?.panel_ready && status?.can_spin && !spinning && !spinMut.isPending)

  const spinCostDays = Math.max(1, status?.spin_cost_days ?? 1)

  const spinButtonLabel = useMemo(() => {
    if (spinning || spinMut.isPending) return t('fortune.spinning')
    if (status?.daily_free_available || (status?.bonus_spins ?? 0) > 0) return t('fortune.spinButtonFree')
    return t('fortune.spinButtonPaid', { count: spinCostDays })
  }, [spinning, spinMut.isPending, status?.daily_free_available, status?.bonus_spins, spinCostDays, t])

  const rawRemainDays = status?.subscription_remain_days
  const subscriptionDaysLeft =
    status == null
      ? 0
      : typeof rawRemainDays === 'number' && rawRemainDays > 0
        ? rawRemainDays
        : Math.max(0, Math.ceil((status.subscription_remain_hours ?? 0) / 24))

  const paidSpinsLeft = useMemo(() => {
    if (!status?.enabled || !status.panel_ready) return 0
    const max = Math.max(0, status.max_spins_per_day ?? 0)
    const used = Math.max(0, status.spins_used_today ?? 0)
    return Math.max(0, max - used)
  }, [status?.enabled, status?.panel_ready, status?.max_spins_per_day, status?.spins_used_today])

  useEffect(() => {
    if (!statsHint) return undefined
    const onPointerDown = (e: PointerEvent) => {
      const root = statsHintClusterRef.current
      if (root && !root.contains(e.target as Node)) setStatsHint(null)
    }
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setStatsHint(null)
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown, true)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [statsHint])

  const showNextFreeTimer = Boolean(
    status?.enabled && status?.panel_ready && status?.daily_free_enabled && status?.daily_free_available === false,
  )
  const nextFreeCountdownMs = useUtcMidnightCountdown(showNextFreeTimer)

  const bonusSpins = status?.bonus_spins ?? 0
  const hasFreeSpin = Boolean(status?.daily_free_available) || bonusSpins > 0

  // Колесо ограничено ещё и высотой окна, чтобы целиком (с кнопкой в центре) помещаться без прокрутки:
  // 370px — шапка, заголовок, лента, отступы и нижняя панель на телефоне; на ПК нижней панели нет — 300px.
  // max(12rem, …) — на совсем низком окне колесо не схлопывается, страница просто прокручивается.
  const wheelWrapClassName =
    'relative mx-auto w-full max-w-[min(calc(100vw-1.5rem),500px,max(12rem,calc(100dvh-370px)))] sm:max-w-[min(100%,520px,max(12rem,calc(100dvh-370px)))] md:max-w-[min(100%,540px,max(16rem,calc(100dvh-300px)))]'

  // Плашка в самой кнопке: при бесплатных — билетик с числом выигранных попыток, при платных — цена
  // крутки в днях подписки (сколько платных осталось, видно в шапке).
  const showSpinPill = !spinning && !spinMut.isPending && (hasFreeSpin ? bonusSpins > 0 : paidSpinsLeft > 0)
  const spinPillHint = hasFreeSpin
    ? t('fortune.bonusSpinsHint', { count: bonusSpins })
    : t('fortune.subtitle', { count: spinCostDays })
  const spinPill = showSpinPill ? (
    <span
      className="inline-flex items-center gap-1 rounded-full bg-primary-foreground/20 px-2 py-0.5 text-xs font-semibold normal-case tabular-nums"
      title={spinPillHint}
      aria-label={spinPillHint}
    >
      {hasFreeSpin ? (
        <>
          <Ticket className="size-3.5" aria-hidden />
          {bonusSpins}
        </>
      ) : (
        <>
          <CalendarMinus className="size-3.5" aria-hidden />
          {t('fortune.spinCostPill', { count: spinCostDays })}
        </>
      )}
    </span>
  ) : null

  const startSpin = () => {
    setSpinError(null)
    setShowPrizeModal(false)
    if (!soundMutedRef.current) {
      void ensureFortuneAudio().then((ctx) => {
        if (ctx) playSpinAnticipation(ctx)
      })
    }
    spinMut.mutate()
  }

  // Кнопка в центре колеса (и ПК, и телефон). Лежит поверх вращающегося диска, а не внутри него, чтобы надпись не
  // крутилась вместе с колесом. Пока колесо крутится, кнопки нет — в центре видна иконка приза под стрелкой.
  // Нельзя крутить и есть таймер — кнопка гаснет и показывает, через сколько бесплатная попытка.
  const hubIdle = canPress || spinMut.isPending
  const hubButton = spinning ? null : (
    <button
      type="button"
      disabled={!canPress}
      onClick={startSpin}
      aria-label={spinButtonLabel}
      className={cn(
        'absolute left-1/2 top-1/2 z-30 flex aspect-square w-[31%] -translate-x-1/2 -translate-y-1/2 flex-col items-center justify-center gap-1 rounded-full text-center outline-none ring-offset-background focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 sm:gap-1.5',
        hubIdle
          ? 'fortune-hub-btn text-base font-bold uppercase tracking-wide text-primary-foreground sm:text-lg'
          : 'cursor-default bg-muted text-[11px] font-semibold leading-snug text-muted-foreground shadow-inner sm:text-xs',
        spinMut.isPending && 'fortune-hub-btn--busy',
      )}
    >
      {spinMut.isPending ? (
        <Spinner className="size-6" />
      ) : hubIdle ? (
        <>
          <span className="leading-none">{t('fortune.hubSpin')}</span>
          {hasFreeSpin && (
            <span className="-mt-0.5 text-[10px] font-bold leading-none tracking-wider text-primary-foreground/90 sm:text-xs">
              {t('fortune.hubFree')}
            </span>
          )}
          {spinPill}
        </>
      ) : showNextFreeTimer ? (
        <>
          <span className="px-3">{t('fortune.nextFreeIn')}</span>
          <span className="text-sm font-bold tabular-nums text-foreground sm:text-base">
            {formatCountdownHms(nextFreeCountdownMs, t)}
          </span>
        </>
      ) : (
        t('fortune.hubSpin')
      )}
    </button>
  )

  const wheelColumn = (
    <div className={wheelWrapClassName}>
      <div
        className="pointer-events-none absolute -top-1 left-1/2 z-20 -translate-x-1/2 drop-shadow-md"
        aria-hidden
      >
        <div className="size-0 border-x-[14px] border-x-transparent border-t-[20px] border-t-primary" />
      </div>
      <motion.div className="relative aspect-square w-full overflow-hidden rounded-full" animate={wheelFxControls}>
        <FortuneWheelFace
          sectors={slotSectors}
          rotationDeg={rotation}
          spinning={spinning || spinMut.isPending}
          spinMs={FORTUNE_SPIN_MS}
          gradient={gradient}
          designVariant={designVariant}
          t={t}
          hubRewardType={hubRewardType}
        />
        <WheelWinAnimation
          active={winCelebration}
          prize={lastResult?.type ?? null}
          amount={lastResult?.value ?? 0}
          winnerIndex={winnerIndex}
          sectorCount={slotSectors.length}
          designVariant={designVariant}
        />
      </motion.div>
      {hubButton}
    </div>
  )

  // Телефон: под колесом только причина, почему крутить нельзя, и ошибка; таймер — в шапке.
  const spinPanel = (reasonText && !status?.can_spin) || spinError ? (
    <div className="mt-4 flex w-full flex-col items-center gap-2 md:hidden">
      {reasonText && !status?.can_spin && (
        <p className="text-center text-sm text-muted-foreground">{reasonText}</p>
      )}
      {spinError && <p className="text-center text-sm text-destructive">{spinError}</p>}
    </div>
  ) : null


  // ПК: справа от колеса — призы столбиком, под ними таймер до бесплатной (когда крутить всё же можно
  // платно; если нельзя совсем, таймер в центре колеса) и причины, почему крутить нельзя.
  const sideColumn = (
    <div className="hidden min-w-0 flex-col gap-3 md:flex">
      <FortunePrizeList sectors={slotSectors} t={t} />
      {showNextFreeTimer && canPress && (
        <p className="text-sm text-muted-foreground">
          <span className="mr-1.5">{t('fortune.nextFreeIn')}</span>
          <span className="font-semibold tabular-nums text-foreground">{formatCountdownHms(nextFreeCountdownMs, t)}</span>
        </p>
      )}
      {reasonText && !status?.can_spin && <p className="text-sm text-muted-foreground">{reasonText}</p>}
      {spinError && <p className="text-sm text-destructive">{spinError}</p>}
    </div>
  )

  return (
    <AppLayout>
      <PageReveal className="mx-auto w-full max-w-5xl space-y-4 md:space-y-6 pt-2 pb-[max(1rem,calc(5.75rem+env(safe-area-inset-bottom)))] md:px-0 md:pb-2">
        <RevealItem className="flex flex-col gap-2">
          <div className="flex min-w-0 flex-row items-start justify-between gap-2">
            <div className="min-w-0 flex-1">
              <h1 className="whitespace-nowrap text-xl font-semibold tracking-tight sm:text-3xl">{t('fortune.title')}</h1>
              <p className="mt-1 hidden text-sm text-muted-foreground sm:block">
                {t('fortune.subtitle', { count: spinCostDays })}
              </p>
            </div>
            {status?.enabled && status?.panel_ready && (
              <div className="flex shrink-0 flex-col items-end gap-1.5 sm:gap-2">
                <div className="flex flex-row items-center justify-end gap-1.5 sm:gap-2">
                  <div ref={statsHintClusterRef} className="relative">
                    <div
                      className="inline-flex min-h-8 items-center gap-1 rounded-lg border border-border/80 bg-card/60 px-1 py-0.5 text-xs tabular-nums sm:min-h-9 sm:px-1.5 sm:py-1 sm:text-sm shadow-sm backdrop-blur-sm dark:bg-card/40"
                      aria-label={t('fortune.statsCompactAria')}
                    >
                      <button
                        type="button"
                        className="inline-flex items-center gap-1 rounded-md px-1 py-0.5 text-foreground outline-none ring-offset-background transition-colors hover:bg-muted/70 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 dark:ring-offset-background"
                        aria-expanded={statsHint === 'paid'}
                        aria-controls="fortune-stats-hint"
                        onClick={() => setStatsHint((h) => (h === 'paid' ? null : 'paid'))}
                      >
                        <Ticket className="size-4 shrink-0 text-primary" aria-hidden />
                        <span className="font-semibold">{paidSpinsLeft}</span>
                      </button>
                      <span className="h-4 w-px shrink-0 bg-border" aria-hidden />
                      <button
                        type="button"
                        className="inline-flex items-center gap-1 rounded-md px-1 py-0.5 text-foreground outline-none ring-offset-background transition-colors hover:bg-muted/70 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 dark:ring-offset-background"
                        aria-expanded={statsHint === 'subscription'}
                        aria-controls="fortune-stats-hint"
                        onClick={() => setStatsHint((h) => (h === 'subscription' ? null : 'subscription'))}
                      >
                        <CalendarDays className="size-4 shrink-0 text-emerald-600 dark:text-emerald-400" aria-hidden />
                        <span className="font-semibold">
                          {subscriptionDaysLeft}
                          {t('fortune.daysShortSuffix')}
                        </span>
                      </button>
                    </div>
                    {statsHint && (
                      <div
                        id="fortune-stats-hint"
                        role="tooltip"
                        className="animate-fade-in absolute right-0 top-[calc(100%+6px)] z-50 max-w-[min(288px,calc(100vw-2rem))] rounded-lg border border-border bg-card px-3 py-2 text-left text-xs leading-snug text-card-foreground shadow-lg"
                      >
                        {statsHint === 'paid'
                          ? t('fortune.paidSpinsLeftHint')
                          : t('fortune.subscriptionDaysHint')}
                      </div>
                    )}
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    className="size-8 shrink-0 sm:size-9"
                    aria-pressed={soundMuted}
                    aria-label={soundMuted ? t('fortune.soundEnableHint') : t('fortune.soundDisableHint')}
                    title={soundMuted ? t('fortune.soundEnableHint') : t('fortune.soundDisableHint')}
                    onClick={() => {
                      const next = !soundMuted
                      hapticSelection()
                      setMutedPersist(next)
                      if (!next) {
                        void ensureFortuneAudio().catch(() => {})
                      }
                    }}
                  >
                    {soundMuted ? <VolumeX className="size-4" /> : <Volume2 className="size-4" />}
                  </Button>
                </div>
                {showNextFreeTimer && status?.can_spin && (
                  <p className="max-w-[11rem] text-right text-[11px] leading-snug text-muted-foreground sm:max-w-none sm:text-xs md:hidden">
                    <span className="mr-1.5 align-middle">{t('fortune.nextFreeIn')}</span>
                    <span className="inline-block align-middle font-semibold tabular-nums text-foreground">
                      {formatCountdownHms(nextFreeCountdownMs, t)}
                    </span>
                  </p>
                )}
              </div>
            )}
          </div>
        </RevealItem>

        {isLoading && (
          <RevealItem>
            <FortuneSkeleton />
          </RevealItem>
        )}
        {error && (
          <RevealItem>
            <p className="text-sm text-destructive">{t('common.error')}</p>
          </RevealItem>
        )}

        {status && !status.enabled && (
          <RevealItem>
            <Card>
              <CardContent className="py-6 text-sm text-muted-foreground">{t('fortune.moduleOff')}</CardContent>
            </Card>
          </RevealItem>
        )}

        {status?.enabled && !status.panel_ready && (
          <RevealItem>
            <Card>
              <CardContent className="py-6 text-sm text-muted-foreground">{t('fortune.noPanel')}</CardContent>
            </Card>
          </RevealItem>
        )}

        {status?.enabled && status.panel_ready && (
          <>
            {winnerFeedEnabled && (
              <RevealItem>
                <FortuneWinnersMarquee items={recentWins?.items ?? []} t={t} />
              </RevealItem>
            )}

            <RevealItem className="md:grid md:grid-cols-[minmax(0,1.5fr)_minmax(18rem,1fr)] md:items-center md:gap-8">
              {wheelColumn}
              {spinPanel}
              {sideColumn}
            </RevealItem>

            <RevealItem className="md:hidden">
              <FortunePossiblePrizes sectors={slotSectors} t={t} />
            </RevealItem>
          </>
        )}
      </PageReveal>

      {lastResult && (
        <FortunePrizeModal
          open={showPrizeModal}
          rewardType={lastResult.type}
          rewardValue={lastResult.value}
          wasFree={lastResult.free}
          onClaim={() => setShowPrizeModal(false)}
        />
      )}
    </AppLayout>
  )
}
