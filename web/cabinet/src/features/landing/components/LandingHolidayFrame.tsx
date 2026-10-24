import type { ReactNode } from 'react'

import { HolidayAtmosphere, HolidayWeather, useHolidayTheme } from '@/features/decor/HolidayAtmosphere'

/** Общая оболочка лендинга и публичного статуса: своя сцена вместо цветных пятен. */
export function LandingHolidayFrame({ children }: { children: ReactNode }) {
  const holiday = useHolidayTheme()

  return (
    <div className="landing-root" data-holiday={holiday ?? undefined}>
      <div className="landing-backdrop" aria-hidden>
        {holiday ? (
          <HolidayAtmosphere theme={holiday} />
        ) : (
          <>
            <div className="landing-backdrop__wash" />
            <div className="landing-orb landing-orb--cyan" />
            <div className="landing-orb landing-orb--violet" />
            <div className="landing-orb landing-orb--emerald" />
            <div className="landing-orb landing-orb--rose" />
          </>
        )}
      </div>
      {holiday ? <HolidayWeather theme={holiday} /> : null}
      <div className="relative z-10">{children}</div>
    </div>
  )
}

export function useLandingHolidayTrim() {
  return useHolidayTheme()
}
