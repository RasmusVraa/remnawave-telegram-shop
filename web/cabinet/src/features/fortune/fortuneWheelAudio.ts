/**
 * Звуки колеса фортуны через Web Audio API (без внешних файлов).
 * Рассчитано на жест пользователя перед resume() контекста.
 */

let sharedCtx: AudioContext | null = null

export function getFortuneAudioContext(): AudioContext | null {
  if (typeof window === 'undefined') return null
  if (!sharedCtx) {
    const w = window as unknown as { AudioContext?: typeof AudioContext; webkitAudioContext?: typeof AudioContext }
    const Ctor = w.AudioContext || w.webkitAudioContext
    if (!Ctor) return null
    try {
      sharedCtx = new Ctor()
    } catch {
      return null
    }
  }
  return sharedCtx
}

export async function ensureFortuneAudio(): Promise<AudioContext | null> {
  const ctx = getFortuneAudioContext()
  if (!ctx) return null
  if (ctx.state === 'suspended') {
    try {
      await ctx.resume()
    } catch {
      return null
    }
  }
  return ctx
}

function now(ctx: AudioContext): number {
  return ctx.currentTime
}

function masterGain(ctx: AudioContext, when: number, peak: number, duration: number): GainNode {
  const g = ctx.createGain()
  g.gain.setValueAtTime(0, when)
  g.gain.linearRampToValueAtTime(peak, when + Math.min(0.02, duration * 0.1))
  g.gain.exponentialRampToValueAtTime(0.001, when + duration)
  g.connect(ctx.destination)
  return g
}

/** Короткий «рывок» при нажатии «Крутить» (до ответа сервера). */
export function playSpinAnticipation(ctx: AudioContext): void {
  const t0 = now(ctx)
  const dur = 0.18
  const g = masterGain(ctx, t0, 0.35, dur)
  const osc = ctx.createOscillator()
  osc.type = 'sine'
  osc.frequency.setValueAtTime(180, t0)
  osc.frequency.exponentialRampToValueAtTime(520, t0 + dur * 0.85)
  osc.connect(g)
  osc.start(t0)
  osc.stop(t0 + dur + 0.02)

  const noise = ctx.createBufferSource()
  const buf = ctx.createBuffer(1, Math.ceil(ctx.sampleRate * 0.06), ctx.sampleRate)
  const d = buf.getChannelData(0)
  for (let i = 0; i < d.length; i++) {
    d[i] = (Math.random() * 2 - 1) * (1 - i / d.length)
  }
  noise.buffer = buf
  const nf = ctx.createBiquadFilter()
  nf.type = 'bandpass'
  nf.frequency.value = 900
  nf.Q.value = 0.7
  const ng = ctx.createGain()
  ng.gain.setValueAtTime(0.12, t0)
  ng.gain.exponentialRampToValueAtTime(0.001, t0 + 0.07)
  noise.connect(nf)
  nf.connect(ng)
  ng.connect(ctx.destination)
  noise.start(t0)
  noise.stop(t0 + 0.08)
}

/** Один «щёлк» сектора у указателя (лёгкий / тяжёлый). */
function playTick(ctx: AudioContext, when: number, heavy: boolean): void {
  const peak = heavy ? 0.22 : 0.11
  const dur = heavy ? 0.045 : 0.028
  const g = masterGain(ctx, when, peak, dur)

  const osc = ctx.createOscillator()
  osc.type = 'triangle'
  osc.frequency.setValueAtTime(heavy ? 95 : 140, when)
  osc.frequency.exponentialRampToValueAtTime(heavy ? 55 : 85, when + dur)
  osc.connect(g)
  osc.start(when)
  osc.stop(when + dur + 0.01)

  const buf = ctx.createBuffer(1, Math.ceil(ctx.sampleRate * 0.02), ctx.sampleRate)
  const ch = buf.getChannelData(0)
  for (let i = 0; i < ch.length; i++) {
    ch[i] = (Math.random() * 2 - 1) * 0.5
  }
  const ns = ctx.createBufferSource()
  ns.buffer = buf
  const bp = ctx.createBiquadFilter()
  bp.type = 'highpass'
  bp.frequency.value = heavy ? 400 : 700
  const ng = ctx.createGain()
  ng.gain.setValueAtTime(heavy ? 0.08 : 0.05, when)
  ng.gain.exponentialRampToValueAtTime(0.001, when + 0.018)
  ns.connect(bp)
  bp.connect(ng)
  ng.connect(ctx.destination)
  ns.start(when)
  ns.stop(when + 0.022)
}

/** Низкий «стук» в момент остановки колеса. */
export function playWheelLand(ctx: AudioContext): void {
  const t0 = now(ctx)
  const g = ctx.createGain()
  g.gain.setValueAtTime(0.45, t0)
  g.gain.exponentialRampToValueAtTime(0.001, t0 + 0.35)
  g.connect(ctx.destination)

  const osc = ctx.createOscillator()
  osc.type = 'sine'
  osc.frequency.setValueAtTime(88, t0)
  osc.frequency.exponentialRampToValueAtTime(42, t0 + 0.28)
  osc.connect(g)
  osc.start(t0)
  osc.stop(t0 + 0.36)
}

export function winTier(rewardType: string): 'small' | 'medium' | 'large' {
  if (rewardType === 'micro') return 'small'
  if (rewardType === 'days_180' || rewardType === 'days_30' || rewardType === 'days_15') return 'large'
  return 'medium'
}

/** Фанфарный «приз» сразу после остановки. */
export function playWinFanfare(ctx: AudioContext, rewardType: string): void {
  const tier = winTier(rewardType)
  const t0 = now(ctx)
  const base = tier === 'large' ? 392 : tier === 'medium' ? 349.23 : 329.63 // G4, F4, E4
  const intervals =
    tier === 'large'
      ? [0, 0.07, 0.14, 0.22, 0.32, 0.42]
      : tier === 'medium'
        ? [0, 0.08, 0.16, 0.28]
        : [0, 0.1, 0.22]

  const freqs =
    tier === 'large'
      ? [base, base * 1.25, base * 1.5, base * 2, base * 2.5, base * 3]
      : tier === 'medium'
        ? [base, base * 1.2599, base * 1.4983, base * 2]
        : [base, base * 1.2599, base * 1.4983]

  for (let i = 0; i < intervals.length; i++) {
    const when = t0 + intervals[i]
    const f = freqs[Math.min(i, freqs.length - 1)]
    const peak = tier === 'large' ? 0.24 - i * 0.018 : tier === 'medium' ? 0.2 : 0.15
    const g = ctx.createGain()
    g.gain.setValueAtTime(0, when)
    g.gain.linearRampToValueAtTime(Math.max(0.08, peak), when + 0.02)
    g.gain.exponentialRampToValueAtTime(0.001, when + (tier === 'small' ? 0.22 : 0.32))
    g.connect(ctx.destination)

    const osc = ctx.createOscillator()
    osc.type = 'sine'
    osc.frequency.setValueAtTime(f, when)
    osc.connect(g)
    osc.start(when)
    osc.stop(when + 0.35)

    const osc2 = ctx.createOscillator()
    osc2.type = 'triangle'
    osc2.frequency.setValueAtTime(f * 2, when)
    const g2 = ctx.createGain()
    g2.gain.value = tier === 'large' ? 0.07 : 0.045
    osc2.connect(g2)
    g2.connect(g)
    osc2.start(when)
    osc2.stop(when + 0.25)
  }

  // Лёгкое «сверкание» на крупном призе
  if (tier === 'large') {
    const sh = t0 + 0.38
    for (let k = 0; k < 6; k++) {
      const w = sh + k * 0.045
      playTick(ctx, w, false)
    }
  }
}

/** Ошибка спина / сеть — короткий нейтральный тон вниз. */
export function playSpinError(ctx: AudioContext): void {
  const t0 = now(ctx)
  const g = masterGain(ctx, t0, 0.12, 0.25)
  const osc = ctx.createOscillator()
  osc.type = 'sine'
  osc.frequency.setValueAtTime(320, t0)
  osc.frequency.exponentialRampToValueAtTime(120, t0 + 0.22)
  osc.connect(g)
  osc.start(t0)
  osc.stop(t0 + 0.26)
}

export type SpinSoundSchedule = {
  clear: () => void
}

function cubicBezierEasing(x1: number, y1: number, x2: number, y2: number): (x: number) => number {
  // Inverts `x` for the cubic-bezier curve and returns `y`, like CSS `transition-timing-function`.
  // Enough for UI sync purposes.
  const cx = 3 * x1
  const bx = 3 * (x2 - x1) - cx
  const ax = 1 - cx - bx

  const cy = 3 * y1
  const by = 3 * (y2 - y1) - cy
  const ay = 1 - cy - by

  function sampleCurveX(t: number): number {
    return ((ax * t + bx) * t + cx) * t
  }
  function sampleCurveY(t: number): number {
    return ((ay * t + by) * t + cy) * t
  }
  function sampleDerivativeX(t: number): number {
    return (3 * ax * t + 2 * bx) * t + cx
  }

  function solveTforX(x: number): number {
    if (x <= 0) return 0
    if (x >= 1) return 1

    // Newton-Raphson iterations.
    let t = x
    for (let i = 0; i < 8; i++) {
      const xEst = sampleCurveX(t)
      const dx = xEst - x
      const d = sampleDerivativeX(t)
      if (Math.abs(dx) < 1e-5) return t
      if (Math.abs(d) < 1e-5) break
      t = t - dx / d
      t = Math.min(1, Math.max(0, t))
    }

    // Fallback: binary search.
    let lo = 0
    let hi = 1
    t = x
    for (let i = 0; i < 20; i++) {
      const xEst = sampleCurveX(t)
      if (Math.abs(xEst - x) < 1e-5) break
      if (xEst < x) lo = t
      else hi = t
      t = (hi + lo) / 2
    }
    return t
  }

  return (x: number) => {
    const t = solveTforX(x)
    return sampleCurveY(t)
  }
}

/**
 * Тики «прокрута» с нарастающим интервалом (быстро → медленно), синхронно с ~4.2s анимацией.
 * В конце вызывается onLanded (туда же положить fanfare).
 */
export function scheduleSpinWheelSounds(
  ctx: AudioContext,
  durationMs: number,
  spinFromRotationDeg: number,
  spinToRotationDeg: number,
  sectorCount: number,
  onLanded: () => void,
): SpinSoundSchedule {
  // Same timing function as in `FortuneWheelFace`.
  const ease = cubicBezierEasing(0.22, 0.61, 0.36, 1)
  const stepAngle = 360 / Math.max(sectorCount, 1)

  const delta = spinToRotationDeg - spinFromRotationDeg
  const totalTransitions = Math.max(1, Math.round(Math.abs(delta) / stepAngle))

  let rafId = 0
  let alive = true
  let lastSectorIdx: number | null = null
  let tickIdx = 0

  // Inline copy of `sectorIndexUnderPointer` logic to keep this file standalone.
  const sectorIndexUnderPointer = (rotationDeg: number): number => {
    const Ln = ((-rotationDeg % 360) + 360) % 360
    const step = 360 / Math.max(sectorCount, 1)
    for (let i = 0; i < sectorCount; i++) {
      const start = i * step
      const end = (i + 1) * step
      if (Ln >= start && Ln < end) return i
    }
    return 0
  }

  const t0Perf = performance.now()
  const tick = (perfNow: number) => {
    if (!alive) return

    const elapsedMs = perfNow - t0Perf
    const u = Math.min(1, elapsedMs / durationMs)
    const eased = ease(u)
    const currentR = spinFromRotationDeg + delta * eased

    const sectorIdx = sectorIndexUnderPointer(currentR)
    if (lastSectorIdx == null) {
      lastSectorIdx = sectorIdx
    } else if (sectorIdx !== lastSectorIdx) {
      const heavy = tickIdx >= totalTransitions - 4
      playTick(ctx, now(ctx), heavy)
      tickIdx++
      lastSectorIdx = sectorIdx
    }

    if (u < 1) {
      rafId = requestAnimationFrame(tick)
      return
    }

    // Один раз на конец.
    alive = false
    playWheelLand(ctx)
    onLanded()
  }

  rafId = requestAnimationFrame(tick)

  return {
    clear: () => {
      alive = false
      cancelAnimationFrame(rafId)
    },
  }
}
