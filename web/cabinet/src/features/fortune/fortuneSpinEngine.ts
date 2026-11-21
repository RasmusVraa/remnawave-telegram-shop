import { sectorIndexUnderPointer } from '@/features/fortune/fortuneWheelGeometry'

/** Откат колеса назад перед стартом (замах), градусы. */
const WINDUP_DEG = 7
const WINDUP_MS = 320
/** Не чаще одного тика в этот интервал: на разгоне секторы мелькают десятками в секунду. */
const MIN_TICK_GAP_MS = 28
/** Длительность «доводки» до финальной позиции при пропуске анимации. */
const SKIP_MS = 450

/** Параметры пружины стрелки (рад, с). */
const POINTER_STIFFNESS = 900
const POINTER_DAMPING = 26
const POINTER_MIN = -0.4
const POINTER_MAX = 0.06

export type WheelSpinOptions = {
  /** Диск колеса: на каждом кадре получает `transform: rotate()`. */
  disk: HTMLElement
  /** Стрелка: отклоняется на пружине при проходе каждого сектора (`transform-origin` — верх по центру). */
  pointer: HTMLElement | null
  from: number
  to: number
  sectorCount: number
  durationMs: number
  /** Сменился сектор под стрелкой. */
  onSector?: (index: number) => void
  /**
   * Колесо прошло границу сектора (не чаще раза в 28 мс).
   * `slow` — 0 на полной скорости, 1 когда колесо почти встало: звук делает тики выше, а удар по стрелке сильнее.
   */
  onTick?: (slow: number) => void
  /** Колесо встало на призе. Стрелка ещё доколебается сама. */
  onLand: () => void
}

export type WheelSpinHandle = {
  /** Мгновенно довести колесо до результата (за ~0,45 с). */
  skip: () => void
  /** Остановить без `onLand` (уход со страницы). */
  cancel: () => void
}

const easeInOutSine = (u: number) => 0.5 - Math.cos(Math.PI * u) / 2
/** Круче кубической: колесо дольше ползёт в хвосте, а не вязнет сразу после разгона. */
const easeOutQuint = (u: number) => 1 - Math.pow(1 - u, 5)
const easeOutCubic = (u: number) => 1 - Math.pow(1 - u, 3)

/**
 * Покадровая анимация колеса: замах назад → торможение по `easeOutQuint`, тики на границах секторов и пружинная стрелка.
 * Вращение пишется прямо в DOM, а не через CSS transition, чтобы звук, вибрация и стрелка шли по реальному углу.
 */
export function startWheelSpin(opts: WheelSpinOptions): WheelSpinHandle {
  const { disk, pointer, from, to, sectorCount, durationMs } = opts
  const windupTo = from - WINDUP_DEG
  const startedAt = performance.now()

  let rafId = 0
  let alive = true
  let landed = false
  let skipAt = 0
  let skipFrom = 0
  let lastCell = sectorIndexUnderPointer(from, sectorCount)
  let lastTickAt = 0
  let lastFrameAt = startedAt
  let rot = from
  let theta = 0
  let omega = 0

  const rotationAt = (now: number): number => {
    if (skipAt) {
      const u = Math.min(1, (now - skipAt) / SKIP_MS)
      return skipFrom + (to - skipFrom) * easeOutCubic(u)
    }
    const el = now - startedAt
    if (el < WINDUP_MS) return from + (windupTo - from) * easeInOutSine(el / WINDUP_MS)
    const u = Math.min(1, (el - WINDUP_MS) / durationMs)
    return windupTo + (to - windupTo) * easeOutQuint(u)
  }

  const finishedAt = (now: number): boolean =>
    skipAt ? now - skipAt >= SKIP_MS : now - startedAt >= WINDUP_MS + durationMs

  const frame = (now: number) => {
    if (!alive) return
    const dt = Math.min(0.05, Math.max(0.001, (now - lastFrameAt) / 1000))
    lastFrameAt = now

    if (!landed) {
      const prev = rot
      rot = rotationAt(now)
      const speed = Math.abs(rot - prev) / dt
      const cell = sectorIndexUnderPointer(rot, sectorCount)
      if (cell !== lastCell) {
        lastCell = cell
        opts.onSector?.(cell)
        if (now - lastTickAt >= MIN_TICK_GAP_MS) {
          lastTickAt = now
          const slow = 1 - Math.min(1, speed / 900)
          opts.onTick?.(slow)
          // Чем медленнее колесо, тем сильнее и заметнее удар по стрелке.
          omega -= 5 + slow * 11
        }
      }
      if (finishedAt(now)) {
        landed = true
        rot = to
        opts.onLand()
      }
      disk.style.transform = `rotate(${rot}deg)`
    }

    // Пружина стрелки считается и после остановки колеса, пока она не затихнет.
    omega += (-POINTER_STIFFNESS * theta - POINTER_DAMPING * omega) * dt
    theta = Math.min(POINTER_MAX, Math.max(POINTER_MIN, theta + omega * dt))
    if (pointer) pointer.style.transform = `rotate(${theta}rad)`

    if (landed && Math.abs(theta) < 0.002 && Math.abs(omega) < 0.05) {
      if (pointer) pointer.style.transform = ''
      alive = false
      return
    }
    rafId = requestAnimationFrame(frame)
  }
  rafId = requestAnimationFrame(frame)

  return {
    skip: () => {
      if (!alive || landed || skipAt) return
      skipAt = performance.now()
      skipFrom = rot
    },
    cancel: () => {
      alive = false
      cancelAnimationFrame(rafId)
      if (pointer) pointer.style.transform = ''
    },
  }
}
