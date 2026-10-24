import { cn } from '@/lib/utils'

import { HolidayAtmosphere } from './HolidayAtmosphere'

import type { DecorThemeId } from './decorThemes'
import { useCabinetDecorTheme } from './useCabinetDecorTheme'

function SummerTree() {
  return (
    <svg
      className="cabinet-decor-scene__tree"
      viewBox="0 0 120 200"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden
    >
      <path
        d="M60 18 C42 42 28 58 32 78 C18 88 14 108 28 118 C22 132 26 148 44 152 C48 168 52 178 60 182 C68 178 72 168 76 152 C94 148 98 132 92 118 C106 108 102 88 88 78 C92 58 78 42 60 18Z"
        fill="currentColor"
        opacity="0.22"
      />
      <rect x="54" y="178" width="12" height="22" rx="2" fill="currentColor" opacity="0.3" />
    </svg>
  )
}

function SpringScene() {
  return (
    <>
      <svg
        className="cabinet-decor-scene__spring-grass cabinet-decor-scene__spring-grass--bl"
        viewBox="0 0 80 48"
        fill="none"
        aria-hidden
      >
        <path d="M8 44 Q10 28 12 44" stroke="currentColor" strokeWidth="2" strokeLinecap="round" opacity="0.35" />
        <path d="M16 44 Q18 22 20 44" stroke="currentColor" strokeWidth="2" strokeLinecap="round" opacity="0.4" />
        <path d="M24 44 Q26 30 28 44" stroke="currentColor" strokeWidth="2" strokeLinecap="round" opacity="0.32" />
        <path d="M32 44 Q34 18 36 44" stroke="currentColor" strokeWidth="2" strokeLinecap="round" opacity="0.38" />
        <ellipse cx="22" cy="44" rx="18" ry="4" fill="currentColor" opacity="0.12" />
      </svg>
      <svg
        className="cabinet-decor-scene__spring-bush cabinet-decor-scene__spring-bush--br"
        viewBox="0 0 72 56"
        fill="none"
        aria-hidden
      >
        <ellipse cx="36" cy="38" rx="22" ry="14" fill="currentColor" opacity="0.18" />
        <ellipse cx="26" cy="32" rx="14" ry="10" fill="currentColor" opacity="0.14" />
        <ellipse cx="48" cy="30" rx="12" ry="9" fill="currentColor" opacity="0.14" />
        <circle cx="30" cy="28" r="2.5" fill="hsl(var(--primary) / 0.35)" />
        <circle cx="42" cy="26" r="2" fill="hsl(var(--primary) / 0.3)" />
        <circle cx="36" cy="34" r="2" fill="hsl(var(--primary) / 0.28)" />
      </svg>
    </>
  )
}

function ValentineHearts() {
  return (
    <>
      <span className="cabinet-decor-scene__heart cabinet-decor-scene__heart--left" aria-hidden>
        ♥
      </span>
      <span className="cabinet-decor-scene__heart cabinet-decor-scene__heart--right" aria-hidden>
        ♥
      </span>
    </>
  )
}

function AuroraBands() {
  return (
    <>
      <div className="cabinet-decor-scene__aurora-band cabinet-decor-scene__aurora-band--a" aria-hidden />
      <div className="cabinet-decor-scene__aurora-band cabinet-decor-scene__aurora-band--b" aria-hidden />
      <div className="cabinet-decor-scene__aurora-band cabinet-decor-scene__aurora-band--c" aria-hidden />
    </>
  )
}

function OceanWave() {
  return <div className="cabinet-decor-scene__ocean-wave" aria-hidden />
}

function CyberGrid() {
  return (
    <>
      <div className="cabinet-decor-scene__cyber-grid" aria-hidden />
      <div className="cabinet-decor-scene__cyber-scanline" aria-hidden />
    </>
  )
}

function SunsetGlow() {
  return (
    <>
      <div className="cabinet-decor-scene__sunset-orb cabinet-decor-scene__sunset-orb--sun" aria-hidden />
      <div className="cabinet-decor-scene__sunset-orb cabinet-decor-scene__sunset-orb--haze" aria-hidden />
    </>
  )
}

/** wine: густая винная дымка по углам + мягкий блик «бокала» у верхней кромки. */
function WineHaze() {
  return (
    <>
      <div className="cabinet-decor-scene__wine-haze cabinet-decor-scene__wine-haze--tl" aria-hidden />
      <div className="cabinet-decor-scene__wine-haze cabinet-decor-scene__wine-haze--br" aria-hidden />
      <div className="cabinet-decor-scene__wine-sheen" aria-hidden />
    </>
  )
}

function LavenderMist() {
  return (
    <>
      <div className="cabinet-decor-scene__lavender-mist cabinet-decor-scene__lavender-mist--tl" aria-hidden />
      <div className="cabinet-decor-scene__lavender-mist cabinet-decor-scene__lavender-mist--br" aria-hidden />
    </>
  )
}

/**
 * nebula: сетка-«блюпринт» + три дышащих цветных пятна.
 * Тот же приём, что на лендинге, — фон живёт, но не отвлекает от контента.
 */
function NebulaGlow() {
  return (
    <>
      <div className="cabinet-decor-scene__nebula-grid" aria-hidden />
      <div className="cabinet-decor-scene__nebula-orb cabinet-decor-scene__nebula-orb--cyan" aria-hidden />
      <div className="cabinet-decor-scene__nebula-orb cabinet-decor-scene__nebula-orb--violet" aria-hidden />
      <div className="cabinet-decor-scene__nebula-orb cabinet-decor-scene__nebula-orb--teal" aria-hidden />
    </>
  )
}

function SceneContent({ theme }: { theme: DecorThemeId }) {
  switch (theme) {
    case 'new_year':
      return <HolidayAtmosphere theme="new_year" />
    case 'summer':
      return <SummerTree />
    case 'halloween':
      return <HolidayAtmosphere theme="halloween" />
    case 'spring':
      return <SpringScene />
    case 'valentine':
      return <ValentineHearts />
    case 'neon':
      return (
        <>
          <div className="cabinet-decor-scene__neon-grid" aria-hidden />
          <div className="cabinet-decor-scene__neon-scanline" aria-hidden />
        </>
      )
    case 'black_friday':
      return (
        <>
          <div className="cabinet-decor-scene__bf-corner cabinet-decor-scene__bf-corner--tl" aria-hidden />
          <div className="cabinet-decor-scene__bf-corner cabinet-decor-scene__bf-corner--br" aria-hidden />
        </>
      )
    case 'aurora':
      return <AuroraBands />
    case 'nebula':
      return <NebulaGlow />
    case 'ocean':
      return <OceanWave />
    case 'cyber':
      return <CyberGrid />
    case 'sunset':
      return <SunsetGlow />
    case 'lavender':
      return <LavenderMist />
    case 'wine':
      return <WineHaze />
    default:
      return null
  }
}

/** Фоновые силуэты и сцены декор-тем (фиксированный слой, не перекрывает клики). */
export function CabinetDecorScene({ theme: themeProp }: { theme?: DecorThemeId } = {}) {
  const fromHook = useCabinetDecorTheme()
  const theme = themeProp ?? fromHook
  if (theme === 'off') return null

  return (
    <div
      className={cn('cabinet-decor-scene', `cabinet-decor-scene--${theme}`)}
      data-cabinet-decor-scene={theme}
      aria-hidden
    >
      <SceneContent theme={theme} />
    </div>
  )
}
