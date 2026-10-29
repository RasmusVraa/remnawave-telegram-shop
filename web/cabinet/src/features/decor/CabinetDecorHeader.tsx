import type { DecorThemeId } from './decorThemes'
import { holidayForTheme, HolidayTrim } from './HolidayAtmosphere'
import { useCabinetDecorTheme } from './useCabinetDecorTheme'

function NeonHeaderAccent() {
  return (
    <div className="cabinet-decor-neon-header pointer-events-none absolute inset-x-0 bottom-0 h-px" aria-hidden>
      <div className="cabinet-decor-neon-header__line" />
    </div>
  )
}

/** Декор нижнего края sticky-хедера (флажки, сосульки, неон). */
export function CabinetDecorHeader({ theme: themeProp }: { theme?: DecorThemeId } = {}) {
  const fromHook = useCabinetDecorTheme()
  const theme = themeProp ?? fromHook

  const holiday = holidayForTheme(theme)
  if (holiday) return <HolidayTrim theme={holiday} />
  if (theme === 'neon') return <NeonHeaderAccent />
  return null
}
