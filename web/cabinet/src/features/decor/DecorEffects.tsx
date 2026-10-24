import { type CSSProperties, useEffect, useMemo, useState } from 'react'

import { cn } from '@/lib/utils'

/** ≥768px — крупные частицы только на ПК/планшете. */
function useIsDesktopViewport(): boolean {
  const [desktop, setDesktop] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(min-width: 768px)').matches,
  )
  useEffect(() => {
    const mq = window.matchMedia('(min-width: 768px)')
    const onChange = () => setDesktop(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])
  return desktop
}

function particleCount(mobileDivisor = 1): number {
  if (typeof window === 'undefined') return 32
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return 0
  const base = window.matchMedia('(max-width: 767px)').matches ? 18 : 36
  return Math.max(4, Math.floor(base / mobileDivisor))
}

function randomBetween(min: number, max: number): number {
  return min + Math.random() * (max - min)
}

interface ParticleStyle {
  left: string
  animationDuration: string
  animationDelay: string
  fontSize: string
  opacity: number
  char: string
}

function buildParticleStyles(
  count: number,
  charOrChars: string | string[],
  opts?: { fontMin?: number; fontMax?: number; delayMax?: number; durationMin?: number; durationMax?: number },
): ParticleStyle[] {
  const fontMin = opts?.fontMin ?? 12
  const fontMax = opts?.fontMax ?? 22
  const delayMax = opts?.delayMax ?? 12
  const durationMin = opts?.durationMin ?? 8
  const durationMax = opts?.durationMax ?? 18
  const chars = Array.isArray(charOrChars) ? charOrChars : [charOrChars]
  return Array.from({ length: count }, () => ({
    left: `${randomBetween(0, 100)}%`,
    animationDuration: `${randomBetween(durationMin, durationMax)}s`,
    animationDelay: `${randomBetween(0, delayMax)}s`,
    fontSize: `${randomBetween(fontMin, fontMax)}px`,
    opacity: randomBetween(0.3, 0.85),
    char: chars[Math.floor(Math.random() * chars.length)]!,
  }))
}

interface FloatingParticlesProps {
  char?: string
  chars?: readonly string[]
  particleClassName: string
  fxClassName?: string
  count?: number
  buildOpts?: Parameters<typeof buildParticleStyles>[2]
}

function FloatingParticles({ char, chars, particleClassName, fxClassName, count, buildOpts }: FloatingParticlesProps) {
  const n = count ?? particleCount()
  const styles = useMemo(() => {
    const glyph = chars ? [...chars] : char ? [char] : ['✦']
    return buildParticleStyles(n, glyph, buildOpts)
  }, [n, char, chars, buildOpts])
  if (styles.length === 0) return null

  return (
    <div className={cn('cabinet-decor-fx', fxClassName)} aria-hidden>
      {styles.map((s, i) => (
        <span
          key={i}
          className={cn('cabinet-decor-particle', particleClassName)}
          style={{
            left: s.left,
            animationDuration: s.animationDuration,
            animationDelay: s.animationDelay,
            fontSize: s.fontSize,
            opacity: s.opacity,
          }}
        >
          {s.char}
        </span>
      ))}
    </div>
  )
}

/** Шестилучевая снежинка-кристалл: лучи с веточками + шестиугольник в центре. */
function SnowCrystalSvg() {
  return (
    <svg viewBox="-12 -12 24 24" aria-hidden className="size-full overflow-visible">
      <g stroke="currentColor" strokeWidth="1.15" strokeLinecap="round" fill="none">
        {[0, 60, 120, 180, 240, 300].map((deg) => (
          <g key={deg} transform={`rotate(${deg})`}>
            <path d="M0 0 V-10.5" />
            <path d="M0 -6.5 L-2.6 -9 M0 -6.5 L2.6 -9" />
            <path d="M0 -3.4 L-1.8 -5.2 M0 -3.4 L1.8 -5.2" />
          </g>
        ))}
      </g>
      <circle r="1.6" fill="currentColor" />
    </svg>
  )
}

interface SnowFlakeStyle {
  left: string
  size: number
  animationDuration: string
  animationDelay: string
  swayDuration: string
  sway: number
  opacity: number
  /** 0 — дальний (мелкий, размытый, медленный), 2 — ближний. */
  depth: 0 | 1 | 2
}

function buildSnowFlakes(
  count: number,
  opts: { sizeMin: number; sizeMax: number; durationMin: number; durationMax: number },
): SnowFlakeStyle[] {
  return Array.from({ length: count }, () => {
    const t = Math.random()
    const depth = (t < 0.45 ? 0 : t < 0.85 ? 1 : 2) as SnowFlakeStyle['depth']
    const k = depth / 2
    // Ближние снежинки крупнее и падают быстрее — так снегопад обретает глубину.
    const size = opts.sizeMin + (opts.sizeMax - opts.sizeMin) * (k * 0.7 + Math.random() * 0.3)
    const duration = opts.durationMax - (opts.durationMax - opts.durationMin) * (k * 0.75 + Math.random() * 0.25)
    return {
      left: `${randomBetween(-2, 100)}%`,
      size,
      animationDuration: `${duration}s`,
      animationDelay: `-${randomBetween(0, duration)}s`,
      swayDuration: `${randomBetween(2.8, 5.5)}s`,
      sway: randomBetween(10, 34),
      opacity: randomBetween(0.55, 0.95) * (depth === 0 ? 0.6 : 1),
      depth,
    }
  })
}

/** new_year: многослойный снегопад из мягких хлопьев и редких кристаллов. */
export function SnowEffect() {
  const desktop = useIsDesktopViewport()
  const reduced =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches

  const flakes = useMemo(
    () =>
      reduced
        ? []
        : buildSnowFlakes(
            desktop ? 64 : 30,
            desktop
              ? { sizeMin: 2.5, sizeMax: 8, durationMin: 9, durationMax: 22 }
              : { sizeMin: 2, sizeMax: 6.5, durationMin: 8, durationMax: 18 },
          ),
    [desktop, reduced],
  )
  const crystals = useMemo(
    () =>
      reduced
        ? []
        : buildSnowFlakes(
            desktop ? 9 : 4,
            desktop
              ? { sizeMin: 14, sizeMax: 28, durationMin: 14, durationMax: 24 }
              : { sizeMin: 12, sizeMax: 20, durationMin: 12, durationMax: 20 },
          ),
    [desktop, reduced],
  )

  if (flakes.length + crystals.length === 0) return null

  const style = (s: SnowFlakeStyle) =>
    ({
      left: s.left,
      width: s.size,
      height: s.size,
      opacity: s.opacity,
      animationDuration: s.animationDuration,
      animationDelay: s.animationDelay,
      ['--ny-sway' as string]: `${s.sway}px`,
      ['--ny-sway-dur' as string]: s.swayDuration,
    }) as CSSProperties

  return (
    <div className="cabinet-decor-fx cabinet-decor-fx--snow" aria-hidden>
      {flakes.map((s, i) => (
        <span key={`f${i}`} className={cn('cabinet-ny-flake', `cabinet-ny-flake--d${s.depth}`)} style={style(s)}>
          <span className="cabinet-ny-flake__sway" />
        </span>
      ))}
      {crystals.map((s, i) => (
        <span key={`c${i}`} className="cabinet-ny-flake cabinet-ny-flake--crystal" style={style(s)}>
          <span className="cabinet-ny-flake__sway">
            <SnowCrystalSvg />
          </span>
        </span>
      ))}
    </div>
  )
}

function staggeredTiming(durationMin: number, durationMax: number): { animationDuration: string; animationDelay: string } {
  const sec = randomBetween(durationMin, durationMax)
  return {
    animationDuration: `${sec}s`,
    animationDelay: `-${randomBetween(0, sec)}s`,
  }
}

export function SunraysEffect() {
  const desktop = useIsDesktopViewport()
  const buildOpts = useMemo(
    () =>
      desktop
        ? { sizeMin: 22, sizeMax: 44, durationMin: 9, durationMax: 16 }
        : { sizeMin: 18, sizeMax: 34, durationMin: 8, durationMax: 14 },
    [desktop],
  )
  const n = particleCount(4)
  const styles = useMemo(() => buildSummerLeafStyles(n, buildOpts), [n, buildOpts])
  if (styles.length === 0) return null

  return (
    <div className="cabinet-decor-fx cabinet-decor-fx--summer" aria-hidden>
      {styles.map((s, i) => (
        <span
          key={i}
          className={cn(
            'cabinet-decor-particle cabinet-decor-particle--summer-leaf',
            s.variant === 'wide' && 'cabinet-decor-particle--summer-leaf-wide',
          )}
          style={{
            left: s.left,
            width: s.size,
            height: s.size,
            animationDuration: s.animationDuration,
            animationDelay: s.animationDelay,
            opacity: s.opacity,
            ['--summer-flip' as string]: String(s.flip),
          }}
        >
          <SummerLeafSvg variant={s.variant} />
        </span>
      ))}
    </div>
  )
}

type SummerLeafVariant = 'default' | 'wide' | 'autumn'

interface SummerLeafStyle {
  left: string
  size: number
  animationDuration: string
  animationDelay: string
  opacity: number
  variant: SummerLeafVariant
  flip: number
}

function buildSummerLeafStyles(
  count: number,
  opts?: { sizeMin?: number; sizeMax?: number; durationMin?: number; durationMax?: number },
): SummerLeafStyle[] {
  const sizeMin = opts?.sizeMin ?? 18
  const sizeMax = opts?.sizeMax ?? 34
  const durationMin = opts?.durationMin ?? 8
  const durationMax = opts?.durationMax ?? 14
  const variants: SummerLeafVariant[] = ['default', 'wide', 'autumn']
  return Array.from({ length: count }, () => {
    const timing = staggeredTiming(durationMin, durationMax)
    return {
      left: `${randomBetween(0, 94)}%`,
      size: randomBetween(sizeMin, sizeMax),
      ...timing,
      opacity: randomBetween(0.4, 0.82),
      variant: variants[Math.floor(Math.random() * variants.length)]!,
      flip: Math.random() > 0.5 ? -1 : 1,
    }
  })
}

function SummerLeafSvg({ variant }: { variant: SummerLeafVariant }) {
  if (variant === 'wide') {
    return (
      <svg viewBox="0 0 24 14" fill="currentColor" aria-hidden className="size-full">
        <path
          d="M2 9 C6 4 10 3 14 5 C18 7 20 9 22 8 C19 11 14 12 10 11 C6 10 4 10 2 9Z"
          opacity="0.9"
        />
        <path d="M12 6 V11" stroke="currentColor" strokeWidth="1" fill="none" opacity="0.45" />
      </svg>
    )
  }
  if (variant === 'autumn') {
    return (
      <svg viewBox="0 0 20 22" fill="currentColor" aria-hidden className="size-full">
        <path
          d="M10 2 C5 6 3 11 4 16 C6 14 8 13 10 13 C12 13 14 14 16 16 C17 11 15 6 10 2Z"
          opacity="0.88"
        />
        <path d="M10 13 V20" stroke="currentColor" strokeWidth="1.2" fill="none" opacity="0.5" />
      </svg>
    )
  }
  return (
    <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden className="size-full">
      <path
        d="M10 3 C6 7 4 11 5 15 C7 13 9 12 10 12 C11 12 13 13 15 15 C16 11 14 7 10 3Z"
        opacity="0.9"
      />
      <path d="M10 12 V17" stroke="currentColor" strokeWidth="1.2" fill="none" opacity="0.5" />
    </svg>
  )
}

/**
 * Тыква-светильник: рёбра, хвостик с листом и вырезанная рожица, которая
 * «горит» изнутри (мерцание — в CSS, .cabinet-hw-jack__face).
 */
export function JackOLanternShape() {
  return (
    <>
      <path d="M30 15 C30 11 29.5 7 28 4 C31 2.6 34 2.8 36 4 C34.5 7.5 34 11 34.5 15 Z" fill="hsl(95 32% 26%)" />
      <path d="M35 7 C39 3 45 3.5 48 7 C44 7.5 40 8.5 37 11" fill="hsl(105 38% 32%)" />
      <ellipse cx="19" cy="37" rx="15" ry="19" fill="hsl(22 86% 40%)" />
      <ellipse cx="45" cy="37" rx="15" ry="19" fill="hsl(22 86% 40%)" />
      <ellipse cx="25.5" cy="36" rx="13" ry="21" fill="hsl(26 92% 48%)" />
      <ellipse cx="38.5" cy="36" rx="13" ry="21" fill="hsl(26 92% 48%)" />
      <ellipse cx="32" cy="35.5" rx="10" ry="21.5" fill="hsl(29 96% 54%)" />
      <ellipse cx="26" cy="23" rx="3.5" ry="6" fill="hsl(36 100% 70% / 0.35)" />
      <g className="cabinet-hw-jack__face">
        <path d="M17 31 L24.5 24.5 L27 33 Z" />
        <path d="M47 31 L39.5 24.5 L37 33 Z" />
        <path d="M32 33 L29.5 38 H34.5 Z" />
        <path d="M14.5 41 C20 49 44 49 49.5 41 C46 42.5 44 43 42 43 L40 46 L37.5 43.5 H26.5 L24 46 L22 43 C20 43 18 42.5 14.5 41 Z" />
      </g>
    </>
  )
}

export function JackOLanternSvg({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 60" aria-hidden className={cn('size-full overflow-visible', className)}>
      <JackOLanternShape />
    </svg>
  )
}

function BatSvg() {
  return (
    <svg viewBox="0 0 64 32" fill="currentColor" aria-hidden className="size-full overflow-visible">
      <g className="cabinet-hw-bat__wing cabinet-hw-bat__wing--l">
        <path d="M30 11 C24 6 14 5 3 9 C7.5 11 9 14 8 18 C11 15.5 14 15.5 16 19 C18 15.5 21 15.5 23 19 C25 15.5 28 15 30 16 Z" />
      </g>
      <g className="cabinet-hw-bat__wing cabinet-hw-bat__wing--r">
        <path d="M34 11 C40 6 50 5 61 9 C56.5 11 55 14 56 18 C53 15.5 50 15.5 48 19 C46 15.5 43 15.5 41 19 C39 15.5 36 15 34 16 Z" />
      </g>
      <ellipse cx="32" cy="14.5" rx="3.6" ry="6" />
      <path d="M29.2 9.5 L29.6 4.5 L31.3 7.6 H32.7 L34.4 4.5 L34.8 9.5 Z" />
    </svg>
  )
}

function GhostSvg() {
  return (
    <svg viewBox="0 0 40 48" aria-hidden className="size-full overflow-visible">
      <path
        className="cabinet-hw-ghost__body"
        d="M20 2 C10 2 4 10 4 20 V44 L9 40 L14 45 L20 40 L26 45 L31 40 L36 44 V20 C36 10 30 2 20 2 Z"
      />
      <ellipse cx="14.5" cy="19" rx="2.6" ry="3.6" className="cabinet-hw-ghost__eye" />
      <ellipse cx="25.5" cy="19" rx="2.6" ry="3.6" className="cabinet-hw-ghost__eye" />
      <ellipse cx="20" cy="28" rx="3" ry="4" className="cabinet-hw-ghost__eye" />
    </svg>
  )
}

/** Паук на нити: тело + 8 лапок. Цвет — currentColor. */
export function SpiderSvg() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden className="size-full overflow-visible">
      <g stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" fill="none">
        <path d="M9 11 L4 7 L2 9" />
        <path d="M8.5 13 L3 12 L1.5 14.5" />
        <path d="M8.5 15 L3.5 17 L3 20" />
        <path d="M9.5 17 L6 21 L6.5 23" />
        <path d="M15 11 L20 7 L22 9" />
        <path d="M15.5 13 L21 12 L22.5 14.5" />
        <path d="M15.5 15 L20.5 17 L21 20" />
        <path d="M14.5 17 L18 21 L17.5 23" />
      </g>
      <ellipse cx="12" cy="15" rx="4" ry="5" />
      <circle cx="12" cy="8.6" r="2.8" />
      <circle cx="10.9" cy="8.4" r="0.7" fill="hsl(28 100% 60%)" />
      <circle cx="13.1" cy="8.4" r="0.7" fill="hsl(28 100% 60%)" />
    </svg>
  )
}

interface HalloweenSprite {
  left: string
  top: string
  size: number
  animationDuration: string
  animationDelay: string
  tilt: number
  /** false — спрайт лежит под колонкой контента (поля нет). */
  inGutter: boolean
}

/** Ширина колонки контента кабинета (max-w-5xl + внутренние отступы). */
const CONTENT_WIDTH_PX = 1100

/**
 * Если по бокам от контента есть поле, тыквы держим в нём, чтобы не
 * выглядывали из-за карточек; иначе (телефон, узкий ПК) — по всей ширине.
 */
function buildLanternStyles(
  count: number,
  opts: { sizeMin: number; sizeMax: number; durationMin: number; durationMax: number },
): HalloweenSprite[] {
  const vw = typeof window === 'undefined' ? 0 : window.innerWidth
  const gutter = (vw - CONTENT_WIDTH_PX) / 2
  return Array.from({ length: count }, (_, i) => {
    const timing = staggeredTiming(opts.durationMin, opts.durationMax)
    const size = randomBetween(opts.sizeMin, opts.sizeMax)
    let left: string
    const inGutter = gutter >= opts.sizeMax + 24
    if (inGutter) {
      const x = randomBetween(8, gutter - size - 8)
      left = `${i % 2 === 0 ? x : vw - x - size}px`
    } else {
      left = `${randomBetween(2, 80)}%`
    }
    return {
      left,
      top: `${randomBetween(14, 82)}%`,
      size,
      ...timing,
      tilt: randomBetween(-9, 9),
      inGutter,
    }
  })
}

function buildFlyerStyles(
  count: number,
  opts: { sizeMin: number; sizeMax: number; durationMin: number; durationMax: number; topMin: number; topMax: number },
): HalloweenSprite[] {
  return Array.from({ length: count }, () => ({
    left: '0',
    top: `${randomBetween(opts.topMin, opts.topMax)}%`,
    size: randomBetween(opts.sizeMin, opts.sizeMax),
    ...staggeredTiming(opts.durationMin, opts.durationMax),
    tilt: 0,
    inGutter: false,
  }))
}

function spriteStyle(s: HalloweenSprite, extra?: CSSProperties): CSSProperties {
  return {
    left: s.left,
    top: s.top,
    width: s.size,
    height: s.size,
    animationDuration: s.animationDuration,
    animationDelay: s.animationDelay,
    ...extra,
  }
}

/** halloween: парящие тыквы-светильники, летучие мыши и пара призраков. */
export function PumpkinsEffect() {
  const desktop = useIsDesktopViewport()
  const reduced =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches

  const lanterns = useMemo(
    () =>
      reduced
        ? []
        : buildLanternStyles(
            desktop ? 6 : 2,
            desktop
              ? { sizeMin: 46, sizeMax: 72, durationMin: 11, durationMax: 17 }
              : { sizeMin: 30, sizeMax: 42, durationMin: 10, durationMax: 15 },
          ),
    [desktop, reduced],
  )
  const bats = useMemo(
    () =>
      reduced
        ? []
        : buildFlyerStyles(
            desktop ? 4 : 2,
            desktop
              ? { sizeMin: 30, sizeMax: 54, durationMin: 13, durationMax: 22, topMin: 4, topMax: 46 }
              : { sizeMin: 24, sizeMax: 36, durationMin: 11, durationMax: 17, topMin: 4, topMax: 40 },
          ),
    [desktop, reduced],
  )
  const ghosts = useMemo(
    () =>
      reduced
        ? []
        : buildLanternStyles(
            desktop ? 2 : 1,
            desktop
              ? { sizeMin: 38, sizeMax: 56, durationMin: 16, durationMax: 24 }
              : { sizeMin: 28, sizeMax: 36, durationMin: 15, durationMax: 22 },
          ),
    [desktop, reduced],
  )

  if (lanterns.length + bats.length + ghosts.length === 0) return null

  return (
    <div className="cabinet-decor-fx cabinet-decor-fx--halloween" aria-hidden>
      {lanterns.map((s, i) => (
        <span
          key={`j${i}`}
          className={cn('cabinet-hw-jack', !s.inGutter && 'cabinet-hw-sprite--under-content')}
          style={spriteStyle(s, { ['--hw-tilt' as string]: `${s.tilt}deg` })}
        >
          <JackOLanternSvg />
        </span>
      ))}
      {ghosts.map((s, i) => (
        <span
          key={`g${i}`}
          className={cn('cabinet-hw-ghost', !s.inGutter && 'cabinet-hw-sprite--under-content')}
          style={spriteStyle(s, { height: s.size * 1.2 })}
        >
          <GhostSvg />
        </span>
      ))}
      {bats.map((s, i) => (
        <span
          key={`b${i}`}
          className={cn('cabinet-hw-bat', i % 2 === 1 && 'cabinet-hw-bat--rtl')}
          style={spriteStyle(s, { height: s.size / 2 })}
        >
          <span className="cabinet-hw-bat__bob">
            <BatSvg />
          </span>
        </span>
      ))}
    </div>
  )
}

export function HeartsEffect() {
  const desktop = useIsDesktopViewport()
  const buildOpts = useMemo(
    () =>
      desktop
        ? { fontMin: 24, fontMax: 42, durationMin: 9, durationMax: 18, delayMax: 12 }
        : { fontMin: 14, fontMax: 26, durationMin: 9, durationMax: 18, delayMax: 12 },
    [desktop],
  )

  return (
    <FloatingParticles
      char="♥"
      fxClassName="cabinet-decor-fx--valentine"
      particleClassName="cabinet-decor-particle--heart"
      count={particleCount(2)}
      buildOpts={buildOpts}
    />
  )
}

const SPRING_GLYPHS = ['petal', 'leaf', 'blossom', 'sprig'] as const
type SpringGlyph = (typeof SPRING_GLYPHS)[number]

interface WindParticleStyle {
  left: string
  top: string
  size: number
  animationDuration: string
  animationDelay: string
  opacity: number
  glyph: SpringGlyph
  flip: number
}

function buildSpringWindStyles(
  count: number,
  opts?: { sizeMin?: number; sizeMax?: number; durationMin?: number; durationMax?: number },
): WindParticleStyle[] {
  const sizeMin = opts?.sizeMin ?? 14
  const sizeMax = opts?.sizeMax ?? 28
  const durationMin = opts?.durationMin ?? 10
  const durationMax = opts?.durationMax ?? 18
  return Array.from({ length: count }, () => {
    const timing = staggeredTiming(durationMin, durationMax)
    return {
      left: `${randomBetween(-4, 92)}%`,
      top: `${randomBetween(8, 72)}%`,
      size: randomBetween(sizeMin, sizeMax),
      animationDuration: timing.animationDuration,
      animationDelay: timing.animationDelay,
      opacity: randomBetween(0.35, 0.8),
      glyph: SPRING_GLYPHS[Math.floor(Math.random() * SPRING_GLYPHS.length)]!,
      flip: Math.random() > 0.5 ? -1 : 1,
    }
  })
}

function SpringGlyphSvg({ kind }: { kind: SpringGlyph }) {
  switch (kind) {
    case 'petal':
      return (
        <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden className="size-full">
          <ellipse cx="10" cy="10" rx="6" ry="3.5" opacity="0.9" transform="rotate(32 10 10)" />
        </svg>
      )
    case 'leaf':
      return (
        <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden className="size-full">
          <path
            d="M10 3 C6 7 4 11 5 15 C7 13 9 12 10 12 C11 12 13 13 15 15 C16 11 14 7 10 3Z"
            opacity="0.88"
          />
          <path d="M10 12 V17" stroke="currentColor" strokeWidth="1.2" fill="none" opacity="0.5" />
        </svg>
      )
    case 'blossom':
      return (
        <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden className="size-full">
          <circle cx="10" cy="6" r="2.2" opacity="0.85" />
          <circle cx="13.5" cy="9" r="2.2" opacity="0.8" />
          <circle cx="12" cy="13" r="2.2" opacity="0.8" />
          <circle cx="8" cy="13" r="2.2" opacity="0.8" />
          <circle cx="6.5" cy="9" r="2.2" opacity="0.85" />
          <circle cx="10" cy="10" r="1.6" opacity="0.95" />
        </svg>
      )
    case 'sprig':
      return (
        <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden className="size-full">
          <path d="M10 17 V8" stroke="currentColor" strokeWidth="1.3" fill="none" opacity="0.55" />
          <ellipse cx="7" cy="11" rx="3.2" ry="1.8" opacity="0.82" transform="rotate(-35 7 11)" />
          <ellipse cx="13" cy="9.5" rx="3" ry="1.7" opacity="0.78" transform="rotate(28 13 9.5)" />
          <ellipse cx="8.5" cy="7" rx="2.6" ry="1.5" opacity="0.75" transform="rotate(-20 8.5 7)" />
        </svg>
      )
  }
}

export function SpringEffect() {
  const desktop = useIsDesktopViewport()
  const buildOpts = useMemo(
    () =>
      desktop
        ? { sizeMin: 16, sizeMax: 32, durationMin: 10, durationMax: 18 }
        : { sizeMin: 12, sizeMax: 24, durationMin: 9, durationMax: 16 },
    [desktop],
  )
  const n = particleCount(2)
  const styles = useMemo(() => buildSpringWindStyles(n, buildOpts), [n, buildOpts])
  if (styles.length === 0) return null

  return (
    <div className="cabinet-decor-fx cabinet-decor-fx--spring" aria-hidden>
      {styles.map((s, i) => (
        <span
          key={i}
          className={cn(
            'cabinet-decor-particle',
            'cabinet-decor-particle--spring',
            s.glyph === 'petal' || s.glyph === 'blossom'
              ? 'cabinet-decor-particle--spring-bloom'
              : 'cabinet-decor-particle--spring-leaf',
          )}
          style={{
            left: s.left,
            top: s.top,
            width: s.size,
            height: s.size,
            animationDuration: s.animationDuration,
            animationDelay: s.animationDelay,
            ['--spring-flip' as string]: String(s.flip),
          }}
        >
          <SpringGlyphSvg kind={s.glyph} />
        </span>
      ))}
    </div>
  )
}

const BLACK_FRIDAY_MONEY = ['💰', '💵', '$', '%'] as const
const BLACK_FRIDAY_GLYPHS = ['tag', 'money', 'flash'] as const
type BlackFridayGlyph = (typeof BLACK_FRIDAY_GLYPHS)[number]

interface SaleParticleStyle {
  left: string
  top: string
  size: number
  animationDuration: string
  animationDelay: string
  opacity: number
  glyph: BlackFridayGlyph
  char: string
  angle: number
}

function buildSaleParticleStyles(
  count: number,
  opts?: { sizeMin?: number; sizeMax?: number; durationMin?: number; durationMax?: number },
): SaleParticleStyle[] {
  const sizeMin = opts?.sizeMin ?? 18
  const sizeMax = opts?.sizeMax ?? 34
  const durationMin = opts?.durationMin ?? 6
  const durationMax = opts?.durationMax ?? 13
  return Array.from({ length: count }, () => {
    const glyph = BLACK_FRIDAY_GLYPHS[Math.floor(Math.random() * BLACK_FRIDAY_GLYPHS.length)]!
    const timing = staggeredTiming(durationMin, durationMax)
    return {
      left: `${randomBetween(-5, 88)}%`,
      top: `${randomBetween(10, 80)}%`,
      size: randomBetween(sizeMin, sizeMax),
      animationDuration: timing.animationDuration,
      animationDelay: timing.animationDelay,
      opacity: randomBetween(0.45, 0.92),
      glyph,
      char: BLACK_FRIDAY_MONEY[Math.floor(Math.random() * BLACK_FRIDAY_MONEY.length)]!,
      angle: randomBetween(-38, -18),
    }
  })
}

function SaleTagSvg() {
  return (
    <svg viewBox="0 0 32 32" fill="none" aria-hidden className="size-full">
      <path
        d="M6 8 L22 6 L26 22 L10 26 Z"
        fill="currentColor"
        opacity="0.92"
      />
      <circle cx="10" cy="10" r="2.2" fill="hsl(var(--background))" opacity="0.55" />
      <text x="16" y="19" textAnchor="middle" fill="hsl(var(--background))" fontSize="9" fontWeight="700" opacity="0.9">
        %
      </text>
    </svg>
  )
}

export function SparksEffect() {
  const desktop = useIsDesktopViewport()
  const buildOpts = useMemo(
    () =>
      desktop
        ? { sizeMin: 20, sizeMax: 38, durationMin: 7, durationMax: 14 }
        : { sizeMin: 16, sizeMax: 28, durationMin: 6, durationMax: 12 },
    [desktop],
  )
  const n = particleCount(4)
  const styles = useMemo(() => buildSaleParticleStyles(n, buildOpts), [n, buildOpts])
  if (styles.length === 0) return null

  return (
    <div className="cabinet-decor-fx cabinet-decor-fx--black_friday" aria-hidden>
      {styles.map((s, i) => (
        <span
          key={i}
          className={cn(
            'cabinet-decor-particle',
            'cabinet-decor-particle--bf',
            s.glyph === 'flash' && 'cabinet-decor-particle--bf-flash',
            s.glyph === 'tag' && 'cabinet-decor-particle--bf-tag',
            s.glyph === 'money' && 'cabinet-decor-particle--bf-money',
          )}
          style={{
            left: s.left,
            top: s.top,
            width: s.glyph === 'flash' ? s.size * 2.2 : s.size,
            height: s.glyph === 'flash' ? s.size * 2.2 : s.size,
            fontSize: s.glyph === 'money' ? `${s.size}px` : undefined,
            animationDuration: s.animationDuration,
            animationDelay: s.animationDelay,
            opacity: s.opacity,
            ['--bf-angle' as string]: `${s.angle}deg`,
          }}
        >
          {s.glyph === 'tag' ? <SaleTagSvg /> : s.glyph === 'money' ? s.char : null}
        </span>
      ))}
    </div>
  )
}

const AURORA_GLYPHS = ['✦', '✧', '·'] as const
const OCEAN_GLYPHS = ['○', '◦', '∘'] as const
/** Halfwidth katakana + digits — как в digital rain, без латиницы для читаемости UI. */
const CYBER_GLYPHS = [
  '0',
  '1',
  '2',
  '3',
  '5',
  '7',
  '8',
  'ｱ',
  'ｳ',
  'ｴ',
  'ｵ',
  'ｶ',
  'ｷ',
  'ｸ',
  'ｹ',
  'ｺ',
  'ｻ',
  'ｼ',
  'ｽ',
  'ｾ',
  'ｿ',
  'ﾀ',
  'ﾁ',
  'ﾂ',
  'ﾃ',
  'ﾄ',
  'ﾅ',
  'ﾆ',
  'ﾇ',
  'ﾈ',
  'ﾉ',
  'ﾊ',
  'ﾋ',
  'ﾌ',
  'ﾍ',
  'ﾎ',
  'ﾏ',
  'ﾐ',
  'ﾑ',
  'ﾒ',
  'ﾓ',
  'ﾔ',
  'ﾕ',
  'ﾖ',
  'ﾗ',
  'ﾘ',
  'ﾙ',
  'ﾚ',
  'ﾛ',
  'ﾜ',
  'ﾝ',
] as const
const SUNSET_GLYPHS = ['·', '•', '✦'] as const
const LAVENDER_GLYPHS = ['·', '•', '○'] as const

interface MatrixColumnStyle {
  left: string
  fontSize: number
  animationDuration: string
  animationDelay: string
  opacity: number
  glyphs: string[]
}

function pickCyberGlyph(): string {
  return CYBER_GLYPHS[Math.floor(Math.random() * CYBER_GLYPHS.length)]!
}

function buildMatrixColumns(
  count: number,
  opts?: { fontMin?: number; fontMax?: number; trailMin?: number; trailMax?: number; durationMin?: number; durationMax?: number },
): MatrixColumnStyle[] {
  const fontMin = opts?.fontMin ?? 11
  const fontMax = opts?.fontMax ?? 15
  const trailMin = opts?.trailMin ?? 5
  const trailMax = opts?.trailMax ?? 9
  const durationMin = opts?.durationMin ?? 9
  const durationMax = opts?.durationMax ?? 16
  return Array.from({ length: count }, () => {
    const trailLen = Math.floor(randomBetween(trailMin, trailMax + 0.99))
    const timing = staggeredTiming(durationMin, durationMax)
    return {
      left: `${randomBetween(2, 96)}%`,
      fontSize: randomBetween(fontMin, fontMax),
      ...timing,
      opacity: randomBetween(0.22, 0.42),
      glyphs: Array.from({ length: trailLen }, () => pickCyberGlyph()),
    }
  })
}

export function AuroraEffect() {
  const desktop = useIsDesktopViewport()
  const buildOpts = useMemo(
    () =>
      desktop
        ? { fontMin: 10, fontMax: 18, durationMin: 10, durationMax: 20, delayMax: 14 }
        : { fontMin: 8, fontMax: 14, durationMin: 10, durationMax: 18, delayMax: 12 },
    [desktop],
  )

  return (
    <FloatingParticles
      chars={AURORA_GLYPHS}
      fxClassName="cabinet-decor-fx--aurora"
      particleClassName="cabinet-decor-particle--aurora"
      count={particleCount(3)}
      buildOpts={buildOpts}
    />
  )
}

export function BubblesEffect() {
  const desktop = useIsDesktopViewport()
  const buildOpts = useMemo(
    () =>
      desktop
        ? { fontMin: 10, fontMax: 22, durationMin: 10, durationMax: 20, delayMax: 14 }
        : { fontMin: 8, fontMax: 16, durationMin: 9, durationMax: 18, delayMax: 12 },
    [desktop],
  )

  return (
    <FloatingParticles
      chars={OCEAN_GLYPHS}
      fxClassName="cabinet-decor-fx--ocean"
      particleClassName="cabinet-decor-particle--bubble"
      count={particleCount(2)}
      buildOpts={buildOpts}
    />
  )
}

/** Digital rain: колонны строго сверху вниз, без вращения; низкая плотность. */
export function MatrixEffect() {
  const desktop = useIsDesktopViewport()
  const columnCount = useMemo(() => {
    if (typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      return 0
    }
    return desktop ? 7 : 4
  }, [desktop])
  const buildOpts = useMemo(
    () =>
      desktop
        ? { fontMin: 11, fontMax: 14, trailMin: 5, trailMax: 8, durationMin: 10, durationMax: 17 }
        : { fontMin: 10, fontMax: 13, trailMin: 4, trailMax: 6, durationMin: 11, durationMax: 18 },
    [desktop],
  )
  const columns = useMemo(() => buildMatrixColumns(columnCount, buildOpts), [columnCount, buildOpts])
  if (columns.length === 0) return null

  return (
    <div className="cabinet-decor-fx cabinet-decor-fx--cyber" aria-hidden>
      {columns.map((col, i) => (
        <span
          key={i}
          className="cabinet-decor-matrix-col"
          style={{
            left: col.left,
            fontSize: col.fontSize,
            animationDuration: col.animationDuration,
            animationDelay: col.animationDelay,
            opacity: col.opacity,
          }}
        >
          {col.glyphs.map((g, gi) => {
            const fromHead = col.glyphs.length - 1 - gi
            return (
              <span
                key={gi}
                className={cn(
                  'cabinet-decor-matrix-glyph',
                  fromHead === 0 && 'cabinet-decor-matrix-glyph--head',
                  fromHead === 1 && 'cabinet-decor-matrix-glyph--near',
                )}
              >
                {g}
              </span>
            )
          })}
        </span>
      ))}
    </div>
  )
}

export function EmbersEffect() {
  const desktop = useIsDesktopViewport()
  const buildOpts = useMemo(
    () =>
      desktop
        ? { fontMin: 8, fontMax: 16, durationMin: 9, durationMax: 18, delayMax: 12 }
        : { fontMin: 7, fontMax: 13, durationMin: 8, durationMax: 16, delayMax: 10 },
    [desktop],
  )

  return (
    <FloatingParticles
      chars={SUNSET_GLYPHS}
      fxClassName="cabinet-decor-fx--sunset"
      particleClassName="cabinet-decor-particle--ember"
      count={particleCount(3)}
      buildOpts={buildOpts}
    />
  )
}

export function DotsEffect() {
  const desktop = useIsDesktopViewport()
  const buildOpts = useMemo(
    () =>
      desktop
        ? { fontMin: 8, fontMax: 14, durationMin: 12, durationMax: 22, delayMax: 14 }
        : { fontMin: 7, fontMax: 12, durationMin: 11, durationMax: 20, delayMax: 12 },
    [desktop],
  )

  return (
    <FloatingParticles
      chars={LAVENDER_GLYPHS}
      fxClassName="cabinet-decor-fx--lavender"
      particleClassName="cabinet-decor-particle--dot"
      count={particleCount(3)}
      buildOpts={buildOpts}
    />
  )
}
