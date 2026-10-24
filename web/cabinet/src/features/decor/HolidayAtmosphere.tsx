import { useLayoutEffect, useState, type CSSProperties } from 'react'

import { Bat, HalloweenFrieze, NewYearFrieze, PumpkinMoon, SpiderCorner, Witch } from './holidayArt'
import { useCabinetDecorTheme } from './useCabinetDecorTheme'
import './holiday.css'

export type HolidayId = 'halloween' | 'new_year'

/** Фиксированный рой: задержки отрицательные, поэтому мыши уже разлетелись, а не стартуют пачкой. */
const BATS: Array<{
  top: string
  delay: number
  dur: number
  scale: number
  back: boolean
  y1: string
  y2: string
  y3: string
}> = [
  { top: '5%', delay: 2, dur: 17, scale: 0.42, back: false, y1: '0px', y2: '22px', y3: '-16px' },
  { top: '11%', delay: 9, dur: 23, scale: 0.7, back: true, y1: '8px', y2: '-28px', y3: '18px' },
  { top: '8%', delay: 14, dur: 19, scale: 0.55, back: false, y1: '-10px', y2: '30px', y3: '4px' },
  { top: '18%', delay: 4, dur: 26, scale: 0.86, back: false, y1: '12px', y2: '-8px', y3: '36px' },
  { top: '15%', delay: 18, dur: 21, scale: 0.48, back: true, y1: '-18px', y2: '14px', y3: '-6px' },
  { top: '24%', delay: 7, dur: 28, scale: 1, back: false, y1: '0px', y2: '40px', y3: '-22px' },
  { top: '21%', delay: 12, dur: 16, scale: 0.38, back: true, y1: '16px', y2: '-20px', y3: '10px' },
  { top: '29%', delay: 1, dur: 24, scale: 0.62, back: false, y1: '-6px', y2: '18px', y3: '28px' },
  { top: '27%', delay: 16, dur: 20, scale: 0.5, back: true, y1: '20px', y2: '-12px', y3: '-24px' },
  { top: '34%', delay: 6, dur: 31, scale: 0.78, back: false, y1: '4px', y2: '-32px', y3: '14px' },
  { top: '32%', delay: 21, dur: 18, scale: 0.44, back: true, y1: '-14px', y2: '26px', y3: '0px' },
  { top: '13%', delay: 25, dur: 22, scale: 0.58, back: false, y1: '24px', y2: '6px', y3: '-18px' },
  { top: '36%', delay: 10, dur: 27, scale: 0.66, back: true, y1: '-4px', y2: '34px', y3: '-10px' },
  { top: '7%', delay: 28, dur: 15, scale: 0.34, back: false, y1: '10px', y2: '-22px', y3: '20px' },
]

const EMBERS = [
  { left: '12%', bottom: '18%', delay: 0, dur: 9 },
  { left: '28%', bottom: '12%', delay: 3, dur: 12 },
  { left: '70%', bottom: '20%', delay: 1.5, dur: 10 },
  { left: '84%', bottom: '14%', delay: 5, dur: 13 },
  { left: '48%', bottom: '10%', delay: 7, dur: 11 },
]

const FLAKES: Array<[number, number, number, number]> = [
  [4, 0, 16, 7],
  [11, 4, 19, 4],
  [18, 9, 14, 9],
  [26, 2, 21, 5],
  [33, 7, 17, 6],
  [41, 1, 23, 3],
  [49, 6, 15, 8],
  [57, 11, 18, 5],
  [64, 3, 20, 4],
  [72, 8, 16, 10],
  [80, 2, 22, 5],
  [88, 5, 15, 7],
  [94, 10, 19, 4],
  [8, 13, 18, 5],
  [52, 14, 17, 6],
  [76, 12, 21, 3],
]

/** Тема из кабинета, а в dev ещё и из ?decor= для просмотра лендинга и статуса. */
export function useHolidayTheme(): HolidayId | null {
  const theme = useCabinetDecorTheme()
  const [forced, setForced] = useState<HolidayId | null>(null)

  useLayoutEffect(() => {
    if (!import.meta.env.DEV) return
    const raw = new URLSearchParams(window.location.search).get('decor')
    if (raw !== 'halloween' && raw !== 'new_year') return
    const root = document.documentElement
    const previous = root.dataset.cabinetDecor
    root.dataset.cabinetDecor = raw
    setForced(raw)
    return () => {
      if (previous) root.dataset.cabinetDecor = previous
      else delete root.dataset.cabinetDecor
    }
  }, [])

  const id = forced ?? theme
  return id === 'halloween' || id === 'new_year' ? id : null
}

/** Снег и искры поверх стекла лендинга. Клики не перехватывает. */
export function HolidayWeather({ theme }: { theme: HolidayId }) {
  if (theme === 'halloween') {
    return (
      <div className="holiday-weather" aria-hidden>
        {EMBERS.map((ember, i) => (
          <span
            key={i}
            className="holiday-ember"
            style={{ left: ember.left, bottom: ember.bottom, animationDuration: `${ember.dur}s`, animationDelay: `-${ember.delay}s` }}
          />
        ))}
      </div>
    )
  }
  return (
    <div className="holiday-weather" aria-hidden>
      {FLAKES.map(([left, delay, dur, size], i) => (
        <span
          key={i}
          className={i % 5 === 0 ? 'holiday-flake holiday-flake--star' : 'holiday-flake'}
          style={{ left: `${left}%`, width: size, height: size, animationDuration: `${dur}s`, animationDelay: `-${delay}s` }}
        >
          {i % 5 === 0 ? '✶' : null}
        </span>
      ))}
    </div>
  )
}

/** Небо, земля и частицы. Шапка рисуется отдельно, чтобы висеть на липком хедере. */
export function HolidayAtmosphere({ theme }: { theme: HolidayId }) {
  if (theme === 'halloween') return <HalloweenNight />
  return <NewYearNight />
}

export function HolidayTrim({ theme }: { theme: HolidayId }) {
  if (theme === 'halloween') {
    return (
      <div className="holiday-trim" aria-hidden>
        <Bunting />
      </div>
    )
  }
  return (
    <div className="holiday-trim" aria-hidden>
      <Icicles />
    </div>
  )
}

function HalloweenNight() {
  return (
    <div className="holiday-sky holiday-sky--halloween" data-holiday="halloween">
      <SpiderCorner />
      <PumpkinMoon />
      <Witch />
      {BATS.map((bat, i) => (
        <Bat
          key={i}
          style={{
            top: bat.top,
            animationDuration: `${bat.dur}s`,
            animationDelay: `-${bat.delay}s`,
            animationName: bat.back ? 'holiday-bat-back' : 'holiday-bat',
            ['--bat-scale' as string]: String(bat.scale),
            ['--bat-y1' as string]: bat.y1,
            ['--bat-y2' as string]: bat.y2,
            ['--bat-y3' as string]: bat.y3,
          }}
        />
      ))}
      <div className="holiday-embers" aria-hidden>
        {EMBERS.map((ember, i) => (
          <span
            key={i}
            className="holiday-ember"
            style={{ left: ember.left, bottom: ember.bottom, animationDuration: `${ember.dur}s`, animationDelay: `-${ember.delay}s` }}
          />
        ))}
      </div>
      <div className="holiday-ground" aria-hidden>
        <svg className="holiday-hill holiday-hill--hw" viewBox="0 0 1440 120" preserveAspectRatio="none">
          <path fill="currentColor" d="M0 120 V78 C180 40 320 96 540 70 C760 44 900 92 1120 62 C1260 42 1360 70 1440 54 V120Z" />
        </svg>
        <HalloweenFrieze />
      </div>
    </div>
  )
}

function NewYearNight() {
  return (
    <div className="holiday-sky holiday-sky--new-year" data-holiday="new_year">
      <div className="holiday-aurora holiday-aurora--a" aria-hidden />
      <div className="holiday-aurora holiday-aurora--b" aria-hidden />
      <span className="holiday-comet" aria-hidden />
      <div className="holiday-snow" aria-hidden>
        {FLAKES.map(([left, delay, dur, size], i) => (
          <span
            key={i}
            className={i % 5 === 0 ? 'holiday-flake holiday-flake--star' : 'holiday-flake'}
            style={{ left: `${left}%`, width: size, height: size, animationDuration: `${dur}s`, animationDelay: `-${delay}s` }}
          >
            {i % 5 === 0 ? '✶' : null}
          </span>
        ))}
      </div>
      <div className="holiday-ground" aria-hidden>
        <svg className="holiday-hill holiday-hill--ny" viewBox="0 0 1440 120" preserveAspectRatio="none">
          <path fill="currentColor" opacity="0.92" d="M0 120 V70 C160 96 300 40 520 64 C740 88 920 36 1140 58 C1280 72 1360 48 1440 60 V120Z" />
        </svg>
        <NewYearFrieze />
      </div>
    </div>
  )
}

function Bunting() {
  const colors = ['#f08a1e', '#6d4ad4', '#1c1024', '#e23b4a', '#f0c14a']
  return (
    <svg className="holiday-bunting" viewBox="0 0 1200 28" preserveAspectRatio="none" aria-hidden>
      <path d="M0 2 Q600 16 1200 2" fill="none" stroke="#3a2418" strokeWidth="1.4" />
      {colors.concat(colors, colors).map((fill, i) => (
        <path key={i} d={`M${18 + i * 52} 6 L${36 + i * 52} 24 L${54 + i * 52} 6 Z`} fill={fill} />
      ))}
    </svg>
  )
}

function Icicles() {
  const drops = Array.from({ length: 28 }, (_, i) => {
    const x = 12 + i * 43
    const h = 10 + ((i * 17) % 14)
    return `M${x} 0 L${x + 5} 0 L${x + 2.5} ${h} Z`
  }).join(' ')
  return (
    <svg className="holiday-icicles" viewBox="0 0 1200 28" preserveAspectRatio="none" aria-hidden>
      <rect width="1200" height="3" fill="hsl(198 40% 92%)" />
      <path d={drops} fill="hsl(198 50% 96% / 0.9)" />
      {Array.from({ length: 18 }, (_, i) => (
        <circle
          key={i}
          className="holiday-bulb"
          cx={30 + i * 66}
          cy={8}
          r={3.2}
          fill={['#e23b4a', '#3ecf8e', '#f2d36b', '#59b7ff', '#c084fc'][i % 5]}
          style={{ animationDelay: `${(i % 5) * 0.35}s` } as CSSProperties}
        />
      ))}
    </svg>
  )
}
